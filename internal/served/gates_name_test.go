package served

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGateByName is a served folder's gate: every folder shares one port, so only
// the exact Host and Origin of the folder's own address count as its own.
func TestGateByName(t *testing.T) {
	const own = "notes.htmlclay.localhost:1212"
	policy := namePolicy(own)
	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		ok      bool
		body    string
	}{
		{"own origin write", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://" + own, "Sec-Fetch-Site": "same-origin"}, true, ""},
		{"another folder's origin write", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://journal.htmlclay.localhost:1212"}, false, "cross"},
		{"a loose file's origin write", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://htmlclay3.localhost:1212"}, false, "cross"},
		{"the dashboard's origin write", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://dashboard.htmlclay.localhost:1212"}, false, "cross"},
		{"own name on another port", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://notes.htmlclay.localhost:4321"}, false, "cross"},
		{"a bare loopback origin", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://127.0.0.1:1212"}, false, "cross"},
		{"Origin null", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "null"}, false, "cross"},
		{"a script: no Origin, no Sec-Fetch-Site", "POST", "/_/save/index.html", map[string]string{"Host": own}, true, ""},
		{"same-site write with no Origin", "POST", "/_/save/index.html", map[string]string{"Host": own, "Sec-Fetch-Site": "same-site"}, false, "cross"},
		{"same-site GET of meta", "GET", "/_/meta", map[string]string{"Host": own, "Sec-Fetch-Site": "same-site"}, false, "cross"},
		{"same-site GET of the data API", "GET", "/_/api/index.html", map[string]string{"Host": own, "Sec-Fetch-Site": "same-site"}, false, "cross"},
		{"own GET of meta", "GET", "/_/meta", map[string]string{"Host": own, "Sec-Fetch-Site": "same-origin"}, true, ""},
		{"same-site iframe navigation", "GET", "/index.html", map[string]string{"Host": own, "Sec-Fetch-Site": "same-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "iframe"}, true, ""},
		{"wrong Host", "GET", "/index.html", map[string]string{"Host": "journal.htmlclay.localhost:1212"}, false, "host"},
		{"bare loopback Host", "GET", "/index.html", map[string]string{"Host": "127.0.0.1:1212"}, false, "host"},
		{"rebound Host", "POST", "/_/save/index.html", map[string]string{"Host": "evil.example", "Origin": "http://" + own}, false, "host"},
		{"upper-case Host", "GET", "/index.html", map[string]string{"Host": "Notes.HTMLClay.localhost:1212"}, true, ""},
		{"upper-case Origin", "POST", "/_/save/index.html", map[string]string{"Host": own, "Origin": "http://NOTES.htmlclay.localhost:1212"}, true, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			_, _, ok := gate(recorder, newGateRequest(testCase.method, testCase.path, testCase.headers), policy)
			if ok != testCase.ok {
				t.Fatalf("gate passed = %v, want %v (status %d, body %q)", ok, testCase.ok, recorder.Code, recorder.Body.String())
			}
			if testCase.ok {
				return
			}
			if recorder.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", recorder.Code)
			}
			want := "Invalid Host header"
			if testCase.body == "cross" {
				want = `{"msg":"Cross-origin requests are not allowed.","msgType":"error"}`
			}
			if got := recorder.Body.String(); got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if got := recorder.Header().Values("Set-Cookie"); len(got) != 0 {
				t.Errorf("a refusal must carry no cookie, got %v", got)
			}
		})
	}
}

// TestGateByPortUnchanged keeps the trial binary's gate as it was: any loopback
// host, and a same-site GET of a system route still passes.
func TestGateByPortUnchanged(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := newGateRequest("GET", "/_/meta", map[string]string{"Sec-Fetch-Site": "same-site"})
	if _, _, ok := gate(recorder, request, portPolicy(4321)); !ok {
		t.Fatalf("the port gate refused a same-site GET (status %d)", recorder.Code)
	}
}
