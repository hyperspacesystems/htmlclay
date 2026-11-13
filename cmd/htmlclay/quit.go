package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
)

// unsavedDocuments asks the service to save, and names what it could not. answered
// is false when the service did not answer, so nothing is known to be saved.
func (a *app) unsavedDocuments() (names []string, answered bool) {
	unsaved, err := a.service.flush()
	if err != nil {
		if errors.Is(err, errNoService) {
			return nil, true
		}
		a.rt.logger.Printf("The live-room service did not answer before quitting: %v", err)
		return nil, false
	}
	for _, doc := range unsaved {
		names = append(names, a.unsavedLine(doc))
	}
	return names, true
}

// unsavedBeforeQuit asks the live-room service to save every edit it holds and
// answers the documents it could not, as lines for a person to read. saved is
// true when nothing would be lost by quitting now: no service, or every edit
// saved. A service that does not answer is not proof, so it counts as unsaved.
func (a *app) unsavedBeforeQuit() (lines []string, saved bool) {
	names, answered := a.unsavedDocuments()
	if !answered {
		return []string{appcore.TextQuitNoAnswer}, false
	}
	return names, len(names) == 0
}

// unsavedLine names one unsaved document by its folder's served name.
func (a *app) unsavedLine(doc unsavedDoc) string {
	a.servedMu.RLock()
	defer a.servedMu.RUnlock()
	for name, m := range a.servedByName {
		if m.folder.ID == doc.Root {
			return name + "/" + doc.Name
		}
	}
	return doc.Name
}

// confirmQuit is the tray's Quit: it quits at once when every edit is saved,
// and otherwise asks, as Hyperclay Local does, until the edits save, the person
// cancels, or they choose to quit anyway.
func (a *app) confirmQuit() bool {
	confirm := platform.Confirm
	if a.rt.confirmQuit != nil {
		confirm = a.rt.confirmQuit
	}
	labels := platform.ConfirmLabels{Deny: appcore.TextQuitCancel, Allow: appcore.TextQuitRetry, Always: appcore.TextQuitAnyway}
	for {
		lines, saved := a.unsavedBeforeQuit()
		if saved {
			return true
		}
		choice, err := confirm(appcore.TextQuitUnsavedTitle, fmt.Sprintf(appcore.TextQuitUnsavedBody, strings.Join(lines, "\n")), labels)
		if err != nil {
			// No dialog could be shown (Linux without zenity or kdialog). Losing
			// edits takes a person's yes, so the app stays open.
			a.rt.logger.Printf("Quit dialog error: %v", err)
			for _, line := range lines {
				a.rt.logger.Printf("Not quitting, unsaved edits: %s", line)
			}
			if notifyErr := a.notifyUser(appcore.TextQuitUnsavedTitle, fmt.Sprintf(appcore.TextQuitNotQuitting, strings.Join(lines, ", "))); notifyErr != nil {
				a.rt.logger.Printf("Could not show the unsaved-edits notice: %v", notifyErr)
			}
			return false
		}
		switch choice {
		case platform.ConfirmAllowOnce:
			continue
		case platform.ConfirmAllowAlways:
			for _, line := range lines {
				a.rt.logger.Printf("Quitting with unsaved edits: %s", line)
			}
			return true
		}
		return false
	}
}
