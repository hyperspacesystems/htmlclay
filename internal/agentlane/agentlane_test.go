package agentlane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const agentTestPort = 24190

var agentTestHost = "127.0.0.1:" + strconv.Itoa(agentTestPort)

const (
	agentServedPath  = "/Users/x/notes/index.html"
	agentTrustedPath = "/Users/x/proj/page.html"
	agentOpenPath    = "/Users/x/docs/loose.html"
)

const (
	agentServedHTML  = "<!doctype html><html><body><p>served</p></body></html>"
	agentTrustedHTML = "<!doctype html><html><body><p>trusted</p></body></html>"
	agentOpenHTML    = "<!doctype html><html><body><p>open</p></body></html>"
	agentVersionHTML = "<!doctype html><html><body><p>kept</p></body></html>"
)

// fakePage is one page of a fake history, filed under the cursor that asks for
// it.
type fakePage struct {
	rows []Version
	next string
}

// fakeAccess is one document's reads.
type fakeAccess struct {
	html    string
	etag    string
	pages   map[string]fakePage
	version map[string]string
}

func (f fakeAccess) Read(ctx context.Context) ([]byte, string, error) {
	return []byte(f.html), f.etag, nil
}

func (f fakeAccess) Versions(ctx context.Context, before string, limit int) ([]Version, string, error) {
	page := f.pages[before]
	return page.rows, page.next, nil
}

func (f fakeAccess) Save(ctx context.Context, html []byte, ifMatch, saveID string) (WriteResult, error) {
	return WriteResult{}, ErrNotReachable
}

func (f fakeAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (WriteResult, error) {
	return WriteResult{}, ErrNotReachable
}

func (f fakeAccess) Version(ctx context.Context, id string) ([]byte, error) {
	html, ok := f.version[id]
	if !ok {
		return nil, ErrNotFound
	}
	return []byte(html), nil
}

// fakeHost is an app whose documents are the ones the test wrote down.
type fakeHost struct {
	docs      []Document
	truncated bool
	access    map[string]fakeAccess
	limit     int
}

func (h *fakeHost) Version() string { return "9.9.9" }

func (h *fakeHost) List(ctx context.Context, limit int) ([]Document, bool, error) {
	h.limit = limit
	return h.docs, h.truncated, nil
}

func (h *fakeHost) Access(ctx context.Context, path string) (Access, Document, error) {
	access, ok := h.access[path]
	if !ok {
		return nil, Document{}, ErrNotReachable
	}
	for _, doc := range h.docs {
		if doc.Path == path {
			return access, doc, nil
		}
	}
	return access, Document{Path: path}, nil
}

// agentTestHostWith is one fake app with a document of each kind, a history of
// three versions spread over two pages, and a version no page holds.
func agentTestHostWith() *fakeHost {
	history := fakeAccess{
		html: agentTrustedHTML,
		etag: `"etag-trusted"`,
		pages: map[string]fakePage{
			"": {
				rows: []Version{
					{ID: "v1", CreatedAt: "2026-01-03T00:00:00Z", Size: 10, Kind: "saved", Pinned: true, Restorable: true},
					{ID: "v2", CreatedAt: "2026-01-02T00:00:00Z", Size: 11, Kind: "saved", Restorable: true},
				},
				next: "v2",
			},
			"v2": {
				rows: []Version{{ID: "v3", CreatedAt: "2026-01-01T00:00:00Z", Size: 12, Kind: "saved", Restorable: true}},
			},
		},
		version: map[string]string{"v1": agentVersionHTML, "v2": agentVersionHTML, "v3": agentVersionHTML},
	}
	unlisted := fakeAccess{
		version: map[string]string{"v9": agentVersionHTML},
	}
	return &fakeHost{
		docs: []Document{
			{Path: agentOpenPath, URL: "http://loose.localhost:24190/docs/loose.html", Kind: "open", Open: true},
			{Path: agentServedPath, URL: "http://notes.htmlclay.localhost:24190/index.html", Folder: "notes", Kind: "served"},
			{Path: agentTrustedPath, URL: "http://proj.localhost:24190/page.html", Folder: "proj", Kind: "trusted"},
		},
		access: map[string]fakeAccess{
			agentServedPath:               {html: agentServedHTML, etag: `"etag-served"`},
			agentTrustedPath:              history,
			agentOpenPath:                 {html: agentOpenHTML, etag: `"etag-open"`},
			"/Users/x/proj/unlisted.html": unlisted,
		},
	}
}

// agentCall drives the lane's handler directly, with the bare host its admission
// requires unless the test says otherwise.
func agentCall(t *testing.T, host http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://"+agentTestHost+target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	host.ServeHTTP(rec, req)
	return rec
}

// agentCallWithHost is agentCall with another Host header.
func agentCallWithHost(t *testing.T, host http.Handler, hostHeader, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://"+hostHeader+target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	host.ServeHTTP(rec, req)
	return rec
}

// whoamiBody is a whoami answer.
type whoamiBody struct {
	Host         string   `json:"host"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

// documentBody is one document's bytes and stamp.
type documentBody struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url"`
	ETag string `json:"etag"`
	HTML string `json:"html"`
}

// decodeAgentBody decodes one JSON answer.
func decodeAgentBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%d answered %q, which is not JSON: %v", rec.Code, rec.Body.String(), err)
	}
	return out
}

// agentRefusal answers the code a refusal carries.
func agentRefusal(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decodeAgentBody[envelope](t, rec).Error.Code
}

// TestM3AgentReads is step 7.37: the read-only lane's routes, its envelope, its
// admission and its ids, over a fake Host.
func TestM3AgentReads(t *testing.T) {
	t.Run("whoami names the product, its version and what it offers", func(t *testing.T) {
		host := agentTestHostWith()
		rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, "/_/agent/v1/whoami", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("whoami = %d %s", rec.Code, rec.Body.String())
		}
		body := decodeAgentBody[whoamiBody](t, rec)
		if body.Host != "htmlclay" || body.Version != "9.9.9" {
			t.Fatalf("whoami = %+v", body)
		}
		if strings.Join(body.Capabilities, ",") != "read,save,versions,restore" {
			t.Fatalf("whoami capabilities = %v, want read, save, versions and restore", body.Capabilities)
		}
	})

	t.Run("a list answers every document with its id", func(t *testing.T) {
		host := agentTestHostWith()
		rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, "/_/agent/v1/documents", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("documents = %d %s", rec.Code, rec.Body.String())
		}
		if host.limit != ListLimit {
			t.Fatalf("the list asked the host for %d rows, want %d", host.limit, ListLimit)
		}
		body := decodeAgentBody[listBody](t, rec)
		if body.Truncated || len(body.Documents) != len(host.docs) {
			t.Fatalf("documents = %+v", body)
		}
		for i, doc := range host.docs {
			row := body.Documents[i]
			if row.ID != DocumentID(doc.Path) || row.Path != doc.Path || row.Kind != doc.Kind || row.Open != doc.Open || row.Folder != doc.Folder || row.URL != doc.URL {
				t.Fatalf("row %d = %+v, want %+v as %s", i, row, doc, DocumentID(doc.Path))
			}
		}
		if back, ok := PathFromID(body.Documents[1].ID); !ok || back != agentServedPath {
			t.Fatalf("the list's served id names %q, want %q", back, agentServedPath)
		}
	})

	t.Run("a document answers its bytes, its stamp and an ETag header", func(t *testing.T) {
		host := agentTestHostWith()
		for _, doc := range host.docs {
			target := "/_/agent/v1/documents/" + DocumentID(doc.Path)
			rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s = %d %s", doc.Path, rec.Code, rec.Body.String())
			}
			body := decodeAgentBody[documentBody](t, rec)
			if body.ID != DocumentID(doc.Path) || body.Path != doc.Path || body.URL != doc.URL {
				t.Fatalf("%s = %+v", doc.Path, body)
			}
			if body.ETag != host.access[doc.Path].etag || rec.Header().Get("ETag") != body.ETag {
				t.Fatalf("%s etag = %q, header %q", doc.Path, body.ETag, rec.Header().Get("ETag"))
			}
			if body.HTML != host.access[doc.Path].html {
				t.Fatalf("%s html = %q", doc.Path, body.HTML)
			}
		}
	})

	t.Run("a version page answers its rows and the cursor", func(t *testing.T) {
		host := agentTestHostWith()
		target := "/_/agent/v1/documents/" + DocumentID(agentTrustedPath) + "/versions"
		first := decodeAgentBody[versionsBody](t, agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil))
		if len(first.Versions) != 2 || first.Versions[0].ID != "v1" || !first.Versions[0].Pinned || first.Versions[0].CreatedAt != "2026-01-03T00:00:00Z" || first.Versions[0].Size != 10 {
			t.Fatalf("the first page = %+v", first)
		}
		if first.NextBefore == nil || *first.NextBefore != "v2" {
			t.Fatalf("the first page's cursor = %v, want v2", first.NextBefore)
		}
		second := decodeAgentBody[versionsBody](t, agentCall(t, Handler(host, agentTestPort), http.MethodGet, target+"?before=v2", nil))
		if len(second.Versions) != 1 || second.Versions[0].ID != "v3" || second.NextBefore != nil {
			t.Fatalf("the second page = %+v, want v3 and no cursor", second)
		}
	})

	t.Run("one version answers its bytes and the time a later page holds", func(t *testing.T) {
		host := agentTestHostWith()
		target := "/_/agent/v1/documents/" + DocumentID(agentTrustedPath) + "/versions/v3"
		rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("version = %d %s", rec.Code, rec.Body.String())
		}
		body := decodeAgentBody[map[string]any](t, rec)
		if body["id"] != "v3" || body["html"] != agentVersionHTML || body["createdAt"] != "2026-01-01T00:00:00Z" {
			t.Fatalf("version = %+v", body)
		}
	})

	t.Run("a version no page holds answers an empty creation time", func(t *testing.T) {
		host := agentTestHostWith()
		target := "/_/agent/v1/documents/" + DocumentID("/Users/x/proj/unlisted.html") + "/versions/v9"
		rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("version = %d %s", rec.Code, rec.Body.String())
		}
		if body := decodeAgentBody[map[string]any](t, rec); body["createdAt"] != "" {
			t.Fatalf("an unlisted version's createdAt = %v, want an empty string", body["createdAt"])
		}
	})

	t.Run("a method other than GET answers 405", func(t *testing.T) {
		host := agentTestHostWith()
		id := DocumentID(agentTrustedPath)
		for _, target := range []string{
			"/_/agent/v1/whoami",
			"/_/agent/v1/documents",
			"/_/agent/v1/documents/" + id,
			"/_/agent/v1/documents/" + id + "/versions",
			"/_/agent/v1/documents/" + id + "/versions/v1",
		} {
			for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
				if method == http.MethodPut && target == "/_/agent/v1/documents/"+id {
					continue // a PUT on a document is a save (7.41a)
				}
				rec := agentCall(t, Handler(host, agentTestPort), method, target, nil)
				if rec.Code != http.StatusMethodNotAllowed || agentRefusal(t, rec) != "method-not-allowed" {
					t.Fatalf("%s %s = %d %s", method, target, rec.Code, rec.Body.String())
				}
			}
		}
		restore := "/_/agent/v1/documents/" + id + "/versions/v1/restore"
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			rec := agentCall(t, Handler(host, agentTestPort), method, restore, nil)
			if rec.Code != http.StatusMethodNotAllowed || agentRefusal(t, rec) != "method-not-allowed" {
				t.Fatalf("%s %s = %d %s", method, restore, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("a route the lane does not have answers 404", func(t *testing.T) {
		host := agentTestHostWith()
		for _, target := range []string{"/_/agent/v1/", "/_/agent/v1/nothing", "/_/agent/v1/documents/a/b/c/d", "/other"} {
			rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil)
			if rec.Code != http.StatusNotFound || agentRefusal(t, rec) != "not-found" {
				t.Fatalf("%s = %d %s", target, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("a bad id answers 400", func(t *testing.T) {
		host := agentTestHostWith()
		for _, id := range []string{"doc_1", "p_", "p_!!!!"} {
			for _, target := range []string{"/_/agent/v1/documents/" + id, "/_/agent/v1/documents/" + id + "/versions", "/_/agent/v1/documents/" + id + "/versions/v1"} {
				rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil)
				if rec.Code != http.StatusBadRequest || agentRefusal(t, rec) != "bad-request" {
					t.Fatalf("%s = %d %s", target, rec.Code, rec.Body.String())
				}
			}
		}
	})

	t.Run("every browser header is refused, even empty", func(t *testing.T) {
		host := agentTestHostWith()
		target := "/_/agent/v1/documents/" + DocumentID(agentTrustedPath)
		for _, name := range []string{"Origin", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest", "Cookie"} {
			rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, map[string]string{name: ""})
			if rec.Code != http.StatusForbidden || agentRefusal(t, rec) != "forbidden" {
				t.Fatalf("with %s = %d %s", name, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), agentTrustedHTML) {
				t.Fatalf("with %s the lane answered the document", name)
			}
		}
	})

	t.Run("another Host is refused", func(t *testing.T) {
		host := agentTestHostWith()
		target := "/_/agent/v1/documents/" + DocumentID(agentTrustedPath)
		for _, hostHeader := range []string{"localhost:" + strconv.Itoa(agentTestPort), "127.0.0.1:1" + strconv.Itoa(agentTestPort), "proj.localhost:" + strconv.Itoa(agentTestPort), "127.0.0.1"} {
			rec := agentCallWithHost(t, Handler(host, agentTestPort), hostHeader, http.MethodGet, target, nil)
			if rec.Code != http.StatusForbidden || agentRefusal(t, rec) != "forbidden" {
				t.Fatalf("Host %s = %d %s", hostHeader, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("a document nothing may reach answers not-reachable", func(t *testing.T) {
		host := agentTestHostWith()
		rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, "/_/agent/v1/documents/"+DocumentID("/Users/x/closed.html"), nil)
		if rec.Code != http.StatusNotFound || agentRefusal(t, rec) != "not-reachable" {
			t.Fatalf("an unreachable document = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("a version that is not there answers not-found", func(t *testing.T) {
		host := agentTestHostWith()
		target := "/_/agent/v1/documents/" + DocumentID(agentTrustedPath) + "/versions/v99"
		rec := agentCall(t, Handler(host, agentTestPort), http.MethodGet, target, nil)
		if rec.Code != http.StatusNotFound || agentRefusal(t, rec) != "not-found" {
			t.Fatalf("a version that is not there = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("an id round-trips its path", func(t *testing.T) {
		for _, path := range []string{agentServedPath, "/Users/x/a b/c.html", "/Users/x/é.html"} {
			back, ok := PathFromID(DocumentID(path))
			if !ok || back != path {
				t.Fatalf("%q round-trips to %q, %v", path, back, ok)
			}
			if !strings.HasPrefix(DocumentID(path), "p_") {
				t.Fatalf("%q's id is not prefixed", path)
			}
		}
		if _, ok := PathFromID("x_" + "abc"); ok {
			t.Fatal("an id without the prefix was accepted")
		}
	})
}
