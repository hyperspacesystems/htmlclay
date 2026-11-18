package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/wire"
)

func TestSetPortRestartAnywayStopsARunningAIEdit(t *testing.T) {
	home := evalTempHome(t)
	file := filepath.Join(home, "notes", "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)
	a := newServedTestApp(t, home, t.TempDir())
	restarted := make(chan struct{}, 1)
	a.rt.restartHost = func() { restarted <- struct{}{} }
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
	port := freeLoopbackPort(t)
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: port})
	if refusal == nil || refusal.Code != appcore.RefuseRestartStopsWork {
		t.Fatalf("unconfirmed host.setPort during an AI edit = %+v, want %s", refusal, appcore.RefuseRestartStopsWork)
	}
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: port, Confirmed: true}); refusal != nil {
		t.Fatalf("confirmed host.setPort = %+v, want a restart", refusal)
	}
	select {
	case <-restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Restart anyway never restarted the app")
	}
	if a.rt.cfg.Port() != port {
		t.Fatalf("port = %d, want %d", a.rt.cfg.Port(), port)
	}
}

func TestSetPortRestartAnywayLetsASaveInFlightLandFirst(t *testing.T) {
	a := newTestAppWithConfigDir(t, t.TempDir(), t.TempDir())
	a.rt.syncQuiet = func(time.Duration) bool { return false }
	var landed atomic.Bool
	restartedAfterLanding := make(chan bool, 1)
	a.rt.restartHost = func() { restartedAfterLanding <- landed.Load() }
	if !a.admitWrite() {
		t.Fatal("a write was refused before the port change")
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		landed.Store(true)
		a.rt.writesInFlight.Add(-1)
	}()
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: freeLoopbackPort(t), Confirmed: true}); refusal != nil {
		t.Fatalf("confirmed host.setPort = %+v", refusal)
	}
	select {
	case ok := <-restartedAfterLanding:
		if !ok {
			t.Fatal("the app restarted while a save was still being written")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Restart anyway never restarted the app")
	}
	if a.admitWrite() {
		a.rt.writesInFlight.Add(-1)
		t.Fatal("a save arriving after the restart began was admitted, so it could be cut off")
	}
}

func TestSetPortRestartAnywayStillRefusesUnsavedEdits(t *testing.T) {
	a := quitUnsavedApp(t)
	restarted := false
	a.rt.restartHost = func() { restarted = true }
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: freeLoopbackPort(t), Confirmed: true})
	if refusal == nil || refusal.Code != appcore.RefusePortUnsaved {
		t.Fatalf("confirmed host.setPort with unsaved edits = %+v, want %s", refusal, appcore.RefusePortUnsaved)
	}
	if restarted {
		t.Fatal("Restart anyway restarted over unsaved edits")
	}
}

func TestSetPortDuringAnUpdateRestartLeavesSavingOpen(t *testing.T) {
	a := newTestAppWithConfigDir(t, t.TempDir(), t.TempDir())
	a.rt.restartHost = func() { t.Error("a port change restarted the app during an update restart") }
	a.updating.Store(true)
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: freeLoopbackPort(t), Confirmed: true})
	// Still running, not the folder-worded not-allowed-now: the person sees
	// that a restart is under way and to try again in a moment.
	if refusal == nil || refusal.Code != appcore.RefuseStillRunning {
		t.Fatalf("confirmed host.setPort during an update restart = %+v, want %s", refusal, appcore.RefuseStillRunning)
	}
	if !a.admitWrite() {
		t.Fatal("a refused port change left every save refused")
	}
	a.rt.writesInFlight.Add(-1)
}

func TestALateSaveDuringAPortRestartIsToldItIsAPortRestart(t *testing.T) {
	a := newTestAppWithConfigDir(t, t.TempDir(), t.TempDir())
	a.rt.syncQuiet = func(time.Duration) bool { return true }
	a.rt.restartHost = func() {}
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: freeLoopbackPort(t), Confirmed: true}); refusal != nil {
		t.Fatalf("confirmed host.setPort = %+v", refusal)
	}
	rec := httptest.NewRecorder()
	a.countWrites(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a save reached the document after the port restart closed writes")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/page.htmlclay", nil))
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable || body.Code != "restarting" || body.Msg != appcore.TextSaveRefusedPortRestart {
		t.Fatalf("late save = %d %+v, want 503 restarting with the port restart sentence", rec.Code, body)
	}
}

// The browser's action request is itself a write countWrites admitted, so a port
// change it asks for must not wait for its own request to finish.
func TestBrowserPortChangeRestartsThroughTheWriteCounter(t *testing.T) {
	a := buildTestApp(t, t.TempDir(), t.TempDir())
	a.rt.port = servedTestPort
	restarted := make(chan struct{}, 1)
	a.rt.restartHost = func() { restarted <- struct{}{} }
	c := openDashboardOver(t, a, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	port := freeLoopbackPort(t)
	start := time.Now()
	if status, body := c.act("p1", "host.setPort", map[string]any{"port": port}); status != http.StatusOK {
		t.Fatalf("host.setPort from the browser = %d after %v: %s", status, time.Since(start).Round(100*time.Millisecond), body)
	}
	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("the browser's port change never restarted")
	}
	if got := a.rt.cfg.Port(); got != port {
		t.Fatalf("config port = %d, want %d", got, port)
	}
}
