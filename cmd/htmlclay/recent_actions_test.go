package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/recents"
)

// newRecentTestApp builds an app whose record of recently opened documents is
// configDir's, so a test can build a second app over it and see what a restart
// leaves behind.
func newRecentTestApp(t *testing.T, home, cfgBase, configDir string) *app {
	t.Helper()
	a := newServedTestApp(t, home, cfgBase)
	a.rt.configDir = configDir
	store, err := recents.Open(configDir)
	if err != nil {
		t.Fatalf("open the recent documents record: %v", err)
	}
	a.core.mu.Lock()
	a.core.recents = store
	a.core.mu.Unlock()
	return a
}

// recentRow finds the row for path in the documents the snapshot lists.
func findRecentRow(t *testing.T, a *app, path string) (appcore.DocumentFact, bool) {
	t.Helper()
	want, err := resolveSymlinks(path)
	if err != nil {
		want = path
	}
	for _, doc := range a.facts().Documents {
		if doc.Path == want {
			return doc, true
		}
	}
	return appcore.DocumentFact{}, false
}

// recentRow is findRecentRow for the row that must be there.
func recentRow(t *testing.T, a *app, path string) appcore.DocumentFact {
	t.Helper()
	doc, ok := findRecentRow(t, a, path)
	if !ok {
		t.Fatalf("no row for %s in %+v", path, a.facts().Documents)
	}
	return doc
}

// closeTestSites closes every tab the app has open, the way quitting a browser
// window does.
func closeTestSites(t *testing.T, a *app) {
	t.Helper()
	a.mu.Lock()
	sites := append([]*site(nil), a.sites...)
	for _, s := range sites {
		a.removeSiteLocked(s)
	}
	a.mu.Unlock()
	for _, s := range sites {
		s.close()
	}
}

// forgetRateLimit clears when each document was last noted, so the next page load
// or registration would record: a test that a reload brings nothing back then
// tests the rule rather than the minute.
func forgetRateLimit(a *app) {
	a.core.mu.Lock()
	a.core.recorded = map[string]time.Time{}
	a.core.mu.Unlock()
}

func TestM3RecentActions(t *testing.T) {
	t.Run("open and close, the row stays with openNow false", func(t *testing.T) {
		home := evalTempHome(t)
		file := filepath.Join(home, "notes", "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		a.rt.launchBrowser = func(string) error { return nil }
		if _, refusal := a.do(context.Background(), coreAction(t, "open-close", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		row := recentRow(t, a, file)
		if !row.OpenNow {
			t.Fatalf("the row for %s is not open after file.open", file)
		}
		id := row.ID

		closeTestSites(t, a)
		row = recentRow(t, a, file)
		if row.ID != id {
			t.Fatalf("the row\u0027s id = %q after the tab closed, want %q", row.ID, id)
		}
		if row.OpenNow {
			t.Fatal("the row is still open after its tab closed")
		}
	})

	t.Run("restart, the row stays", func(t *testing.T) {
		home := evalTempHome(t)
		file := filepath.Join(home, "notes", "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)
		cfgBase, configDir := t.TempDir(), t.TempDir()

		first := newRecentTestApp(t, home, cfgBase, configDir)
		first.rt.launchBrowser = func(string) error { return nil }
		if _, refusal := first.do(context.Background(), coreAction(t, "open-restart", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		id := recentRow(t, first, file).ID
		closeTestSites(t, first)

		second := newRecentTestApp(t, home, cfgBase, configDir)
		row := recentRow(t, second, file)
		if row.ID != id {
			t.Fatalf("the row\u0027s id after a restart = %q, want %q", row.ID, id)
		}
		if row.OpenNow {
			t.Fatal("the row is open after a restart")
		}
		if row.OpenedAt == nil {
			t.Fatal("the row has no openedAt after a restart")
		}
	})

	t.Run("a served page load records and an asset load does not", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		document := filepath.Join(dir, "index.html")
		asset := filepath.Join(dir, "style.css")
		writeTestFile(t, document, servedTestDocument)
		writeTestFile(t, asset, "body{margin:0}\n")

		a := newServedTestApp(t, home, t.TempDir())
		if _, refusal, err := a.serveFolder(dir, "notes"); err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		host := servedHostName("notes", servedTestPort)
		if rec := servedRequest(t, a, "GET", host, "/index.html", "", nil); rec.Code != 200 {
			t.Fatalf("GET /index.html = %d, want 200", rec.Code)
		}
		recentRow(t, a, document)

		if rec := servedRequest(t, a, "GET", host, "/style.css", "", map[string]string{"Sec-Fetch-Dest": "style"}); rec.Code != 200 {
			t.Fatalf("GET /style.css = %d, want 200", rec.Code)
		}
		if _, ok := findRecentRow(t, a, asset); ok {
			t.Fatal("an asset load was recorded as an open")
		}
	})

	t.Run("delete the file, fileMissing is true and the row stays", func(t *testing.T) {
		home := evalTempHome(t)
		file := filepath.Join(home, "notes", "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		a.rt.launchBrowser = func(string) error { return nil }
		if _, refusal := a.do(context.Background(), coreAction(t, "open-missing-later", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		id := recentRow(t, a, file).ID
		closeTestSites(t, a)

		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		row := recentRow(t, a, file)
		if row.ID != id {
			t.Fatalf("the deleted document\u0027s row id = %q, want %q", row.ID, id)
		}
		if !row.FileMissing {
			t.Fatal("a deleted document\u0027s row does not say its file is missing")
		}
	})

	t.Run("forget removes it and a reload does not bring it back", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		document := filepath.Join(dir, "index.html")
		writeTestFile(t, document, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		if _, refusal, err := a.serveFolder(dir, "notes"); err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		host := servedHostName("notes", servedTestPort)
		if rec := servedRequest(t, a, "GET", host, "/index.html", "", nil); rec.Code != 200 {
			t.Fatalf("GET /index.html = %d, want 200", rec.Code)
		}
		id := recentRow(t, a, document).ID

		if _, refusal := a.do(context.Background(), coreAction(t, "forget-1", "document.forget", map[string]string{"id": id}), ChannelTray); refusal != nil {
			t.Fatalf("document.forget refused: %+v", refusal)
		}
		if _, ok := findRecentRow(t, a, document); ok {
			t.Fatal("the row is still listed after document.forget")
		}

		forgetRateLimit(a)
		if rec := servedRequest(t, a, "GET", host, "/index.html", "", nil); rec.Code != 200 {
			t.Fatalf("the reload answered %d, want 200", rec.Code)
		}
		if _, ok := findRecentRow(t, a, document); ok {
			t.Fatal("a reload brought a forgotten row back")
		}
	})

	t.Run("a document forgotten while open stays off the list until its next open", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		file := filepath.Join(dir, "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		if err := a.trustFolder(dir); err != nil {
			t.Fatalf("trustFolder(%s): %v", dir, err)
		}
		a.rt.launchBrowser = func(string) error { return nil }
		if _, refusal := a.do(context.Background(), coreAction(t, "open-forgotten", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		row := recentRow(t, a, file)
		if !row.OpenNow {
			t.Fatalf("the row for %s is not open", file)
		}
		if _, refusal := a.do(context.Background(), coreAction(t, "forget-open", "document.forget", map[string]string{"id": row.ID}), ChannelTray); refusal != nil {
			t.Fatalf("document.forget refused: %+v", refusal)
		}
		if _, ok := findRecentRow(t, a, file); ok {
			t.Fatal("a forgotten document is still listed while it is open")
		}

		forgetRateLimit(a)
		if _, ok := a.routeTrusted(file); !ok {
			t.Fatal("the document is no longer open, so the reload proves nothing")
		}
		if _, ok := findRecentRow(t, a, file); ok {
			t.Fatal("a reload brought back a document forgotten while it was open")
		}

		if _, refusal := a.do(context.Background(), coreAction(t, "open-again", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		if _, ok := findRecentRow(t, a, file); !ok {
			t.Fatal("an explicit open did not bring the document back onto the list")
		}
	})

	t.Run("document.open with an id that is not in the record answers no-such-document", func(t *testing.T) {
		home := evalTempHome(t)
		file := filepath.Join(home, "notes", "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		unknown := map[string]string{"id": "0123456789abcdef", "path": file}
		for _, channel := range []Channel{ChannelShell, ChannelDashboard} {
			if _, refusal := a.do(context.Background(), coreAction(t, "open-unknown", "document.open", unknown), channel); refusal == nil || refusal.Code != appcore.RefuseNoSuchDocument {
				t.Fatalf("document.open with an id not in the record = %+v, want %q", refusal, appcore.RefuseNoSuchDocument)
			}
		}
		if len(launched) != 0 {
			t.Fatalf("an id not in the record opened %q", launched)
		}
		if _, ok := findRecentRow(t, a, file); ok {
			t.Fatal("an id not in the record recorded a document")
		}
	})

	t.Run("a replaced file is reopened from the Mac app and refused from the dashboard", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		file := filepath.Join(dir, "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		if _, refusal := a.do(context.Background(), coreAction(t, "open-before-replace", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		id := recentRow(t, a, file).ID
		closeTestSites(t, a)

		// The new document is written beside the old one and renamed over it, so
		// the path holds a different file.
		replacement := filepath.Join(dir, "replacement.htmlclay")
		writeTestFile(t, replacement, "<!doctype html>\n<html><body><p>another document</p></body></html>\n")
		if err := os.Rename(replacement, file); err != nil {
			t.Fatal(err)
		}

		launched = nil
		if _, refusal := a.do(context.Background(), coreAction(t, "reopen-shell", "document.open", map[string]string{"id": id}), ChannelShell); refusal != nil {
			t.Fatalf("the Mac app could not reopen a replaced document: %+v", refusal)
		}
		if len(launched) != 1 || !strings.Contains(launched[0], "page.htmlclay") {
			t.Fatalf("the Mac app opened %q, want one URL naming page.htmlclay", launched)
		}

		launched = nil
		_, refusal := a.do(context.Background(), coreAction(t, "reopen-dashboard", "document.open", map[string]string{"id": id}), ChannelDashboard)
		if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("the dashboard reopening a replaced document = %+v, want %q", refusal, appcore.RefuseNeedsConfirmation)
		}
		if len(launched) != 0 {
			t.Fatalf("the dashboard opened a replaced document: %q", launched)
		}
	})

	t.Run("the document\u0027s bytes on disk are identical before and after open, close and forget", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		file := filepath.Join(dir, "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		a.rt.launchBrowser = func(string) error { return nil }
		before, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		names, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, refusal := a.do(context.Background(), coreAction(t, "bytes-open", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		row := recentRow(t, a, file)
		closeTestSites(t, a)
		if _, refusal := a.do(context.Background(), coreAction(t, "bytes-forget", "document.forget", map[string]string{"id": row.ID}), ChannelTray); refusal != nil {
			t.Fatalf("document.forget refused: %+v", refusal)
		}

		after, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("the document could not be read after open, close and forget: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("the document changed:\nbefore: %q\nafter:  %q", before, after)
		}
		left, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != len(names) {
			t.Fatalf("the folder beside the document now holds %d entries, want the %d it held", len(left), len(names))
		}
	})
}
