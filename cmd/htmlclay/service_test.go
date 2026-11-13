package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/service"
)

// serviceRepoTarget needs node on PATH and the service's node_modules, since
// the tests that use it run the real service program.
func serviceRepoTarget(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not on PATH")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "service", "node_modules")); err != nil || !info.IsDir() {
		t.Skip("service/node_modules is not installed")
	}
	t.Setenv("HTMLCLAY_SERVICE_CMD", "node "+filepath.Join(root, "service", "main.cjs"))
}

// newServiceTestApp builds an app the way main does before it serves anything:
// the runtime's service runner first, then the served host.
func newServiceTestApp(t *testing.T, home, cfgBase string) *app {
	t.Helper()
	a := buildTestApp(t, home, cfgBase)
	a.rt.port = servedTestPort
	a.service = newServiceRunner(a)
	a.startServed()
	t.Cleanup(func() {
		if a.service != nil {
			a.service.close()
		}
		if a.served != nil {
			a.served.Close()
		}
	})
	return a
}

func serveThroughDo(t *testing.T, a *app, id, dir, name string) appcore.Snapshot {
	t.Helper()
	snapshot, refusal := a.do(context.Background(), coreAction(t, id, "folder.serve", map[string]string{
		"path": dir,
		"name": name,
	}), ChannelTray)
	if refusal != nil {
		t.Fatalf("folder.serve refused: %+v", refusal)
	}
	return snapshot
}

func waitForServiceState(t *testing.T, a *app, want string, within time.Duration) appcore.Snapshot {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		snapshot := a.snapshot()
		if snapshot.Host.Service == want {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("host.service = %q after %v, want %q", snapshot.Host.Service, within, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForServiceGone(t *testing.T, a *app, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if a.service.process() == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the live-room service process is still there after %v", within)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// waitForServiceSettled waits for one exit to be noticed and for the machine to
// settle afterwards: running again with a process, or failed. The first wait is
// what keeps a running reading from before the exit ended the second one early.
func waitForServiceSettled(t *testing.T, a *app, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for a.snapshot().Host.Service == appcore.ServiceRunning {
		if time.Now().After(deadline) {
			t.Fatalf("host.service is still running %v after an exit", within)
		}
		time.Sleep(25 * time.Millisecond)
	}
	for {
		state := a.snapshot().Host.Service
		if state == appcore.ServiceFailed {
			return
		}
		if state == appcore.ServiceRunning && a.service.process() != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("host.service = %q %v after an exit, want running or failed", state, within)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func saveServed(t *testing.T, a *app, name, dir, document string) {
	t.Helper()
	host := servedHostName(name, servedTestPort)
	rec := servedRequest(t, a, http.MethodPost, host, "/_/save", document, map[string]string{
		"Document-URL": "http://" + host + "/index.html",
		"Content-Type": "text/html",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /_/save = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onDisk), "second") {
		t.Fatalf("index.html on disk = %q, want the saved bytes", onDisk)
	}
}

// Without a service command the app is exactly what it was before the service
// existed: no process, no service link, and saves on Go's native lanes.
func TestServiceOffWithoutCommand(t *testing.T) {
	if service.Bundled() {
		t.Skip("a bundled build always runs its own service")
	}
	t.Setenv("HTMLCLAY_SERVICE_CMD", "")
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	if a.service.enabled() {
		t.Fatal("the runner has a command with none in the environment")
	}
	serveThroughDo(t, a, "serve-1", dir, "notes")

	for start := time.Now(); time.Since(start) < time.Second; {
		if service := a.snapshot().Host.Service; service != appcore.ServiceOff {
			t.Fatalf("host.service = %q, want off", service)
		}
		time.Sleep(50 * time.Millisecond)
	}

	saveServed(t, a, "notes", dir, strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1))
}

func TestServiceStartsWithFirstServedFolder(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	snapshot := serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)

	// A live stream on the folder's public address is answered by the service,
	// not by Go: it reaches the room that keeps tabs on one document live.
	app := httptest.NewServer(http.HandlerFunc(a.serveHost))
	defer app.Close()
	host := servedHostName("notes", servedTestPort)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	document := "http://" + host + "/index.html"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+host+"/live-sync/stream?lane=saved&document-url="+url.QueryEscape(document), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.URL.Host = strings.TrimPrefix(app.URL, "http://")
	request.Host = host
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET the live stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /live-sync/stream = %d, want 200", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("the live stream's Content-Type = %q, want text/event-stream", contentType)
	}

	folder := snapshotFolderAt(t, snapshot, dir)
	if _, refusal := a.do(context.Background(), coreAction(t, "stop-1", "folder.stopServing", map[string]string{
		"id": folder.ID,
	}), ChannelTray); refusal != nil {
		t.Fatalf("folder.stopServing refused: %+v", refusal)
	}
	waitForServiceState(t, a, appcore.ServiceOff, 10*time.Second)
	waitForServiceGone(t, a, 10*time.Second)
}

func TestServiceKilledComesBack(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	loose := filepath.Join(home, "loose.htmlclay")
	writeTestFile(t, loose, servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)

	s, rel := a.openForTest(t, loose)
	token := tokenFor(t, s, loose)
	if rel == "" {
		t.Fatal("the loose file has no path inside its site")
	}

	before := a.service.process()
	if before == nil {
		t.Fatal("no live-room service process while a folder is served")
	}
	before.Kill()
	waitForServiceState(t, a, appcore.ServiceRunning, 10*time.Second)
	after := a.service.process()
	if after == nil || after == before {
		t.Fatalf("the live-room service did not come back as a new process (%p, then %p)", before, after)
	}

	rec := servedRequest(t, a, http.MethodPost, s.host, "/_/save/"+token, strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1), map[string]string{
		"Content-Type":   "text/html",
		"Sec-Fetch-Site": "same-origin",
		"Origin":         "http://" + s.host,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /_/save/%s = %d, want 200 (%s)", token, rec.Code, rec.Body.String())
	}
	onDisk, err := os.ReadFile(loose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onDisk), "second") {
		t.Fatalf("loose.htmlclay on disk = %q, want the saved bytes", onDisk)
	}
}

func TestServiceFiveExitsFails(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)

	for i := 0; i < 5; i++ {
		proc := a.service.process()
		if proc == nil {
			t.Fatalf("no live-room service process before exit %d", i+1)
		}
		proc.Kill()
		waitForServiceSettled(t, a, 20*time.Second)
	}

	snapshot := waitForServiceState(t, a, appcore.ServiceFailed, 20*time.Second)
	if !a.served.ServiceDetached() {
		t.Fatal("the host still consults a service that failed")
	}
	found := false
	for _, item := range snapshot.Waiting {
		if item.Kind == appcore.WaitingServiceFailed {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("the waiting list has no serviceFailed item: %+v", snapshot.Waiting)
	}

	saveServed(t, a, "notes", dir, strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1))
	if got := a.served.UnconfirmedCount(); got != 1 {
		t.Fatalf("documents waiting for the service after a native save = %d, want 1", got)
	}

	if _, refusal := a.do(context.Background(), coreAction(t, "restart-1", "service.restart", nil), ChannelTray); refusal != nil {
		t.Fatalf("service.restart refused: %+v", refusal)
	}
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	if a.served.ServiceDetached() {
		t.Fatal("the host is still detached after the service restarted")
	}
	// The restarted service asks Go which documents changed without it and
	// reconciles each one, which lets semantic writes through again.
	deadline := time.Now().Add(15 * time.Second)
	for a.served.UnconfirmedCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("documents still waiting for the restarted service after 15 s: %d", a.served.UnconfirmedCount())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServiceQuitClosesIt(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)

	a.shutdown()
	waitForServiceGone(t, a, 8*time.Second)
}
