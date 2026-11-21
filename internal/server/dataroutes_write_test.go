package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/panphora/htmlclay/internal/wire"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
)

// dataroutes_write_test.go covers POST /_/api/<path>: the trust model (local processes in, browsers
// only with the target file's Save-Token), the engine's refusals surfaced as bodies, and the
// commit itself, which must be exactly a save's.

// dataWriteDoc is the test document: one rules tag named "api" over a heading, a list and a URL
// attribute.
const dataWriteDoc = `<!DOCTYPE html>
<html><head><script type="application/json" data-rules-name="api" data-rules-version="1">{title:"h1",items:"li[]",link:"a@href"}</script></head>
<body>
<h1>Hello</h1>
<ul><li>A</li><li>B</li></ul>
<a href="/x">x</a>
</body></html>
`

// postDataWrite drives the full stack, HostValidationMiddleware, the mux, then the handler, so the
// routing of the write face is exercised rather than assumed. The default headers are a local
// process's: no Origin and no Sec-Fetch-Site.
func postDataWrite(t *testing.T, srv *Server, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

type decodedWriteError struct {
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
	Details json.RawMessage `json:"details"`
}

func decodeWriteError(t *testing.T, w *httptest.ResponseRecorder) decodedWriteError {
	t.Helper()
	var body decodedWriteError
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON (%d): %s", w.Code, w.Body.String())
	}
	return body
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A local process writes through the page's own rules tag, content only, and the answer is the
// fresh extraction with the stamp of what landed on disk.
func TestDataWriteAppliesTheBodyThroughThePagesRulesTag(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"title":"World","items":["A","B"],"link":"/x"}` {
		t.Errorf("body = %s", got)
	}

	after := readDoc(t, path)
	if want := specwire.Etag([]byte(after)); w.Header().Get("ETag") != want {
		t.Errorf("ETag = %q, want the stamp of the stored bytes %q", w.Header().Get("ETag"), want)
	}

	// The splice promise, stated on the bytes rather than on the parse: everything outside the
	// changed element is bit-for-bit what it was, so a write cannot reformat a document it did not
	// mean to touch.
	start := strings.Index(dataWriteDoc, "<h1>") + len("<h1>")
	end := strings.Index(dataWriteDoc, "</h1>")
	prefix, suffix := dataWriteDoc[:start], dataWriteDoc[end:]
	if !strings.HasPrefix(after, prefix) {
		t.Errorf("the bytes before the heading changed:\n%q\n%q", prefix, after)
	}
	if !strings.HasSuffix(after, suffix) {
		t.Errorf("the bytes after the heading changed:\n%q\n%q", suffix, after)
	}
	if len(after) > len(prefix)+len(suffix) {
		if mid := after[len(prefix) : len(after)-len(suffix)]; mid != "World" {
			t.Errorf("inside the heading = %q, want %q", mid, "World")
		}
	}
}

// A body that changes nothing writes nothing: no bytes, no mtime, no backup.
func TestDataWriteNoOpLeavesTheFileAlone(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"Hello"}`, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got := readDoc(t, path); got != dataWriteDoc {
		t.Errorf("the file changed:\n%s", got)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("mtime moved from %v to %v", before.ModTime(), after.ModTime())
	}
}

// A page from another origin, and a page on another port of this same host, is refused before any
// path work happens at all.
func TestDataWriteRefusesOtherSites(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		map[string]string{"Sec-Fetch-Site": "same-site"})
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
	if got := readDoc(t, path); got != dataWriteDoc {
		t.Errorf("a refused write changed the file:\n%s", got)
	}
}

// The rule SECURITY.md states: a silent background fetch() from a page in a trusted folder cannot
// write a sibling, because the write needs the TARGET file's own Save-Token.
func TestDataWriteBrowserNeedsTheTargetFilesSaveToken(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	otherPath := writeHome(t, srv, "other.htmlclay", dataWriteDoc)
	other, err := srv.sessions.Register(otherPath, session.ViaOsOpen)
	if err != nil {
		t.Fatal(err)
	}

	browser := func(extra map[string]string) map[string]string {
		headers := map[string]string{
			"Sec-Fetch-Site": "same-origin",
			"Origin":         fmt.Sprintf("http://127.0.0.1:%d", srv.port),
		}
		for k, v := range extra {
			headers[k] = v
		}
		return headers
	}

	noToken := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, browser(nil))
	if noToken.Code != 403 {
		t.Fatalf("no token: status = %d, want 403: %s", noToken.Code, noToken.Body.String())
	}
	if got := decodeWriteError(t, noToken).Error; got != "Save-Token required" {
		t.Errorf("no token: error = %q", got)
	}

	withToken := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		browser(map[string]string{wire.TokenHeader: f.Token}))
	if withToken.Code != 200 {
		t.Fatalf("the file's own token: status = %d, want 200: %s", withToken.Code, withToken.Body.String())
	}

	wrongToken := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"Again"}`,
		browser(map[string]string{wire.TokenHeader: other.Token}))
	if wrongToken.Code != 403 {
		t.Fatalf("a sibling's token: status = %d, want 403: %s", wrongToken.Code, wrongToken.Body.String())
	}
	if got := readDoc(t, path); !strings.Contains(got, "<h1>World</h1>") {
		t.Errorf("the refused write changed the file:\n%s", got)
	}
}

func TestDataWriteRequiresJSON(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if w.Code != 415 {
		t.Fatalf("status = %d, want 415: %s", w.Code, w.Body.String())
	}
	if got := decodeWriteError(t, w).Error; got != "Unsupported Media Type" {
		t.Errorf("error = %q", got)
	}
}

func TestDataWriteInvalidJSON(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{title:"World"}`, nil)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if got := decodeWriteError(t, w).Error; got != "Invalid JSON body" {
		t.Errorf("error = %q", got)
	}
}

// The engine's refusals reach the caller structured, not as a sentence: a caller that sent a key
// the rules do not define is told which one.
func TestDataWriteRejectedCarriesTheUnknownKeys(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"nmae":"x"}`, nil)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Error != "Write rejected" {
		t.Fatalf("error = %q", body.Error)
	}
	var details struct {
		UnknownKeys []string `json:"unknownKeys"`
	}
	if err := json.Unmarshal(body.Details, &details); err != nil {
		t.Fatalf("details is not the structured refusal: %s (%v)", body.Details, err)
	}
	if len(details.UnknownKeys) != 1 || details.UnknownKeys[0] != "nmae" {
		t.Errorf("unknownKeys = %v, want [nmae]", details.UnknownKeys)
	}
	if got := readDoc(t, path); got != dataWriteDoc {
		t.Errorf("a rejected write changed the file:\n%s", got)
	}
}

// Content only: a URL the policy refuses is refused whole, and nothing lands on disk.
func TestDataWriteRefusedLeavesTheFileAlone(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"link":"javascript:alert(1)"}`, nil)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if got := decodeWriteError(t, w).Error; got != "Write refused" {
		t.Errorf("error = %q", got)
	}
	if got := readDoc(t, path); got != dataWriteDoc {
		t.Errorf("a refused write changed the file:\n%s", got)
	}
}

// A write may carry the stamp of the bytes it read, and then it is judged against them before
// anything is versioned or written.
func TestDataWriteIfMatch(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	stale := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		map[string]string{"If-Match": `"wrong"`})
	if stale.Code != 412 {
		t.Fatalf("status = %d, want 412: %s", stale.Code, stale.Body.String())
	}
	if want := specwire.Etag([]byte(dataWriteDoc)); stale.Header().Get("ETag") != want {
		t.Errorf("412 ETag = %q, want the current stamp %q", stale.Header().Get("ETag"), want)
	}
	if got := readDoc(t, path); got != dataWriteDoc {
		t.Fatalf("a refused conditional write changed the file:\n%s", got)
	}

	// The stamp the read face hands out is exactly the one the write face accepts.
	etag := getAPI(t, srv, "test.htmlclay").Header().Get("ETag")
	if etag == "" {
		t.Fatal("GET /_/api/<file> carried no ETag")
	}
	fresh := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		map[string]string{"If-Match": etag})
	if fresh.Code != 200 {
		t.Fatalf("If-Match from the GET: status = %d, want 200: %s", fresh.Code, fresh.Body.String())
	}
}

// The commit is a save's commit, which is what makes the write recoverable: the previous content is
// in history before the file is replaced.
func TestDataWriteVersionsThePreviousContent(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	if w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, nil); w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	f.Lock()
	key := f.HistoryKey()
	f.Unlock()
	if key == "" {
		t.Fatal("the write resolved no history key")
	}
	entries, err := srv.versions.List(key, f.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("a changed write left no version behind")
	}
	// Both states, not just one: the pre-write content is what a caller would
	// want back, and the written content is what the receipt promises.
	before, after := false, false
	for _, e := range entries {
		body, err := srv.versions.Read(key, f.AbsPath, e.Name)
		if err != nil {
			t.Fatal(err)
		}
		before = before || strings.Contains(string(body), "<h1>Hello</h1>")
		after = after || strings.Contains(string(body), "<h1>World</h1>")
	}
	if !before || !after {
		t.Errorf("history holds the old content: %v, the written content: %v; want both", before, after)
	}
}

// The refusal for a path this route may not write is decided on text alone: it reads the same
// whether or not anything is at that path, so the route cannot be used to enumerate the home tree.
func TestDataWriteUnregisteredIsNotAnExistenceOracle(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "loose.htmlclay", dataWriteDoc)

	present := postDataWrite(t, srv, "/_/api/loose.htmlclay", `{"title":"World"}`, nil)
	absent := postDataWrite(t, srv, "/_/api/absent.htmlclay", `{"title":"World"}`, nil)

	for name, w := range map[string]*httptest.ResponseRecorder{"present": present, "absent": absent} {
		if w.Code != 403 {
			t.Fatalf("%s: status = %d, want 403: %s", name, w.Code, w.Body.String())
		}
		if got := decodeWriteError(t, w).Error; got != "Not writable" {
			t.Errorf("%s: error = %q", name, got)
		}
	}
	if present.Body.String() != absent.Body.String() {
		t.Errorf("the refusals differ:\npresent %s\nabsent  %s", present.Body.String(), absent.Body.String())
	}
	if got := readDoc(t, filepath.Join(srv.sessions.HomeDir(), "loose.htmlclay")); got != dataWriteDoc {
		t.Errorf("a refused write changed the file:\n%s", got)
	}
}

// The write face owns the same address as the read face, including the bare /_/api that ServeMux
// would otherwise 307-redirect, and answers its own path errors before any filesystem access.
func TestDataWriteRouting(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	cases := []struct {
		target, wantError string
		wantStatus        int
	}{
		{"/_/api", "Missing path", 400},
		{"/_/api/", "Missing path", 400},
		{"/_/api/style.css", "Unsupported file type", 404},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			w := postDataWrite(t, srv, c.target, `{"title":"World"}`, nil)
			if w.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, c.wantStatus, w.Body.String())
			}
			if got := decodeWriteError(t, w).Error; got != c.wantError {
				t.Errorf("error = %q, want %q", got, c.wantError)
			}
		})
	}
}

// A trusted folder's file registers on demand, through the same seam a navigation uses, so a local
// script can write a document the user never opened.
func TestDataWriteRegistersATrustedFolderFile(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	ws := filepath.Join(srv.sessions.HomeDir(), "ws")
	page := writeHome(t, srv, "ws/note.htmlclay", dataWriteDoc)

	srv.SetHooks(Hooks{
		TrustedCovers: func(absPath string) bool { return session.EqualOrUnder(absPath, ws) },
		Route: func(absPath string) (string, bool) {
			if _, err := srv.sessions.Register(absPath, session.ViaTrusted); err != nil {
				return "", false
			}
			return fmt.Sprintf("http://127.0.0.1:%d/", srv.port), true
		},
	})
	if err := srv.sessions.InstallTrustedRoot(ws); err != nil {
		t.Fatal(err)
	}

	w := postDataWrite(t, srv, "/_/api/ws/note.htmlclay", `{"title":"World"}`, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if _, registered := srv.sessions.LookupByPath(page); !registered {
		t.Error("the write did not register the file")
	}
	if got := readDoc(t, page); !strings.Contains(got, "<h1>World</h1>") {
		t.Errorf("the write did not land:\n%s", got)
	}
}

// A page can only write a file it already holds a token for, so the write face never registers on
// a browser's behalf and never answers differently for a trusted file than for a missing one: the
// three leaks the review found all needed the resolvable-path answer that only a local process gets.
func TestDataWriteBrowserCannotRegisterOrProbe(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	ws := filepath.Join(srv.sessions.HomeDir(), "ws")
	page := writeHome(t, srv, "ws/note.htmlclay", dataWriteDoc)

	routes := 0
	srv.SetHooks(Hooks{
		TrustedCovers: func(absPath string) bool { return session.EqualOrUnder(absPath, ws) },
		Route: func(absPath string) (string, bool) {
			routes++
			if _, err := srv.sessions.Register(absPath, session.ViaTrusted); err != nil {
				return "", false
			}
			return fmt.Sprintf("http://127.0.0.1:%d/", srv.port), true
		},
	})
	if err := srv.sessions.InstallTrustedRoot(ws); err != nil {
		t.Fatal(err)
	}

	browser := func() map[string]string {
		return map[string]string{
			"Sec-Fetch-Site": "same-origin",
			"Origin":         fmt.Sprintf("http://127.0.0.1:%d", srv.port),
		}
	}

	present := postDataWrite(t, srv, "/_/api/ws/note.htmlclay", `{"title":"World"}`, browser())
	absent := postDataWrite(t, srv, "/_/api/ws/absent.htmlclay", `{"title":"World"}`, browser())

	for name, w := range map[string]*httptest.ResponseRecorder{"present": present, "absent": absent} {
		if w.Code != 403 {
			t.Fatalf("%s: status = %d, want 403: %s", name, w.Code, w.Body.String())
		}
		if got := decodeWriteError(t, w).Error; got != "Save-Token required" {
			t.Errorf("%s: error = %q, want %q", name, got, "Save-Token required")
		}
	}
	if present.Body.String() != absent.Body.String() {
		t.Errorf("the refusals differ:\npresent %s\nabsent  %s", present.Body.String(), absent.Body.String())
	}
	if routes != 0 {
		t.Errorf("Route was called %d time(s) for a browser caller, want 0", routes)
	}
	if _, registered := srv.sessions.LookupByPath(page); registered {
		t.Error("a browser write registered a trusted-folder file")
	}
	if got := readDoc(t, page); got != dataWriteDoc {
		t.Errorf("a refused write changed the file:\n%s", got)
	}
}

// The existence-oracle check runs again with the trusted-folder hooks wired, which is the only
// state in which the old path resolution could have answered a local process differently for a
// present and an absent file.
func TestDataWriteUnregisteredOutsideTrustedFoldersIsNotAnExistenceOracle(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	ws := filepath.Join(srv.sessions.HomeDir(), "ws")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	writeHome(t, srv, "loose.htmlclay", dataWriteDoc)

	srv.SetHooks(Hooks{
		TrustedCovers: func(absPath string) bool { return session.EqualOrUnder(absPath, ws) },
		Route: func(absPath string) (string, bool) {
			if _, err := srv.sessions.Register(absPath, session.ViaTrusted); err != nil {
				return "", false
			}
			return fmt.Sprintf("http://127.0.0.1:%d/", srv.port), true
		},
	})
	if err := srv.sessions.InstallTrustedRoot(ws); err != nil {
		t.Fatal(err)
	}

	present := postDataWrite(t, srv, "/_/api/loose.htmlclay", `{"title":"World"}`, nil)
	absent := postDataWrite(t, srv, "/_/api/absent.htmlclay", `{"title":"World"}`, nil)

	for name, w := range map[string]*httptest.ResponseRecorder{"present": present, "absent": absent} {
		if w.Code != 403 {
			t.Fatalf("%s: status = %d, want 403: %s", name, w.Code, w.Body.String())
		}
		if got := decodeWriteError(t, w).Error; got != "Not writable" {
			t.Errorf("%s: error = %q", name, got)
		}
	}
	if present.Body.String() != absent.Body.String() {
		t.Errorf("the refusals differ:\npresent %s\nabsent  %s", present.Body.String(), absent.Body.String())
	}
}

// A rule that names a DOM property the engine refuses to write is the caller's mistake, so the
// write face answers 400 with the engine's own words rather than the extraction face's 500.
func TestDataWriteReadOnlyRule(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", `<!DOCTYPE html>
<html><head><script type="application/json" data-rules-name="api" data-rules-version="1">{t:"h1",tag:"h1@tagName"}</script></head>
<body><h1>Hello</h1></body></html>
`)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"tag":"H2"}`, nil)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Error != "Read-only rule" {
		t.Errorf("error = %q, want %q", body.Error, "Read-only rule")
	}
	if body.Message != `cannot write to read-only DOM property "tagName"` {
		t.Errorf("message = %q", body.Message)
	}
}

// Both GET faces answer the stamp of the stored bytes, which is what a client holds to write
// conditionally.
func TestDataReadFacesCarryTheETag(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)
	want := specwire.Etag([]byte(readDoc(t, path)))

	if got := getAPI(t, srv, "test.htmlclay").Header().Get("ETag"); got != want {
		t.Errorf("/_/api ETag = %q, want %q", got, want)
	}
	query := get(t, srv, `/test.htmlclay?data={title:"h1"}`, "test.htmlclay")
	if query.Code != 200 {
		t.Fatalf("?data= status = %d: %s", query.Code, query.Body.String())
	}
	if got := query.Header().Get("ETag"); got != want {
		t.Errorf("?data= ETag = %q, want %q", got, want)
	}
}

// A data write has no browser tab behind it to relay the snapshot to the other editors, so without
// an announcement an open edit-mode tab learns of it only on its next save, as a 412. It is
// announced exactly as an external edit is: the content rides the live lane's notification, the
// disk HTML follows on the saved lane, and the re-anchoring acceptServerReplacement performs before
// the publish keeps the incarnation's generation from rolling and throwing the tab's replay away.
func TestDataWriteReachesEditTabsOnTheLiveLane(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	live := newSubscriber(f.AbsPath, laneLive)
	saved := newSubscriber(f.AbsPath, laneSaved)
	srv.hub.add(live)
	srv.hub.add(saved)
	before := srv.hub.incs[f.AbsPath].generation

	if w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, nil); w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	f.Lock()
	key := f.HistoryKey()
	f.Unlock()
	stored := []byte(readDoc(t, path))
	want := forBrowser(stored, key, srv.documentURLFor(f))

	notice := waitFrame(t, live, time.Second)
	if notice["type"] != "notification" {
		t.Fatalf("live lane got %v, want a notification", notice)
	}
	data, ok := notice["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("notification carries no data object: %v", notice)
	}
	if data["kind"] != "external-change" {
		t.Errorf("kind = %v, want external-change", data["kind"])
	}
	if data["html"] != want {
		t.Errorf("html = %q, want the browser-facing bytes %q", data["html"], want)
	}
	if data["etag"] != specwire.Etag(stored) {
		t.Errorf("etag = %v, want the stamp of the disk bytes %q", data["etag"], specwire.Etag(stored))
	}

	content := waitFrame(t, saved, time.Second)
	if content["html"] != want || content["sender"] != "file-system" {
		t.Errorf("saved lane payload = %v, want the stored bytes from the file-system", content)
	}

	if got := srv.hub.incs[f.AbsPath].generation; got != before {
		t.Errorf("an API write rolled the generation from %d to %d", before, got)
	}
}

// An API write's bytes are this host's, but they are not an open tab's. A tab still holding the
// stamp from before it must be refused, and told nothing about who moved the document, because
// `another-tab` here would be a confident wrong answer about an agent's write.
func TestDataWriteConflictIsNotAnotherTab(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)
	stamp := specwire.Etag([]byte(dataWriteDoc))

	if w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, nil); w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	w := saveThroughMux(t, srv, f, dataWriteDoc, stamp)
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("a save holding the pre-write stamp = %d, want 412: %s", w.Code, w.Body.String())
	}
	if got, present := decode(t, w)["changedBy"]; present {
		t.Errorf("changedBy = %v, but the write came through the data API, not another tab", got)
	}
}

// The write is announced once. RecordServerWrite keeps the watcher's suppression in step, so its
// next look at the same bytes confirms a change already delivered rather than publishing a second
// external change for the tab to apply again.
func TestDataWriteWatcherStaysQuiet(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)

	if w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, nil); w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if notice := waitFrame(t, live, time.Second); notice["type"] != "notification" {
		t.Fatalf("live lane got %v, want the write's notification", notice)
	}

	// Hand-driven and hand-aged, as the watcher's own tests do: the first look arms the candidate,
	// the second is the one that would publish.
	wt := srv.watcher
	wt.quiet = 0
	e := &watchEntry{file: f, refs: 1}
	wt.check(e)
	wt.check(e)

	expectNoFrame(t, live, 100*time.Millisecond)
}

// A program that proved it read the version it is replacing gets its write refused
// when the bytes it replaces cannot be kept: nothing else holds them, so writing
// would lose the only copy. The file is left exactly as it was.
func TestDataWriteProvenReaderRefusedWhenTheOldVersionCannotBeKept(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)
	srv.backupHook = func() error { return errors.New("history is unavailable") }

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		map[string]string{"If-Match": specwire.Etag([]byte(dataWriteDoc))})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Code != "recovery-unavailable" || body.Error != "Could not keep the current version" {
		t.Fatalf("refusal body = %+v", body)
	}
	if want := "The current version could not be stored in history, so nothing was written. Try again."; body.Message != want {
		t.Fatalf("message = %q, want %q", body.Message, want)
	}
	if got := readDoc(t, path); got != dataWriteDoc {
		t.Fatalf("a refused write changed the file:\n%s", got)
	}
}

// The same failure on a write that showed no proof of reading keeps today's
// behaviour: the backup is logged and the write happens anyway.
func TestDataWriteWithoutProofStillWritesWhenTheOldVersionCannotBeKept(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)
	srv.backupHook = func() error { return errors.New("history is unavailable") }

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := readDoc(t, path); !strings.Contains(got, "<h1>World</h1>") {
		t.Fatalf("the write did not land:\n%s", got)
	}
}

// Bytes a recent version already holds need no second copy, so a proven reader is
// never refused for a version that is already kept.
func TestDataWriteProvenReaderNeedsNoNewCopyOfKeptBytes(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	if w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"World"}`,
		map[string]string{"If-Match": specwire.Etag([]byte(dataWriteDoc))}); w.Code != 200 {
		t.Fatalf("first write: status = %d: %s", w.Code, w.Body.String())
	}
	srv.backupHook = func() error { return errors.New("history is unavailable") }

	kept := readDoc(t, mustDocPath(t, srv, "test.htmlclay"))
	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"title":"Again"}`,
		map[string]string{"If-Match": specwire.Etag([]byte(kept))})
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

// mustDocPath returns the path writeHome gave a document.
func mustDocPath(t *testing.T, srv *Server, name string) string {
	t.Helper()
	path := filepath.Join(srv.sessions.HomeDir(), name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s is not there: %v", name, err)
	}
	return path
}

// A program that removes data without naming the version it read is refused: it has not read the
// data it would take, so the answer is a 428 it can fix by reading first, and nothing is written.
func TestDataWriteRefusesAProgramRemovalWithoutAVersion(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	servedETag := getAPI(t, srv, "test.htmlclay").Header().Get("ETag")
	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"items":["A"]}`, nil)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, want 428: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Code != "read-required" {
		t.Errorf("code = %q, want read-required (body %s)", body.Code, w.Body.String())
	}
	var full struct {
		Etag        string `json:"etag"`
		WouldRemove struct {
			Items  int      `json:"items"`
			Values int      `json:"values"`
			Paths  []string `json:"paths"`
		} `json:"wouldRemove"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &full); err != nil {
		t.Fatalf("refusal body is not JSON: %v", err)
	}
	if full.Etag != servedETag {
		t.Errorf("etag = %q, want the served etag %q", full.Etag, servedETag)
	}
	if len(full.WouldRemove.Paths) == 0 || full.WouldRemove.Items+full.WouldRemove.Values == 0 {
		t.Errorf("wouldRemove = %+v, want what the write would have taken", full.WouldRemove)
	}
	assertUnchanged(t, path, dataWriteDoc)
}

// The same write goes through once the program names the version it read.
func TestDataWriteProgramRemovalWithTheVersionItRead(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	etag := getAPI(t, srv, "test.htmlclay").Header().Get("ETag")
	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"items":["A"]}`, map[string]string{"If-Match": etag})
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	after := readDoc(t, path)
	if !strings.Contains(after, "<li>A</li></ul>") {
		t.Errorf("the item was not removed:\n%s", after)
	}
}

// Adding a value removes nothing, so a program needs no version for it.
func TestDataWriteProgramAdditionNeedsNoVersion(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "test.htmlclay", dataWriteDoc)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"items":["A","B","C"]}`, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if after := readDoc(t, path); !strings.Contains(after, "<li>C</li>") {
		t.Errorf("the item was not added:\n%s", after)
	}
}

// A page is never refused this way: the person's own tabs keep the guard, and their write lands.
func TestDataWritePageRemovalNeedsNoVersion(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	f, path := registerFixture(t, srv, "test.htmlclay", dataWriteDoc)
	if w := serve(t, srv, "/fixtures/test.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve = %d", w.Code)
	}

	headers := map[string]string{
		"Sec-Fetch-Site": "same-origin",
		"Origin":         fmt.Sprintf("http://127.0.0.1:%d", srv.port),
		wire.TokenHeader: f.Token,
	}
	w := postDataWrite(t, srv, "/_/api/fixtures/test.htmlclay", `{"items":["A"]}`, headers)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if after := readDoc(t, path); !strings.Contains(after, "<li>A</li></ul>") {
		t.Errorf("the page's removal was not written:\n%s", after)
	}
}

// A sheet patch that clears a cell from a program with no version removes a value, so it is
// refused the same way.
func TestDataWriteSheetPatchClearWithoutAVersionIsRefused(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	_, path := registerFixture(t, srv, "sheet.htmlclay", sheetGuardSource)
	if w := serve(t, srv, "/fixtures/sheet.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve = %d", w.Code)
	}

	w := postDataWrite(t, srv, "/_/api/fixtures/sheet.htmlclay", `{"patch":[{"range":"B1","values":[[""]]}]}`, nil)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, want 428: %s", w.Code, w.Body.String())
	}
	if body := decodeWriteError(t, w); body.Code != "read-required" {
		t.Errorf("code = %q, want read-required (body %s)", body.Code, w.Body.String())
	}
	assertUnchanged(t, path, sheetGuardSource)
}

// A rules write whose rules read a value a page's script computes answers that the value is old:
// HTML Clay runs no page script, so after {"qty":"7"} the invoice's total still reads 10. The
// answer says so in a last __meta__ key and a Clay-Stale header, and the file it wrote carries one
// stale mark so a later read can say the same. A page with a script that computes nothing the
// rules read answers plain data and gets no mark.
func TestDataWriteSaysWhenComputedValuesAreOld(t *testing.T) {
	const why = "This write updated inputs and left computed values unchanged. Write with ?recalculate=wait for fresh values, or open the document."
	srv, _, _ := setupHandlerTest(t)
	invoice := writeHome(t, srv, "invoice.htmlclay", `<!DOCTYPE html><html><body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="qty*price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`)
	notes := writeHome(t, srv, "notes.htmlclay", `<!DOCTYPE html><html><body>
<h1>Notes</h1><div editor-ui><button id="b">Bold</button></div>
<script>document.getElementById("b").textContent = "B"</script>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1"}</script>
</body></html>`)
	ownMeta := writeHome(t, srv, "own-meta.htmlclay", `<!DOCTYPE html><html><body>
<output id="sum">3</output><p id="m">mine</p>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"sum":"#sum","__meta__":"#m"}</script>
</body></html>`)
	totals := writeHome(t, srv, "totals.htmlclay", `<!DOCTYPE html><html><body>
<output>12</output>
<script type="application/json" data-rules-name="api" data-rules-version="1">"output[]"</script>
</body></html>`)
	for _, p := range []string{invoice, notes, ownMeta, totals} {
		if _, err := srv.sessions.Register(p, session.ViaOsOpen); err != nil {
			t.Fatal(err)
		}
	}

	w := postDataWrite(t, srv, "/_/api/invoice.htmlclay", `{"qty":"7"}`, nil)
	stale := `{"qty":"7","total":"10","__meta__":{"stale":true,"why":"` + why + `"}}`
	if w.Code != 200 || w.Body.String() != stale {
		t.Fatalf("invoice: %d %s\nwant %s", w.Code, w.Body.String(), stale)
	}
	if got := w.Header().Get("Clay-Stale"); got != "true" {
		t.Errorf("invoice: Clay-Stale = %q, want true", got)
	}

	// The agent posts the whole answer back: __meta__ is not a rule, so it is dropped, not refused,
	// and never reaches the file.
	w = postDataWrite(t, srv, "/_/api/invoice.htmlclay", `{"qty":"8","total":"10","__meta__":{"stale":true,"why":"x"}}`, nil)
	if w.Code != 200 {
		t.Fatalf("posting the answer back: %d %s", w.Code, w.Body.String())
	}
	if doc := readDoc(t, invoice); strings.Contains(doc, "__meta__") || strings.Count(doc, string(dataapi.StaleMark)) != 1 || strings.Contains(doc, `"stale"`) || !strings.Contains(doc, `value="8"`) {
		t.Errorf("invoice file after the post back:\n%s", doc)
	}

	w = postDataWrite(t, srv, "/_/api/notes.htmlclay", `{"title":"Monday"}`, nil)
	if w.Code != 200 || w.Body.String() != `{"title":"Monday"}` || w.Header().Get("Clay-Stale") != "" {
		t.Errorf("notes: %d %q Clay-Stale=%q, want plain data", w.Code, w.Body.String(), w.Header().Get("Clay-Stale"))
	}
	if strings.Contains(readDoc(t, notes), "clay-stale") {
		t.Errorf("notes: a plain document got the stale mark")
	}

	// A query string write keeps its old answer: no key, no header.
	w = postDataWrite(t, srv, "/_/api/invoice.htmlclay?data="+url.QueryEscape(`{"qty":"#qty@value","total":"#total"}`), `{"qty":"9"}`, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "__meta__") || w.Header().Get("Clay-Stale") != "" {
		t.Errorf("?data= write: %d %s Clay-Stale=%q, want the old answer", w.Code, w.Body.String(), w.Header().Get("Clay-Stale"))
	}

	// Rules that define __meta__ own the key: the document's value stays, and the header alone
	// says the values are old.
	w = postDataWrite(t, srv, "/_/api/own-meta.htmlclay", `{"__meta__":"theirs"}`, nil)
	if w.Code != 200 || w.Body.String() != `{"sum":"3","__meta__":"theirs"}` || w.Header().Get("Clay-Stale") != "true" {
		t.Errorf("rules-defined __meta__: %d %s Clay-Stale=%q", w.Code, w.Body.String(), w.Header().Get("Clay-Stale"))
	}

	// ?meta asks for the whole key, stale or not.
	w = postDataWrite(t, srv, "/_/api/notes.htmlclay?meta", `{"title":"Tuesday"}`, nil)
	etag := specwire.Etag([]byte(readDoc(t, notes)))
	full := `{"title":"Tuesday","__meta__":{"etag":` + mustJSON(t, etag) + `,"changed":true,"recalculated":false,"stale":false}}`
	if w.Body.String() != full {
		t.Errorf("notes ?meta: %s\nwant %s", w.Body.String(), full)
	}

	// A stale ?meta answer carries everything, why last.
	w = postDataWrite(t, srv, "/_/api/invoice.htmlclay?meta", `{"qty":"7"}`, nil)
	etag = specwire.Etag([]byte(readDoc(t, invoice)))
	full = `{"qty":"7","total":"10","__meta__":{"etag":` + mustJSON(t, etag) + `,"changed":true,"recalculated":false,"stale":true,"why":"` + why + `"}}`
	if w.Body.String() != full {
		t.Errorf("invoice ?meta: %s\nwant %s", w.Body.String(), full)
	}

	// Data that is a list has no place for the key: the header alone says the values are old.
	w = postDataWrite(t, srv, "/_/api/totals.htmlclay", `["6"]`, map[string]string{"If-Match": specwire.Etag([]byte(readDoc(t, totals)))})
	if w.Code != 200 || strings.Contains(w.Body.String(), "__meta__") || w.Header().Get("Clay-Stale") != "true" {
		t.Errorf("list data: %d %s Clay-Stale=%q, want the header only", w.Code, w.Body.String(), w.Header().Get("Clay-Stale"))
	}
}

// A read of a file a stale write left the mark in answers that its computed values are old, and
// keeps saying so with ?meta too. The bug it guards is a read that hides old computed values: an
// agent that writes and then reads would get 10 with nothing saying 10 is not 7*10.
func TestDataReadSaysWhenComputedValuesAreOld(t *testing.T) {
	const why = "No page has recalculated since the last data write, so computed values may be old. Open the document to refresh them."
	srv, _, _ := setupHandlerTest(t)
	invoice := writeHome(t, srv, "invoice.htmlclay", `<!DOCTYPE html><html><body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="qty*price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`)
	if _, err := srv.sessions.Register(invoice, session.ViaOsOpen); err != nil {
		t.Fatal(err)
	}

	if w := postDataWrite(t, srv, "/_/api/invoice.htmlclay", `{"qty":"7"}`, nil); w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body.String())
	}

	w := getAPI(t, srv, "invoice.htmlclay")
	want := `{"qty":"7","total":"10","__meta__":{"stale":true,"why":"` + why + `"}}`
	if w.Code != 200 || w.Body.String() != want {
		t.Fatalf("read: %d %s\nwant %s", w.Code, w.Body.String(), want)
	}
	if got := w.Header().Get("Clay-Stale"); got != "true" {
		t.Errorf("read: Clay-Stale = %q, want true", got)
	}

	w = getAPIQuery(t, srv, "invoice.htmlclay", "meta")
	if w.Code != 200 {
		t.Fatalf("read ?meta: %d %s", w.Code, w.Body.String())
	}
	var full struct {
		Qty  string `json:"qty"`
		Meta struct {
			Etag  string `json:"etag"`
			Stale bool   `json:"stale"`
			Why   string `json:"why"`
		} `json:"__meta__"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &full); err != nil {
		t.Fatalf("read ?meta body is not JSON: %v: %s", err, w.Body.String())
	}
	if full.Meta.Etag != w.Header().Get("ETag") || full.Meta.Etag == "" {
		t.Errorf("read ?meta: etag = %q, want the ETag header %q", full.Meta.Etag, w.Header().Get("ETag"))
	}
	if !full.Meta.Stale || full.Meta.Why != why {
		t.Errorf("read ?meta: %s, want stale true and the why sentence", w.Body.String())
	}

	// The first page that recalculates and saves strips the mark, so the file alone decides.
	fresh := strings.Replace(readDoc(t, invoice), string(dataapi.StaleMark), "", 1)
	if err := os.WriteFile(invoice, []byte(fresh), 0644); err != nil {
		t.Fatal(err)
	}
	w = getAPI(t, srv, "invoice.htmlclay")
	if w.Body.String() != `{"qty":"7","total":"10"}` || w.Header().Get("Clay-Stale") != "" {
		t.Errorf("read after the mark is gone: %s Clay-Stale=%q, want plain data", w.Body.String(), w.Header().Get("Clay-Stale"))
	}
}

// A data write with ?recalculate=wait names itself in the disk frame it sends open edit tabs and
// waits for one to save a version built on it, so an agent gets fresh computed values from one
// call instead of the bytes the write itself produced.
func TestDataWriteWaitsForAnOpenPage(t *testing.T) {
	const doc = `<!DOCTYPE html><html><body>
<main sap state="qty=1 price=10"><input id="qty" bind="qty" value="1"><b id="total" text="qty*price">10</b></main>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"qty":"#qty@value","total":"#total"}</script>
</body></html>`

	// setup writes and registers the invoice document, exactly as the M1a test does.
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

	// startWait runs the waited write in the background and returns the id and etag it named
	// itself with in its disk frame, plus the channel its answer arrives on.
	startWait := func(t *testing.T, srv *Server, sub *subscriber, target string) (string, string, chan *httptest.ResponseRecorder) {
		t.Helper()
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- postDataWrite(t, srv, target, `{"qty":"7"}`, nil) }()
		data, _ := waitFrame(t, sub, 2*time.Second)["data"].(map[string]interface{})
		recalc, _ := data["recalculate"].(map[string]interface{})
		if recalc == nil {
			t.Fatalf("the disk frame carries no recalculate field: %v", data)
		}
		id, _ := recalc["id"].(string)
		etag, _ := recalc["etag"].(string)
		if id == "" || etag == "" {
			t.Fatalf("recalculate = %v, want an id and an etag", recalc)
		}
		return id, etag, done
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

	t.Run("a save built on the written version answers fresh values", func(t *testing.T) {
		// Bug: the save does not wake the write, or the write answers the bytes it wrote.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		_, etag, done := startWait(t, srv, sub, "/_/api/invoice.htmlclay?recalculate=wait")

		recalculated := strings.Replace(readDoc(t, f.AbsPath), string(dataapi.StaleMark), "", 1)
		recalculated = strings.Replace(recalculated, ">10</b>", ">70</b>", 1)
		if w := saveThroughMux(t, srv, f, recalculated, etag); w.Code != 200 {
			t.Fatalf("the recalculating save = %d, want 200: %s", w.Code, w.Body.String())
		}

		w := recv(t, done)
		if w.Code != 200 || w.Body.String() != `{"qty":"7","total":"70"}` {
			t.Errorf("the waited write = %d %s, want the saved file's fresh values", w.Code, w.Body.String())
		}
		if got := w.Header().Get("Clay-Stale"); got != "" {
			t.Errorf("Clay-Stale = %q, want no header on a recalculated answer", got)
		}
	})

	t.Run("with ?meta the answer says it recalculated", func(t *testing.T) {
		// Bug: a waited write never claims it recalculated, or stamps the answer with the wrong etag.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		_, etag, done := startWait(t, srv, sub, "/_/api/invoice.htmlclay?recalculate=wait&meta")

		recalculated := strings.Replace(readDoc(t, f.AbsPath), string(dataapi.StaleMark), "", 1)
		recalculated = strings.Replace(recalculated, ">10</b>", ">70</b>", 1)
		if w := saveThroughMux(t, srv, f, recalculated, etag); w.Code != 200 {
			t.Fatalf("the recalculating save = %d, want 200: %s", w.Code, w.Body.String())
		}

		w := recv(t, done)
		meta := metaOf(t, w)
		if meta["recalculated"] != true || meta["changed"] != true || meta["stale"] != false {
			t.Errorf("__meta__ = %v, want recalculated and changed true and stale false", meta)
		}
		if meta["etag"] != w.Header().Get("ETag") || meta["etag"] == "" {
			t.Errorf("__meta__ etag = %v, want the ETag header %q", meta["etag"], w.Header().Get("ETag"))
		}
		if code, ok := meta["code"]; ok {
			t.Errorf("__meta__ carries code %v on a recalculated answer", code)
		}
	})

	t.Run("no open tab answers no-browser at once", func(t *testing.T) {
		// Bug: the write waits for tabs that do not exist.
		srv, _ := setup(t)
		start := time.Now()
		w := postDataWrite(t, srv, "/_/api/invoice.htmlclay?recalculate=wait", `{"qty":"7"}`, nil)
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("the write took %v: it waited out the tabs' time with no tab open", took)
		}
		want := `{"qty":"7","total":"10","__meta__":{"stale":true,"code":"no-browser","why":"` + recalcWhy["no-browser"] + `"}}`
		if w.Code != 200 || w.Body.String() != want {
			t.Fatalf("the waited write = %d %s\nwant %s", w.Code, w.Body.String(), want)
		}
		if got := w.Header().Get("Clay-Stale"); got != "true" {
			t.Errorf("Clay-Stale = %q, want true", got)
		}
	})

	t.Run("a tab that says busy ends the wait", func(t *testing.T) {
		// Bug: a busy tab leaves the agent waiting the full limit.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		id, etag, done := startWait(t, srv, sub, "/_/api/invoice.htmlclay?recalculate=wait")

		note := func(t *testing.T) *httptest.ResponseRecorder {
			t.Helper()
			req := httptest.NewRequest("POST", "/_/recalc/"+f.Token,
				strings.NewReader(`{"id":"`+id+`","etag":"`+etag+`","status":"busy"}`))
			req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
			sameOriginHeaders(req)
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			return w
		}
		noted := time.Now()
		if w := note(t); w.Code != http.StatusNoContent {
			t.Fatalf("the busy note = %d, want 204: %s", w.Code, w.Body.String())
		}
		w := recv(t, done)
		if took := time.Since(noted); took > 2*time.Second {
			t.Errorf("the write answered %v after the busy note: it waited out the tabs' time", took)
		}
		if meta := metaOf(t, w); meta["code"] != "no-browser" {
			t.Errorf("__meta__ code = %v, want no-browser (body %s)", meta["code"], w.Body.String())
		}

		if w := note(t); w.Code != http.StatusNotFound {
			t.Errorf("the second busy note = %d, want 404: the wait is gone", w.Code)
		}
	})

	t.Run("a save that lands just after a busy note still answers fresh values", func(t *testing.T) {
		// Bug: a browser's tabs share one subscription, so one tab's busy note ended the wait while a
		// sibling tab (or the busy tab's own merge save) was still saving fresh values.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		id, etag, done := startWait(t, srv, sub, "/_/api/invoice.htmlclay?recalculate=wait")

		req := httptest.NewRequest("POST", "/_/recalc/"+f.Token,
			strings.NewReader(`{"id":"`+id+`","etag":"`+etag+`","status":"busy"}`))
		req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
		sameOriginHeaders(req)
		nw := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(nw, req)
		if nw.Code != http.StatusNoContent {
			t.Fatalf("the busy note = %d, want 204: %s", nw.Code, nw.Body.String())
		}
		recalculated := strings.Replace(readDoc(t, f.AbsPath), string(dataapi.StaleMark), "", 1)
		recalculated = strings.Replace(recalculated, ">10</b>", ">70</b>", 1)
		if w := saveThroughMux(t, srv, f, recalculated, etag); w.Code != 200 {
			t.Fatalf("the sibling's save = %d, want 200: %s", w.Code, w.Body.String())
		}

		w := recv(t, done)
		if w.Code != 200 || w.Body.String() != `{"qty":"7","total":"70"}` {
			t.Errorf("the waited write = %d %s, want the sibling's fresh values", w.Code, w.Body.String())
		}
	})

	t.Run("a refused save answers save-refused", func(t *testing.T) {
		// Bug: a refused save leaves the agent waiting, or reports fresh values that never landed.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		_, etag, done := startWait(t, srv, sub, "/_/api/invoice.htmlclay?recalculate=wait")

		changed := strings.Replace(readDoc(t, f.AbsPath), `value="7"`, `value="9"`, 1)
		if err := os.WriteFile(f.AbsPath, []byte(changed), 0644); err != nil {
			t.Fatal(err)
		}
		recalculated := strings.Replace(changed, string(dataapi.StaleMark), "", 1)
		recalculated = strings.Replace(recalculated, ">10</b>", ">70</b>", 1)
		refusedAt := time.Now()
		if w := saveThroughMux(t, srv, f, recalculated, etag); w.Code != http.StatusPreconditionFailed {
			t.Fatalf("the stale save = %d, want 412: %s", w.Code, w.Body.String())
		}

		w := recv(t, done)
		if took := time.Since(refusedAt); took > 2*time.Second {
			t.Errorf("the write answered %v after the refusal: it waited out the tabs' time", took)
		}
		if meta := metaOf(t, w); meta["code"] != "save-refused" {
			t.Errorf("__meta__ code = %v, want save-refused (body %s)", meta["code"], w.Body.String())
		}
	})

	t.Run("a silent tab runs into the timeout", func(t *testing.T) {
		// Bug: the limit is ignored.
		srv, f := setup(t)
		sub := newSubscriber(f.AbsPath, laneLive)
		srv.hub.add(sub)

		_, _, done := startWait(t, srv, sub, "/_/api/invoice.htmlclay?recalculate=wait&timeout=600")
		w := recv(t, done)
		if meta := metaOf(t, w); meta["code"] != "timeout" {
			t.Errorf("__meta__ code = %v, want timeout (body %s)", meta["code"], w.Body.String())
		}
	})

	t.Run("malformed parameters are refused before the file is touched", func(t *testing.T) {
		// Bug: a typo writes without waiting.
		srv, f := setup(t)
		before := readDoc(t, f.AbsPath)
		for _, target := range []string{
			"/_/api/invoice.htmlclay?recalculate=now",
			"/_/api/invoice.htmlclay?recalculate=wait&timeout=100",
			"/_/api/invoice.htmlclay?recalculate=wait&timeout=abc",
			"/_/api/invoice.htmlclay?data=%7B%22q%22%3A%22%23qty%40value%22%7D&recalculate=wait",
		} {
			if w := postDataWrite(t, srv, target, `{"qty":"7"}`, nil); w.Code != http.StatusBadRequest {
				t.Errorf("%s = %d, want 400: %s", target, w.Code, w.Body.String())
			}
			if after := readDoc(t, f.AbsPath); after != before {
				t.Fatalf("%s wrote the file:\n%s", target, after)
			}
		}
	})
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
