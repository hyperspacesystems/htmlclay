//go:build managedtrial

package managedtrial

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// forwardPage is a document whose composed form has a live page store.
const forwardPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Live</title>
    <script merge="page" clay="live"></script>
  </head>
  <body>
    <h1>Live</h1>
  </body>
</html>
`

// forwardPlain is a document with no page store.
const forwardPlain = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Plain</title>
  </head>
  <body>
    <h1>Plain</h1>
  </body>
</html>
`

// forwardedRequest is one request the fake service received.
type forwardedRequest struct {
	Method   string
	Path     string
	RawPath  string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// forwardService is the fake JS service: it records every request and answers
// with the per-test handler.
type forwardService struct {
	server *httptest.Server
	mu     sync.Mutex
	seen   []forwardedRequest
}

func newForwardService(t *testing.T, answer func(http.ResponseWriter, *http.Request)) *forwardService {
	t.Helper()
	fake := &forwardService{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.seen = append(fake.seen, forwardedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawPath:  r.URL.RawPath,
			RawQuery: r.URL.RawQuery,
			Header:   r.Header.Clone(),
			Body:     body,
		})
		fake.mu.Unlock()
		if answer != nil {
			answer(w, r)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *forwardService) peer() servicePeer {
	return servicePeer{URL: f.server.URL, Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa"}
}

func (f *forwardService) requests() []forwardedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]forwardedRequest(nil), f.seen...)
}

// only is the one request the fake saw.
func (f *forwardService) only(t *testing.T) forwardedRequest {
	t.Helper()
	seen := f.requests()
	if len(seen) != 1 {
		t.Fatalf("the fake service saw %d requests, want 1", len(seen))
	}
	return seen[0]
}

// forwardRoot serves one root with the fake service attached, and hands back
// the link so a test can mark a document unconfirmed.
func forwardRoot(t *testing.T, peer servicePeer) (*httptest.Server, *rootServer, *serviceLink, string) {
	t.Helper()
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the root server reads that checkout's bundled runtime release")
	}
	dir := consultTestRoot(t)
	events := newEventRing()
	link := newServiceLink(events, peer)
	root, err := newRootHandler(rootSpec{id: "root-consult", path: dir}, config{localSource: localSource, events: events, service: link})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(root)
	t.Cleanup(server.Close)
	return server, root, link, dir
}

// forwardSend sends one request to Go's test server and reads the whole answer.
func forwardSend(t *testing.T, server *httptest.Server, method, path string, body io.Reader, headers map[string]string) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, answer
}

// TestForwardStreamCarriesPrivateHeaders proves a live stream reaches the
// service's /v1/route with Go's own private headers and none of the client's.
func TestForwardStreamCarriesPrivateHeaders(t *testing.T) {
	fake := newForwardService(t, nil)
	server, _, _, _ := forwardRoot(t, fake.peer())

	document := url.QueryEscape(server.URL + "/page.html")
	status, _, _ := forwardSend(t, server, http.MethodGet, "/_/sync?url="+document, nil, map[string]string{
		"X-Trial-Canonical": "/etc/x",
		"Authorization":     "Bearer nope",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	seen := fake.only(t)
	if seen.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", seen.Method)
	}
	if seen.Path != "/v1/route/_/sync" {
		t.Fatalf("path = %q, want /v1/route/_/sync", seen.Path)
	}
	if seen.RawQuery != "url="+document {
		t.Fatalf("raw query = %q, want the client's own", seen.RawQuery)
	}
	if got := seen.Header.Get("Authorization"); got != controlScheme+strings.Repeat("ab", 32) {
		t.Fatalf("Authorization = %q, want Go's bearer", got)
	}
	if got := seen.Header.Get("X-Trial-Root"); got != "root-consult" {
		t.Fatalf("X-Trial-Root = %q, want root-consult", got)
	}
	host := strings.TrimPrefix(server.URL, "http://")
	if got := seen.Header.Get("X-Trial-Public-Host"); got != host {
		t.Fatalf("X-Trial-Public-Host = %q, want %q", got, host)
	}
	if got := seen.Header.Get("X-Trial-Public-Origin"); got != "http://"+host {
		t.Fatalf("X-Trial-Public-Origin = %q, want http://%s", got, host)
	}
	if got := seen.Header.Get("X-Trial-Epoch"); got == "" {
		t.Fatal("X-Trial-Epoch is empty, want Go's own epoch")
	}
	if got := seen.Header.Get("X-Trial-Peer-Epoch"); got != "aaaaaaaaaaaaaaaa" {
		t.Fatalf("X-Trial-Peer-Epoch = %q, want aaaaaaaaaaaaaaaa", got)
	}
	if got := seen.Header.Get("X-Trial-Canonical"); got != "" {
		t.Fatalf("X-Trial-Canonical = %q, want the client's header dropped", got)
	}
}

// TestForwardStreamFlushesAndCancels proves the answer reaches the client while
// the service is still writing, and the upstream request ends with the client.
func TestForwardStreamFlushesAndCancels(t *testing.T) {
	gone := make(chan struct{})
	fake := newForwardService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "data: one\n\n"); err != nil {
			return
		}
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(gone)
		case <-time.After(5 * time.Second):
		}
	})
	server, _, _, _ := forwardRoot(t, fake.peer())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/_/sync?document-url="+url.QueryEscape(server.URL+"/page.html"), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	frames := make(chan string, 8)
	go func() {
		reader := bufio.NewReader(response.Body)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				frames <- strings.TrimRight(line, "\n")
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case frame := <-frames:
		if frame != "data: one" {
			t.Fatalf("first frame = %q, want data: one", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stream never delivered its first frame")
	}

	cancel()
	response.Body.Close()
	select {
	case <-gone:
	case <-time.After(2 * time.Second):
		t.Fatal("the service never saw its request end")
	}
}

// TestForwardRelayDataAndPresence proves the relay, the data read and presence
// reach the service with their bodies unchanged and their answers unaltered.
func TestForwardRelayDataAndPresence(t *testing.T) {
	fake := newForwardService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/route/_/sync" {
			w.Header().Set("X-Service-Answer", "yes")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"session":"s1"}`)
		}
	})
	server, _, _, _ := forwardRoot(t, fake.peer())

	payload := `{"session":"s1","pages":[{"road":"a.html"}]}`
	status, header, answer := forwardSend(t, server, http.MethodPost, "/_/sync", strings.NewReader(payload), map[string]string{
		"Content-Type": "application/json",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if header.Get("X-Service-Answer") != "yes" {
		t.Fatalf("X-Service-Answer = %q, want the service's own header", header.Get("X-Service-Answer"))
	}
	if string(answer) != `{"session":"s1"}` {
		t.Fatalf("answer = %q, want the service's own body", answer)
	}

	status, _, _ = forwardSend(t, server, http.MethodGet, "/_/sync/data?s=live:0", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("data status = %d, want 200", status)
	}
	status, _, _ = forwardSend(t, server, http.MethodPost, "/_/presence", strings.NewReader(`{"name":"t"}`), nil)
	if status != http.StatusOK {
		t.Fatalf("presence status = %d, want 200", status)
	}

	seen := fake.requests()
	if len(seen) != 3 {
		t.Fatalf("the fake service saw %d requests, want 3", len(seen))
	}
	if seen[0].Method != http.MethodPost || seen[0].Path != "/v1/route/_/sync" {
		t.Fatalf("relay = %s %s, want POST /v1/route/_/sync", seen[0].Method, seen[0].Path)
	}
	if string(seen[0].Body) != payload {
		t.Fatalf("relay body = %q, want it byte-identical", seen[0].Body)
	}
	if seen[1].Method != http.MethodGet || seen[1].Path != "/v1/route/_/sync/data" || seen[1].RawQuery != "s=live:0" {
		t.Fatalf("data = %s %s?%s, want GET /v1/route/_/sync/data with the query", seen[1].Method, seen[1].Path, seen[1].RawQuery)
	}
	if seen[2].Method != http.MethodPost || seen[2].Path != "/v1/route/_/presence" {
		t.Fatalf("presence = %s %s, want POST /v1/route/_/presence", seen[2].Method, seen[2].Path)
	}
	if string(seen[2].Body) != `{"name":"t"}` {
		t.Fatalf("presence body = %q, want it byte-identical", seen[2].Body)
	}
}

// TestForwardDataAPIOnPageStore proves a data API request to a document with a
// live page store is the service's, carrying the canonical path Go resolved.
func TestForwardDataAPIOnPageStore(t *testing.T) {
	fake := newForwardService(t, nil)
	server, _, _, dir := forwardRoot(t, fake.peer())
	consultWriteFile(t, dir, "page.html", []byte(forwardPage))

	status, _, _ := forwardSend(t, server, http.MethodPost, "/_/api/page.html", strings.NewReader(`{"title":"x"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if status != http.StatusOK {
		t.Fatalf("write status = %d, want 200", status)
	}
	status, _, _ = forwardSend(t, server, http.MethodGet, "/page.html?data=", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("read status = %d, want 200", status)
	}

	seen := fake.requests()
	if len(seen) != 2 {
		t.Fatalf("the fake service saw %d requests, want 2", len(seen))
	}
	if seen[0].Method != http.MethodPost || seen[0].Path != "/v1/route/_/api/page.html" {
		t.Fatalf("write = %s %s, want POST /v1/route/_/api/page.html", seen[0].Method, seen[0].Path)
	}
	canonical := upathJoin(dir, "page.html")
	if got := seen[0].Header.Get("X-Trial-Canonical"); got != canonical {
		t.Fatalf("X-Trial-Canonical = %q, want %q", got, canonical)
	}
	if seen[1].Method != http.MethodGet || seen[1].Path != "/v1/route/page.html" {
		t.Fatalf("read = %s %s, want GET /v1/route/page.html", seen[1].Method, seen[1].Path)
	}
	if got := seen[1].Header.Get("X-Trial-Canonical"); got != canonical {
		t.Fatalf("read X-Trial-Canonical = %q, want %q", got, canonical)
	}
}

// TestForwardDoesNotRepeatSessionCookies proves a tab receives each session
// cookie once when Local's app, answering a forwarded request, sets the same
// cookies Go's gate already set, and still receives any other cookie the app
// sets.
func TestForwardDoesNotRepeatSessionCookies(t *testing.T) {
	fake := newForwardService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "isAdminOfCurrentResource=true; Path=/; SameSite=Lax")
		w.Header().Add("Set-Cookie", "isLoggedIn=true; Path=/; SameSite=Lax")
		w.Header().Add("Set-Cookie", "appOnly=1; Path=/")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	})
	server, _, _, dir := forwardRoot(t, fake.peer())
	consultWriteFile(t, dir, "page.html", []byte(forwardPage))

	status, header, _ := forwardSend(t, server, http.MethodGet, "/_/api/page.html", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.requests()) != 1 {
		t.Fatalf("the fake service saw %d requests, want 1", len(fake.requests()))
	}
	got := strings.Join(header.Values("Set-Cookie"), "\n")
	want := strings.Join([]string{
		"isAdminOfCurrentResource=true; Path=/; SameSite=Lax",
		"isLoggedIn=true; Path=/; SameSite=Lax",
		"appOnly=1; Path=/",
	}, "\n")
	if got != want {
		t.Fatalf("Set-Cookie =\n%s\nwant\n%s", got, want)
	}
}

// TestForwardDataAPIPlainPageStaysNative proves a document with no page store
// keeps Go's own data API.
func TestForwardDataAPIPlainPageStaysNative(t *testing.T) {
	fake := newForwardService(t, nil)
	server, _, _, dir := forwardRoot(t, fake.peer())
	consultWriteFile(t, dir, "plain.html", []byte(forwardPlain))

	status, _, answer := forwardSend(t, server, http.MethodGet, "/_/api/plain.html", nil, nil)
	if status != http.StatusBadRequest || !strings.Contains(string(answer), "No api rules tag") {
		t.Fatalf("status = %d, answer = %q, want Go's own 400", status, answer)
	}
	if seen := fake.requests(); len(seen) != 0 {
		t.Fatalf("the fake service saw %d requests, want none", len(seen))
	}
}

// TestForwardServiceDownIsRecoverable proves an unreachable service answers the
// recoverable 503 instead of a broken page.
func TestForwardServiceDownIsRecoverable(t *testing.T) {
	fake := newForwardService(t, nil)
	peer := fake.peer()
	fake.server.Close()
	server, _, _, _ := forwardRoot(t, peer)

	status, header, answer := forwardSend(t, server, http.MethodGet, "/_/sync?document-url=x", nil, nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if got := header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if !strings.Contains(string(answer), `"reason":"service-unavailable"`) || !strings.Contains(string(answer), `"retryable":true`) {
		t.Fatalf("answer = %q, want the recoverable refusal", answer)
	}
}

// TestForwardUnconfirmedWriteIsRefused proves a data API write to an unconfirmed
// document is the recoverable `unconfirmed` 503 without reaching the service.
func TestForwardUnconfirmedWriteIsRefused(t *testing.T) {
	fake := newForwardService(t, nil)
	server, _, link, dir := forwardRoot(t, fake.peer())
	consultWriteFile(t, dir, "page.html", []byte(forwardPage))
	link.markUnconfirmed(upathJoin(dir, "page.html"), "root-consult", "page.html")

	status, header, answer := forwardSend(t, server, http.MethodPost, "/_/api/page.html", strings.NewReader(`{"title":"x"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if got := header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if !strings.Contains(string(answer), `"reason":"unconfirmed"`) {
		t.Fatalf("answer = %q, want the unconfirmed refusal", answer)
	}
	if seen := fake.requests(); len(seen) != 0 {
		t.Fatalf("the fake service saw %d requests, want none", len(seen))
	}
}

// TestForwardWithoutServiceKeepsNative proves a root with no service still
// answers the live stream itself.
func TestForwardWithoutServiceKeepsNative(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the root server reads that checkout's bundled runtime release")
	}
	dir := consultTestRoot(t)
	handler, err := newRootHandler(rootSpec{id: "root-consult", path: dir}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/_/sync?document-url="+url.QueryEscape(server.URL+"/plain.html"), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want Go's own live stream", got)
	}
}
