package served

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/served/assets"
)

// TestAStrangerPageIsServedWithItsScriptsOff is Opus 12 and Carbon 1/4: a
// document nobody vouches for is sandboxed on the folder's origin, a compressed
// page counts as a page, and a document the person's own is served as itself.
func TestAStrangerPageIsServedWithItsScriptsOff(t *testing.T) {
	const host = "sandbox.htmlclay.localhost:1212"
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const document = "<!doctype html><html><body><p>page</p></body></html>\n"
	for _, name := range []string{"stranger.html", "own.html", "evil.html.gz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(document), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	strangers := map[string]bool{
		filepath.Join(dir, "stranger.html"): true,
		filepath.Join(dir, "evil.html.gz"):  true,
	}
	host1 := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		Stranger:        func(absPath string) bool { return strangers[absPath] },
	})
	t.Cleanup(host1.Close)
	handler, err := host1.AddRoot(RootSpec{ID: "sandbox", Path: dir, PublicHost: host})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	get := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://"+host+target, nil))
		return response
	}
	if got := get("/stranger.html").Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Fatalf("a stranger's page is served with %q, want sandbox", got)
	}
	if got := get("/own.html").Header().Get("Content-Security-Policy"); got == "sandbox" {
		t.Fatal("the person's own page was sandboxed")
	}
	if got := get("/evil.html.gz").Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Fatalf("a compressed page is served with %q, want sandbox", got)
	}
}
