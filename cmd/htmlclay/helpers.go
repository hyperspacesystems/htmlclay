package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/tier"
	"github.com/panphora/htmlclay/internal/wire"
)

const helperDeclarationReadLimit = 512 << 10

// pageAskQuietFor is how long a page must wait after a dialog that granted
// nothing before it may raise the helper dialog again.
const pageAskQuietFor = time.Minute

type helperOpenPlan struct {
	names   []string
	allowed map[string]config.HelperProgram
	denied  []string
}

type helperApprovalDialogs struct {
	setup   func(title, message, refuse, middle, affirm string) (platform.Choice, error)
	choose  func(prompt, dir string) (string, bool, error)
	prepare func(path string) (bool, error)
	confirm func(title, message, affirmative string) (bool, error)
}

// helperCandidate is one undecided name. program is set when exactly one
// program is registered under the name; registered lists every registration,
// so a name with several is shown with all of them and chosen, never guessed.
type helperCandidate struct {
	name       string
	program    config.HelperProgram
	registered []config.HelperProgram
	path       string
}

type helperDecisionUndo struct {
	document string
	name     string
	previous config.HelperDecision
	had      bool
}

func (a *app) systemHelperApprovalDialogs() helperApprovalDialogs {
	setup := a.rt.chooseHelperSetup
	if setup == nil {
		setup = a.dialogs().choose
	}
	choose := a.rt.chooseHelperProgram
	if choose == nil {
		choose = a.dialogs().selectFile
	}
	prepare := a.rt.prepareHelperProgram
	if prepare == nil {
		prepare = func(path string) (bool, error) {
			return prepareHelperProgram(path, a.dialogs().confirmWithButtons)
		}
	}
	return helperApprovalDialogs{
		setup:   setup,
		choose:  choose,
		prepare: prepare,
		confirm: a.dialogs().confirmWithButtons,
	}
}

// helpersForOpen resolves a document's permissions for something the user did:
// opening a file, a tray action, untrusting a folder. A failure is reported on
// screen because someone is waiting on the action that caused it.
func (a *app) helpersForOpen(document string, prompt bool) helperOpenPlan {
	return a.helpersForOpenWith(document, prompt, true, a.systemHelperApprovalDialogs())
}

// helpersForNavigation resolves the same permissions for an HTTP request, which
// nobody asked for directly. It never prompts, so it never takes helperMu and
// cannot hold a page load behind a dialog someone left open elsewhere, and it
// reports a failure to the log alone rather than raising a banner over whatever
// the user is actually doing.
func (a *app) helpersForNavigation(document string) helperOpenPlan {
	return a.helpersForOpenWith(document, false, false, a.systemHelperApprovalDialogs())
}

// askHelperForUse is each site server's AskHelper hook: a page tried to use
// name, a helper it declares and the person has not answered. It asks about
// that one name, reattaches the document's helpers, and reports whether name is
// now allowed.
func (a *app) askHelperForUse(document, name string) bool {
	plan := a.helpersForUseWith(document, name, a.systemHelperApprovalDialogs())
	if core, ok := a.wireCoreFor(document); ok {
		a.applyHelperPlan(core, document, plan)
	}
	_, allowed := plan.allowed[name]
	return allowed
}

// helpersForUseWith is the page's setup request: the person clicked Set Up, or
// the page's own button, for a declared name that is not allowed. A name the
// person revoked can be set up again this way. It keeps this open's denials, so
// a page cannot raise the dialog past a Deny until the next direct open, and a
// dialog that granted nothing quiets the page for a minute.
func (a *app) helpersForUseWith(document, name string, dialogs helperApprovalDialogs) helperOpenPlan {
	a.helperMu.Lock()
	defer a.helperMu.Unlock()
	names, err := readDocumentHelperNames(document)
	if err != nil {
		a.rt.logger.Printf("Could not configure helpers for %s: %v", document, err)
		return helperOpenPlan{}
	}
	plan := helperOpenPlan{names: names}
	plan.allowed, plan.denied = a.resolvedHelpers(document, names)
	if !slices.Contains(names, name) {
		return plan
	}
	if _, ok := plan.allowed[name]; ok {
		return plan
	}
	if a.hasOpenDenials(document) || time.Now().Before(a.pageAskQuietUntil(document)) {
		return plan
	}
	a.rt.logger.Printf("Setting up program %s for %s", name, document)
	return a.askHelperSetup(document, names, plan, name, dialogs, false)
}

func (a *app) helpersForOpenWith(document string, prompt, notify bool, dialogs helperApprovalDialogs) helperOpenPlan {
	if prompt {
		a.helperMu.Lock()
		defer a.helperMu.Unlock()
	}

	names, err := readDocumentHelperNames(document)
	if err != nil {
		if notify {
			a.reportHelperSetupError(document, err)
		} else {
			a.rt.logger.Printf("Could not configure helpers for %s: %v", document, err)
		}
		return helperOpenPlan{}
	}
	if prompt {
		a.setOpenDenials(document, nil)
	}
	plan := helperOpenPlan{names: names}
	plan.allowed, plan.denied = a.resolvedHelpers(document, names)
	return plan
}

func readDocumentHelperNames(document string) ([]string, error) {
	f, err := os.Open(document)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, helperDeclarationReadLimit))
	if err != nil {
		return nil, err
	}
	return htmlutil.ReadHelperNames(data), nil
}

func readDocumentHelperFolders(document string) map[string]string {
	f, err := os.Open(document)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, helperDeclarationReadLimit))
	if err != nil {
		return nil
	}
	return htmlutil.ReadHelperFolders(data)
}

// helperPickerFolder is where the file picker for name starts: the folder the
// document's declaration suggests, relative to the document or, when it starts
// with ~/, to the home folder, else the document's own folder. It only chooses where the picker opens. The person
// still picks the file, and the next dialog shows its full path. A suggestion
// outside home, or one that is not a folder, falls back to the document's.
func (a *app) helperPickerFolder(document, name string) string {
	dir := filepath.Dir(document)
	hint := readDocumentHelperFolders(document)[name]
	if hint == "" {
		return dir
	}
	candidate := filepath.Join(dir, filepath.FromSlash(hint))
	if rest, ok := strings.CutPrefix(hint, "~/"); ok {
		candidate = filepath.Join(a.rt.home, filepath.FromSlash(rest))
	}
	if _, ok := session.ContainWithinHome(a.rt.home, candidate); !ok {
		return dir
	}
	if info, err := os.Stat(candidate); err != nil || !info.IsDir() {
		return dir
	}
	return candidate
}

// scopeOf is what a program decision about document is stored and read as. The
// folder identity is the one the identity pin uses, so a team folder is still
// found after it is renamed, moved, or reached through another spelling.
func (a *app) scopeOf(document string) config.Scope {
	return a.rt.cfg.ScopeOf(document, platform.DirIdentity)
}

// programTrusted is whether document's folder may run programs at all: a folder
// the person trusts, or a folder this app serves. Which program, and whether
// it asks, is still the decision for document in its scope.
func (a *app) programTrusted(document string) bool {
	if a.trustedLive(document) {
		return true
	}
	owner, err := a.ResolveDocument(document)
	return err == nil && owner.Kind == OwnerServed
}

// wireCoreFor answers the wire core that serves document: its served folder's
// root, or the loose site that has it registered.
func (a *app) wireCoreFor(document string) (*wire.Core, bool) {
	if owner, err := a.ResolveDocument(document); err == nil && owner.Kind == OwnerServed {
		if a.served == nil {
			return nil, false
		}
		return a.served.Wire(owner.FolderID)
	}
	a.mu.Lock()
	s, _, ok := a.lookupLocked(document)
	a.mu.Unlock()
	if !ok || s.srv == nil {
		return nil, false
	}
	return s.srv.Core, true
}

// servedWireCores answers the wire core of every folder being served now.
func (a *app) servedWireCores() []*wire.Core {
	if a.served == nil {
		return nil
	}
	a.servedMu.Lock()
	ids := make([]string, 0, len(a.servedByName))
	for _, m := range a.servedByName {
		ids = append(ids, m.folder.ID)
	}
	a.servedMu.Unlock()
	var cores []*wire.Core
	for _, id := range ids {
		if core, ok := a.served.Wire(id); ok {
			cores = append(cores, core)
		}
	}
	return cores
}

// helperGranted is the server's last question before a program's process starts:
// may name run programID for file right now? It reads the tier, the programs
// folder and the scope at this moment rather than anything cached when the
// dispatcher attached, so a folder that became a team folder, a teammate's
// code change, developer mode turned off, or an access the person removed,
// stops the process.
func (a *app) helperGranted(file, name, programID string) bool {
	view := a.helperViewOf(file, []string{name})
	if p, ok := view.folder[name]; ok {
		return p.ID == programID
	}
	if view.waits(name) {
		return false
	}
	resolution, _ := a.rt.cfg.ResolveHelper(file, name, a.programTrusted(file), a.scopeOf(file))
	return resolution.Decided && resolution.Allowed && resolution.Program.ID == programID
}

// helperView is what deciding a document's programs reads from disk: the
// document's tier, the programs folder's programs that answer it now, and the
// names whose folder program waits for the warning or the team question. It is
// read before helperStateMu, because reading a document takes a while.
type helperView struct {
	verdict tier.Verdict
	folder  map[string]config.HelperProgram
	waiting map[string]bool
}

func (a *app) helperViewOf(document string, names []string) helperView {
	view := helperView{verdict: a.tierOf(document), folder: map[string]config.HelperProgram{}, waiting: map[string]bool{}}
	for _, name := range names {
		switch program, ok, waits := a.folderProgram(name, view.verdict); {
		case ok:
			view.folder[name] = program
		case waits:
			view.waiting[name] = true
		}
	}
	return view
}

// waits reports a name that cannot run until the person answers the question
// its document needs: a folder program that waits for it, or a registered
// program of a document nobody vouches for, or of a team document whose code
// changed since the person allowed it.
func (v helperView) waits(name string) bool {
	return v.waiting[name] || v.verdict.Warn()
}

// A name that is in neither result was never decided: nobody was asked, the
// picker was cancelled, the program behind an old allow is gone, or the
// document waits for the warning or the team question.
func (a *app) resolvedHelpers(document string, names []string) (map[string]config.HelperProgram, []string) {
	view := a.helperViewOf(document, names)
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	return a.resolvedHelpersLocked(document, names, view)
}

func (a *app) resolvedHelpersLocked(document string, names []string, view helperView) (map[string]config.HelperProgram, []string) {
	allowed := make(map[string]config.HelperProgram)
	var denied []string
	scope := a.scopeOf(document)
	for _, name := range names {
		if program, ok := view.folder[name]; ok {
			allowed[name] = program
			continue
		}
		if view.verdict.Denied {
			denied = append(denied, name)
			continue
		}
		if view.waits(name) {
			continue
		}
		resolution, _ := a.rt.cfg.ResolveHelper(document, name, a.programTrusted(document), scope)
		switch {
		case !resolution.Decided:
		case resolution.Allowed:
			allowed[name] = resolution.Program
		default:
			denied = append(denied, name)
		}
	}
	for _, name := range a.openDenials[document] {
		if _, ok := allowed[name]; ok {
			delete(allowed, name)
		}
		if !slices.Contains(denied, name) {
			denied = append(denied, name)
		}
	}
	return allowed, denied
}

func (a *app) hasOpenDenials(document string) bool {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	return len(a.openDenials[document]) > 0
}

func (a *app) pageAskQuietUntil(document string) time.Time {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	return a.pageAskQuiet[document]
}

func (a *app) addOpenDenials(document string, names []string) {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	if a.openDenials == nil {
		a.openDenials = make(map[string][]string)
	}
	for _, name := range names {
		if !slices.Contains(a.openDenials[document], name) {
			a.openDenials[document] = append(a.openDenials[document], name)
		}
	}
}

func (a *app) setOpenDenials(document string, names []string) {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	if len(names) == 0 {
		delete(a.openDenials, document)
		return
	}
	if a.openDenials == nil {
		a.openDenials = make(map[string][]string)
	}
	a.openDenials[document] = names
}

func (a *app) saveHelperDecisionSet(document string, candidates []helperCandidate, choice platform.ConfirmChoice) error {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	return a.saveHelperDecisionSetLocked(document, candidates, choice)
}

// saveHelperDecisionSetLocked is saveHelperDecisionSet for a caller that already
// holds helperStateMu, so a registration check and the save share one transaction.
func (a *app) saveHelperDecisionSetLocked(document string, candidates []helperCandidate, choice platform.ConfirmChoice) error {
	scope := a.scopeOf(document)
	// "Always" reaches every document in a trusted folder, which is a promise this
	// machine makes about its own folders. A team folder's documents are the
	// team's, so the widest answer there is still one document. Refused before
	// anything is registered or stored, so a refusal leaves no trace.
	if choice == platform.ConfirmAllowAlways && scope.Kind != config.ScopeLocal {
		return fmt.Errorf("a program cannot be allowed for every document in a %s folder", scope.Kind)
	}
	added := make([]config.HelperProgram, 0, len(candidates))
	flagUndo := make(map[string]bool)
	decisionUndo := make([]helperDecisionUndo, 0, len(candidates))
	rollback := func() {
		for i := len(decisionUndo) - 1; i >= 0; i-- {
			undo := decisionUndo[i]
			a.rt.cfg.ForgetHelperDecision(undo.document, undo.name)
			if undo.had {
				a.rt.cfg.RestoreHelperDecisions([]config.HelperDecision{undo.previous})
			}
		}
		for id, previous := range flagUndo {
			a.rt.cfg.SetHelperAnyDocument(id, previous)
		}
		for i := len(added) - 1; i >= 0; i-- {
			a.rt.cfg.RemoveHelperProgram(added[i].ID)
		}
	}

	for i := range candidates {
		if candidates[i].program.ID != "" {
			continue
		}
		if program, ok := registeredAtPath(candidates[i].registered, candidates[i].path); ok {
			candidates[i].program = program
			continue
		}
		program, err := a.rt.cfg.AddHelperProgram(candidates[i].name, candidates[i].path)
		if err != nil {
			rollback()
			return fmt.Errorf("could not register %s: %w", candidates[i].name, err)
		}
		candidates[i].program = program
		added = append(added, program)
	}

	if choice == platform.ConfirmAllowAlways {
		for _, candidate := range candidates {
			if _, seen := flagUndo[candidate.program.ID]; seen {
				continue
			}
			previous, ok := a.rt.cfg.SetHelperAnyDocument(candidate.program.ID, true)
			if !ok {
				rollback()
				return fmt.Errorf("could not allow %s for any document", candidate.name)
			}
			flagUndo[candidate.program.ID] = previous
		}
	}

	now := time.Now().UnixNano()
	for i, candidate := range candidates {
		decision := config.HelperDecision{
			Document:  document,
			Name:      candidate.name,
			Program:   candidate.program.ID,
			Allowed:   true,
			DecidedAt: now + int64(i),
			Scope:     scope,
		}
		previous, had, err := a.rt.cfg.DecideHelper(decision)
		if err != nil {
			rollback()
			return fmt.Errorf("could not record the decision for %s: %w", candidate.name, err)
		}
		decisionUndo = append(decisionUndo, helperDecisionUndo{document: document, name: candidate.name, previous: previous, had: had})
	}
	if err := a.rt.cfg.Save(); err != nil {
		rollback()
		return fmt.Errorf("could not save helper decisions: %w", err)
	}
	for _, program := range added {
		a.rt.logger.Printf("Helper program registered: id=%s name=%s path=%s", program.ID, program.Name, program.Path)
	}
	reach := "document"
	if choice == platform.ConfirmAllowAlways {
		reach = "any document"
	}
	for i, candidate := range candidates {
		previous := ""
		if decisionUndo[i].had {
			previous = decisionUndo[i].previous.Program
		}
		a.rt.logger.Printf("Helper binding saved: document=%s name=%s program=%s path=%s scope=%s previous=%s", document, candidate.name, candidate.program.ID, candidate.program.Path, reach, previous)
	}
	return nil
}

// registeredAtPath finds the registration a picked file already belongs to, so
// choosing between programs that share a name reuses the chosen one instead of
// registering it a second time.
func registeredAtPath(programs []config.HelperProgram, path string) (config.HelperProgram, bool) {
	for _, program := range programs {
		if program.Path == path {
			return program, true
		}
	}
	picked, err := os.Stat(path)
	if err != nil {
		return config.HelperProgram{}, false
	}
	for _, program := range programs {
		if info, err := os.Stat(program.Path); err == nil && os.SameFile(info, picked) {
			return program, true
		}
	}
	return config.HelperProgram{}, false
}

func (a *app) applyHelperPlan(core *wire.Core, document string, plan helperOpenPlan) {
	var view helperView
	if len(plan.names) > 0 {
		view = a.helperViewOf(document, plan.names)
	}
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	if len(plan.names) == 0 {
		core.DetachHelpers(document)
		return
	}
	allowed, denied := a.resolvedHelpersLocked(document, plan.names, view)
	if err := core.AttachHelpers(document, allowed, denied); err != nil {
		a.rt.logger.Printf("Could not attach helpers for %s: %v", document, err)
	}
}

func (a *app) refreshHelperDispatchers() {
	type openDocument struct {
		site *site
		path string
	}
	a.mu.Lock()
	var documents []openDocument
	for _, s := range a.sites {
		for _, registration := range s.sessions.Registrations() {
			documents = append(documents, openDocument{site: s, path: registration.Path})
		}
	}
	a.mu.Unlock()
	for _, document := range documents {
		a.applyHelperPlan(document.site.srv.Core, document.path, a.helpersForNavigation(document.path))
	}
	for _, core := range a.servedWireCores() {
		for _, path := range core.BoundFiles() {
			a.applyHelperPlan(core, path, a.helpersForNavigation(path))
		}
	}
}

// cancelAIEdits stops every running AI edit and ai-tools request on every
// site, after the AI Editing switch is turned off.
func (a *app) cancelAIEdits() {
	a.mu.Lock()
	sites := append([]*site(nil), a.sites...)
	a.mu.Unlock()
	for _, s := range sites {
		if s.srv != nil {
			s.srv.CancelAI()
		}
	}
	for _, core := range a.servedWireCores() {
		core.CancelAI()
	}
	a.closeAITools()
}

func (a *app) reportHelperSetupError(document string, err error) {
	a.rt.logger.Printf("Could not configure helpers for %s: %v", document, err)
	message := fmt.Sprintf("%s could not configure its programs: %v", filepath.Base(document), err)
	if notifyErr := a.notifyUser("HTML Clay could not configure programs", message); notifyErr != nil {
		a.rt.logger.Printf("Could not show helper notification: %v", notifyErr)
	}
}
