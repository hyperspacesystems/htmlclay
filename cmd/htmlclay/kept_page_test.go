package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/versions"
)

type keptClient struct {
	t      *testing.T
	a      *app
	origin string
	cookie *http.Cookie
}

func newKeptApp(t *testing.T) *app {
	t.Helper()
	a := buildTestApp(t, t.TempDir(), t.TempDir())
	a.rt.port = 24192
	return a
}

func (c *keptClient) do(method, target string, body url.Values, mutate func(*http.Request)) *httptest.ResponseRecorder {
	c.t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, c.origin+target, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", c.origin)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	} else {
		req = httptest.NewRequest(method, c.origin+target, nil)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	c.a.serveHost(rec, req)
	return rec
}

// openKeptForTest opens the page the way the tray will and keeps the session
// cookie.
func openKeptForTest(t *testing.T, a *app) *keptClient {
	t.Helper()
	var opened string
	a.rt.openKeptURL = func(u string) error { opened = u; return nil }
	a.openKeptPage()
	c := &keptClient{t: t, a: a, origin: "http://127.0.0.1:" + strconv.Itoa(a.rt.port)}
	if !strings.HasPrefix(opened, c.origin+"/kept/open?t=") {
		t.Fatalf("opened %q, want a one-time link on %s", opened, c.origin)
	}
	rec := c.do("GET", strings.TrimPrefix(opened, c.origin), nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "none") })
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/kept" {
		t.Fatalf("redeem = %d %q, want 303 to /kept", rec.Code, rec.Header().Get("Location"))
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == keptCookie {
			if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/kept" {
				t.Fatalf("cookie %+v, want HttpOnly, SameSite=Strict and path /kept", ck)
			}
			c.cookie = ck
		}
	}
	if c.cookie == nil {
		t.Fatal("no session cookie")
	}
	return c
}

func (c *keptClient) csrf() string {
	c.t.Helper()
	rec := c.do("GET", "/kept", nil, nil)
	if rec.Code != http.StatusOK {
		c.t.Fatalf("GET /kept = %d", rec.Code)
	}
	body := rec.Body.String()
	_, rest, ok := strings.Cut(body, `name="csrf" value="`)
	if !ok {
		c.t.Fatalf("no csrf field in page")
	}
	token, _, _ := strings.Cut(rest, `"`)
	return token
}

func (c *keptClient) action(values url.Values) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.do("POST", "/kept/action", values, nil)
}

// pinKeptCopy backs up one version of path and keeps it, returning the history
// folder and the kept version's name.
func pinKeptCopy(t *testing.T, a *app, path, body string) (string, string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	key := versions.Key(path, nil)
	if _, err := a.rt.versions.Backup(key, path, []byte(body)); err != nil {
		t.Fatalf("backup: %v", err)
	}
	entries, err := a.rt.versions.List(key, path)
	if err != nil || len(entries) == 0 {
		t.Fatalf("list after backup = %d entries, %v", len(entries), err)
	}
	name := entries[0].Name
	if err := a.rt.versions.Pin(key, path, name); err != nil {
		t.Fatalf("pin: %v", err)
	}
	kept, err := a.rt.versions.ListKept()
	if err != nil {
		t.Fatalf("ListKept: %v", err)
	}
	for _, h := range kept {
		if h.AbsPath != path {
			continue
		}
		for _, e := range h.Entries {
			if e.Name == name {
				return h.Folder, name
			}
		}
	}
	t.Fatalf("the pinned copy of %s was not listed", path)
	return "", ""
}

func TestKeptPageNeedsTheTrayLink(t *testing.T) {
	a := newKeptApp(t)
	c := &keptClient{t: t, a: a, origin: "http://127.0.0.1:24192"}
	if rec := c.do("GET", "/kept", nil, nil); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "csrf") {
		t.Fatalf("GET /kept without a session = %d, want a 403 sign-in page", rec.Code)
	}
	if !strings.Contains(c.do("GET", "/kept", nil, nil).Body.String(), "This link has expired") {
		t.Fatal("the sign-in page does not explain the expired link")
	}
	if rec := c.do("GET", "/kept/open?t=forged", nil, nil); rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a forged ticket = %d with cookies %v", rec.Code, rec.Result().Cookies())
	}
}

func TestKeptTicketOpensThePageOnceAndOnlyOnce(t *testing.T) {
	a := newKeptApp(t)
	var opened string
	a.rt.openKeptURL = func(u string) error { opened = u; return nil }
	a.openKeptPage()
	if !strings.HasPrefix(opened, "http://127.0.0.1:24192/kept/open?t=") {
		t.Fatalf("openKeptPage opened %q", opened)
	}
	c := &keptClient{t: t, a: a, origin: "http://127.0.0.1:24192"}
	target := strings.TrimPrefix(opened, c.origin)
	rec := c.do("GET", target, nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "none") })
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/kept" {
		t.Fatalf("first redeem = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	cookie := rec.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != keptCookie {
		t.Fatalf("first redeem set cookies %v, want %s", cookie, keptCookie)
	}
	c.cookie = cookie[0]
	if rec := c.do("GET", target, nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "none") }); rec.Code != http.StatusForbidden {
		t.Fatalf("second redeem = %d, want 403", rec.Code)
	}
	if rec := c.do("GET", "/kept", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /kept with the cookie = %d", rec.Code)
	}
}

func TestKeptPageIsOnlyOn127001(t *testing.T) {
	a := newKeptApp(t)
	for _, host := range []string{"localhost:24192", "search.localhost:24192"} {
		rec := serveHostForTest(t, a, host, func(r *http.Request) { r.URL.Path = "/kept" })
		if strings.Contains(rec.Body.String(), "<title>Kept Copies") {
			t.Fatalf("%s served the Kept Copies page", host)
		}
	}
}

func TestKeptPageHeadersAndNoScript(t *testing.T) {
	a := newKeptApp(t)
	c := openKeptForTest(t, a)
	rec := c.do("GET", "/kept", nil, nil)
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "frame-ancestors 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q lacks %q", csp, want)
		}
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<script") {
		t.Fatal("the Kept Copies page carries a script")
	}
	if !strings.Contains(body, "No kept copies.") {
		t.Fatal("an empty store did not show the empty state")
	}
}

func TestKeptAndProgramsCookiesDoNotCross(t *testing.T) {
	a := newKeptApp(t)
	programs := signIn(t, a)
	if rec := programs.do("GET", "/kept", nil, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a Programs cookie opened /kept: %d", rec.Code)
	}
	kept := openKeptForTest(t, a)
	if rec := kept.do("GET", "/programs", nil, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a Kept cookie opened /programs: %d", rec.Code)
	}
}

func TestKeptActionNeedsCSRFAndOrigin(t *testing.T) {
	a := newKeptApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.html")
	folder, name := pinKeptCopy(t, a, path, "<html>one</html>")
	c := openKeptForTest(t, a)
	form := url.Values{"action": {"delete"}, "folder": {folder}, "name": {name}}

	if rec := c.action(form); rec.Code != http.StatusForbidden {
		t.Fatalf("no csrf = %d, want 403", rec.Code)
	}
	form.Set("csrf", c.csrf())
	if rec := c.do("POST", "/kept/action", form, func(r *http.Request) { r.Header.Set("Origin", "http://search.localhost:24192") }); rec.Code != http.StatusForbidden {
		t.Fatalf("a document origin = %d, want 403", rec.Code)
	}
	if rec := c.do("POST", "/kept/action", form, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") }); rec.Code != http.StatusForbidden {
		t.Fatalf("same-site = %d, want 403", rec.Code)
	}
	if _, err := a.rt.versions.KeptPath(folder, name); err != nil {
		t.Fatalf("a refused action deleted the kept copy: %v", err)
	}

	rec := c.action(form)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/kept?done=deleted" {
		t.Fatalf("delete = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if _, err := a.rt.versions.KeptPath(folder, name); err == nil {
		t.Fatal("an allowed action did not delete the kept copy")
	}
}

func TestKeptPageListsConfirmsAndDeletes(t *testing.T) {
	local := time.Local
	time.Local = time.FixedZone("CET", 3600)
	t.Cleanup(func() { time.Local = local })

	a := newKeptApp(t)
	dirA, dirB := t.TempDir(), t.TempDir()
	pathA := filepath.Join(dirA, "alpha.html")
	pathB := filepath.Join(dirB, "beta.html")
	folderA, nameA1 := pinKeptCopy(t, a, pathA, "<html>alpha one</html>")
	_, nameA2 := pinKeptCopy(t, a, pathA, "<html>alpha two is longer</html>")
	folderB, nameB := pinKeptCopy(t, a, pathB, "<html>beta</html>")

	c := openKeptForTest(t, a)
	page := c.do("GET", "/kept", nil, nil).Body.String()
	for _, want := range []string{"alpha.html", "beta.html", "3 copies", "Delete All 2 Copies"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the list lacks %q", want)
		}
	}
	if !strings.Contains(page, `href="/kept/confirm?folder=`+folderA+`&all=1"`) {
		t.Fatalf("the list lacks the delete-all link for %s", folderA)
	}

	link := `/kept/confirm?` + url.Values{"folder": {folderA}, "name": {nameA1}}.Encode()
	if !strings.Contains(page, `href="`+link+`"`) && !strings.Contains(page, `href="`+strings.ReplaceAll(link, "&", "&amp;")+`"`) {
		t.Fatalf("the list lacks the delete link %s", link)
	}
	confirm := c.do("GET", link, nil, nil)
	if confirm.Code != http.StatusOK {
		t.Fatalf("GET /kept/confirm = %d", confirm.Code)
	}
	for _, want := range []string{"After you delete this copy", `value="delete"`, `value="` + folderA + `"`, `value="` + strings.ReplaceAll(nameA1, "+", "&#43;") + `"`} {
		if !strings.Contains(confirm.Body.String(), want) {
			t.Fatalf("the confirm page lacks %q", want)
		}
	}
	if n := len(c.do("GET", "/kept", nil, nil).Body.String()); n == 0 {
		t.Fatal("the list rendered nothing after the confirm page")
	}
	if _, err := a.rt.versions.KeptPath(folderA, nameA1); err != nil {
		t.Fatalf("the confirm page deleted a copy: %v", err)
	}

	if rec := c.do("GET", "/kept/confirm?folder="+folderA+"&name=not-a-version.html", nil, nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/kept?done=gone" {
		t.Fatalf("confirm of an unkept name = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	csrf := c.csrf()
	rec := c.action(url.Values{"csrf": {csrf}, "action": {"delete"}, "folder": {folderA}, "name": {nameA1}})
	if rec.Header().Get("Location") != "/kept?done=deleted" {
		t.Fatalf("delete = %q", rec.Header().Get("Location"))
	}
	kept, err := a.rt.versions.ListKept()
	if err != nil {
		t.Fatal(err)
	}
	var namesA []string
	for _, h := range kept {
		if h.Folder == folderA {
			for _, e := range h.Entries {
				namesA = append(namesA, e.Name)
			}
		}
	}
	if len(namesA) != 1 || namesA[0] != nameA2 {
		t.Fatalf("after one delete, %s keeps %v, want only %s", folderA, namesA, nameA2)
	}
	if _, err := a.rt.versions.KeptPath(folderB, nameB); err != nil {
		t.Fatalf("deleting one document's copy touched another: %v", err)
	}

	all := c.do("GET", "/kept/confirm?folder="+folderA+"&all=1", nil, nil)
	if all.Code != http.StatusOK || !strings.Contains(all.Body.String(), "After you delete these copies") {
		t.Fatalf("about-to-delete-all page = %d", all.Code)
	}
	if rec := c.action(url.Values{"csrf": {csrf}, "action": {"delete-all"}, "folder": {folderA}, "name": namesA}); rec.Header().Get("Location") != "/kept?done=deleted-all" {
		t.Fatalf("delete-all = %q", rec.Header().Get("Location"))
	}
	kept, err = a.rt.versions.ListKept()
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].Folder != folderB {
		t.Fatalf("after delete-all of %s, kept = %+v, want only %s", folderA, kept, folderB)
	}
}

func TestKeptDeleteAllKeepsACopyKeptAfterTheConfirmPage(t *testing.T) {
	a := newKeptApp(t)
	path := filepath.Join(t.TempDir(), "delta.html")
	folder, name1 := pinKeptCopy(t, a, path, "<html>delta one</html>")
	_, name2 := pinKeptCopy(t, a, path, "<html>delta two is longer</html>")

	c := openKeptForTest(t, a)
	all := c.do("GET", "/kept/confirm?folder="+folder+"&all=1", nil, nil)
	if all.Code != http.StatusOK {
		t.Fatalf("GET /kept/confirm?all=1 = %d", all.Code)
	}
	for _, name := range []string{name1, name2} {
		want := `<input type="hidden" name="name" value="` + strings.ReplaceAll(name, "+", "&#43;") + `">`
		if !strings.Contains(all.Body.String(), want) {
			t.Fatalf("the delete-all form lacks the hidden name for %s", name)
		}
	}

	_, name3 := pinKeptCopy(t, a, path, "<html>delta three</html>")

	csrf := c.csrf()
	rec := c.action(url.Values{"csrf": {csrf}, "action": {"delete-all"}, "folder": {folder}, "name": {name1, name2}})
	if rec.Header().Get("Location") != "/kept?done=deleted-all" {
		t.Fatalf("delete-all = %q", rec.Header().Get("Location"))
	}
	if _, err := a.rt.versions.KeptPath(folder, name3); err != nil {
		t.Fatalf("a copy kept after the confirm page was deleted too: %v", err)
	}
	for _, name := range []string{name1, name2} {
		if _, err := a.rt.versions.KeptPath(folder, name); err == nil {
			t.Fatalf("the copy %s on the confirm page was not deleted", name)
		}
	}
}

func TestKeptPageUsesSingularWordingForOneCopy(t *testing.T) {
	a := newKeptApp(t)
	path := filepath.Join(t.TempDir(), "epsilon.html")
	folder, _ := pinKeptCopy(t, a, path, "<html>epsilon</html>")

	c := openKeptForTest(t, a)
	page := c.do("GET", "/kept", nil, nil).Body.String()
	if !strings.Contains(page, "1 copy") || strings.Contains(page, "1 copies") {
		t.Fatalf("the list does not use singular wording for one copy")
	}
	confirm := c.do("GET", "/kept/confirm?folder="+folder+"&all=1", nil, nil).Body.String()
	if !strings.Contains(confirm, "1 copy") || strings.Contains(confirm, "1 copies") {
		t.Fatalf("the confirm page does not use singular wording for one copy")
	}
	if !strings.Contains(confirm, "Delete This Copy") {
		t.Fatalf("the confirm page does not offer Delete This Copy for one copy")
	}
}

func TestKeptActionExpiredSessionShowsTheExpiredPage(t *testing.T) {
	a := newKeptApp(t)
	c := &keptClient{t: t, a: a, origin: "http://127.0.0.1:24192"}
	rec := c.action(url.Values{"action": {"delete"}, "folder": {"somefolder"}, "name": {"2024-01-01-00-00-00-000+0000.html"}})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This link has expired") {
		t.Fatalf("a POST from an expired session = %d, want the 403 expired-link page", rec.Code)
	}
}

func TestKeptRevealCallsThePlatformOpener(t *testing.T) {
	a := newKeptApp(t)
	path := filepath.Join(t.TempDir(), "gamma.html")
	folder, name := pinKeptCopy(t, a, path, "<html>gamma</html>")
	want, err := a.rt.versions.KeptPath(folder, name)
	if err != nil {
		t.Fatal(err)
	}
	var revealed []string
	a.rt.revealKept = func(p string) error { revealed = append(revealed, p); return nil }
	c := openKeptForTest(t, a)
	csrf := c.csrf()
	if rec := c.action(url.Values{"csrf": {csrf}, "action": {"reveal"}, "folder": {folder}, "name": {name}}); rec.Header().Get("Location") != "/kept?done=revealed" {
		t.Fatalf("reveal = %q", rec.Header().Get("Location"))
	}
	if len(revealed) != 1 || revealed[0] != want {
		t.Fatalf("revealed %v, want [%s]", revealed, want)
	}
}

func TestKeptPageEscapesADocumentName(t *testing.T) {
	a := newKeptApp(t)
	// A name with a slash in it can never be a file on disk, so this history
	// stands for a document bound to a path this machine does not hold.
	path := filepath.Join(t.TempDir(), "<b>x</b>.html")
	body := []byte("<html>bold</html>")
	key := versions.Key(path, nil)
	if _, err := a.rt.versions.Backup(key, path, body); err != nil {
		t.Fatalf("backup: %v", err)
	}
	entries, err := a.rt.versions.List(key, path)
	if err != nil || len(entries) == 0 {
		t.Fatalf("list after backup = %d entries, %v", len(entries), err)
	}
	if err := a.rt.versions.Pin(key, path, entries[0].Name); err != nil {
		t.Fatalf("pin: %v", err)
	}
	c := openKeptForTest(t, a)
	page := c.do("GET", "/kept", nil, nil).Body.String()
	if !strings.Contains(page, "&lt;b&gt;x&lt;/b&gt;.html") {
		t.Fatal("the document name is not escaped")
	}
	if strings.Contains(page, "<b>x</b>") {
		t.Fatal("the page carried a raw document name")
	}
}
