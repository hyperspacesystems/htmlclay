package served

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/served/assets"
)

// hostRingHas reports whether the ring holds a change frame for one path and one
// observation.
func hostRingHas(t *testing.T, ring *eventRing, rel, event string) bool {
	t.Helper()
	for _, body := range ring.heldFrames() {
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(body), &frame); err != nil {
			t.Fatalf("frame %q is not JSON: %v", body, err)
		}
		if frame["rel"] == rel && frame["event"] == event {
			return true
		}
	}
	return false
}

func hostTestHost(t *testing.T) *Host {
	t.Helper()
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
	})
	t.Cleanup(host.Close)
	return host
}

// TestHostAddAfterStartWatches proves a root added while the host runs starts its
// own watcher: a file written from outside becomes an add frame in the root's ring.
func TestHostAddAfterStartWatches(t *testing.T) {
	host := hostTestHost(t)
	host.Start()

	dir := t.TempDir()
	handler, err := host.AddRoot(RootSpec{ID: "a", Path: dir, PublicHost: "a.htmlclay.localhost:1212"})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	server, ok := handler.(*rootServer)
	if !ok {
		t.Fatalf("AddRoot returned %T, want *rootServer", handler)
	}

	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for !hostRingHas(t, server.control.events, "note.txt", "add") {
		if time.Now().After(deadline) {
			t.Fatal("the root added after Start never reported the outside file")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestHostRemoveRoot proves a removed root is no longer found and its id is free.
func TestHostRemoveRoot(t *testing.T) {
	host := hostTestHost(t)
	if _, err := host.AddRoot(RootSpec{ID: "first", Path: t.TempDir(), PublicHost: "first.htmlclay.localhost:1212"}); err != nil {
		t.Fatalf("AddRoot first: %v", err)
	}
	if _, err := host.AddRoot(RootSpec{ID: "second", Path: t.TempDir(), PublicHost: "second.htmlclay.localhost:1212"}); err != nil {
		t.Fatalf("AddRoot second: %v", err)
	}

	if !host.RemoveRoot("first") {
		t.Fatal("RemoveRoot(first) = false, want true")
	}
	if _, ok := host.Handler("first"); ok {
		t.Fatal("Handler(first) still finds the removed root")
	}
	if _, ok := host.Handler("second"); !ok {
		t.Fatal("Handler(second) lost the root that stayed")
	}
	if host.RemoveRoot("first") {
		t.Fatal("RemoveRoot(first) a second time = true, want false")
	}
	if _, err := host.AddRoot(RootSpec{ID: "first", Path: t.TempDir(), PublicHost: "first.htmlclay.localhost:1212"}); err != nil {
		t.Fatalf("AddRoot reused the removed id: %v", err)
	}
}

// TestHostServesAProductRoot proves a root starts and serves its document with the
// product's own assets.
func TestHostServesAProductRoot(t *testing.T) {
	host := hostTestHost(t)
	dir := t.TempDir()
	const document = "<!doctype html><html><body><p>served from the product</p></body></html>\n"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := host.AddRoot(RootSpec{ID: "a", Path: dir, PublicHost: "a.htmlclay.localhost:1212"})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://a.htmlclay.localhost:1212/index.html", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET index.html answered %d: %s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, "served from the product") {
		t.Fatalf("GET index.html body does not hold the document: %q", body)
	}
}

// A served document carries the offline copies boot tag on a real navigation only: a
// Clay-Source read is the stored file for an editor to save back, and a fetch() of the
// document runs no worker.
func TestServedDocumentCarriesTheOfflineBootScriptOnlyForRealLoads(t *testing.T) {
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		OfflineBoot: func(html []byte) []byte {
			return htmlutil.InsertAtHeadStart(html, htmlutil.OfflineBootTag)
		},
	})
	t.Cleanup(host.Close)

	dir := t.TempDir()
	const document = "<!doctype html><html><head><title>t</title></head><body><p>x</p></body></html>\n"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := host.AddRoot(RootSpec{ID: "a", Path: dir, PublicHost: "a.htmlclay.localhost:1212"})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}

	cases := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{name: "document navigation", headers: map[string]string{"Sec-Fetch-Dest": "document"}, want: true},
		{name: "no Sec-Fetch-Dest", want: true},
		{name: "Clay-Source read", headers: map[string]string{"Sec-Fetch-Dest": "document", "Clay-Source": "1"}},
		{name: "fetch of the document", headers: map[string]string{"Sec-Fetch-Dest": "empty"}},
	}
	for _, c := range cases {
		request := httptest.NewRequest(http.MethodGet, "http://a.htmlclay.localhost:1212/index.html", nil)
		for name, value := range c.headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: GET index.html answered %d: %s", c.name, response.Code, response.Body.String())
		}
		if got := strings.Contains(response.Body.String(), "/_/offline/boot.js"); got != c.want {
			t.Errorf("%s: body carries the boot script = %v, want %v: %s", c.name, got, c.want, response.Body.String())
		}
	}
}
