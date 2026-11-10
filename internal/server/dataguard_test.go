package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/versions"
)

const guardTestDoc = `<html><body><h1 class="title">Board</h1>` +
	`<script type="application/json" data-rules-name="api" data-rules-version="1">{title:".title"}</script>` +
	`</body></html>`

func guardTestHistory(t *testing.T, bodies ...string) (*GuardHistory, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v := versions.New(filepath.Join(dir, "versions"))
	doc := filepath.Join(dir, "board.htmlclay")
	if err := os.WriteFile(doc, []byte(bodies[len(bodies)-1]), 0o644); err != nil {
		t.Fatal(err)
	}
	key := versions.Key(doc, nil)
	for _, body := range bodies {
		if _, err := v.Backup(key, doc, []byte(body)); err != nil {
			t.Fatalf("backup: %v", err)
		}
	}
	return NewGuardHistory(v, key, doc), doc
}

// GuardExtract reads the island an api rules tag carries, and fails open on a
// page that has none.
func TestGuardExtractor(t *testing.T) {
	cases := []struct {
		name    string
		html    string
		want    string
		wantErr bool
	}{
		{
			name: "rules tag",
			html: guardTestDoc,
			want: `{"api":{"title":"Board"}}`,
		},
		{
			name: "no rules tag is a nil island",
			html: `<html><body><h1>Board</h1></body></html>`,
			want: `null`,
		},
		{
			name:    "unknown rules version does not parse",
			html:    `<script data-rules-name="api" data-rules-version="2">{t:"h1"}</script>`,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := Extractor{}.Extract([]byte(c.html))
			if c.wantErr {
				if err == nil {
					t.Fatalf("extracted %#v, want an error", v)
				}
				return
			}
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			got := "null"
			if v != nil {
				raw, err := dataapi.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				got = string(raw)
			}
			if got != c.want {
				t.Fatalf("island = %s, want %s", got, c.want)
			}
		})
	}
}

// lossParityPage builds a page whose api rules tag reads a heading and a list of
// items, the shape the shared guard vectors use.
func lossParityPage(title string, items ...string) []byte {
	var b strings.Builder
	b.WriteString(`<html><body><h1 class="title">` + title + `</h1><ul>`)
	for _, item := range items {
		b.WriteString(`<li class="item">` + item + `</li>`)
	}
	b.WriteString(`</ul><script type="application/json" data-rules-name="api" data-rules-version="1">` +
		`{title:".title", items:".item[]"}</script></body></html>`)
	return []byte(b.String())
}

// GuardLossSummaryMatchesJS holds the Go guard to the JS core on the same two
// islands. The expected value is the literal output of
// data-loss-core.cjs: classifyDestruction({api:{title:"T",items:[alpha,beta,gamma,delta]}},
// {api:{title:"T",items:[alpha]}}) then lossSummary("external", D).
func TestGuardLossSummaryMatchesJS(t *testing.T) {
	before, err := Extractor{}.Extract(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	after, err := Extractor{}.Extract(lossParityPage("T", "alpha"))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	raw, err := json.Marshal(dataguard.LossSummaryOf(dataguard.External, dataguard.ClassifyDestruction(before, after)))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"provenance":"external","destroyedAtoms":3,"removedItems":0,"modifiedAtoms":0,"paths":["api.items[1]","api.items[2]","api.items[3]"]}`
	if string(raw) != want {
		t.Fatalf("lossSummary = %s, want %s", raw, want)
	}
}

// GuardCanRestore is the dry run behind the panel's Restore offer: data that
// cannot round-trip is not offered.
func TestGuardCanRestore(t *testing.T) {
	recoverable, err := Extractor{}.Extract([]byte(guardTestDoc))
	if err != nil {
		t.Fatal(err)
	}
	if !(Extractor{}).CanRestore([]byte(guardTestDoc), recoverable) {
		t.Fatal("a document that carries the data cannot be restored from it")
	}
	if (Extractor{}).CanRestore([]byte(guardTestDoc), nil) {
		t.Fatal("no data offered a restore")
	}
	clobbered := []byte(`<html><body><h1 class="title">elsewhere</h1>` +
		`<script type="application/json" data-rules-name="api" data-rules-version="1">{other:".title"}</script></body></html>`)
	if (Extractor{}).CanRestore(clobbered, recoverable) {
		t.Fatal("data for rules the page no longer has offered a restore")
	}
}

// GuardRestore writes the recoverable data onto the current page and refuses
// when the write cannot reproduce it.
func TestGuardRestore(t *testing.T) {
	restorable, err := Extractor{}.Extract([]byte(guardTestDoc))
	if err != nil {
		t.Fatal(err)
	}
	clobberedTitle := []byte(`<html><body><h1 class="title">elsewhere</h1>` +
		`<script type="application/json" data-rules-name="api" data-rules-version="1">{title:".title"}</script>` +
		`</body></html>`)

	t.Run("restores the data onto the page", func(t *testing.T) {
		merged, err := (Extractor{}).Restore(clobberedTitle, restorable)
		if err != nil {
			t.Fatalf("Restore: %v", err)
		}
		if !bytes.Contains(merged, []byte(`class="title">Board<`)) {
			t.Fatalf("restored page does not carry the data:\n%s", merged)
		}
		after, err := Extractor{}.Extract(merged)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if !dataguard.IslandsEqual(after, restorable) {
			t.Fatalf("island after restore = %#v, want %#v", after, restorable)
		}
	})

	t.Run("a value that does not round-trip is refused", func(t *testing.T) {
		// Extraction trims a scalar's text, so data that arrived with outer
		// whitespace cannot be read back the way it went in.
		inner := dataapi.NewObject()
		inner.Define("title", "Board   ")
		padded := dataapi.NewObject()
		padded.Set("api", inner)
		if _, err := (Extractor{}).Restore(clobberedTitle, padded); !errors.Is(err, dataguard.ErrNotRestorable) {
			t.Fatalf("Restore err = %v, want ErrNotRestorable", err)
		}
	})

	t.Run("no data is not restorable", func(t *testing.T) {
		if _, err := (Extractor{}).Restore(clobberedTitle, nil); !errors.Is(err, dataguard.ErrNotRestorable) {
			t.Fatalf("Restore err = %v, want ErrNotRestorable", err)
		}
	})

	t.Run("an emptied list has no row to clone", func(t *testing.T) {
		emptied := []byte(`<html><body><ul></ul>` +
			`<script type="application/json" data-rules-name="api" data-rules-version="1">{items:[".item",{name:".name"}]}</script>` +
			`</body></html>`)
		full := []byte(`<html><body><ul><li class="item"><span class="name">alpha</span></li></ul>` +
			`<script type="application/json" data-rules-name="api" data-rules-version="1">{items:[".item",{name:".name"}]}</script>` +
			`</body></html>`)
		recoverable, err := Extractor{}.Extract(full)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (Extractor{}).Restore(emptied, recoverable); !errors.Is(err, dataguard.ErrNotRestorable) {
			t.Fatalf("Restore err = %v, want ErrNotRestorable", err)
		}
		if (Extractor{}).CanRestore(emptied, recoverable) {
			t.Fatal("a page with no row to clone offered a restore")
		}
	})
}

// GuardHistory must hand back the newest version that is NOT the bytes just
// written, because the watcher backs those bytes up before the guard runs.
func TestGuardHistoryNewestDifferingFrom(t *testing.T) {
	older := guardTestDoc
	newer := `<html><body><p>clobbered</p>` +
		`<script type="application/json" data-rules-name="api" data-rules-version="1">{title:".title"}</script></body></html>`

	t.Run("skips the newest version when it is the new bytes", func(t *testing.T) {
		h, _ := guardTestHistory(t, older, newer)
		body, ok := h.NewestDifferingFrom(versions.Hash([]byte(newer)))
		if !ok {
			t.Fatal("no differing version found")
		}
		if !bytes.Equal(body, []byte(older)) {
			t.Fatalf("got %q, want the older version", body)
		}
	})

	t.Run("returns the newest version when none matches", func(t *testing.T) {
		h, _ := guardTestHistory(t, older, newer)
		body, ok := h.NewestDifferingFrom(versions.Hash([]byte("unrelated")))
		if !ok || !bytes.Equal(body, []byte(newer)) {
			t.Fatalf("got %q (ok=%v), want the newest version", body, ok)
		}
	})

	t.Run("no versions at all", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		v := versions.New(filepath.Join(dir, "versions"))
		doc := filepath.Join(dir, "board.htmlclay")
		h := NewGuardHistory(v, versions.Key(doc, nil), doc)
		if body, ok := h.NewestDifferingFrom("abc"); ok || body != nil {
			t.Fatalf("got %q, want nothing", body)
		}
	})
}

// guardTestServer registers a document the guard can judge: an api rules tag over
// a heading and a list, which is the shape every loss rule is written against.
func guardTestServer(t *testing.T, body string) (*Server, *session.File, string) {
	t.Helper()
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "board.htmlclay", body)
	f, err := srv.sessions.Register(path, session.ViaOsOpen)
	if err != nil {
		t.Fatalf("register board.htmlclay: %v", err)
	}
	return srv, f, path
}

// guardSave posts a document body the way a tab does, with whatever provenance
// headers the case names.
func guardSave(t *testing.T, srv *Server, f *session.File, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/_/save/"+f.Token, strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.SetPathValue("token", f.Token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.handleSave(w, req)
	return w
}

// guardKey reads the file's resolved history identity.
func guardKey(t *testing.T, f *session.File) string {
	t.Helper()
	f.Lock()
	defer f.Unlock()
	if f.HistoryKey() == "" {
		t.Fatal("the document has no history key, so the guard would skip it")
	}
	return f.HistoryKey()
}

// A save that clobbers the list is judged off the request path, and the response
// is the same success every save gets.
func TestGuardObservesSave(t *testing.T) {
	srv, f, path := guardTestServer(t, string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	key := guardKey(t, f)

	clobbered := string(lossParityPage("T", "alpha"))
	w := guardSave(t, srv, f, clobbered, map[string]string{"Save-Trigger": "user"})
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("save body is not JSON: %v", err)
	}
	if res["ok"] != true || res["msg"] != "Saved" || res["msgType"] != "success" {
		t.Errorf("save response = %s, want the ordinary success body", w.Body.String())
	}
	if res["etag"] != specwire.Etag([]byte(clobbered)) {
		t.Errorf("save etag = %v, want the stamp of what was stored", res["etag"])
	}

	srv.guardQueue.Settle()
	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event == nil {
		t.Fatalf("a gestured save that removed 3 of 4 items raised no event: %+v", st)
	}
	if got := st.Event.LossSummary.DestroyedAtoms; got != 3 {
		t.Errorf("destroyedAtoms = %d, want 3", got)
	}
	if got := st.Event.LossSummary.Provenance; got != dataguard.UIGestured {
		t.Errorf("provenance = %q, want %q", got, dataguard.UIGestured)
	}
}

// A data API write has no tab behind it, so the guard judges it as an external
// edit, exactly as Local does for origin === 'api'.
func TestGuardObservesDataAPIWrite(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	f, path := registerFixture(t, srv, "board.htmlclay", string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/fixtures/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	key := guardKey(t, f)

	w := postDataWrite(t, srv, "/_/api/fixtures/board.htmlclay", `{"title":"T","items":["alpha"]}`, nil)
	if w.Code != 200 {
		t.Fatalf("data write: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event == nil {
		t.Fatalf("an api write that clobbered the list raised no event: %+v", st)
	}
	if got := st.Event.LossSummary.Provenance; got != dataguard.External {
		t.Errorf("provenance = %q, want %q", got, dataguard.External)
	}
}

// Fact 1: a document edited on disk while no tab is open reaches no subscriber, so
// the watcher drops it. The page load is the only place it can be caught.
func TestGuardObservesOutsideEditWhileClosed(t *testing.T) {
	srv, f, path := guardTestServer(t, string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	key := guardKey(t, f)
	if w := guardSave(t, srv, f, string(lossParityPage("T2", "alpha", "beta", "gamma", "delta")), nil); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	if err := os.WriteFile(path, lossParityPage("T2", "alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve after the outside edit: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event == nil {
		t.Fatalf("an outside edit made while the document was closed raised no event: %+v", st)
	}
}

// A file the guard cannot key, and a file that is not HTML, are both skipped
// without a state file or an error: the guard must never break a write.
func TestGuardUnversionedSkips(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	html, err := os.ReadFile(f.AbsPath)
	if err != nil {
		t.Fatal(err)
	}

	srv.observeWrite(f, "", html, html, dataguard.External)
	srv.observeWrite(f, "key:no-such-document", nil, html, dataguard.External)
	txt := &session.File{AbsPath: filepath.Join(srv.sessions.HomeDir(), "notes.txt")}
	srv.observeWrite(txt, "key:no-such-document", nil, html, dataguard.External)
	srv.guardQueue.Settle()

	if _, err := srv.versions.ExistingFolder("key:no-such-document", f.AbsPath); err == nil {
		t.Fatal("a skipped observation created a history folder")
	}
	if st, err := srv.guardStore.Read("key:no-such-document", f.AbsPath); err == nil && st.Baseline != nil {
		t.Fatalf("a skipped observation seeded a baseline: %+v", st)
	}
}

// The guard parses a whole document per write. That work must never sit between a
// save and its response, or a large file would make every save feel slow.
func TestGuardDoesNotDelaySave(t *testing.T) {
	srv, f, _ := guardTestServer(t, string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}

	inside := make(chan struct{})
	release := make(chan struct{})
	// Registered here so it runs before the server's own drain cleanup: a guard
	// that does wait for its observation would otherwise hang the test binary
	// instead of failing this one case.
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	var once bool
	srv.guard = dataguard.NewGuard(dataguard.NewStore(func(key, absPath string) (string, error) {
		if !once {
			once = true
			close(inside)
			<-release
		}
		return "", errors.New("guard store blocked for this test")
	}), Extractor{}, nil)
	srv.guard.SetQueue(srv.guardQueue)

	body := string(lossParityPage("T", "alpha", "beta"))
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest("POST", "/_/save/"+f.Token, strings.NewReader(body))
		req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
		req.SetPathValue("token", f.Token)
		req.Header.Set("Save-Trigger", "user")
		w := httptest.NewRecorder()
		srv.handleSave(w, req)
		done <- w
	}()

	select {
	case <-inside:
	case <-time.After(5 * time.Second):
		t.Fatal("the guard never started judging the save")
	}
	select {
	case w := <-done:
		if w.Code != 200 {
			t.Errorf("save = %d %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the save waited for the guard to finish")
	}
	close(release)
	released = true
	srv.guardQueue.Settle()
}

// dataLossGet reads the document's data-loss address through the full stack, so
// the route and its headers are exercised rather than the handler alone.
func dataLossGet(t *testing.T, srv *Server, token string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(t, srv, "/_/data-loss/"+token, nil)
}

// dataLossPost resolves an event the way a page does: same-origin, JSON body.
func dataLossPost(t *testing.T, srv *Server, token, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/_/data-loss/"+token, strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Origin", "http://"+req.Host)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

func decodeDataLossEvent(t *testing.T, w *httptest.ResponseRecorder) *dataguard.ClientEvent {
	t.Helper()
	var body dataLossEventBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("data-loss body is not JSON: %v (%s)", err, w.Body.String())
	}
	return body.Event
}

// dataLossPending establishes a known-good baseline, then clobbers it with a save
// so the guard raises a pending event, and returns that event's id.
func dataLossPending(t *testing.T) (*Server, *session.File, string, string) {
	t.Helper()
	srv, f, path := guardTestServer(t, string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	if w := guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobber: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
	if event == nil {
		t.Fatal("the clobber raised no pending event")
	}
	return srv, f, path, event.ID
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A document with nothing pending answers null, and says so uncacheably.
func TestDataLossGetNoEvent(t *testing.T) {
	srv, f, _ := guardTestServer(t, string(lossParityPage("T", "alpha", "beta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}

	w := dataLossGet(t, srv, f.Token)
	if w.Code != 200 {
		t.Fatalf("data-loss GET = %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"event":null}` {
		t.Errorf("body = %s, want {\"event\":null}", got)
	}
}

// Fact 1 for the read route: an outside edit that landed while no tab was open is
// caught by the first GET, which answers with the event rather than null.
func TestDataLossGetCatchesUpOutsideEdit(t *testing.T) {
	srv, f, path := guardTestServer(t, string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	if w := guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	if err := os.WriteFile(path, lossParityPage("T", "alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := dataLossGet(t, srv, f.Token)
	if w.Code != 200 {
		t.Fatalf("data-loss GET = %d: %s", w.Code, w.Body.String())
	}
	event := decodeDataLossEvent(t, w)
	if event == nil {
		t.Fatal("the first GET after an outside edit answered null")
	}
	if event.Status != "pending" {
		t.Errorf("status = %q, want pending", event.Status)
	}
	if got := event.LossSummary.DestroyedAtoms; got != 3 {
		t.Errorf("destroyedAtoms = %d, want 3", got)
	}
}

// Each choice reaches its status and leaves the bytes that choice implies on disk.
func TestDataLossResolveChoices(t *testing.T) {
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	clobbered := string(lossParityPage("T", "alpha"))

	cases := []struct {
		choice string
		status string
		want   string
	}{
		{choice: "restore", status: "restored", want: full},
		{choice: "revert", status: "reverted", want: full},
		{choice: "dismiss", status: "dismissed", want: clobbered},
	}
	for _, c := range cases {
		t.Run(c.choice, func(t *testing.T) {
			srv, f, path, id := dataLossPending(t)
			w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":%q}`, id, c.choice), nil)
			if w.Code != 200 {
				t.Fatalf("%s = %d: %s", c.choice, w.Code, w.Body.String())
			}
			var res map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("resolve body is not JSON: %v", err)
			}
			if res["ok"] != true || res["choice"] != c.choice || res["status"] != c.status {
				t.Errorf("resolve body = %s, want ok/choice %s/status %s", w.Body.String(), c.choice, c.status)
			}
			if got := string(readFileBytes(t, path)); got != c.want {
				t.Errorf("file bytes after %s = %q, want %q", c.choice, got, c.want)
			}
		})
	}
}

// Every refusal leaves the file exactly as it was.
func TestDataLossResolveRefusals(t *testing.T) {
	t.Run("unknown choice is 400", func(t *testing.T) {
		srv, f, path, _ := dataLossPending(t)
		before := readFileBytes(t, path)
		w := dataLossPost(t, srv, f.Token, `{"choice":"explode"}`, nil)
		if w.Code != 400 {
			t.Fatalf("unknown choice = %d, want 400: %s", w.Code, w.Body.String())
		}
		if got := readFileBytes(t, path); !bytes.Equal(got, before) {
			t.Errorf("a refused resolve changed the file")
		}
	})

	t.Run("no pending event is 404", func(t *testing.T) {
		srv, f, path := guardTestServer(t, string(lossParityPage("T", "alpha")))
		if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
			t.Fatalf("serve: %d %s", w.Code, w.Body.String())
		}
		before := readFileBytes(t, path)
		w := dataLossPost(t, srv, f.Token, `{"choice":"dismiss"}`, nil)
		if w.Code != 404 {
			t.Fatalf("no event = %d, want 404: %s", w.Code, w.Body.String())
		}
		if got := readFileBytes(t, path); !bytes.Equal(got, before) {
			t.Errorf("a refused resolve changed the file")
		}
	})

	t.Run("id mismatch is 409", func(t *testing.T) {
		srv, f, path, _ := dataLossPending(t)
		before := readFileBytes(t, path)
		w := dataLossPost(t, srv, f.Token, `{"id":"not-this-event","choice":"dismiss"}`, nil)
		if w.Code != 409 {
			t.Fatalf("id mismatch = %d, want 409: %s", w.Code, w.Body.String())
		}
		if got := readFileBytes(t, path); !bytes.Equal(got, before) {
			t.Errorf("a refused resolve changed the file")
		}
	})

	t.Run("unrestorable is 422", func(t *testing.T) {
		srv, f, path, id := dataLossPending(t)
		// The page no longer has the rules the recoverable data was read with, so
		// a restore cannot reproduce it and must be refused rather than written.
		broken := []byte(`<html><body><h1 class="title">T</h1>` +
			`<script type="application/json" data-rules-name="api" data-rules-version="1">{other:".title"}</script></body></html>`)
		if err := os.WriteFile(path, broken, 0o644); err != nil {
			t.Fatal(err)
		}
		w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"restore"}`, id), nil)
		if w.Code != 422 {
			t.Fatalf("unrestorable = %d, want 422: %s", w.Code, w.Body.String())
		}
		if got := readFileBytes(t, path); !bytes.Equal(got, broken) {
			t.Errorf("a refused restore changed the file")
		}
	})
}

// The resolve route is a write, so a request the browser marks as a different
// site, or one carrying a foreign Origin, must never reach it. The host-wide
// middleware already refuses the former; the foreign Origin is what proves the
// route's own sameOrigin wrapper is still in front of the handler.
func TestDataLossCrossSiteRefused(t *testing.T) {
	cases := map[string]map[string]string{
		"cross-site":     {"Sec-Fetch-Site": "cross-site"},
		"foreign origin": {"Sec-Fetch-Site": "same-site", "Origin": "http://evil.example"},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			srv, f, path, id := dataLossPending(t)
			before := readFileBytes(t, path)
			w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"dismiss"}`, id), headers)
			if w.Code != 403 {
				t.Fatalf("refused resolve = %d, want 403: %s", w.Code, w.Body.String())
			}
			if got := readFileBytes(t, path); !bytes.Equal(got, before) {
				t.Errorf("a refused resolve changed the file")
			}
		})
	}
}

// Discovery names the address a client should read, and only for a document the
// guard can key, and a keyable document's meta announces the capability.
func TestDataGuardMetaHref(t *testing.T) {
	srv, f, _ := guardTestServer(t, string(lossParityPage("T", "alpha")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}

	var meta struct {
		Extensions []string `json:"extensions"`
		Document   *struct {
			DataGuard *struct {
				Href string `json:"href"`
			} `json:"dataGuard"`
		} `json:"document"`
	}
	w := serve(t, srv, "/_/meta/"+f.Token, nil)
	if w.Code != 200 {
		t.Fatalf("meta = %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatalf("meta body is not JSON: %v", err)
	}
	if meta.Document == nil || meta.Document.DataGuard == nil {
		t.Fatalf("a versioned document named no dataGuard: %s", w.Body.String())
	}
	if want := "/_/data-loss/" + f.Token; meta.Document.DataGuard.Href != want {
		t.Errorf("dataGuard.href = %q, want %q", meta.Document.DataGuard.Href, want)
	}
	announced := false
	for _, ext := range meta.Extensions {
		if ext == "data-guard-v1" {
			announced = true
		}
	}
	if !announced {
		t.Errorf("extensions did not announce data-guard-v1: %v", meta.Extensions)
	}

	plain, plainFile, _ := setupHandlerTest(t)
	w = serve(t, plain, "/_/meta/"+plainFile.Token, nil)
	if w.Code != 200 {
		t.Fatalf("unversioned meta = %d: %s", w.Code, w.Body.String())
	}
	meta = struct {
		Extensions []string `json:"extensions"`
		Document   *struct {
			DataGuard *struct {
				Href string `json:"href"`
			} `json:"dataGuard"`
		} `json:"document"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatalf("unversioned meta body is not JSON: %v", err)
	}
	if meta.Document != nil && meta.Document.DataGuard != nil {
		t.Errorf("an unversioned document named a dataGuard: %s", w.Body.String())
	}
}

// An open tab hears an event raised and then resolved on the live lane, with the
// same notification shape Local sends.
func TestDataLossNotifications(t *testing.T) {
	srv, f, _ := guardTestServer(t, string(lossParityPage("T", "alpha", "beta", "gamma", "delta")))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	if w := guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)

	if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobber: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	raised := waitFrame(t, live, time.Second)
	if raised["type"] != "notification" || raised["msgType"] != "data-loss" || raised["action"] != "raised" {
		t.Fatalf("raised frame = %v, want a data-loss notification with action raised", raised)
	}
	if raised["msg"] != "Saved data overwritten" {
		t.Errorf("raised msg = %v", raised["msg"])
	}
	data, ok := raised["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("raised notification carries no event object: %v", raised)
	}
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("raised event has no id: %v", data)
	}

	w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"dismiss"}`, id), nil)
	if w.Code != 200 {
		t.Fatalf("dismiss = %d: %s", w.Code, w.Body.String())
	}

	resolved := waitFrame(t, live, time.Second)
	if resolved["type"] != "notification" || resolved["msgType"] != "data-loss" || resolved["action"] != "resolved" {
		t.Fatalf("resolved frame = %v, want a data-loss notification with action resolved", resolved)
	}
	if resolved["msg"] != "Data guard resolved" {
		t.Errorf("resolved msg = %v", resolved["msg"])
	}
	resolvedData, ok := resolved["data"].(map[string]interface{})
	if !ok || resolvedData["id"] != id {
		t.Errorf("resolved data = %v, want {\"id\": %q}", resolved["data"], id)
	}
}

// A pending event reaches a page whose runtime cannot draw the notice: a document
// pinned to an older ClayJS, or using none, gets the same notice as a bar. Keys in
// the summary come from the document, so they must arrive escaped.
func TestGuardBarServedWhenPending(t *testing.T) {
	hostileKey := `<img src=x onerror=alert(1)>`
	whole := `<html><body><h1 class="title">T</h1><ul><li class="item">alpha</li><li class="item">beta</li>` +
		`<li class="item">gamma</li><li class="item">delta</li></ul>` +
		`<script type="application/json" data-rules-name="api" data-rules-version="1">{"` + hostileKey + `":".title", items:".item[]"}</script>` +
		`</body></html>`
	clobbered := `<html><body><ul><li class="item">alpha</li></ul>` +
		`<script type="application/json" data-rules-name="api" data-rules-version="1">{items:".item[]"}</script>` +
		`</body></html>`

	srv, f, path := guardTestServer(t, whole)
	w := serve(t, srv, "/board.htmlclay", nil)
	if w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); strings.Contains(body, "data-htmlclay-guard") {
		t.Errorf("a load with nothing pending carried the bar")
	}
	if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	// The outside edit loses the heading, so the event's summary names the key
	// the document used for it. The clobbered bytes served afterwards hold no such
	// string, so its presence in the page can only come from the bar.
	if err := os.WriteFile(path, []byte(clobbered), 0o644); err != nil {
		t.Fatal(err)
	}

	w = serve(t, srv, "/board.htmlclay", nil)
	if w.Code != 200 {
		t.Fatalf("serve with a pending event: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"data-htmlclay-guard",
		"Saved data was overwritten",
		"Undo the change",
		"Keep as is",
		`data-choice="dismiss"`,
		"data-guard-message",
		"/_/data-loss/",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the served page is missing %q from the bar", want)
		}
	}
	if strings.Contains(body, hostileKey) {
		t.Errorf("the bar served a document key raw: %q", hostileKey)
	}
	if want := "api.&lt;img src=x onerror=alert(1)&gt;"; !strings.Contains(body, want) {
		t.Errorf("the served page does not carry the escaped key %q", want)
	}

	event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
	if event == nil {
		t.Fatal("the outside edit raised no event for the bar to report")
	}
	if event.CanRevert != strings.Contains(body, "Undo the change") {
		t.Errorf("Undo the change shown = %v, but canRevert = %v", strings.Contains(body, "Undo the change"), event.CanRevert)
	}
	if event.Restorable != strings.Contains(body, `data-choice="restore"`) {
		t.Errorf("Restore shown = %v, but restorable = %v", strings.Contains(body, `data-choice="restore"`), event.Restorable)
	}
}

// The bar offers Restore only for an event the server says it can put back: a
// button for a restore that cannot land is a button that does nothing.
func TestGuardBarRestoreOnlyWhenRestorable(t *testing.T) {
	t.Run("a page that can take the data back draws Restore", func(t *testing.T) {
		srv, f, _, id := dataLossPending(t)
		event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
		if event == nil || event.ID != id || !event.Restorable {
			t.Fatalf("the clobber left no restorable event: %+v", event)
		}
		body := serve(t, srv, "/board.htmlclay", nil).Body.String()
		if !strings.Contains(body, `data-choice="restore"`) {
			t.Errorf("a restorable event served no Restore button: %s", body)
		}
	})

	t.Run("a page with no row to clone does not", func(t *testing.T) {
		srv, f, path := guardTestServer(t, string(lossParityPage("T", "alpha", "beta")))
		if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
			t.Fatalf("serve: %d %s", w.Code, w.Body.String())
		}
		if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha", "beta")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
		srv.guardQueue.Settle()
		// The outside edit empties the list, so the lost rows have no row left
		// to clone and the event cannot offer a restore.
		if err := os.WriteFile(path, lossParityPage("T"), 0o644); err != nil {
			t.Fatal(err)
		}
		if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
			t.Fatalf("serve with a pending event: %d %s", w.Code, w.Body.String())
		}
		srv.guardQueue.Settle()

		event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
		if event == nil {
			t.Fatal("the outside edit raised no event for the bar to report")
		}
		if event.Restorable {
			t.Fatalf("an emptied list reported a restorable event: %+v", event)
		}
		body := serve(t, srv, "/board.htmlclay", nil).Body.String()
		if strings.Contains(body, `data-choice="restore"`) {
			t.Errorf("an unrestorable event served a Restore button: %s", body)
		}
		for _, want := range []string{"data-htmlclay-guard", "data-guard-message", `data-choice="dismiss"`, "Keep as is"} {
			if !strings.Contains(body, want) {
				t.Errorf("the served page is missing %q from the bar", want)
			}
		}
	})
}

// browserRoundTrip returns the bytes a browser sends back for a page it was
// served: the document parsed and rendered again. Anything a parser moves lands
// where this puts it.
func browserRoundTrip(t *testing.T, served string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(served))
	if err != nil {
		t.Fatalf("parse the served page: %v", err)
	}
	var b strings.Builder
	if err := html.Render(&b, doc); err != nil {
		t.Fatalf("render the parsed page: %v", err)
	}
	return b.String()
}

// The bar is a server-injected banner like the setup bar, so a page that saves the
// bytes it was served stores none of it — including after the browser has parsed
// and re-rendered them, which is what a real save carries.
func TestGuardBarNeverSaved(t *testing.T) {
	srv, f, path, _ := dataLossPending(t)

	w := serve(t, srv, "/board.htmlclay", nil)
	if w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	served := w.Body.String()
	if !strings.Contains(served, "data-htmlclay-guard") {
		t.Fatalf("the served page did not carry the bar: %s", served)
	}

	if w := guardSave(t, srv, f, browserRoundTrip(t, served), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save of the served bytes: %d %s", w.Code, w.Body.String())
	}
	saved := string(readFileBytes(t, path))
	for _, forbidden := range []string{"data-htmlclay-guard", "Saved data was overwritten", "htmlclay-banner", "/_/data-loss/"} {
		if strings.Contains(saved, forbidden) {
			t.Errorf("a save of the served bytes stored %q: %q", forbidden, saved)
		}
	}
	if !strings.Contains(saved, "alpha") {
		t.Errorf("the save lost its real content: %q", saved)
	}
}

// A browser moves content placed after </body> into the body, and a comment
// there stays a child of <html>, so the banner's markers come back reversed and
// a byte-exact strip can no longer find them. The bar is inserted inside the
// body for that reason, and this drives the whole round trip a real save takes.
func TestGuardBarNotSavedAfterBrowserParse(t *testing.T) {
	srv, f, path, _ := dataLossPending(t)

	w := serve(t, srv, "/board.htmlclay", nil)
	if w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	served := w.Body.String()
	if !strings.Contains(served, "data-htmlclay-guard") {
		t.Fatalf("the served page did not carry the bar: %s", served)
	}

	parsed := browserRoundTrip(t, served)
	start := strings.Index(parsed, "<!--htmlclay-banner-->")
	end := strings.Index(parsed, "<!--/htmlclay-banner-->")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("the parsed page lost the banner markers (start %d, end %d): %s", start, end, parsed)
	}

	if w := guardSave(t, srv, f, parsed, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save of the parsed bytes: %d %s", w.Code, w.Body.String())
	}
	saved := string(readFileBytes(t, path))
	for _, forbidden := range []string{"data-htmlclay-guard", "Saved data was overwritten", "htmlclay-banner", "/_/data-loss/"} {
		if strings.Contains(saved, forbidden) {
			t.Errorf("a save of the parsed page stored %q: %q", forbidden, saved)
		}
	}
	if !strings.Contains(saved, "alpha") {
		t.Errorf("the save lost its real content: %q", saved)
	}
}

// A document may use any string as a rules key, $o included. The state decoder
// used to read a leading $o as the conformance vectors' ordered-object encoding,
// so a document with that field decoded to no baseline and quietly lost its
// guard. Production reads every key literally; only the vector loader unwraps.
func TestGuardDocumentFieldNamedDollarO(t *testing.T) {
	full := strings.Replace(string(lossParityPage("T", "alpha", "beta", "gamma", "delta")), `title:".title"`, `"$o":".title"`, 1)
	clobbered := strings.Replace(string(lossParityPage("T", "alpha")), `title:".title"`, `"$o":".title"`, 1)

	srv, f, path := guardTestServer(t, full)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, clobbered, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	st, err := srv.guardStore.Read(guardKey(t, f), path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Event == nil {
		t.Fatalf("an ordinary $o field hides the loss; stored baseline=%s", st.Baseline.Data)
	}
}

// The event lives in the document's versions folder, not in a server, so a
// restart still reports it and can still restore from the recover copy.
func TestGuardEventSurvivesRestartAndPrune(t *testing.T) {
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	storeDir := t.TempDir()
	docPath := filepath.Join(homeDir, "board.htmlclay")
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	if err := os.WriteFile(docPath, []byte(whole), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := newTestManager(t, homeDir)
	f, err := mgr.Register(docPath, session.ViaOsOpen)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	srv := newTestServer(t, listen(t), mgr, logging.NewStdout(), versions.New(storeDir))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	key := guardKey(t, f)
	if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobber: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	restarted := newTestServer(t, listen(t), mgr, logging.NewStdout(), versions.New(storeDir))
	restarted.versions.MaybePrune(key, docPath)

	event := decodeDataLossEvent(t, dataLossGet(t, restarted, f.Token))
	if event == nil {
		t.Fatal("the restarted server reported no event: the notice is lost across a restart")
	}
	if event.Status != "pending" {
		t.Errorf("status after restart = %q, want pending", event.Status)
	}
	if !event.CanRevert {
		t.Errorf("canRevert after restart = false, want the recover copy to still be there")
	}

	w := dataLossPost(t, restarted, f.Token, fmt.Sprintf(`{"id":%q,"choice":"restore"}`, event.ID), nil)
	if w.Code != 200 {
		t.Fatalf("restore after restart = %d: %s", w.Code, w.Body.String())
	}
	if got := string(readFileBytes(t, docPath)); got != whole {
		t.Errorf("bytes after restore = %q, want %q", got, whole)
	}
}

// A real browser save that clobbers saved data is the case the whole guard
// exists for: the event a page reads back names the loss and offers every way
// out of it.
func TestGuardBrowserSaveClobberRaises(t *testing.T) {
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	srv, f, _ := guardTestServer(t, whole)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobber: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	w := dataLossGet(t, srv, f.Token)
	if w.Code != 200 {
		t.Fatalf("data-loss GET = %d: %s", w.Code, w.Body.String())
	}
	event := decodeDataLossEvent(t, w)
	if event == nil {
		t.Fatal("a save that removed 3 of 4 items raised no event")
	}
	if event.Status != "pending" {
		t.Errorf("status = %q, want pending", event.Status)
	}
	if got := event.LossSummary.DestroyedAtoms; got != 3 {
		t.Errorf("destroyedAtoms = %d, want the 3 removed items", got)
	}
	if got := event.LossSummary.Provenance; got != dataguard.UIGestured {
		t.Errorf("provenance = %q, want %q", got, dataguard.UIGestured)
	}
	if !event.CanRevert {
		t.Error("canRevert = false, want the pre-clobber bytes to be recoverable")
	}
}

// Restore puts the whole island back on the page and answers the notice: the file
// carries the data again and the event is gone.
func TestGuardRestorePutsDataBack(t *testing.T) {
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	srv, f, path, id := dataLossPending(t)

	w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"restore"}`, id), nil)
	if w.Code != 200 {
		t.Fatalf("restore = %d: %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	onDisk, err := Extractor{}.Extract(readFileBytes(t, path))
	if err != nil {
		t.Fatalf("extract the restored file: %v", err)
	}
	original, err := Extractor{}.Extract([]byte(whole))
	if err != nil {
		t.Fatalf("extract the original: %v", err)
	}
	if !dataguard.IslandsEqual(onDisk, original) {
		t.Errorf("the restored file's data does not equal the original")
	}

	if event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token)); event != nil {
		t.Errorf("the restored document still reports an event: %+v", event)
	}
}

// A restore is this host's own write. Neither the watcher poll nor the page-load
// catch-up may read the bytes it left behind as a fresh outside edit, and the
// guard may not raise a second event for data it just put back.
func TestGuardRestoreDoesNotRefire(t *testing.T) {
	srv, f, path, id := dataLossPending(t)
	key := guardKey(t, f)

	// A tab is open on the document, so the watcher is watching it when the
	// restore lands, which is the poll that could mistake the new bytes.
	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)
	srv.watcher.quiet = 0
	srv.watcher.watch(f)
	t.Cleanup(func() {
		srv.watcher.shutdown()
		srv.hub.shutdown()
	})

	w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"restore"}`, id), nil)
	if w.Code != 200 {
		t.Fatalf("restore = %d: %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	// The restore records the bytes it left on disk, so nothing downstream reads
	// them as an edit nobody has judged yet.
	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if want := versions.Hash(readFileBytes(t, path)); st.LastSeenHash != want {
		t.Errorf("lastSeenHash = %q, want the hash of the restored bytes %q", st.LastSeenHash, want)
	}

	// The restore itself tells the open tab about the document it put back.
	restored := waitFrame(t, live, time.Second)
	if msg, _ := restored["msg"].(string); !strings.Contains(msg, "was restored") {
		t.Fatalf("the restore's own frame = %q, want the restored document", msg)
	}

	// One poll is enough to reach publish, so no quiet interval has to elapse.
	// The poll finds the restored bytes; because this host wrote them, it must
	// report nothing rather than announcing a change made outside the tab.
	srv.watcher.tick()
	srv.watcher.tick()
	srv.guardQueue.Settle()
	if msgs := changedOutsideFrames(t, live); len(msgs) != 0 {
		t.Fatalf("the poll reported this host's own restore as an outside change: %q", msgs)
	}

	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve after restore: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	if msgs := changedOutsideFrames(t, live); len(msgs) != 0 {
		t.Fatalf("the page load reported this host's own restore as an outside change: %q", msgs)
	}

	if ids := guardRaisedIDs(t, live); len(ids) != 0 {
		t.Fatalf("a restore raised new events: %v", ids)
	}
	if event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token)); event != nil {
		t.Fatalf("a restore raised a new event: %+v", event)
	}
}

// An outside edit to a document with a tab open reaches the guard through the
// watcher, which is the only path that sees it while the file is still watched.
func TestGuardOutsideEditWithTabOpen(t *testing.T) {
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	srv, f, path := guardTestServer(t, whole)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	srv.watcher.poll = 10 * time.Millisecond
	srv.watcher.quiet = 10 * time.Millisecond
	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)
	srv.watcher.watch(f)
	t.Cleanup(func() {
		srv.watcher.shutdown()
		srv.hub.shutdown()
	})

	if err := os.WriteFile(path, lossParityPage("T", "alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The ordinary outside-change notice arrives on the same lane first, so the
	// data-loss one is looked for among the frames that follow.
	deadline := time.Now().Add(5 * time.Second)
	var frame map[string]interface{}
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("the watcher never announced the outside edit to the guard")
		}
		frame = waitFrame(t, live, remaining)
		if frame["msgType"] == "data-loss" {
			break
		}
	}
	if frame["type"] != "notification" || frame["action"] != "raised" {
		t.Fatalf("frame = %v, want a data-loss notification with action raised", frame)
	}
	srv.guardQueue.Settle()

	event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
	if event == nil {
		t.Fatal("the announced outside edit left no event to resolve")
	}
	if event.LossSummary.Provenance != dataguard.External {
		t.Errorf("provenance = %q, want %q", event.LossSummary.Provenance, dataguard.External)
	}
}

// The guard keeps its state in the versions store, never beside the document: a
// user's folder holds their documents and nothing this feature added.
func TestGuardStateOutsideUserFolder(t *testing.T) {
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	srv, f, path := guardTestServer(t, whole)
	before := folderNames(t, filepath.Dir(path))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{whole, whole, string(lossParityPage("T", "alpha"))} {
		if w := guardSave(t, srv, f, body, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
		srv.guardQueue.Settle()
	}
	if event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token)); event == nil {
		t.Fatal("the clobber raised no event, so there is no guard state to keep out of the folder")
	}

	after := folderNames(t, filepath.Dir(path))
	if len(after) != len(before) {
		t.Errorf("the document's folder gained entries: %v, want %v", after, before)
	}
}

// folderNames lists what a document's own folder holds, sorted.
func folderNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// A resolve rewrites the user's file. Whatever an outside edit held must survive
// that rewrite in a version, so choosing the wrong button does not destroy another
// editor's work with no copy kept.
func TestGuardResolveKeepsCopyOfOutsideEdit(t *testing.T) {
	for _, choice := range []string{"revert", "restore"} {
		t.Run(choice, func(t *testing.T) {
			whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
			srv, f, path := guardTestServer(t, whole)
			if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
				t.Fatalf("serve: %d %s", w.Code, w.Body.String())
			}
			if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
				t.Fatalf("save: %d %s", w.Code, w.Body.String())
			}
			srv.guardQueue.Settle()
			key := guardKey(t, f)

			outside := lossParityPage("Outside work", "alpha", "outside-written-item")
			if err := os.WriteFile(path, outside, 0o644); err != nil {
				t.Fatal(err)
			}
			if w := serve(t, srv, "/board.htmlclay", nil); !strings.Contains(w.Body.String(), "data-htmlclay-guard") {
				t.Fatal("the outside edit raised no notice")
			}
			event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
			if event == nil {
				t.Fatal("the outside edit left no pending event to resolve")
			}
			if w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":%q}`, event.ID, choice), nil); w.Code != 200 {
				t.Fatalf("%s: %d %s", choice, w.Code, w.Body.String())
			}

			entries, err := srv.versions.List(key, path)
			if err != nil {
				t.Fatalf("list versions: %v", err)
			}
			kept := false
			for _, entry := range entries {
				body, err := srv.versions.Read(key, path, entry.Name)
				if err != nil {
					t.Fatalf("read version %s: %v", entry.Name, err)
				}
				if strings.Contains(string(body), "outside-written-item") {
					kept = true
				}
			}
			if !kept {
				t.Errorf("the outside edit is in no version after a %s, so the rewrite destroyed it", choice)
			}
		})
	}
}

// A folder whose trust was revoked no longer authorizes a rewrite, so a resolve
// is refused and the file keeps the bytes it had.
func TestGuardResolveRefusedAfterTrustRevoked(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	path := writeHome(t, srv, "ws/board.htmlclay", whole)
	root := filepath.Dir(path)
	if err := srv.sessions.InstallTrustedRoot(root); err != nil {
		t.Fatalf("install trusted root: %v", err)
	}
	f, err := srv.sessions.Register(path, session.ViaTrusted)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobber: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
	if event == nil {
		t.Fatal("the clobber raised no pending event")
	}

	srv.sessions.RevokeTrustedRoot(root)
	if !srv.trustedWriteRevoked(f) {
		t.Fatal("the fixture still holds write authority after revoking trust")
	}
	before := readFileBytes(t, path)
	w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"revert"}`, event.ID), nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("resolve after revoked trust = %d %s, want %d", w.Code, w.Body.String(), http.StatusUnauthorized)
	}
	if !bytes.Equal(before, readFileBytes(t, path)) {
		t.Error("the refused resolve wrote to the file")
	}
}

// A resolve replaces the document on disk, so every other open tab hears the new
// document, and hears it before the event is announced resolved: a tab that heard
// the resolve first would ask for the event it still holds.
func TestGuardResolvePublishesToOtherTabs(t *testing.T) {
	srv, f, _, id := dataLossPending(t)
	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)
	t.Cleanup(func() { srv.hub.shutdown() })

	w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"restore"}`, id), nil)
	if w.Code != 200 {
		t.Fatalf("restore: %d %s", w.Code, w.Body.String())
	}

	var frames []map[string]interface{}
	deadline := time.After(700 * time.Millisecond)
loop:
	for {
		select {
		case raw := <-live.ch:
			_, out := splitFrame(t, raw.frame)
			frames = append(frames, out)
		case <-deadline:
			break loop
		}
	}

	documentAt, resolvedAt := -1, -1
	for i, fr := range frames {
		if fr["msgType"] == "data-loss" && fr["action"] == "resolved" {
			if resolvedAt < 0 {
				resolvedAt = i
			}
			continue
		}
		if documentAt < 0 {
			if data, ok := fr["data"].(map[string]interface{}); ok {
				if htmlText, ok := data["html"].(string); ok && strings.Contains(htmlText, "beta") {
					documentAt = i
				}
			}
		}
	}
	if documentAt < 0 {
		t.Fatalf("the open tab received no restored document, only %d frame(s)", len(frames))
	}
	if resolvedAt < 0 {
		t.Fatalf("the open tab never heard the event was resolved, only %d frame(s)", len(frames))
	}
	if documentAt > resolvedAt {
		t.Error("the resolved frame reached the open tab before the restored document")
	}
}

// The guard's event and its recover copy are the whole point of the notice, so an
// aged-out first-open history must not carry them away with it.
func TestGuardPendingEventSurvivesProvisionalPrune(t *testing.T) {
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	storeDir := t.TempDir()
	docPath := filepath.Join(homeDir, "board.htmlclay")
	if err := os.WriteFile(docPath, lossParityPage("T", "alpha", "beta", "gamma", "delta"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := newTestManager(t, homeDir)
	f, err := mgr.Register(docPath, session.ViaOsOpen)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	srv := newTestServer(t, listen(t), mgr, logging.NewStdout(), versions.New(storeDir))
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	key := guardKey(t, f)
	dir, err := srv.versions.ExistingFolder(key, docPath)
	if err != nil {
		t.Fatalf("existing folder: %v", err)
	}
	metaPath := filepath.Join(dir, "meta.json")
	readMeta := func() map[string]interface{} {
		t.Helper()
		var meta map[string]interface{}
		if err := json.Unmarshal(readFileBytes(t, metaPath), &meta); err != nil {
			t.Fatalf("meta is not JSON: %v", err)
		}
		return meta
	}
	if meta := readMeta(); meta["provisional"] != true {
		t.Fatalf("the first-open history is not provisional, so pruning would never reach the branch under test: %v", meta)
	}
	if err := os.WriteFile(docPath, lossParityPage("T"), 0o644); err != nil {
		t.Fatal(err)
	}
	event := decodeDataLossEvent(t, dataLossGet(t, srv, f.Token))
	if event == nil {
		t.Fatal("the outside edit raised no pending event")
	}
	meta := readMeta()
	if meta["provisional"] == true {
		t.Errorf("the raised event left the history provisional, so a later prune may reclaim it: %v", meta)
	}
	meta["updatedAt"] = time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	aged, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, aged, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	pruned := versions.New(storeDir)
	pruned.PruneAll()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("pruning deleted the folder holding the pending event and its recover copy: %v", err)
	}

	guardStore := dataguard.NewStore(pruned.ExistingFolder)
	st, err := guardStore.Read(key, docPath)
	if err != nil {
		t.Fatalf("read guard state after pruning: %v", err)
	}
	if st.Event == nil {
		t.Fatal("pruning cleared the pending event")
	}
	recoverCopy, err := guardStore.ReadRecover(key, docPath)
	if err != nil {
		t.Fatalf("read recover copy after pruning: %v", err)
	}
	if len(recoverCopy) == 0 {
		t.Error("pruning deleted the recover copy the event offers back")
	}
}

// Every event's timestamps come from the clock at the moment it was raised, so a
// person can tell a loss that just happened from one the server started with.
func TestGuardTimestampsAdvance(t *testing.T) {
	whole := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	srv, f, path := guardTestServer(t, whole)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, whole, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	before := time.Now().UnixMilli()
	if w := guardSave(t, srv, f, string(lossParityPage("T", "alpha")), map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobber: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	key := guardKey(t, f)
	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("read guard state: %v", err)
	}
	if st.Event == nil {
		t.Fatal("the clobber raised no event")
	}
	if st.Event.FirstDetectedAt < before {
		t.Errorf("loss detected at %d, before the destructive request began at %d (lastWriteAt=%d)", st.Event.FirstDetectedAt, before, st.Event.LastWriteAt)
	}
}

// A document with no history folder yet has nothing pending, and says so with the
// contract's 200 and null event rather than failing the read.
func TestGuardGetWithNoHistory(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "fresh.htmlclay", string(lossParityPage("T", "alpha", "beta")))
	f, err := srv.sessions.Register(path, session.ViaTrusted)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if w := serve(t, srv, "/fresh.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}

	w := dataLossGet(t, srv, f.Token)
	if w.Code != 200 {
		t.Fatalf("data-loss GET = %d %s, want 200", w.Code, w.Body.String())
	}
	if event := decodeDataLossEvent(t, w); event != nil {
		t.Errorf("event = %+v, want null", event)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"event":null}` {
		t.Errorf("body = %s, want {\"event\":null}", got)
	}
}

// observedGuardStore replaces the server's guard store with one that signals on its
// first read, so a test can tell when a load has read the document and is about to
// judge it, without waiting on a timer.
func observedGuardStore(t *testing.T, srv *Server, reached chan struct{}) {
	t.Helper()
	var once sync.Once
	store := dataguard.NewStore(func(key, absPath string) (string, error) {
		once.Do(func() { close(reached) })
		return srv.versions.ExistingFolder(key, absPath)
	})
	srv.guardStore = store
	srv.guard = dataguard.NewGuard(store, Extractor{}, nil)
	srv.guard.SetQueue(srv.guardQueue)
}

// A load whose read began before a write must not clear the loss that write raised:
// its judgment runs on the document's queue against bytes read inside that job.
func TestGuardStaleReadDoesNotClearNewLoss(t *testing.T) {
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	clobbered := string(lossParityPage("T", "alpha"))
	srv, f, path := guardTestServer(t, full)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	key := guardKey(t, f)

	read := make(chan struct{})
	observedGuardStore(t, srv, read)
	// The in-memory record is dropped so the load's comparison consults the store,
	// which is where the signal comes from.
	srv.guardSeenMu.Lock()
	delete(srv.guardSeen, key)
	srv.guardSeenMu.Unlock()

	// Hold the document's queue, so the load's judgment waits behind the write.
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	srv.guardQueue.Do(key, func() {
		close(entered)
		<-release
	})
	<-entered

	loadDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { loadDone <- dataLossGet(t, srv, f.Token) }()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("the load never read the document")
	}

	if w := guardSave(t, srv, f, clobbered, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("clobbering save: %d %s", w.Code, w.Body.String())
	}

	close(release)
	released = true
	select {
	case <-loadDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the load never returned")
	}
	srv.guardQueue.Settle()

	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event == nil {
		t.Fatalf("a read holding old bytes cleared the loss the save raised (status %q)", st.Status)
	}
	if st.Event.Status != "pending" {
		t.Fatalf("event status = %q, want pending", st.Event.Status)
	}
}

// A document that is only opened, never saved, still has a baseline, so an outside
// edit made after that one open is caught by the next load.
func TestGuardFirstOpenThenOutsideClobberRaises(t *testing.T) {
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	srv, f, path := guardTestServer(t, full)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	key := guardKey(t, f)
	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Baseline == nil || st.LastSeenHash == "" {
		t.Fatalf("the first load seeded no baseline: %+v", st)
	}

	if err := os.WriteFile(path, lossParityPage("T", "alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := serve(t, srv, "/board.htmlclay", nil)
	if w.Code != 200 {
		t.Fatalf("second load: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	st, err = srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event == nil {
		t.Fatal("an outside edit after a single open raised no event")
	}
	if !strings.Contains(w.Body.String(), "data-htmlclay-guard") {
		t.Fatal("the load after the outside edit served no notice for the pending loss")
	}
}

// A page load waiting on its catch-up must not hold the document's file lock, or
// every save to that document waits behind the guard's queue.
func TestGuardCatchUpDoesNotBlockSave(t *testing.T) {
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	outside := string(lossParityPage("T", "alpha", "beta", "gamma"))
	srv, f, path := guardTestServer(t, full)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	key := guardKey(t, f)

	catchUp := make(chan struct{})
	observedGuardStore(t, srv, catchUp)
	srv.guardSeenMu.Lock()
	delete(srv.guardSeen, key)
	srv.guardSeenMu.Unlock()

	// Hold the document's queue, so the load's catch-up cannot finish.
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	srv.guardQueue.Do(key, func() {
		close(entered)
		<-release
	})
	<-entered

	if err := os.WriteFile(path, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}

	loadDone := make(chan struct{})
	go func() {
		serve(t, srv, "/board.htmlclay", nil)
		close(loadDone)
	}()
	select {
	case <-catchUp:
	case <-time.After(5 * time.Second):
		t.Fatal("the page load never reached its guard catch-up")
	}

	saveDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { saveDone <- guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}) }()
	select {
	case w := <-saveDone:
		if w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a save waited behind the page load's catch-up for the file lock")
	}

	close(release)
	released = true
	select {
	case <-loadDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the page load never returned")
	}
	srv.guardQueue.Settle()
}

// guardRaisedIDs drains the raised guard events a live subscriber received.
// changedOutsideFrames drains what the live lane delivered and returns the
// messages that report a change made outside an open tab.
func changedOutsideFrames(t *testing.T, sub *subscriber) []string {
	t.Helper()
	var found []string
	for {
		select {
		case q := <-sub.ch:
			_, out := splitFrame(t, q.frame)
			if msg, _ := out["msg"].(string); strings.Contains(msg, "changed on disk outside this tab") {
				found = append(found, msg)
			}
		default:
			return found
		}
	}
}

func guardRaisedIDs(t *testing.T, sub *subscriber) []string {
	t.Helper()
	var ids []string
	for {
		select {
		case q := <-sub.ch:
			_, out := splitFrame(t, q.frame)
			if out["msgType"] != "data-loss" || out["action"] != "raised" {
				continue
			}
			raw, err := json.Marshal(out["data"])
			if err != nil {
				t.Fatalf("raised event is not JSON: %v", err)
			}
			var event struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatalf("raised event is not JSON: %v", err)
			}
			ids = append(ids, event.ID)
		default:
			return ids
		}
	}
}

// Two sites share one live-sync runtime and one document. An outside edit is judged
// by the site that registered the document and by no other, so one edit raises one
// event, the id it raised is the one a resolve answers, and a dismissal stays
// dismissed.
func TestGuardHostedSitesJudgeOnce(t *testing.T) {
	full := string(lossParityPage("T", "alpha", "beta", "gamma", "delta"))
	clobbered := lossParityPage("T", "alpha")
	srv, f, path := guardTestServer(t, full)
	if w := serve(t, srv, "/board.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve: %d %s", w.Code, w.Body.String())
	}
	if w := guardSave(t, srv, f, full, map[string]string{"Save-Trigger": "user"}); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	key := guardKey(t, f)

	// The second site shares the live-sync runtime and the store but registered
	// nothing, so the document is not its to judge.
	other := newTestHostedServer(t, "other.localhost:9999", nil, newTestManager(t, srv.sessions.HomeDir()), srv.logger, srv.versions, srv.ls)

	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)
	t.Cleanup(func() { srv.hub.remove(live) })

	if err := os.WriteFile(path, clobbered, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.versions.Backup(key, path, clobbered); err != nil {
		t.Fatal(err)
	}
	f.Lock()
	srv.watcher.notifyExternalWrites(f, key, clobbered)
	f.Unlock()
	srv.guardQueue.Settle()
	other.guardQueue.Settle()

	raised := guardRaisedIDs(t, live)
	if len(raised) != 1 {
		t.Fatalf("one outside edit raised %d events: %v", len(raised), raised)
	}
	st, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event == nil || st.Event.ID != raised[0] {
		t.Fatalf("persisted event %+v is not the one the sites raised (%s)", st.Event, raised[0])
	}

	w := dataLossPost(t, srv, f.Token, fmt.Sprintf(`{"id":%q,"choice":"dismiss"}`, raised[0]), nil)
	if w.Code != 200 {
		t.Fatalf("dismiss of the raised event: %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	other.guardQueue.Settle()

	st, err = srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if st.Event != nil {
		t.Fatalf("the dismissed loss was resurrected: %+v", st.Event)
	}
	if again := guardRaisedIDs(t, live); len(again) != 0 {
		t.Fatalf("the dismissed loss was raised again: %v", again)
	}
}
