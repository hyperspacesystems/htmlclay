package served

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/served/assets"
)

const framePolicyDashboard = "http://htmlclay.localhost:1212"

const framePolicyHost = "frames.htmlclay.localhost:1212"

// TestM3FramePolicy is step 7.13: every answer of a served folder refuses a frame
// except its own pages and the dashboard, and a folder that allows frames stops.
func TestM3FramePolicy(t *testing.T) {
	wantPolicy := "frame-ancestors 'self' " + framePolicyDashboard
	wantDashboardOnly := "frame-ancestors " + framePolicyDashboard

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Security-Policy", "frame-ancestors https://elsewhere.example")
		w.Write([]byte("<!doctype html><p>bridged</p>"))
	}))
	t.Cleanup(stub.Close)

	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		Service:         true,
		Dashboard:       framePolicyDashboard,
	})
	t.Cleanup(host.Close)

	dir := t.TempDir()
	const document = "<!doctype html><html><body><p>framed</p></body></html>\n"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler, err := host.AddRoot(RootSpec{ID: "frames", Path: dir, PublicHost: framePolicyHost})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}

	answer := func(method, target string, body io.Reader) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "http://"+framePolicyHost+target, body)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	frameHeader := func(response *httptest.ResponseRecorder) string {
		return strings.Join(response.Header().Values("Content-Security-Policy"), " | ")
	}

	t.Run("document", func(t *testing.T) {
		response := answer(http.MethodGet, "/index.html", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET index.html = %d: %s", response.Code, response.Body.String())
		}
		if got := frameHeader(response); got != wantPolicy {
			t.Errorf("frame header = %q, want %q", got, wantPolicy)
		}
	})

	t.Run("listing", func(t *testing.T) {
		response := answer(http.MethodGet, "/folder/", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET folder/ = %d: %s", response.Code, response.Body.String())
		}
		if got := frameHeader(response); got != wantPolicy {
			t.Errorf("frame header = %q, want %q", got, wantPolicy)
		}
	})

	t.Run("not found", func(t *testing.T) {
		response := answer(http.MethodGet, "/missing.html", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET missing.html = %d: %s", response.Code, response.Body.String())
		}
		if got := frameHeader(response); got != wantPolicy {
			t.Errorf("frame header = %q, want %q", got, wantPolicy)
		}
	})

	t.Run("versions", func(t *testing.T) {
		response := answer(http.MethodGet, "/_/versions/index.html", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET versions = %d: %s", response.Code, response.Body.String())
		}
		if got := frameHeader(response); got != wantPolicy {
			t.Errorf("frame header = %q, want %q", got, wantPolicy)
		}
	})

	t.Run("save refusal", func(t *testing.T) {
		body := strings.NewReader(strings.Repeat("x", saveMaxBytes+1))
		response := answer(http.MethodPost, "/_/save", body)
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("POST save over the limit = %d: %s", response.Code, response.Body.String())
		}
		if got := frameHeader(response); got != "default-src 'none'; frame-ancestors 'none'" {
			t.Errorf("save refusal frame header = %q", got)
		}
	})

	t.Run("workspace frame load", func(t *testing.T) {
		if err := host.SetServicePeer(stub.URL, strings.Repeat("ab", 32), "0123456789abcdef"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(host.ClearServicePeer)
		response := answer(http.MethodGet, "/index.html?workspace-frame=3", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET framed document = %d: %s", response.Code, response.Body.String())
		}
		values := response.Header().Values("Content-Security-Policy")
		if len(values) != 2 || values[0] != wantPolicy || values[1] != wantDashboardOnly {
			t.Fatalf("a framed load carried %q, want both %q and %q", values, wantPolicy, wantDashboardOnly)
		}
	})

	t.Run("frames allowed", func(t *testing.T) {
		host.SetAllowFraming("frames", true)
		response := answer(http.MethodGet, "/index.html", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET index.html = %d: %s", response.Code, response.Body.String())
		}
		if got := frameHeader(response); strings.Contains(got, "frame-ancestors") {
			t.Errorf("a folder that allows frames still carried %q", got)
		}
	})
}
