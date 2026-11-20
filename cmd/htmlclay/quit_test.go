package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
)

// quitUnsavedApp builds an app whose live-room service holds one unsaved edit:
// a live-store update the checkpoint cannot render.
func quitUnsavedApp(t *testing.T) *app {
	t.Helper()
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), `<!doctype html><html><head><title>Notes</title></head><body><script type="application/json" merge="notes" clay="live">{"text":"before"}</script></body></html>`)
	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)

	server := httptest.NewServer(http.HandlerFunc(a.serveHost))
	t.Cleanup(server.Close)
	host := servedHostName("notes", servedTestPort)
	doc := "http://" + host + "/index.html"
	req, err := http.NewRequest(http.MethodGet, server.URL+"/live-sync/stream?lane=live&client-id=quit&document-url="+url.QueryEscape(doc), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	reader := bufio.NewReader(res.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, ": connected") {
			break
		}
	}
	headers := map[string]string{"Document-URL": doc, "Origin": "http://" + host}
	rec := servedRequest(t, a, http.MethodGet, host, "/_/sync/data?client-id=quit&document-url="+url.QueryEscape(doc), "", headers)
	var state map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &state) != nil || state["conn"] == nil {
		t.Fatalf("live state: %d %s", rec.Code, rec.Body.String())
	}
	body, _ := json.Marshal(map[string]any{"conn": state["conn"], "epoch": state["epoch"], "name": "notes", "n": 1,
		"ops": []any{map[string]any{"op": "set", "path": []string{"text"}, "value": "accepted"}}})
	headers["Content-Type"] = "application/json"
	if rec := servedRequest(t, a, http.MethodPost, host, "/_/sync/data?client-id=quit", string(body), headers); rec.Code != http.StatusOK {
		t.Fatalf("live update: %d %s", rec.Code, rec.Body.String())
	}
	if lines, saved := a.unsavedBeforeQuit(); saved || len(lines) != 1 || lines[0] != "notes/index.html" {
		t.Fatalf("unsavedBeforeQuit = %v %v, want notes/index.html unsaved", lines, saved)
	}
	return a
}

// TestQuitHoldsUnsavedEdits proves a quit never drops edits the live-room
// service reports unsaved: host.quit refuses unless forced, and the tray's Quit
// asks first, staying open on Cancel and quitting on Quit Anyway. The unsaved
// edit is a live-store update the checkpoint cannot render (found by review).
func TestQuitHoldsUnsavedEdits(t *testing.T) {
	a := quitUnsavedApp(t)

	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.quit"}); refusal == nil || refusal.Code != appcore.RefuseQuitUnsaved {
		t.Fatalf("host.quit = %+v, want %s", refusal, appcore.RefuseQuitUnsaved)
	}

	var asked []string
	answer := platform.ConfirmDismissed
	a.rt.confirmQuit = func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error) {
		asked = append(asked, message)
		return answer, nil
	}
	if a.confirmQuit() {
		t.Fatal("the tray quit with unsaved edits after Cancel")
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "notes/index.html") {
		t.Fatalf("the dialog asked %q, want it to name notes/index.html", asked)
	}
	answer = platform.ConfirmAllowAlways
	if !a.confirmQuit() {
		t.Fatal("Quit Anyway did not quit")
	}
	if a.service.state() != appcore.ServiceRunning {
		t.Fatalf("asking about quitting changed the service: %s", a.service.state())
	}
}

// TestQuitRefusalNamesDocuments proves host.quit's refusal says which documents
// it would lose, in the same "<served name>/<file>" form the tray's dialog uses.
func TestQuitRefusalNamesDocuments(t *testing.T) {
	a := quitUnsavedApp(t)
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.quit"})
	if refusal == nil || refusal.Code != appcore.RefuseQuitUnsaved {
		t.Fatalf("host.quit = %+v, want %s", refusal, appcore.RefuseQuitUnsaved)
	}
	if len(refusal.Documents) != 1 || refusal.Documents[0] != "notes/index.html" {
		t.Fatalf("host.quit documents = %v, want [notes/index.html]", refusal.Documents)
	}

	c := openDashboardOver(t, a, nil)
	status, body := c.act("quit-1", "host.quit", map[string]any{})
	var answer struct {
		Refusal struct {
			Code      string   `json:"code"`
			Documents []string `json:"documents"`
		} `json:"refusal"`
	}
	if err := json.Unmarshal([]byte(body), &answer); status != http.StatusConflict || err != nil {
		t.Fatalf("host.quit from a dashboard page = %d %s", status, body)
	}
	if answer.Refusal.Code != appcore.RefuseQuitUnsaved || !slices.Equal(answer.Refusal.Documents, []string{"notes/index.html"}) {
		t.Fatalf("host.quit from a dashboard page = %s, want %s naming notes/index.html (the Windows panel's Quit question lists them)", body, appcore.RefuseQuitUnsaved)
	}
}

// TestQuitByeAfterFlush proves shutdown saves what the live-room service holds
// before it tells the Mac app's streams bye.
func TestQuitByeAfterFlush(t *testing.T) {
	a := quitUnsavedApp(t)
	a.shellDone = make(chan struct{})
	var mu sync.Mutex
	var order []string
	a.service.afterSave = func() {
		mu.Lock()
		order = append(order, "save")
		mu.Unlock()
	}
	bye := make(chan struct{})
	go func() {
		<-a.shellDone
		mu.Lock()
		order = append(order, "bye")
		mu.Unlock()
		close(bye)
	}()
	a.shutdown()
	<-bye
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "save" || got[1] != "bye" {
		t.Fatalf("shutdown order = %v, want [save bye]", got)
	}
}

// TestQuitDialogLabels proves the dialog's own buttons: Cancel is the Deny
// choice, so a dismissed dialog and a Cancel click cannot disagree.
func TestQuitDialogLabels(t *testing.T) {
	a := quitUnsavedApp(t)
	var labels platform.ConfirmLabels
	answer := platform.ConfirmDismissed
	a.rt.confirmQuit = func(title, message string, got platform.ConfirmLabels) (platform.ConfirmChoice, error) {
		labels = got
		return answer, nil
	}
	if a.confirmQuit() {
		t.Fatal("Cancel quit the app")
	}
	if labels.Deny != appcore.TextQuitCancel {
		t.Fatalf("the dialog's Deny label is %q, want %q", labels.Deny, appcore.TextQuitCancel)
	}
	answer = platform.ConfirmDeny
	if a.confirmQuit() {
		t.Fatal("an explicit Deny quit the app")
	}
}

// TestQuitDialogErrorQuits proves a quit still terminates when no dialog can be
// shown, so a machine without zenity or kdialog is not left unable to quit.
// TestQuitDialogErrorStaysOpen: losing unsaved edits takes a person's yes, so
// a dialog that cannot be shown (Linux without zenity or kdialog) keeps the app
// open.
func TestQuitDialogErrorStaysOpen(t *testing.T) {
	a := quitUnsavedApp(t)
	var notices []string
	a.rt.notify = func(title, message string) error {
		notices = append(notices, message)
		return nil
	}
	asked := 0
	a.rt.confirmQuit = func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error) {
		asked++
		return platform.ConfirmDismissed, errors.New("no dialog could be shown")
	}
	if a.confirmQuit() {
		t.Fatal("a quit whose dialog failed went ahead and lost the unsaved edits")
	}
	if asked != 1 {
		t.Fatalf("the dialog was tried %d times, want 1", asked)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "notes/") {
		t.Fatalf("notices = %q, want one naming the unsaved document", notices)
	}
}

// TestQuitRestartForUpdateCancel proves an update restart holds the same line as
// a quit: unsaved edits and a Cancel leave this version running.
func TestQuitRestartForUpdateCancel(t *testing.T) {
	a := quitUnsavedApp(t)
	a.rt.confirmQuit = func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error) {
		return platform.ConfirmDismissed, nil
	}
	installed := false
	err := a.restartForUpdate("9.9.9", func(handoffPath string) error {
		installed = true
		return nil
	})
	if !errors.Is(err, errUpdateCancelled) {
		t.Fatalf("restartForUpdate with an unsaved edit and Cancel = %v, want %v", err, errUpdateCancelled)
	}
	if installed {
		t.Fatal("install ran after the person cancelled the restart")
	}
	if a.updating.Load() {
		t.Fatal("the cancelled restart left the app marked as updating")
	}
}

// TestQuitShutdownRunsOnce proves two shutdowns, from two goroutines as the tray,
// host.quit and the shell can all ask, finish cleanly and run the teardown once.
func TestQuitShutdownRunsOnce(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), `<!doctype html><html><head><title>Notes</title></head><body><p>notes</p></body></html>`)
	a := newServiceTestApp(t, home, t.TempDir())
	logger, readLog := diagnosticLog(t)
	a.rt.logger = logger
	serveThroughDo(t, a, "serve-1", dir, "notes")

	var wait sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			a.shutdown()
		}()
	}
	close(start)
	wait.Wait()

	a.mu.Lock()
	stopping := a.stopping
	a.mu.Unlock()
	if !stopping {
		t.Fatal("two shutdowns left the app not stopping")
	}
	if ran := strings.Count(readLog(), "Shutting down..."); ran != 1 {
		t.Fatalf("the teardown ran %d times for two shutdowns, want 1", ran)
	}
}

// TestQuitAsksAboutTabsWithUnsavedEdits proves Quit hears a page that saves only
// on Ctrl+S: a tab's report reaches the same unsaved list every Quit path uses,
// a second tab on the same document is listed once, and the report can arrive
// over the document's own unsaved route.
func TestQuitAsksAboutTabsWithUnsavedEdits(t *testing.T) {
	home := evalTempHome(t)
	a := newTestApp(t, home)
	doc := filepath.Join(home, "notes", "index.html")
	writeTestFile(t, doc, "<html><body>notes</body></html>")

	a.noteUnsavedReport(doc, "t1", 1, true, false)
	lines, saved := a.unsavedBeforeQuit()
	if saved || len(lines) != 1 || lines[0] != "notes/index.html" {
		t.Fatalf("unsavedBeforeQuit = %v %v, want notes/index.html unsaved", lines, saved)
	}

	a.noteUnsavedReport(doc, "t2", 1, true, false)
	if lines, saved = a.unsavedBeforeQuit(); saved || len(lines) != 1 || lines[0] != "notes/index.html" {
		t.Fatalf("a second unsaved tab changed the list: %v %v", lines, saved)
	}

	a.noteUnsavedReport(doc, "t1", 2, false, false)
	a.noteUnsavedReport(doc, "t2", 2, false, false)
	if lines, saved = a.unsavedBeforeQuit(); !saved || len(lines) != 0 {
		t.Fatalf("after both tabs saved: %v %v, want saved with no lines", lines, saved)
	}

	s, _ := a.openForTest(t, doc)
	token := tokenFor(t, s, doc)
	target := "http://" + s.host + "/_/unsaved/" + token
	if code, out := postSameOrigin(t, target, "application/json", `{"tab":"t9","seq":1,"unsaved":true}`); code != http.StatusNoContent {
		t.Fatalf("POST %s = %d %s, want 204", target, code, out)
	}
	if lines, saved = a.unsavedBeforeQuit(); saved || len(lines) != 1 || lines[0] != "notes/index.html" {
		t.Fatalf("after the tab's report: %v %v, want notes/index.html unsaved", lines, saved)
	}
}

// Stop serving waits while an open tab says it holds unsaved edits: the tab could
// not save them once the folder's address is gone. The refusal names the document
// by its served name, also when the folder was added in other letter case, as a
// case-insensitive disk allows.
func TestStopServingWaitsForUnsavedTabs(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "proj")
	doc := filepath.Join(dir, "index.html")
	writeTestFile(t, doc, servedTestDocument)
	added := dir
	if _, err := os.Stat(filepath.Join(home, "PROJ")); err == nil {
		added = filepath.Join(home, "PROJ")
	}
	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(added); err != nil {
		t.Fatalf("trustFolder(%s): %v", added, err)
	}
	entry, refusal, err := a.serveFolder(added, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	a.noteUnsavedReport(doc, "t1", 1, true, false)
	var unsaved errUnsavedEdits
	if err := a.stopServing(entry.ID); !errors.As(err, &unsaved) || len(unsaved.Names) != 1 || unsaved.Names[0] != "notes/index.html" {
		t.Fatalf("stopServing with an unsaved tab = %v (%+v), want notes/index.html unsaved", err, unsaved)
	}
	a.noteUnsavedReport(doc, "t1", 2, false, false)
	if err := a.stopServing(entry.ID); err != nil {
		t.Fatalf("stopServing after the tab saved: %v", err)
	}
}

// Untrusting a folder takes its open pages' address, so they can neither save
// nor say "saved" as they close: what they reported is forgotten, or Quit would
// ask about them for ten minutes.
func TestUntrustForgetsItsTabsReports(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "proj")
	doc := filepath.Join(dir, "index.html")
	writeTestFile(t, doc, "<html><body>notes</body></html>")
	a := newTestApp(t, home)
	if err := a.trustFolder(dir); err != nil {
		t.Fatalf("trustFolder(%s): %v", dir, err)
	}
	a.openForTest(t, doc)
	a.noteUnsavedReport(doc, "t1", 1, true, false)
	if _, saved := a.unsavedBeforeQuit(); saved {
		t.Fatal("the tab's report did not count before untrusting")
	}
	if err := a.untrustFolder(dir); err != nil {
		t.Fatalf("untrustFolder: %v", err)
	}
	if lines, saved := a.unsavedBeforeQuit(); !saved {
		t.Fatalf("after untrusting: %v, want nothing unsaved", lines)
	}
}
