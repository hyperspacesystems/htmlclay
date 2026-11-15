package dashboard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const host = "htmlclay.localhost:1212"

type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

func newTestServer(t *testing.T, api http.Handler) (*Server, *clock) {
	t.Helper()
	c := &clock{at: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)}
	s, err := New(Options{Host: host, ProductName: "HTML Clay", API: api, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func do(s *Server, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+target, nil)
	r.Host = host
	for k, v := range headers {
		if k == "Host" {
			r.Host = v
			continue
		}
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func pathOf(t *testing.T, entry string) string {
	t.Helper()
	u, err := url.Parse(entry)
	if err != nil {
		t.Fatal(err)
	}
	return u.RequestURI()
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "htmlclay-dashboard") {
			return c
		}
	}
	t.Fatalf("no session cookie in %v", w.Header()["Set-Cookie"])
	return nil
}

func enterWith(t *testing.T, s *Server, to string) *http.Cookie {
	t.Helper()
	entry, err := s.EntryURL(to)
	if err != nil {
		t.Fatal(err)
	}
	w := do(s, "GET", pathOf(t, entry), nil)
	if w.Code != http.StatusFound {
		t.Fatalf("enter = %d", w.Code)
	}
	return sessionCookie(t, w)
}

func TestEnterSetsTheSessionOnTheDashboardHostOnly(t *testing.T) {
	s, _ := newTestServer(t, nil)
	entry, err := s.EntryURL("/roots/r1/files")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(entry, "http://"+host+"/enter?ticket=") {
		t.Fatalf("entry = %s", entry)
	}
	w := do(s, "GET", pathOf(t, entry), nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/roots/r1/files" {
		t.Fatalf("enter = %d to %q", w.Code, w.Header().Get("Location"))
	}
	c := sessionCookie(t, w)
	raw := w.Header().Get("Set-Cookie")
	if c.Name != "htmlclay-dashboard-1212" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 86400 {
		t.Fatalf("cookie = %q", raw)
	}
	if strings.Contains(strings.ToLower(raw), "domain=") {
		t.Fatalf("the cookie names a domain, so it would reach the folders: %q", raw)
	}
}

func TestATicketIsSpentOnce(t *testing.T) {
	s, _ := newTestServer(t, nil)
	entry, _ := s.EntryURL("/")
	do(s, "GET", pathOf(t, entry), nil)
	w := do(s, "GET", pathOf(t, entry), nil)
	if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Fatalf("second use = %d with cookies %v", w.Code, w.Result().Cookies())
	}
}

func TestAnExpiredTicketIsRefused(t *testing.T) {
	s, c := newTestServer(t, nil)
	entry, _ := s.EntryURL("/")
	c.at = c.at.Add(TicketTTL)
	if w := do(s, "GET", pathOf(t, entry), nil); w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Fatalf("expired = %d", w.Code)
	}
	if w := do(s, "GET", "/enter?ticket=made-up", nil); w.Code != http.StatusForbidden {
		t.Fatalf("unknown ticket = %d", w.Code)
	}
}

func TestEntryURLOpensOnlyDashboardPages(t *testing.T) {
	s, _ := newTestServer(t, nil)
	for _, to := range []string{"//evil.example/", "http://evil.example/", "/api/roots", "/enter", "/roots/r1/secret", "/main.js"} {
		if _, err := s.EntryURL(to); err == nil {
			t.Fatalf("EntryURL(%q) succeeded", to)
		}
	}
	if _, err := s.EntryURL("/roots/r1/document?path=a.html"); err != nil {
		t.Fatalf("a document page with its path: %v", err)
	}
}

func TestThePageWithASession(t *testing.T) {
	s, _ := newTestServer(t, nil)
	c := enterWith(t, s, "/")
	w := do(s, "GET", "/roots/r1/files", map[string]string{"Cookie": c.Name + "=" + c.Value})
	if w.Code != http.StatusOK {
		t.Fatalf("page = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<meta name="workspace-token" content="` + s.token + `">`,
		`<meta name="workspace-product" content="HTML Clay">`,
		`<link rel="stylesheet" href="/` + s.manifest.Entry.CSS[0] + `">`,
		`<script type="module" src="/` + s.manifest.Entry.JavaScript + `">`,
		`<div id="workspace-root">`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks %s", want)
		}
	}
	h := w.Header()
	if h.Get("Content-Security-Policy") != "frame-ancestors 'none'" || h.Get("X-Frame-Options") != "DENY" ||
		h.Get("Referrer-Policy") != "no-referrer" || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("page headers = %v", h)
	}
}

func TestAPageWithoutASession(t *testing.T) {
	s, _ := newTestServer(t, nil)
	typed := do(s, "GET", "/roots/r1/document?path=a.html", map[string]string{"Sec-Fetch-Site": "none", "Sec-Fetch-Dest": "document"})
	if typed.Code != http.StatusFound || !strings.HasPrefix(typed.Header().Get("Location"), "/enter?ticket=") {
		t.Fatalf("typed address = %d to %q", typed.Code, typed.Header().Get("Location"))
	}
	entered := do(s, "GET", typed.Header().Get("Location"), nil)
	if entered.Header().Get("Location") != "/roots/r1/document?path=a.html" {
		t.Fatalf("the ticket from a typed address leads to %q", entered.Header().Get("Location"))
	}
	for name, headers := range map[string]map[string]string{
		"a link from another site": {"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Dest": "document"},
		"a folder's page":          {"Sec-Fetch-Site": "same-site", "Sec-Fetch-Dest": "document"},
		"a frame":                  {"Sec-Fetch-Site": "none", "Sec-Fetch-Dest": "iframe"},
		"a wrong cookie":           {"Sec-Fetch-Site": "cross-site", "Cookie": "htmlclay-dashboard-1212=guess"},
	} {
		w := do(s, "GET", "/", headers)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), TextHowToOpen) || len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s = %d %q", name, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), s.token) {
			t.Fatalf("%s got the workspace token", name)
		}
	}
}

func TestTheAPI(t *testing.T) {
	reached := 0
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(http.StatusOK) })
	s, _ := newTestServer(t, api)
	ok := map[string]string{"X-Workspace-Token": "", "Sec-Fetch-Site": "same-origin", "Origin": "http://" + host}
	ok["X-Workspace-Token"] = s.token
	if w := do(s, "GET", "/api/bootstrap", ok); w.Code != http.StatusOK || reached != 1 {
		t.Fatalf("admitted = %d, reached %d", w.Code, reached)
	}
	if w := do(s, "GET", "/api/bootstrap", map[string]string{"X-Workspace-Token": s.token}); w.Code != http.StatusOK || reached != 2 {
		t.Fatalf("no origin headers = %d", w.Code)
	}
	wrong := "x" + s.token[1:]
	if wrong == s.token {
		wrong = "y" + s.token[1:]
	}
	refused := map[string]map[string]string{
		"no token":         {"Sec-Fetch-Site": "same-origin"},
		"a wrong token":    {"X-Workspace-Token": wrong},
		"a folder origin":  {"X-Workspace-Token": s.token, "Origin": "http://notes.htmlclay.localhost:1212", "Sec-Fetch-Site": "same-site"},
		"another site":     {"X-Workspace-Token": s.token, "Sec-Fetch-Site": "cross-site"},
		"only the origin":  {"X-Workspace-Token": s.token, "Origin": "http://notes.htmlclay.localhost:1212"},
		"another host":     {"X-Workspace-Token": s.token, "Host": "notes.htmlclay.localhost:1212"},
		"the bare port":    {"X-Workspace-Token": s.token, "Host": "127.0.0.1:1212"},
		"a session cookie": {"Cookie": "htmlclay-dashboard-1212=" + s.session},
	}
	for name, headers := range refused {
		if w := do(s, "POST", "/api/roots/r1/commands", headers); w.Code != http.StatusForbidden {
			t.Fatalf("%s = %d", name, w.Code)
		}
	}
	if reached != 2 {
		t.Fatalf("a refused request reached the API (%d)", reached)
	}
}

func TestTheAPIWithNoService(t *testing.T) {
	s, _ := newTestServer(t, nil)
	w := do(s, "GET", "/api/bootstrap", map[string]string{"X-Workspace-Token": s.token})
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("no service = %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
}

func TestStaticFiles(t *testing.T) {
	s, _ := newTestServer(t, nil)
	js := do(s, "GET", "/"+s.manifest.Entry.JavaScript, nil)
	if js.Code != http.StatusOK || !strings.Contains(js.Header().Get("Content-Type"), "javascript") ||
		!strings.Contains(js.Header().Get("Cache-Control"), "immutable") || js.Body.Len() == 0 {
		t.Fatalf("entry script = %d %v", js.Code, js.Header())
	}
	font := do(s, "GET", "/"+s.manifest.Assets[0].Path, nil)
	if font.Code != http.StatusOK || font.Header().Get("Content-Type") != "font/woff2" {
		t.Fatalf("font = %d %q", font.Code, font.Header().Get("Content-Type"))
	}
	for _, p := range []string{"/manifest.json", "/assets/missing.woff2", "/../lock.json", "/lock.json"} {
		if w := do(s, "GET", p, nil); w.Code != http.StatusNotFound {
			t.Fatalf("%s = %d", p, w.Code)
		}
	}
	if w := do(s, "GET", "/"+s.manifest.Entry.JavaScript, map[string]string{"Host": "notes.htmlclay.localhost:1212"}); w.Code != http.StatusForbidden {
		t.Fatalf("a static file on another host = %d", w.Code)
	}
	if w := do(s, "POST", "/", nil); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / = %d", w.Code)
	}
}

// Owns names exactly the paths the server answers itself: every other path gets
// the missing answer, so the app may send it on without taking it from the
// dashboard.
func TestOwnsIsWhatTheServerAnswers(t *testing.T) {
	s, _ := newTestServer(t, nil)
	own := []string{
		"/", "/recent", "/needs-you", "/roots/r1/files", "/roots/r1/document", "/roots/r1/new",
		"/recent/d1/versions", "/trusted/t1/files", "/enter",
		"/api/roots", "/api/notes.htmlclay", "/api/app/snapshot",
		"/" + s.manifest.Entry.JavaScript, "/" + s.manifest.Assets[0].Path,
	}
	for _, css := range s.manifest.Entry.CSS {
		own = append(own, "/"+css)
	}
	for _, p := range own {
		if !s.Owns(p) {
			t.Errorf("Owns(%q) = false for a path the server answers", p)
		}
		if w := do(s, "GET", p, nil); w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), `"missing"`) {
			t.Errorf("GET %s is missing, so this proves nothing about it", p)
		}
	}
	for _, p := range []string{
		"/notes/a.htmlclay", "/a.htmlclay", "/recent/a.htmlclay", "/roots/r1/files/a.htmlclay",
		"/api.htmlclay", "/apis/a.htmlclay", "/enter/a.htmlclay", "/assets/a.htmlclay", "/manifest.json",
	} {
		if s.Owns(p) {
			t.Errorf("Owns(%q) = true for a path the server does not answer", p)
		}
		if w := do(s, "GET", p, nil); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"missing"`) {
			t.Errorf("GET %s = %d %s, want the missing answer", p, w.Code, w.Body.String())
		}
	}
}
