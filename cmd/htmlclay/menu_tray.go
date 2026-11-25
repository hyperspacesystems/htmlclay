package main

import (
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tray"
)

// trayMenuRows is the one menu for the Windows and Linux tray, separators
// included.
func (a *app) trayMenuRows() []tray.MenuItem {
	s := a.snapshot()
	phase := appcore.PhaseOf(s)
	var convert func([]appcore.MenuRow) []tray.MenuItem
	convert = func(rows []appcore.MenuRow) []tray.MenuItem {
		out := make([]tray.MenuItem, 0, len(rows))
		for _, r := range rows {
			out = append(out, tray.MenuItem{ID: r.ID, Title: appcore.MenuTitle(r, s, phase), Enabled: r.Enabled, Children: convert(r.Children)})
		}
		return out
	}
	return convert(appcore.MenuRows(s, phase))
}

// doTrayMenu runs a clicked row of the one menu. Two rows do more from the
// tray than their action: Open a file shows the tray's own chooser, and Check
// for updates announces its result in a notification.
func (a *app) doTrayMenu(id string) {
	var refusal *appcore.Refusal
	switch id {
	case appcore.MenuOpenFile:
		path, ok, err := platform.ChooseFile("Open a file")
		if err != nil {
			a.rt.logger.Printf("Could not show the file chooser: %v", err)
			refusal = &appcore.Refusal{Code: appcore.RefuseFailed}
		} else if ok {
			a.openFile(path)
		}
	case appcore.MenuCheckUpdates:
		refusal = a.checkForUpdatesFromTray()
	default:
		if typ, args := menuAction(id, a.snapshot()); typ != "" {
			refusal = a.trayDo(typ, args)
		}
	}
	a.reportTrayRefusal(refusal)
}

// menuAction is the action one row of the one menu runs, for the tray and for
// the Windows panel's gear menu (GET /api/app/menu); "" for a row that only
// shows something. A Recent or Needs you child's id is its document's or its
// item's.
func menuAction(id string, s appcore.Snapshot) (string, map[string]any) {
	switch id {
	case appcore.MenuStatus:
		if s.Update.Ready == nil {
			return "", nil
		}
		return "update.start", map[string]any{}
	case appcore.MenuPauseSync:
		return "sync.setPaused", map[string]any{"paused": !s.Sync.Paused}
	case appcore.MenuOpenDashboard:
		return "dashboard.open", map[string]any{}
	case appcore.MenuOpenFile:
		return "file.open", map[string]any{}
	case appcore.MenuRecovered:
		return "app.openRecoveredChanges", map[string]any{}
	case appcore.MenuSettings:
		return "dashboard.open", map[string]any{"route": "/settings"}
	case appcore.MenuCheckUpdates:
		return "update.check", map[string]any{}
	case appcore.MenuQuit:
		return "host.quit", map[string]any{}
	}
	for _, d := range s.Documents {
		if d.ID == id {
			return "document.open", map[string]any{"id": id}
		}
	}
	for _, w := range s.Waiting {
		if w.ID == id {
			return waitingTrayAction(w)
		}
	}
	return "", nil
}

// reportTrayRefusal tells the person why a tray click did nothing.
func (a *app) reportTrayRefusal(refusal *appcore.Refusal) {
	if refusal == nil {
		return
	}
	a.rt.logger.Printf("Tray action refused: %s", appcore.RefusalMessage(refusal))
	_ = a.notifyUser(appcore.RefusalMessage(refusal), "")
}

// waitingTrayAction is what a Needs you row in the tray does: a program request
// shows its own dialog, a conflict or a held removal opens Needs you where it is
// answered, and anything else opens the Settings section that can fix it, as
// the Mac app opens its pane.
func waitingTrayAction(w appcore.WaitingItem) (string, map[string]any) {
	switch {
	case w.Kind == appcore.WaitingProgramRequest && w.SubjectID != nil && *w.SubjectID != "":
		return "request.show", map[string]any{"id": *w.SubjectID}
	case w.Kind == appcore.WaitingConflict || w.Kind == appcore.WaitingRemovalHeld || w.Pane == "":
		return "dashboard.open", map[string]any{"route": "/needs-you"}
	}
	return "dashboard.open", map[string]any{"route": "/settings/" + w.Pane}
}
