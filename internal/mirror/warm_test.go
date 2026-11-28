package mirror_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/mirror"
)

// standInCDN answers a fixed set of paths and counts the requests it gets for each.
type standInCDN struct {
	server *httptest.Server
	files  map[string]struct{ kind, body string }

	mu   sync.Mutex
	hits map[string]int
}

func newStandInCDN(t *testing.T, files map[string]struct{ kind, body string }) *standInCDN {
	t.Helper()
	c := &standInCDN{files: files, hits: map[string]int{}}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.hits[r.URL.Path]++
		c.mu.Unlock()
		f, ok := c.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", f.kind)
		io.WriteString(w, f.body)
	}))
	t.Cleanup(c.server.Close)
	return c
}

func (c *standInCDN) url(path string) string { return c.server.URL + path }

func (c *standInCDN) addr() string { return c.server.Listener.Addr().String() }

func (c *standInCDN) count(path string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits[path]
}

func (c *standInCDN) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, v := range c.hits {
		n += v
	}
	return n
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAFileAddedToAFolderGetsItsTagsAndStylesheetReferencesCopied(t *testing.T) {
	cdn := newStandInCDN(t, map[string]struct{ kind, body string }{
		"/lib/a.js":      {"text/javascript", "window.a=1"},
		"/css/s.css":     {"text/css", `@import url("/css/root.css"); @font-face{src:url("../fonts/f.woff2")}`},
		"/css/root.css":  {"text/css", "body{}"},
		"/fonts/f.woff2": {"font/woff2", "font"},
		"/img/p.png":     {"image/png", "png"},
		"/img/p2.png":    {"image/png", "png"},
	})
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub", "page.htmlclay"), `<html><head>`+
		`<script src="`+cdn.url("/lib/a.js")+`"></script>`+
		`<link rel="stylesheet" href="`+cdn.url("/css/s.css")+`">`+
		`<script src="https://clayjs.com/v1/clay.js"></script>`+
		`<link rel="stylesheet" crossorigin="use-credentials" href="`+cdn.url("/private.css")+`">`+
		`</head><body>`+
		`<picture><source srcset="`+cdn.url("/img/p2.png")+` 2x"></picture>`+
		`<img src="`+cdn.url("/img/p.png")+`">`+
		`<video src="`+cdn.url("/v.mp4")+`" poster="`+cdn.url("/img/p.png")+`"></video>`+
		`<script src="local.js"></script>`+
		`</body></html>`)
	writeFile(t, filepath.Join(dir, "node_modules", "x", "skip.html"),
		`<script src="`+cdn.url("/skipped.js")+`"></script>`)

	store := openStore(t, mirror.DefaultLimits)
	w := &mirror.Warmer{
		Fetch:   mirror.NewFetcher(store, allowOnly(cdn.addr())),
		Folders: func() []string { return []string{dir} },
	}
	w.Pass(context.Background())

	want := []string{
		cdn.url("/lib/a.js"),
		cdn.url("/css/s.css"),
		cdn.url("/css/root.css"),
		cdn.url("/fonts/f.woff2"),
		cdn.url("/img/p.png"),
		cdn.url("/img/p2.png"),
	}
	if n := store.Held("anyfolder.localhost", want); n != len(want) {
		t.Fatalf("another origin may read %d of the page's %d files, want %d", n, len(want), len(want))
	}
	for _, p := range []string{"/private.css", "/v.mp4", "/skipped.js"} {
		if n := cdn.count(p); n != 0 {
			t.Fatalf("the CDN answered %d request(s) for %s, want 0", n, p)
		}
	}
	clay := []string{"https://clayjs.com/v1/clay.js"}
	if n := store.Held("anyfolder.localhost", clay); n != 0 {
		t.Fatalf("the store holds %d copy of clayjs.com, want 0", n)
	}
}

func TestStaticReferencesSkipsMediaSourcesOutsidePicture(t *testing.T) {
	doc := `<video><source src="https://cdn.example/v.mp4"></video>` +
		`<picture><source srcset="https://cdn.example/a.png 1x, https://cdn.example/b.png 2x"></picture>` +
		`<link rel="icon" href="//cdn.example/i.ico">` +
		`<link rel="alternate" href="https://cdn.example/feed.xml">`
	got := mirror.StaticReferences([]byte(doc))
	want := []string{"https://cdn.example/a.png", "https://cdn.example/b.png", "https://cdn.example/i.ico"}
	if len(got) != len(want) {
		t.Fatalf("StaticReferences = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("StaticReferences = %v, want %v", got, want)
		}
	}
}

func TestAPageCannotWriteIntoTheSharedIndex(t *testing.T) {
	store := openStore(t, mirror.DefaultLimits)
	h := &mirror.Handler{Store: store}
	target := "https://cdn.example/a.js"
	for _, origin := range []string{mirror.Shared, "../x"} {
		req := httptest.NewRequest(http.MethodPost, "http://b.localhost/_/offline/store?url="+url.QueryEscape(target)+"&dest=a.js", strings.NewReader("evil"))
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Clay-Content-Type", "text/javascript")
		rec := httptest.NewRecorder()
		h.Serve(rec, req, origin)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("the store route answered %d %s for origin %q, want 403", rec.Code, rec.Body.String(), origin)
		}
	}
	if n := store.Held("b.localhost", []string{target}); n != 0 {
		t.Fatalf("the store holds %d copy written from a page, want 0", n)
	}
}

func TestAnUnchangedFileIsNotRefetched(t *testing.T) {
	cdn := newStandInCDN(t, map[string]struct{ kind, body string }{
		"/lib/a.js":     {"text/javascript", "window.a=1"},
		"/css/s.css":    {"text/css", `@import url("/css/root.css");`},
		"/css/root.css": {"text/css", "body{}"},
	})
	dir := t.TempDir()
	page := filepath.Join(dir, "page.htmlclay")
	writeFile(t, page, `<html><head>`+
		`<script src="`+cdn.url("/lib/a.js")+`"></script>`+
		`<link rel="stylesheet" href="`+cdn.url("/css/s.css")+`">`+
		`</head></html>`)

	store := openStore(t, mirror.DefaultLimits)
	newWarmer := func() *mirror.Warmer {
		return &mirror.Warmer{
			Fetch:   mirror.NewFetcher(store, allowOnly(cdn.addr())),
			Folders: func() []string { return []string{dir} },
		}
	}
	ctx := context.Background()
	w := newWarmer()
	w.Pass(ctx)
	w.Pass(ctx)
	if n := cdn.count("/lib/a.js"); n != 1 {
		t.Fatalf("the CDN answered %d request(s) for the unchanged file's script, want 1", n)
	}

	restarted := newWarmer()
	restarted.Pass(ctx)
	if n := cdn.count("/lib/a.js"); n != 1 {
		t.Fatalf("a restarted warmer refetched the fresh copy: %d request(s) for the script, want 1", n)
	}

	writeFile(t, page, `<html><head>`+
		`<script src="`+cdn.url("/lib/a.js")+`"></script>`+
		`<link rel="stylesheet" href="`+cdn.url("/css/s.css")+`">`+
		`<!-- changed -->`+
		`</head></html>`)
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(page, later, later); err != nil {
		t.Fatal(err)
	}
	restarted.Pass(ctx)
	if n := cdn.count("/lib/a.js"); n != 2 {
		t.Fatalf("a changed file was not revalidated: %d request(s) for the script, want 2", n)
	}
}

func TestWarmingStopsWhileOffCopiesAreOff(t *testing.T) {
	cdn := newStandInCDN(t, map[string]struct{ kind, body string }{
		"/lib/a.js": {"text/javascript", "window.a=1"},
	})
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "page.htmlclay"), `<script src="`+cdn.url("/lib/a.js")+`"></script>`)

	store := openStore(t, mirror.DefaultLimits)
	w := &mirror.Warmer{
		Fetch:   mirror.NewFetcher(store, allowOnly(cdn.addr())),
		Folders: func() []string { return []string{dir} },
		Off:     func() bool { return true },
	}
	w.Pass(context.Background())
	if n := cdn.total(); n != 0 {
		t.Fatalf("the CDN answered %d request(s) while offline copies were off, want 0", n)
	}
}
