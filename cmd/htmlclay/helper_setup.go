package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
)

const (
	helperSetupDeny        = "Deny"
	helperSetupMoreOptions = "More Options…"
	helperSetupChooseFile  = "Choose File…"
	helperSetupCancel      = "Cancel"
	helperSetupChooseOther = "Choose Another File…"
	helperSetupTrusted     = "Allow in Trusted Folders"
	helperSetupAllowAnyway = "Allow Anyway"
)

const (
	helperAnswerDeny            = "deny"
	helperAnswerAllowDocument   = "allow-document"
	helperAnswerAllowAllTrusted = "allow-all-trusted"
	helperAnswerChooseFile      = "choose-file"
	helperAnswerAllowAnyway     = "allow-anyway"
)

// helperTeamDocumentSentence is the app's team copy, shown beside a document a
// team shares so the person knows why it asks one at a time.
const helperTeamDocumentSentence = "This document is in a folder a team shares. Its editors can change it later, so a program is allowed for this document only." // TODO-COPY: helper setup dialog sentence for a team document, max 160 chars

// helperSetupPresenter is the one setup flow's view of who answers: the
// request window under the Mac app, today's dialogs otherwise. widePick is the
// dialog presenter's no-suggestion widening, which the request window never
// offers.
type helperSetupPresenter struct {
	confirm.Confirmer
	widePick bool
}

// helperSuggestNow is the clock the setup dialog judges file ages by. Tests
// move it forward so files they just wrote count as settled.
var helperSuggestNow = time.Now

// askHelperSetup is the one question that connects a program to a document. The
// person started it, from the setup bar, the page's own button or the tray. It
// leads with the real file HTML Clay found and quotes the name the page uses, so
// the page cannot dress the request up. The person answers through the presenter:
// the request window under the Mac app, today's dialogs otherwise. Caller holds
// helperMu; it is released while the person decides.
func (a *app) askHelperSetup(document string, names []string, plan helperOpenPlan, name string, dialogs helperApprovalDialogs, notify bool) helperOpenPlan {
	before := plan.allowed
	defer func() {
		_, has := plan.allowed[name]
		_, had := before[name]
		if has && !had {
			return
		}
		a.helperStateMu.Lock()
		if a.pageAskQuiet == nil {
			a.pageAskQuiet = make(map[string]time.Time)
		}
		a.pageAskQuiet[document] = time.Now().Add(pageAskQuietFor)
		a.helperStateMu.Unlock()
	}()
	report := func(err error) {
		if notify {
			a.reportHelperSetupError(document, err)
		} else {
			a.rt.logger.Printf("Could not configure helpers for %s: %v", document, err)
		}
	}

	if !a.reserveHelperSetup(document, name) {
		a.rt.logger.Printf("A program question for %s in %s is already open; not asking again", name, document)
		return plan
	}
	defer a.releaseHelperSetup(document, name)

	suggestion := a.suggestHelperProgram(document, name, helperSuggestNow())
	// What the question shows is this exact file. Saving checks it is still the
	// same file, so a swap or a retargeted link while the question is open never
	// gets the permission the person gave the file they saw.
	var shown os.FileInfo
	if suggestion.path != "" {
		shown = helperFileSnapshot(suggestion.path)
	}
	presenter := a.setupConfirmer(dialogs, suggestion)
	ask := a.helperSetupAsk(document, name, suggestion, a.scopeOf(document), presenter.widePick)

	// The person can take minutes. helperMu stays free so another document's
	// setup, and Remove Access, still run; everything is checked again below.
	a.helperMu.Unlock()
	defer a.helperMu.Lock()
	ctx := context.Background()
	reply, err := presenter.Ask(ctx, ask)
	if err != nil {
		report(fmt.Errorf("could not show the program dialog: %w", err))
		return plan
	}
	defer func() { presenter.Close(reply.ID) }()

	scope := platform.ConfirmAllowOnce
	switch reply.Choice {
	case helperAnswerDeny:
		a.addOpenDenials(document, []string{name})
		a.rt.logger.Printf("Program %s denied for this open of %s; the next direct open can ask again", name, document)
		plan.allowed, plan.denied = a.resolvedHelpers(document, names)
		return plan
	case helperAnswerAllowDocument, helperAnswerChooseFile:
		scope = platform.ConfirmAllowOnce
	case helperAnswerAllowAllTrusted:
		scope = platform.ConfirmAllowAlways
	default:
		a.rt.logger.Printf("Program dialog for %s in %s closed without an answer", name, document)
		return plan
	}

	path, program := suggestion.path, suggestion.program
	if path == "" || reply.Choice == helperAnswerChooseFile {
		if presenter.widePick {
			picked, ok, err := a.chooseHelperFile(dialogs, document, "Choose the program for “"+name+"” in "+filepath.Base(document), a.helperPickerFolder(document, name))
			if err != nil {
				report(fmt.Errorf("could not choose %s: %w", name, err))
				return plan
			}
			if !ok {
				a.rt.logger.Printf("Program picker for %s in %s was cancelled; nothing was saved", name, document)
				return plan
			}
			path, program = picked, config.HelperProgram{}
			if ok, err := dialogs.prepare(path); err != nil {
				report(fmt.Errorf("could not prepare %s: %w", name, err))
				return plan
			} else if !ok {
				return plan
			}
		} else {
			picked, ok, err := a.helperRegistryFile(ctx, presenter, ask, reply.ID, document, name, reply.Path, true)
			if err != nil {
				report(fmt.Errorf("could not choose %s: %w", name, err))
				return plan
			}
			if !ok {
				a.rt.logger.Printf("Program picker for %s in %s was cancelled; nothing was saved", name, document)
				return plan
			}
			path, program = picked, config.HelperProgram{}
			// A file the person picked is one document only, whatever the
			// question before it offered.
			scope = platform.ConfirmAllowOnce
		}
	} else if again := a.suggestHelperProgram(document, name, helperSuggestNow()); again.path != path || again.program.ID != program.ID || !sameFileAsShown(path, shown) {
		report(fmt.Errorf("the program for %s changed while the dialog was open; nothing was saved", name))
		return plan
	} else if presenter.widePick {
		if ok, err := dialogs.prepare(path); err != nil {
			report(fmt.Errorf("could not prepare %s: %w", name, err))
			return plan
		} else if !ok {
			return plan
		}
	} else {
		checked, ok, err := a.helperRegistryFile(ctx, presenter, ask, reply.ID, document, name, path, false)
		if err != nil {
			report(fmt.Errorf("could not prepare %s: %w", name, err))
			return plan
		}
		if !ok {
			return plan
		}
		if checked != path {
			path, program = checked, config.HelperProgram{}
			scope = platform.ConfirmAllowOnce
		}
	}

	// The question and the picker can take minutes. Store nothing for a document
	// that stopped declaring the program meanwhile.
	if current, err := readDocumentHelperNames(document); err != nil || !slices.Contains(current, name) {
		report(fmt.Errorf("%s no longer declares %s; nothing was saved", filepath.Base(document), name))
		return plan
	}

	candidate := helperCandidate{name: name, path: path}
	for _, p := range a.rt.cfg.HelperProgramList() {
		if p.Name == name {
			candidate.registered = append(candidate.registered, p)
		}
	}
	// An allow for this document may reuse the file's registration under any
	// name. Allowing every trusted document needs a registration under the name
	// the documents ask for, because that is the name ResolveHelper matches.
	if program.ID != "" && (scope == platform.ConfirmAllowOnce || program.Name == name) {
		candidate.program = program
	}
	if err := a.saveHelperDecisionSet(document, []helperCandidate{candidate}, scope); err != nil {
		report(err)
		return plan
	}
	plan.allowed, plan.denied = a.resolvedHelpers(document, names)
	return plan
}

// helperSetupAsk is the question the flow puts to the person: the file HTML Clay
// found, if any, and the answers this document's scope may offer.
func (a *app) helperSetupAsk(document, name string, suggestion helperSuggestion, scope config.Scope, widePick bool) confirm.Ask {
	ask := confirm.Ask{
		Document: document,
		Program:  name,
		Identity: platform.DirIdentity(document),
	}
	if suggestion.path == "" {
		ask.Kind = appcore.RequestNoFile
		ask.Answers = []string{helperAnswerDeny, helperAnswerChooseFile}
		if widePick {
			ask.Answers = []string{helperAnswerDeny, helperAnswerAllowAllTrusted, helperAnswerChooseFile}
		}
		return ask
	}
	ask.Kind = appcore.RequestFound
	ask.Path = suggestion.path
	ask.Answers = []string{helperAnswerDeny, helperAnswerAllowDocument}
	switch scope.Kind {
	case config.ScopeTeam:
		ask.Kind = appcore.RequestTeamDocument
		ask.Folder = filepath.Base(filepath.Dir(document))
	case config.ScopeUnknown:
		ask.Detail = helperDetailScopeUnknown
	default:
		ask.Answers = append(ask.Answers, helperAnswerAllowAllTrusted)
	}
	ask.Answers = append(ask.Answers, helperAnswerChooseFile)
	return ask
}

// helperRegistryFile checks a file the request window's presenter will grant,
// asking again on the same request about a risky file or one without its
// executable bit. It returns the file to grant, which may be one the person
// chose instead, and ok=false when nothing was decided.
func (a *app) helperRegistryFile(ctx context.Context, presenter helperSetupPresenter, ask confirm.Ask, id, document, name, path string, risky bool) (string, bool, error) {
	for {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("%s is not a file", path)
		}
		if risky {
			if detail := a.helperFileRiskDetail(document, path, helperSuggestNow()); detail != "" {
				reply, err := presenter.Ask(ctx, a.helperRiskyAsk(ask, id, path, detail))
				if err != nil {
					return "", false, err
				}
				switch reply.Choice {
				case helperAnswerAllowAnyway:
					risky = false
				case helperAnswerChooseFile:
					path = reply.Path
					continue
				default:
					return "", false, nil
				}
			}
		}
		needs, err := helperProgramNeedsExecutable(path)
		if err != nil {
			return "", false, err
		}
		if needs {
			reply, err := presenter.Ask(ctx, a.helperRiskyAsk(ask, id, path, helperDetailNotExecutable))
			if err != nil {
				return "", false, err
			}
			switch reply.Choice {
			case helperAnswerAllowAnyway:
				if ok, err := makeHelperProgramExecutable(path); err != nil || !ok {
					return "", false, err
				}
			case helperAnswerChooseFile:
				path = reply.Path
				continue
			default:
				return "", false, nil
			}
		}
		return path, true, nil
	}
}

// helperRiskyAsk is the same request, asked again about one file.
func (a *app) helperRiskyAsk(ask confirm.Ask, id, path, detail string) confirm.Ask {
	ask.ID = id
	ask.Kind = appcore.RequestRiskyFile
	ask.Detail = detail
	ask.Path = path
	ask.Answers = []string{helperAnswerDeny, helperAnswerChooseFile, helperAnswerAllowAnyway}
	return ask
}

// reserveHelperSetup stops a second question about the same program in the same
// document while the first waits for a person. Caller holds helperMu.
func (a *app) reserveHelperSetup(document, name string) bool {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	key := document + "\x00" + name
	if a.helperSetupOpen[key] {
		return false
	}
	if a.helperSetupOpen == nil {
		a.helperSetupOpen = make(map[string]bool)
	}
	a.helperSetupOpen[key] = true
	return true
}

func (a *app) releaseHelperSetup(document, name string) {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	delete(a.helperSetupOpen, document+"\x00"+name)
}

// sameFileAsShown reports whether path is still the file a dialog showed: the
// same file, size and modification time.
func sameFileAsShown(path string, shown os.FileInfo) bool {
	if shown == nil {
		return false
	}
	now, err := os.Stat(path)
	return err == nil && os.SameFile(shown, now) && now.Size() == shown.Size() && now.ModTime().Equal(shown.ModTime())
}

func helperFileSnapshot(path string) os.FileInfo {
	info, err := os.Stat(path)
	// Windows resolves Stat's file identity lazily. Cache it before the dialog
	// opens, while the path still points to the file being offered.
	if err != nil || !os.SameFile(info, info) {
		return nil
	}
	return info
}

// chooseHelperFile opens the file picker. A file HTML Clay would not have
// suggested (an interpreter, a download, a file changed minutes ago, or one
// beside the document or in Downloads) gets a warning the person must answer
// before it is used.
func (a *app) chooseHelperFile(dialogs helperApprovalDialogs, document, prompt, folder string) (string, bool, error) {
	for {
		picked, ok, err := dialogs.choose(prompt, nearestFolder(folder))
		if err != nil || !ok {
			return "", ok, err
		}
		reason := a.helperFileRisk(document, picked, helperSuggestNow())
		if reason == "" {
			return picked, true, nil
		}
		message := fmt.Sprintf("%s %s.\n\nAllow it only if you know where it came from. It runs as you and can read or change any file your account can access.", a.tildePath(picked), reason)
		choice, err := dialogs.setup("Use this file?", message, helperSetupCancel, helperSetupChooseOther, helperSetupAllowAnyway)
		if err != nil {
			return "", false, err
		}
		switch choice {
		case platform.ChoiceAffirm:
			return picked, true, nil
		case platform.ChoiceMiddle:
			folder = filepath.Dir(picked)
		default:
			return "", false, nil
		}
	}
}

// nearestFolder is dir or its closest ancestor that still exists. A relink starts
// the picker in the missing program's folder, and macOS refuses to open a picker
// at a folder that is gone (AppleScript -1700), which failed every relink of a
// program whose whole folder had moved.
func nearestFolder(dir string) string {
	for dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// helperAllowLabel is the affirmative button for a found file: its name, or a
// plain "Allow File" when the name would make the button unreadable.
func helperAllowLabel(path string) string {
	base := filepath.Base(path)
	if len(base) > 24 {
		return "Allow File"
	}
	return "Allow " + base
}

func (a *app) helperSetupMessage(document, name string, suggestion helperSuggestion) string {
	var b strings.Builder
	base := filepath.Base(document)
	if suggestion.path != "" {
		fmt.Fprintf(&b, "%s wants to run this program:\n\n    %s\n    %s\n\nThe document calls it “%s”.", base, filepath.Base(suggestion.path), a.tildePath(suggestion.path), name)
		if suggestion.program.ID != "" && suggestion.program.Name != name {
			fmt.Fprintf(&b, " You allowed this file before as “%s”, which is why “%s” was not connected.", suggestion.program.Name, name)
		}
	} else {
		fmt.Fprintf(&b, "%s wants to run a program it calls “%s”. HTML Clay did not find it in the folders where you keep programs, so choose the file yourself.", base, name)
	}
	for _, line := range suggestion.withheld {
		b.WriteString("\n\n" + line)
	}
	b.WriteString("\n\nThe program runs as you and can read or change any file your account can access. Deny lasts until you open the document again.")
	return b.String()
}

func (a *app) helperTrustedFoldersMessage(name string, suggestion helperSuggestion) string {
	target := "the program you choose next"
	if suggestion.path != "" {
		target = a.tildePath(suggestion.path)
	}
	return fmt.Sprintf("Allow %s for every document in your trusted folders that asks for “%s”?\n\n"+
		"Those documents will run it with their own input, without asking you. Documents outside your trusted folders still ask, one at a time.\n\n"+
		"Choose Another File allows a different file for this document only.", target, name)
}

func (a *app) helperChooseAnotherMessage(name string, suggestion helperSuggestion) string {
	target := "the program you choose next"
	if suggestion.path != "" {
		target = a.tildePath(suggestion.path)
	}
	return fmt.Sprintf("Choose a different file for “%s” in this document?\n\n%s is allowed for this document only.", name, target)
}
