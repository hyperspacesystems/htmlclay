package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// originBrowser is one browser's cookies for the dashboard's host. They outlive
// the app's run, as a browser's do.
type originBrowser struct {
	cookies map[string]string
}

// headers is load with this browser's cookies.
func (b *originBrowser) headers(load map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range load {
		out[name] = value
	}
	pairs := []string{}
	for name, value := range b.cookies {
		pairs = append(pairs, name+"="+value)
	}
	sort.Strings(pairs)
	if len(pairs) > 0 {
		out["Cookie"] = strings.Join(pairs, "; ")
	}
	return out
}

// open is Open Workspace in the app's menu, opened in this browser: the entry,
// then the page it sends the browser to. It answers whether the browser was told
// to clear what the address stores, which the entry and the page say together.
func (b *originBrowser) open(t *testing.T, a *app) bool {
	t.Helper()
	entry, err := a.dashboard.EntryURL("/")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(entry)
	if err != nil {
		t.Fatal(err)
	}
	entered := servedRequest(t, a, http.MethodGet, u.Host, u.RequestURI(), "", b.headers(nestedNav))
	if entered.Code != http.StatusFound || entered.Header().Get("Location") != "/" {
		t.Fatalf("the entry at %s = %d to %q", u.Host, entered.Code, entered.Header().Get("Location"))
	}
	b.keep(entered)
	page := servedRequest(t, a, http.MethodGet, u.Host, "/", "", b.headers(nestedNav))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<meta name="workspace-token"`) {
		t.Fatalf("the page after the entry = %d", page.Code)
	}
	b.keep(page)
	atEntry, atPage := entered.Header().Values("Clear-Site-Data"), page.Header().Values("Clear-Site-Data")
	switch {
	case len(atEntry) == 0 && len(atPage) == 0:
		return false
	case len(atEntry) == 1 && atEntry[0] == `"storage"` && len(atPage) == 1 && atPage[0] == `"storage"`:
		return true
	}
	t.Fatalf("Clear-Site-Data at the entry = %q, at the page = %q", atEntry, atPage)
	return false
}

func (b *originBrowser) keep(rec *httptest.ResponseRecorder) {
	if b.cookies == nil {
		b.cookies = map[string]string{}
	}
	for _, c := range rec.Result().Cookies() {
		b.cookies[c.Name] = c.Value
	}
}

// cleanMark is the mark config.json holds, or "".
func (f *addressFixture) cleanMark(t *testing.T) string {
	t.Helper()
	var c struct {
		Mark string `json:"dashboardCleanMark"`
	}
	if err := json.Unmarshal([]byte(f.bytes(t)), &c); err != nil {
		t.Fatal(err)
	}
	return c.Mark
}

var cleanMarkShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// The dashboard answers where the first document site did. What that site left in
// a browser is cleared the first time each browser enters, a page of it that is
// still open gets nothing from the dashboard, and a config that never had a site
// there is left alone.
func TestM3DashboardOriginClean(t *testing.T) {
	t.Run("a config whose first site moved: each browser is cleared once, across restarts", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		f.seed(t, f.firstSite(f.docs))
		first := f.launch(t)
		if !first.loaded.MarkedFirstSiteOrigin || first.rt.cfg.Writes() != 1 {
			t.Fatalf("the start made a mark: %v, in %d config writes; want one write", first.loaded.MarkedFirstSiteOrigin, first.rt.cfg.Writes())
		}
		mark := f.cleanMark(t)
		if !cleanMarkShape.MatchString(mark) {
			t.Fatalf("the mark on disk = %q, want 64 hex", mark)
		}

		safari, chrome := &originBrowser{}, &originBrowser{}
		if !safari.open(t, first) {
			t.Fatal("the first entry from a browser cleared nothing")
		}
		if safari.cookies[fmt.Sprintf("htmlclay-origin-clean-%d", nestedTestPort)] != mark {
			t.Fatalf("the browser's record of the clearing is not the config's mark: %v", safari.cookies)
		}
		if safari.open(t, first) {
			t.Fatal("the same browser was cleared a second time")
		}
		if !chrome.open(t, first) {
			t.Fatal("a second browser's first entry cleared nothing")
		}
		first.shutdown()
		settled := f.bytes(t)

		second := f.launch(t)
		if second.loaded.MarkedFirstSiteOrigin || second.rt.cfg.Writes() != 0 {
			t.Fatalf("the restart made a mark again (%v) or wrote the config %d times", second.loaded.MarkedFirstSiteOrigin, second.rt.cfg.Writes())
		}
		if safari.open(t, second) || chrome.open(t, second) {
			t.Fatal("a browser that was cleared was cleared again after the restart")
		}
		if firefox := (&originBrowser{}); !firefox.open(t, second) {
			t.Fatal("after the restart a new browser's first entry cleared nothing")
		}
		second.shutdown()
		if after := f.bytes(t); after != settled {
			t.Fatalf("the restart changed the config:\n%s\nwas\n%s", after, settled)
		}
	})

	t.Run("a config that never had a site at the address clears nothing", func(t *testing.T) {
		seeds := map[string]func(f *addressFixture) string{
			"a first launch": nil,
			"a config with only later labels": func(f *addressFixture) string {
				return fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay1" }, "retiredSlugs": [ "htmlclay2" ], "nextSiteNumber": 4 }`, f.other)
			},
		}
		for name, seed := range seeds {
			f := newAddressFixture(t, "notes")
			if seed != nil {
				f.seed(t, seed(f))
			}
			a := f.launch(t)
			if a.loaded.MarkedFirstSiteOrigin || a.rt.cfg.Writes() != 0 {
				t.Fatalf("%s made a mark (%v) or wrote the config %d times", name, a.loaded.MarkedFirstSiteOrigin, a.rt.cfg.Writes())
			}
			f.open(t, a, f.doc)
			browser := &originBrowser{}
			if browser.open(t, a) || browser.open(t, a) {
				t.Fatalf("%s: an address that only ever was the dashboard's was cleared", name)
			}
			if len(browser.cookies) != 1 {
				t.Fatalf("%s: the dashboard set %v, want the session alone", name, browser.cookies)
			}
			if mark := f.cleanMark(t); mark != "" || a.rt.cfg.CleanMark() != "" {
				t.Fatalf("%s: a config that never had the label holds a mark: %q", name, mark)
			}
			a.shutdown()
		}
	})

	t.Run("a config whose first site was retired, or moved by an earlier build, gets its mark in one write", func(t *testing.T) {
		seeds := map[string]func(f *addressFixture) string{
			"retired": func(f *addressFixture) string {
				return fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay1" }, "retiredSlugs": [ "htmlclay" ], "nextSiteNumber": 3 }`, f.other)
			},
			"moved by an earlier build": func(f *addressFixture) string {
				return fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay3", %q: "htmlclay1" }, "retiredSlugs": [ "htmlclay2" ],
  "nextSiteNumber": 5, "movedFirstSite": %q }`, f.docs, f.other, f.docs)
			},
		}
		for name, seed := range seeds {
			f := newAddressFixture(t, "notes")
			f.seed(t, seed(f))
			first := f.launch(t)
			if first.loaded.MovedFirstSite || !first.loaded.MarkedFirstSiteOrigin || first.rt.cfg.Writes() != 1 {
				t.Fatalf("%s: moved %v, marked %v, %d config writes; want only a mark, in one write",
					name, first.loaded.MovedFirstSite, first.loaded.MarkedFirstSiteOrigin, first.rt.cfg.Writes())
			}
			mark := f.cleanMark(t)
			if !cleanMarkShape.MatchString(mark) {
				t.Fatalf("%s: the mark on disk = %q, want 64 hex", name, mark)
			}
			browser := &originBrowser{}
			if !browser.open(t, first) || browser.open(t, first) {
				t.Fatalf("%s: a browser was not cleared exactly once", name)
			}
			first.shutdown()

			second := f.launch(t)
			if second.loaded.MarkedFirstSiteOrigin || second.rt.cfg.Writes() != 0 || f.cleanMark(t) != mark {
				t.Fatalf("%s: the restart made a new mark or wrote the config (%d writes)", name, second.rt.cfg.Writes())
			}
			if browser.open(t, second) {
				t.Fatalf("%s: the browser was cleared again after the restart", name)
			}
			second.shutdown()
		}
	})

	t.Run("a page of the old first site, still open, gets nothing from the dashboard", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		f.seed(t, f.firstSite(f.docs))
		a := f.launch(t)
		person := &originBrowser{}
		person.open(t, a)

		// The person's own tab: the page with its token, cut off from any opener,
		// and the app's routes answer its script.
		page := servedRequest(t, a, http.MethodGet, dashboardAddress, "/", "", person.headers(nestedNav))
		match := regexp.MustCompile(`<meta name="workspace-token" content="([^"]+)">`).FindStringSubmatch(page.Body.String())
		if page.Code != http.StatusOK || match == nil {
			t.Fatalf("the person's tab = %d: %s", page.Code, page.Body.String())
		}
		token := match[1]
		if got := page.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
			t.Fatalf("the person's tab: Cross-Origin-Opener-Policy = %q", got)
		}
		fetch := map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"}
		script := person.headers(fetch)
		script["X-Workspace-Token"] = token
		if rec := servedRequest(t, a, http.MethodGet, dashboardAddress, "/api/app/snapshot", "", script); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"v":`) {
			t.Fatalf("the dashboard's own snapshot = %d %s", rec.Code, rec.Body.String())
		}

		// The old document's tab is on the same origin, so the browser sends its
		// requests with the same cookies. It has no token and cannot fetch one.
		leftover := person.headers(fetch)
		fetched := servedRequest(t, a, http.MethodGet, dashboardAddress, "/", "", leftover)
		if fetched.Code != http.StatusOK || strings.Contains(fetched.Body.String(), token) || strings.Contains(fetched.Body.String(), "workspace-token") {
			t.Fatalf("the old page's fetch of / = %d, with the token: %v", fetched.Code, strings.Contains(fetched.Body.String(), token))
		}
		for _, target := range []string{"/api/app/snapshot", "/api/app/events", "/api/bootstrap"} {
			if rec := servedRequest(t, a, http.MethodGet, dashboardAddress, target, "", leftover); rec.Code != http.StatusForbidden {
				t.Fatalf("the old page's fetch of %s = %d %s, want 403", target, rec.Code, rec.Body.String())
			}
		}
		before := f.bytes(t)
		form := person.headers(map[string]string{
			"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document",
			"Origin": "http://" + dashboardAddress, "Content-Type": "text/plain",
		})
		action := `{"id":"a1","type":"document.forget","args":{"documentID":"d0"}}`
		if rec := servedRequest(t, a, http.MethodPost, dashboardAddress, "/api/app/actions", action, form); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "snapshot") {
			t.Fatalf("the old page's form to the actions = %d %s, want 403", rec.Code, rec.Body.String())
		}
		if after := f.bytes(t); after != before {
			t.Fatal("the old page's form changed the config")
		}

		// A tab the old page opens on the dashboard goes out through the old
		// dashboard host, which sends it back, and only then is it served.
		opened := map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}
		returned := map[string]string{"Sec-Fetch-Site": "same-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}
		out := servedRequest(t, a, http.MethodGet, dashboardAddress, "/recent", "", person.headers(opened))
		oldHost := fmt.Sprintf("dashboard.htmlclay.localhost:%d", nestedTestPort)
		through, err := url.Parse(out.Header().Get("Location"))
		if err != nil || out.Code != http.StatusFound || through.Host != oldHost || through.Path != "/recent" || through.Query().Get("detour") == "" || strings.Contains(out.Body.String(), token) {
			t.Fatalf("a tab the old page opened = %d to %q, want a redirect through %s", out.Code, out.Header().Get("Location"), oldHost)
		}
		back := servedRequest(t, a, http.MethodGet, through.Host, through.RequestURI(), "", returned)
		if want := "http://" + dashboardAddress + through.RequestURI(); back.Code != http.StatusFound || back.Header().Get("Location") != want {
			t.Fatalf("the old host = %d to %q, want 302 to %q", back.Code, back.Header().Get("Location"), want)
		}
		dropped := servedRequest(t, a, http.MethodGet, dashboardAddress, through.RequestURI(), "", person.headers(returned))
		if dropped.Code != http.StatusFound || dropped.Header().Get("Location") != "/recent" {
			t.Fatalf("the way back = %d to %q, want 302 to /recent", dropped.Code, dropped.Header().Get("Location"))
		}
		arrived := servedRequest(t, a, http.MethodGet, dashboardAddress, "/recent", "", person.headers(returned))
		if arrived.Code != http.StatusOK || !strings.Contains(arrived.Body.String(), token) || arrived.Header().Get("Cross-Origin-Opener-Policy") != "same-origin" {
			t.Fatalf("the tab back at /recent = %d", arrived.Code)
		}

		// Its own address still sends a person on to where the document lives now.
		rel := f.rel(f.doc)
		nestedRedirect(t, a, dashboardAddress, rel, fileURL(nestedHost("htmlclay3"), rel))
	})
}
