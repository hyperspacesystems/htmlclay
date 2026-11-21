package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/recalc"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/versions"
)

type fakeRecalcRunner struct {
	mu   sync.Mutex
	urls []string
	fn   func(ctx context.Context, docURL, etag string) error
}

func (f *fakeRecalcRunner) Recalculate(ctx context.Context, docURL, etag string) error {
	f.mu.Lock()
	f.urls = append(f.urls, docURL)
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(ctx, docURL, etag)
	}
	return nil
}

func (f *fakeRecalcRunner) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

func saveAsHidden(t *testing.T, srv *Server, f *session.File, body, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/_/save/"+f.Token, strings.NewReader(body))
	req.Host = srv.Host()
	sameOriginHeaders(req)
	req.Header.Set("Clay-Recalc-Source", "hidden")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

// TestHiddenBrowserFallbackUnit covers the fallback to the hidden browser with a fake runner:
// which outcome each runner error maps to, that a hidden save reaches open edit tabs, and that
// quitting cancels an in-flight recalculation.
func TestHiddenBrowserFallbackUnit(t *testing.T) {
	const doc = `<!DOCTYPE html><html><body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="state.qty*state.price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`

	setup := func(t *testing.T) (*Server, *session.File) {
		t.Helper()
		srv, _, _ := setupHandlerTest(t)
		invoice := writeHome(t, srv, "invoice.htmlclay", doc)
		if _, err := srv.sessions.Register(invoice, session.ViaOsOpen); err != nil {
			t.Fatal(err)
		}
		f, ok := srv.sessions.LookupByPath(invoice)
		if !ok {
			t.Fatal("the invoice is not registered")
		}
		return srv, f
	}

	recv := func(t *testing.T, done chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
		t.Helper()
		select {
		case w := <-done:
			return w
		case <-time.After(10 * time.Second):
			t.Fatal("the waited write did not answer within 10s")
			return nil
		}
	}

	metaOf := func(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
		t.Helper()
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("answer is not JSON: %v (%q)", err, w.Body.String())
		}
		meta, _ := body["__meta__"].(map[string]interface{})
		if meta == nil {
			t.Fatalf("answer carries no __meta__: %q", w.Body.String())
		}
		return meta
	}

	t.Run("a hidden save answers fresh values and reaches open edit tabs", func(t *testing.T) {
		// Bug: open edit tabs miss the hidden save and hit a 412 conflict on their next save,
		// or the write answers the stale bytes it wrote itself.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		var runner *fakeRecalcRunner
		runner = &fakeRecalcRunner{fn: func(ctx context.Context, docURL, etag string) error {
			if got, want := docURL, srv.documentURLFor(f); got != want {
				t.Errorf("Recalculate URL = %q, want %q", got, want)
			}
			stored := strings.Replace(readDoc(t, f.AbsPath), string(dataapi.StaleMark), "", 1)
			stored = strings.Replace(stored, ">10</b>", ">70</b>", 1)
			w := saveAsHidden(t, srv, f, stored, etag)
			if w.Code != 200 {
				return recalc.ErrSaveRefused
			}
			return nil
		}}
		srv.SetRecalcRunner(runner)

		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			done <- postDataWrite(t, srv, "/_/api/invoice.htmlclay?recalculate=wait&meta", `{"qty":"7"}`, nil)
		}()

		first := waitFrame(t, sub, 2*time.Second)
		data, _ := first["data"].(map[string]interface{})
		recalcField, _ := data["recalculate"].(map[string]interface{})
		if recalcField == nil {
			t.Fatalf("the disk frame carries no recalculate field: %v", data)
		}
		id, _ := recalcField["id"].(string)
		etag, _ := recalcField["etag"].(string)

		req := httptest.NewRequest("POST", "/_/recalc/"+f.Token,
			strings.NewReader(`{"id":"`+id+`","etag":"`+etag+`","status":"busy"}`))
		req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
		sameOriginHeaders(req)
		nw := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(nw, req)
		if nw.Code != http.StatusNoContent {
			t.Fatalf("the busy note = %d, want 204: %s", nw.Code, nw.Body.String())
		}

		w := recv(t, done)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":"70"`) {
			t.Fatalf("the waited write = %d %s, want fresh values", w.Code, w.Body.String())
		}
		meta := metaOf(t, w)
		if meta["recalculated"] != true || meta["stale"] != false {
			t.Errorf("__meta__ = %v, want recalculated true and stale false", meta)
		}
		if got := w.Header().Get("Clay-Stale"); got != "" {
			t.Errorf("Clay-Stale = %q, want no header on a recalculated answer", got)
		}
		if strings.Contains(readDoc(t, f.AbsPath), string(dataapi.StaleMark)) {
			t.Error("the stale mark survived the hidden save")
		}
		if calls := runner.calls(); len(calls) != 1 {
			t.Fatalf("the runner ran %d times, want 1 (%v)", len(calls), calls)
		}

		second := waitFrame(t, sub, 2*time.Second)
		sdata, _ := second["data"].(map[string]interface{})
		html, _ := sdata["html"].(string)
		if !strings.Contains(html, ">70</b>") {
			t.Errorf("the edit tabs were not told about the hidden save: %v", second)
		}
	})

	t.Run("runner errors map to the write's codes", func(t *testing.T) {
		cases := []struct {
			err  error
			want string
		}{
			{recalc.ErrNoBrowser, "no-browser"},
			{recalc.ErrSaveRefused, "save-refused"},
			{recalc.ErrPageError, "page-error"},
			{recalc.ErrTimeout, "timeout"},
		}
		for _, tc := range cases {
			t.Run(tc.want, func(t *testing.T) {
				srv, _ := setup(t)
				srv.SetRecalcRunner(&fakeRecalcRunner{fn: func(ctx context.Context, docURL, etag string) error {
					return tc.err
				}})
				w := postDataWrite(t, srv, "/_/api/invoice.htmlclay?recalculate=wait&meta", `{"qty":"7"}`, nil)
				if meta := metaOf(t, w); meta["code"] != tc.want {
					t.Errorf("__meta__ code = %v, want %s (body %s)", meta["code"], tc.want, w.Body.String())
				}
			})
		}
	})

	t.Run("a hidden save without If-Match is refused with 428 and leaves disk untouched", func(t *testing.T) {
		// Bug: a hidden save fired before host discovery settled overwrote concurrent edits last-write-wins.
		srv, f := setup(t)
		srv.SetRecalcRunner(&fakeRecalcRunner{})
		before := readDoc(t, f.AbsPath)
		w := saveAsHidden(t, srv, f, strings.Replace(before, ">10</b>", ">999</b>", 1), "")
		if w.Code != http.StatusPreconditionRequired {
			t.Fatalf("headerless hidden save = %d %s, want 428", w.Code, w.Body.String())
		}
		if after := readDoc(t, f.AbsPath); after != before {
			t.Errorf("headerless hidden save modified disk")
		}
	})

	t.Run("a late hidden save after timeout still publishes to open edit tabs", func(t *testing.T) {
		// Bug: a hidden save arriving milliseconds after the wait timed out skipped laneLive and stranded open edit tabs at 412.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)
		srv.SetRecalcRunner(&fakeRecalcRunner{fn: func(ctx context.Context, docURL, etag string) error {
			return recalc.ErrTimeout
		}})
		w := postDataWrite(t, srv, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=500", `{"qty":"7"}`, nil)
		if meta := metaOf(t, w); meta["code"] != "timeout" {
			t.Fatalf("code = %v, want timeout", meta["code"])
		}
		first := waitFrame(t, sub, 2*time.Second)
		etag, _ := first["data"].(map[string]interface{})["etag"].(string)
		stored := strings.Replace(readDoc(t, f.AbsPath), string(dataapi.StaleMark), "", 1)
		stored = strings.Replace(stored, ">10</b>", ">70</b>", 1)
		if sw := saveAsHidden(t, srv, f, stored, etag); sw.Code != 200 {
			t.Fatalf("late hidden save = %d %s, want 200", sw.Code, sw.Body.String())
		}
		second := waitFrame(t, sub, 2*time.Second)
		sdata, _ := second["data"].(map[string]interface{})
		html, _ := sdata["html"].(string)
		if !strings.Contains(html, ">70</b>") {
			t.Errorf("late hidden save did not reach open edit tabs: %v", second)
		}
	})

	t.Run("short timeout with a busy tab still invokes the hidden runner", func(t *testing.T) {
		// Bug: fixed 750ms busy grace starved ?timeout=600 before the hidden runner ever ran.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)
		runner := &fakeRecalcRunner{fn: func(ctx context.Context, docURL, etag string) error {
			stored := strings.Replace(readDoc(t, f.AbsPath), string(dataapi.StaleMark), "", 1)
			stored = strings.Replace(stored, ">10</b>", ">70</b>", 1)
			if sw := saveAsHidden(t, srv, f, stored, etag); sw.Code != 200 {
				return recalc.ErrSaveRefused
			}
			return nil
		}}
		srv.SetRecalcRunner(runner)

		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			done <- postDataWrite(t, srv, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=600", `{"qty":"7"}`, nil)
		}()
		first := waitFrame(t, sub, 2*time.Second)
		rf := first["data"].(map[string]interface{})["recalculate"].(map[string]interface{})
		req := httptest.NewRequest("POST", "/_/recalc/"+f.Token,
			strings.NewReader(`{"id":"`+rf["id"].(string)+`","etag":"`+rf["etag"].(string)+`","status":"busy"}`))
		req.Host = srv.Host()
		sameOriginHeaders(req)
		nw := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(nw, req)
		if nw.Code != http.StatusNoContent {
			t.Fatalf("busy note = %d, want 204", nw.Code)
		}
		w := recv(t, done)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":"70"`) {
			t.Fatalf("short timeout write = %d %s, want total 70", w.Code, w.Body.String())
		}
		if len(runner.calls()) != 1 {
			t.Fatalf("runner calls = %d, want 1", len(runner.calls()))
		}
	})

	t.Run("hidden request escapes special filename characters and blocks desktop actions", func(t *testing.T) {
		srv, _, _ := setupHandlerTest(t)
		p := writeHome(t, srv, "Invoice #12 10%.htmlclay", doc)
		if _, err := srv.sessions.Register(p, session.ViaOsOpen); err != nil {
			t.Fatal(err)
		}
		f, _ := srv.sessions.LookupByPath(p)
		var gotURL string
		srv.SetRecalcRunner(&fakeRecalcRunner{fn: func(ctx context.Context, docURL, etag string) error {
			gotURL = docURL
			return recalc.ErrNoBrowser
		}})
		postDataWrite(t, srv, "/_/api/"+url.PathEscape("Invoice #12 10%.htmlclay")+"?recalculate=wait", `{"qty":"7"}`, nil)
		parsed, err := url.Parse(gotURL)
		if err != nil {
			t.Fatalf("Recalculate received unparseable URL %q: %v", gotURL, err)
		}
		if parsed.Path != "/Invoice #12 10%.htmlclay" {
			t.Fatalf("decoded path = %q, want /Invoice #12 10%%.htmlclay (raw %q)", parsed.Path, gotURL)
		}
		// Desktop action routes must refuse hidden-browser requests.
		var revealed bool
		srv.SetHooks(Hooks{Reveal: func(string) error { revealed = true; return nil }})
		req := httptest.NewRequest("POST", "/_/reveal/"+f.Token, nil)
		req.Host = srv.Host()
		sameOriginHeaders(req)
		req.Header.Set(recalc.HiddenSourceHeader, "hidden")
		rw := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rw, req)
		if rw.Code != http.StatusForbidden || revealed {
			t.Errorf("hidden /_/reveal = %d revealed=%v, want 403 and false", rw.Code, revealed)
		}
	})

	t.Run("shutdown cancels an in-flight recalculation", func(t *testing.T) {
		// Bug: quitting htmlclay hangs on a page that is still recalculating.
		srv, _ := setup(t)
		entered := make(chan struct{})
		exited := make(chan error, 1)
		var once sync.Once
		srv.SetRecalcRunner(&fakeRecalcRunner{fn: func(ctx context.Context, docURL, etag string) error {
			once.Do(func() { close(entered) })
			<-ctx.Done()
			exited <- ctx.Err()
			return ctx.Err()
		}})

		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- postDataWrite(t, srv, "/_/api/invoice.htmlclay?recalculate=wait", `{"qty":"7"}`, nil) }()

		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the hidden runner was never called")
		}
		closedAt := time.Now()
		_ = srv.Close()
		select {
		case err := <-exited:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("the in-flight context ended with %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("shutdown did not cancel the in-flight recalculation")
		}
		if took := time.Since(closedAt); took > 2*time.Second {
			t.Errorf("cancellation took %v", took)
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("the waited write never answered after shutdown")
		}
	})
}

func findTestBrowser() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HTMLCLAY_RECALC_BROWSER")), "none") {
		return ""
	}
	if b := recalc.FindBrowser(); b != "" {
		return b
	}
	var roots []string
	if p := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); p != "" {
		roots = append(roots, p)
	}
	if localAppData := os.Getenv("LocalAppData"); localAppData != "" {
		roots = append(roots, filepath.Join(localAppData, "ms-playwright"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Library", "Caches", "ms-playwright"))
		roots = append(roots, filepath.Join(home, ".cache", "ms-playwright"))
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || (!strings.HasPrefix(entry.Name(), "chromium") && !strings.HasPrefix(entry.Name(), "chrome-for-testing")) {
				continue
			}
			var found string
			_ = filepath.WalkDir(filepath.Join(root, entry.Name()), func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || found != "" {
					return nil
				}
				switch d.Name() {
				case "chrome", "Google Chrome for Testing", "chrome-headless-shell", "headless_shell",
					"chrome.exe", "chrome-headless-shell.exe", "headless_shell.exe":
					if runtime.GOOS == "windows" {
						found = path
					} else if info, statErr := d.Info(); statErr == nil && info.Mode()&0o111 != 0 {
						found = path
					}
				}
				return nil
			})
			if found != "" {
				return found
			}
		}
	}
	return ""
}

func requireTestBrowser(t *testing.T) string {
	t.Helper()
	bin := findTestBrowser()
	if bin == "" {
		if os.Getenv("HTMLCLAY_REQUIRE_BROWSER") == "1" {
			t.Fatal("HTMLCLAY_REQUIRE_BROWSER=1 but no Chromium-family browser was found")
		}
		t.Skip("no Chromium-family browser installed")
	}
	return bin
}

type recalcEnv struct {
	srv  *Server
	f    *session.File
	home string
	port int
}

func setupRecalcEnv(t *testing.T, doc string) *recalcEnv {
	t.Helper()
	homeDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mgr := newTestManager(t, homeDir)
	store := versions.New(t.TempDir())
	logger := logging.NewStdout()
	ls := NewLiveSync(store, logger)
	t.Cleanup(ls.Shutdown)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	host := fmt.Sprintf("htmlclay1.localhost:%d", port)
	srv := NewHosted(host, func(string) bool { return true }, mgr, logger, store, ls)
	srv.uploadsDir = t.TempDir()
	t.Cleanup(func() { _ = srv.Close() })

	hs := &http.Server{Handler: srv}
	go hs.Serve(ln)
	t.Cleanup(func() { _ = hs.Close() })

	path := filepath.Join(homeDir, "invoice.htmlclay")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Register(path, session.ViaOsOpen); err != nil {
		t.Fatal(err)
	}
	f, ok := mgr.LookupByPath(path)
	if !ok {
		t.Fatal("the invoice is not registered")
	}
	return &recalcEnv{srv: srv, f: f, home: homeDir, port: port}
}

func (e *recalcEnv) newRunner(t *testing.T) *recalc.Runner {
	t.Helper()
	runner := recalc.New(
		recalc.WithBrowser(requireTestBrowser(t)),
		recalc.WithIdleTimeout(10*time.Second),
		recalc.WithLogger(t.Logf),
	)
	t.Cleanup(runner.Close)
	e.srv.SetRecalcRunner(runner)
	return runner
}

func (e *recalcEnv) postWrite(t *testing.T, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(body))
	req.Host = e.srv.Host()
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, req)
	return w
}

func decodeWriteMeta(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("answer is not JSON: %v (%q)", err, w.Body.String())
	}
	meta, _ := body["__meta__"].(map[string]interface{})
	if meta == nil {
		t.Fatalf("answer carries no __meta__: %q", w.Body.String())
	}
	return meta
}

func TestHiddenBrowserRecalc(t *testing.T) {
	if runtime.GOOS == "windows" && testOrigUserProfile != "" {
		t.Setenv("USERPROFILE", testOrigUserProfile)
	}
	requireTestBrowser(t)

	t.Run("hostile calculator with no autosave recalculates and every guard holds", func(t *testing.T) {
		var otherHits atomic.Int64
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			otherHits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer other.Close()
		otherPort := other.URL[strings.LastIndex(other.URL, ":")+1:]

		const docTemplate = `<!DOCTYPE html>
<html>
<head>
<script src="https://clayjs.com/v1/clay.js"></script>
<script src="https://clayjs.com/v1/sap.js"></script>
</head>
<body>
<main sap state="qty=1 price=10">
<input id="qty" bind="qty" value="1">
<b id="total" text="state.qty*state.price">10</b>
</main>
<script>
alert("a");
confirm("b");
prompt("c");
window.print();
document.documentElement.dataset.printed = "ok";
const pop = window.open("http://127.0.0.1:OTHERPORT/popup");
document.documentElement.dataset.popupBlocked = pop === null ? "ok" : "opened";
const f = document.createElement("iframe");
f.src = "http://other.localhost:OTHERPORT/iframe";
document.body.appendChild(f);
try { new WebSocket("ws://127.0.0.1:OTHERPORT/ws"); } catch {}
try { new WebSocket("ws://other.localhost:OTHERPORT/ws"); } catch {}
try {
  const wb = new Blob(['fetch("http://127.0.0.1:OTHERPORT/worker").catch(()=>{})'], {type: "application/javascript"});
  new Worker(URL.createObjectURL(wb));
} catch {}
const a = document.createElement("a");
a.href = "data:text/plain,hi";
a.download = "leak.txt";
document.body.appendChild(a);
a.click();
const img1 = new Image();
img1.src = "http://127.0.0.1:OTHERPORT/img1.png";
const img2 = new Image();
img2.src = "http://other.localhost:OTHERPORT/img2.png";
fetch("http://127.0.0.1:OTHERPORT/private", {mode: "no-cors"}).catch(() => {});
fetch("http://other.localhost:OTHERPORT/private", {mode: "no-cors"}).catch(() => {});
fetch("https://example.com/leak", {mode: "no-cors"}).catch(() => {});
</script>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body>
</html>`
		doc := strings.ReplaceAll(docTemplate, "OTHERPORT", otherPort)

		env := setupRecalcEnv(t, doc)
		runner := env.newRunner(t)

		sub := newSubscriber(env.f.AbsPath, laneLive)
		env.srv.hub.add(sub)

		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"7"}`) }()

		first := waitFrame(t, sub, 2*time.Second)
		data, _ := first["data"].(map[string]interface{})
		recalcField, _ := data["recalculate"].(map[string]interface{})
		if recalcField == nil {
			t.Fatalf("the disk frame carries no recalculate field: %v", data)
		}
		id, _ := recalcField["id"].(string)
		etag, _ := recalcField["etag"].(string)
		note := httptest.NewRequest("POST", "/_/recalc/"+env.f.Token,
			strings.NewReader(`{"id":"`+id+`","etag":"`+etag+`","status":"busy"}`))
		note.Host = env.srv.Host()
		sameOriginHeaders(note)
		nw := httptest.NewRecorder()
		env.srv.ServeHTTP(nw, note)
		if nw.Code != http.StatusNoContent {
			t.Fatalf("the busy note = %d, want 204: %s", nw.Code, nw.Body.String())
		}

		var w *httptest.ResponseRecorder
		select {
		case w = <-done:
		case <-time.After(25 * time.Second):
			t.Fatal("the waited write did not answer within 25s")
		}
		if w.Code != 200 {
			t.Fatalf("the waited write = %d %s", w.Code, w.Body.String())
		}
		var values map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &values); err != nil {
			t.Fatalf("answer is not JSON: %v (%q)", err, w.Body.String())
		}
		if values["qty"] != "7" || values["total"] != "70" {
			t.Fatalf("the waited write = %s, want qty 7 and total 70", w.Body.String())
		}
		meta := decodeWriteMeta(t, w)
		if meta["recalculated"] != true || meta["stale"] != false {
			t.Errorf("__meta__ = %v, want recalculated true and stale false", meta)
		}
		if got := w.Header().Get("Clay-Stale"); got != "" {
			t.Errorf("Clay-Stale = %q, want no header on a recalculated answer", got)
		}
		storedAfter := readDoc(t, env.f.AbsPath)
		if strings.Contains(storedAfter, string(dataapi.StaleMark)) {
			t.Error("the stale mark survived the hidden save")
		}
		if !strings.Contains(storedAfter, `data-printed="ok"`) {
			t.Error("window.print() did not return cleanly as a no-op")
		}
		if !strings.Contains(storedAfter, `data-popup-blocked="ok"`) {
			t.Error("window.open() was not blocked")
		}

		stats := runner.Stats()
		if stats.Dialogs < 3 {
			t.Errorf("Dialogs = %d, want at least 3", stats.Dialogs)
		}
		if stats.Downloads < 1 {
			t.Errorf("Downloads = %d, want at least 1", stats.Downloads)
		}
		if stats.Blocked < 2 {
			t.Errorf("Blocked = %d, want at least 2", stats.Blocked)
		}
		if hits := otherHits.Load(); hits != 0 {
			t.Errorf("the hidden page reached another origin %d times", hits)
		}

		second := waitFrame(t, sub, 2*time.Second)
		sdata, _ := second["data"].(map[string]interface{})
		html, _ := sdata["html"].(string)
		if !strings.Contains(html, ">70</b>") {
			t.Errorf("the edit tabs were not told about the hidden save: %v", second)
		}
	})

	t.Run("clay.effect calculator without sapjs recalculates", func(t *testing.T) {
		const doc = `<!DOCTYPE html>
<html>
<head><script src="https://clayjs.com/v1/clay.js"></script></head>
<body>
<input id="qty" value="1">
<output id="total">10</output>
<script>
clay.start(() => {
  const qty = document.getElementById("qty");
  const total = document.getElementById("total");
  clay.effect(() => { total.textContent = String(Number(qty.value || 0) * 10); });
});
</script>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body>
</html>`
		env := setupRecalcEnv(t, doc)
		env.newRunner(t)

		w := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"7"}`)
		if w.Code != 200 {
			t.Fatalf("the waited write = %d %s", w.Code, w.Body.String())
		}
		var values map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &values); err != nil {
			t.Fatalf("answer is not JSON: %v (%q)", err, w.Body.String())
		}
		if values["total"] != "70" {
			t.Fatalf("the waited write = %s, want total 70", w.Body.String())
		}
	})

	t.Run("concurrent edit before save returns save-refused without overwriting", func(t *testing.T) {
		const doc = `<!DOCTYPE html>
<html>
<head>
<script src="https://clayjs.com/v1/clay.js"></script>
<script src="https://clayjs.com/v1/sap.js"></script>
</head>
<body>
<main sap state="qty=1 price=10">
<input id="qty" bind="qty" value="1">
<b id="total" text="state.qty*state.price">10</b>
</main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body>
</html>`
		homeDir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		mgr := newTestManager(t, homeDir)
		store := versions.New(t.TempDir())
		logger := logging.NewStdout()
		ls := NewLiveSync(store, logger)
		t.Cleanup(ls.Shutdown)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		host := fmt.Sprintf("htmlclay1.localhost:%d", port)
		srv := NewHosted(host, func(string) bool { return true }, mgr, logger, store, ls)
		srv.uploadsDir = t.TempDir()
		t.Cleanup(func() { _ = srv.Close() })

		path := filepath.Join(homeDir, "invoice.htmlclay")
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := mgr.Register(path, session.ViaOsOpen); err != nil {
			t.Fatal(err)
		}

		// Simulate another editor modifying disk right after the hidden browser fetches the page,
		// so the hidden browser's conditional save carries the old If-Match and is refused (412).
		var raced atomic.Bool
		hs := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			srv.ServeHTTP(w, r)
			if r.Method == http.MethodGet && r.URL.Path == "/invoice.htmlclay" && raced.CompareAndSwap(false, true) {
				cur, rErr := os.ReadFile(path)
				if rErr == nil {
					_ = os.WriteFile(path, []byte(strings.Replace(string(cur), `value="7"`, `value="9"`, 1)), 0o644)
				}
			}
		})}
		go hs.Serve(ln)
		t.Cleanup(func() { _ = hs.Close() })

		runner := recalc.New(recalc.WithBrowser(requireTestBrowser(t)), recalc.WithIdleTimeout(10*time.Second), recalc.WithLogger(t.Logf))
		t.Cleanup(runner.Close)
		srv.SetRecalcRunner(runner)

		req := httptest.NewRequest("POST", "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", strings.NewReader(`{"qty":"7"}`))
		req.Host = srv.Host()
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		meta := decodeWriteMeta(t, w)
		if meta["code"] != "save-refused" {
			t.Fatalf("__meta__ code = %v, want save-refused (body %s)", meta["code"], w.Body.String())
		}
		if stored := readDoc(t, path); !strings.Contains(stored, `value="9"`) {
			t.Errorf("the concurrent edit was overwritten: %s", stored)
		}
	})

	t.Run("infinite loop times out and kills browser process", func(t *testing.T) {
		const warmDoc = `<!DOCTYPE html>
<html><head><script src="https://clayjs.com/v1/clay.js"></script><script src="https://clayjs.com/v1/sap.js"></script></head>
<body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="state.qty*state.price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`
		const doc = `<!DOCTYPE html>
<html><body>
<main sap state="qty=1"><input id="qty" bind="qty" value="1"><b id="total" text="state.qty">1</b></main>
<script>while (true) {}</script>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`
		env := setupRecalcEnv(t, warmDoc)
		runner := env.newRunner(t)

		// Warm the browser first so the 1500ms timeout hits while(true){}, not cold browser startup.
		if warm := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"2"}`); warm.Code != 200 || !strings.Contains(warm.Body.String(), `"total":"20"`) {
			t.Fatalf("warm write = %d %s", warm.Code, warm.Body.String())
		}
		if err := os.WriteFile(env.f.AbsPath, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}

		w := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=1500", `{"qty":"7"}`)
		meta := decodeWriteMeta(t, w)
		if meta["code"] != "timeout" {
			t.Fatalf("__meta__ code = %v, want timeout (body %s)", meta["code"], w.Body.String())
		}
		pid := runner.LastPID()
		if pid <= 0 {
			t.Fatal("no browser PID was recorded")
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && !processGone(pid) {
			time.Sleep(20 * time.Millisecond)
		}
		if !processGone(pid) {
			t.Errorf("browser PID %d is still alive after a timeout", pid)
		}
	})

	t.Run("Close kills browser process cleanly", func(t *testing.T) {
		const doc = `<!DOCTYPE html>
<html><head><script src="https://clayjs.com/v1/clay.js"></script></head>
<body>
<input id="qty" value="1">
<output id="total">10</output>
<script>
clay.start(() => {
  const qty = document.getElementById("qty");
  const total = document.getElementById("total");
  clay.effect(() => { total.textContent = String(Number(qty.value || 0) * 10); });
});
</script>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`
		env := setupRecalcEnv(t, doc)
		runner := env.newRunner(t)

		w := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"7"}`)
		if w.Code != 200 {
			t.Fatalf("the waited write = %d %s", w.Code, w.Body.String())
		}
		pid := runner.LastPID()
		if pid <= 0 {
			t.Fatal("no browser PID was recorded")
		}
		if processGone(pid) {
			t.Fatalf("browser PID %d died before Close() was called (warm browser was killed prematurely)", pid)
		}
		runner.Close()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && !processGone(pid) {
			time.Sleep(20 * time.Millisecond)
		}
		if !processGone(pid) {
			t.Errorf("browser PID %d is still alive after Close", pid)
		}
	})

	t.Run("warm browser reuses one process across consecutive writes", func(t *testing.T) {
		// Bug: self-cancellation on save or stale about:blank load kills the browser between writes.
		const doc = `<!DOCTYPE html>
<html>
<head>
<script src="https://clayjs.com/v1/clay.js"></script>
<script src="https://clayjs.com/v1/sap.js"></script>
</head>
<body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="state.qty*state.price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`
		env := setupRecalcEnv(t, doc)
		runner := env.newRunner(t)

		w1 := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"7"}`)
		if w1.Code != 200 || !strings.Contains(w1.Body.String(), `"total":"70"`) {
			t.Fatalf("write 1 = %d %s, want total 70", w1.Code, w1.Body.String())
		}
		pid1 := runner.LastPID()
		if pid1 <= 0 || processGone(pid1) {
			t.Fatalf("browser PID %d is not warm after write 1", pid1)
		}

		w2 := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta", `{"qty":"9"}`)
		if w2.Code != 200 || !strings.Contains(w2.Body.String(), `"total":"90"`) {
			t.Fatalf("write 2 = %d %s, want total 90", w2.Code, w2.Body.String())
		}
		pid2 := runner.LastPID()
		if pid2 != pid1 {
			t.Errorf("warm browser restarted across consecutive writes: pid1=%d pid2=%d", pid1, pid2)
		}
	})

	t.Run("idle timeout reaps the warm browser process", func(t *testing.T) {
		const doc = `<!DOCTYPE html>
<html>
<head>
<script src="https://clayjs.com/v1/clay.js"></script>
<script src="https://clayjs.com/v1/sap.js"></script>
</head>
<body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="state.qty*state.price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`
		env := setupRecalcEnv(t, doc)
		runner := recalc.New(recalc.WithBrowser(requireTestBrowser(t)), recalc.WithIdleTimeout(300*time.Millisecond), recalc.WithLogger(t.Logf))
		t.Cleanup(runner.Close)
		env.srv.SetRecalcRunner(runner)

		w := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"7"}`)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":"70"`) {
			t.Fatalf("write = %d %s, want total 70", w.Code, w.Body.String())
		}
		pid := runner.LastPID()
		if pid <= 0 || processGone(pid) {
			t.Fatalf("browser PID %d died before idle timeout", pid)
		}
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) && !processGone(pid) {
			time.Sleep(30 * time.Millisecond)
		}
		if !processGone(pid) {
			t.Errorf("browser PID %d is still alive after idle timeout", pid)
		}
	})

	t.Run("form oninput calculator that does not recompute on load keeps the stale mark", func(t *testing.T) {
		// Bug: hidden save stripped the stale mark and reported recalculated:true with the old total.
		const doc = `<!DOCTYPE html>
<html>
<head><script src="https://clayjs.com/v1/clay.js"></script></head>
<body>
<form oninput="total.value = Number(qty.value) * 10">
<input id="qty" name="qty" value="1">
<output id="total" name="total">10</output>
</form>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`
		env := setupRecalcEnv(t, doc)
		env.newRunner(t)

		w := env.postWrite(t, "/_/api/invoice.htmlclay?recalculate=wait&meta&timeout=15000", `{"qty":"7"}`)
		meta := decodeWriteMeta(t, w)
		if meta["recalculated"] != false || meta["stale"] != true || meta["code"] != "no-browser" {
			t.Fatalf("__meta__ = %v, want recalculated:false stale:true code:no-browser (body %s)", meta, w.Body.String())
		}
		if !strings.Contains(readDoc(t, env.f.AbsPath), string(dataapi.StaleMark)) {
			t.Error("the stale mark was cleared even though the page never recalculated")
		}
	})
}
