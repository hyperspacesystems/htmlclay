package main

import (
	"context"
	"encoding/json"
	"github.com/panphora/htmlclay/internal/wire"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// formerVisit is a page visit arriving at an address the app served on before.
func formerVisit(t *testing.T, a *app, method, host, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://"+host+target, nil)
	req.Host = host
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	a.formerPortHandler(4000).ServeHTTP(rec, req)
	return rec
}

func TestFormerPortSendsPageVisitsOn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.rt.port = 5000
	a.former = []formerPort{{port: 4000}}

	rec := formerVisit(t, a, http.MethodGet, "localhost:4000", "/docs/a.html?x=1")
	if rec.Code != http.StatusFound {
		t.Fatalf("page visit = %d %s, want 302", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "http://localhost:5000/docs/a.html?x=1" {
		t.Fatalf("redirected to %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin" {
		t.Fatalf("Referrer-Policy = %q, want strict-origin", got)
	}
}

func TestFormerPortTellsSavesWhereTheAppWent(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.rt.port = 5000
	a.former = []formerPort{{port: 4000}}

	rec := formerVisit(t, a, http.MethodPost, "localhost:4000", "/docs/a.html?x=1")
	if rec.Code != http.StatusConflict {
		t.Fatalf("save = %d %s, want 409", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("409 body: %v", err)
	}
	if body["code"] != "port-moved" {
		t.Fatalf("code = %q, want port-moved", body["code"])
	}
	if want := "http://localhost:5000/docs/a.html?x=1"; body["to"] != want {
		t.Fatalf("to = %q, want %q", body["to"], want)
	}
}

func TestFormerPortRefusesOtherHosts(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.rt.port = 5000
	a.former = []formerPort{{port: 4000}}

	if rec := formerVisit(t, a, http.MethodGet, "evil.example:4000", "/docs/a.html"); rec.Code != http.StatusForbidden {
		t.Fatalf("another host = %d, want 403", rec.Code)
	}
	if rec := formerVisit(t, a, http.MethodGet, "localhost:4001", "/docs/a.html"); rec.Code != http.StatusForbidden {
		t.Fatalf("a port that is not former = %d, want 403", rec.Code)
	}
}

func TestSetPortRefusesATakenPort(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: port})
	if refusal == nil || refusal.Code != appcore.RefusePortTaken {
		t.Fatalf("host.setPort = %+v, want %s", refusal, appcore.RefusePortTaken)
	}
	if refusal.OtherName != strconv.Itoa(port) {
		t.Fatalf("refusal names port %q, want %d", refusal.OtherName, port)
	}
}

func TestSetPortRefusesUnsavedEdits(t *testing.T) {
	a := quitUnsavedApp(t)
	port := freeLoopbackPort(t)

	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: port})
	if refusal == nil || refusal.Code != appcore.RefusePortUnsaved {
		t.Fatalf("host.setPort = %+v, want %s", refusal, appcore.RefusePortUnsaved)
	}
	if len(refusal.Documents) != 1 || refusal.Documents[0] != "notes/index.html" {
		t.Fatalf("refusal names %v, want notes/index.html", refusal.Documents)
	}
}

func TestSetPortSavesAndRestarts(t *testing.T) {
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, t.TempDir(), cfgBase)
	restarted := make(chan struct{}, 1)
	a.rt.restartHost = func() { restarted <- struct{}{} }
	oldPort := a.rt.cfg.Port()
	port := freeLoopbackPort(t)

	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: port}); refusal != nil {
		t.Fatalf("host.setPort = %+v", refusal)
	}
	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatal("the port change never restarted the app")
	}

	fresh, _, err := config.LoadFrom(cfgBase, platform.DirIdentity)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fresh.Port() != port {
		t.Fatalf("saved port = %d, want %d", fresh.Port(), port)
	}
	if got := fresh.FormerPortList(); len(got) != 1 || got[0] != oldPort {
		t.Fatalf("former ports = %v, want [%d]", got, oldPort)
	}
}

func TestPortRestartReopensSettingsWithTheTray(t *testing.T) {
	if got := portRestartArgs(42, false); !slices.Equal(got, []string{"--after-pid=42", "--open-dashboard=/settings/advanced"}) {
		t.Fatalf("with the tray: %q", got)
	}
	if got := portRestartArgs(42, true); !slices.Equal(got, []string{"--after-pid=42", "--no-tray"}) {
		t.Fatalf("without the tray: %q", got)
	}
}

func TestSetPortMovesBackToAFormerPort(t *testing.T) {
	a := newTestAppWithConfigDir(t, t.TempDir(), t.TempDir())
	restarted := make(chan struct{}, 1)
	a.rt.restartHost = func() { restarted <- struct{}{} }
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port
	a.mu.Lock()
	a.former = append(a.former, formerPort{port: port})
	a.mu.Unlock()

	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: port}); refusal != nil {
		t.Fatalf("moving back to former port %d = %+v, want a restart", port, refusal)
	}
	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatal("moving back never restarted the app")
	}
	if got := a.rt.cfg.Port(); got != port {
		t.Fatalf("port = %d, want %d", got, port)
	}
}

func TestSetPortWaitsForARunningAIEdit(t *testing.T) {
	home := evalTempHome(t)
	file := filepath.Join(home, "notes", "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)
	a := newServedTestApp(t, home, t.TempDir())
	restarted := false
	a.rt.restartHost = func() { restarted = true }
	if _, refusal := a.do(context.Background(), coreAction(t, "open-first", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
		t.Fatalf("file.open refused: %+v", refusal)
	}
	core, ok := a.wireCoreFor(file)
	if !ok {
		t.Fatal("no wire core for the document")
	}
	core.AIEdit.Mu.Lock()
	if core.AIEdit.Running == nil {
		core.AIEdit.Running = map[string]wire.AIEditRun{}
	}
	core.AIEdit.Running[file] = wire.AIEditRun{}
	core.AIEdit.Mu.Unlock()
	t.Cleanup(func() {
		core.AIEdit.Mu.Lock()
		delete(core.AIEdit.Running, file)
		core.AIEdit.Mu.Unlock()
	})

	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: freeLoopbackPort(t)})
	if refusal == nil || refusal.Code != appcore.RefuseStillRunning {
		t.Fatalf("host.setPort during an AI edit = %+v, want %s", refusal, appcore.RefuseStillRunning)
	}
	if restarted {
		t.Fatal("the port change restarted the app and stopped the AI edit")
	}
}
