package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/service"
)

// folderListView is one entry of a folder list answer.
type folderListView struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Open     string `json:"open"`
}

// dashboardFolderListView is one folder list answer.
type dashboardFolderListView struct {
	Items        []folderListView `json:"items"`
	Capabilities map[string]bool  `json:"capabilities"`
}

// folderList asks one folder's entries and decodes them.
func folderList(t *testing.T, c *dashboardNoService, id, parent string) dashboardFolderListView {
	t.Helper()
	target := "/api/app/folders/" + url.PathEscape(id) + "/list"
	if parent != "" {
		target += "?parent=" + url.QueryEscape(parent)
	}
	status, body := c.api(http.MethodGet, target, nil)
	var list dashboardFolderListView
	if status != http.StatusOK || json.Unmarshal([]byte(body), &list) != nil {
		t.Fatalf("the list of %q = %d %s", parent, status, body)
	}
	return list
}

// folderItem answers one entry of a list by name.
func folderItem(t *testing.T, list dashboardFolderListView, name string) folderListView {
	t.Helper()
	for _, item := range list.Items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("the list has no entry %q: %+v", name, list.Items)
	return folderListView{}
}

// listed reports whether a list names one entry.
func listed(list dashboardFolderListView, name string) bool {
	for _, item := range list.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}

// folderOpen asks the open route without following its answer, extracting the
// destination URL from the strict-origin HTML trampoline when admitted.
func folderOpen(t *testing.T, c *dashboardNoService, id, rel string) (int, string, string) {
	t.Helper()
	target := c.server.URL + "/api/app/folders/" + url.PathEscape(id) + "/open?path=" + url.QueryEscape(rel)
	r, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = c.host
	r.Header.Set("Cookie", c.cookie)
	r.Header.Set("Origin", "http://"+c.host)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	location := res.Header.Get("Location")
	if res.StatusCode == http.StatusOK {
		if got := res.Header.Get("Referrer-Policy"); got != "strict-origin" {
			t.Fatalf("open trampoline Referrer-Policy = %q, want strict-origin", got)
		}
		const prefix = "window.location.replace("
		if i := strings.Index(body, prefix); i >= 0 {
			rest := body[i+len(prefix):]
			if j := strings.Index(rest, ");</script>"); j >= 0 {
				var parsed string
				if err := json.Unmarshal([]byte(rest[:j]), &parsed); err == nil {
					location = parsed
				}
			}
		}
	}
	return res.StatusCode, location, body
}

// serviceRoute calls a route that belongs to the live-room service, as the entry
// page's script does.
func serviceRoute(t *testing.T, c *dashboardNoService, method, target string, body map[string]any) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(encoded))
	}
	r, err := http.NewRequest(method, c.server.URL+target, reader)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = c.host
	r.Header.Set("Cookie", c.cookie)
	r.Header.Set("X-Workspace-Token", c.token)
	r.Header.Set("Origin", "http://"+c.host)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	text, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(text)
}

// trustedDashboard trusts dir in an app whose sites are up, with no live-room
// service, and answers the dashboard over it.
func trustedDashboard(t *testing.T, home, dir string) (*app, *dashboardNoService, string) {
	t.Helper()
	a := newTestAppWithConfigDir(t, home, t.TempDir())
	if err := a.trustFolder(dir); err != nil {
		t.Fatalf("trust %s: %v", dir, err)
	}
	a.startSites()
	c := openDashboardOver(t, a, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the dashboard sent %s %s to the service, and no service is running", r.Method, r.URL.Path)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	id := ""
	for _, entry := range a.rt.cfg.TrustedFolderList() {
		if entry.Path == dir {
			id = entry.ID
		}
	}
	if id == "" {
		t.Fatalf("the config has no trusted entry for %s", dir)
	}
	return a, c, id
}

// TestM3DashboardFolders is step 7.23: the dashboard browses a trusted folder,
// and a served folder while the live-room service is stopped, through a
// read-only list the Go host answers itself, and a link never leads out of the
// folder.
func TestM3DashboardFolders(t *testing.T) {
	t.Run("an unknown id is refused", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		if status, body := c.api(http.MethodGet, "/api/app/folders/0123456789abcdef/list", nil); status != http.StatusNotFound {
			t.Fatalf("an unknown folder = %d %s", status, body)
		}
		if status, body := c.api(http.MethodGet, "/api/app/folders/not-an-id/open?path=index.html", nil); status != http.StatusNotFound {
			t.Fatalf("an unknown folder's open = %d %s", status, body)
		}
	})

	t.Run("a link inside the folder that points outside it is neither listed nor opened", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		writeTestFile(t, filepath.Join(dir, "sub", "b.html"), servedTestDocument)
		outside := filepath.Join(home, "elsewhere")
		writeTestFile(t, filepath.Join(outside, "secret.html"), servedTestDocument)
		if err := os.Symlink(filepath.Join(outside, "secret.html"), filepath.Join(dir, "escape.html")); err != nil {
			t.Fatalf("link: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "outside")); err != nil {
			t.Fatalf("link: %v", err)
		}

		a, c, id := trustedDashboard(t, home, dir)
		list := folderList(t, c, id, "")
		if list.Capabilities["write"] {
			t.Fatalf("the list offers a write: %+v", list.Capabilities)
		}
		document := folderItem(t, list, "index.html")
		if document.Kind != "document" || document.Size == 0 || document.Modified == "" {
			t.Fatalf("index.html = %+v", document)
		}
		wantOpen := "/api/app/folders/" + url.PathEscape(id) + "/open?path=index.html"
		if document.Open != wantOpen {
			t.Fatalf("index.html opens at %q, want %q", document.Open, wantOpen)
		}
		if folder := folderItem(t, list, "sub"); folder.Kind != "folder" {
			t.Fatalf("sub = %+v", folder)
		}
		for _, name := range []string{"escape.html", "outside"} {
			if listed(list, name) {
				t.Fatalf("the link %s is listed: %+v", name, list.Items)
			}
		}

		for _, rel := range []string{"escape.html", path.Join("outside", "secret.html"), "../elsewhere/secret.html"} {
			status, location, body := folderOpen(t, c, id, rel)
			if status != http.StatusNotFound || location != "" {
				t.Fatalf("opening %q = %d to %q: %s", rel, status, location, body)
			}
		}
		for _, doc := range a.snapshot().Documents {
			if strings.Contains(doc.Name, "secret") {
				t.Fatalf("the open registered a document outside the folder: %+v", doc)
			}
		}

		status, location, body := folderOpen(t, c, id, "index.html")
		if status != http.StatusOK || !strings.HasPrefix(location, "http://") || !strings.HasSuffix(location, "/index.html") {
			t.Fatalf("opening index.html = %d to %q: %s", status, location, body)
		}
		registered := false
		for _, doc := range a.snapshot().Documents {
			if doc.URL == location {
				registered = true
			}
		}
		if !registered {
			t.Fatalf("opening index.html registered no document at %q: %+v", location, a.snapshot().Documents)
		}
	})

	t.Run("an unserved trusted file opens from the dashboard with a strict-origin referrer and survives reload", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		_, c, id := trustedDashboard(t, home, dir)

		status, location, body := folderOpen(t, c, id, "index.html")
		if status != http.StatusOK || location == "" || !strings.Contains(body, `name="referrer" content="strict-origin"`) {
			t.Fatalf("opening index.html = %d to %q: %s", status, location, body)
		}

		// The trampoline page replaces itself under Referrer-Policy: strict-origin,
		// so the browser sends the dashboard origin as Referer on initial load and
		// on Cmd+R reload, while cross-site fetches and referrer-less navigations fail.
		originReferer := "http://" + c.host + "/"
		request := func(mode, dest, referer string) (int, string) {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, location, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Sec-Fetch-Mode", mode)
			req.Header.Set("Sec-Fetch-Dest", dest)
			if referer != "" {
				req.Header.Set("Referer", referer)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			out, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			return res.StatusCode, string(out)
		}

		if status, body := request("cors", "empty", originReferer); status != http.StatusForbidden {
			t.Fatalf("a cross-site fetch of the opened file = %d %s, want 403", status, body)
		}
		if status, body := request("navigate", "document", ""); status != http.StatusForbidden {
			t.Fatalf("a cross-site navigation with no referer = %d %s, want 403", status, body)
		}
		if status, body := request("navigate", "document", originReferer); status != http.StatusOK {
			t.Fatalf("the trampoline's initial navigation = %d %s, want 200", status, body)
		}
		if status, body := request("navigate", "document", originReferer); status != http.StatusOK {
			t.Fatalf("reloading (Cmd+R) the opened tab = %d %s, want 200", status, body)
		}
	})

	t.Run("a served folder lists with the service stopped", func(t *testing.T) {
		if service.Bundled() {
			t.Skip("a bundled build always runs its own service")
		}
		t.Setenv("HTMLCLAY_SERVICE_CMD", "")
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		writeTestFile(t, filepath.Join(dir, "sub", "b.html"), servedTestDocument)
		writeTestFile(t, filepath.Join(dir, "notes.txt"), "plain text")
		writeTestFile(t, filepath.Join(dir, ".hyperclay", "internal.html"), servedTestDocument)

		a := newServiceTestApp(t, home, t.TempDir())
		serveThroughDo(t, a, "serve-1", dir, "notes")
		if state := a.service.state(); state == appcore.ServiceRunning {
			t.Fatalf("the live-room service is %q, and this subtest needs it stopped", state)
		}
		served := a.rt.cfg.ServedFolderList()[0]
		address := a.servedAddress(served)
		if entry := a.workspaceEntry(served.ID); !strings.HasPrefix(entry, a.dashboardOrigin()+"/enter?ticket=") {
			t.Fatalf("the workspace entry with the service stopped = %q", entry)
		}
		c := openDashboardOver(t, a, a.served.DashboardAPI())

		list := folderList(t, c, served.ID, "")
		if list.Capabilities["write"] {
			t.Fatalf("the list offers a write: %+v", list.Capabilities)
		}
		document := folderItem(t, list, "index.html")
		if document.Kind != "document" || document.Open != address+"/index.html" {
			t.Fatalf("index.html = %+v, want it opened at %s/index.html", document, address)
		}
		if folder := folderItem(t, list, "sub"); folder.Kind != "folder" || folder.Size != 0 {
			t.Fatalf("sub = %+v", folder)
		}
		if file := folderItem(t, list, "notes.txt"); file.Kind != "file" || file.Size == 0 || file.Open != "" {
			t.Fatalf("notes.txt = %+v", file)
		}
		for _, item := range list.Items {
			if strings.HasPrefix(item.Name, ".") {
				t.Fatalf("the list shows an internal entry: %+v", item)
			}
		}
		if sub := folderList(t, c, served.ID, "sub"); len(sub.Items) != 1 || sub.Items[0].Path != "sub/b.html" {
			t.Fatalf("the list of sub = %+v", sub.Items)
		}

		status, location, body := folderOpen(t, c, served.ID, "index.html")
		if status != http.StatusSeeOther || location != address+"/index.html" {
			t.Fatalf("opening a served document = %d to %q: %s", status, location, body)
		}

		status, body = serviceRoute(t, c, http.MethodPost, "/api/roots/"+url.PathEscape(served.ID)+"/commands", map[string]any{})
		if status != http.StatusServiceUnavailable || !strings.Contains(body, "service-unavailable") {
			t.Fatalf("a write route for the folder = %d %s, want the service's own unavailable error", status, body)
		}
	})
}
