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
	if strings.Contains(body, "{{VERSION}}") || strings.Contains(body, "{{MAX_BYTES}}") {
		t.Fatal("the worker was served with a placeholder left in it")
	}
	if !strings.Contains(body, "const MAX_BYTES = 26214400;") {
		t.Fatal("the worker was served without the default per-file limit")
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
	if body := serveWorker(&unlimited).Body.String(); !strings.Contains(body, "const MAX_BYTES = Infinity;") {
		t.Fatal("a store with no per-file limit was served a finite MAX_BYTES")
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
