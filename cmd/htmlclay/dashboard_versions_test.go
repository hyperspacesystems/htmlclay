package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/specwire"
)

// recentVersionsPage is one page of a recent document's versions.
type recentVersionsPage struct {
	Items []historyItem `json:"items"`
	Next  *string       `json:"next"`
	Etag  *string       `json:"etag"`
}

// recentVersions asks one page of a recent document's kept versions.
func recentVersions(t *testing.T, c *dashboardNoService, id, before string) (int, string) {
	t.Helper()
	target := "/api/app/documents/" + url.PathEscape(id) + "/versions"
	if before != "" {
		target += "?before=" + url.QueryEscape(before)
	}
	return c.api(http.MethodGet, target, nil)
}

// mustRecentVersions is recentVersions for the page that must answer.
func mustRecentVersions(t *testing.T, c *dashboardNoService, id string) recentVersionsPage {
	t.Helper()
	status, body := recentVersions(t, c, id, "")
	var page recentVersionsPage
	if status != http.StatusOK || json.Unmarshal([]byte(body), &page) != nil {
		t.Fatalf("the versions of %s = %d %s", id, status, body)
	}
	return page
}

// versionBytes asks one kept version and answers its status, headers and body.
func versionBytes(t *testing.T, c *dashboardNoService, id, version string) (int, http.Header, string) {
	t.Helper()
	target := c.server.URL + "/api/app/documents/" + url.PathEscape(id) + "/versions/" + url.PathEscape(version)
	r, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = c.host
	r.Header.Set("Cookie", c.cookie)
	r.Header.Set("Origin", "http://"+c.host)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header, string(body)
}

// restoreVersion asks the dashboard to put one version back, naming the etag the
// person last saw. An empty ifMatch sends no If-Match at all.
func restoreVersion(t *testing.T, c *dashboardNoService, id, version, ifMatch string) (int, string) {
	t.Helper()
	headers := map[string]string{"Cookie": c.cookie, "Origin": "http://" + c.host, "Sec-Fetch-Site": "same-origin"}
	if ifMatch != "" {
		headers["If-Match"] = ifMatch
	}
	return c.request(http.MethodPost, "/api/app/documents/"+url.PathEscape(id)+"/versions/"+url.PathEscape(version)+"/restore", nil, headers)
}

// versionWith answers the id of the one kept version whose bytes hold marker.
func versionWith(t *testing.T, c *dashboardNoService, id, marker string) string {
	t.Helper()
	for _, item := range mustRecentVersions(t, c, id).Items {
		if status, _, body := versionBytes(t, c, id, item.ID); status == http.StatusOK && strings.Contains(body, marker) {
			return item.ID
		}
	}
	t.Fatalf("no kept version of %s holds %q", id, marker)
	return ""
}

// recentVersionsFixture is an app with one open loose document, two kept
// versions of it, and the dashboard over it, all with no live-room service.
type recentVersionsFixture struct {
	app      *app
	dash     *dashboardNoService
	path     string
	id       string
	contents string
}

// newRecentVersionsFixture opens one loose document through the Mac app, so it
// is registered and recorded, and keeps two versions of it.
func newRecentVersionsFixture(t *testing.T, contents string) *recentVersionsFixture {
	t.Helper()
	home := evalTempHome(t)
	path := filepath.Join(home, "notes", "page.html")
	writeTestFile(t, path, contents)

	a := buildTestApp(t, home, t.TempDir())
	a.rt.port = servedTestPort
	a.rt.launchBrowser = func(string) error { return nil }
	if _, refusal := a.do(context.Background(), coreAction(t, "open-versions", "file.open", map[string]string{"path": path}), ChannelTray); refusal != nil {
		t.Fatalf("file.open refused: %+v", refusal)
	}
	keepLooseVersion(t, a, path, "<!doctype html>\n<html><body><p>kept older</p></body></html>\n")
	keepLooseVersion(t, a, path, "<!doctype html>\n<html><body><p>kept newer</p></body></html>\n")

	client := openDashboardOver(t, a, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the dashboard sent %s %s to the service, and no service is running", r.Method, r.URL.Path)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	return &recentVersionsFixture{app: a, dash: client, path: path, id: recentRow(t, a, path).ID, contents: contents}
}

// TestM3RecentVersions is step 7.24: the dashboard lists, previews as plain text
// and restores a recent document's kept versions, a restore never overwrites a
// save made after the person looked, and a version is never served as a page
// that could run.
func TestM3RecentVersions(t *testing.T) {
	current := "<!doctype html>\n<html><body><p>current bytes</p></body></html>\n"

	t.Run("a recent document lists its versions with the etag of its bytes", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		page := mustRecentVersions(t, fx.dash, fx.id)
		if len(page.Items) != 2 {
			t.Fatalf("the versions of %s = %+v, want two", fx.id, page.Items)
		}
		if page.Next != nil {
			t.Fatalf("the only page offers a next page: %q", *page.Next)
		}
		if page.Etag == nil || *page.Etag != specwire.Etag([]byte(current)) {
			t.Fatalf("etag = %v, want the stamp of the bytes on disk %q", page.Etag, specwire.Etag([]byte(current)))
		}
		for _, item := range page.Items {
			if !item.Restorable || item.Kind != "saved" || item.CreatedAt == "" {
				t.Fatalf("row = %+v", item)
			}
		}
	})

	t.Run("a version is answered as plain text", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		id := versionWith(t, fx.dash, fx.id, "kept older")
		status, header, body := versionBytes(t, fx.dash, fx.id, id)
		if status != http.StatusOK || !strings.Contains(body, "kept older") {
			t.Fatalf("the version = %d %q", status, body)
		}
		if got := header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
			t.Fatalf("the version's Content-Type = %q", got)
		}
	})

	t.Run("a version that contains a script tag is answered as text with the sandbox header", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		keepLooseVersion(t, fx.app, fx.path, "<!doctype html>\n<html><body><script>alert(1)</script></body></html>\n")
		id := versionWith(t, fx.dash, fx.id, "<script>alert(1)</script>")
		status, header, body := versionBytes(t, fx.dash, fx.id, id)
		if status != http.StatusOK || !strings.Contains(body, "<script>alert(1)</script>") {
			t.Fatalf("the version = %d %q", status, body)
		}
		if got := header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
			t.Fatalf("a version holding a script tag is answered as %q", got)
		}
		if got := header.Get("Content-Security-Policy"); got != "sandbox" {
			t.Fatalf("Content-Security-Policy = %q, want sandbox", got)
		}
		if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
		}
	})

	t.Run("a restored document's replaced bytes are in its history", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		page := mustRecentVersions(t, fx.dash, fx.id)
		older := versionWith(t, fx.dash, fx.id, "kept older")
		status, body := restoreVersion(t, fx.dash, fx.id, older, *page.Etag)
		if status != http.StatusOK {
			t.Fatalf("the restore = %d %s", status, body)
		}
		onDisk, err := os.ReadFile(fx.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(onDisk), "kept older") {
			t.Fatalf("the document on disk = %q, want the version that was restored", onDisk)
		}
		var answer struct {
			Etag *string `json:"etag"`
		}
		if err := json.Unmarshal([]byte(body), &answer); err != nil || answer.Etag == nil {
			t.Fatalf("the restore answered %s", body)
		}
		if *answer.Etag != specwire.Etag(onDisk) {
			t.Fatalf("the restore answered etag %q, want the stamp of the bytes it left %q", *answer.Etag, specwire.Etag(onDisk))
		}
		if versionWith(t, fx.dash, fx.id, "current bytes") == "" {
			t.Fatal("the bytes the restore replaced are not in the document's history")
		}
	})

	t.Run("a restore with an old etag changes nothing", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		page := mustRecentVersions(t, fx.dash, fx.id)
		older := versionWith(t, fx.dash, fx.id, "kept older")
		if status, body := restoreVersion(t, fx.dash, fx.id, older, *page.Etag); status != http.StatusOK {
			t.Fatalf("the first restore = %d %s", status, body)
		}
		restored, err := os.ReadFile(fx.path)
		if err != nil {
			t.Fatal(err)
		}

		// A save lands after the person looked, and the dashboard still holds the
		// etag from before it.
		saved := "<!doctype html>\n<html><body><p>saved since</p></body></html>\n"
		writeTestFile(t, fx.path, saved)
		status, body := restoreVersion(t, fx.dash, fx.id, older, *page.Etag)
		if status != http.StatusPreconditionFailed || !strings.Contains(body, `"code":"stale"`) {
			t.Fatalf("a restore with the old etag = %d %s, want 412 stale", status, body)
		}
		onDisk, err := os.ReadFile(fx.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(onDisk) != saved {
			t.Fatalf("a refused restore changed the document from %q to %q (restored bytes were %q)", saved, onDisk, restored)
		}
	})

	t.Run("a restore without an etag is refused", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		older := versionWith(t, fx.dash, fx.id, "kept older")
		status, body := restoreVersion(t, fx.dash, fx.id, older, "")
		if status != http.StatusPreconditionRequired || !strings.Contains(body, `"code":"etag-required"`) {
			t.Fatalf("a restore with no If-Match = %d %s, want 428 etag-required", status, body)
		}
		onDisk, err := os.ReadFile(fx.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(onDisk) != current {
			t.Fatalf("a refused restore changed the document to %q", onDisk)
		}
	})

	t.Run("an unknown document or version is not there", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		for _, target := range []string{
			"/api/app/documents/0123456789abcdef/versions",
			"/api/app/documents/nope/versions",
		} {
			if status, body := fx.dash.api(http.MethodGet, target, nil); status != http.StatusNotFound {
				t.Fatalf("%s = %d %s, want 404", target, status, body)
			}
		}
		if status, _, body := versionBytes(t, fx.dash, fx.id, "2026-01-02-03-04-05-006Z.html"); status != http.StatusNotFound {
			t.Fatalf("an unknown version = %d %s, want 404", status, body)
		}
		if status, body := restoreVersion(t, fx.dash, fx.id, "2026-01-02-03-04-05-006Z.html", *mustRecentVersions(t, fx.dash, fx.id).Etag); status != http.StatusNotFound {
			t.Fatalf("restoring an unknown version = %d %s, want 404", status, body)
		}
	})

	t.Run("a document that is neither open nor in a trusted folder is not restorable", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		older := versionWith(t, fx.dash, fx.id, "kept older")
		etag := *mustRecentVersions(t, fx.dash, fx.id).Etag
		closeTestSites(t, fx.app)

		status, body := restoreVersion(t, fx.dash, fx.id, older, etag)
		if status != http.StatusConflict || !strings.Contains(body, `"code":"not-restorable"`) {
			t.Fatalf("restoring a closed document outside a trusted folder = %d %s, want 409 not-restorable", status, body)
		}
		onDisk, err := os.ReadFile(fx.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(onDisk) != current {
			t.Fatalf("a refused restore changed the document to %q", onDisk)
		}
	})

	t.Run("a document that is gone still lists its versions, with no etag", func(t *testing.T) {
		fx := newRecentVersionsFixture(t, current)
		if err := os.Remove(fx.path); err != nil {
			t.Fatal(err)
		}
		page := mustRecentVersions(t, fx.dash, fx.id)
		if len(page.Items) != 2 || page.Etag != nil {
			t.Fatalf("a missing document's versions = %+v, etag %v", page.Items, page.Etag)
		}
		for _, item := range page.Items {
			if item.Restorable {
				t.Fatalf("a version of a missing document reads as restorable: %+v", item)
			}
		}
		older := versionWith(t, fx.dash, fx.id, "kept older")
		if status, body := restoreVersion(t, fx.dash, fx.id, older, specwire.Etag([]byte(current))); status != http.StatusPreconditionFailed {
			t.Fatalf("restoring a missing document = %d %s, want 412", status, body)
		}
	})
}
