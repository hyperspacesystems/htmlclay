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
	"app.openRecoveredChanges":  ClassSafe,
	"document.open":             ClassSafe,
	"document.reveal":           ClassSafe,
	"document.forget":           ClassSafe,
	"document.export":           ClassSafe,
	"document.versions":         ClassSafe,
	"dashboard.open":            ClassSafe,
	"folder.reveal":             ClassSafe,
	"program.reveal":            ClassSafe,
	"folder.copyAddress":        ClassSafe,
	"waiting.markSeen":          ClassSafe,
	"request.show":              ClassSafe,
	"prefs.set":                 ClassSafe,
	"sync.setPaused":            ClassSafe,
	"sync.revealLog":            ClassSafe,
	"conflict.resolve":          ClassSafe,
	"folder.resolveRemoval":     ClassSafe,
	"update.start":              ClassSafe,
	"service.restart":           ClassSafe,
	"host.quit":                 ClassSafe,
	"move.quitLocal":            ClassSafe,
	"app.revealLog":             ClassSafe,
	"update.check":              ClassSafe,
	"host.setPort":              ClassSafe,
	"aiEdit.setModel":           ClassSafe,
	"aiEdit.setEffort":          ClassSafe,
	"fileTypes.makeDefault":     ClassSafe,
	"fileTypes.setHTMLOpenWith": ClassSafe,
	"fileTypes.stopOpening":     ClassSafe,
	"template.create":           ClassSafe,

	"folder.stopServing":      ClassRevoke,
	"folder.stopTrusting":     ClassRevoke,
	"program.removeAccess":    ClassRevoke,
	"program.removeAllAccess": ClassRevoke,
	"program.remove":          ClassRevoke,
	"sync.disconnect":         ClassRevoke,

	"folder.trust":        ClassGrant,
	"folder.serve":        ClassGrant,
	"folder.merge":        ClassGrant,
	"folder.relink":       ClassGrant,
	"file.open":           ClassGrant,
	"document.recover":    ClassGrant,
	"request.answer":      ClassGrant,
	"program.add":         ClassGrant,
	"program.relink":      ClassGrant,
	"program.askAgain":    ClassGrant,
	"sync.connect":        ClassGrant,
	"folder.setUpTeam":    ClassGrant,
	"move.start":          ClassGrant,
	"aiEdit.setCustom":    ClassGrant,
	"program.openExample": ClassGrant,

	// These four are a revoke when their switch is false and a grant when it is
	// true; ClassOf reads the argument.
	"program.setAllTrusted": ClassGrant,
	"folder.setSynced":      ClassGrant,
	"folder.setFraming":     ClassGrant,
	"program.setOn":         ClassGrant,
}

// ClassOf is the class of one action, reading the switch for the types whose class
// depends on it: "on" for two, "allow" for folder.setFraming. An unknown type is 0.
func ClassOf(a Action) Class {
	class := ActionClasses[a.Type]
	if a.Type == "program.setAllTrusted" || a.Type == "folder.setSynced" || a.Type == "program.setOn" {
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
	"aiEditTool":  "string",
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
	RequestBuiltinProgram:  {"deny", "allow-document"},
}

// AnswersFor is the answers a request of kind offers, in button order.
func AnswersFor(kind string) []string { return slices.Clone(requestAnswers[kind]) }

// Effect is one change Plan decided on. The app runs each with code it already
// has (cmd/htmlclay/core_effects.go). Only the fields its Type uses are set.
type Effect struct {
	Type       string          `json:"type"`
	FolderID   string          `json:"folderID,omitempty"`
	ParentID   string          `json:"parentID,omitempty"`
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
	// Existing is folder.setUpTeam syncing into a folder that already has files;
	// Confirmed is the person having seen what that will do.
	Existing  bool `json:"existing,omitempty"`
	Confirmed bool `json:"confirmed,omitempty"`
	// Conflict is conflict.resolve's held conflict (its EventFact id, which
	// starts with the sync session's id) and Keep the copy the person chose.
	Conflict string `json:"conflict,omitempty"`
	Keep     string `json:"keep,omitempty"`
	// Choice is folder.resolveRemoval's: restore the attachments here, or
	// delete them on the server too.
	Choice string `json:"choice,omitempty"`
	// Acknowledged is what the sheet showed about the file or the folder: a code
	// the effect finds and this list does not hold refuses needs-confirmation.
	Acknowledged []string `json:"acknowledged,omitempty"`
	Port         int      `json:"port,omitempty"`
	Tool         string   `json:"tool,omitempty"`
	Model        string   `json:"model,omitempty"`
	Effort       string   `json:"effort,omitempty"`
	Command      string   `json:"command,omitempty"`
	TemplateID   string   `json:"templateID,omitempty"`
}

// actionArgs is every argument any action takes; each action reads its own.
// DocumentID is a document's id for the document actions, and the document's path
// for the program actions, whose decisions are keyed by the path.
type actionArgs struct {
	ID          string          `json:"id"`
	IDs         []string        `json:"ids"`
	Path        string          `json:"path"`
	Name        string          `json:"name"`
	Route       string          `json:"route"`
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	On          *bool           `json:"on"`
	Allow       *bool           `json:"allow"`
	Paused      *bool           `json:"paused"`
	Force       bool            `json:"force"`
	ProgramID   string          `json:"programID"`
	DocumentID  string          `json:"documentID"`
	Revision    uint64          `json:"revision"`
	Answer      string          `json:"answer"`
	Username    string          `json:"username"`
	FolderIDs   []string        `json:"folderIDs"`
	Destination string          `json:"destination"`
	Existing    bool            `json:"existing"`
	Confirmed   bool            `json:"confirmed"`
	Keep        string          `json:"keep"`
	Choice      string          `json:"choice"`
	// Acknowledged is the warnings a sheet showed, for the two actions that
	// register a program file, for a folder relink, and for trusting a folder.
	Acknowledged []string `json:"acknowledged"`
	Port         int      `json:"port"`
	Tool         string   `json:"tool"`
	Model        string   `json:"model"`
	Effort       string   `json:"effort"`
	Command      string   `json:"command"`
	TemplateID   string   `json:"templateID"`
}

// conflictChoices is which copies Local's resolveConflict can keep for each
// held conflict kind (08-sync.md 8.0h; execute.js resolveConflict): both
// copies only where a server copy is kept beside the file.
var conflictChoices = map[string][]string{
	"both-edited":    {"mine", "theirs", "both"},
	"rejected":       {"mine", "theirs", "both"},
	"remote-deleted": {"mine", "theirs"},
	"unbound":        {"mine", "theirs", "both"},
	"name-taken":     {"mine"},
}

// ConflictChoices answers the choices a held conflict of kind offers, in the
// order a screen shows them; an unknown kind offers none.
func ConflictChoices(kind string) []string { return slices.Clone(conflictChoices[kind]) }

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
	case "document.open", "document.reveal", "document.forget", "document.export", "document.versions":
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
	case "document.recover":
		if args.ID == "" {
			return bad("id")
		}
		if args.Destination == "" {
			return bad("destination")
		}
		if !hasDocument(f, args.ID) {
			return nil, &Refusal{Code: RefuseNoSuchDocument, SubjectID: args.ID}
		}
		return one(Effect{Type: a.Type, DocumentID: args.ID, Path: args.Destination})
	case "dashboard.open":
		return one(Effect{Type: a.Type, Route: args.Route})
	case "sync.revealLog":
		return one(Effect{Type: a.Type})
	case "app.openRecoveredChanges", "app.revealLog", "update.check", "fileTypes.makeDefault", "fileTypes.stopOpening":
		return one(Effect{Type: a.Type})
	case "fileTypes.setHTMLOpenWith":
		if args.On == nil {
			return bad("on")
		}
		return one(Effect{Type: a.Type, On: *args.On})
	case "host.setPort":
		if args.Port < 1024 || args.Port > 65535 {
			return nil, &Refusal{Code: RefusePortInvalid, Field: "port"}
		}
		if args.Port == f.Host.Port {
			return nil, &Refusal{Code: RefusePortSame, Field: "port"}
		}
		return one(Effect{Type: a.Type, Port: args.Port, Confirmed: args.Confirmed})
	case "program.setOn", "program.openExample":
		p, ok := programByID(f, args.ProgramID)
		if !ok || !p.Builtin || p.ID == AIEditProgram {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if a.Type == "program.openExample" {
			if !p.On {
				return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: p.ID}
			}
			return one(Effect{Type: a.Type, ProgramID: p.ID})
		}
		if args.On == nil {
			return bad("on")
		}
		return one(Effect{Type: a.Type, ProgramID: p.ID, On: *args.On})
	case "aiEdit.setModel":
		tool, ok := aiTool(f, args.Tool)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchTool, Field: "tool"}
		}
		if tool.Custom {
			return nil, &Refusal{Code: RefuseNotAllowedNow, Field: "tool"}
		}
		return one(Effect{Type: a.Type, Tool: tool.ID, Model: args.Model})
	case "aiEdit.setEffort":
		tool, ok := aiTool(f, args.Tool)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchTool, Field: "tool"}
		}
		if len(tool.Efforts) == 0 {
			return nil, &Refusal{Code: RefuseNotAllowedNow, Field: "tool"}
		}
		if args.Effort != "" && !slices.Contains(tool.Efforts, args.Effort) {
			return bad("effort")
		}
		return one(Effect{Type: a.Type, Tool: tool.ID, Effort: args.Effort})
	case "template.create":
		if args.TemplateID == "" {
			return bad("templateID")
		}
		return one(Effect{Type: a.Type, TemplateID: args.TemplateID})
	case "aiEdit.setCustom":
		if strings.TrimSpace(args.Command) == "" {
			return bad("command")
		}
		return one(Effect{Type: a.Type, Command: args.Command})
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
	case "conflict.resolve":
		var held *EventFact
		for i := range f.Events {
			if f.Events[i].Kind == WaitingConflict && f.Events[i].ID == args.ID {
				held = &f.Events[i]
			}
		}
		if held == nil {
			return nil, &Refusal{Code: RefuseConflictGone}
		}
		if args.Path != "" && args.Path != held.Name {
			return nil, &Refusal{Code: RefuseConflictChanged, SubjectID: held.SubjectID}
		}
		if !slices.Contains(ConflictChoices(held.Detail), args.Keep) {
			return bad("keep")
		}
		if f.Sync.KeyStopped {
			return nil, &Refusal{Code: RefuseSyncKeyStopped, SubjectID: held.SubjectID}
		}
		// A folder switched off, or a disconnected account, has no running
		// session to take the choice, so the conflict waits until sync is back.
		if f.Sync.Paused || !f.Sync.Connected || !folderSyncing(f, held.SubjectID) {
			return nil, &Refusal{Code: RefuseSyncPaused, SubjectID: held.SubjectID}
		}
		return one(Effect{Type: a.Type, FolderID: held.SubjectID, Path: held.Name, Conflict: held.ID, Keep: args.Keep})
	case "folder.resolveRemoval":
		held := false
		for _, e := range f.Events {
			held = held || (e.Kind == WaitingRemovalHeld && e.SubjectID == args.ID)
		}
		if !held {
			return nil, &Refusal{Code: RefuseRemovalGone, SubjectID: args.ID}
		}
		if args.Choice != "restore" && args.Choice != "delete" {
			return bad("choice")
		}
		if f.Sync.KeyStopped {
			return nil, &Refusal{Code: RefuseSyncKeyStopped, SubjectID: args.ID}
		}
		if f.Sync.Paused {
			return nil, &Refusal{Code: RefuseSyncPaused, SubjectID: args.ID}
		}
		return one(Effect{Type: a.Type, FolderID: args.ID, Choice: args.Choice})
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

	case "folder.stopServing", "folder.stopTrusting":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		can, why := folderRules(f, ff, folderState(ff))
		allowed, because := can.StopServing, (*Reason)(nil)
		if a.Type == "folder.stopTrusting" {
			allowed, because = can.StopTrusting, why.StopTrusting
		}
		if !allowed {
			return nil, refusedBecause(because, ff.ID)
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID})
	case "folder.merge":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		can, why := folderRules(f, ff, folderState(ff))
		if !can.Merge {
			return nil, refusedBecause(why.Merge, ff.ID)
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID, ParentID: ff.ParentID})
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
		if args.DocumentID == "" {
			return bad("documentID")
		}
		// DocumentID is the document's path, which is what a decision is keyed
		// by, so a document that is not open can be removed too.
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID, DocumentID: args.DocumentID})
	case "program.removeAllAccess":
		if _, ok := programByID(f, args.ProgramID); !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID})
	case "program.remove":
		program, ok := programByID(f, args.ProgramID)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if program.Builtin {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: args.ProgramID}
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID})
	case "program.reveal":
		program, ok := programByID(f, args.ProgramID)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if program.Builtin {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: args.ProgramID}
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
		return one(Effect{Type: a.Type, Path: args.Path, Acknowledged: args.Acknowledged})
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
	case "request.show":
		r, ok := requestByID(f, args.ID)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchRequest, SubjectID: args.ID}
		}
		return one(Effect{Type: a.Type, RequestID: r.ID})
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
		program, ok := programByID(f, args.ProgramID)
		if !ok {
			return nil, &Refusal{Code: RefuseNoSuchProgram, SubjectID: args.ProgramID}
		}
		if program.Builtin {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: args.ProgramID}
		}
		if args.Path == "" {
			return bad("path")
		}
		return one(Effect{Type: a.Type, ProgramID: args.ProgramID, Path: args.Path, Acknowledged: args.Acknowledged})
	case "sync.connect":
		// The username is optional (08-sync.md decision 9): blank takes the key's
		// own account, and a typed one must match it, which only the server knows.
		if args.Key == "" {
			return bad("key")
		}
		return one(Effect{Type: a.Type, Username: args.Username, Key: args.Key})
	case "folder.setUpTeam":
		ff, refusal := folderArg(f, args.ID)
		if refusal != nil {
			return nil, refusal
		}
		if ff.OnThisComputer || ff.Sync.State == FolderSyncPaused {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: ff.ID}
		}
		// A team's first sync always starts in an empty folder (08-sync.md, 8.20):
		// only the person's own account may sync into files already there.
		if args.Existing && ff.Origin.Kind == OriginTeam {
			return nil, &Refusal{Code: RefuseNotAllowedNow, SubjectID: ff.ID, Field: "existing"}
		}
		if args.Path == "" {
			return nil, &Refusal{Code: RefuseBadArguments, Field: "path"}
		}
		return one(Effect{Type: a.Type, FolderID: ff.ID, Path: args.Path, Existing: args.Existing, Confirmed: args.Confirmed})
	case "move.start":
		if f.Move == nil || f.Move.State != MoveOffered {
			return nil, &Refusal{Code: RefuseNothingToMove}
		}
		return one(Effect{Type: a.Type, IDs: args.FolderIDs})
	case "move.quitLocal":
		if f.Move == nil {
			return nil, &Refusal{Code: RefuseNothingToMove}
		}
		return one(Effect{Type: a.Type})
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

func aiTool(f Facts, id string) (AIEditTool, bool) {
	for _, t := range f.Preferences.AIEditTools {
		if t.ID == id {
			return t, true
		}
	}
	return AIEditTool{}, false
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

// folderSyncing is whether the folder's sync is switched on.
func folderSyncing(f Facts, folderID string) bool {
	for _, ff := range f.Folders {
		if ff.ID == folderID {
			return ff.Sync.State != "" && ff.Sync.State != FolderSyncOff
		}
	}
	return false
}
