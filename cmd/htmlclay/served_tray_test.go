package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/testutil"
)

// servedTrayCapture collects what the tray hooks told the person and opened.
type servedTrayCapture struct {
	notified chan string
	launched []string
}

func newServedTrayCapture(a *app) *servedTrayCapture {
	c := &servedTrayCapture{notified: make(chan string, 4)}
	a.rt.notify = func(title, message string) error {
		c.notified <- title + ": " + message
		return nil
	}
	a.rt.launchBrowser = func(target string) error {
		c.launched = append(c.launched, target)
		return nil
	}
	return c
}

// The menu lists what is being served and nothing else: a folder the person
// stopped serving keeps its config entry and its name but has no address, so it
// must not appear. A folder that has gone missing stays listed, marked, because
// its address is still the thing they need to see and stop.
func TestServedTrayRowsListOnlyServingFoldersAndMarkAMissingOne(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(home, "notes")
	drafts := filepath.Join(home, "drafts")
	for _, dir := range []string{notes, drafts} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	a := newServedTestApp(t, home, t.TempDir())
	servedNotes, refusal, err := a.serveFolderNamed(notes, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolderNamed(notes): refusal=%+v err=%v", refusal, err)
	}
	servedDrafts, refusal, err := a.serveFolderNamed(drafts, "drafts")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolderNamed(drafts): refusal=%+v err=%v", refusal, err)
	}

	rows := a.servedFolderRows()
	if len(rows) != 2 {
		t.Fatalf("%d rows for two served folders, want 2", len(rows))
	}
	for _, r := range rows {
		if !r.Serving {
			t.Fatalf("row %q is not marked as a served folder", r.Path)
		}
	}

	if err := a.stopServing(servedDrafts.ID); err != nil {
		t.Fatalf("stopServing: %v", err)
	}
	rows = a.servedFolderRows()
	if len(rows) != 1 || rows[0].Path != servedNotes.ID {
		t.Fatalf("rows after stopping drafts = %+v, want only %s", rows, servedNotes.ID)
	}
	if !strings.HasPrefix(rows[0].Label, "notes") || !strings.Contains(rows[0].Label, "~/notes") {
		t.Fatalf("label = %q, want the name and the path with home shown as ~", rows[0].Label)
	}
	if strings.Contains(rows[0].Label, appcore.TextFolderMissing) {
		t.Fatalf("label %q is marked missing while the folder is there", rows[0].Label)
	}

	if err := os.RemoveAll(notes); err != nil {
		t.Fatal(err)
	}
	rows = a.servedFolderRows()
	if len(rows) != 1 {
		t.Fatalf("%d rows after deleting the folder, want it still listed once", len(rows))
	}
	if !strings.Contains(rows[0].Label, appcore.TextFolderMissing) {
		t.Fatalf("label = %q, want it to carry %q once the folder is gone", rows[0].Label, appcore.TextFolderMissing)
	}
}

// Serving from the menu serves the picked folder, tells the person where it now
// answers, and opens that address: nothing else on screen confirms it worked.
func TestServedTrayPickServesNotifiesAndLaunches(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newServedTestApp(t, home, t.TempDir())
	capture := newServedTrayCapture(a)
	a.rt.selectFolder = func(prompt string) (string, bool, error) {
		if prompt != appcore.TextPickServeFolder {
			t.Errorf("picker prompt = %q, want %q", prompt, appcore.TextPickServeFolder)
		}
		return dir, true, nil
	}

	rows := a.pickAndServeFolder()
	if len(rows) != 1 {
		t.Fatalf("%d rows after serving the picked folder, want 1", len(rows))
	}
	entry, ok := a.rt.cfg.ServedFolderByName("notes")
	if !ok || !entry.Serving || entry.Path != dir {
		t.Fatalf("served entry = %+v (found %v), want %s serving as notes", entry, ok, dir)
	}

	address := fmt.Sprintf("http://notes.htmlclay.localhost:%d/", servedTestPort)
	if len(capture.launched) != 1 || capture.launched[0] != address {
		t.Fatalf("launched %q, want exactly %q", capture.launched, address)
	}
	got := testutil.Receive(t, 10*time.Second, "the now-served notification", capture.notified)
	want := "HTML Clay: " + fmt.Sprintf(appcore.TextNowServed, "notes", strings.TrimSuffix(address, "/"))
	if got != want {
		t.Fatalf("notification = %q, want %q", got, want)
	}
}

// Picking the home folder is the refusal a person is most likely to hit, and it
// has to come back as the plain-language reason rather than a silent nothing.
func TestServedTrayPickRefusesTheHomeFolder(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	a := newServedTestApp(t, home, t.TempDir())
	capture := newServedTrayCapture(a)
	a.rt.selectFolder = func(string) (string, bool, error) {
		return home, true, nil
	}

	rows := a.pickAndServeFolder()
	if len(rows) != 0 {
		t.Fatalf("%d rows after a refused pick, want none", len(rows))
	}
	if len(a.rt.cfg.ServedFolderList()) != 0 {
		t.Fatalf("a refused pick recorded %+v", a.rt.cfg.ServedFolderList())
	}
	if len(capture.launched) != 0 {
		t.Fatalf("a refused pick launched %q", capture.launched)
	}
	got := testutil.Receive(t, 10*time.Second, "the refusal notification", capture.notified)
	want := appcore.TextCannotServe + ": " + appcore.RefusalText[appcore.RefuseIsHome]
	if got != want {
		t.Fatalf("notification = %q, want %q", got, want)
	}
}

// Stopping closes an address a bookmark may point at, so it asks first, with the
// affirmative button saying what it does.
func TestServedTrayStopAsksFirst(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := a.serveFolderNamed(dir, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolderNamed: refusal=%+v err=%v", refusal, err)
	}

	var title, message, affirmative string
	a.rt.confirmTrust = func(t, m, a string) (bool, error) {
		title, message, affirmative = t, m, a
		return false, nil
	}
	rows := a.stopServedFromTray(entry.ID)
	if len(rows) != 1 {
		t.Fatalf("declining left %d rows, want the folder still served", len(rows))
	}
	if stored, ok := a.rt.cfg.ServedFolderByID(entry.ID); !ok || !stored.Serving {
		t.Fatalf("declining the confirm stopped serving: %+v", stored)
	}
	address := a.servedAddress(entry)
	if title != appcore.TextStopServingTitle || affirmative != appcore.TextStopServing {
		t.Fatalf("dialog = (%q, %q), want (%q, %q)", title, affirmative, appcore.TextStopServingTitle, appcore.TextStopServing)
	}
	if want := fmt.Sprintf(appcore.TextStopServingMessage, address); message != want {
		t.Fatalf("dialog body = %q, want %q", message, want)
	}

	a.rt.confirmTrust = func(string, string, string) (bool, error) { return true, nil }
	rows = a.stopServedFromTray(entry.ID)
	if len(rows) != 0 {
		t.Fatalf("%d rows after stopping, want none", len(rows))
	}
	if stored, ok := a.rt.cfg.ServedFolderByID(entry.ID); !ok || stored.Serving {
		t.Fatalf("confirming did not stop serving: %+v (found %v)", stored, ok)
	}
}

// Copy Address hands the address a person would type to the clipboard seam, so
// the test proves the exact string rather than a clipboard nobody can read back.
func TestServedTrayCopyAddressHandsTheAddressToTheClipboard(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := a.serveFolderNamed(dir, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolderNamed: refusal=%+v err=%v", refusal, err)
	}

	var copied []string
	a.rt.copyText = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	a.copyServedAddress(entry.ID)
	want := fmt.Sprintf("http://notes.htmlclay.localhost:%d/", servedTestPort)
	if len(copied) != 1 || copied[0] != want {
		t.Fatalf("clipboard got %q, want exactly %q", copied, want)
	}
}
