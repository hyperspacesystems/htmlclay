package main

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/tray"
)

func helperProgramStatus(path string) string {
	if helperProgramMissing(path) {
		return "program file missing"
	}
	if runtime.GOOS != "windows" && !helperProgramExecutable(path) {
		return "program is not executable"
	}
	return ""
}

func (a *app) helperBindingStatus(document, name string) string {
	a.helperStateMu.Lock()
	denied := slices.Contains(a.openDenials[document], name)
	a.helperStateMu.Unlock()
	if denied {
		return "denied for this open"
	}
	resolution, found := a.rt.cfg.ResolveHelper(document, name, a.trustedLive(document))
	if resolution.Program.ID != "" {
		if status := helperProgramStatus(resolution.Program.Path); status != "" {
			return status + ": " + resolution.Program.Path
		}
	}
	if resolution.Allowed {
		return "allowed: " + resolution.Program.Path
	}
	if resolution.Revoked {
		return "access removed"
	}
	for _, decision := range a.rt.cfg.HelperDecisionList() {
		if decision.Document == document && decision.Name == name && decision.Allowed {
			if _, ok := a.rt.cfg.LookupHelperProgram(decision.Program); !ok {
				return "previous program was removed"
			}
		}
	}
	if !found {
		return "no program registered"
	}
	if len(resolution.Candidates) > 1 {
		return "choose between registered programs"
	}
	return "approval needed: " + resolution.Program.Path
}

func (a *app) helperDocumentRows() []tray.Row {
	a.mu.Lock()
	documents := make(map[string]bool)
	if !a.stopping {
		for _, s := range a.sites {
			for _, registration := range s.sessions.Registrations() {
				documents[registration.Path] = true
			}
		}
	}
	a.mu.Unlock()
	paths := make([]string, 0, len(documents))
	for document := range documents {
		paths = append(paths, document)
	}
	slices.Sort(paths)
	var rows []tray.Row
	for _, document := range paths {
		names, err := readDocumentHelperNames(document)
		if err != nil {
			continue
		}
		for _, name := range names {
			label := document
			if rel, err := filepath.Rel(a.rt.home, document); err == nil && document != a.rt.home && session.EqualOrUnder(document, a.rt.home) {
				label = "~/" + filepath.ToSlash(rel)
			}
			rows = append(rows, tray.Row{
				Path:  document + "\x00" + name,
				Label: label + ": “" + name + "” " + a.trayHelperStatus(document, name),
			})
		}
	}
	return rows
}

func (a *app) helperDocumentOpenLocked(document string) bool {
	if a.stopping {
		return false
	}
	_, _, ok := a.lookupLocked(document)
	return ok
}

// trayHelperStatus is the short state the tray shows beside an open document's
// program: what it runs, or what the person can do about it.
func (a *app) trayHelperStatus(document, name string) string {
	a.helperStateMu.Lock()
	denied := slices.Contains(a.openDenials[document], name)
	a.helperStateMu.Unlock()
	if denied {
		return "denied until reopened"
	}
	resolution, _ := a.rt.cfg.ResolveHelper(document, name, a.trustedLive(document))
	switch {
	case resolution.Allowed && helperProgramMissing(resolution.Program.Path):
		return "file missing"
	case resolution.Allowed:
		return "runs " + filepath.Base(resolution.Program.Path)
	default:
		return "needs setup…"
	}
}

// trayHelperDocument is a click on an open document's row: a program that is
// not set up gets the setup dialog, and one that is opens the Programs page.
func (a *app) trayHelperDocument(key string) []tray.Row {
	document, name, _ := strings.Cut(key, "\x00")
	if resolution, _ := a.rt.cfg.ResolveHelper(document, name, a.trustedLive(document)); resolution.Allowed && !helperProgramMissing(resolution.Program.Path) {
		a.openProgramsPage()
		return a.helperDocumentRows()
	}
	return a.configureHelperDocument(key)
}

func (a *app) configureHelperDocument(key string) []tray.Row {
	return a.configureHelperDocumentWith(key, a.systemHelperApprovalDialogs())
}

func (a *app) configureHelperDocumentWith(key string, dialogs helperApprovalDialogs) []tray.Row {
	a.helperMu.Lock()
	defer a.helperMu.Unlock()
	document, name, ok := strings.Cut(key, "\x00")
	if !ok || document == "" || name == "" {
		return a.helperDocumentRows()
	}
	a.mu.Lock()
	open := a.helperDocumentOpenLocked(document)
	a.mu.Unlock()
	names, err := readDocumentHelperNames(document)
	if !open || err != nil || !slices.Contains(names, name) {
		return a.helperDocumentRows()
	}
	status := a.helperBindingStatus(document, name)
	a.rt.logger.Printf("Native helper configuration requested: document=%s name=%s status=%s", document, name, status)
	a.helperStateMu.Lock()
	if denied := slices.DeleteFunc(slices.Clone(a.openDenials[document]), func(other string) bool { return other == name }); len(denied) == 0 {
		delete(a.openDenials, document)
	} else {
		a.openDenials[document] = denied
	}
	a.helperStateMu.Unlock()
	plan := helperOpenPlan{names: names}
	plan.allowed, plan.denied = a.resolvedHelpers(document, names)
	if _, ok := plan.allowed[name]; !ok {
		a.askHelperSetup(document, names, plan, name, dialogs, true)
	}
	a.refreshHelperDispatchers()
	return a.helperDocumentRows()
}
