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

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

type programsClient struct {
	t      *testing.T
	a      *app
	origin string
	cookie *http.Cookie
}

func newProgramsApp(t *testing.T) *app {
	t.Helper()
	a := buildTestApp(t, t.TempDir(), t.TempDir())
	a.rt.port = 24191
	return a
}

func (c *programsClient) do(method, target string, body url.Values, mutate func(*http.Request)) *httptest.ResponseRecorder {
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

// signIn opens the page the way the tray does and keeps the session cookie.
func signIn(t *testing.T, a *app) *programsClient {
	t.Helper()
	var opened string
	a.rt.openProgramsURL = func(u string) error { opened = u; return nil }
	a.openProgramsPage()
	c := &programsClient{t: t, a: a, origin: "http://127.0.0.1:" + strconv.Itoa(a.rt.port)}
	if !strings.HasPrefix(opened, c.origin+"/programs/open?t=") {
		t.Fatalf("opened %q, want a one-time link on %s", opened, c.origin)
	}
	rec := c.do("GET", strings.TrimPrefix(opened, c.origin), nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "none") })
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/programs" {
		t.Fatalf("redeem = %d %q, want 303 to /programs", rec.Code, rec.Header().Get("Location"))
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == programsCookie {
			if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie %+v, want HttpOnly and SameSite=Strict", ck)
			}
			c.cookie = ck
		}
	}
	if c.cookie == nil {
		t.Fatal("no session cookie")
	}
	return c
}

func (c *programsClient) csrf() string {
	c.t.Helper()
	rec := c.do("GET", "/programs", nil, nil)
	if rec.Code != http.StatusOK {
		c.t.Fatalf("GET /programs = %d", rec.Code)
	}
	body := rec.Body.String()
	_, rest, ok := strings.Cut(body, `name="csrf" value="`)
	if !ok {
		c.t.Fatalf("no csrf field in page")
	}
	token, _, _ := strings.Cut(rest, `"`)
	return token
}

func (c *programsClient) action(values url.Values) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.do("POST", "/programs/action", values, nil)
}

func TestProgramsPageNeedsTheTrayLink(t *testing.T) {
	a := newProgramsApp(t)
	c := &programsClient{t: t, a: a, origin: "http://127.0.0.1:24191"}
	if rec := c.do("GET", "/programs", nil, nil); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "csrf") {
		t.Fatalf("GET /programs without a session = %d, want 403 sign-in page", rec.Code)
	}
	if rec := c.do("GET", "/programs/open?t=forged", nil, nil); rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a forged ticket = %d with cookies %v", rec.Code, rec.Result().Cookies())
	}
}

func TestProgramsTicketIsSingleUseAndExpires(t *testing.T) {
	a := newProgramsApp(t)
	c := &programsClient{t: t, a: a, origin: "http://127.0.0.1:24191"}
	now := time.Now()
	ticket, err := a.programs.mintTicket(now)
	if err != nil {
		t.Fatal(err)
	}
	if rec := c.do("GET", "/programs/open?t="+ticket, nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("first redeem = %d", rec.Code)
	}
	if rec := c.do("GET", "/programs/open?t="+ticket, nil, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("second redeem = %d, want 403", rec.Code)
	}
	old, err := a.programs.mintTicket(now.Add(-2 * programsTicketTTL))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.programs.redeem(old, now); ok {
		t.Fatal("an expired ticket was redeemed")
	}
}

func TestProgramsTicketRefusesACrossSiteNavigation(t *testing.T) {
	a := newProgramsApp(t)
	ticket, err := a.programs.mintTicket(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c := &programsClient{t: t, a: a, origin: "http://127.0.0.1:24191"}
	rec := c.do("GET", "/programs/open?t="+ticket, nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") })
	if rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a link followed from another page = %d, want 403 and no cookie", rec.Code)
	}
}

func TestProgramsPageIsOnlyOn127001(t *testing.T) {
	a := newProgramsApp(t)
	for _, host := range []string{"localhost:24191", "search.localhost:24191"} {
		rec := serveHostForTest(t, a, host, func(r *http.Request) { r.URL.Path = "/programs" })
		if strings.Contains(rec.Body.String(), "<title>Programs") {
			t.Fatalf("%s served the Programs page", host)
		}
	}
}

func TestProgramsPageHeadersAndNoScript(t *testing.T) {
	a := newProgramsApp(t)
	c := signIn(t, a)
	rec := c.do("GET", "/programs", nil, nil)
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "frame-ancestors 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q lacks %q", csp, want)
		}
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
	if strings.Contains(rec.Body.String(), "<script") {
		t.Fatal("the Programs page carries a script")
	}
}

func TestProgramsActionNeedsCSRFAndOrigin(t *testing.T) {
	a := newProgramsApp(t)
	home := a.rt.home
	document := writeHelperPage(t, home, "search")
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: document, Name: "search", Program: program.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, a)
	form := url.Values{"action": {"revoke-document"}, "program": {program.ID}, "document": {document}, "name": {"search"}}

	if rec := c.action(form); rec.Code != http.StatusForbidden {
		t.Fatalf("no csrf = %d, want 403", rec.Code)
	}
	form.Set("csrf", c.csrf())
	if rec := c.do("POST", "/programs/action", form, func(r *http.Request) { r.Header.Set("Origin", "http://search.localhost:24191") }); rec.Code != http.StatusForbidden {
		t.Fatalf("a document origin = %d, want 403", rec.Code)
	}
	if rec := c.do("POST", "/programs/action", form, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") }); rec.Code != http.StatusForbidden {
		t.Fatalf("same-site = %d, want 403", rec.Code)
	}
	if resolution, _ := a.rt.cfg.ResolveHelper(document, "search", false); !resolution.Allowed {
		t.Fatal("a refused action changed the permission")
	}

	rec := c.action(form)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/programs?done=revoked" {
		t.Fatalf("revoke = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	resolution, _ := a.rt.cfg.ResolveHelper(document, "search", true)
	if resolution.Allowed || !resolution.Revoked {
		t.Fatalf("after revoke resolution = %+v, want revoked", resolution)
	}
	if page := c.do("GET", "/programs?done=revoked", nil, nil).Body.String(); !strings.Contains(page, "Access removed") {
		t.Fatal("the page does not confirm the revoke")
	}
}

func TestProgramsDoneIgnoresUnknownText(t *testing.T) {
	a := newProgramsApp(t)
	c := signIn(t, a)
	page := c.do("GET", "/programs?done=%3Cb%3Eyou+were+hacked%3C%2Fb%3E", nil, nil).Body.String()
	if strings.Contains(page, "hacked") {
		t.Fatal("the page echoed text from the URL")
	}
}

func TestProgramsWidenOnlyThroughTheDialog(t *testing.T) {
	a := newProgramsApp(t)
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	answer := platform.ChoiceRefuse
	dialogs := helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			titles = append(titles, title)
			return answer, nil
		},
		choose:  func(string, string) (string, bool, error) { t.Fatal("picker opened"); return "", false, nil },
		prepare: func(string) (bool, error) { return true, nil },
	}
	if outcome, err := a.widenHelperProgram(program, dialogs); err != nil || outcome != "unchanged" {
		t.Fatalf("cancelled widen = %q, %v", outcome, err)
	}
	if p, _ := a.rt.cfg.LookupHelperProgram(program.ID); p.AnyDocument {
		t.Fatal("a cancelled dialog widened the program")
	}
	answer = platform.ChoiceAffirm
	if outcome, err := a.widenHelperProgram(program, dialogs); err != nil || outcome != "widened" {
		t.Fatalf("widen = %q, %v", outcome, err)
	}
	if p, _ := a.rt.cfg.LookupHelperProgram(program.ID); !p.AnyDocument {
		t.Fatal("an allowed widen did not stick")
	}
	if len(titles) != 2 {
		t.Fatalf("dialogs = %d, want 2", len(titles))
	}
}

func TestProgramsRelinkKeepsTheProgramID(t *testing.T) {
	settledSetup(t)
	a := newProgramsApp(t)
	dir := t.TempDir()
	path := writeTestProgram(t, dir, "tweet-score", 0755)
	program, err := a.rt.cfg.AddHelperProgram("tweet-score", path)
	if err != nil {
		t.Fatal(err)
	}
	document := writeHelperPage(t, a.rt.home, "tweet-score")
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: document, Name: "tweet-score", Program: program.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	moved := writeTestProgram(t, t.TempDir(), "tweet-score", 0755)
	dialogs := helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			if !strings.Contains(message, "no longer there") {
				t.Fatalf("relink message = %q", message)
			}
			return platform.ChoiceAffirm, nil
		},
		choose:  func(string, string) (string, bool, error) { return moved, true, nil },
		prepare: func(string) (bool, error) { return true, nil },
	}
	if outcome, err := a.relinkHelperProgram(program, dialogs); err != nil || outcome != "relinked" {
		t.Fatalf("relink = %q, %v", outcome, err)
	}
	resolution, _ := a.rt.cfg.ResolveHelper(document, "tweet-score", false)
	if !resolution.Allowed || resolution.Program.ID != program.ID || filepath.Clean(resolution.Program.Path) != filepath.Clean(moved) {
		t.Fatalf("after relink resolution = %+v, want %s at %s", resolution, program.ID, moved)
	}
}

func TestProgramsRelinkPickerStartsInAFolderThatExists(t *testing.T) {
	settledSetup(t)
	a := newProgramsApp(t)
	parent := t.TempDir()
	gone := filepath.Join(parent, "tweet-score")
	if err := os.Mkdir(gone, 0755); err != nil {
		t.Fatal(err)
	}
	program, err := a.rt.cfg.AddHelperProgram("tweet-score", writeTestProgram(t, gone, "tweet-score.mjs", 0755))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	var started string
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) { return platform.ChoiceAffirm, nil },
		choose: func(_, dir string) (string, bool, error) {
			started = dir
			return "", false, nil
		},
		prepare: func(string) (bool, error) { return true, nil },
	}
	if outcome, err := a.relinkHelperProgram(program, dialogs); err != nil || outcome != "unchanged" {
		t.Fatalf("relink = %q, %v", outcome, err)
	}
	if filepath.Clean(started) != filepath.Clean(parent) {
		t.Fatalf("picker started in %q, want the nearest existing folder %q", started, parent)
	}
}

func TestProgramsRemoveRefusesAProgramThatStillExists(t *testing.T) {
	a := newProgramsApp(t)
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, a)
	rec := c.action(url.Values{"csrf": {c.csrf()}, "action": {"remove"}, "program": {program.ID}})
	if rec.Header().Get("Location") != "/programs?done=failed" {
		t.Fatalf("remove of a present program = %q", rec.Header().Get("Location"))
	}
	if _, ok := a.rt.cfg.LookupHelperProgram(program.ID); !ok {
		t.Fatal("a present program was removed")
	}
}

func TestTrayHelperDocumentStatusAndClick(t *testing.T) {
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	a.openForTest(t, document)

	if got := a.trayHelperStatus(document, "search"); got != "needs setup…" {
		t.Fatalf("status before a decision = %q, want %q", got, "needs setup…")
	}
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: document, Name: "search", Program: program.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	if got, want := a.trayHelperStatus(document, "search"), "runs "+filepath.Base(program.Path); got != want {
		t.Fatalf("status with the file present = %q, want %q", got, want)
	}
	if err := os.Remove(program.Path); err != nil {
		t.Fatal(err)
	}
	if got := a.trayHelperStatus(document, "search"); got != "file missing" {
		t.Fatalf("status with the file gone = %q, want %q", got, "file missing")
	}
	present := writeTestProgram(t, t.TempDir(), "search", 0755)
	if _, ok := a.rt.cfg.SetHelperProgramPath(program.ID, present); !ok {
		t.Fatal("the program was not relinked")
	}

	var opened []string
	a.rt.openProgramsURL = func(u string) error { opened = append(opened, u); return nil }
	a.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
		t.Error("a click on an allowed program opened the setup dialog")
		return platform.ChoiceNone, nil
	}
	a.trayHelperDocument(document + "\x00search")
	if len(opened) != 1 {
		t.Fatalf("the click opened %d Programs pages, want 1: %v", len(opened), opened)
	}
}
