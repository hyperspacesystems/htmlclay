package main

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/server"
)

// readChoiceTrustFolder is the answer that records a durable trust. It is the
// one choice a page's read question and its own-folder question share.
const readChoiceTrustFolder = "trust-folder"

// dialogConfirm shows a native dialog and reports what the person clicked. It
// is the seam a test drives instead of the real dialog.
type dialogConfirm func(title, message string, allowTrust bool) (platform.ConfirmChoice, error)

// platformDialogs is the one seam every native dialog in this package goes
// through. Under the Mac app there is no window a dialog could appear in, and a
// question nobody sees is a grant nobody gave, so a call there is refused: it
// logs one line and answers deny. A test hook stands in for showing the dialog,
// so a call that would open one fails the test instead of the machine.
type platformDialogs struct {
	app *app
}

func (a *app) dialogs() platformDialogs { return platformDialogs{app: a} }

// attempt reports whether this call is refused instead of shown.
func (d platformDialogs) attempt(name string) bool {
	if d.app.shell {
		d.app.rt.logger.Printf("No %s dialog when the Mac app runs this host; the answer is no", name)
		return true
	}
	if d.app.rt.dialogAttempt != nil {
		d.app.rt.dialogAttempt(name)
		return true
	}
	return false
}

func (d platformDialogs) confirm(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error) {
	if d.attempt("confirm") {
		return platform.ConfirmDeny, nil
	}
	return platform.Confirm(title, message, labels)
}

func (d platformDialogs) confirmWithButtons(title, message, allowLabel string) (bool, error) {
	if d.attempt("confirm") {
		return false, nil
	}
	return platform.ConfirmWithButtons(title, message, allowLabel)
}

func (d platformDialogs) confirmChange(title, message, actionLabel string) (bool, error) {
	if d.attempt("confirm-change") {
		return false, nil
	}
	return platform.ConfirmChange(title, message, actionLabel)
}

func (d platformDialogs) choose(title, message, refuse, middle, affirm string) (platform.Choice, error) {
	if d.attempt("choose") {
		return platform.ChoiceRefuse, nil
	}
	return platform.Choose(title, message, refuse, middle, affirm)
}

func (d platformDialogs) selectFile(prompt, dir string) (string, bool, error) {
	if d.attempt("select-file") {
		return "", false, nil
	}
	return platform.SelectFile(prompt, dir)
}

func (d platformDialogs) selectFolder(prompt string) (string, bool, error) {
	if d.attempt("select-folder") {
		return "", false, nil
	}
	return platform.SelectFolder(prompt)
}

// readConfirmer is how a page's read question reaches the person: the registry
// when the Mac app runs this host, so the question shows in the request window,
// and the native dialog otherwise.
func (a *app) readConfirmer() confirm.Confirmer {
	return a.presenter(readDialog{app: a, confirm: a.rt.confirm})
}

// trustConfirmer is readConfirmer for a page's request to trust its own folder.
// The dialog wording is built by the flow, because it names the requesting file
// and whether the person reached it from a link.
func (a *app) trustConfirmer(title, message string) confirm.Confirmer {
	return a.presenter(trustDialog{app: a, title: title, message: message})
}

// presenter picks the one presenter this launch uses. Only the presenter
// differs: the flow that decides what to ask, and what an answer does, is the
// same either way.
func (a *app) presenter(dialog confirm.Confirmer) confirm.Confirmer {
	if a.shell {
		return a.registry()
	}
	return dialog
}

// setupConfirmer is readConfirmer for a program setup question: the request
// window under the Mac app, today's dialogs otherwise. widePick is the dialog
// presenter's no-suggestion widening, which the request window never offers.
func (a *app) setupConfirmer(dialogs helperApprovalDialogs, suggestion helperSuggestion) helperSetupPresenter {
	return helperSetupPresenter{
		Confirmer: a.presenter(nativeSetupConfirmer{app: a, dialogs: dialogs, suggestion: suggestion}),
		widePick:  !a.shell,
	}
}

// nativeSetupConfirmer presents a program setup question with today's dialogs:
// the first names the file, and More Options opens the wider grant, or a
// different file where the wider grant is not offered.
type nativeSetupConfirmer struct {
	app        *app
	dialogs    helperApprovalDialogs
	suggestion helperSuggestion
}

func (c nativeSetupConfirmer) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	setup := c.dialogs.setup
	if setup == nil {
		setup = c.app.systemHelperApprovalDialogs().setup
	}
	affirm := helperSetupChooseFile
	if c.suggestion.path != "" {
		affirm = helperAllowLabel(c.suggestion.path)
	}
	message := c.app.helperSetupMessage(ask.Document, ask.Program, c.suggestion)
	if ask.Kind == appcore.RequestTeamDocument {
		message += "\n\n" + helperTeamDocumentSentence
	}
	choice, err := setup("Allow a program for "+filepath.Base(ask.Document)+"?", message, helperSetupDeny, helperSetupMoreOptions, affirm)
	if err != nil {
		return confirm.Reply{}, err
	}
	switch choice {
	case platform.ChoiceRefuse:
		return confirm.Reply{Choice: helperAnswerDeny}, nil
	case platform.ChoiceAffirm:
		if c.suggestion.path == "" {
			return confirm.Reply{Choice: helperAnswerChooseFile}, nil
		}
		return confirm.Reply{Choice: helperAnswerAllowDocument}, nil
	case platform.ChoiceMiddle:
	default:
		return confirm.Reply{}, nil
	}

	if slices.Contains(ask.Answers, helperAnswerAllowAllTrusted) {
		choice, err = setup("More options for “"+ask.Program+"”", c.app.helperTrustedFoldersMessage(ask.Program, c.suggestion), helperSetupCancel, helperSetupChooseOther, helperSetupTrusted)
		if err != nil {
			return confirm.Reply{}, err
		}
		switch choice {
		case platform.ChoiceAffirm:
			return confirm.Reply{Choice: helperAnswerAllowAllTrusted}, nil
		case platform.ChoiceMiddle:
			return confirm.Reply{Choice: helperAnswerChooseFile}, nil
		}
		return confirm.Reply{}, nil
	}
	confirmOther := c.dialogs.confirm
	if confirmOther == nil {
		return confirm.Reply{Choice: helperAnswerChooseFile}, nil
	}
	ok, err := confirmOther("More options for “"+ask.Program+"”", c.app.helperChooseAnotherMessage(ask.Program, c.suggestion), helperSetupChooseOther)
	if err != nil {
		return confirm.Reply{}, err
	}
	if ok {
		return confirm.Reply{Choice: helperAnswerChooseFile}, nil
	}
	return confirm.Reply{}, nil
}

func (nativeSetupConfirmer) Close(string) {}

// isMainFolder reports whether dir is one of the main personal folders. The
// request window says so beside the path, because trusting one of them reaches
// files the person never wrote.
func (a *app) isMainFolder(dir string) bool {
	return a.rt.policy.IsPersonal(dir)
}

// readDialog presents the read question with today's native dialog.
type readDialog struct {
	app     *app
	confirm dialogConfirm
}

func (d readDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	show := d.confirm
	if show == nil {
		show = d.app.readPrompt
	}
	allowTrust := slices.Contains(ask.Answers, readChoiceTrustFolder)
	title, message := server.ReadDialogText(ask)
	choice, err := show(title, message, allowTrust)
	return server.ReadReply(choice, allowTrust), err
}

func (readDialog) Close(string) {}

// readPrompt is the real dialog the app shows when no test seam is installed.
// Its labels are this prompt's own: its wider grant really is trusting a folder.
func (a *app) readPrompt(title, message string, allowTrust bool) (platform.ConfirmChoice, error) {
	if allowTrust {
		return a.dialogs().confirm(title, message, platform.ConfirmLabels{
			Allow:  "Allow Once",
			Always: "Trust This Folder",
		})
	}
	ok, err := a.dialogs().confirmWithButtons(title, message, "Allow")
	if err != nil || !ok {
		return platform.ConfirmDeny, err
	}
	return platform.ConfirmAllowOnce, nil
}

// trustDialog presents the page's own-folder question with today's two-button
// dialog, whose affirmative label is what the person clicked.
type trustDialog struct {
	app     *app
	title   string
	message string
}

func (d trustDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	trusted, err := d.app.confirmTrustRequest(d.title, d.message, "Trust Folder")
	if err != nil || !trusted {
		return confirm.Reply{ID: ask.ID, Choice: confirm.ChoiceDeny}, err
	}
	return confirm.Reply{ID: ask.ID, Choice: readChoiceTrustFolder}, nil
}

func (trustDialog) Close(string) {}
