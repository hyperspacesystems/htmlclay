package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

// unsavedDocuments asks the service to save, and names what it could not,
// with every document an open tab says still holds unsaved edits, each once.
// answered is false when the service did not answer, so nothing it holds is
// known to be saved; the tabs' documents are still named.
func (a *app) unsavedDocuments() (names []string, answered bool) {
	paths := map[string]bool{}
	answered = true
	unsaved, err := a.service.flush()
	switch {
	case err == nil:
		for _, doc := range unsaved {
			if p, ok := a.servedDocPath(doc); ok {
				paths[p] = true
			} else {
				names = append(names, doc.Name)
			}
		}
	case errors.Is(err, errNoService):
	default:
		a.rt.logger.Printf("The live-room service did not answer before quitting: %v", err)
		answered = false
	}
	for _, p := range a.unsaved.unsavedPaths(time.Now()) {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	// One document can arrive spelled two ways (the service's folder path and
	// a tab's resolved path), so each line is named once.
	seen := map[string]bool{}
	for _, p := range sorted {
		if line := a.unsavedPathLine(p); !seen[line] {
			seen[line] = true
			names = append(names, line)
		}
	}
	return names, answered
}

// unsavedBeforeQuit asks the live-room service to save every edit it holds and
// answers the documents still unsaved, as lines for a person to read. saved is
// true when nothing would be lost by quitting now. A service that does not
// answer is not proof, so it counts as unsaved.
func (a *app) unsavedBeforeQuit() (lines []string, saved bool) {
	names, answered := a.unsavedDocuments()
	if !answered {
		return append([]string{appcore.TextQuitNoAnswer}, names...), false
	}
	return names, len(names) == 0
}

// servedDocPath is the absolute path of a document the service named by its
// served folder's id.
func (a *app) servedDocPath(doc unsavedDoc) (string, bool) {
	a.servedMu.RLock()
	defer a.servedMu.RUnlock()
	for _, m := range a.servedByName {
		if m.folder.ID == doc.Root {
			return filepath.Join(m.folder.Path, filepath.FromSlash(doc.Name)), true
		}
	}
	return "", false
}

// unsavedPathLine names one unsaved document for a person: by its served
// folder's name when a folder serves it, else by its path inside the home
// folder.
func (a *app) unsavedPathLine(abs string) string {
	a.servedMu.RLock()
	for name, m := range a.servedByName {
		if session.EqualOrUnder(abs, m.folder.Path) {
			if rel, ok := relativeWithin(abs, m.folder.Path); ok {
				a.servedMu.RUnlock()
				return name + "/" + filepath.ToSlash(rel)
			}
		}
	}
	a.servedMu.RUnlock()
	if rel, ok := relativeWithin(abs, a.rt.home); ok {
		return filepath.ToSlash(rel)
	}
	return abs
}

// confirmQuit is the tray's Quit: it quits at once when every edit is saved,
// and otherwise asks, as Hyperclay Local does, until the edits save, the person
// cancels, or they choose to quit anyway.
func (a *app) confirmQuit() bool {
	confirm := a.dialogs().confirm
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
