package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/secret"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// canaryHex is the canary key's 64 hex characters: a hit on them finds the key
// with or without its hcsk_ prefix.
const canaryHex = "c0ffee11c0ffee22c0ffee33c0ffee44c0ffee55c0ffee66c0ffee77c0ffee88"

// TestM4KeyCanary connects with a canary key through the shell API's action
// route and the real service, restarts the service, and disconnects; then it
// searches every place 08-sync.md step 8.0f found a key could leak: the config
// folder (the service's data dir is inside it), the app's log file, which also
// carries the service's stdout and stderr, every action reply, and every
// snapshot the event stream sent. The only allowed hit is the file store's own
// file, and only while connected.
func TestM4KeyCanary(t *testing.T) {
	serviceRepoTarget(t)
	key := "hcsk_" + canaryHex

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_/sync/accounts" || r.Header.Get("X-API-Key") != key {
			http.Error(w, `{"msg":"no"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"protocol":2,"features":{"accountScopes":true,"accountEvents":true,"conditionalContent":true,"conditionalStructure":true,"completeInventory":true},`+
			`"actor":{"id":12,"username":"maya","person":{"id":"a1","name":"Maya Lin"}},"accounts":[{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}}]}`)
	}))
	t.Cleanup(fake.Close)
	setSyncServer(t, fake.URL)

	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	a := newServiceTestApp(t, home, t.TempDir())
	logPath := filepath.Join(t.TempDir(), "htmlclay.log")
	logger, err := logging.New(logPath)
	if err != nil {
		t.Fatal(err)
	}
	// Closed before its folder is removed: Windows cannot delete a file that is still open.
	t.Cleanup(logger.Close)
	a.rt.logger = logger
	a.rt.syncStore = secret.NewFile(filepath.Join(a.rt.configDir, "secrets"))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	const shellSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	done := make(chan struct{})
	srv := httptest.NewUnstartedServer(shellapi.New(shellBackend{a}, shellSecret, ln.Addr().(*net.TCPAddr).Port, done))
	srv.Listener = ln
	srv.Start()
	t.Cleanup(func() { close(done); srv.Close() })

	var streamMu sync.Mutex
	var stream bytes.Buffer
	events, err := http.NewRequest(http.MethodGet, srv.URL+"/shell/v1/events?since=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	events.Header.Set("Authorization", "Bearer "+shellSecret)
	eventsResp, err := srv.Client().Do(events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eventsResp.Body.Close() })
	go func() {
		scanner := bufio.NewScanner(eventsResp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
		for scanner.Scan() {
			streamMu.Lock()
			stream.WriteString(scanner.Text() + "\n")
			streamMu.Unlock()
		}
	}()

	var replies bytes.Buffer
	act := func(id, typ string, args any) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"id": id, "type": typ, "args": args})
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/shell/v1/actions", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+shellSecret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		replies.Write(body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d %s", typ, resp.StatusCode, body)
		}
	}

	act("serve", "folder.serve", map[string]string{"path": dir, "name": "notes"})
	waitForServiceState(t, a, appcore.ServiceRunning, 30*time.Second)
	act("connect", "sync.connect", map[string]string{"key": key})
	if !a.snapshot().Sync.Connected {
		t.Fatal("connect did not connect")
	}
	// service.restart only restarts a service that failed, so the restart here
	// is an exit, which the supervisor answers with a new process.
	before := a.service.process()
	before.Kill()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if p := a.service.process(); p != nil && p != before && a.service.state() == appcore.ServiceRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the service did not restart")
		}
		time.Sleep(50 * time.Millisecond)
	}
	storeFile := filepath.Join(a.rt.configDir, "secrets", syncKeyName)
	search := func(when string, allowed map[string]bool) {
		t.Helper()
		var hits []string
		_ = filepath.WalkDir(a.rt.configDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil && bytes.Contains(data, []byte(canaryHex)) && !allowed[path] {
				hits = append(hits, path)
			}
			return nil
		})
		if data, err := os.ReadFile(logPath); err != nil {
			t.Fatal(err)
		} else if bytes.Contains(data, []byte(canaryHex)) {
			hits = append(hits, "the log file "+logPath)
		}
		if strings.Contains(replies.String(), canaryHex) {
			hits = append(hits, "an action reply")
		}
		streamMu.Lock()
		if strings.Contains(stream.String(), canaryHex) {
			hits = append(hits, "a snapshot on the event stream")
		}
		streamMu.Unlock()
		if strings.Contains(mustJSON(t, a.snapshot()), canaryHex) {
			hits = append(hits, "the snapshot")
		}
		if len(hits) > 0 {
			t.Fatalf("%s, the key was found in: %s", when, strings.Join(hits, ", "))
		}
	}
	search("connected, after a restart", map[string]bool{storeFile: true})
	if data, err := os.ReadFile(storeFile); err != nil || string(data) != key {
		t.Fatalf("the file store does not hold the key: %v", err)
	}

	act("disconnect", "sync.disconnect", nil)
	time.Sleep(200 * time.Millisecond)
	search("after disconnect", nil)
	if _, err := os.Stat(storeFile); !os.IsNotExist(err) {
		t.Fatalf("the store's file is still there: %v", err)
	}
	streamMu.Lock()
	streamed := stream.Len()
	streamMu.Unlock()
	if streamed == 0 {
		t.Fatal("the event stream carried nothing; the search of it proves nothing")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
