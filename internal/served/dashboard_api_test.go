package served

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDashboardAPIForward is the dashboard's API on its way to the service: the
// path under /v1/shell, Go's bearer, both epochs and the dashboard origin, no
// client X-Trial-* header, Authorization or cookie, a stream that flushes, and
// the recoverable 503 with no service.
func TestDashboardAPIForward(t *testing.T) {
	var mu sync.Mutex
	var seen []*http.Request
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Clone(r.Context()))
		mu.Unlock()
		if r.URL.Path == "/v1/shell/api/events" {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Write([]byte("{\"type\":\"roots\"}\n"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(stub.Close)

	host := switchTestHost(t)
	api := host.DashboardAPI()
	request := func(target string, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://dashboard.htmlclay.localhost:1212"+target, nil)
		for name, value := range headers {
			r.Header.Set(name, value)
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}

	if w := request("/api/roots", nil); w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("no peer = %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}

	secret := strings.Repeat("ab", 32)
	if err := host.SetServicePeer(stub.URL, secret, "0123456789abcdef"); err != nil {
		t.Fatalf("SetServicePeer: %v", err)
	}
	w := request("/api/roots/r%2F1/list?parent=a", map[string]string{
		"X-Trial-Root": "forged", "Authorization": "Bearer forged", "Cookie": "htmlclay-dashboard-1212=s", "X-Workspace-Token": "t",
	})
	if w.Code != http.StatusOK || w.Body.String() != `{"ok":true}` {
		t.Fatalf("forwarded = %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	got := seen[0]
	mu.Unlock()
	if got.URL.EscapedPath() != "/v1/shell/api/roots/r%2F1/list" || got.URL.RawQuery != "parent=a" {
		t.Fatalf("the service saw %s?%s", got.URL.EscapedPath(), got.URL.RawQuery)
	}
	if got.Header.Get("Authorization") != "Bearer "+secret || got.Header.Get("X-Trial-Peer-Epoch") != "0123456789abcdef" ||
		got.Header.Get("X-Trial-Epoch") != host.Epoch() || got.Header.Get("X-Trial-Public-Origin") != "http://dashboard.htmlclay.localhost:1212" {
		t.Fatalf("the service saw headers %v", got.Header)
	}
	if got.Header.Get("X-Trial-Root") != "" || got.Header.Get("Cookie") != "" {
		t.Fatalf("a client header reached the service: %v", got.Header)
	}

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	res, err := http.Get(server.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line := make(chan string, 1)
	go func() {
		text, _ := bufio.NewReader(res.Body).ReadString('\n')
		line <- text
	}()
	select {
	case text := <-line:
		if text != "{\"type\":\"roots\"}\n" {
			t.Fatalf("the stream's first line = %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the event stream's first line did not arrive while the stream stayed open")
	}

	host.DetachService()
	if w := request("/api/roots", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("detached = %d", w.Code)
	}
}
