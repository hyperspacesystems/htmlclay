package main

import (
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tray"
)

// trayMenuRows is the one menu for the Windows and Linux tray.
func (a *app) trayMenuRows() []tray.MenuItem {
	s := a.snapshot()
	phase := appcore.PhaseOf(s)
	var convert func([]appcore.MenuRow) []tray.MenuItem
	convert = func(rows []appcore.MenuRow) []tray.MenuItem {
		out := make([]tray.MenuItem, 0, len(rows))
		for _, r := range rows {
			if r.ID == appcore.MenuSeparator {
				continue
			}
			out = append(out, tray.MenuItem{ID: r.ID, Title: appcore.MenuTitle(r, s, phase), Enabled: r.Enabled, Children: convert(r.Children)})
		}
		return out
	}
	return convert(appcore.MenuRows(s, phase))
}

// doTrayMenu runs a clicked row of the one menu.
func (a *app) doTrayMenu(id string) {
	s := a.snapshot()
	var refusal *appcore.Refusal
	switch id {
	case appcore.MenuStatus:
		refusal = a.trayDo("update.start", map[string]any{})
	case appcore.MenuPauseSync:
		refusal = a.trayDo("sync.setPaused", map[string]any{"paused": !s.Sync.Paused})
	case appcore.MenuOpenDashboard:
		refusal = a.trayDo("dashboard.open", map[string]any{})
	case appcore.MenuOpenFile:
		path, ok, err := platform.ChooseFile("Open a file")
		if err != nil {
			a.rt.logger.Printf("Could not show the file chooser: %v", err)
			refusal = &appcore.Refusal{Code: appcore.RefuseFailed}
		} else if ok {
			a.openFile(path)
		}
	case appcore.MenuRecovered:
		refusal = a.trayDo("app.openRecoveredChanges", map[string]any{})
	case appcore.MenuCheckUpdates:
		refusal = a.trayDo("update.check", map[string]any{})
	default:
		for _, d := range s.Documents {
			if d.ID == id {
				refusal = a.trayDo("document.open", map[string]any{"documentID": id})
				break
			}
		}
		for _, w := range s.Waiting {
			if w.ID == id {
				refusal = a.trayDo("dashboard.open", map[string]any{"route": "/needs-you"})
				break
			}
		}
	}
	a.reportTrayRefusal(refusal)
}

// reportTrayRefusal tells the person why a tray click did nothing.
func (a *app) reportTrayRefusal(refusal *appcore.Refusal) {
	if refusal == nil {
		return
	}
	a.rt.logger.Printf("Tray action refused: %s", appcore.RefusalMessage(refusal))
	_ = a.notifyUser(appcore.RefusalMessage(refusal), "")
}
