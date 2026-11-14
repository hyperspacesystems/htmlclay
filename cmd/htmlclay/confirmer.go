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

// readConfirmer is how a page's read question reaches the person: the registry
// when the Mac app runs this host, so the question shows in the request window,
// and the native dialog otherwise.
func (a *app) readConfirmer() confirm.Confirmer {
	return a.presenter(readDialog{confirm: a.rt.confirm})
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
	confirm dialogConfirm
}

func (d readDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	show := d.confirm
	if show == nil {
		show = defaultReadDialog
	}
	allowTrust := slices.Contains(ask.Answers, readChoiceTrustFolder)
	title, message := server.ReadDialogText(ask)
	choice, err := show(title, message, allowTrust)
	return server.ReadReply(choice, allowTrust), err
}

func (readDialog) Close(string) {}

// defaultReadDialog is the real dialog the app shows when no test seam is
// installed. Its labels are this prompt's own: its wider grant really is
// trusting a folder.
func defaultReadDialog(title, message string, allowTrust bool) (platform.ConfirmChoice, error) {
	if allowTrust {
		return platform.Confirm(title, message, platform.ConfirmLabels{
			Allow:  "Allow Once",
			Always: "Trust This Folder",
		})
	}
	ok, err := platform.ConfirmWithButtons(title, message, "Allow")
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
