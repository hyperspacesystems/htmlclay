package dashboard

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// What a browser sends for each way a page on the dashboard's origin can ask for
// one of its addresses. A page cannot set or remove any of these headers.
var (
	typedAddress = map[string]string{"Sec-Fetch-Site": "none", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Sec-Fetch-User": "?1"}
	openedTab    = map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Sec-Fetch-User": "?1"}
	returnedTab  = map[string]string{"Sec-Fetch-Site": "same-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Sec-Fetch-User": "?1"}
	scriptLoads  = map[string]map[string]string{
		"fetch":                {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"},
		"a same-origin fetch":  {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "same-origin", "Sec-Fetch-Dest": "empty"},
		"a no-cors fetch":      {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "no-cors", "Sec-Fetch-Dest": "empty"},
		"an event source":      {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty", "Accept": "text/event-stream"},
		"a frame":              {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "iframe"},
		"an object":            {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "object"},
		"an embed":             {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "embed"},
		"a script element":     {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "no-cors", "Sec-Fetch-Dest": "script"},
		"a worker":             {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "same-origin", "Sec-Fetch-Dest": "worker"},
		"a service worker":     {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "same-origin", "Sec-Fetch-Dest": "serviceworker", "Service-Worker": "script"},
		"a document by a mode": {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "document"},
		// What a service worker sends when it passes a tab's load on with fetch(),
		// and the copy of a tab's load the browser hands a service worker.
		"a passed-on tab load": {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "empty"},
		"a preloaded tab load": {"Sec-Fetch-Site": "none", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Service-Worker-Navigation-Preload": "true"},
	}
)

// with is base and more in one map.
func with(base map[string]string, more ...string) map[string]string {
	out := map[string]string{}
	for name, value := range base {
		out[name] = value
	}
	for i := 0; i+1 < len(more); i += 2 {
		out[more[i]] = more[i+1]
	}
	return out
}

func doBody(s *Server, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+target, strings.NewReader(body))
	r.Host = host
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

// TestM3DashboardOriginClean is step 7.45d. The dashboard's address was the first
// document site's, so a page of that site still open in a tab is same-origin with
// the dashboard and the browser sends it the dashboard's session. Each subtest is
// one thing such a page can do, and what it gets.
func TestM3DashboardOriginClean(t *testing.T) {
	const mark = "5f1d3c0b9a8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817060504"

	t.Run("a script's request for the page gets the page without the token", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		c := enterWith(t, s, "/")
		cookie := c.Name + "=" + c.Value
		for _, page := range []string{"/", "/recent", "/roots/r1/files", "/recent/d0123456789abcd/versions"} {
			for name, load := range scriptLoads {
				w := do(s, http.MethodGet, page, with(load, "Cookie", cookie))
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<div id="workspace-root">`) {
					t.Fatalf("%s for %s = %d, want the page", name, page, w.Code)
				}
				if body := w.Body.String(); strings.Contains(body, s.token) || strings.Contains(body, "workspace-token") {
					t.Fatalf("%s for %s was served the workspace token", name, page)
				}
			}
			// The same address in a tab of its own still carries it, or the dashboard
			// would not work; so does a browser that sends neither header.
			for name, load := range map[string]map[string]string{"a typed address": typedAddress, "an opened tab": openedTab, "a browser without the headers": {}} {
				w := do(s, http.MethodGet, page, with(load, "Cookie", cookie))
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<meta name="workspace-token" content="`+s.token+`">`) {
					t.Fatalf("%s for %s = %d without the token", name, page, w.Code)
				}
			}
		}
	})

	t.Run("a tab opened by another page is cut off from that page", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		c := enterWith(t, s, "/")
		for name, load := range map[string]map[string]string{"an opened tab": openedTab, "a typed address": typedAddress} {
			w := do(s, http.MethodGet, "/roots/r1/files", with(load, "Cookie", c.Name+"="+c.Value))
			if !strings.Contains(w.Body.String(), s.token) {
				t.Fatalf("%s has no token, so its opener policy protects nothing", name)
			}
			h := w.Header()
			if got := h.Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
				t.Fatalf("%s: Cross-Origin-Opener-Policy = %q, want same-origin", name, got)
			}
			if h.Get("Content-Security-Policy") != "frame-ancestors 'none'" || h.Get("X-Frame-Options") != "DENY" {
				t.Fatalf("%s may be framed: %v", name, h)
			}
		}
	})

	t.Run("the session without the page's token reaches none of the app's routes", func(t *testing.T) {
		reached := 0
		s, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(http.StatusOK) }))
		c := enterWith(t, s, "/")
		cookie := c.Name + "=" + c.Value
		fetch := with(scriptLoads["fetch"], "Cookie", cookie)
		post := with(fetch, "Origin", "http://"+host, "Content-Type", "application/json")
		form := with(openedTab, "Cookie", cookie, "Origin", "http://"+host, "Content-Type", "text/plain")
		action := `{"id":"a1","type":"folder.stopServing","args":{"id":"f1"}}`
		asks := []struct {
			name, method, target, body string
			headers                    map[string]string
		}{
			{"a fetch of the snapshot", http.MethodGet, "/api/app/snapshot", "", fetch},
			{"an event source on the events", http.MethodGet, "/api/app/events", "", with(scriptLoads["an event source"], "Cookie", cookie)},
			{"a fetch that posts an action", http.MethodPost, "/api/app/actions", action, post},
			{"a form that posts an action", http.MethodPost, "/api/app/actions", action, form},
			{"a fetch of a folder's list", http.MethodGet, "/api/app/folders/f1/list", "", fetch},
			{"a fetch of a document's versions", http.MethodGet, "/api/app/documents/d1/versions", "", fetch},
			{"a fetch of one version", http.MethodGet, "/api/app/documents/d1/versions/v1", "", fetch},
			{"a fetch that restores a version", http.MethodPost, "/api/app/documents/d1/versions/v1/restore", "", with(post, "If-Match", `"e"`)},
			{"the snapshot in an opened tab", http.MethodGet, "/api/app/snapshot", "", with(openedTab, "Cookie", cookie)},
			{"one version in an opened tab", http.MethodGet, "/api/app/documents/d1/versions/v1", "", with(openedTab, "Cookie", cookie)},
			{"the snapshot in a frame", http.MethodGet, "/api/app/snapshot", "", with(scriptLoads["a frame"], "Cookie", cookie)},
			{"a request with only the cookie", http.MethodGet, "/api/app/snapshot", "", map[string]string{"Cookie": cookie}},
		}
		for _, ask := range asks {
			if w := doBody(s, ask.method, ask.target, ask.body, ask.headers); w.Code != http.StatusForbidden {
				t.Fatalf("%s with the session alone = %d, want 403", ask.name, w.Code)
			}
		}
		if reached != 0 {
			t.Fatalf("%d requests with the session alone reached the app", reached)
		}
		// The entry page's own script sends both, and every one of those arrives.
		for _, ask := range asks {
			if w := doBody(s, ask.method, ask.target, ask.body, with(ask.headers, "X-Workspace-Token", s.token)); w.Code != http.StatusOK {
				t.Fatalf("%s with the session and the token = %d, want 200", ask.name, w.Code)
			}
		}
		if reached != len(asks) {
			t.Fatalf("%d of %d requests with the session and the token reached the app", reached, len(asks))
		}
		// And the token without the session is still not enough.
		if w := do(s, http.MethodGet, "/api/app/snapshot", with(scriptLoads["fetch"], "X-Workspace-Token", s.token)); w.Code != http.StatusForbidden || reached != len(asks) {
			t.Fatalf("the token without the session = %d", w.Code)
		}
	})

	t.Run("a folder's document opens in a tab without the token, and nothing else does", func(t *testing.T) {
		var reached []string
		s, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = append(reached, r.Method+" "+r.URL.RequestURI())
			w.WriteHeader(http.StatusSeeOther)
		}))
		c := enterWith(t, s, "/")
		cookie := c.Name + "=" + c.Value
		open := "/api/app/folders/f1/open?path=notes%2Fa.html"
		if w := do(s, http.MethodGet, open, with(openedTab, "Cookie", cookie)); w.Code != http.StatusSeeOther || len(reached) != 1 || reached[0] != "GET "+open {
			t.Fatalf("the open in a tab = %d, reached %v", w.Code, reached)
		}
		refused := []struct {
			name, method, target string
			headers              map[string]string
		}{
			{"a fetch of the open", http.MethodGet, open, with(scriptLoads["fetch"], "Cookie", cookie)},
			{"a no-cors fetch of the open", http.MethodGet, open, with(scriptLoads["a no-cors fetch"], "Cookie", cookie)},
			{"the open in a frame", http.MethodGet, open, with(scriptLoads["a frame"], "Cookie", cookie)},
			{"the open as a form's post", http.MethodPost, open, with(openedTab, "Cookie", cookie, "Origin", "http://"+host)},
			{"the open without the session", http.MethodGet, open, openedTab},
			{"the open from a served folder's page", http.MethodGet, open, with(openedTab, "Cookie", cookie, "Sec-Fetch-Site", "same-site")},
			{"the open from another site", http.MethodGet, open, with(openedTab, "Cookie", cookie, "Sec-Fetch-Site", "cross-site")},
			{"a folder's list in a tab", http.MethodGet, "/api/app/folders/f1/list", with(openedTab, "Cookie", cookie)},
			{"a path below the open", http.MethodGet, "/api/app/folders/f1/open/list", with(openedTab, "Cookie", cookie)},
			{"a path that only ends like the open", http.MethodGet, "/api/app/documents/d1/versions/open", with(openedTab, "Cookie", cookie)},
			{"a folder named by two segments", http.MethodGet, "/api/app/folders/f1/f2/open", with(openedTab, "Cookie", cookie)},
		}
		for _, ask := range refused {
			if w := do(s, ask.method, ask.target, ask.headers); w.Code != http.StatusForbidden {
				t.Fatalf("%s = %d, want 403", ask.name, w.Code)
			}
		}
		if len(reached) != 1 {
			t.Fatalf("a refused request reached the app: %v", reached[1:])
		}
	})

	t.Run("a browser is cleared on its first entry and its first page, and not again", func(t *testing.T) {
		at := &clock{at: time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)}
		s, err := New(Options{Host: host, ProductName: "HTML Clay", Now: at.now, CleanMark: mark})
		if err != nil {
			t.Fatal(err)
		}
		const recordName = "htmlclay-origin-clean-1212"
		// visit is one opening of the dashboard in a browser that holds cookies: the
		// entry, then the page the entry sends the browser to.
		visit := func(cookies string) (entry, page *httptest.ResponseRecorder) {
			t.Helper()
			link, err := s.EntryURL("/recent")
			if err != nil {
				t.Fatal(err)
			}
			headers := with(typedAddress)
			if cookies != "" {
				headers["Cookie"] = cookies
			}
			entry = do(s, http.MethodGet, pathOf(t, link), headers)
			if entry.Code != http.StatusFound || entry.Header().Get("Location") != "/recent" {
				t.Fatalf("the entry = %d to %q", entry.Code, entry.Header().Get("Location"))
			}
			set := entry.Result().Cookies()
			if len(set) != 1 || set[0].Name != "htmlclay-dashboard-1212" || set[0].Value != s.session {
				t.Fatalf("the entry's cookies = %q, want the session alone", entry.Header()["Set-Cookie"])
			}
			jar := set[0].Name + "=" + set[0].Value
			if cookies != "" {
				jar = cookies + "; " + jar
			}
			page = do(s, http.MethodGet, "/recent", with(typedAddress, "Cookie", jar))
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), s.token) {
				t.Fatalf("the page after the entry = %d", page.Code)
			}
			return entry, page
		}
		cleared := func(name string, w *httptest.ResponseRecorder) {
			t.Helper()
			if got := w.Header().Values("Clear-Site-Data"); len(got) != 1 || got[0] != `"storage"` {
				t.Fatalf("%s: Clear-Site-Data = %q, want one header, \"storage\"", name, got)
			}
		}
		untouched := func(name string, w *httptest.ResponseRecorder) {
			t.Helper()
			if got := w.Header().Values("Clear-Site-Data"); len(got) != 0 {
				t.Fatalf("%s: a browser that was cleared was cleared again: %q", name, got)
			}
		}

		entry, page := visit("")
		cleared("the first entry", entry)
		cleared("the first page", page)
		left := page.Result().Cookies()
		if len(left) != 1 {
			t.Fatalf("the first page's cookies = %q, want the record alone", page.Header()["Set-Cookie"])
		}
		record := left[0]
		if record.Name != recordName || record.Value != mark || !record.HttpOnly || record.SameSite != http.SameSiteLaxMode ||
			record.Path != "/" || record.MaxAge != 400*86400 || record.Domain != "" {
			t.Fatalf("the record of the clearing = %q", page.Header()["Set-Cookie"])
		}

		// The same browser again, in this run of the app and in the next, where its
		// session is new and its record is not.
		browser := recordName + "=" + mark
		for name, cookies := range map[string]string{
			"the same run": browser + "; htmlclay-dashboard-1212=" + s.session,
			"the next run": browser + "; htmlclay-dashboard-1212=an-earlier-run",
			"no session":   browser,
		} {
			entry, page := visit(cookies)
			untouched(name+", the entry", entry)
			untouched(name+", the page", page)
			if len(page.Result().Cookies()) != 0 {
				t.Fatalf("%s: a later page set %q", name, page.Header()["Set-Cookie"])
			}
		}

		// Another browser has no record, and a page cannot write one: only the
		// config's value counts, on whichever path the cookie sits.
		for name, cookies := range map[string]string{
			"another browser":                 "",
			"a record a page wrote":           recordName + "=1",
			"a record with one character off": recordName + "=" + mark[:len(mark)-1] + "5",
			"a record for another port":       "htmlclay-origin-clean-1213=" + mark,
			"the session as the record":       recordName + "=" + s.session,
		} {
			entry, page := visit(cookies)
			cleared(name+", the entry", entry)
			cleared(name+", the page", page)
			if left := page.Result().Cookies(); len(left) != 1 || left[0].Name != recordName || left[0].Value != mark {
				t.Fatalf("%s: the page left %q, want the record", name, page.Header()["Set-Cookie"])
			}
		}
		entry, page = visit(recordName + "=1; " + browser)
		untouched("a cookie a page wrote in front of the record, the entry", entry)
		untouched("a cookie a page wrote in front of the record, the page", page)

		// Only a tab's own load of a page leaves the record, so a script's fetch
		// cannot use it up before the browser was cleared, and nothing but a spent
		// ticket or a page clears.
		session := "htmlclay-dashboard-1212=" + s.session
		for name, load := range scriptLoads {
			w := do(s, http.MethodGet, "/recent", with(load, "Cookie", session))
			if w.Code != http.StatusOK || len(w.Header().Values("Clear-Site-Data")) != 0 || len(w.Result().Cookies()) != 0 {
				t.Fatalf("%s for the page = %d, Clear-Site-Data %q, cookies %q", name, w.Code, w.Header().Values("Clear-Site-Data"), w.Header()["Set-Cookie"])
			}
		}
		if w := do(s, http.MethodGet, "/enter?ticket=made-up", typedAddress); w.Code != http.StatusForbidden || w.Header().Get("Clear-Site-Data") != "" || len(w.Result().Cookies()) != 0 {
			t.Fatalf("a refused entry = %d, Clear-Site-Data %q, cookies %q", w.Code, w.Header().Get("Clear-Site-Data"), w.Header()["Set-Cookie"])
		}
		for _, target := range []string{"/local-app/missing.js", "/api/app/snapshot", "/nowhere"} {
			if w := do(s, http.MethodGet, target, with(typedAddress, "Cookie", session)); w.Header().Get("Clear-Site-Data") != "" || len(w.Result().Cookies()) != 0 {
				t.Fatalf("%s: Clear-Site-Data %q, cookies %q", target, w.Header().Get("Clear-Site-Data"), w.Header()["Set-Cookie"])
			}
		}

		// An address no document site ever answered at has nothing to clear.
		plain, _ := newTestServer(t, nil)
		c := enterWith(t, plain, "/")
		link, err := plain.EntryURL("/")
		if err != nil {
			t.Fatal(err)
		}
		again := do(plain, http.MethodGet, pathOf(t, link), typedAddress)
		first := do(plain, http.MethodGet, "/", with(typedAddress, "Cookie", c.Name+"="+c.Value))
		if again.Code != http.StatusFound || again.Header().Get("Clear-Site-Data") != "" || len(again.Result().Cookies()) != 1 {
			t.Fatalf("an entry with nothing to clear = %d, Clear-Site-Data %q, cookies %q", again.Code, again.Header().Get("Clear-Site-Data"), again.Header()["Set-Cookie"])
		}
		if first.Code != http.StatusOK || first.Header().Get("Clear-Site-Data") != "" || len(first.Result().Cookies()) != 0 {
			t.Fatalf("a page with nothing to clear = %d, Clear-Site-Data %q, cookies %q", first.Code, first.Header().Get("Clear-Site-Data"), first.Header()["Set-Cookie"])
		}
	})

	t.Run("a tab a page on this origin opens goes out through the old host and comes back cut off", func(t *testing.T) {
		const oldHost = "dashboard.htmlclay.localhost:1212"
		at := &clock{at: time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)}
		s, err := New(Options{Host: host, ProductName: "HTML Clay", Now: at.now, Detour: oldHost})
		if err != nil {
			t.Fatal(err)
		}
		c := enterWith(t, s, "/")
		cookie := c.Name + "=" + c.Value
		around := regexp.MustCompile(`^http://dashboard\.htmlclay\.localhost:1212(/[^?]*(?:\?tab=1&|\?))detour=([A-Za-z0-9_-]{20,})$`)
		// sentAround asserts w sends the tab out through the old host for target and
		// answers the address it comes back to, with its ticket.
		sentAround := func(name, target string, w *httptest.ResponseRecorder) (back, id string) {
			t.Helper()
			m := around.FindStringSubmatch(w.Header().Get("Location"))
			if w.Code != http.StatusFound || m == nil || strings.TrimRight(m[1], "?&") != target {
				t.Fatalf("%s = %d to %q, want a redirect through the old host for %s", name, w.Code, w.Header().Get("Location"), target)
			}
			if strings.Contains(w.Body.String(), s.token) || w.Header().Get("Cross-Origin-Opener-Policy") != "" || len(w.Result().Cookies()) != 0 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s: the redirect carries the token, an opener policy or a cookie, or may be stored: %v", name, w.Header())
			}
			return m[1] + "detour=" + m[2], m[2]
		}
		served := func(name string, w *httptest.ResponseRecorder) {
			t.Helper()
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), s.token) || w.Header().Get("Cross-Origin-Opener-Policy") != "same-origin" {
				t.Fatalf("%s = %d to %q, want the page with its token and opener policy", name, w.Code, w.Header().Get("Location"))
			}
		}
		opened := with(openedTab, "Cookie", cookie)
		returned := with(returnedTab, "Cookie", cookie)

		// A browser that reports the way back as same-site: the ticket is dropped
		// from the address, and the page is served there.
		for _, target := range []string{"/", "/recent", "/recent?tab=1", "/roots/r1/files"} {
			back, _ := sentAround("a tab opened on "+target, target, do(s, http.MethodGet, target, opened))
			dropped := do(s, http.MethodGet, back, returned)
			if dropped.Code != http.StatusFound || dropped.Header().Get("Location") != target || strings.Contains(dropped.Body.String(), s.token) {
				t.Fatalf("the way back to %s = %d to %q, want a redirect to %s", back, dropped.Code, dropped.Header().Get("Location"), target)
			}
			served("the tab back at "+target, do(s, http.MethodGet, target, returned))
		}

		// A browser that still reports it as same-origin is served on the ticket,
		// once, for the address the ticket was made for.
		back, id := sentAround("a tab opened on /recent", "/recent", do(s, http.MethodGet, "/recent", opened))
		served("the tab back with its ticket", do(s, http.MethodGet, back, opened))
		_, again := sentAround("the same ticket a second time", "/recent", do(s, http.MethodGet, back, opened))
		if again == id {
			t.Fatal("a spent ticket was handed out again")
		}
		sentAround("a made-up ticket", "/recent", do(s, http.MethodGet, "/recent?detour=made-up-made-up-made-up", opened))
		sentAround("an empty ticket", "/recent", do(s, http.MethodGet, "/recent?detour=", opened))
		_, other := sentAround("a tab opened on /needs-you", "/needs-you", do(s, http.MethodGet, "/needs-you", opened))
		sentAround("a ticket made for another address", "/recent", do(s, http.MethodGet, "/recent?detour="+other, opened))
		_, late := sentAround("a tab opened on /recent", "/recent", do(s, http.MethodGet, "/recent", opened))
		at.at = at.at.Add(TicketTTL)
		sentAround("a ticket past its time", "/recent", do(s, http.MethodGet, "/recent?detour="+late, opened))

		// The two kinds of ticket do not stand in for each other.
		entry, err := s.EntryURL("/recent")
		if err != nil {
			t.Fatal(err)
		}
		entering := strings.TrimPrefix(pathOf(t, entry), "/enter?ticket=")
		sentAround("an entry's ticket as the proof", "/recent", do(s, http.MethodGet, "/recent?detour="+entering, opened))
		_, proof := sentAround("a tab opened on /recent", "/recent", do(s, http.MethodGet, "/recent", opened))
		if w := do(s, http.MethodGet, "/enter?ticket="+proof, typedAddress); w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
			t.Fatalf("the proof as an entry's ticket = %d, cookies %q", w.Code, w.Header()["Set-Cookie"])
		}

		// A load no page here started is served where it is, and a script's fetch
		// is never sent around: it is served the page without the token.
		served("a typed address", do(s, http.MethodGet, "/recent", with(typedAddress, "Cookie", cookie)))
		served("a browser without the headers", do(s, http.MethodGet, "/recent", map[string]string{"Cookie": cookie}))
		served("a link on another site", do(s, http.MethodGet, "/recent", with(openedTab, "Cookie", cookie, "Sec-Fetch-Site", "cross-site")))
		for name, load := range scriptLoads {
			if w := do(s, http.MethodGet, "/recent", with(load, "Cookie", cookie, "Sec-Fetch-Site", "same-origin")); w.Code != http.StatusOK || strings.Contains(w.Body.String(), s.token) {
				t.Fatalf("%s for the page = %d to %q, want the page without the token", name, w.Code, w.Header().Get("Location"))
			}
		}
		if w := do(s, http.MethodGet, "/recent", map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}); w.Code != http.StatusForbidden || w.Header().Get("Location") != "" {
			t.Fatalf("a tab opened without a session = %d to %q, want 403", w.Code, w.Header().Get("Location"))
		}
	})

	t.Run("a cookie a page wrote cannot stand in front of the session", func(t *testing.T) {
		reached := 0
		s, _ := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(http.StatusOK) }))
		c := enterWith(t, s, "/")
		// A page on this host can set a cookie of the session's name on a longer
		// path, and the browser sends that one first.
		shadowed := c.Name + "=planted; " + c.Name + "=" + c.Value
		if w := do(s, http.MethodGet, "/recent", with(openedTab, "Cookie", shadowed)); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), s.token) {
			t.Fatalf("the page behind a planted cookie = %d", w.Code)
		}
		api := with(scriptLoads["fetch"], "Cookie", shadowed, "X-Workspace-Token", s.token)
		if w := do(s, http.MethodGet, "/api/app/snapshot", api); w.Code != http.StatusOK || reached != 1 {
			t.Fatalf("the app's route behind a planted cookie = %d", w.Code)
		}
		// The planted cookie is still no session.
		planted := c.Name + "=planted; " + c.Name + "=planted-too"
		if w := do(s, http.MethodGet, "/recent", with(openedTab, "Cookie", planted)); w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), s.token) {
			t.Fatalf("the page with only planted cookies = %d", w.Code)
		}
		if w := do(s, http.MethodGet, "/api/app/snapshot", with(api, "Cookie", planted)); w.Code != http.StatusForbidden || reached != 1 {
			t.Fatalf("the app's route with only planted cookies = %d", w.Code)
		}
	})
}
