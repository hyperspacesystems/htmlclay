package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

const (
	helperSetupDeny        = "Deny"
	helperSetupMoreOptions = "More Options…"
	helperSetupChooseFile  = "Choose File…"
	helperSetupCancel      = "Cancel"
	helperSetupChooseOther = "Choose Another File…"
	helperSetupTrusted     = "Allow in Trusted Folders"
)

// helperSuggestNow is the clock the setup dialog judges file ages by. Tests
// move it forward so files they just wrote count as settled.
var helperSuggestNow = time.Now

// askHelperSetup is the one dialog that connects a program to a document. The
// person started it, from the setup bar, the page's own button or the tray. It
// leads with the real file HTML Clay found and quotes the name the page uses,
// so the page cannot dress the request up. No button is the keyboard default.
// Allow binds this document only; More Options can widen to every document in
// a trusted folder. Caller holds helperMu.
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

	suggestion := a.suggestHelperProgram(document, name, helperSuggestNow())
	affirm := helperSetupChooseFile
	if suggestion.path != "" {
		affirm = helperAllowLabel(suggestion.path)
	}
	// What the dialog shows is this exact file. Saving checks it is still the
	// same file, so a swap or a retargeted link while the dialog is open never
	// gets the permission the person gave the file they saw.
	var shown os.FileInfo
	if suggestion.path != "" {
		shown = helperFileSnapshot(suggestion.path)
	}
	choice, err := dialogs.setup(
		"Allow a program for "+filepath.Base(document)+"?",
		a.helperSetupMessage(document, name, suggestion),
		helperSetupDeny, helperSetupMoreOptions, affirm,
	)
	if err != nil {
		report(fmt.Errorf("could not show the program dialog: %w", err))
		return plan
	}
	scope := platform.ConfirmAllowOnce
	switch choice {
	case platform.ChoiceNone:
		a.rt.logger.Printf("Program dialog for %s in %s closed without an answer", name, document)
		return plan
	case platform.ChoiceRefuse:
		a.addOpenDenials(document, []string{name})
		a.rt.logger.Printf("Program %s denied for this open of %s; the next direct open can ask again", name, document)
		plan.allowed, plan.denied = a.resolvedHelpers(document, names)
		return plan
	case platform.ChoiceMiddle:
		choice, err = dialogs.setup(
			"More options for “"+name+"”",
			a.helperTrustedFoldersMessage(name, suggestion),
			helperSetupCancel, helperSetupChooseOther, helperSetupTrusted,
		)
		if err != nil {
			report(fmt.Errorf("could not show the program dialog: %w", err))
			return plan
		}
		switch choice {
		case platform.ChoiceMiddle:
			suggestion = helperSuggestion{}
		case platform.ChoiceAffirm:
			scope = platform.ConfirmAllowAlways
		default:
			return plan
		}
	case platform.ChoiceAffirm:
	default:
		report(errors.New("the program dialog returned an invalid choice"))
		return plan
	}

	path, program := suggestion.path, suggestion.program
	if path == "" {
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
	} else if again := a.suggestHelperProgram(document, name, helperSuggestNow()); again.path != path || again.program.ID != program.ID || !sameFileAsShown(path, shown) {
		report(fmt.Errorf("the program for %s changed while the dialog was open; nothing was saved", name))
		return plan
	}
	if ok, err := dialogs.prepare(path); err != nil {
		report(fmt.Errorf("could not prepare %s: %w", name, err))
		return plan
	} else if !ok {
		return plan
	}

	// The dialog and the picker can take minutes. Store nothing for a document
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
		picked, ok, err := dialogs.choose(prompt, folder)
		if err != nil || !ok {
			return "", ok, err
		}
		reason := a.helperFileRisk(document, picked, helperSuggestNow())
		if reason == "" {
			return picked, true, nil
		}
		message := fmt.Sprintf("%s %s.\n\nAllow it only if you know where it came from. It runs as you and can read or change any file your account can access.", a.tildePath(picked), reason)
		choice, err := dialogs.setup("Use this file?", message, helperSetupCancel, helperSetupChooseOther, "Allow Anyway")
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
