package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
)

// shortHelperParkLimit shortens how long a parked request waits, and puts it
// back before the rest of the package runs.
func shortHelperParkLimit(t *testing.T, limit time.Duration) {
	t.Helper()
	previous := helperParkLimit
	helperParkLimit = limit
	t.Cleanup(func() { helperParkLimit = previous })
}

func postHelperSetup(t *testing.T, srv *Server, token, name string, stamp func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/_/helper-setup/"+token, strings.NewReader(fmt.Sprintf(`{"name":%q}`, name)))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.Header.Set("Content-Type", "application/json")
	if stamp != nil {
		stamp(req)
	} else {
		sameOriginHeaders(req)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

func helperSetupState(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode setup answer %q: %v", w.Body.String(), err)
	}
	return out.State
}

func getServedDocument(t *testing.T, srv *Server, pageURL string, headers map[string]string) string {
	t.Helper()
	req := httptest.NewRequest("GET", pageURL, nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200: %s", pageURL, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestParkedRequestExpires(t *testing.T) {
	shortHelperParkLimit(t, 200*time.Millisecond)
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	structuredHelper(t, "result-null")
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool {
		calls.Add(1)
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.wire.publish(file.AbsPath, wireEnvelope{
		V: 1, Type: "wire/request", ID: "parked", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "parked")

	deadline := time.After(time.Second)
	for {
		select {
		case raw := <-observer.ch:
			env, err := decodeWireEnvelope(raw)
			if err != nil {
				t.Fatal(err)
			}
			if env.Type != "wire/error" || env.ID != "parked" {
				continue
			}
			if code := helperErrorCode(t, env); code != "helper_setup_expired" {
				t.Fatalf("terminal code = %q, want helper_setup_expired", code)
			}
			if calls.Load() != 0 {
				t.Fatalf("AskHelper calls = %d, want none without setup", calls.Load())
			}
			return
		case <-deadline:
			t.Fatal("the parked request was never expired")
		}
	}
}

func TestLateApprovalNeverRunsAnExpiredRequest(t *testing.T) {
	shortHelperParkLimit(t, 200*time.Millisecond)
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	countPath := filepath.Join(t.TempDir(), "count")
	t.Setenv(serverHelperArg, countPath)
	program := helperProgram("search", structuredHelper(t, "count"))
	approved := make(chan *wireSub, 1)
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		time.Sleep(400 * time.Millisecond)
		if err := srv.AttachHelpers(path, map[string]config.HelperProgram{name: program}, nil); err != nil {
			t.Errorf("reattach after approval: %v", err)
			return false
		}
		sub := &wireSub{key: path, ch: make(chan []byte, 64), done: make(chan struct{})}
		if _, _, err := srv.wire.add(sub, 0); err != nil {
			t.Errorf("observe after approval: %v", err)
			return false
		}
		approved <- sub
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.wire.publish(file.AbsPath, wireEnvelope{
		V: 1, Type: "wire/request", ID: "parked", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "parked")

	if granted, ran := srv.setUpHelper(file.AbsPath, "search"); !granted || !ran {
		t.Fatalf("setup = granted %v, ran %v, want both true", granted, ran)
	}
	env := receiveHelperType(t, observer, "wire/error", "parked")
	if code := helperErrorCode(t, env); code != "helper_setup_expired" {
		t.Fatalf("terminal code = %q, want helper_setup_expired", code)
	}
	if _, err := os.Stat(countPath); err == nil {
		t.Fatal("a request that expired while parked ran the helper program")
	}

	var late *wireSub
	select {
	case late = <-approved:
	case <-time.After(2 * time.Second):
		t.Fatal("no wire subscription after the approval reattached the helpers")
	}
	t.Cleanup(func() { srv.wire.remove(late) })

	srv.wire.publish(file.AbsPath, wireEnvelope{
		V: 1, Type: "wire/request", ID: "after", File: file.AbsPath,
		Helper: "search", Document: "none", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, late, "wire/ack", "after")
	receiveHelperType(t, late, "wire/done", "after")
	count, err := os.ReadFile(countPath)
	if err != nil || strings.TrimSpace(string(count)) != "1" {
		t.Fatalf("count = %q, %v; want the program to have run exactly once", count, err)
	}
}

func TestHelperSetupEndpointStates(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	writeHelperDocument(t, file.AbsPath, "search", "ocr")
	exe := structuredHelper(t, "result-null")
	program := helperProgram("search", exe)
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		if name != "search" {
			return false
		}
		if err := srv.AttachHelpers(path, map[string]config.HelperProgram{name: program}, nil); err != nil {
			t.Errorf("reattach after approval: %v", err)
			return false
		}
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })

	if state := helperSetupState(t, postHelperSetup(t, srv, file.Token, "missing", nil)); state != "undeclared" {
		t.Fatalf("state for an undeclared name = %q, want undeclared", state)
	}
	if w := postHelperSetup(t, srv, file.Token, "Not A Name", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid name = %d, want 400: %s", w.Code, w.Body.String())
	}
	if state := helperSetupState(t, postHelperSetup(t, srv, file.Token, "search", nil)); state != "ready" {
		t.Fatalf("state for a granted name = %q, want ready", state)
	}
	if state := helperSetupState(t, postHelperSetup(t, srv, file.Token, "ocr", nil)); state != "denied" {
		t.Fatalf("state for a refused name = %q, want denied", state)
	}
	if state := helperSetupState(t, postHelperSetup(t, srv, file.Token, "ocr", nil)); state != "retry" {
		t.Fatalf("state for a name refused a moment ago = %q, want retry", state)
	}
	if w := postHelperSetup(t, srv, "bad-token", "search", nil); w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("wrong token = %d, want 401 or 403: %s", w.Code, w.Body.String())
	}
	noAttestation := func(req *http.Request) { req.Header.Set("Sec-Fetch-Site", "cross-site") }
	if w := postHelperSetup(t, srv, file.Token, "search", noAttestation); w.Code != http.StatusForbidden {
		t.Fatalf("cross-site setup = %d, want 403: %s", w.Code, w.Body.String())
	}
}

func TestHelperSetupEndpointRunsOneDialogAtATime(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	program := helperProgram("search", structuredHelper(t, "result-null"))
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		if err := srv.AttachHelpers(path, map[string]config.HelperProgram{name: program}, nil); err != nil {
			t.Errorf("reattach after approval: %v", err)
			return false
		}
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })

	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- postHelperSetup(t, srv, file.Token, "search", nil) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the setup dialog was never raised")
	}
	if state := helperSetupState(t, postHelperSetup(t, srv, file.Token, "search", nil)); state != "busy" {
		t.Fatalf("second setup while one runs = %q, want busy", state)
	}
	close(release)
	select {
	case w := <-first:
		if state := helperSetupState(t, w); state != "ready" {
			t.Fatalf("first setup = %q, want ready", state)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the first setup never answered")
	}
	if calls.Load() != 1 {
		t.Fatalf("AskHelper calls = %d, want one dialog for two requests", calls.Load())
	}
}

func TestSetupBarInjection(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	page := `<!doctype html><html><head><meta name="htmlclay-helper" content="search"></head><body>hi</body></html>`
	if err := os.WriteFile(file.AbsPath, []byte(page), 0644); err != nil {
		t.Fatal(err)
	}
	exe := structuredHelper(t, "result-null")
	program := helperProgram("search", exe)
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })

	navigation := map[string]string{"Sec-Fetch-Dest": "document"}
	if bare := getServedDocument(t, srv, "/page.htmlclay", navigation); strings.Contains(bare, "data-htmlclay-setup") {
		t.Fatalf("a host that cannot show the setup dialog served the setup bar: %s", bare)
	}
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool { return false }})
	body := getServedDocument(t, srv, "/page.htmlclay", navigation)
	for _, want := range []string{"data-htmlclay-setup", "called “search”", "<!--htmlclay-banner-->"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the served document is missing %q: %s", want, body)
		}
	}
	for _, path := range []string{exe, filepath.Dir(exe), srv.sessions.HomeDir()} {
		if strings.Contains(body, path) {
			t.Fatalf("the setup bar leaked the filesystem path %q: %s", path, body)
		}
	}

	if hidden := getServedDocument(t, srv, "/page.htmlclay", map[string]string{"Sec-Fetch-Dest": "empty"}); strings.Contains(hidden, "data-htmlclay-setup") {
		t.Fatalf("a silent fetch got the setup bar: %s", hidden)
	}
	source := map[string]string{"Sec-Fetch-Dest": "document", "Clay-Source": "1"}
	if raw := getServedDocument(t, srv, "/page.htmlclay", source); strings.Contains(raw, "data-htmlclay-setup") {
		t.Fatalf("a source read got the setup bar: %s", raw)
	}

	save := httptest.NewRequest("POST", "/_/save/"+file.Token, strings.NewReader(body))
	save.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	sameOriginHeaders(save)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, save)
	if w.Code != http.StatusOK {
		t.Fatalf("saving the served bytes = %d, want 200: %s", w.Code, w.Body.String())
	}
	saved, err := os.ReadFile(file.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "data-htmlclay-setup") || strings.Contains(string(saved), "<!--htmlclay-banner-->") {
		t.Fatalf("the setup bar reached the saved document: %s", saved)
	}

	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}
	if allowed := getServedDocument(t, srv, "/page.htmlclay", navigation); strings.Contains(allowed, "data-htmlclay-setup") {
		t.Fatalf("a set-up document still got the setup bar: %s", allowed)
	}
}
