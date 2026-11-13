package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

// guardService runs the service with Local's write guard on the served folder
// and answers where its report lands once the service exits.
func guardService(t *testing.T, dir string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "guard.json")
	guard := filepath.Join(root, "service", "vendor", "local", "trial", "fs-guard.cjs")
	t.Setenv("HTMLCLAY_SERVICE_CMD", "node -r "+guard+" "+filepath.Join(root, "service", "main.cjs"))
	roots, _ := json.Marshal([]string{dir})
	t.Setenv("FS_GUARD_ROOTS", string(roots))
	t.Setenv("FS_GUARD_OUT", report)
	return report
}

// guardWrites stops the service and answers every write the guard saw it make
// in the served folder, allowed or not.
func guardWrites(t *testing.T, a *app, report string) []string {
	t.Helper()
	a.service.close()
	var got struct {
		Forbidden []struct{ Op, Path string } `json:"forbidden"`
		Allowed   []struct{ Op, Path string } `json:"allowed"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(report)
		if err == nil && json.Unmarshal(b, &got) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the guard wrote no report: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var writes []string
	for _, w := range append(got.Forbidden, got.Allowed...) {
		writes = append(writes, w.Op+" "+w.Path)
	}
	return writes
}

// TestDashboardCommandsCreateFolder makes a document from a template and a
// folder through the dashboard: Go writes both, the service writes nothing in
// the folder, and a name already taken is refused with nothing changed.
func TestDashboardCommandsCreateFolder(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	report := guardService(t, dir)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)
	id := c.rootIDOf()

	if status, body := c.command(id, map[string]any{"command": "folder", "parent": nil, "name": "Made"}); status != http.StatusOK || body != `{"path":"Made"}` {
		t.Fatalf("folder = %d %s", status, body)
	}
	if info, err := os.Stat(filepath.Join(dir, "Made")); err != nil || !info.IsDir() {
		t.Fatalf("the folder is not on disk: %v", err)
	}
	if status, body := c.command(id, map[string]any{"command": "create", "template": "blank", "parent": "Made", "name": "page.html"}); status != http.StatusOK || body != `{"path":"Made/page.html"}` {
		t.Fatalf("create = %d %s", status, body)
	}
	made, err := os.ReadFile(filepath.Join(dir, "Made", "page.html"))
	if err != nil || !strings.Contains(strings.ToLower(string(made)), "<html") {
		t.Fatalf("the created document = %q, %v", made, err)
	}
	status, body := c.api(http.MethodGet, "/api/roots/"+url.PathEscape(id)+"/tree", nil)
	if status != http.StatusOK || !strings.Contains(body, `"path":"Made/page.html"`) {
		t.Fatalf("tree after the commands = %d %s", status, body)
	}

	before, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if status, body := c.command(id, map[string]any{"command": "create", "template": "blank", "parent": nil, "name": "index.html"}); status != http.StatusConflict {
		t.Fatalf("create over an existing document = %d %s", status, body)
	}
	if status, body := c.command(id, map[string]any{"command": "folder", "parent": nil, "name": "Made"}); status != http.StatusConflict {
		t.Fatalf("a folder that exists = %d %s", status, body)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "index.html")); !bytes.Equal(before, after) {
		t.Fatal("a refused create changed the document")
	}
	if status, _ := c.command(id, map[string]any{"command": "trash", "path": "index.html"}); status != http.StatusNotImplemented {
		t.Fatalf("a command group not switched on = %d", status)
	}

	if writes := guardWrites(t, a, report); len(writes) != 0 {
		t.Fatalf("the service wrote in the served folder: %v", writes)
	}
}
