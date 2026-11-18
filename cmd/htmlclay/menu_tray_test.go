package main

import (
	"path/filepath"
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

// A built-in program's row carries its switch and, once it is on, the row that
// opens its example, which turns it back off.
func TestTraySettingsBuiltinRows(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.rt.cfg.SetBuiltinOn("file-search", true)

	rows := a.traySettingItems("builtin")
	at := -1
	for i, r := range rows {
		if r.Title == "File search" {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("the built-in rows hold no File search row: %+v", rows)
	}
	if rows[at].Checked == nil || !*rows[at].Checked {
		t.Errorf("File search is on but its row does not show a check: %+v", rows[at])
	}
	if at+1 >= len(rows) || rows[at+1].Title != "Open example" || rows[at+1].Action != "example:file-search" {
		t.Fatalf("the row after File search is %+v, want its example", rows)
	}

	a.doTraySetting("setOn:file-search")
	for _, p := range a.snapshot().Programs {
		if p.ID == "file-search" && p.On {
			t.Fatal("setOn:file-search left the program on")
		}
	}
	for _, r := range a.traySettingItems("builtin") {
		if r.Title == "Open example" {
			t.Fatalf("a program that is off still offers its example: %+v", r)
		}
	}
}

// Show sync log is a row only once sync has been on, which sync.json records.
func TestTraySettingsSyncLogNeedsEverOn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	for _, r := range a.traySettingItems("advanced") {
		if r.Title == "Show sync log" {
			t.Fatalf("advanced offers Show sync log before sync was ever on: %+v", r)
		}
	}

	b := newTestApp(t, t.TempDir())
	writeTestFile(t, filepath.Join(b.rt.configDir, "sync.json"), `{"v":1,"deviceId":"d","syncSessions":[{"id":"s-1","rootId":"r-1","accountId":40,"kind":"team"}]}`)
	found := false
	for _, r := range b.traySettingItems("advanced") {
		if r.Title == "Show sync log" {
			found = true
			if r.Action != "syncLog" || !r.Enabled {
				t.Errorf("Show sync log row = %+v, want an enabled syncLog row", r)
			}
		}
	}
	if !found {
		t.Fatal("advanced holds no Show sync log row after sync has been on")
	}
}
