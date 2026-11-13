package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/dashboard"
)

// dashboardClient is a browser on the dashboard: it spent a ticket, holds the
// session cookie and carries the entry page's token on API calls.
type dashboardClient struct {
	t      *testing.T
	host   string
	server *httptest.Server
	token  string
}

func (c *dashboardClient) do(method, target string, body any, headers map[string]string) (int, string) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	r, err := http.NewRequest(method, c.server.URL+target, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	r.Host = c.host
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(r)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	text, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res.StatusCode, string(text)
}

// api calls the workspace API as the entry page's script does.
func (c *dashboardClient) api(method, target string, body any) (int, string) {
	c.t.Helper()
	headers := map[string]string{"X-Workspace-Token": c.token, "Sec-Fetch-Site": "same-origin", "Origin": "http://" + c.host}
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	return c.do(method, target, body, headers)
}

// command posts one file command for the only served folder.
func (c *dashboardClient) command(rootID string, body map[string]any) (int, string) {
	c.t.Helper()
	return c.api(http.MethodPost, "/api/roots/"+url.PathEscape(rootID)+"/commands", body)
}

// openDashboard enters the dashboard of an app whose service is running.
func openDashboard(t *testing.T, a *app) *dashboardClient {
	t.Helper()
	host := fmt.Sprintf("%s:%d", appcore.DashboardHost, servedTestPort)
	d, err := dashboard.New(dashboard.Options{Host: host, API: a.served.DashboardAPI()})
	if err != nil {
		t.Fatal(err)
	}
	c := &dashboardClient{t: t, host: host, server: httptest.NewServer(d)}
	t.Cleanup(c.server.Close)
	entry, err := d.EntryURL("/")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(entry)
	r, _ := http.NewRequest(http.MethodGet, c.server.URL+u.RequestURI(), nil)
	r.Host = host
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cookies := res.Cookies()
	if res.StatusCode != http.StatusFound || len(cookies) != 1 {
		t.Fatalf("enter = %d with %d cookies", res.StatusCode, len(cookies))
	}
	_, page := c.do(http.MethodGet, "/", nil, map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value})
	match := regexp.MustCompile(`<meta name="workspace-token" content="([^"]+)">`).FindStringSubmatch(page)
	if match == nil {
		t.Fatalf("the entry page has no token: %s", page)
	}
	c.token = match[1]
	return c
}

// rootIDOf answers the one served folder's id from the bootstrap.
func (c *dashboardClient) rootIDOf() string {
	c.t.Helper()
	status, body := c.api(http.MethodGet, "/api/bootstrap", nil)
	var boot struct {
		Roots []struct{ ID string } `json:"roots"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &boot) != nil || len(boot.Roots) != 1 {
		c.t.Fatalf("bootstrap = %d %s", status, body)
	}
	return boot.Roots[0].ID
}

// TestDashboardForward walks the dashboard as a browser would, against the real
// service: a ticket opens the entry page, its token reaches the API, and the
// service's workspace routes answer from the folder Go serves.
func TestDashboardForward(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "sub", "b.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)

	status, body := c.api(http.MethodGet, "/api/bootstrap", nil)
	var boot struct {
		ShellOrigin string `json:"shellOrigin"`
		Roots       []struct {
			ID, Kind, Origin string
		} `json:"roots"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &boot) != nil {
		t.Fatalf("bootstrap = %d %s", status, body)
	}
	if boot.ShellOrigin != "http://"+c.host || len(boot.Roots) != 1 || boot.Roots[0].Kind != "local" ||
		boot.Roots[0].Origin != "http://"+servedHostName("notes", servedTestPort) {
		t.Fatalf("bootstrap = %s", body)
	}

	status, body = c.api(http.MethodGet, "/api/roots/"+url.PathEscape(boot.Roots[0].ID)+"/tree", nil)
	if status != http.StatusOK || !strings.Contains(body, `"path":"sub/b.html"`) || !strings.Contains(body, `"path":"index.html"`) {
		t.Fatalf("tree = %d %s", status, body)
	}

	if status, _ := c.do(http.MethodGet, "/api/bootstrap", nil, map[string]string{"Sec-Fetch-Site": "same-origin"}); status != http.StatusForbidden {
		t.Fatalf("the API without the token = %d", status)
	}
}

// TestDashboardFramedDocument opens a document in the workspace's frame: the
// address the dashboard hands out loads through the folder's own address, the
// service bridges it, and only the dashboard may frame it. An id the workspace
// never handed out loads the plain document.
func TestDashboardFramedDocument(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)

	status, body := c.api(http.MethodGet, "/api/roots/"+url.PathEscape(c.rootIDOf())+"/document?path=index.html", nil)
	var doc struct {
		FrameURL string `json:"frameUrl"`
		WireID   int    `json:"wireId"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &doc) != nil || doc.FrameURL == "" {
		t.Fatalf("document = %d %s", status, body)
	}
	frame, err := url.Parse(doc.FrameURL)
	if err != nil || frame.Host != servedHostName("notes", servedTestPort) {
		t.Fatalf("frameUrl = %q", doc.FrameURL)
	}

	app := httptest.NewServer(http.HandlerFunc(a.serveHost))
	defer app.Close()
	load := func(target string) (*http.Response, string) {
		t.Helper()
		r, err := http.NewRequest(http.MethodGet, app.URL+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Host = frame.Host
		r.Header.Set("Sec-Fetch-Dest", "iframe")
		r.Header.Set("Sec-Fetch-Site", "same-site")
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	res, page := load(frame.RequestURI())
	if res.StatusCode != http.StatusOK || !strings.Contains(page, "data-hyperclay-bridge") {
		t.Fatalf("the framed load = %d, bridged %v", res.StatusCode, strings.Contains(page, "data-hyperclay-bridge"))
	}
	if csp := res.Header.Get("Content-Security-Policy"); csp != "frame-ancestors "+a.dashboardOrigin() {
		t.Fatalf("the framed load's frame header = %q", csp)
	}
	res, page = load(fmt.Sprintf("/index.html?workspace-frame=%d", doc.WireID+100))
	if res.StatusCode != http.StatusOK || strings.Contains(page, "data-hyperclay-bridge") {
		t.Fatalf("a load with an id never handed out = %d, bridged %v", res.StatusCode, strings.Contains(page, "data-hyperclay-bridge"))
	}
}

// TestDashboardOpen is Open Workspace: the tray row and dashboard.open open the
// dashboard's entry, a top-level load of the folder's "/" goes there too, and
// any other request for "/" keeps the listing.
func TestDashboardOpen(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServiceTestApp(t, home, t.TempDir())
	captured := newServedTrayCapture(a)
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	f := a.rt.cfg.ServedFolderList()[0]
	entryPrefix := a.dashboardOrigin() + "/enter?ticket="

	a.openServedFolder(f.ID)
	if len(captured.launched) != 1 || !strings.HasPrefix(captured.launched[0], entryPrefix) {
		t.Fatalf("Open Workspace launched %v", captured.launched)
	}
	if refusal := a.trayDo("dashboard.open", map[string]any{"route": "/"}); refusal != nil {
		t.Fatalf("dashboard.open refused: %v", refusal.Code)
	}
	if len(captured.launched) != 2 || !strings.HasPrefix(captured.launched[1], entryPrefix) {
		t.Fatalf("dashboard.open launched %v", captured.launched)
	}

	app := httptest.NewServer(http.HandlerFunc(a.serveHost))
	defer app.Close()
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	root := func(host, dest string) *http.Response {
		t.Helper()
		r, err := http.NewRequest(http.MethodGet, app.URL+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Host = host
		if dest != "" {
			r.Header.Set("Sec-Fetch-Dest", dest)
			r.Header.Set("Sec-Fetch-Mode", "navigate")
			r.Header.Set("Sec-Fetch-Site", "none")
		}
		res, err := noRedirect.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	folderHost := servedHostName("notes", servedTestPort)
	res := root(folderHost, "document")
	if res.StatusCode != http.StatusFound || !strings.HasPrefix(res.Header.Get("Location"), entryPrefix) {
		t.Fatalf("a page load of / = %d to %q", res.StatusCode, res.Header.Get("Location"))
	}
	if res := root(folderHost, "image"); res.StatusCode != http.StatusOK {
		t.Fatalf("an image load of / = %d, want the listing", res.StatusCode)
	}
	entry, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest(http.MethodGet, app.URL+entry.RequestURI(), nil)
	r.Host = entry.Host
	r.Header.Set("Sec-Fetch-Site", "none")
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	got, err := noRedirect.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	got.Body.Close()
	if got.StatusCode != http.StatusFound || got.Header.Get("Set-Cookie") == "" || got.Header.Get("Location") != "/roots/"+url.PathEscape(f.ID)+"/files" {
		t.Fatalf("the folder's entry = %d to %q, cookie %q", got.StatusCode, got.Header.Get("Location"), got.Header.Get("Set-Cookie"))
	}
}
