package mirror_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/mirror"
)

const workerPath = "/_/offline/sw.js"

func serveRoute(h *mirror.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://a.localhost"+target, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.Serve(rec, req, "a.localhost")
	return rec
}

func TestPageRoutesRefuseAnythingButSameOrigin(t *testing.T) {
	const target = "https://cdn.example/a.js"
	routes := []struct {
		name    string
		method  string
		target  string
		body    string
		headers map[string]string
	}{
		{"store", http.MethodPost, "/_/offline/store?url=" + url.QueryEscape(target), "x", map[string]string{"Clay-Content-Type": "text/javascript"}},
		{"nominate", http.MethodPost, "/_/offline/nominate", "[]", nil},
		{"copy", http.MethodGet, "/_/offline/copy?url=" + url.QueryEscape(target), "", nil},
		{"has", http.MethodPost, "/_/offline/has", "[]", nil},
	}
	store := openStore(t, mirror.DefaultLimits)
	h := &mirror.Handler{Store: store}
	for _, site := range []string{"same-site", "cross-site", "none", ""} {
		for _, route := range routes {
			headers := map[string]string{}
			for k, v := range route.headers {
				headers[k] = v
			}
			if site != "" {
				headers["Sec-Fetch-Site"] = site
			}
			rec := serveRoute(h, route.method, route.target, route.body, headers)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s with Sec-Fetch-Site %q = %d, want 403", route.name, site, rec.Code)
			}
			if n := store.Held("a.localhost", []string{target}); n != 0 {
				t.Fatalf("%s with Sec-Fetch-Site %q left %d cop(ies) in the store", route.name, site, n)
			}
			if _, entries := store.Usage(); entries != 0 {
				t.Fatalf("%s with Sec-Fetch-Site %q left %d entries in the store", route.name, site, entries)
			}
		}
	}

	rec := serveRoute(h, http.MethodPost, "/_/offline/store?url="+url.QueryEscape(target), "x", map[string]string{
		"Sec-Fetch-Site":    "same-origin",
		"Clay-Content-Type": "text/javascript",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the same-origin store request = %d %s, want 204", rec.Code, rec.Body.String())
	}
	if n := store.Held("a.localhost", []string{target}); n != 1 {
		t.Fatalf("the same-origin store request left %d copies, want 1", n)
	}
}

func TestWorkerIsServedForScopeRootAndUnregistersWhenOff(t *testing.T) {
	serveWorker := func(h *mirror.Handler) *httptest.ResponseRecorder {
		return serveRoute(h, http.MethodGet, workerPath, "", nil)
	}

	on := mirror.Handler{Store: openStore(t, mirror.DefaultLimits)}
	rec := serveWorker(&on)
	if rec.Code != http.StatusOK {
		t.Fatalf("the worker route = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Service-Worker-Allowed"); got != "/" {
		t.Fatalf("Service-Worker-Allowed = %q, want /", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("Content-Type = %q, want text/javascript", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "networkFirst") {
		t.Fatal("the worker does not carry networkFirst")
	}
	if strings.Contains(body, "{{VERSION}}") || strings.Contains(body, "{{MAX_BYTES}}") ||
		strings.Contains(body, "{{IMAGES}}") || strings.Contains(body, "{{VIDEO}}") || strings.Contains(body, "{{AUDIO}}") {
		t.Fatal("the worker was served with a placeholder left in it")
	}
	if !strings.Contains(body, "const MAX_BYTES = Number('26214400');") {
		t.Fatal("the worker was served without the default per-file limit")
	}
	if !strings.Contains(body, "KEEP_IMAGES = Number('1')") || !strings.Contains(body, "KEEP_VIDEO = Number('0')") || !strings.Contains(body, "KEEP_AUDIO = Number('0')") {
		t.Fatal("the worker was served the default policy's kinds wrong")
	}

	off := mirror.Handler{Store: openStore(t, mirror.DefaultLimits), Off: func() bool { return true }}
	body = serveWorker(&off).Body.String()
	if !strings.Contains(body, "unregister()") {
		t.Fatalf("the worker served while offline copies are off does not unregister itself: %q", body)
	}
	if strings.Contains(body, "networkFirst") {
		t.Fatalf("the worker served while offline copies are off still mirrors: %q", body)
	}

	unlimited := mirror.Handler{Store: openStore(t, mirror.Limits{})}
	if body := serveWorker(&unlimited).Body.String(); !strings.Contains(body, "const MAX_BYTES = Number('Infinity');") {
		t.Fatal("a store with no per-file limit was served a finite MAX_BYTES")
	}

	full := openStore(t, mirror.Limits{})
	full.SetPolicy(mirror.PolicyFor("full", "", false, true, true, true, 0))
	body = serveWorker(&mirror.Handler{Store: full}).Body.String()
	for _, want := range []string{"KEEP_IMAGES = Number('1')", "KEEP_VIDEO = Number('1')", "KEEP_AUDIO = Number('1')"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the worker served under the fully offline policy lacks %q", want)
		}
	}
}

func TestStoreRefusesNonHTTPAndOversizedBodies(t *testing.T) {
	store := openStore(t, mirror.Limits{PerFile: 8})
	h := &mirror.Handler{Store: store}
	post := func(target, body string) *httptest.ResponseRecorder {
		return serveRoute(h, http.MethodPost, "/_/offline/store?url="+url.QueryEscape(target), body, map[string]string{
			"Sec-Fetch-Site":    "same-origin",
			"Clay-Content-Type": "text/javascript",
		})
	}

	if rec := post("file:///etc/hosts", "x"); rec.Code != http.StatusBadRequest {
		t.Fatalf("a file:// URL = %d, want 400", rec.Code)
	}
	if rec := post("https://cdn.example/a.js", "123456789"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a 9-byte body over an 8-byte per-file limit = %d, want 413", rec.Code)
	}
	if n := store.Held("a.localhost", []string{"https://cdn.example/a.js"}); n != 0 {
		t.Fatalf("the store holds %d copies after both refusals, want 0", n)
	}
	if _, entries := store.Usage(); entries != 0 {
		t.Fatalf("the store holds %d entries after both refusals, want 0", entries)
	}
}

// The boot tag every served document carries points here. The script runs before any
// other script on the page; with copies switched off it must still remove its own tag.
func TestBootScriptIsServedAndStillRemovesItselfWhenOff(t *testing.T) {
	on := mirror.Handler{Store: openStore(t, mirror.DefaultLimits)}
	rec := serveRoute(&on, http.MethodGet, "/_/offline/boot.js", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the boot route = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("Content-Type = %q, want text/javascript", got)
	}
	body := rec.Body.String()
	for _, want := range []string{"serviceWorker", "removeChild(me)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the boot script does not carry %q: %q", want, body)
		}
	}

	off := mirror.Handler{Store: openStore(t, mirror.DefaultLimits), Off: func() bool { return true }}
	body = serveRoute(&off, http.MethodGet, "/_/offline/boot.js", "", nil).Body.String()
	if !strings.Contains(body, "removeChild(s)") {
		t.Fatalf("the boot script served while offline copies are off does not remove its tag: %q", body)
	}
	if strings.Contains(body, "serviceWorker") {
		t.Fatalf("the boot script served while offline copies are off still registers: %q", body)
	}
}

// Opened directly or framed on the folder's origin, a stored third-party SVG would run
// its scripts there. Only the worker's fetch() may read a copy.
func TestCopiesAreOnlyReadByTheWorkersFetch(t *testing.T) {
	store, err := mirror.Open(t.TempDir(), mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	const u = "https://cdn.example/badge.svg"
	if err := store.Put("a.localhost", u, "image/svg+xml", []byte(`<svg><script>alert(1)</script></svg>`), "page"); err != nil {
		t.Fatal(err)
	}
	h := &mirror.Handler{Store: store}
	target := "/_/offline/copy?url=" + url.QueryEscape(u)
	for _, dest := range []string{"document", "iframe"} {
		rec := serveRoute(h, "GET", target, "", map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": dest})
		if rec.Code != http.StatusForbidden {
			t.Errorf("Sec-Fetch-Dest %s = %d, want 403", dest, rec.Code)
		}
	}
	rec := serveRoute(h, "GET", target, "", map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": "empty"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("the worker's read = %d with CSP %q, want 200 with a sandbox policy", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
}

// A media element asks with a Range header: the copy must answer that range, or a video
// played offline starts over at byte zero.
func TestACopyAnswersARangeRequest(t *testing.T) {
	store, err := mirror.Open(t.TempDir(), mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	store.SetPolicy(mirror.DefaultPolicy)
	const u = "https://cdn.example/a.js"
	if err := store.Put("a.localhost", u, "text/javascript", []byte("01234567890123456789"), "page"); err != nil {
		t.Fatal(err)
	}
	h := &mirror.Handler{Store: store}
	rec := serveRoute(h, http.MethodGet, "/_/offline/copy?url="+url.QueryEscape(u), "", map[string]string{
		"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": "empty", "Range": "bytes=0-9",
	})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("a range read = %d, want 206", rec.Code)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 0-9/20" {
		t.Fatalf("Content-Range = %q, want bytes 0-9/20", got)
	}
	if got := rec.Body.String(); got != "0123456789" {
		t.Fatalf("the range read = %q, want 0123456789", got)
	}
}

// The default policy never keeps video: the worker may still hand one over, and the store
// route must drop it without a copy.
func TestTheStoreRouteKeepsNothingThePolicyLeavesOut(t *testing.T) {
	cases := []struct{ name, dest, contentType string }{
		{"a video hand-over", "video", "video/mp4"},
		{"a video request whose type the server left opaque", "video", "application/octet-stream"},
		{"video bytes on a request with no destination", "", "video/mp4"},
	}
	for _, c := range cases {
		store := openStore(t, mirror.DefaultLimits)
		store.SetPolicy(mirror.DefaultPolicy)
		h := &mirror.Handler{Store: store}
		const u = "https://cdn.example/v.mp4"
		rec := serveRoute(h, http.MethodPost, "/_/offline/store?url="+url.QueryEscape(u)+"&dest="+c.dest, "mp4",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Clay-Content-Type": c.contentType})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s = %d, want 204", c.name, rec.Code)
		}
		if n := store.Held("a.localhost", []string{u}); n != 0 {
			t.Fatalf("the store holds %d copy after %s, want 0", n, c.name)
		}
	}
}

func TestAVideoCopiedAheadIsNotFetchedAgain(t *testing.T) {
	cdn := newStandInCDN(t, map[string]struct{ kind, body string }{
		"/v.mp4": {"video/mp4", "mp4"},
	})
	store := openStore(t, mirror.Limits{})
	store.SetPolicy(mirror.PolicyFor("full", "", false, true, true, true, 0))
	target := cdn.url("/v.mp4")
	if err := store.Put(mirror.Shared, target, "video/mp4", []byte("mp4"), "warm"); err != nil {
		t.Fatal(err)
	}
	h := &mirror.Handler{Store: store, Fetch: mirror.NewFetcher(store, allowOnly(cdn.addr()))}
	body := `[{"url":"` + target + `","kind":"video"}]`
	rec := serveRoute(h, http.MethodPost, "/_/offline/nominate", body, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the nominate route = %d, want 204", rec.Code)
	}
	h.Wait()
	if n := cdn.count("/v.mp4"); n != 0 {
		t.Fatalf("the stand-in server answered %d request(s) for a video already copied ahead, want 0", n)
	}
}

func TestUnderTheDefaultAVideoNominationFollowsItsBytes(t *testing.T) {
	cdn := newStandInCDN(t, map[string]struct{ kind, body string }{
		"/poster.png": {"image/png", "png"},
		"/clip.webm":  {"video/webm", "webm"},
	})
	store := openStore(t, mirror.DefaultLimits)
	store.SetPolicy(mirror.DefaultPolicy)
	poster, clip := cdn.url("/poster.png"), cdn.url("/clip.webm")
	h := &mirror.Handler{Store: store, Fetch: mirror.NewFetcher(store, allowOnly(cdn.addr()))}
	body := `[{"url":"` + poster + `","kind":"video"},{"url":"` + clip + `","kind":"video"}]`
	rec := serveRoute(h, http.MethodPost, "/_/offline/nominate", body, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the nominate route = %d, want 204", rec.Code)
	}
	h.Wait()
	if n := store.Held("a.localhost", []string{poster}); n != 1 {
		t.Fatalf("the store holds %d copy of the video poster under the default policy, want 1", n)
	}
	if n := store.Held("a.localhost", []string{clip}); n != 0 {
		t.Fatalf("the store holds %d copy of a real video under the default policy, want 0", n)
	}
}
