package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// On Windows and Linux the dashboard is the settings window, so a page there
// may ask for something that gives access: trust a folder, serve one, connect
// sync. The page never decides it. The host shows a dialog the system draws,
// with words from copy.go and facts the host holds, and what gives the access
// (a folder, a file, the sync key) is chosen or typed in that dialog and never
// sent by the page. The page's request is answered "pending" at once; when the
// person says yes the action runs as a tray action and the change reaches the
// page on the snapshot stream. One such dialog is open at a time.
//
// Under the Mac app, or with no tray, none of this runs: the dashboard stays
// refused (needs-confirmation).

// grantArgs is the part of a grant action's arguments the host reads.
type grantArgs struct {
	ID         string `json:"id"`
	ProgramID  string `json:"programID"`
	DocumentID string `json:"documentID"`
	On         *bool  `json:"on"`
	Allow      *bool  `json:"allow"`
	Existing   bool   `json:"existing"`
	Name       string `json:"name"`
	Command    string `json:"command"`
}

// dialogOnly is every argument that names what is granted, or that says the
// person has already seen a warning. Each is chosen or answered in the host's
// own dialog. A page that sends one is refused.
var dialogOnly = []string{"path", "key", "destination", "acknowledged", "confirmed"}

// nativeGrantTypes is every grant a dashboard page may ask for where the host
// confirms with its own dialogs. The other grant actions stay refused there:
// a document's request is answered by the dialog the document already raises,
// and a program is added when a document asks for one.
var nativeGrantTypes = map[string]bool{
	"aiEdit.setCustom":      true,
	"file.open":             true,
	"folder.merge":          true,
	"folder.relink":         true,
	"folder.serve":          true,
	"folder.setFraming":     true,
	"folder.setSynced":      true,
	"folder.setUpTeam":      true,
	"folder.trust":          true,
	"move.start":            true,
	"program.add":           true,
	"program.askAgain":      true,
	"program.openExample":   true,
	"program.relink":        true,
	"program.setAllTrusted": true,
	"program.setOn":         true,
	"sync.connect":          true,
}

// dashboardGrant answers a grant action that came from a dashboard page. It
// never runs the action itself: it either refuses, or starts the dialog and
// answers pending.
func (a *app) dashboardGrant(action appcore.Action) *appcore.Refusal {
	if !a.nativeGrants || !nativeGrantTypes[action.Type] {
		return &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation}
	}
	var args grantArgs
	raw := map[string]any{}
	if len(action.Args) > 0 {
		if json.Unmarshal(action.Args, &args) != nil || json.Unmarshal(action.Args, &raw) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments}
		}
	}
	if raw == nil {
		raw = map[string]any{}
	}
	for _, field := range dialogOnly {
		if _, sent := raw[field]; sent {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: field}
		}
	}
	if action.Type == "program.add" {
		if builtin.Is(args.Name) || args.Name == aiEditProgramID {
			return &appcore.Refusal{Code: appcore.RefuseNameTaken, Field: "name"}
		}
		for _, p := range a.rt.cfg.HelperProgramList() {
			if p.Name == args.Name {
				return &appcore.Refusal{Code: appcore.RefuseNameTaken, Field: "name"}
			}
		}
	}
	if action.Type == "aiEdit.setCustom" {
		if _, refusal := customCommand(args.Command); refusal != nil {
			return refusal
		}
	}
	flow := a.grantFlow(action.Type, args, raw)
	if flow == nil {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments}
	}
	if a.dialogAdvice != "" {
		return &appcore.Refusal{Code: appcore.RefuseNoDialogs}
	}
	if !a.grantOpen.CompareAndSwap(false, true) {
		return &appcore.Refusal{Code: appcore.RefuseDialogOpen}
	}
	go func() {
		defer a.grantOpen.Store(false)
		flow()
		// A new revision whatever the answer was, so the page that shows "answer
		// the dialog" sees that the dialog is closed.
		a.bumpRev()
		a.changed()
	}()
	return &appcore.Refusal{Code: appcore.RefusePending}
}

// grantFlow is the dialog and the action behind one grant, or nil when the
// arguments name nothing to ask about.
func (a *app) grantFlow(typ string, args grantArgs, raw map[string]any) func() {
	snap := a.snapshot()
	folder := func() (appcore.Folder, bool) {
		for _, f := range snap.Folders {
			if f.ID == args.ID && args.ID != "" {
				return f, true
			}
		}
		return appcore.Folder{}, false
	}
	program := func() (appcore.Program, bool) {
		for _, p := range snap.Programs {
			if p.ID == args.ProgramID && args.ProgramID != "" {
				return p, true
			}
		}
		return appcore.Program{}, false
	}
	on := func(flag *bool) bool { return flag != nil && *flag }

	switch typ {
	case "folder.trust":
		return func() { a.pickAndTrustFolder() }
	case "file.open":
		return func() {
			path, ok, err := a.selectFileToOpen()
			if err != nil {
				a.grantDialogFailed(typ, err)
				return
			}
			if ok {
				a.openFile(path)
			}
		}
	case "folder.serve":
		if args.ID == "" {
			return func() { a.pickAndServeFolder() }
		}
		f, ok := folder()
		if !ok {
			return nil
		}
		return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantServe, f.Path), appcore.TextGrantServeYes)
	case "folder.relink":
		f, ok := folder()
		if !ok {
			return nil
		}
		return func() { a.grantRelink(f.ID) }
	case "folder.setUpTeam":
		f, ok := folder()
		if !ok {
			return nil
		}
		return func() { a.grantTeamFolder(f.ID, args.Existing) }
	case "folder.setSynced", "folder.setFraming", "folder.merge":
		f, ok := folder()
		if !ok {
			return nil
		}
		switch {
		case typ == "folder.merge":
			return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantMerge, f.Path), appcore.TextGrantMergeYes)
		case typ == "folder.setSynced" && on(args.On):
			return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantSync, f.Path), appcore.TextGrantSyncYes)
		case typ == "folder.setFraming" && on(args.Allow):
			return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantFraming, f.Path), appcore.TextGrantFramingYes)
		}
		// Turning access off needs no question.
		return func() { a.runGrant(typ, raw) }
	case "program.setAllTrusted", "program.askAgain", "program.relink":
		p, ok := program()
		if !ok {
			return nil
		}
		switch {
		case typ == "program.relink":
			return func() { a.relinkHelperProgramFromTray(p.ID) }
		case typ == "program.askAgain":
			if args.DocumentID == "" {
				return nil
			}
			return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantAskAgain, grantProgramTitle(p), args.DocumentID), appcore.TextGrantAskAgainYes)
		case on(args.On):
			return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantAllTrusted, grantProgramTitle(p)), appcore.TextGrantAllTrustedYes)
		}
		return func() { a.runGrant(typ, raw) }
	case "program.add":
		if !config.ValidHelperName(args.Name) {
			return nil
		}
		return func() { a.grantAddProgram(args.Name) }
	case "program.setOn", "program.openExample":
		p, ok := program()
		if !ok || !p.Builtin {
			return nil
		}
		if typ == "program.openExample" {
			return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantExample, grantProgramTitle(p)), appcore.TextGrantExampleYes)
		}
		if !on(args.On) {
			return nil
		}
		return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantBuiltinOn, grantProgramTitle(p)), appcore.TextGrantBuiltinOnYes)
	case "aiEdit.setCustom":
		return a.confirmedGrant(typ, raw, fmt.Sprintf(appcore.TextGrantCustomCommand, strings.TrimSpace(args.Command)), appcore.TextGrantCustomCommandYes)
	case "move.start":
		return a.confirmedGrant(typ, raw, appcore.TextGrantMove, appcore.TextGrantMoveYes)
	case "sync.connect":
		return func() {
			key, ok, err := a.promptSecret(appcore.TextGrantConnectTitle, appcore.TextGrantConnectMessage)
			if err != nil {
				a.grantDialogFailed(typ, err)
				return
			}
			if key = strings.TrimSpace(key); ok && key != "" {
				raw["key"] = key
				a.runGrant(typ, raw)
			}
		}
	}
	return nil
}

// confirmedGrant asks one yes or no question and runs the action on yes.
func (a *app) confirmedGrant(typ string, raw map[string]any, message, affirmative string) func() {
	return func() {
		ok, err := a.confirmTrayChange(appcore.TextGrantTitle, message, affirmative)
		if err != nil {
			a.grantDialogFailed(typ, err)
			return
		}
		if ok {
			a.runGrant(typ, raw)
		}
	}
}

// grantRelink is folder.relink: the person picks where the folder is now. A
// folder that only moved needs nothing more. Anything the host would warn about
// (a different folder, one of the person's main folders) is shown in the host's
// words, and only a yes there acknowledges it.
func (a *app) grantRelink(folderID string) {
	const typ = "folder.relink"
	path, ok, err := a.selectFolder(appcore.TextGrantRelinkPick)
	if err != nil {
		a.grantDialogFailed(typ, err)
		return
	}
	if !ok {
		return
	}
	preview, refusal := a.folderRelinkPreview(folderID, path)
	if refusal != nil {
		a.grantRefused(typ, refusal)
		return
	}
	codes := []string{}
	if len(preview.Warnings) > 0 {
		texts := make([]string, 0, len(preview.Warnings))
		for _, warning := range preview.Warnings {
			codes = append(codes, warning.Code)
			texts = append(texts, warning.Text)
		}
		yes, err := a.confirmTrayChange(appcore.TextGrantTitle, fmt.Sprintf(appcore.TextGrantRelinkWarn, preview.Path, strings.Join(texts, "\n\n")), appcore.TextGrantRelinkYes)
		if err != nil {
			a.grantDialogFailed(typ, err)
			return
		}
		if !yes {
			return
		}
	}
	a.runGrant(typ, map[string]any{"id": folderID, "path": path, "acknowledged": codes})
}

// grantTeamFolder is folder.setUpTeam: the person picks the folder on this
// computer. Syncing into a folder that already has files is answered with what
// it would do, and only a yes to that confirms it.
func (a *app) grantTeamFolder(folderID string, existing bool) {
	const typ = "folder.setUpTeam"
	path, ok, err := a.selectFolder(appcore.ForSystem(appcore.TextGrantTeamPick, runtime.GOOS))
	if err != nil {
		a.grantDialogFailed(typ, err)
		return
	}
	if !ok {
		return
	}
	args := map[string]any{"id": folderID, "path": path, "existing": existing}
	refusal := a.trayDo(typ, args)
	if refusal != nil && refusal.Code == appcore.RefuseConfirmNeeded && refusal.Counts != nil {
		counts := refusal.Counts
		yes, err := a.confirmTrayChange(appcore.TextGrantTitle, fmt.Sprintf(appcore.TextGrantSyncExisting, path, counts.ToUpload, counts.ToDownload, counts.Differ), appcore.TextGrantSyncYes)
		if err != nil {
			a.grantDialogFailed(typ, err)
			return
		}
		if !yes {
			return
		}
		args["confirmed"] = true
		refusal = a.trayDo(typ, args)
	}
	a.grantRefused(typ, refusal)
}

// grantAddProgram is program.add: the person picks the program file in the
// system's file picker, and one question names the file and any risk the host
// sees in it before the program is added under the name the page chose.
func (a *app) grantAddProgram(name string) {
	const typ = "program.add"
	path, ok, err := a.selectProgramFile()
	if err != nil {
		a.grantDialogFailed(typ, err)
		return
	}
	if !ok {
		return
	}
	codes, refusal := a.programFileWarnings(path)
	if refusal != nil {
		a.grantRefused(typ, refusal)
		return
	}
	message := fmt.Sprintf(appcore.TextGrantAddProgram, name, path)
	if len(codes) > 0 {
		message += "\n\n" + appcore.TextGrantAddProgramRisk
	}
	yes, err := a.confirmTrayChange(appcore.TextGrantTitle, message, appcore.TextGrantAddProgramYes)
	if err != nil {
		a.grantDialogFailed(typ, err)
		return
	}
	if yes {
		a.runGrant(typ, map[string]any{"name": name, "path": path, "acknowledged": codes})
	}
}

// selectProgramFile shows the file picker for a program through the runtime seam.
func (a *app) selectProgramFile() (string, bool, error) {
	if a.rt.selectFile != nil {
		return a.rt.selectFile(appcore.TextGrantProgramPick)
	}
	return a.dialogs().selectFile(appcore.TextGrantProgramPick, "")
}

// grantProgramTitle is what a dialog calls a program: the display title for a
// built-in program, whose name is its internal id.
func grantProgramTitle(p appcore.Program) string {
	if p.ID == aiEditProgramID {
		return "AI editing"
	}
	if p.Builtin {
		return builtin.Title(p.ID)
	}
	return p.Name
}

// customCommand splits a custom AI command and finds its program. A page's
// request is checked with it before its question, and the effect checks again
// when it runs.
func customCommand(command string) ([]string, *appcore.Refusal) {
	argv, err := aiedit.SplitCommand(command)
	if err != nil || len(argv) == 0 {
		return nil, &appcore.Refusal{Code: appcore.RefuseCommandInvalid, Field: "command"}
	}
	if err := aiedit.Find(argv[0]); err != nil {
		return nil, &appcore.Refusal{Code: appcore.RefuseCommandNotFound, Field: "command", OtherName: argv[0]}
	}
	return argv, nil
}

// runGrant runs a grant the person has just approved, as a tray action.
func (a *app) runGrant(typ string, args map[string]any) {
	a.grantRefused(typ, a.trayDo(typ, args))
}

// grantRefused tells the person when an approved grant was refused: the page
// that asked hears nothing more than the snapshot.
func (a *app) grantRefused(typ string, refusal *appcore.Refusal) {
	if refusal == nil {
		return
	}
	a.rt.logger.Printf("Grant %s from the dashboard was refused: %s", typ, refusal.Code)
	_ = a.notifyUser(appcore.TextGrantTitle, appcore.RefusalMessage(refusal))
}

func (a *app) grantDialogFailed(typ string, err error) {
	a.rt.logger.Printf("Could not show the dialog for %s: %v", typ, err)
}

// selectFileToOpen shows the file picker through the runtime seam.
func (a *app) selectFileToOpen() (string, bool, error) {
	if a.rt.selectFile != nil {
		return a.rt.selectFile(appcore.TextGrantOpenPick)
	}
	return platform.ChooseFile(appcore.TextGrantOpenPick)
}

// promptSecret asks for a value the screen must not show, through the runtime seam.
func (a *app) promptSecret(title, message string) (string, bool, error) {
	if a.rt.promptSecret != nil {
		return a.rt.promptSecret(title, message)
	}
	return a.dialogs().promptSecret(title, message)
}

// nativeDashboardSnapshot is the snapshot a dashboard page reads on a host
// that confirms grants with its own dialogs: only the grants with no dialog
// read false.
func nativeDashboardSnapshot(s appcore.Snapshot) appcore.Snapshot {
	can := make(map[string]bool, len(s.Host.Can))
	for typ, ok := range s.Host.Can {
		can[typ] = ok && (appcore.ActionClasses[typ] != appcore.ClassGrant || nativeGrantTypes[typ])
	}
	s.Host.Can = can
	return s
}
