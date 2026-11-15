package served

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/served/assets"
)

func TestM4TheirsRoute(t *testing.T) {
	held := map[string]string{
		"board.html":      ".hyperclay/conflicts/board.remote-aaaa1111.html",
		"outside.html":    "page.html",
		"escape.html":     ".hyperclay/conflicts/../../page.html",
		"linked.html":     ".hyperclay/conflicts/linked.remote-bbbb2222.html",
		"apps/notes.html": ".hyperclay/conflicts/notes.remote-cccc3333.html",
		"gone-copy.html":  ".hyperclay/conflicts/gone.remote-dddd4444.html",
	}
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		HeldCopy: func(rootID, rel string) (string, bool) {
			copy, ok := held[rel]
			return copy, ok && rootID == "theirs"
		},
	})
	t.Cleanup(host.Close)
	server, dir := switchTestServer(t, host, "theirs")
	conflicts := filepath.Join(dir, ".hyperclay", "conflicts")
	if err := os.MkdirAll(conflicts, 0o755); err != nil {
		t.Fatal(err)
	}
	theirs := "<!doctype html><script>parent.saved = true</script><p>their edit</p>"
	consultWriteFile(t, conflicts, "board.remote-aaaa1111.html", []byte(theirs))
	consultWriteFile(t, conflicts, "notes.remote-cccc3333.html", []byte("<p>their notes</p>"))
	if err := os.Symlink(filepath.Join(dir, "page.html"), filepath.Join(conflicts, "linked.remote-bbbb2222.html")); err != nil {
		t.Fatal(err)
	}

	get := func(target string) (*http.Response, string) {
		t.Helper()
		res, err := http.Get(server.URL + target)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res, string(body)
	}

	t.Run("a held copy is served sandboxed, unsniffed and uncached", func(t *testing.T) {
		res, body := get("/_/sync/theirs/board.html")
		if res.StatusCode != http.StatusOK || body != theirs {
			t.Fatalf("GET = %d %q", res.StatusCode, body)
		}
		policy := res.Header.Get("Content-Security-Policy")
		if !strings.Contains(policy, "sandbox") || !strings.Contains(policy, "frame-ancestors") {
			t.Fatalf("Content-Security-Policy = %q, want the frame rule and sandbox", policy)
		}
		if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("headers = %v", res.Header)
		}
	})

	t.Run("a document in a subfolder finds its copy", func(t *testing.T) {
		if res, body := get("/_/sync/theirs/apps/notes.html"); res.StatusCode != http.StatusOK || body != "<p>their notes</p>" {
			t.Fatalf("GET = %d %q", res.StatusCode, body)
		}
	})

	for _, target := range []string{
		"/_/sync/theirs/page.html",
		"/_/sync/theirs/outside.html",
		"/_/sync/theirs/escape.html",
		"/_/sync/theirs/linked.html",
		"/_/sync/theirs/gone-copy.html",
		"/_/sync/theirs/..%2Fpage.html",
	} {
		t.Run("404: "+target, func(t *testing.T) {
			if res, body := get(target); res.StatusCode != http.StatusNotFound {
				t.Fatalf("GET %s = %d %q, want 404", target, res.StatusCode, body)
			}
		})
	}
}
