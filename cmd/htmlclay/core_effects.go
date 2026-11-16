package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/browser"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/versions"
)

// implementedActions is every action type runEffect has a case for. The snapshot
// reports it as host.can; TestImplementedActionsMatchRunEffect keeps the two equal.
var implementedActions = []string{
	"app.openKeptPage",
	"dashboard.open",
	"document.export",
	"document.forget",
	"document.open",
	"document.recover",
	"document.reveal",
	"document.versions",
	"file.open",
	"folder.copyAddress",
	"folder.merge",
	"folder.relink",
	"folder.reveal",
	"folder.serve",
	"folder.setFraming",
	"folder.setSynced",
	"folder.setUpTeam",
	"folder.stopServing",
	"folder.stopTrusting",
	"folder.trust",
	"host.quit",
	"prefs.set",
	"program.add",
	"program.askAgain",
	"program.relink",
	"program.remove",
	"program.removeAccess",
	"program.reveal",
	"program.setAllTrusted",
	"request.answer",
	"request.show",
	"service.restart",
	"sync.connect",
	"sync.disconnect",
	"sync.setPaused",
	"sync.revealLog",
	"conflict.resolve",
	"folder.resolveRemoval",
	"waiting.markSeen",
}

// implemented lists the actions this host runs now: request.answer is answered
// by the Mac app's window, so only a host the Mac app started offers it.
func (a *app) implemented() []string {
	out := make([]string, 0, len(implementedActions))
	for _, typ := range implementedActions {
		if typ == "request.answer" && !a.shell {
			continue
		}
		if typ == "document.recover" && !a.shell {
			continue
		}
		out = append(out, typ)
	}
	return out
}

// revealPath shows a folder in the file manager through the runtime seam, so a
// test reads what would be revealed instead of opening a Finder window.
func (a *app) revealPath(path string) error {
	if a.rt.reveal != nil {
		return a.rt.reveal(path)
	}
	return browser.Reveal(path)
}

// runEffect runs one effect with the code the app already has for that change.
// An effect whose code is not here yet answers not-yet.
func (a *app) runEffect(ctx context.Context, e appcore.Effect) *appcore.Refusal {
	switch e.Type {
	case "folder.trust":
		return a.trustFromAction(e)
	case "folder.relink":
		return a.relinkFolder(e)
	case "folder.merge":
		return a.mergeFolder(e)
	case "folder.stopTrusting":
		path := a.folderPath(e.FolderID)
		if path == "" {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
		}
		if err := a.untrustFolder(path); err != nil {
			a.rt.logger.Printf("Could not stop trusting folder %s: %v", path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.serve":
		_, refusal, err := a.serveFolderNamed(e.Path, e.Name)
		if refusal != nil {
			return refusal
		}
		if err != nil {
			a.rt.logger.Printf("Could not serve folder %s: %v", e.Path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.setUpTeam":
		return a.setUpSyncFolder(e)
	case "folder.setSynced":
		return a.setFolderSynced(e.FolderID, e.On)
	case "folder.setFraming":
		return a.setFolderAllowFraming(e)
	case "folder.stopServing":
		if err := a.stopServing(e.FolderID); err != nil {
			var unsaved errUnsavedEdits
			if errors.As(err, &unsaved) {
				return &appcore.Refusal{Code: appcore.RefuseUnsavedEdits, SubjectID: e.FolderID}
			}
			a.rt.logger.Printf("Could not stop serving folder %s: %v", e.FolderID, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "service.restart":
		a.service.restart()
		return nil
	case "sync.connect":
		return a.connectSync(ctx, e)
	case "move.start":
		return a.startMove(e)
	case "move.quitLocal":
		return a.quitLocal()
	case "sync.disconnect":
		return a.disconnectSync()
	case "sync.setPaused":
		return a.setSyncPaused(e.On)
	case "sync.revealLog":
		// The service's sync engine writes its logs here (service/sync.cjs logsDir,
		// the engine's logger adds sync/); made first so Finder has a folder to show.
		dir := a.syncLogDir()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			a.rt.logger.Printf("Could not make the sync log folder: %v", err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		if err := a.revealPath(dir); err != nil {
			a.rt.logger.Printf("Could not show the sync log folder: %v", err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "conflict.resolve":
		return a.resolveConflict(e)
	case "folder.resolveRemoval":
		return a.resolveRemoval(e)
	case "host.quit":
		if !e.Force {
			names, answered := a.unsavedDocuments()
			if !answered || len(names) > 0 {
				return &appcore.Refusal{Code: appcore.RefuseQuitUnsaved, Documents: names}
			}
		}
		go func() {
			a.shutdown()
			os.Exit(0)
		}()
		return nil
	case "folder.reveal":
		path := a.folderPath(e.FolderID)
		if path == "" {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
		}
		if err := a.revealPath(path); err != nil {
			a.rt.logger.Printf("Could not show folder %s: %v", path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.copyAddress":
		if err := a.copyServedAddressText(e.FolderID); err != nil {
			a.rt.logger.Printf("Could not copy the address of folder %s: %v", e.FolderID, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "prefs.set":
		return a.setPreference(e)
	case "waiting.markSeen":
		a.core.mu.Lock()
		if a.core.seen == nil {
			a.core.seen = map[string]bool{}
		}
		for _, id := range e.IDs {
			a.core.seen[id] = true
		}
		a.core.mu.Unlock()
		return nil
	case "app.openKeptPage":
		a.openKeptPage()
		return nil
	case "dashboard.open":
		if a.dashboard == nil {
			return &appcore.Refusal{Code: appcore.RefuseNotYet}
		}
		entry, err := a.dashboard.EntryURL(e.Route)
		if err != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments}
		}
		a.launchBrowser(entry)
		return nil
	case "document.open":
		for _, doc := range a.facts().Documents {
			if doc.ID != e.DocumentID {
				continue
			}
			if !doc.OpenNow {
				// A row that is not open resolves its path from the record and
				// nowhere else, then runs what file.open runs, with its refusals.
				return a.openPath(doc.Path)
			}
			if doc.URL == "" {
				return &appcore.Refusal{Code: appcore.RefuseNotYet, SubjectID: doc.ID}
			}
			a.noteOpen(doc.Path, recents.HowFile, true)
			a.launchBrowser(doc.URL)
			return nil
		}
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	case "document.recover":
		return a.recoverDocument(e)
	case "document.forget":
		return a.forgetRecent(e.DocumentID)
	case "document.export":
		for _, doc := range a.facts().Documents {
			if doc.ID != e.DocumentID {
				continue
			}
			if err := a.exportForPage(doc.Path); err != nil {
				a.rt.logger.Printf("Could not export %s: %v", doc.Path, err)
				return &appcore.Refusal{Code: appcore.RefuseFailed}
			}
			return nil
		}
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	case "document.versions":
		for _, doc := range a.facts().Documents {
			if doc.ID != e.DocumentID {
				continue
			}
			route, refusal := a.versionsRoute(doc)
			if refusal != nil {
				return refusal
			}
			if a.dashboard == nil {
				return &appcore.Refusal{Code: appcore.RefuseNotYet}
			}
			entry, err := a.dashboard.EntryURL(route)
			if err != nil {
				return &appcore.Refusal{Code: appcore.RefuseBadArguments}
			}
			a.launchBrowser(entry)
			return nil
		}
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	case "document.reveal":
		for _, doc := range a.facts().Documents {
			if doc.ID != e.DocumentID {
				continue
			}
			if err := a.revealPath(doc.Path); err != nil {
				a.rt.logger.Printf("Could not show document %s: %v", doc.Path, err)
				return &appcore.Refusal{Code: appcore.RefuseFailed}
			}
			return nil
		}
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	case "file.open":
		return a.openPath(e.Path)
	case "request.answer":
		return a.answerRequest(e)
	case "request.show":
		return a.showRequest(e)
	case "program.add", "program.relink":
		return a.registerProgram(e)
	case "program.setAllTrusted":
		return a.setProgramAllTrusted(e)
	case "program.askAgain":
		return a.askProgramAgain(e)
	case "program.removeAccess":
		return a.removeProgramAccess(e)
	case "program.remove":
		return a.removeProgram(e)
	case "program.reveal":
		return a.revealProgram(e)
	}
	return &appcore.Refusal{Code: appcore.RefuseNotYet}
}

// recoverDocument runs document.recover: the newest kept version of a loose
// document whose file is gone is written to a new file, never over an existing
// one, and the document's history and record follow it there.
func (a *app) recoverDocument(e appcore.Effect) *appcore.Refusal {
	store := a.recentsStore()
	if store == nil {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	}
	entry, ok := store.Find(e.DocumentID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	}
	if info, err := os.Stat(entry.Path); err == nil {
		if info.Mode().IsRegular() {
			return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: e.DocumentID}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		a.rt.logger.Printf("Could not check the recovered document %s: %v", entry.Path, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if _, _, served := servedFolderAbove(entry.Path, a.folderFacts()); served {
		return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: e.DocumentID}
	}

	destination := e.Path
	if !filepath.IsAbs(destination) {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "destination"}
	}
	destination = filepath.Clean(destination)
	switch strings.ToLower(filepath.Ext(destination)) {
	case ".html", ".htmlclay":
	default:
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "destination"}
	}
	dir, err := resolveSymlinks(filepath.Dir(destination))
	if err != nil {
		a.rt.logger.Printf("Could not resolve the folder for %s: %v", destination, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	destination, ok = session.ContainWithinHome(a.rt.home, filepath.Join(dir, filepath.Base(destination)))
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseFileOutsideHome, Field: "destination"}
	}

	key, _, data, err := a.rt.versions.Newest(entry.Path)
	if err != nil {
		if errors.Is(err, versions.ErrNoHistory) || errors.Is(err, fs.ErrNotExist) {
			return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: e.DocumentID}
		}
		a.rt.logger.Printf("Could not read the newest version of %s: %v", entry.Path, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return &appcore.Refusal{Code: appcore.RefuseDestinationExists, Field: "destination"}
		}
		a.rt.logger.Printf("Could not create the recovered file %s: %v", destination, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		os.Remove(destination)
		a.rt.logger.Printf("Could not write the recovered file %s: %v", destination, writeErr)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}

	if err := a.rt.versions.RebindTo(key, data, destination); err != nil {
		os.Remove(destination)
		a.rt.logger.Printf("Could not move the history of %s to %s: %v", entry.Path, destination, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if err := store.Rebind(entry.Path, destination); err != nil {
		if moved := versions.Key(destination, data); moved != key {
			if backErr := a.rt.versions.RebindTo(moved, data, entry.Path); backErr != nil {
				a.rt.logger.Printf("Could not move the history of %s back: %v", destination, backErr)
			}
		}
		os.Remove(destination)
		a.rt.logger.Printf("Could not record the recovered document at %s: %v", destination, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}

	return a.openPath(destination)
}

// setFolderAllowFraming runs folder.setFraming: the choice is saved first, then the
// serving host follows it, so a save that failed changes neither.
func (a *app) setFolderAllowFraming(e appcore.Effect) *appcore.Refusal {
	previous, ok := a.rt.cfg.ServedFolderByID(e.FolderID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
	}
	if !a.rt.cfg.SetServedAllowFraming(e.FolderID, e.On) {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
	}
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetServedAllowFraming(e.FolderID, previous.AllowFraming)
		a.rt.logger.Printf("Could not change whether folder %s may be framed: %v", e.FolderID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if a.served != nil {
		a.served.SetAllowFraming(e.FolderID, e.On)
	}
	return nil
}

// programFileWarnings is what a person should know before a program file is
// registered: the file risk details of 7.7 that apply to it, and not-executable.
// The shell's check and the effect both read it, so the sheet and the click judge
// the same rules. A file that is gone refuses file-missing.
func (a *app) programFileWarnings(path string) ([]string, *appcore.Refusal) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, &appcore.Refusal{Code: appcore.RefuseFileMissing}
	}
	var codes []string
	if detail := a.helperFileRiskDetail("", path, helperSuggestNow()); detail != "" {
		codes = append(codes, detail)
	}
	needs, err := helperProgramNeedsExecutable(path)
	if err != nil {
		a.rt.logger.Printf("Could not check whether %s can run: %v", path, err)
		return nil, &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if needs {
		codes = append(codes, helperDetailNotExecutable)
	}
	return codes, nil
}

// registerProgram runs program.add and program.relink. The file is checked again
// at this moment: a code the sheet did not show refuses needs-confirmation, so a
// file that changed between the sheet and the click is never registered. A
// not-executable file the person acknowledged is made executable here, which is
// the only thing that permission allows.
func (a *app) registerProgram(e appcore.Effect) *appcore.Refusal {
	codes, refusal := a.programFileWarnings(e.Path)
	if refusal != nil {
		return refusal
	}
	for _, code := range codes {
		if !slices.Contains(e.Acknowledged, code) {
			return &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation}
		}
	}
	if slices.Contains(codes, helperDetailNotExecutable) {
		if ok, err := makeHelperProgramExecutable(e.Path); err != nil || !ok {
			a.rt.logger.Printf("Could not make the program %s executable: %v", e.Path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
	}
	a.helperStateMu.Lock()
	refusal = a.registerProgramLocked(e)
	a.helperStateMu.Unlock()
	if refusal != nil {
		return refusal
	}
	a.refreshHelperDispatchers()
	return nil
}

// registerProgramLocked stores a registration or a relink. Caller holds
// helperStateMu, so the relink and its decisions change together.
func (a *app) registerProgramLocked(e appcore.Effect) *appcore.Refusal {
	if e.Type == "program.relink" {
		previous, ok := a.rt.cfg.SetHelperProgramPath(e.ProgramID, e.Path)
		if !ok {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: e.ProgramID}
		}
		if err := a.rt.cfg.Save(); err != nil {
			a.rt.cfg.SetHelperProgramPath(e.ProgramID, previous)
			a.rt.logger.Printf("Could not relink the program %s: %v", e.ProgramID, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
	} else {
		program, err := a.rt.cfg.AddHelperProgram(e.Name, e.Path)
		if err != nil {
			a.rt.logger.Printf("Could not register the program %s: %v", e.Name, err)
			if errors.Is(err, config.ErrHelperNameInvalid) {
				return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "name"}
			}
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		if err := a.rt.cfg.Save(); err != nil {
			a.rt.cfg.RemoveHelperProgram(program.ID)
			a.rt.logger.Printf("Could not save the program %s: %v", e.Name, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		a.rt.logger.Printf("Helper program registered: id=%s name=%s path=%s", program.ID, program.Name, program.Path)
	}
	return nil
}

// setProgramAllTrusted runs program.setAllTrusted. The flag reaches documents in
// trusted local folders only, which ResolveHelper decides, so it never widens a
// team folder.
func (a *app) setProgramAllTrusted(e appcore.Effect) *appcore.Refusal {
	a.helperStateMu.Lock()
	refusal := a.setProgramAllTrustedLocked(e)
	a.helperStateMu.Unlock()
	if refusal != nil {
		return refusal
	}
	a.refreshHelperDispatchers()
	return nil
}

// setProgramAllTrustedLocked stores the broad grant. Caller holds helperStateMu.
func (a *app) setProgramAllTrustedLocked(e appcore.Effect) *appcore.Refusal {
	previous, ok := a.rt.cfg.SetHelperAnyDocument(e.ProgramID, e.On)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: e.ProgramID}
	}
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetHelperAnyDocument(e.ProgramID, previous)
		a.rt.logger.Printf("Could not change what program %s may run: %v", e.ProgramID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// askProgramAgain runs program.askAgain: the setup question of 7.7 for one
// document, so a person can allow a program they removed access to. The action
// grants nothing by itself; the answer does.
func (a *app) askProgramAgain(e appcore.Effect) *appcore.Refusal {
	program, ok := a.rt.cfg.LookupHelperProgram(e.ProgramID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: e.ProgramID}
	}
	names, err := readDocumentHelperNames(e.DocumentID)
	if err != nil {
		a.rt.logger.Printf("Could not read the programs %s declares: %v", e.DocumentID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.DocumentID}
	}
	if !slices.Contains(names, program.Name) {
		return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: e.DocumentID}
	}
	a.helperMu.Lock()
	defer a.helperMu.Unlock()
	plan := helperOpenPlan{names: names}
	plan.allowed, plan.denied = a.resolvedHelpers(e.DocumentID, names)
	a.askHelperSetup(e.DocumentID, names, plan, program.Name, a.systemHelperApprovalDialogs(), true)
	a.refreshHelperDispatchers()
	return nil
}

// removeProgramAccess runs program.removeAccess: one document, or every document
// that has the program when the action names none. Each denial is stored under
// the document's own scope, so a removal in a team folder refuses at once instead
// of asking again.
func (a *app) removeProgramAccess(e appcore.Effect) *appcore.Refusal {
	if e.ProgramID == aiEditProgramID {
		return a.removeAIEditAccess(e)
	}
	a.helperStateMu.Lock()
	refusal := a.removeProgramAccessLocked(e)
	a.helperStateMu.Unlock()
	if refusal != nil {
		return refusal
	}
	a.refreshHelperDispatchers()
	return nil
}

// removeProgramAccessLocked stores the denials. Caller holds helperStateMu.
func (a *app) removeProgramAccessLocked(e appcore.Effect) *appcore.Refusal {
	program, ok := a.rt.cfg.LookupHelperProgram(e.ProgramID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: e.ProgramID}
	}
	documents := []string{e.DocumentID}
	if e.DocumentID == "" {
		documents = documents[:0]
		for _, d := range a.rt.cfg.HelperDecisionList() {
			if d.Allowed && d.Program == e.ProgramID && !slices.Contains(documents, d.Document) {
				documents = append(documents, d.Document)
			}
		}
	}
	undo := make([]helperDecisionUndo, 0, len(documents))
	var err error
	for _, document := range documents {
		for _, name := range a.programNamesForDocument(document, e.ProgramID, program.Name) {
			previous, had, rerr := a.rt.cfg.RevokeHelperInScope(document, name, a.scopeOf(document))
			if rerr != nil {
				err = rerr
				break
			}
			undo = append(undo, helperDecisionUndo{document: document, name: name, previous: previous, had: had})
		}
		if err != nil {
			break
		}
	}
	// Removing every document's access also takes the program's broad grant away,
	// or a trusted folder's documents would go on running it.
	widened, hadWide := false, false
	if err == nil && e.DocumentID == "" {
		widened, hadWide = a.rt.cfg.SetHelperAnyDocument(e.ProgramID, false)
	}
	if err == nil {
		err = a.rt.cfg.Save()
	}
	if err != nil {
		for i := len(undo) - 1; i >= 0; i-- {
			a.rt.cfg.ForgetHelperDecision(undo[i].document, undo[i].name)
			if undo[i].had {
				a.rt.cfg.RestoreHelperDecisions([]config.HelperDecision{undo[i].previous})
			}
		}
		if hadWide {
			a.rt.cfg.SetHelperAnyDocument(e.ProgramID, widened)
		}
		a.rt.logger.Printf("Could not remove access to %s: %v", program.Name, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// programNamesForDocument is every name document was allowed the program under,
// or the program's own name when it had only the broad grant to fall back on.
func (a *app) programNamesForDocument(document, programID, fallback string) []string {
	var names []string
	for _, d := range a.rt.cfg.HelperDecisionList() {
		if d.Document == document && d.Allowed && d.Program == programID && !slices.Contains(names, d.Name) {
			names = append(names, d.Name)
		}
	}
	if len(names) == 0 {
		return []string{fallback}
	}
	return names
}

// removeProgram runs program.remove: the registration goes, and every document
// that had access keeps a refusal, so none of them falls through to another
// program with the same name without asking.
func (a *app) removeProgram(e appcore.Effect) *appcore.Refusal {
	a.helperStateMu.Lock()
	refusal := a.removeProgramLocked(e)
	a.helperStateMu.Unlock()
	if refusal != nil {
		return refusal
	}
	a.refreshHelperDispatchers()
	return nil
}

// removeProgramLocked unregisters the program. Caller holds helperStateMu.
func (a *app) removeProgramLocked(e appcore.Effect) *appcore.Refusal {
	program, ok := a.rt.cfg.LookupHelperProgram(e.ProgramID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: e.ProgramID}
	}
	removed := a.rt.cfg.ForgetHelperProgramDecisions(program.ID)
	a.rt.cfg.RemoveHelperProgram(program.ID)
	undo := make([]helperDecisionUndo, 0, len(removed))
	var err error
	for _, d := range removed {
		previous, had, rerr := a.rt.cfg.RevokeHelperInScope(d.Document, d.Name, a.scopeOf(d.Document))
		if rerr != nil {
			err = rerr
			break
		}
		undo = append(undo, helperDecisionUndo{document: d.Document, name: d.Name, previous: previous, had: had})
	}
	if err == nil {
		err = a.rt.cfg.Save()
	}
	if err != nil {
		for i := len(undo) - 1; i >= 0; i-- {
			a.rt.cfg.ForgetHelperDecision(undo[i].document, undo[i].name)
			if undo[i].had {
				a.rt.cfg.RestoreHelperDecisions([]config.HelperDecision{undo[i].previous})
			}
		}
		a.rt.cfg.RestoreHelperProgram(program)
		a.rt.cfg.RestoreHelperDecisions(removed)
		a.rt.logger.Printf("Could not remove the program %s: %v", program.ID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.rt.logger.Printf("Helper program removed: id=%s name=%s path=%s", program.ID, program.Name, program.Path)
	return nil
}

// revealProgram runs program.reveal: the program's file in the file manager.
func (a *app) revealProgram(e appcore.Effect) *appcore.Refusal {
	program, ok := a.rt.cfg.LookupHelperProgram(e.ProgramID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: e.ProgramID}
	}
	if err := a.revealPath(program.Path); err != nil {
		a.rt.logger.Printf("Could not show the program %s: %v", program.Path, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// folderPath answers the path the facts give a folder id, or "".
func (a *app) folderPath(id string) string {
	for _, ff := range a.facts().Folders {
		if ff.ID == id {
			return ff.Path
		}
	}
	return ""
}

// setPreference applies a prefs.set effect.
func (a *app) setPreference(e appcore.Effect) *appcore.Refusal {
	switch e.Key {
	case "aiEditing":
		var on bool
		if json.Unmarshal(e.Value, &on) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		a.rt.cfg.SetAIEditEnabled(on)
		a.saveConfig("preference aiEditing")
		if !on {
			a.cancelAIEdits()
		}
		return nil
	case "appearance":
		var v string
		if json.Unmarshal(e.Value, &v) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		a.rt.cfg.SetAppearance(v)
		a.saveConfig("preference appearance")
		return nil
	case "openAtLogin":
		var on bool
		if json.Unmarshal(e.Value, &on) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		// The Mac app owns the login item (CR-5): a host it started, or any host inside
		// its bundle, records the choice and the app's reconciler applies it.
		if _, mac := bundledMacHost(); a.shell || mac {
			a.rt.cfg.SetStartOnLogin(on)
			a.saveConfig("preference openAtLogin")
			return nil
		}
		if err := a.setLoginItem(on); err != nil {
			a.rt.logger.Printf("Could not set Start on Login: %v", err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "useName", "yourName":
		profile := a.rt.cfg.ProfileState()
		switch e.Key {
		case "useName":
			if json.Unmarshal(e.Value, &profile.Enabled) != nil {
				return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
			}
			// The tray mints the id when sharing is first turned on; the Mac app's
			// switch needs the same, or turning it on is refused for a missing id.
			if profile.Enabled && !config.ValidProfileID(profile.ID) {
				id, err := config.NewProfileID()
				if err != nil {
					return &appcore.Refusal{Code: appcore.RefuseFailed}
				}
				profile.ID = id
			}
		case "yourName":
			if json.Unmarshal(e.Value, &profile.Name) != nil {
				return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
			}
		}
		if err := a.rt.cfg.UpdateProfile(profile); err != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		return nil
	}
	return &appcore.Refusal{Code: appcore.RefuseNotYet}
}

// setLoginItem records the Start on Login preference the way the tray's toggle
// does: the system login item first, then the config, and both back if the save
// fails.
func (a *app) setLoginItem(on bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	if execPath == "" {
		return fmt.Errorf("the executable path is empty")
	}
	if err := platform.SetLoginItem(on, execPath); err != nil {
		return err
	}
	previous := a.rt.cfg.StartOnLoginEnabled()
	a.rt.cfg.SetStartOnLogin(on)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetStartOnLogin(previous)
		if backErr := platform.SetLoginItem(previous, execPath); backErr != nil {
			a.rt.logger.Printf("Could not put the login item back: %v", backErr)
		}
		return err
	}
	return nil
}
