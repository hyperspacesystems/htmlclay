package recalc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The origin lock is the only thing keeping the hidden page from reaching another site: it must
// match the whole Host, not a prefix, and it must refuse the live-sync stream so a hidden page
// never subscribes to SSE.
func TestOriginAllowed(t *testing.T) {
	const host = "htmlclay1.localhost:1212"
	allowed := []string{
		"http://htmlclay1.localhost:1212/doc.htmlclay",
		"http://htmlclay1.localhost:1212/_/clayjs/v1/clay.js",
		"http://htmlclay1.localhost:1212/_/meta/tok",
		"http://htmlclay1.localhost:1212/_/save/tok",
	}
	for _, u := range allowed {
		if ok, suppress := originAllowed(u, host); !ok || suppress {
			t.Errorf("originAllowed(%q) = (%v, %v), want (true, false)", u, ok, suppress)
		}
	}
	denied := []string{
		"http://htmlclay1.localhost:1212.evil.com/",
		"http://127.0.0.1:1212/",
		"http://other.localhost:1212/",
		"https://htmlclay1.localhost:1212/",
		"http://htmlclay1.localhost:9999/",
		"http://htmlclay1.localhost:1212/_/reveal/tok",
		"http://htmlclay1.localhost:1212/_/export/tok",
		"http://htmlclay1.localhost:1212/_/versions-folder/tok",
		"http://htmlclay1.localhost:1212/_/trust-request/tok",
		"http://htmlclay1.localhost:1212/_/helper/tok",
		"data:text/plain,hi",
		"about:blank",
		"",
	}
	for _, u := range denied {
		if ok, _ := originAllowed(u, host); ok {
			t.Errorf("originAllowed(%q) = true, want false", u)
		}
	}
	if ok, suppress := originAllowed("http://htmlclay1.localhost:1212/_/sync/doc", host); ok || !suppress {
		t.Errorf("the live-sync stream = (%v, %v), want (false, true)", ok, suppress)
	}
	if ok, _ := originAllowed("http://htmlclay1.localhost:1212/doc.htmlclay", ""); ok {
		t.Error("an empty allowed host must allow nothing")
	}
}

// HTMLCLAY_RECALC_BROWSER=none must disable the hidden browser, and a path must be honored only
// when it names an executable file.
func TestFindBrowserEnvOverride(t *testing.T) {
	t.Setenv("HTMLCLAY_RECALC_BROWSER", "none")
	if got := FindBrowser(); got != "" {
		t.Errorf("HTMLCLAY_RECALC_BROWSER=none gave %q, want \"\"", got)
	}

	t.Setenv("HTMLCLAY_RECALC_BROWSER", filepath.Join(t.TempDir(), "missing"))
	if got := FindBrowser(); got != "" {
		t.Errorf("a missing path gave %q, want \"\"", got)
	}

	exe := filepath.Join(t.TempDir(), "browser")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTMLCLAY_RECALC_BROWSER", exe)
	if got := FindBrowser(); got != exe {
		t.Errorf("FindBrowser() = %q, want %q", got, exe)
	}
}

// A crashed browser must wake every in-flight call instead of leaving it to its deadline, and
// replies and events must round-trip NUL-delimited JSON.
func TestCDPPipeFramingAndEOF(t *testing.T) {
	connRead, browserWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	browserRead, connWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var eventsMu sync.Mutex
	var events []cdpMsg
	eventsCh := make(chan cdpMsg, 4)
	conn := newCDPConn(connRead, connWrite, func(m cdpMsg) {
		eventsMu.Lock()
		events = append(events, m)
		eventsMu.Unlock()
		eventsCh <- m
	})
	t.Cleanup(func() { _ = connWrite.Close() })

	go func() {
		br := bufio.NewReader(browserRead)
		for {
			raw, err := br.ReadBytes(0)
			if len(raw) > 0 {
				var req map[string]any
				if json.Unmarshal(raw[:len(raw)-1], &req) == nil {
					id, _ := req["id"].(float64)
					method, _ := req["method"].(string)
					if method == "Test.hang" {
						// Never answer: the caller must be woken by the pipe closing.
						continue
					}
					if method == "Test.event" {
						evt, _ := json.Marshal(map[string]any{"method": "Test.fired", "params": map[string]any{"n": 1}})
						_, _ = browserWrite.Write(append(evt, 0))
					}
					reply, _ := json.Marshal(map[string]any{"id": int64(id), "result": map[string]any{"echo": "hi"}})
					_, _ = browserWrite.Write(append(reply, 0))
				}
			}
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := conn.call(ctx, "", "Test.echo", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var out struct {
		Echo string `json:"echo"`
	}
	if err := json.Unmarshal(res, &out); err != nil || out.Echo != "hi" {
		t.Fatalf("result = %s (%v), want the echo", res, err)
	}

	if _, err := conn.call(ctx, "", "Test.event", nil); err != nil {
		t.Fatalf("event call: %v", err)
	}
	select {
	case m := <-eventsCh:
		if m.Method != "Test.fired" {
			t.Errorf("event = %q, want Test.fired", m.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event was delivered")
	}

	hung := make(chan error, 1)
	go func() {
		_, err := conn.call(context.Background(), "", "Test.hang", nil)
		hung <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = browserWrite.Close()
	select {
	case err := <-hung:
		if !errors.Is(err, errPipeClosed) {
			t.Errorf("call on a closed pipe = %v, want errPipeClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a crashed browser left the call hanging")
	}
}

// A popup target created with an openerId must be closed immediately by Target.closeTarget and
// counted in Stats().Popups.
func TestPopupTargetClosed(t *testing.T) {
	connRead, browserWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	browserRead, connWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = connWrite.Close()
		_ = browserWrite.Close()
		_ = browserRead.Close()
	})

	r := New()
	sess := &browserSession{loadCh: make(chan struct{}, 1)}
	sess.cdp = newCDPConn(connRead, connWrite, func(m cdpMsg) {
		r.handleEvent(sess, m)
	})

	evt, _ := json.Marshal(map[string]any{
		"method": "Target.targetCreated",
		"params": map[string]any{
			"targetInfo": map[string]any{
				"targetId": "popup-1",
				"type":     "page",
				"openerId": "main-1",
			},
		},
	})
	if _, err := browserWrite.Write(append(evt, 0)); err != nil {
		t.Fatal(err)
	}

	gotCh := make(chan map[string]any, 1)
	go func() {
		raw, err := bufio.NewReader(browserRead).ReadBytes(0)
		if err != nil || len(raw) == 0 {
			return
		}
		var req map[string]any
		if json.Unmarshal(raw[:len(raw)-1], &req) == nil {
			gotCh <- req
		}
	}()

	select {
	case req := <-gotCh:
		if req["method"] != "Target.closeTarget" {
			t.Fatalf("method = %v, want Target.closeTarget", req["method"])
		}
		params, _ := req["params"].(map[string]any)
		if params["targetId"] != "popup-1" {
			t.Fatalf("targetId = %v, want popup-1", params["targetId"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Target.closeTarget was not sent for popup target")
	}
	if got := r.Stats().Popups; got != 1 {
		t.Errorf("Stats().Popups = %d, want 1", got)
	}
}
