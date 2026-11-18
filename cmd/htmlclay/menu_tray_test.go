package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
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

func TestTrayRecentDocumentOpensIt(t *testing.T) {
	home := evalTempHome(t)
	file := filepath.Join(home, "notes", "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)
	a := newServedTestApp(t, home, t.TempDir())
	var launched []string
	a.rt.launchBrowser = func(target string) error {
		launched = append(launched, target)
		return nil
	}
	if _, refusal := a.do(context.Background(), coreAction(t, "open-first", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
		t.Fatalf("file.open refused: %+v", refusal)
	}
	id := recentRow(t, a, file).ID

	launched = nil
	a.doTrayMenu(id)

	if len(launched) != 1 || !strings.Contains(launched[0], "page.htmlclay") {
		t.Fatalf("clicking the recent row opened %q, want one URL naming page.htmlclay", launched)
	}
}

func TestTrayNeedsYouOpensWhereEachRowIsFixed(t *testing.T) {
	request := "r1"
	for _, tc := range []struct {
		item   appcore.WaitingItem
		action string
		route  string
	}{
		{appcore.WaitingItem{Kind: appcore.WaitingFolderMissing, Pane: appcore.PaneFolders}, "dashboard.open", "/settings/folders"},
		{appcore.WaitingItem{Kind: appcore.WaitingServiceFailed, Pane: appcore.PaneAdvanced}, "dashboard.open", "/settings/advanced"},
		{appcore.WaitingItem{Kind: appcore.WaitingSyncKeyRefused, Pane: appcore.PaneSync}, "dashboard.open", "/settings/sync"},
		{appcore.WaitingItem{Kind: appcore.WaitingConflict, Pane: appcore.PaneFolders}, "dashboard.open", "/needs-you"},
		{appcore.WaitingItem{Kind: appcore.WaitingProgramRequest, Pane: appcore.PanePrograms, SubjectID: &request}, "request.show", ""},
	} {
		action, args := waitingTrayAction(tc.item)
		if action != tc.action || (tc.route != "" && args["route"] != tc.route) {
			t.Errorf("%s row: %s %v, want %s %s", tc.item.Kind, action, args, tc.action, tc.route)
		}
	}
}
