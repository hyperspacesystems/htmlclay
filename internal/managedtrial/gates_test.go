//go:build managedtrial

package managedtrial

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatesHostHeaderTable(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"localhost", true},
		{"localhost:4321", true},
		{"127.0.0.1", true},
		{"127.0.0.1:4321", true},
		{"127.1.2.3:80", true},
		{"[::1]", true},
		{"[::1]:4321", true},
		{"[0:0:0:0:0:0:0:1]", true},
		{"[0:0:0:0:0:0:0:1]:4321", true},
		{"[::2]:4321", false},
		{"[2001:db8::1]:4321", false},
		{"evil.com", false},
		{"evil.com:4321", false},
		{"", false},
		{"localhost.evil.com", false},
		{"localhost@evil.com", false},
		{"evil.com/localhost", false},
	}
	for _, testCase := range cases {
		if got := isLoopbackHostHeader(testCase.value); got != testCase.want {
			t.Errorf("isLoopbackHostHeader(%q) = %v, want %v", testCase.value, got, testCase.want)
		}
	}
}

func TestGatesOriginHelpers(t *testing.T) {
	cases := []struct {
		origin   string
		loopback bool
		port     int
		parsed   bool
	}{
		{"http://localhost:4321", true, 4321, true},
		{"http://localhost", true, 80, true},
		{"https://localhost", true, 443, true},
		{"http://127.0.0.1:9999", true, 9999, true},
		{"http://[::1]:8080", true, 8080, true},
		{"https://evil.example", false, 443, true},
		{"null", false, 0, false},
		{"", false, 0, false},
	}
	for _, testCase := range cases {
		if got := isLoopbackOrigin(testCase.origin); got != testCase.loopback {
			t.Errorf("isLoopbackOrigin(%q) = %v, want %v", testCase.origin, got, testCase.loopback)
		}
		port, ok := originPort(testCase.origin)
		if ok != testCase.parsed || (ok && port != testCase.port) {
			t.Errorf("originPort(%q) = %d, %v, want %d, %v", testCase.origin, port, ok, testCase.port, testCase.parsed)
		}
	}
}

func TestGatesIsOwnOrigin(t *testing.T) {
	if !isOwnOrigin("http://localhost:4321", 0, 4321) {
		t.Error("the root port must count as this app's own origin")
	}
	if !isOwnOrigin("http://localhost:5432", 5432, 4321) {
		t.Error("the connection's local port must count as this app's own origin")
	}
	if isOwnOrigin("http://127.0.0.1:9999", 5432, 4321) {
		t.Error("another loopback port is another folder's page")
	}
	if isOwnOrigin("null", 5432, 4321) {
		t.Error("an origin with no parseable port is not this app's own origin")
	}
}

func TestGatesStripSystemRouteMarker(t *testing.T) {
	cases := []struct{ value, want string }{
		{"/_/save", "/save"},
		{"/_/live-sync/stream?page-url=/blog/post.html", "/live-sync/stream?page-url=/blog/post.html"},
		{"/_/live-sync/save", "/live-sync/save"},
		{"/save", "/save"},
		{"/live-sync/stream?page-url=/x.html", "/live-sync/stream?page-url=/x.html"},
		{"/f/index.html", "/f/index.html"},
		{"/", "/"},
		{"/_foo", "/_foo"},
		{"/_", "/_"},
		{"/file_with_underscore.html", "/file_with_underscore.html"},
	}
	for _, testCase := range cases {
		if got := stripSystemRouteMarker(testCase.value); got != testCase.want {
			t.Errorf("stripSystemRouteMarker(%q) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

func TestGatesMarkerRewrite(t *testing.T) {
	path, marked, ok := gate(httptest.NewRecorder(), newGateRequest("GET", "/_/save", nil), 4321)
	if !ok || path != "/save" || !marked {
		t.Errorf("gate(/_/save) = %q, %v, %v, want /save, true, true", path, marked, ok)
	}

	path, marked, ok = gate(httptest.NewRecorder(), newGateRequest("GET", "/save", nil), 4321)
	if !ok || path != "/save" || marked {
		t.Errorf("gate(/save) = %q, %v, %v, want /save, false, true", path, marked, ok)
	}

	recorder := httptest.NewRecorder()
	path, marked, ok = gate(recorder, newGateRequest("GET", "/_/foo.html", nil), 4321)
	if ok || marked || path != "" {
		t.Errorf("gate(/_/foo.html) = %q, %v, %v, want the marker 404", path, marked, ok)
	}
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
	if recorder.Body.String() != "File not found" {
		t.Errorf("body = %q, want %q", recorder.Body.String(), "File not found")
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", got)
	}
	if got := recorder.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("a marker 404 must carry no cookie, got %v", got)
	}
}

func TestGatesCookiesOnPassingRequests(t *testing.T) {
	recorder := httptest.NewRecorder()
	if _, _, ok := gate(recorder, newGateRequest("GET", "/index.html", nil), 4321); !ok {
		t.Fatal("a loopback GET must pass")
	}
	cookies := recorder.Header().Values("Set-Cookie")
	want := []string{
		"isAdminOfCurrentResource=true; Path=/; SameSite=Lax",
		"isLoggedIn=true; Path=/; SameSite=Lax",
	}
	if len(cookies) != len(want) {
		t.Fatalf("cookies = %v, want %v", cookies, want)
	}
	for i := range want {
		if cookies[i] != want[i] {
			t.Errorf("cookie %d = %q, want %q", i, cookies[i], want[i])
		}
	}
}

func TestGatesHostRunsBeforeOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := newGateRequest("POST", "/save", map[string]string{
		"Host":   "evil.com",
		"Origin": "https://evil.example",
	})
	if _, _, ok := gate(recorder, request, 4321); ok {
		t.Fatal("a rebound Host must be refused")
	}
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", recorder.Code)
	}
	if recorder.Body.String() != "Invalid Host header" {
		t.Errorf("body = %q, want the Host refusal, not the Origin one", recorder.Body.String())
	}
	if got := recorder.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("a gate refusal must carry no cookie, got %v", got)
	}
}

func TestGatesOriginGate(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		local   int
		ok      bool
	}{
		{"cross-origin POST is refused", "POST", "/save", map[string]string{"Origin": "https://evil.example"}, 0, false},
		{"Origin null is refused", "POST", "/save", map[string]string{"Origin": "null"}, 0, false},
		{"cross-site is refused with no Origin", "POST", "/save", map[string]string{"Sec-Fetch-Site": "cross-site"}, 0, false},
		{"same-site is refused", "POST", "/save", map[string]string{"Sec-Fetch-Site": "same-site"}, 0, false},
		{"the root port passes", "POST", "/save", map[string]string{"Origin": "http://localhost:4321", "Sec-Fetch-Site": "same-origin"}, 0, true},
		{"the connection's own port passes", "POST", "/save", map[string]string{"Origin": "http://127.0.0.1:5432"}, 5432, true},
		{"another loopback port is refused", "POST", "/save", map[string]string{"Origin": "http://127.0.0.1:9999"}, 0, false},
		{"no Origin passes", "POST", "/save", nil, 0, true},
		{"GET is never blocked", "GET", "/index.html", map[string]string{"Origin": "https://evil.example"}, 0, true},
		{"a safe method with a bad fetch site passes", "GET", "/index.html", map[string]string{"Sec-Fetch-Site": "cross-site"}, 0, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := newGateRequest(testCase.method, testCase.path, testCase.headers)
			if testCase.local != 0 {
				request = withLocalPort(request, testCase.local)
			}
			_, _, ok := gate(recorder, request, 4321)
			if ok != testCase.ok {
				t.Fatalf("gate passed = %v, want %v (status %d, body %q)", ok, testCase.ok, recorder.Code, recorder.Body.String())
			}
			if testCase.ok {
				return
			}
			if recorder.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("content-type = %q", got)
			}
			if got := recorder.Body.String(); got != `{"msg":"Cross-origin requests are not allowed.","msgType":"error"}` {
				t.Errorf("body = %q", got)
			}
		})
	}
}

func TestGatesSyncDataRefusalCarriesCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := newGateRequest("POST", "/_/sync/data", map[string]string{"Origin": "https://evil.example"})
	if _, _, ok := gate(recorder, request, 4321); ok {
		t.Fatal("a cross-origin POST must be refused")
	}
	want := `{"msg":"Cross-origin requests are not allowed.","msgType":"error","code":"forbidden"}`
	if got := recorder.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestGatesBareAllowed(t *testing.T) {
	cases := []struct {
		path string
		bare bool
	}{
		{"/sync/data", false},
		{"/presence", false},
		{"/sync/worker.js", false},
		{"/meta", false},
		{"/uploads/", false},
		{"/uploads/a.png", false},
		{"/versions/a.html", false},
		{"/version/a.html/2026-01-01.html", false},
		{"/restore/a.html/2026-01-01.html", false},
		{"/api", false},
		{"/api/", false},
		{"/api/index.html", false},
		{"/save", true},
		{"/index.html", true},
		{"/", true},
		{"/apix", true},
		{"/uploads", true},
		{"/sync", true},
	}
	for _, testCase := range cases {
		if got := bareAllowed(testCase.path, false); got != testCase.bare {
			t.Errorf("bareAllowed(%q, false) = %v, want %v", testCase.path, got, testCase.bare)
		}
		if !bareAllowed(testCase.path, true) {
			t.Errorf("bareAllowed(%q, true) must be true", testCase.path)
		}
	}
}

func TestGatesRootHandlerCarriesCookiesAndPoweredBy(t *testing.T) {
	handler := newRootHandler(4321)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newGateRequest("GET", "/index.html", nil))
	if got := recorder.Header().Get("X-Powered-By"); got != "Express" {
		t.Errorf("X-Powered-By = %q", got)
	}
	if got := len(recorder.Header().Values("Set-Cookie")); got != 2 {
		t.Errorf("cookies = %d, want 2", got)
	}
	if recorder.Code != http.StatusNotFound || recorder.Body.String() != "Not found" {
		t.Errorf("placeholder = %d %q", recorder.Code, recorder.Body.String())
	}
}

func newGateRequest(method, path string, headers map[string]string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Host = "127.0.0.1:4321"
	for key, value := range headers {
		if strings.EqualFold(key, "Host") {
			request.Host = value
			continue
		}
		request.Header.Set(key, value)
	}
	return request
}

func withLocalPort(request *http.Request, port int) *http.Request {
	address := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
	return request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, address))
}
