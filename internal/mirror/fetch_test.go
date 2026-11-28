package mirror_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/egress"
	"github.com/panphora/htmlclay/internal/mirror"
)

// allowOnly lets the test reach its stand-in server and nothing private: every other
// address goes through the real public-only check.
func allowOnly(addr string) func(ctx context.Context, network, want string) (net.Conn, error) {
	return func(ctx context.Context, network, want string) (net.Conn, error) {
		if want == addr {
			return (&net.Dialer{}).DialContext(ctx, network, want)
		}
		return egress.DialPublic(ctx, network, want)
	}
}

func openStore(t *testing.T, limits mirror.Limits) *mirror.Store {
	t.Helper()
	store, err := mirror.Open(t.TempDir(), limits)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestHostFetchRefusesLoopback(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/javascript")
		io.WriteString(w, "window.x=1")
	}))
	defer cdn.Close()
	target := cdn.URL + "/x.js"

	store := openStore(t, mirror.DefaultLimits)
	f := mirror.NewFetcher(store, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := f.Fetch(ctx, "a.localhost", target, mirror.Hints{}, "host"); err == nil {
		t.Fatal("the host fetched a loopback address")
	}
	if n := store.Held("a.localhost", []string{target}); n != 0 {
		t.Fatalf("the store holds %d loopback copies, want 0", n)
	}
	mu.Lock()
	got := requests
	mu.Unlock()
	if got != 0 {
		t.Fatalf("the loopback server answered %d request(s), want 0", got)
	}
}

func TestStylesheetReferencesAreFollowedFromTheStylesheetsURL(t *testing.T) {
	bodies := map[string]struct{ kind, body string }{
		"/css/style.css": {"text/css", "@import url(\"/css/root.css\");\n@font-face { font-family: F; src: url(\"../fonts/f.ttf\"); }\nbody { background: url(data:image/png;base64,AAAA); }"},
		"/css/root.css":  {"text/css", "body{}"},
		"/fonts/f.ttf":   {"font/ttf", "font"},
	}
	var mu sync.Mutex
	var paths []string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		b, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", b.kind)
		io.WriteString(w, b.body)
	}))
	defer cdn.Close()

	store := openStore(t, mirror.DefaultLimits)
	f := mirror.NewFetcher(store, allowOnly(cdn.Listener.Addr().String()))
	if err := f.Fetch(context.Background(), "a.localhost", cdn.URL+"/css/style.css", mirror.Hints{}, "host"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := []string{cdn.URL + "/css/style.css", cdn.URL + "/css/root.css", cdn.URL + "/fonts/f.ttf"}
	if n := store.Held("a.localhost", want); n != len(want) {
		t.Fatalf("the store holds %d of the stylesheet's %d files", n, len(want))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range paths {
		if strings.Contains(p, "data:") {
			t.Fatalf("the CDN was asked for the inline data URL %q", p)
		}
		switch p {
		case "/css/style.css", "/css/root.css", "/fonts/f.ttf":
		default:
			t.Fatalf("the CDN was asked for %q, which the stylesheet does not name", p)
		}
	}
}

func TestStoringAChangedStylesheetFromAPageRevalidatesItsReferences(t *testing.T) {
	var mu sync.Mutex
	version := "v1"
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/css/root.css" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		v := version
		mu.Unlock()
		etag := fmt.Sprintf("%q", v)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("ETag", etag)
		io.WriteString(w, "body{color:"+v+"}")
	}))
	defer cdn.Close()

	store := openStore(t, mirror.DefaultLimits)
	dial := allowOnly(cdn.Listener.Addr().String())
	f := mirror.NewFetcher(store, dial)
	root := cdn.URL + "/css/root.css"
	if err := f.Fetch(context.Background(), "a.localhost", root, mirror.Hints{}, "host"); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	mu.Lock()
	version = "v2"
	mu.Unlock()

	h := &mirror.Handler{Store: store, Fetch: mirror.NewFetcher(store, dial)}
	req := httptest.NewRequest("POST", "http://a.localhost/_/offline/store?url="+url.QueryEscape(cdn.URL+"/css/style.css")+"&dest=style", strings.NewReader("@import url(\"/css/root.css\");"))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Clay-Content-Type", "text/css")
	rec := httptest.NewRecorder()
	h.Serve(rec, req, "a.localhost")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the store route = %d %s, want 204", rec.Code, rec.Body.String())
	}
	h.Wait()

	entry, body, ok := store.Get("a.localhost", root)
	if !ok {
		t.Fatal("the store no longer holds the imported stylesheet")
	}
	if string(body) != "body{color:v2}" {
		t.Fatalf("the imported stylesheet came back as %q (type %q), want the v2 body", body, entry.Type)
	}
}

func TestHostFetchSendsNoCookiesOrAuthorization(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	body := "first"
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, fmt.Sprintf("cookie=%q authorization=%q", r.Header.Get("Cookie"), r.Header.Get("Authorization")))
		current := body
		mu.Unlock()
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Set-Cookie", "s=1")
		io.WriteString(w, current)
	}))
	defer cdn.Close()

	store := openStore(t, mirror.DefaultLimits)
	f := mirror.NewFetcher(store, allowOnly(cdn.Listener.Addr().String()))
	target := cdn.URL + "/a.js"
	if err := f.Fetch(context.Background(), "a.localhost", target, mirror.Hints{}, "host"); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	mu.Lock()
	body = "second"
	mu.Unlock()
	if err := f.Fetch(context.Background(), "a.localhost", target, mirror.Hints{}, "host"); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("the CDN answered %d requests, want 2: %v", len(seen), seen)
	}
	for i, got := range seen {
		if got != `cookie="" authorization=""` {
			t.Fatalf("request %d reached the CDN with %s", i+1, got)
		}
	}
}

func TestCopyIsStoredWithoutSetCookieWhateverItsCachingHeaders(t *testing.T) {
	const body = "window.x=1"
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Cache-Control", "no-store, private")
		w.Header().Set("Set-Cookie", "s=1")
		io.WriteString(w, body)
	}))
	defer cdn.Close()

	store := openStore(t, mirror.DefaultLimits)
	h := &mirror.Handler{Store: store, Fetch: mirror.NewFetcher(store, allowOnly(cdn.Listener.Addr().String()))}
	target := cdn.URL + "/a.js"
	if err := h.Fetch.Fetch(context.Background(), "a.localhost", target, mirror.Hints{}, "host"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	req := httptest.NewRequest("GET", "http://a.localhost/_/offline/copy?url="+url.QueryEscape(target), nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.Serve(rec, req, "a.localhost")
	if rec.Code != http.StatusOK {
		t.Fatalf("the copy route = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != body {
		t.Fatalf("the copy route answered %q, want %q", rec.Body.String(), body)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript" {
		t.Fatalf("Content-Type = %q, want text/javascript", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("the copy route replayed a cookie: %v", got)
	}
}

// Minified stylesheets drop the space after @import, and file names can hold
// parentheses inside quotes. Both must still be followed, or fonts and images are
// missing offline.
func TestCSSReferencesReadsMinifiedImportsAndQuotedParentheses(t *testing.T) {
	got := mirror.CSSReferences(`@import"theme.css";@import url('a b.css');.h{background:url("img/hero_(1).png")}`)
	want := []string{"theme.css", "a b.css", "img/hero_(1).png"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("CSSReferences = %q, want %q", got, want)
	}
}

// A versionless CDN address redirects to the real stylesheet. Its relative font
// references resolve against where it landed, not the address the page named.
func TestARedirectedStylesheetsReferencesResolveFromWhereItLanded(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pkg":
			http.Redirect(w, r, "/pkg@1/dist/style.css", http.StatusFound)
		case "/pkg@1/dist/style.css":
			w.Header().Set("Content-Type", "text/css")
			io.WriteString(w, `@font-face{src:url(../fonts/f.woff2)}`)
		case "/pkg@1/fonts/f.woff2":
			w.Header().Set("Content-Type", "font/woff2")
			io.WriteString(w, "font")
		default:
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	store := openStore(t, mirror.DefaultLimits)
	f := mirror.NewFetcher(store, allowOnly(cdn.Listener.Addr().String()))
	if err := f.Fetch(context.Background(), "a.localhost", cdn.URL+"/pkg", mirror.Hints{}, "host"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if store.Held("a.localhost", []string{cdn.URL + "/pkg@1/fonts/f.woff2"}) != 1 {
		t.Fatal("the font next to the redirected stylesheet was not copied")
	}
}

func TestADownloadThatStallsIsGivenUp(t *testing.T) {
	defer mirror.SetFetchIdleForTest(200 * time.Millisecond)()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		io.WriteString(w, "mp4")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer cdn.Close()
	store := openStore(t, mirror.Limits{})
	store.SetPolicy(mirror.PolicyFor("full", "", false, true, true, true, 0))
	f := mirror.NewFetcher(store, allowOnly(cdn.Listener.Addr().String()))
	start := time.Now()
	if err := f.Fetch(context.Background(), "a.localhost", cdn.URL+"/v.mp4", mirror.Hints{}, "host"); err == nil {
		t.Fatal("the fetch of a download that stalled returned no error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the stalled download took %v to give up, want well under 5s", elapsed)
	}
}
