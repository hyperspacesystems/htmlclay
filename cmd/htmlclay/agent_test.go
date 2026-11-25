package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/agentlane"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// agentLaneHost is the bare loopback host the lane answers on, and agentLanePrefix
// is where its routes start.
var agentLaneHost = "127.0.0.1:" + strconv.Itoa(servedTestPort)

const agentLanePrefix = "/_/agent/v1"

// testAgentToken is the token the agent fixtures hand the lane, and testAgentTokenFile
// is the file a refusal names.
const (
	testAgentToken     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testAgentTokenFile = "/test/agent-token"
)

// agentLaneRow is one row of the lane's list.
type agentLaneRow struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	URL    string `json:"url"`
	Folder string `json:"folder"`
	Kind   string `json:"kind"`
	Open   bool   `json:"open"`
}

// agentLaneList is the lane's list answer.
type agentLaneList struct {
	Documents []agentLaneRow `json:"documents"`
	Truncated bool           `json:"truncated"`
}

// agentLaneDocument is one document's bytes and stamp.
type agentLaneDocument struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url"`
	ETag string `json:"etag"`
	HTML string `json:"html"`
}

// agentLaneVersions is one page of a document's history.
type agentLaneVersions struct {
	Versions   []agentlane.Version `json:"versions"`
	NextBefore *string             `json:"nextBefore"`
}

// agentLaneRefusal is one refusal's envelope.
type agentLaneRefusal struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// agentLaneFixture is one app with a served folder, a live trusted folder, an open
// loose file, a loose file nobody opened, and a document in a temporary folder no
// folder owns.
type agentLaneFixture struct {
	app     *app
	served  string
	trusted string
	open    string
	closed  string
	outside string
}

func newAgentLaneFixture(t *testing.T) *agentLaneFixture {
	t.Helper()
	home := evalTempHome(t)
	servedDir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(servedDir, "index.html"), servedTestDocument)
	trustedDir := filepath.Join(home, "proj")
	writeTestFile(t, filepath.Join(trustedDir, "page.html"), documentAccessDocument)
	openPath := filepath.Join(home, "docs", "open.html")
	writeTestFile(t, openPath, documentAccessDocument)
	closedPath := filepath.Join(home, "docs", "closed.html")
	writeTestFile(t, closedPath, documentAccessDocument)
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "away.html")
	writeTestFile(t, outsidePath, documentAccessDocument)

	a := newServedTestApp(t, home, t.TempDir())
	a.rt.agentToken, a.rt.agentTokenFile = testAgentToken, testAgentTokenFile
	if _, refusal, err := a.serveFolder(servedDir, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}
	if err := a.trustFolder(trustedDir); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	a.openForTest(t, openPath)
	return &agentLaneFixture{
		app:     a,
		served:  filepath.Join(servedDir, "index.html"),
		trusted: filepath.Join(trustedDir, "page.html"),
		open:    openPath,
		closed:  closedPath,
		outside: outsidePath,
	}
}

// agentLaneRequest sends one request through the app's one handler on the lane's
// bare host.
func agentLaneRequest(t *testing.T, a *app, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	if _, ok := headers["Authorization"]; !ok && a.rt.agentToken != "" {
		sent := make(map[string]string, len(headers)+1)
		for name, value := range headers {
			sent[name] = value
		}
		sent["Authorization"] = "Bearer " + a.rt.agentToken
		headers = sent
	}
	return servedRequest(t, a, method, agentLaneHost, agentLanePrefix+target, "", headers)
}

// agentLaneGet reads one document's bytes and stamp.
func agentLaneGet(t *testing.T, a *app, path string) (agentLaneDocument, *httptest.ResponseRecorder) {
	t.Helper()
	rec := agentLaneRequest(t, a, http.MethodGet, "/documents/"+agentlane.DocumentID(path), nil)
	var doc agentLaneDocument
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &doc) != nil {
		t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
	}
	return doc, rec
}

// agentLaneRows asks the lane's list and decodes it.
func agentLaneRows(t *testing.T, a *app) agentLaneList {
	t.Helper()
	rec := agentLaneRequest(t, a, http.MethodGet, "/documents", nil)
	var list agentLaneList
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("GET documents = %d %s", rec.Code, rec.Body.String())
	}
	return list
}

// agentLaneRowFor answers one row of a list by the path it names.
func agentLaneRowFor(t *testing.T, list agentLaneList, path string) agentLaneRow {
	t.Helper()
	for _, row := range list.Documents {
		if row.Path == path {
			return row
		}
	}
	t.Fatalf("the list has no row for %s: %+v", path, list.Documents)
	return agentLaneRow{}
}

// agentLaneFileBytes reads one file as the test writes it.
func agentLaneFileBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestM3AgentLaneReads is step 7.37: the read-only lane lists and reads the three
// kinds of document a local program may reach, refuses everything that looks like
// a browser, and never registers anything on the way.
func TestM3AgentLaneReads(t *testing.T) {
	t.Run("read and list for the three kinds", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		list := agentLaneRows(t, fx.app)
		if list.Truncated {
			t.Fatalf("the list is truncated: %+v", list)
		}
		for _, want := range []struct {
			path   string
			kind   string
			folder string
			open   bool
		}{
			{fx.served, "served", "notes", false},
			{fx.trusted, "trusted", "proj", false},
			{fx.open, "open", "", true},
		} {
			row := agentLaneRowFor(t, list, want.path)
			if row.Kind != want.kind || row.Folder != want.folder || row.Open != want.open {
				t.Fatalf("%s = %+v, want kind %q in folder %q open %v", want.path, row, want.kind, want.folder, want.open)
			}
			if row.ID != agentlane.DocumentID(want.path) {
				t.Fatalf("%s's id = %q, want %q", want.path, row.ID, agentlane.DocumentID(want.path))
			}
			if row.URL == "" {
				t.Fatalf("%s has no address in the list", want.path)
			}
		}

		whoami := agentLaneRequest(t, fx.app, http.MethodGet, "/whoami", nil)
		if whoami.Code != http.StatusOK || !strings.Contains(whoami.Body.String(), `"host":"htmlclay"`) {
			t.Fatalf("GET whoami = %d %s", whoami.Code, whoami.Body.String())
		}
		if body := whoami.Body.String(); !strings.Contains(body, `"read"`) || !strings.Contains(body, `"versions"`) {
			t.Fatalf("whoami = %s, want read and versions", body)
		}

		for _, path := range []string{fx.served, fx.trusted, fx.open} {
			doc, rec := agentLaneGet(t, fx.app, path)
			if doc.HTML != agentLaneFileBytes(t, path) {
				t.Fatalf("%s: the lane answered %q, and the file holds %q", path, doc.HTML, agentLaneFileBytes(t, path))
			}
			if doc.ETag == "" || rec.Header().Get("ETag") != doc.ETag {
				t.Fatalf("%s: etag %q, header %q", path, doc.ETag, rec.Header().Get("ETag"))
			}
			if doc.ID != agentlane.DocumentID(path) || doc.Path != path {
				t.Fatalf("%s = %+v", path, doc)
			}
		}
	})

	t.Run("a document in a live trusted folder that is not open is reachable", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		if _, open := siteOf(fx.app, fx.trusted); open {
			t.Fatalf("%s is registered, and this subtest needs a document nobody opened", fx.trusted)
		}
		if row := agentLaneRowFor(t, agentLaneRows(t, fx.app), fx.trusted); row.Kind != "trusted" || row.Open {
			t.Fatalf("%s = %+v, want trusted and not open", fx.trusted, row)
		}
		doc, _ := agentLaneGet(t, fx.app, fx.trusted)
		if doc.HTML != documentAccessDocument {
			t.Fatalf("%s: the lane answered %q", fx.trusted, doc.HTML)
		}
		if _, open := siteOf(fx.app, fx.trusted); open {
			t.Fatalf("reading %s registered it", fx.trusted)
		}
	})

	t.Run("a closed loose file and a file in a temporary folder are not-reachable", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		list := agentLaneRows(t, fx.app)
		for _, path := range []string{fx.closed, fx.outside} {
			for _, row := range list.Documents {
				if row.Path == path {
					t.Fatalf("the list names %s, which no folder owns and nobody opened", path)
				}
			}
			rec := agentLaneRequest(t, fx.app, http.MethodGet, "/documents/"+agentlane.DocumentID(path), nil)
			var refusal agentLaneRefusal
			if rec.Code != http.StatusNotFound || json.Unmarshal(rec.Body.Bytes(), &refusal) != nil || refusal.Error.Code != "not-reachable" {
				t.Fatalf("GET %s = %d %s, want 404 not-reachable", path, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "reached") {
				t.Fatalf("%s: the refusal carried the document's bytes", path)
			}
		}
		if _, open := siteOf(fx.app, fx.closed); open {
			t.Fatalf("asking for %s registered it", fx.closed)
		}
	})

	t.Run("a document's versions are listed and one is read", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		_, etag := readDocumentAccess(t, fx.app, fx.served)
		body := strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1)
		if code := saveWithStamp(t, fx.app, fx.served, body, etag); code != http.StatusOK {
			t.Fatalf("the save that keeps a version = %d, want 200", code)
		}
		rec := agentLaneRequest(t, fx.app, http.MethodGet, "/documents/"+agentlane.DocumentID(fx.served)+"/versions", nil)
		var page agentLaneVersions
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
			t.Fatalf("GET versions = %d %s", rec.Code, rec.Body.String())
		}
		if len(page.Versions) == 0 {
			t.Fatalf("the versions page is empty: %s", rec.Body.String())
		}
		version := page.Versions[0]
		if version.CreatedAt == "" || version.Size == 0 {
			t.Fatalf("the version row = %+v", version)
		}
		rec = agentLaneRequest(t, fx.app, http.MethodGet, "/documents/"+agentlane.DocumentID(fx.served)+"/versions/"+version.ID, nil)
		var one struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
			HTML      string `json:"html"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &one) != nil {
			t.Fatalf("GET one version = %d %s", rec.Code, rec.Body.String())
		}
		kept := agentLaneFileBytes(t, fx.served)
		if one.ID != version.ID || one.CreatedAt != version.CreatedAt || one.HTML != kept {
			t.Fatalf("the version = %+v, want %q kept at %q", one, kept, version.CreatedAt)
		}
		if agentLaneFileBytes(t, fx.served) != kept {
			t.Fatalf("reading a version moved the document")
		}
	})

	t.Run("a browser header or another Host never reaches a document", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		target := "/documents/" + agentlane.DocumentID(fx.trusted)
		refused := func(name string, rec *httptest.ResponseRecorder) {
			t.Helper()
			var refusal agentLaneRefusal
			if rec.Code != http.StatusForbidden || json.Unmarshal(rec.Body.Bytes(), &refusal) != nil || refusal.Error.Code != "forbidden" {
				t.Fatalf("%s = %d %s, want 403 forbidden", name, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "<p>reached</p>") {
				t.Fatalf("%s was answered with the document", name)
			}
		}
		for name, headers := range map[string]map[string]string{
			"Origin":         {"Origin": "http://localhost:3000"},
			"Sec-Fetch-Site": {"Sec-Fetch-Site": "same-origin"},
			"Cookie":         {"Cookie": "session=1"},
		} {
			refused(name, agentLaneRequest(t, fx.app, http.MethodGet, target, headers))
		}
		refused("another Host", servedRequest(t, fx.app, http.MethodGet, "localhost:"+strconv.Itoa(servedTestPort), agentLanePrefix+target, "", nil))
	})

	t.Run("the same path on the private shell listener answers 404", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		port := servedTestPort
		secret := "agent-lane-secret"
		shell := shellapi.New(shellBackend{fx.app}, secret, port, make(chan struct{}))
		req := httptest.NewRequest(http.MethodGet, "http://"+agentLaneHost+agentLanePrefix+"/documents/"+agentlane.DocumentID(fx.trusted), nil)
		req.Host = agentLaneHost
		req.Header.Set("Authorization", "Bearer "+secret)
		rec := httptest.NewRecorder()
		shell.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("the lane's path on the private listener = %d %s, want 404", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "<p>reached</p>") {
			t.Fatal("the private listener answered the document")
		}
	})
}

// TestM3AgentListStopsAtTheLimit proves the folder walks stop one row past the
// limit, so a large trusted folder is never walked to the bottom.
func TestM3AgentListStopsAtTheLimit(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "big")
	for i := 0; i < 6; i++ {
		writeTestFile(t, filepath.Join(dir, fmt.Sprintf("p%d.html", i)), documentAccessDocument)
	}
	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(dir); err != nil {
		t.Fatal(err)
	}
	var walked int
	a.walkAgentFolder(context.Background(), dir, func() bool { return walked > 2 }, func(string, string) { walked++ })
	if walked != 3 {
		t.Fatalf("the walk visited %d documents, want it to stop at 3", walked)
	}
	docs, truncated, err := agentHost{a: a}.List(context.Background(), 2)
	if err != nil || !truncated || len(docs) != 2 {
		t.Fatalf("List(2) = %d rows, truncated %v, err %v; want 2 rows, truncated", len(docs), truncated, err)
	}
}
