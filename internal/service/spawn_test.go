package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const fakeSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMain(m *testing.M) {
	if mode := os.Getenv("HTMLCLAY_FAKE_SERVICE"); mode != "" {
		fakeService(mode)
		return
	}
	os.Exit(m.Run())
}

// fakeService stands in for service/main.cjs: the test binary runs itself with
// HTMLCLAY_FAKE_SERVICE set and speaks the service's protocol instead of running
// tests.
func fakeService(mode string) {
	in := bufio.NewReader(os.Stdin)
	line, _ := in.ReadString('\n')
	if mode == "badready" {
		fmt.Println("READY not-json")
		os.Exit(0)
	}
	var boot struct {
		V      int    `json:"v"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal([]byte(line), &boot); err != nil || boot.V != 1 || len(boot.Secret) != 64 {
		fmt.Fprintln(os.Stderr, "fake service: bad bootstrap line")
		os.Exit(3)
	}
	switch mode {
	case "silent":
		io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	case "crash":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(3)
	case "ok", "stubborn":
	default:
		fmt.Fprintf(os.Stderr, "fake service: unknown mode %q\n", mode)
		os.Exit(2)
	}
	ready, _ := json.Marshal(Ready{URL: "http://127.0.0.1:1", Epoch: "e1", PID: os.Getpid()})
	fmt.Printf("READY %s\n", ready)
	fmt.Fprintln(os.Stderr, "fake service up")
	if mode == "stubborn" {
		time.Sleep(60 * time.Second)
		os.Exit(0)
	}
	for {
		next, err := in.ReadString('\n')
		if strings.Contains(next, `"host"`) {
			fmt.Println("ATTACHED")
		}
		if err != nil {
			break
		}
	}
	os.Exit(0)
}

// lineLog collects the lines the service writes to stderr and the stdout lines
// nobody was waiting for.
type lineLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *lineLog) add(line string) {
	l.mu.Lock()
	l.lines = append(l.lines, line)
	l.mu.Unlock()
}

func (l *lineLog) has(line string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, got := range l.lines {
		if got == line {
			return true
		}
	}
	return false
}

func (l *lineLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string{}, l.lines...)
}

func fakeSpec(t *testing.T, mode string, log *lineLog) Spec {
	t.Helper()
	spec := Spec{
		Command: []string{os.Args[0]},
		DataDir: t.TempDir(),
		Env:     []string{"HTMLCLAY_FAKE_SERVICE=" + mode},
	}
	if log != nil {
		spec.Log = log.add
	}
	return spec
}

func startFake(t *testing.T, mode string, log *lineLog) (*Process, Ready) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	p, ready, err := Start(ctx, fakeSpec(t, mode, log), fakeSecret, 10*time.Second)
	if err != nil {
		t.Fatalf("start %s: %v", mode, err)
	}
	t.Cleanup(p.Kill)
	return p, ready
}

func waitClosed(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("%s did not happen within %v", what, d)
	}
}

func TestSpawnReadyAttachClose(t *testing.T) {
	log := &lineLog{}
	p, ready := startFake(t, "ok", log)
	if ready.Epoch != "e1" {
		t.Fatalf("epoch %q, want e1", ready.Epoch)
	}
	if ready.PID != p.cmd.Process.Pid {
		t.Fatalf("pid %d, want %d", ready.PID, p.cmd.Process.Pid)
	}
	if !log.has("fake service up") {
		t.Fatalf("log %q, want the stderr line", log.all())
	}
	if err := p.Attach(Host{URL: "http://127.0.0.1:1", Secret: fakeSecret, Epoch: "e1", Roots: nil}, 10*time.Second); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := p.Close(2 * time.Second); err != nil {
		t.Fatalf("close: %v", err)
	}
	waitClosed(t, p.Done(), 2*time.Second, "the exit")
	if code := p.ExitCode(); code != 0 {
		t.Fatalf("exit code %d, want 0; log %q", code, log.all())
	}
}

func TestSpawnReadyTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, _, err := Start(ctx, fakeSpec(t, "silent", nil), fakeSecret, 300*time.Millisecond)
	if err == nil {
		p.Kill()
		t.Fatal("start succeeded, want a timeout")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error %q, want it to name the timeout", err)
	}
	if p == nil {
		t.Fatal("no process returned with the error")
	}
	waitClosed(t, p.Done(), 5*time.Second, "the exit")
}

func TestSpawnCrashBeforeReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, _, err := Start(ctx, fakeSpec(t, "crash", nil), fakeSecret, 10*time.Second)
	if err == nil {
		p.Kill()
		t.Fatal("start succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q, want the stderr line boom", err)
	}
	if !strings.Contains(err.Error(), "exit 3") {
		t.Fatalf("error %q, want the exit code 3", err)
	}
	if p != nil {
		waitClosed(t, p.Done(), 5*time.Second, "the exit")
	}
}

func TestSpawnBadReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, _, err := Start(ctx, fakeSpec(t, "badready", nil), fakeSecret, 10*time.Second)
	if err == nil {
		p.Kill()
		t.Fatal("start succeeded, want a malformed READY error")
	}
	if !strings.Contains(err.Error(), "READY") {
		t.Fatalf("error %q, want it to name READY", err)
	}
	if p != nil {
		waitClosed(t, p.Done(), 5*time.Second, "the exit")
	}
}

func TestSpawnCloseKillsStubborn(t *testing.T) {
	p, _ := startFake(t, "stubborn", nil)
	began := time.Now()
	if err := p.Close(300 * time.Millisecond); err != nil {
		t.Fatalf("close: %v", err)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("close took %v, want it within 2s", took)
	}
	waitClosed(t, p.Done(), 2*time.Second, "the exit")
}

func TestSpawnExitIsSeen(t *testing.T) {
	p, ready := startFake(t, "ok", nil)
	child, err := os.FindProcess(ready.PID)
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	if err := child.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	waitClosed(t, p.Done(), 2*time.Second, "the exit")
}

// stubGo answers the service's control calls: it keeps /v1/events open and
// answers everything else with an empty success.
func stubGo(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/events") {
			w.Header().Set("content-type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
			return
		}
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/v1/unconfirmed" {
			io.WriteString(w, `{"documents":[]}`)
			return
		}
		io.WriteString(w, "{}")
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	return server
}

func TestSpawnRealService(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	if info, err := os.Stat("../../service/node_modules"); err != nil || !info.IsDir() {
		t.Skip("service/node_modules is missing")
	}
	main, err := filepath.Abs("../../service/main.cjs")
	if err != nil {
		t.Fatalf("main.cjs path: %v", err)
	}
	server := stubGo(t)
	log := &lineLog{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	spec := Spec{Command: []string{node, main}, DataDir: t.TempDir(), Log: log.add}
	p, ready, err := Start(ctx, spec, fakeSecret, 20*time.Second)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(p.Kill)
	if ready.URL == "" || ready.Epoch == "" || ready.PID == 0 {
		t.Fatalf("ready %+v, want a url, an epoch and a pid", ready)
	}
	if err := p.Attach(Host{URL: server.URL, Secret: fakeSecret, Epoch: "a1b2c3d4e5f60718"}, 20*time.Second); err != nil {
		t.Fatalf("attach: %v; log %q", err, log.all())
	}
	if err := p.Close(5 * time.Second); err != nil {
		t.Fatalf("close: %v", err)
	}
	waitClosed(t, p.Done(), 5*time.Second, "the exit")
	if code := p.ExitCode(); code != 0 {
		t.Fatalf("exit code %d, want 0; log %q", code, log.all())
	}
}

func TestCommandFromEnv(t *testing.T) {
	t.Setenv("HTMLCLAY_SERVICE_CMD", "")
	if got := CommandFromEnv(); got != nil {
		t.Fatalf("unset: %v, want nil", got)
	}
	t.Setenv("HTMLCLAY_SERVICE_CMD", "node /abs/path/service/main.cjs")
	got := CommandFromEnv()
	want := []string{"node", "/abs/path/service/main.cjs"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("CommandFromEnv() %v, want %v", got, want)
	}
}
