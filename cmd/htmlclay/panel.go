package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/panel"
	"github.com/panphora/htmlclay/internal/tray"
)

// newTrayPanel is the Windows panel a left click on the tray icon opens, or
// nil where there is none: outside Windows, and on a PC without the WebView2
// runtime, where a left click keeps showing the menu. Two starts that fail in
// a row turn it off for this run and give the left click back to the menu.
func (a *app) newTrayPanel() *panel.Supervisor {
	if !panel.RuntimeInstalled() {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		a.rt.logger.Printf("The panel is unavailable: %v", err)
		return nil
	}
	data := filepath.Join(a.rt.configDir, "panel-webview")
	return &panel.Supervisor{
		Spawn:           func() *exec.Cmd { return panel.ChildCommand(exe, data) },
		Entry:           a.panelEntry,
		Allow:           panel.AllowForeground,
		OnUnavailable:   tray.MenuOnLeftClick,
		Logf:            a.rt.logger.Printf,
		MaxFailedStarts: 2,
	}
}

// panelEntry signs the panel in to its own page, when it starts and whenever
// its page's session has ended.
func (a *app) panelEntry() (string, error) {
	if a.dashboard == nil {
		return "", errors.New("the dashboard is not running")
	}
	return a.dashboard.EntryURL("/panel")
}

// panelTapped is the tray's left click. It runs on the tray's own thread,
// which must return at once, so it only reads where the click was there.
func (a *app) panelTapped() {
	at := panel.CursorPos()
	go func() {
		if err := a.trayPanel.Toggle(int(at.X), int(at.Y)); err != nil && !errors.Is(err, panel.ErrUnavailable) {
			a.rt.logger.Printf("The panel did not open: %v", err)
		}
	}()
}
