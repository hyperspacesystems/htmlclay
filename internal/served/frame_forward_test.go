package served

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/panphora/htmlclay/internal/served/assets"
)

// TestFrameForward is the workspace frame's route: a document load carrying
// workspace-frame goes to the service, whose answer may be framed by the
// dashboard alone; a load with no id stays Go's own and gains no frame header.
func TestFrameForward(t *testing.T) {
	var mu sync.Mutex
	var forwarded []string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		forwarded = append(forwarded, r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Write([]byte("<!doctype html><p>bridged</p>"))
	}))
	t.Cleanup(stub.Close)

	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		Service:         true,
		Dashboard:       "http://htmlclay.localhost:1212",
	})
	t.Cleanup(host.Close)
	server, _ := switchTestServer(t, host, "framed")
	if err := host.SetServicePeer(stub.URL, strings.Repeat("ab", 32), "0123456789abcdef"); err != nil {
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

	res, body := get("/page.html?workspace-frame=3")
	if res.StatusCode != http.StatusOK || body != "<!doctype html><p>bridged</p>" {
		t.Fatalf("a framed load = %d %q", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); csp != "frame-ancestors http://htmlclay.localhost:1212" {
		t.Fatalf("a framed load's frame header = %q", csp)
	}
	if res.Header.Get("X-Frame-Options") != "" {
		t.Fatal("a framed load kept X-Frame-Options, which would refuse the dashboard")
	}
	mu.Lock()
	if len(forwarded) != 1 || forwarded[0] != "/v1/route/page.html?workspace-frame=3" {
		t.Fatalf("the service saw %v", forwarded)
	}
	mu.Unlock()

	res, body = get("/page.html")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "Saved") || strings.Contains(res.Header.Get("Content-Security-Policy"), "htmlclay.localhost") {
		t.Fatalf("a load with no id = %d, CSP %q, body %q", res.StatusCode, res.Header.Get("Content-Security-Policy"), body)
	}
	if res, _ := get("/style.css?workspace-frame=3"); res.StatusCode == http.StatusOK && res.Header.Get("Content-Security-Policy") != "" {
		t.Fatal("a file that is not a document was sent to the frame route")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(forwarded) != 1 {
		t.Fatalf("a load with no id, or not of a document, reached the service: %v", forwarded)
	}
}
