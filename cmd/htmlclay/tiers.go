package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/codeset"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/tier"
)

// strangerReadProgram names the read a page nobody vouches for asked for, so
// the warning says what the page wanted. servedOpenProgram is the same for a
// document opened from a served folder.
const (
	strangerReadProgram = "read"
	servedOpenProgram   = "open"
)

const (
	documentAnswerDeny    = "deny-document"
	documentAnswerTrust   = "trust-document"
	documentDialogTitle   = "Make sure you trust %s before allowing it"
	documentDialogDetail  = "Once allowed, this page can read and change the files in %s, and send anything it sees anywhere on the internet. Allowing this document holds until the page's code changes."
	teamCodeDialogTitle   = "Let %s run its new code?"
	teamCodeDialogMessage = "This document is in %s, a team folder, and its code changed since you last allowed it. Allowing holds until its code changes again."
)

// openTiers opens the trust store in the config folder. The first launch that
// has no upgrade record runs the upgrade: it records the trusted folders of
// that moment, and every document opened before that has no download mark
// counts as the person's own. Any command that opens the store first, such as
// htmlclay trust from a terminal, cannot skip it that way.
func (a *app) openTiers(opened *recents.Store) *tier.Store {
	store, _, err := tier.Open(filepath.Join(a.rt.configDir, "trust"))
	if err != nil {
		a.rt.logger.Printf("Could not open the record of trusted documents: %v", err)
		return nil
	}
	if _, done := store.Upgrade(); done {
		return store
	}
	var folders []string
	for _, f := range a.rt.cfg.TrustedFolderList() {
		folders = append(folders, closedPath(f.Path))
	}
	if err := store.MarkUpgrade(folders); err != nil {
		a.rt.logger.Printf("Could not record the upgrade of trusted documents: %v", err)
	}
	a.keepTeamAllows(store)
	if opened == nil {
		return store
	}
	for _, e := range opened.List() {
		if _, err := os.Stat(e.Path); err != nil || platform.Downloaded(e.Path) {
			continue
		}
		if first, err := store.NoteSeen(closedPath(e.Path), false); err == nil && first {
			_ = store.TrustPath(closedPath(e.Path), false, tier.HowUpgrade)
		}
	}
	return store
}

// tierStore is the record of where documents came from and whose code the
// person trusts. Nil when this launch has no config folder.
func (a *app) tierStore() *tier.Store {
	a.initCore()
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	return a.core.tiers
}

// tierOf decides who vouches for document's code. The first time HTML Clay
// sees a document it records whether the file carried a download mark, before
// a save of its own could change the file.
func (a *app) tierOf(document string) tier.Verdict {
	v, _ := a.readTier(document)
	return v
}

// readTier reads document once and returns its verdict with the bytes the
// verdict is for, so a caller that acts on the file's content acts on exactly
// the code that was checked. data is nil when the file cannot be read.
func (a *app) readTier(document string) (v tier.Verdict, data []byte) {
	path := closedPath(document)
	store := a.tierStore()
	if store == nil {
		return tier.Verdict{Tier: tier.Unknown, Reason: tier.ReasonUnrecorded}, nil
	}
	in := tier.Input{
		Path:       path,
		Team:       a.scopeOf(path).Kind != config.ScopeLocal,
		Downloaded: platform.Downloaded(path),
		Keys:       codeset.Ecosystem,
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > codeset.MaxDocument {
		in.Unreadable = true
	} else if data, err = os.ReadFile(path); err != nil || len(data) > codeset.MaxDocument {
		in.Unreadable, data = true, nil
	} else if set, ok := a.codeSetOfData(path, data); ok {
		in.Set = &set
	} else {
		in.Unreadable, data = true, nil
	}
	seen, known := store.Seen(path)
	if !known && !in.Unreadable {
		_, sigErr := codeset.Verify(*in.Set, in.Keys)
		sight := tier.Sight{Path: path, Code: in.Set.Hash, Downloaded: in.Downloaded, BrokenSignature: tier.BrokenSignature(sigErr), Developer: store.Developer(), Changed: platform.WrittenAt(info)}
		if err := store.Note(sight); err != nil {
			a.rt.logger.Printf("Could not record the first sight of %s: %v", path, err)
		}
		if seen, known = store.Seen(path); !known {
			seen = tier.SeenRecord{Path: path, Downloaded: in.Downloaded}
		}
	}
	// Developer mode acts through the first sight only: turning it on later
	// does not adopt documents HTML Clay already saw.
	in.Developer = seen.Developer
	return store.Of(in), data
}

type cachedCodeSet struct {
	sum [sha256.Size]byte
	set codeset.Set
}

// codeSetOf returns the document's code set. Every program call and AI request
// asks for the tier, and parsing costs about 25 ms per megabyte, so a document
// is parsed again only when its bytes changed since the last read. The bytes,
// not the file's times, decide, because trust follows the code and any program
// can set a file's times back.
func (a *app) codeSetOf(path string) (codeset.Set, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return codeset.Set{}, false
	}
	return a.codeSetOfData(path, data)
}

func (a *app) codeSetOfData(path string, data []byte) (codeset.Set, bool) {
	sum := sha256.Sum256(data)
	a.core.mu.Lock()
	cached, ok := a.core.codeSets[path]
	a.core.mu.Unlock()
	if ok && cached.sum == sum {
		return cached.set, true
	}
	set, err := codeset.Of(data)
	if err != nil {
		return codeset.Set{}, false
	}
	a.core.mu.Lock()
	if a.core.codeSets == nil {
		a.core.codeSets = make(map[string]cachedCodeSet)
	}
	a.core.codeSets[path] = cachedCodeSet{sum: sum, set: set}
	a.core.mu.Unlock()
	return set, true
}

// aiEditFiles is both hosts' answer to whether document's AI requests may
// attach its sibling files: only a document the person or a signed release
// vouches for.
func (a *app) aiEditFiles(document string) bool {
	return a.tierOf(document).Trusted()
}

// trustDocument records document as the person's own: its path, and its code
// wherever that code turns up, so another copy of the same code opens without
// asking. A document trusted while it carries a download mark stays trusted
// with it. A document with no code at all trusts only its path: every page
// without a script has that same empty code set, so trusting it everywhere
// would vouch for every plain page anyone downloads.
func (a *app) trustDocument(document string, how string) error {
	path := closedPath(document)
	store := a.tierStore()
	if store == nil {
		return fmt.Errorf("HTML Clay has no settings folder to record this in")
	}
	set, readable := a.codeSetOf(path)
	seen, _ := store.Seen(path)
	if err := store.TrustPath(path, platform.Downloaded(path) || seen.Downloaded, how); err != nil {
		return err
	}
	if !readable || set.Parts == 0 {
		return nil
	}
	return store.TrustCodeViaPath(set.Hash, path, how)
}

// recordMadeDocument records a document HTML Clay itself wrote as the person's
// own at its path. Its code is not trusted everywhere: a starter template's
// code is not only the person's, and every page without a script shares the
// same empty code set.
func (a *app) recordMadeDocument(document, how string) error {
	path := closedPath(document)
	store := a.tierStore()
	if store == nil {
		return fmt.Errorf("HTML Clay has no settings folder to record this in")
	}
	marked := platform.Downloaded(path)
	if seen, ok := store.Seen(path); ok && seen.Downloaded {
		marked = true
	}
	if _, err := store.NoteSeen(path, marked); err != nil {
		return err
	}
	return store.TrustPath(path, marked, how)
}

// closedPath is document's path with its links resolved, or its cleaned
// absolute path while the file is gone.
func closedPath(document string) string {
	if real, err := resolveSymlinks(document); err == nil {
		return real
	}
	if abs, err := filepath.Abs(document); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(document)
}

// askDocument raises the one question a document's code needs before program
// reaches its folder, and returns at once because it runs on the page's own
// request: the warning for a document nobody vouches for, and the team
// question for a team document whose code the person has not allowed. One
// question per document is open at a time. After "Don't allow" on the warning
// the document never asks again until htmlclay untrust; after any other
// question that allowed nothing it stays quiet for a minute.
func (a *app) askDocument(document, program string) {
	path, err := resolveSymlinks(document)
	if err != nil {
		return
	}
	v := a.tierOf(path)
	if !v.Warn() || v.Denied {
		return
	}
	a.helperStateMu.Lock()
	if a.documentAsking[path] || time.Now().Before(a.documentQuiet[path]) {
		a.helperStateMu.Unlock()
		return
	}
	if a.documentAsking == nil {
		a.documentAsking = make(map[string]bool)
	}
	a.documentAsking[path] = true
	a.helperStateMu.Unlock()
	go a.runDocumentAsk(path, program, v)
}

func (a *app) runDocumentAsk(path, program string, v tier.Verdict) {
	kept := false
	defer func() {
		a.helperStateMu.Lock()
		delete(a.documentAsking, path)
		if kept {
			delete(a.documentQuiet, path)
		} else {
			if a.documentQuiet == nil {
				a.documentQuiet = make(map[string]time.Time)
			}
			a.documentQuiet[path] = time.Now().Add(pageAskQuietFor)
		}
		a.helperStateMu.Unlock()
	}()
	presenter := a.presenter(documentDialog{app: a})
	reply, err := presenter.Ask(context.Background(), a.documentAsk(path, program, v))
	if err != nil {
		a.rt.logger.Printf("Could not ask about %s: %v", path, err)
		return
	}
	defer presenter.Close(reply.ID)
	switch reply.Choice {
	case helperAnswerAllowDocument:
		err = a.allowDocument(path, v.Code)
	case documentAnswerTrust:
		err = a.trustDocument(path, tier.HowAnswer)
	case documentAnswerDeny:
		if err = a.denyDocument(path); err == nil {
			a.settleRefusedDocument(path)
		}
	default:
		if reply.Choice == helperAnswerDeny {
			a.settleRefusedDocument(path)
		}
		a.rt.logger.Printf("%s was not allowed; its page can ask again in a minute", path)
		return
	}
	if err != nil {
		a.rt.logger.Printf("Could not record the answer for %s: %v", path, err)
		return
	}
	kept = true
	a.rt.logger.Printf("Document %s: %s", reply.Choice, path)
	a.refreshHelperDispatchers()
	a.changed()
}

// strangerRead is the server's answer to a page nobody vouches for asking to
// read outside what it may: the read is refused and the warning is raised, as
// for AI editing or a program. A person who wants the page to reach its folder
// answers the warning, which allows the page for the code it is running.
func (a *app) strangerRead(document string) bool {
	if !a.tierOf(document).Warn() {
		return false
	}
	a.askDocument(document, strangerReadProgram)
	return true
}

// teamCodeDetail marks the team question about a code change, so the window
// can say the code changed rather than showing the ordinary team sentence.
const teamCodeDetail = "team"

// teamCodeChanged reports a team document whose code the person allowed before
// and has changed since.
func (a *app) teamCodeChanged(path string, v tier.Verdict) bool {
	if v.Tier != tier.Team || v.Allowed || v.Denied || v.Code == "" {
		return false
	}
	store := a.tierStore()
	if store == nil {
		return false
	}
	answer, ok := store.Answer(path)
	return ok && !answer.Denied && answer.Code != "" && answer.Code != v.Code
}

// keepTeamAllows runs once, at the upgrade: each team document the settings
// already allow for AI editing, a built-in program or a helper program keeps
// that allow for the code it has now, so the upgrade asks nothing new. An
// allow given after the upgrade binds to the code only through the team
// question. It runs under core.mu, so it reads files and settings directly.
func (a *app) keepTeamAllows(store *tier.Store) {
	seen := map[string]bool{}
	var documents []string
	add := func(document string) {
		if document != "" && !seen[document] {
			seen[document] = true
			documents = append(documents, document)
		}
	}
	for _, d := range a.rt.cfg.AIEditDocumentList() {
		add(d.Document)
	}
	for _, id := range builtin.IDs {
		if a.rt.cfg.BuiltinOn(id) {
			for _, d := range a.rt.cfg.BuiltinDocuments(id) {
				add(d)
			}
		}
	}
	for _, d := range a.rt.cfg.HelperDecisionList() {
		if d.Allowed {
			add(d.Document)
		}
	}
	for _, document := range documents {
		path := closedPath(document)
		if a.scopeOf(path).Kind == config.ScopeLocal {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > codeset.MaxDocument {
			continue
		}
		set, err := codeset.Of(data)
		if err != nil || set.Hash == "" {
			continue
		}
		if err := store.Allow(path, set.Hash); err != nil {
			a.rt.logger.Printf("Could not keep the earlier allow for %s: %v", path, err)
		}
	}
}

// settleRefusedDocument stops what a document started and dismisses every
// question still waiting for it, so a refusal leaves nothing hanging: no
// helper call parked until it times out, no AI edit running, no built-in
// program waiting, and no banner on the page.
func (a *app) settleRefusedDocument(path string) {
	a.registry().CloseDocument(path)
	if core, ok := a.wireCoreFor(path); ok {
		core.CancelAIFor(path)
		core.CancelBuiltinFor("", path)
	}
	for _, core := range a.servedWireCores() {
		core.CancelAIFor(path)
		core.CancelBuiltinFor("", path)
	}
	a.cancelBuiltins("", path)
	a.refreshHelperDispatchers()
}

// allowDocument records "Allow for this document" for the code the person was
// shown, so the document asks again once its code changes.
func (a *app) allowDocument(path, code string) error {
	store := a.tierStore()
	if store == nil {
		return fmt.Errorf("HTML Clay has no settings folder to record this in")
	}
	return store.Allow(path, code)
}

func (a *app) denyDocument(path string) error {
	store := a.tierStore()
	if store == nil {
		return fmt.Errorf("HTML Clay has no settings folder to record this in")
	}
	if err := store.Deny(path); err != nil {
		return err
	}
	if s := a.aiToolsService(); s != nil {
		_ = s.Store().ForgetFolder(closedPath(path))
	}
	return nil
}

// allowTeamCode records the code a team document had when the person allowed
// one of its programs, so a teammate's later change to the code asks once
// more. code is what tierOf answered before the question.
func (a *app) allowTeamCode(document string, v tier.Verdict) {
	if v.Tier != tier.Team || v.Code == "" {
		return
	}
	path, err := resolveSymlinks(document)
	if err != nil {
		path = filepath.Clean(document)
	}
	if err := a.allowDocument(path, v.Code); err != nil {
		a.rt.logger.Printf("Could not record the allowed code of %s: %v", path, err)
	}
}

// documentAsk is the question: for the warning, the document, the folder it
// could reach, and why HTML Clay cannot vouch for it; for a team document, the
// program that asked.
func (a *app) documentAsk(path, program string, v tier.Verdict) confirm.Ask {
	folder := filepath.Dir(path)
	if owner, err := a.ResolveDocument(path); err == nil && (owner.Kind == OwnerTrusted || owner.Kind == OwnerServed) {
		folder = owner.FolderPath
	}
	if v.Tier == tier.Team {
		ask := confirm.Ask{
			Kind:     appcore.RequestTeamDocument,
			Document: path,
			Program:  program,
			Folder:   filepath.Base(folder),
			Identity: platform.DirIdentity(path),
			Answers:  []string{helperAnswerDeny, helperAnswerAllowDocument},
		}
		if a.scopeOf(path).Kind == config.ScopeUnknown {
			ask.Detail = helperDetailScopeUnknown
		} else if a.teamCodeChanged(path, v) {
			ask.Detail = teamCodeDetail
		}
		return ask
	}
	return confirm.Ask{
		Kind:     appcore.RequestUnknownDocument,
		Document: path,
		Folder:   filepath.Base(folder),
		Path:     folder,
		Identity: platform.DirIdentity(path),
		Detail:   v.Reason,
		Answers:  appcore.AnswersFor(appcore.RequestUnknownDocument),
	}
}

// documentDialog is the question where the request window is not there. The
// warning offers the same three answers the request window does, so "Trust it
// like a file I made" is reachable without a terminal; the team question asks
// about code and offers no trust.
type documentDialog struct{ app *app }

const (
	documentDialogDenyLabel  = "Don't allow"
	documentDialogTrustLabel = "Trust it like a file I made"
)

// closeChoice is the answer a closed, cancelled or timed out dialog reports.
// The flow stores nothing for it, so the document stays undecided.
const closeChoice = "close"

func (d documentDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	title := fmt.Sprintf(documentDialogTitle, filepath.Base(ask.Document))
	message := fmt.Sprintf(documentDialogDetail, ask.Path)
	labels := platform.ConfirmLabels{
		Allow:  helperAnswerAllowDocument,
		Always: documentDialogTrustLabel,
		Deny:   documentDialogDenyLabel,
	}
	refuse := documentAnswerDeny
	if ask.Kind == appcore.RequestTeamDocument {
		title = fmt.Sprintf(teamCodeDialogTitle, filepath.Base(ask.Document))
		message = fmt.Sprintf(teamCodeDialogMessage, ask.Folder)
		labels = platform.ConfirmLabels{Allow: helperAnswerAllowDocument, Deny: documentDialogDenyLabel}
		refuse = helperAnswerDeny
	}
	show := d.app.rt.confirmDocument
	if show == nil {
		show = func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error) {
			return d.app.dialogs().confirm(title, message, labels)
		}
	}
	choice, err := show(title, message, labels)
	if err != nil {
		return confirm.Reply{ID: ask.ID, Choice: closeChoice}, err
	}
	switch choice {
	case platform.ConfirmDeny:
		return confirm.Reply{ID: ask.ID, Choice: refuse}, nil
	case platform.ConfirmAllowAlways:
		if ask.Kind == appcore.RequestTeamDocument {
			return confirm.Reply{ID: ask.ID, Choice: helperAnswerAllowDocument}, nil
		}
		return confirm.Reply{ID: ask.ID, Choice: documentAnswerTrust}, nil
	case platform.ConfirmAllowOnce:
		return confirm.Reply{ID: ask.ID, Choice: helperAnswerAllowDocument}, nil
	}
	return confirm.Reply{ID: ask.ID, Choice: closeChoice}, nil
}

func (documentDialog) Close(string) {}
