package main

import (
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

// The tray's one menu is the host's, row for row: separators left out, and the
// Recent submenu holding its own empty row until a document opens.
func TestTrayMenuRowsFollowTheSnapshot(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	rows := a.trayMenuRows()
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	want := []string{appcore.MenuStatus, appcore.MenuOpenDashboard, appcore.MenuOpenFile, appcore.MenuRecent, appcore.MenuSettings, appcore.MenuCheckUpdates, appcore.MenuQuit}
	if len(ids) != len(want) {
		t.Fatalf("menu rows = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("menu rows = %v, want %v", ids, want)
		}
	}
	for _, r := range rows {
		if r.ID != appcore.MenuRecent {
			continue
		}
		if len(r.Children) != 1 || r.Children[0].ID != appcore.MenuNoRecent {
			t.Fatalf("an empty Recent holds %+v, want one %q row", r.Children, appcore.MenuNoRecent)
		}
		if r.Children[0].Enabled {
			t.Error("the empty Recent row must be disabled: there is nothing to open")
		}
	}
}

// The Quit row says what this system calls quitting.
func TestTrayMenuQuitTitleBySystem(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	var title string
	for _, r := range a.trayMenuRows() {
		if r.ID == appcore.MenuQuit {
			title = r.Title
		}
	}
	want := appcore.TextMenuQuitMac
	switch runtime.GOOS {
	case "windows":
		want = appcore.TextMenuQuitWindows
	case "linux":
		want = appcore.TextMenuQuitLinux
	}
	if title != want {
		t.Fatalf("the Quit row reads %q, want %q on %s", title, want, runtime.GOOS)
	}
}

// A program request in the Needs you submenu raises that request's own dialog.
// It does not send the person to the dashboard, where they would click Show
// request again to get the same dialog.
func TestTrayNeedsYouProgramRequestShowsTheRequest(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	var opened []string
	a.rt.launchBrowser = func(u string) error {
		opened = append(opened, u)
		return nil
	}

	askThroughRegistry(t, a, teamDocumentAsk("todo-search"))
	request := openRequestNamed(t, waitForOpenRequests(t, a, 1), "todo-search")

	row := ""
	for _, w := range a.snapshot().Waiting {
		if w.Kind == appcore.WaitingProgramRequest {
			row = w.ID
		}
	}
	if row == "" {
		t.Fatalf("the snapshot lists no program request row: %+v", a.snapshot().Waiting)
	}

	a.doTrayMenu(row)

	if len(opened) != 0 {
		t.Fatalf("clicking the program request opened %v, want its own dialog", opened)
	}
	for _, r := range a.snapshot().Requests {
		if r.ID == request.ID {
			if r.Attention == 0 {
				t.Errorf("the click did not ask for request %s to be shown: %+v", request.ID, r)
			}
			return
		}
	}
	t.Fatalf("request %s is gone after the click", request.ID)
}
