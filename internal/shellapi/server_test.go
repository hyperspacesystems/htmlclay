package shellapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// testSecret is a bootstrap-shaped secret: 64 lowercase hex characters.
const testSecret = "1f0c8d3a5b7e9d2c4a6f8b0d1e3c5a7f9b2d4c6e8a0f1b3d5c7e9a2b4d6f8c0e"

type fakeAnswer struct {
	Snapshot appcore.Snapshot
	Refusal  *appcore.Refusal
}

// fakeBackend is the app the listener talks to in these tests: a revision that
// counts up, a map of remembered answers, and a channel of snapshots to stream.
type fakeBackend struct {
	mu      sync.Mutex
	rev     uint64
	answers map[string]fakeAnswer
	updates chan appcore.Snapshot
}

func (f *fakeBackend) snapshotLocked() appcore.Snapshot {
	return appcore.Snapshot{
		V:     appcore.SnapshotVersion,
		Rev:   f.rev,
		Epoch: "epoch-1",
		Host:  appcore.HostState{Version: "1.16.2", OS: "darwin", Port: 24190},
	}
}

func (f *fakeBackend) Snapshot() appcore.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshotLocked()
}

func (f *fakeBackend) Do(_ context.Context, act appcore.Action) (appcore.Snapshot, *appcore.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rev++
	snapshot := f.snapshotLocked()
	var refusal *appcore.Refusal
	if act.Type == "folder.trust" {
		refusal = &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: "t1"}
	}
	f.answers[act.ID] = fakeAnswer{Snapshot: snapshot, Refusal: refusal}
	return snapshot, refusal
}

func (f *fakeBackend) Check(act appcore.Action) (Preview, *appcore.Refusal) {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(act.Args, &args)
	if args.Path == "/refused" {
		return Preview{}, &appcore.Refusal{Code: appcore.RefuseOutsideHome}
	}
	return Preview{Name: "blog", Address: "blog.htmlclay.localhost:24190"}, nil
}

func (f *fakeBackend) Answer(id string) (appcore.Snapshot, *appcore.Refusal, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	answer, ok := f.answers[id]
	return answer.Snapshot, answer.Refusal, ok
}

func (f *fakeBackend) Subscribe() (<-chan appcore.Snapshot, func()) { return f.updates, func() {} }

func (f *fakeBackend) push(s appcore.Snapshot) { f.updates <- s }

// shellTest is one listener bound to a real loopback port, so the Host check is
// exercised exactly as the Mac app's client would meet it.
type shellTest struct {
	fake   *fakeBackend
	srv    *httptest.Server
	port   int
	secret string
	done   chan struct{}
	once   sync.Once
}

func newShellTest(t *testing.T) *shellTest {
	t.Helper()
	fake := &fakeBackend{rev: 1, answers: map[string]fakeAnswer{}, updates: make(chan appcore.Snapshot, 8)}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	h := &shellTest{fake: fake, port: port, secret: testSecret, done: make(chan struct{})}
	handler := New(fake, h.secret, port, h.done)
	// Shortened from the 15 second default so a test waits a moment, not a minute.
	handler.ping = 200 * time.Millisecond
	h.srv = httptest.NewUnstartedServer(handler)
	h.srv.Listener = ln
	h.srv.Start()
	t.Cleanup(func() {
		h.closeDone()
		h.srv.Close()
	})
	return h
}

func (h *shellTest) closeDone() { h.once.Do(func() { close(h.done) }) }

// request sends one request as the Mac app's client does: no browser headers, the
// secret in the Authorization header, and Host taken from the URL.
func (h *shellTest) request(t *testing.T, method, path, body string, edit func(*http.Request)) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.secret)
	if edit != nil {
		edit(req)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestShellRefusesAStranger(t *testing.T) {
	h := newShellTest(t)
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/shell/v1/snapshot", ""},
		{http.MethodPost, "/shell/v1/actions", `{"id":"a1","type":"prefs.set"}`},
		{http.MethodGet, "/shell/v1/actions/a1", ""},
		{http.MethodPost, "/shell/v1/check", `{"id":"c1","type":"folder.serve","args":{"path":"/x"}}`},
		{http.MethodGet, "/shell/v1/events?since=0", ""},
	}
	badSecrets := map[string]string{
		"missing": "",
		"wrong":   strings.Repeat("ab", 32),
		"longer":  h.secret + "0",
	}
	for name, secret := range badSecrets {
		for _, route := range routes {
			t.Run(name+" "+route.method+" "+route.path, func(t *testing.T) {
				resp := h.request(t, route.method, route.path, route.body, func(r *http.Request) {
					if secret == "" {
						r.Header.Del("Authorization")
						return
					}
					r.Header.Set("Authorization", "Bearer "+secret)
				})
				if resp.StatusCode != http.StatusUnauthorized {
					t.Fatalf("%s %s with %s secret = %d, want 401", route.method, route.path, name, resp.StatusCode)
				}
			})
		}
	}
}

func TestShellRefusesTheWrongHost(t *testing.T) {
	h := newShellTest(t)
	hosts := map[string]string{
		"localhost":  "localhost:" + strconv.Itoa(h.port),
		"other port": "127.0.0.1:" + strconv.Itoa(h.port+1),
	}
	for name, host := range hosts {
		t.Run(name, func(t *testing.T) {
			resp := h.request(t, http.MethodGet, "/shell/v1/snapshot", "", func(r *http.Request) {
				r.Host = host
			})
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("GET snapshot with Host %s = %d, want 403", host, resp.StatusCode)
			}
		})
	}
}

func TestShellRefusesBrowserHeaders(t *testing.T) {
	h := newShellTest(t)
	headers := map[string]string{
		"Origin":         "http://evil.example",
		"Sec-Fetch-Site": "cross-site",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Dest": "empty",
		"Cookie":         "session=1",
	}
	for name, value := range headers {
		t.Run(name, func(t *testing.T) {
			resp := h.request(t, http.MethodGet, "/shell/v1/snapshot", "", func(r *http.Request) {
				r.Header.Set(name, value)
			})
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("GET snapshot with %s = %d, want 403", name, resp.StatusCode)
			}
		})
	}
}

func TestShellSnapshot(t *testing.T) {
	h := newShellTest(t)
	resp := h.request(t, http.MethodGet, "/shell/v1/snapshot", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET snapshot = %d, want 200", resp.StatusCode)
	}
	var got appcore.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if want := h.fake.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %+v, want %+v", got, want)
	}
}

func TestShellActions(t *testing.T) {
	h := newShellTest(t)

	t.Run("answer", func(t *testing.T) {
		resp := h.request(t, http.MethodPost, "/shell/v1/actions", `{"id":"a1","type":"prefs.set"}`, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST actions = %d, want 200", resp.StatusCode)
		}
		var body struct {
			OK       bool             `json:"ok"`
			Rev      uint64           `json:"rev"`
			Snapshot appcore.Snapshot `json:"snapshot"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode answer: %v", err)
		}
		if !body.OK || body.Rev == 0 || body.Snapshot.Rev != body.Rev {
			t.Fatalf("answer = %+v, want ok with a revision and its snapshot", body)
		}
	})

	t.Run("refusal", func(t *testing.T) {
		resp := h.request(t, http.MethodPost, "/shell/v1/actions", `{"id":"a2","type":"folder.trust"}`, nil)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("POST refused action = %d, want 409", resp.StatusCode)
		}
		assertRefusal(t, resp.Body)
	})

	t.Run("bad bodies", func(t *testing.T) {
		bodies := map[string]string{
			"unknown field": `{"id":"a3","type":"prefs.set","nope":1}`,
			"no id":         `{"type":"prefs.set"}`,
			"long id":       `{"id":"` + strings.Repeat("x", 101) + `","type":"prefs.set"}`,
		}
		for name, body := range bodies {
			resp := h.request(t, http.MethodPost, "/shell/v1/actions", body, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("POST actions with %s = %d, want 400", name, resp.StatusCode)
			}
		}
	})
}

// TestErrorBodyDocuments proves a refusal names the documents it is about, so the
// Mac app can show a person which edits a quit would lose.
func TestErrorBodyDocuments(t *testing.T) {
	body := errorBody(&appcore.Refusal{Code: appcore.RefuseQuitUnsaved, Documents: []string{"notes/a.html"}})
	if !reflect.DeepEqual(body["documents"], []string{"notes/a.html"}) {
		t.Fatalf("errorBody documents = %v, want [notes/a.html]", body["documents"])
	}
	if _, ok := errorBody(&appcore.Refusal{Code: appcore.RefuseQuitUnsaved})["documents"]; ok {
		t.Fatal("a refusal without documents carried a documents key")
	}
}

func assertRefusal(t *testing.T, body io.Reader) {
	t.Helper()
	var decoded struct {
		OK    bool `json:"ok"`
		Error struct {
			Code      string `json:"code"`
			Text      string `json:"text"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&decoded); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if decoded.OK || decoded.Error.Code == "" || decoded.Error.Text == "" {
		t.Fatalf("refusal = %+v, want ok false with a code and a sentence", decoded)
	}
}

func TestShellLookup(t *testing.T) {
	h := newShellTest(t)
	if resp := h.request(t, http.MethodPost, "/shell/v1/actions", `{"id":"a1","type":"prefs.set"}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST actions = %d, want 200", resp.StatusCode)
	}

	resp := h.request(t, http.MethodGet, "/shell/v1/actions/a1", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET a remembered id = %d, want 200", resp.StatusCode)
	}
	var body struct {
		OK       bool             `json:"ok"`
		Snapshot appcore.Snapshot `json:"snapshot"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if !body.OK || body.Snapshot.Rev == 0 {
		t.Fatalf("lookup = %+v, want the remembered answer", body)
	}

	unknown := h.request(t, http.MethodGet, "/shell/v1/actions/nobody", "", nil)
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("GET an unknown id = %d, want 404", unknown.StatusCode)
	}
}

func TestShellCheck(t *testing.T) {
	h := newShellTest(t)

	refused := h.request(t, http.MethodPost, "/shell/v1/check", `{"id":"c1","type":"folder.serve","args":{"path":"/refused"}}`, nil)
	if refused.StatusCode != http.StatusConflict {
		t.Fatalf("POST check of a refused folder = %d, want 409", refused.StatusCode)
	}
	assertRefusal(t, refused.Body)

	preview := h.request(t, http.MethodPost, "/shell/v1/check", `{"id":"c2","type":"folder.serve","args":{"path":"/blog","name":"blog"}}`, nil)
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("POST check = %d, want 200", preview.StatusCode)
	}
	var body struct {
		OK      bool    `json:"ok"`
		Preview Preview `json:"preview"`
	}
	if err := json.NewDecoder(preview.Body).Decode(&body); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if !body.OK || body.Preview.Address == "" {
		t.Fatalf("preview = %+v, want ok with an address", body)
	}
}

// eventLines reads the stream's lines so a test can wait for one with a deadline.
func eventLines(t *testing.T, body io.Reader) <-chan string {
	t.Helper()
	lines := make(chan string, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return lines
}

func nextEvent(t *testing.T, lines <-chan string) map[string]any {
	t.Helper()
	select {
	case line, ok := <-lines:
		if !ok {
			t.Fatal("the event stream ended")
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
	}
	return nil
}

func TestShellEvents(t *testing.T) {
	h := newShellTest(t)
	resp := h.request(t, http.MethodGet, "/shell/v1/events?since=0", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET events = %d, want 200", resp.StatusCode)
	}
	lines := eventLines(t, resp.Body)

	first := nextEvent(t, lines)
	if first["type"] != "snapshot" || first["rev"] != float64(1) {
		t.Fatalf("first event = %+v, want the current snapshot", first)
	}

	h.fake.push(appcore.Snapshot{V: appcore.SnapshotVersion, Rev: 2, Epoch: "epoch-1"})
	changed := nextEvent(t, lines)
	if changed["type"] != "snapshot" || changed["rev"] != float64(2) {
		t.Fatalf("second event = %+v, want the pushed snapshot", changed)
	}

	ping := nextEvent(t, lines)
	if ping["type"] != "ping" {
		t.Fatalf("third event = %+v, want a ping", ping)
	}

	h.closeDone()
	bye := nextEvent(t, lines)
	if bye["type"] != "bye" || bye["reason"] != "quit" {
		t.Fatalf("last event = %+v, want a bye for quit", bye)
	}
	if _, ok := <-lines; ok {
		t.Fatal("the stream did not end after the bye line")
	}
}
