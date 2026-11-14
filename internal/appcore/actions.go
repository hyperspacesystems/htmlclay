package appcore

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/panphora/htmlclay/internal/config"
)

// Class is how an action changes what documents and programs may do. It decides
// who may send it: the Mac app and the tray may send every class; the dashboard,
// a browser page, sends safe and revoke actions directly, and a grant only goes
// through the host's confirmer (a native dialog or the Mac app's window).
type Class int

const (
	ClassSafe   Class = iota + 1 // changes no access
	ClassRevoke                  // access goes down
	ClassGrant                   // access goes up
)

func (c Class) String() string {
	switch c {
	case ClassSafe:
		return "safe"
	case ClassRevoke:
		return "revoke"
	case ClassGrant:
		return "grant"
	}
	return "unknown"
}

// Action is one thing a person asked for. ID makes a repeat harmless: the host
// answers a repeated id with its first answer.
type Action struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Args json.RawMessage `json:"args"`
}

// ActionClasses is every action type. An action type missing here is refused, and
// TestPlanEveryTypeHasAClass fails when Plan knows a type this table does not.
var ActionClasses = map[string]Class{
	"document.open":      ClassSafe,
	"document.reveal":    ClassSafe,
	"document.forget":    ClassSafe,
	"dashboard.open":     ClassSafe,
	"folder.reveal":      ClassSafe,
	"program.reveal":     ClassSafe,
	"folder.copyAddress": ClassSafe,
	"waiting.markSeen":   ClassSafe,
	"prefs.set":          ClassSafe,
	"sync.setPaused":     ClassSafe,
	"update.start":       ClassSafe,
	"service.restart":    ClassSafe,
	"host.quit":          ClassSafe,

	"folder.stopServing":   ClassRevoke,
	"folder.stopTrusting":  ClassRevoke,
	"folder.merge":         ClassRevoke,
	"program.removeAccess": ClassRevoke,
	"program.remove":       ClassRevoke,
	"sync.disconnect":      ClassRevoke,

	"folder.trust":     ClassGrant,
	"folder.serve":     ClassGrant,
	"folder.relink":    ClassGrant,
	"file.open":        ClassGrant,
	"request.answer":   ClassGrant,
	"program.add":      ClassGrant,
	"program.relink":   ClassGrant,
	"program.askAgain": ClassGrant,
	"sync.connect":     ClassGrant,
	"folder.setUpTeam": ClassGrant,
	"move.start":       ClassGrant,

	// These three are a revoke when their switch is false and a grant when it is
	// true; ClassOf reads the argument.
	"program.setAllTrusted": ClassGrant,
	"folder.setSynced":      ClassGrant,
	"folder.setFraming":     ClassGrant,
}

// ClassOf is the class of one action, reading the switch for the types whose class
// depends on it: "on" for two, "allow" for folder.setFraming. An unknown type is 0.
func ClassOf(a Action) Class {
	class := ActionClasses[a.Type]
	if a.Type == "program.setAllTrusted" || a.Type == "folder.setSynced" {
		var args struct {
			On bool `json:"on"`
		}
		if json.Unmarshal(a.Args, &args) == nil && !args.On {
			return ClassRevoke
		}
	}
	if a.Type == "folder.setFraming" {
		var args struct {
			Allow bool `json:"allow"`
		}
		if json.Unmarshal(a.Args, &args) == nil && !args.Allow {
			return ClassRevoke
		}
	}
	return class
}

// Preferences prefs.set accepts, with the JSON type of each value.
var preferenceKeys = map[string]string{
	"openAtLogin": "bool",
	"aiEditing":   "bool",
	"appearance":  "appearance",
	"yourName":    "string",
	"useName":     "bool",
	"autoUpdate":  "bool",
}

// requestAnswers is what each request kind offers, the preview's request window
// (macos/Sources/PreviewUI/Screens/Request/RequestBody.swift).
var requestAnswers = map[string][]string{
	RequestFound:           {"deny", "allow-document", "allow-all-trusted", "choose-file"},
	RequestNoFile:          {"deny", "choose-file"},
	RequestTeamDocument:    {"deny", "allow-document", "choose-file"},
	RequestRiskyFile:       {"deny", "choose-file", "allow-anyway"},
	RequestFolderRead:      {"deny", "allow-once", "trust-folder"},
	RequestFolderReadAgain: {"deny", "trust-folder"},
}

// AnswersFor is the answers a request of kind offers, in button order.
func AnswersFor(kind string) []string { return slices.Clone(requestAnswers[kind]) }

// Effect is one change Plan decided on. The app runs each with code it already
// has (cmd/htmlclay/core_effects.go). Only the fields its Type uses are set.
type Effect struct {
	Type       string          `json:"type"`
	FolderID   string          `json:"folderID,omitempty"`
	Path       string          `json:"path,omitempty"`
	Name       string          `json:"name,omitempty"`
	ProgramID  string          `json:"programID,omitempty"`
	DocumentID string          `json:"documentID,omitempty"`
	RequestID  string          `json:"requestID,omitempty"`
	Revision   uint64          `json:"revision,omitempty"`
	Answer     string          `json:"answer,omitempty"`
	Key        string          `json:"key,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	On         bool            `json:"on,omitempty"`
	IDs        []string        `json:"ids,omitempty"`
	Route      string          `json:"route,omitempty"`
	Username   string          `json:"username,omitempty"`
	Force      bool            `json:"force,omitempty"`
	// Acknowledged is what the sheet showed about the file: a code the effect
	// finds and this list does not hold refuses needs-confirmation.
	Acknowledged []string `json:"acknowledged,omitempty"`
}

// actionArgs is every argument any action takes; each action reads its own.
// DocumentID is a document's id for the document actions, and the document's path
// for the program actions, whose decisions are keyed by the path.
type actionArgs struct {
	ID         string          `json:"id"`
	IDs        []string        `json:"ids"`
	Path       string          `json:"path"`
	Name       string          `json:"name"`
	Route      string          `json:"route"`
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	On         *bool           `json:"on"`
	Allow      *bool           `json:"allow"`
	Paused     *bool           `json:"paused"`
	Force      bool            `json:"force"`
	ProgramID  string          `json:"programID"`
	DocumentID string          `json:"documentID"`
	Revision   uint64          `json:"revision"`
	Answer     string          `json:"answer"`
	Username   string          `json:"username"`
	FolderIDs  []string        `json:"folderIDs"`
	// Acknowledged is the warnings a sheet showed, for the two actions that
	// register a program file and for a folder relink.
	Acknowledged []string `json:"acknowledged"`
}

// Plan decides an action against the facts: the effects to run, or why not. It
// checks what the snapshot can show (a folder exists, can.serve is true, a request
// offered that answer at that revision); the effects recheck anything that depends
// on the disk, such as appcore.CheckServe for a new folder.
func Plan(f Facts, a Action) ([]Effect, *Refusal) {
	if _, ok := ActionClasses[a.Type]; !ok {
		return nil, &Refusal{Code: RefuseUnknownAction}
	}
	var args actionArgs
	if len(a.Args) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(a.Args)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&args); err != nil {
			return nil, &Refusal{Code: RefuseBadArguments}
		}
	}
	bad := func(field string) ([]Effect, *Refusal) {
		return nil, &Refusal{Code: RefuseBadArguments, Field: field}
	}
	one := func(e Effect) ([]Effect, *Refusal) { return []Effect{e}, nil }

	switch a.Type {
	case "document.open", "document.reveal", "document.forget":
		if args.ID == "" {
			return bad("id")
		}
		if !hasDocument(f, args.ID) {
			return nil, &Refusal{Code: RefuseNoSuchDocument, SubjectID: args.ID}
		}
		return one(Effect{Type: a.Type, DocumentID: args.ID})
	case "file.open":
		if args.Path == "" {
			return bad("path")
		}
		return one(Effect{Type: a.Type, Path: args.Path})
	case "dashboard.open":
		return one(Effect{Type: a.Type, Route: args.Route})
	case "folder.reveal", "folder.copyAddress":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		if a.Type == "folder.copyAddress" && !ff.Served {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: ff.ID}
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID})
	case "waiting.markSeen":
		if len(args.IDs) == 0 {
			return bad("ids")
		}
		return one(Effect{Type: a.Type, IDs: args.IDs})
	case "prefs.set":
		kind, ok := preferenceKeys[args.Key]
		if !ok {
			return nil, &Refusal{Code: RefuseUnknownPreference, Field: "key"}
		}
		if !valueIs(args.Value, kind) {
			return bad("value")
		}
		return one(Effect{Type: a.Type, Key: args.Key, Value: args.Value})
	case "sync.setPaused":
		if args.Paused == nil {
			return bad("paused")
		}
		if !f.Sync.Connected {
			return nil, &Refusal{Code: RefuseNotConnected}
		}
		return one(Effect{Type: a.Type, On: *args.Paused})
	case "update.start":
		if f.Update.Ready == nil {
			return nil, &Refusal{Code: RefuseNothingReady}
		}
		return one(Effect{Type: a.Type})
	case "service.restart":
		return one(Effect{Type: a.Type})
	case "host.quit":
		return one(Effect{Type: a.Type, Force: args.Force})

	case "folder.stopServing", "folder.stopTrusting", "folder.merge":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		can, why := folderRules(f, ff, folderState(ff))
		allowed, because := can.StopServing, (*Reason)(nil)
		switch a.Type {
		case "folder.stopTrusting":
			allowed, because = can.StopTrusting, why.StopTrusting
		case "folder.merge":
			allowed, because = can.Merge, why.Merge
		}
		if !allowed {
			return nil, refusedBecause(because, ff.ID)
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID})
	case "folder.setSynced":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		if args.On == nil {
			return bad("on")
		}
		can, why := folderRules(f, ff, folderState(ff))
		if *args.On && !can.Sync {
			return nil, refusedBecause(why.Sync, ff.ID)
		}
		if !*args.On && !can.StopSyncing {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: ff.ID}
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID, On: *args.On})
	case "folder.setFraming":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		if args.Allow == nil {
			return bad("allow")
		}
		can, _ := folderRules(f, ff, folderState(ff))
		if *args.Allow && !can.SetFraming {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: ff.ID}
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID, On: *args.Allow})
	case "program.removeAccess":
		if _, ok := programByID(f, args.ProgramID); !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		// An empty document takes the program away from every document that has
		// it. DocumentID is the document's path, which is what a decision is keyed
		// by, so a document that is not open can be removed too.
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID, DocumentID: args.DocumentID})
	case "program.remove":
		if _, ok := programByID(f, args.ProgramID); !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID})
	case "program.reveal":
		if _, ok := programByID(f, args.ProgramID); !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID})
	case "program.askAgain":
		program, ok := programByID(f, args.ProgramID)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if args.DocumentID == "" {
			return bad("documentID")
		}
		return one(Effect{Type: a.Type, ProgramID: program.ID, DocumentID: args.DocumentID, Name: program.Name})
	case "program.setAllTrusted":
		if _, ok := programByID(f, args.ProgramID); !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if args.On == nil {
			return bad("on")
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID, On: *args.On})
	case "sync.disconnect":
		if !f.Sync.Connected {
			return nil, &Refusal{Code: RefuseNotConnected}
		}
		return one(Effect{Type: a.Type})

	case "folder.trust":
		if args.Path == "" {
			return bad("path")
		}
		return one(Effect{Type: a.Type, Path: args.Path})
	case "folder.relink":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		if args.Path == "" {
			return bad("path")
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID, Path: args.Path, Acknowledged: args.Acknowledged})
	case "folder.serve":
		if args.ID != "" {
			ff, refusal := folderArg(f, args.ID)
			if refusal != nil {
				return nil, refusal
			}
			can, why := folderRules(f, ff, folderState(ff))
			if !can.Serve {
				return nil, refusedBecause(why.Serve, ff.ID)
			}
			return one(Effect{Type: a.Type, FolderID: ff.ID, Path: ff.Path, Name: args.Name})
		}
		if args.Path == "" {
			return bad("path")
		}
		return one(Effect{Type: a.Type, Path: args.Path, Name: args.Name})
	case "request.answer":
		r, ok := requestByID(f, args.ID)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchRequest, SubjectID: args.ID}
		}
		if args.Revision != r.Revision {
			return nil, &Refusal{Code: RefuseRequestChanged, SubjectID: r.ID}
		}
		if !slices.Contains(r.Answers, args.Answer) {
			return nil, &Refusal{Code: RefuseAnswerNotOffered, Field: "answer", SubjectID: r.ID}
		}
		return one(Effect{Type: a.Type, RequestID: r.ID, Revision: r.Revision, Answer: args.Answer, Path: args.Path})
	case "program.add":
		if args.Path == "" {
			return bad("path")
		}
		if !config.ValidHelperName(args.Name) {
			return bad("name")
		}
		return one(Effect{Type: a.Type, Path: args.Path, Name: args.Name, Acknowledged: args.Acknowledged})
	case "program.relink":
		if _, ok := programByID(f, args.ProgramID); !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if args.Path == "" {
			return bad("path")
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID, Path: args.Path, Acknowledged: args.Acknowledged})
	case "sync.connect":
		if args.Username == "" {
			return bad("username")
		}
		if args.Key == "" {
			return bad("key")
		}
		return one(Effect{Type: a.Type, Username: args.Username, Key: args.Key})
	case "folder.setUpTeam":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		if ff.OnThisComputer {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: ff.ID}
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID, Path: args.Path})
	case "move.start":
		if f.Move == nil || f.Move.State != MoveOffered {
			return nil, &Refusal{Code: RefuseNothingToMove}
		}
		return one(Effect{Type: a.Type, IDs: args.FolderIDs})
	}
	return nil, &Refusal{Code: RefuseUnknownAction}
}

func folderArg(f Facts, id string) (FolderFact, *Refusal) {
	if id == "" {
		return FolderFact{}, &Refusal{Code: RefuseBadArguments, Field: "id"}
	}
	for _, ff := range f.Folders {
		if ff.ID == id {
			return ff, nil
		}
	}
	return FolderFact{}, &Refusal{Code: RefuseNoSuchFolder, SubjectID: id}
}

// refusedBecause turns a folder rule's reason into a refusal, keeping its code.
func refusedBecause(why *Reason, subject string) *Refusal {
	if why == nil {
		return &Refusal{Code: RefuseNotAllowedNow, SubjectID: subject}
	}
	return &Refusal{Code: why.Code, SubjectID: subject, Text: why.Text}
}

func hasDocument(f Facts, id string) bool {
	for _, d := range f.Documents {
		if d.ID == id {
			return true
		}
	}
	return false
}

func programByID(f Facts, id string) (Program, bool) {
	for _, p := range f.Programs {
		if p.ID == id {
			return p, true
		}
	}
	return Program{}, false
}

func requestByID(f Facts, id string) (RequestFact, bool) {
	for _, r := range f.Requests {
		if r.ID == id {
			return r, true
		}
	}
	return RequestFact{}, false
}

func valueIs(raw json.RawMessage, kind string) bool {
	switch kind {
	case "bool":
		var v bool
		return json.Unmarshal(raw, &v) == nil
	case "string":
		var v string
		return json.Unmarshal(raw, &v) == nil
	case "appearance":
		var v string
		return json.Unmarshal(raw, &v) == nil && (v == AppearanceSystem || v == AppearanceLight || v == AppearanceDark)
	}
	return false
}
