package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/agentlane"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/wire"
)

// agentDataDocument publishes a title, a note and a list through its own api
// rules tag.
const agentDataDocument = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Data</title>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1","note":"p.note","items":["ul.items li",{"name":".name"}]}</script>
</head>
<body>
<h1>List</h1>
<p class="note">first note</p>
<ul class="items"><li><span class="name">alpha</span></li><li><span class="name">beta</span></li></ul>
</body>
</html>
`

const (
	agentDataRead     = `{"title":"List","note":"first note","items":[{"name":"alpha"},{"name":"beta"}]}`
	agentDataRename   = `{"title":"Renamed"}`
	agentDataRemoval  = `{"note":"","items":[{"name":"alpha"}]}`
	agentDataRemoved  = `{"title":"List","note":"","items":[{"name":"alpha"}]}`
	agentDataTabGone  = `[{"sheet":"Budget","delete":true}]`
	agentDataReadNote = "This write would remove data. Read the document first and send its etag as If-Match."
)

// agentDataFixture is one app with a served folder, a live trusted folder nobody
// opened and open loose files. Each owner holds a document that publishes data;
// the served folder and the loose files also hold one that publishes none, and
// one loose file is a sheet with three tabs.
type agentDataFixture struct {
	app         *app
	served      string
	servedPlain string
	trusted     string
	open        string
	openPlain   string
	sheet       string
}

func newAgentDataFixture(t *testing.T) *agentDataFixture {
	t.Helper()
	sheet, err := os.ReadFile(filepath.Join("..", "..", "internal", "dataapi", "testdata", "conformance", "sheet", "sheet-patch-delete-by-name.html"))
	if err != nil {
		t.Fatal(err)
	}
	home := evalTempHome(t)
	fx := &agentDataFixture{
		served:      filepath.Join(home, "notes", "index.html"),
		servedPlain: filepath.Join(home, "notes", "plain.html"),
		trusted:     filepath.Join(home, "proj", "page.html"),
		open:        filepath.Join(home, "docs", "open.html"),
		openPlain:   filepath.Join(home, "docs", "plain.html"),
		sheet:       filepath.Join(home, "docs", "sheet.htmlclay"),
	}
	writeTestFile(t, fx.served, agentDataDocument)
	writeTestFile(t, fx.servedPlain, servedTestDocument)
	writeTestFile(t, fx.trusted, agentDataDocument)
	writeTestFile(t, fx.open, agentDataDocument)
	writeTestFile(t, fx.openPlain, documentAccessDocument)
	writeTestFile(t, fx.sheet, string(sheet))

	fx.app = newServedTestApp(t, home, t.TempDir())
	if _, refusal, err := fx.app.serveFolder(filepath.Dir(fx.served), "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}
	if err := fx.app.trustFolder(filepath.Dir(fx.trusted)); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	for _, path := range []string{fx.open, fx.openPlain, fx.sheet} {
		fx.app.openForTest(t, path)
	}
	return fx
}

// agentLaneData is a data read's answer.
type agentLaneData struct {
	Data json.RawMessage `json:"data"`
	ETag string          `json:"etag"`
}

// agentLaneDataOf reads one document's data and the stamp its data write compares.
func agentLaneDataOf(t *testing.T, a *app, path string) agentLaneData {
	t.Helper()
	rec := agentLaneRequest(t, a, http.MethodGet, "/documents/"+agentlane.DocumentID(path)+"/data", nil)
	var data agentLaneData
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &data) != nil {
		t.Fatalf("GET data of %s = %d %s", path, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != data.ETag {
		t.Fatalf("ETag header = %q, want %q", got, data.ETag)
	}
	return data
}

// agentLaneDataBody is a data write's JSON body: field is data or patch.
func agentLaneDataBody(field, value, operationID string) string {
	return `{"` + field + `":` + value + `,"operationId":"` + operationID + `"}`
}

// agentLaneDataWrite sends one data write of the document at path.
func agentLaneDataWrite(t *testing.T, a *app, path, field, value, operationID, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	return agentLaneWrite(t, a, http.MethodPost, "/documents/"+agentlane.DocumentID(path)+"/data",
		agentLaneDataBody(field, value, operationID), "If-Match", ifMatch)
}

// unprovenHost is the app as the lane sees it, except that a data write reaches
// its owner naming no version, as If-Match: * does. No request can carry that to
// an owner: the lane's door admits one strong etag only, and both owners answer
// an etag that is not the document's with 412 before their data guard is asked.
// So this is the one way to see what the lane answers when a guard refuses.
type unprovenHost struct{ agentHost }

func (h unprovenHost) Access(ctx context.Context, path string) (agentlane.Access, agentlane.Document, error) {
	access, doc, err := h.agentHost.Access(ctx, path)
	if err != nil {
		return nil, doc, err
	}
	return unprovenAccess{access}, doc, nil
}

type unprovenAccess struct{ agentlane.Access }

func (d unprovenAccess) WriteData(ctx context.Context, write agentlane.DataWrite) (agentlane.WriteResult, error) {
	write.IfMatch = "*"
	return d.Access.WriteData(ctx, write)
}

// unprovenDataWrite sends one data write through the lane's own handler over the
// real owners, with the read its etag proves taken away before the owner sees it.
func unprovenDataWrite(t *testing.T, a *app, path, field, value, operationID, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"http://"+agentLaneHost+agentLanePrefix+"/documents/"+agentlane.DocumentID(path)+"/data",
		strings.NewReader(agentLaneDataBody(field, value, operationID)))
	req.Header.Set("If-Match", ifMatch)
	rec := httptest.NewRecorder()
	agentlane.Handler(unprovenHost{agentHost{a: a}}, a.rt.port).ServeHTTP(rec, req)
	return rec
}

// ownDataRoute posts body to the document's own data route, POST /_/api/<name>:
// as a page of that origin when page is set, with the token a loose file's page
// holds, and as a program otherwise.
func ownDataRoute(t *testing.T, a *app, absPath, body string, page bool) *httptest.ResponseRecorder {
	t.Helper()
	owner, err := a.ResolveDocument(absPath)
	if err != nil {
		t.Fatalf("resolve %s: %v", absPath, err)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	host, target := "", ""
	if owner.Kind == OwnerServed {
		host, target = strings.TrimPrefix(owner.PublicOrigin, "http://"), "/_/api/"+owner.RelativePath
	} else {
		s, ok := siteOf(a, absPath)
		if !ok {
			t.Fatalf("%s is not registered in its site", absPath)
		}
		f, ok := s.sessions.LookupByPath(absPath)
		if !ok {
			t.Fatalf("%s has no save token in its site", absPath)
		}
		host, target = s.host, "/_/api/"+f.RelPath
		if page {
			headers[wire.TokenHeader] = f.Token
		}
	}
	if page {
		headers["Sec-Fetch-Site"] = "same-origin"
		headers["Origin"] = "http://" + host
	}
	return servedRequest(t, a, http.MethodPost, host, target, body, headers)
}

// failingVersionStores makes both owners' version stores refuse a new version.
func failingVersionStores(t *testing.T, fx *agentDataFixture) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only history folder")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores a folder's read-only mode")
	}
	for _, dir := range []string{
		servedHistoryDir(filepath.Dir(fx.served), "index.html", "versions"),
		fx.app.rt.versions.BaseDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	}
}

// TestM3AgentLaneData is step 7.41b through the real app: a program reads and
// writes the data a document publishes, through each owner's own data write. The
// replaced bytes are kept or nothing is written, an old etag is refused, one
// operation id writes once, and the owner's data guard is passed through.
func TestM3AgentLaneData(t *testing.T) {
	t.Run("served", func(t *testing.T) {
		testAgentLaneData(t, true,
			func(fx *agentDataFixture) string { return fx.served },
			func(fx *agentDataFixture) string { return fx.servedPlain })

		t.Run("a patch answers 400 patch-unsupported", func(t *testing.T) {
			fx := newAgentDataFixture(t)
			before := agentLaneFileBytes(t, fx.served)
			read := agentLaneDataOf(t, fx.app, fx.served)
			rec := agentLaneDataWrite(t, fx.app, fx.served, "patch", agentDataTabGone, "op-1", read.ETag)
			refusal := wantAgentLaneRefusal(t, rec, http.StatusBadRequest, "patch-unsupported")
			if len(refusal.Error.Message) > 160 || !strings.Contains(refusal.Error.Message, "whole data only") {
				t.Fatalf("the refusal's sentence = %q", refusal.Error.Message)
			}
			if got := agentLaneFileBytes(t, fx.served); got != before {
				t.Fatalf("the refused patch changed the file: %q", got)
			}
		})

		t.Run("a page with stored page state writes its data with the stamp it was read with", func(t *testing.T) {
			fx := newAgentDataFixture(t)
			folder := filepath.Dir(fx.served)
			path := filepath.Join(folder, "paged.html")
			page := strings.Replace(documentAccessPage, "</head>",
				`<script type="application/json" data-rules-name="api" data-rules-version="1">{"note":"p.note"}</script>`+"\n</head>", 1)
			page = strings.Replace(page, "</body>", `<p class="note">first note</p>`+"\n</body>", 1)
			writeTestFile(t, path, page)
			tag, err := json.Marshal(documentAccessPageTag)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(folder, ".hyperclay", "blockeditor-state", "paged.html.json"),
				`{"v":1,"rev":"`+documentAccessPageRev+`","tag":`+string(tag)+`}`)

			read := agentLaneDataOf(t, fx.app, path)
			if string(read.Data) != `{"note":"first note"}` {
				t.Fatalf("data = %s, want the page's note", read.Data)
			}
			if read.ETag == specwire.Etag([]byte(page)) {
				t.Fatal("the page's stored state was no part of its stamp, so this round trip proved nothing about it")
			}
			if doc, _ := agentLaneGet(t, fx.app, path); read.ETag != doc.ETag {
				t.Fatalf("the data's etag = %q, and the document's is %q", read.ETag, doc.ETag)
			}
			written := wantAgentLaneWrite(t, agentLaneDataWrite(t, fx.app, path, "data", `{"note":"second note"}`, "op-1", read.ETag), "op-1")
			if got := agentLaneFileBytes(t, path); got != strings.Replace(page, "first note", "second note", 1) {
				t.Fatalf("the file holds %q, want the page with the new note", got)
			}
			if after := agentLaneDataOf(t, fx.app, path); after.ETag != written.ETag || string(after.Data) != `{"note":"second note"}` {
				t.Fatalf("the next read answers %s stamped %q, and the write answered %q", after.Data, after.ETag, written.ETag)
			}
			if got := agentLaneVersioned(t, fx.app, path, *written.UndoVersionID); got != page {
				t.Fatalf("undo version %s holds %q, want the page the write replaced", *written.UndoVersionID, got)
			}
		})
	})
	t.Run("loose", func(t *testing.T) {
		testAgentLaneData(t, false,
			func(fx *agentDataFixture) string { return fx.open },
			func(fx *agentDataFixture) string { return fx.openPlain })

		t.Run("a sheet tab delete that does not prove the read answers 428 read-required", testAgentLaneSheetDelete)
	})
	t.Run("an owner that changed since the document was reached is not written to", testAgentLaneDataOwner)
	t.Run("trusted", func(t *testing.T) {
		t.Run("a document nobody opened reads and writes its data", func(t *testing.T) {
			fx := newAgentDataFixture(t)
			before := agentLaneFileBytes(t, fx.trusted)
			read := agentLaneDataOf(t, fx.app, fx.trusted)
			if string(read.Data) != agentDataRead {
				t.Fatalf("data = %s, want %s", read.Data, agentDataRead)
			}
			written := wantAgentLaneWrite(t, agentLaneDataWrite(t, fx.app, fx.trusted, "data", agentDataRename, "op-1", read.ETag), "op-1")
			if got := agentLaneFileBytes(t, fx.trusted); !strings.Contains(got, "<h1>Renamed</h1>") {
				t.Fatalf("the file holds %q, want the new title", got)
			}
			if got := agentLaneVersioned(t, fx.app, fx.trusted, *written.UndoVersionID); got != before {
				t.Fatalf("undo version %s holds %q, want the bytes the write replaced", *written.UndoVersionID, got)
			}
		})
	})
}

// testAgentLaneDataOwner proves each owner resolves the document again once it
// holds the document's lock: an owner told the document is no longer its own
// writes nothing, and neither does the one lane after the owner really changed.
func testAgentLaneDataOwner(t *testing.T) {
	fx := newAgentDataFixture(t)
	ctx := context.Background()
	notOwned := func() bool { return false }
	owner, err := fx.app.ResolveDocument(fx.served)
	if err != nil {
		t.Fatal(err)
	}
	looseSite, ok := siteOf(fx.app, fx.open)
	if !ok {
		t.Fatalf("%s is not registered in its site", fx.open)
	}

	paths := []string{fx.served, fx.trusted, fx.open}
	original, etags, reached := map[string]string{}, map[string]string{}, map[string]DocumentAccess{}
	for _, path := range paths {
		original[path] = agentLaneFileBytes(t, path)
		etags[path] = agentLaneDataOf(t, fx.app, path).ETag
		reached[path] = documentAccessOf(t, fx.app, path)
	}

	_, err = fx.app.served.WriteData(owner.FolderID, owner.RelativePath,
		served.DataWrite{Data: []byte(agentDataRename), IfMatch: etags[fx.served], Owned: notOwned})
	if !errors.Is(err, served.ErrNotOwned) {
		t.Fatalf("a served write the owner no longer holds = %v, want ErrNotOwned", err)
	}
	_, err = looseSite.srv.WriteData(fx.open,
		server.DataWrite{Data: []byte(agentDataRename), IfMatch: etags[fx.open], Owned: notOwned})
	if !errors.Is(err, server.ErrNotOwned) {
		t.Fatalf("a loose write the owner no longer holds = %v, want ErrNotOwned", err)
	}

	if err := fx.app.stopServing(owner.FolderID); err != nil {
		t.Fatalf("stopServing: %v", err)
	}
	if err := fx.app.untrustFolder(filepath.Dir(fx.trusted)); err != nil {
		t.Fatalf("untrustFolder: %v", err)
	}
	if _, refusal, err := fx.app.serveFolder(filepath.Dir(fx.open), "docs"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: %v, %+v", err, refusal)
	}
	for _, path := range paths {
		_, err := reached[path].WriteData(ctx, DataWrite{Data: []byte(agentDataRename), IfMatch: etags[path]})
		if !errors.Is(err, ErrNotReachable) {
			t.Fatalf("%s: a data write through the owner it had before = %v, want ErrNotReachable", path, err)
		}
		if got := agentLaneFileBytes(t, path); got != original[path] {
			t.Fatalf("%s: the refused write changed the file: %q", path, got)
		}
	}
}

// testAgentLaneSheetDelete is a loose sheet's tab delete, which is a patch: the
// owner's data guard refuses it for a program that has not shown it read the
// sheet, and the same patch with the etag of a read lands. A patch answers the
// engine's stale: false for a delete that leaves no formula behind, and true for
// new cells, which no host can recalculate.
func testAgentLaneSheetDelete(t *testing.T) {
	fx := newAgentDataFixture(t)
	before := agentLaneFileBytes(t, fx.sheet)
	read := agentLaneDataOf(t, fx.app, fx.sheet)
	if !strings.Contains(string(read.Data), `"Budget"`) || !strings.Contains(string(read.Data), `"Notes"`) {
		t.Fatalf("the sheet's data = %s, want its Budget and Notes tabs", read.Data)
	}

	unproven := unprovenDataWrite(t, fx.app, fx.sheet, "patch", agentDataTabGone, "op-1", read.ETag)
	refusal := wantAgentLaneRefusal(t, unproven, http.StatusPreconditionRequired, "read-required")
	if refusal.Error.ETag != read.ETag || refusal.Error.Message != agentDataReadNote {
		t.Fatalf("the refusal = %s, want the guard's sentence and the etag %q", unproven.Body.String(), read.ETag)
	}
	if !strings.Contains(unproven.Body.String(), `"wouldRemove":{"items":`) {
		t.Fatalf("the refusal = %s, want what the delete would remove", unproven.Body.String())
	}
	if got := agentLaneFileBytes(t, fx.sheet); got != before {
		t.Fatalf("the refused delete changed the file: %q", got)
	}

	proven := agentLaneDataWrite(t, fx.app, fx.sheet, "patch", agentDataTabGone, "op-2", read.ETag)
	deleted := wantAgentLaneWrite(t, proven, "op-2")
	var answer map[string]json.RawMessage
	if err := json.Unmarshal(proven.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer) != 5 || string(answer["stale"]) != "false" {
		t.Fatalf("the delete answered %s, want etag, changed, operationId, undoVersionId and stale false", proven.Body.String())
	}
	after := agentLaneDataOf(t, fx.app, fx.sheet)
	if strings.Contains(string(after.Data), `"Budget"`) || !strings.Contains(string(after.Data), `"Notes"`) || after.ETag != deleted.ETag {
		t.Fatalf("after the delete the sheet's data = %s stamped %q, want no Budget tab and the stamp %q", after.Data, after.ETag, deleted.ETag)
	}
	if got := agentLaneVersioned(t, fx.app, fx.sheet, *deleted.UndoVersionID); got != before {
		t.Fatalf("undo version %s holds %q, want the sheet before the delete", *deleted.UndoVersionID, got)
	}

	appended := agentLaneDataWrite(t, fx.app, fx.sheet, "patch", `[{"sheet":"Notes","append":[[12]]}]`, "op-3", deleted.ETag)
	wantAgentLaneWrite(t, appended, "op-3")
	if !strings.Contains(appended.Body.String(), `"stale":true`) {
		t.Fatalf("new cells answered %s, want stale true", appended.Body.String())
	}
}

func testAgentLaneData(t *testing.T, served bool, pick, pickPlain func(*agentDataFixture) string) {
	t.Helper()

	t.Run("a data read answers the data and the stamp a write compares", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		read := agentLaneDataOf(t, fx.app, path)
		if string(read.Data) != agentDataRead {
			t.Fatalf("data = %s, want %s", read.Data, agentDataRead)
		}
		if doc, _ := agentLaneGet(t, fx.app, path); read.ETag != doc.ETag {
			t.Fatalf("the data's etag = %q, and the document's is %q", read.ETag, doc.ETag)
		}
	})

	t.Run("a document with no api rules answers 400 no-api-rules", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pickPlain(fx)
		before := agentLaneFileBytes(t, path)
		rec := agentLaneRequest(t, fx.app, http.MethodGet, "/documents/"+agentlane.DocumentID(path)+"/data", nil)
		wantAgentLaneRefusal(t, rec, http.StatusBadRequest, "no-api-rules")
		doc, _ := agentLaneGet(t, fx.app, path)
		rec = agentLaneDataWrite(t, fx.app, path, "data", agentDataRename, "op-1", doc.ETag)
		wantAgentLaneRefusal(t, rec, http.StatusBadRequest, "no-api-rules")
		if got := agentLaneFileBytes(t, path); got != before {
			t.Fatalf("the refused write changed the file: %q", got)
		}
	})

	t.Run("a data write changes the data and keeps the document it replaced", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		read := agentLaneDataOf(t, fx.app, path)
		rec := agentLaneDataWrite(t, fx.app, path, "data", agentDataRename, "op-1", read.ETag)
		written := wantAgentLaneWrite(t, rec, "op-1")
		var answer map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || len(answer) != 4 {
			t.Fatalf("a data write answered %s, want etag, changed, operationId and undoVersionId only", rec.Body.String())
		}
		if got := agentLaneFileBytes(t, path); !strings.Contains(got, "<h1>Renamed</h1>") || !strings.Contains(got, "beta") {
			t.Fatalf("the file holds %q, want the new title and the list it had", got)
		}
		after := agentLaneDataOf(t, fx.app, path)
		if string(after.Data) != strings.Replace(agentDataRead, `"List"`, `"Renamed"`, 1) || after.ETag != written.ETag || after.ETag == read.ETag {
			t.Fatalf("after the write the data = %s stamped %q, want the new title and the stamp the write answered %q", after.Data, after.ETag, written.ETag)
		}
		if !agentLaneHasVersion(agentLaneVersionsOf(t, fx.app, path), *written.UndoVersionID) {
			t.Fatalf("the versions list does not name the undo version %s", *written.UndoVersionID)
		}
		if got := agentLaneVersioned(t, fx.app, path, *written.UndoVersionID); got != before {
			t.Fatalf("undo version %s holds %q, want the document the write replaced", *written.UndoVersionID, got)
		}
	})

	t.Run("a stale If-Match answers 412 with the current etag and changes nothing", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		read := agentLaneDataOf(t, fx.app, path)
		written := wantAgentLaneWrite(t, agentLaneDataWrite(t, fx.app, path, "data", agentDataRename, "op-1", read.ETag), "op-1")
		kept := agentLaneFileBytes(t, path)

		// One late write would remove data. The etag of an older version is a
		// conflict even then, never the guard's read-required.
		for id, late := range map[string]string{"op-2": agentDataRemoval, "op-3": `{"title":"Late"}`} {
			rec := agentLaneDataWrite(t, fx.app, path, "data", late, id, read.ETag)
			refusal := wantAgentLaneRefusal(t, rec, http.StatusPreconditionFailed, "conflict")
			if refusal.Error.ETag != written.ETag || refusal.Error.ETag != agentLaneDataOf(t, fx.app, path).ETag {
				t.Fatalf("the refusal names %q, and the document is stamped %q", refusal.Error.ETag, written.ETag)
			}
			if got := rec.Header().Get("ETag"); got != written.ETag {
				t.Fatalf("the refusal's ETag header = %q, want %q", got, written.ETag)
			}
		}
		if got := agentLaneFileBytes(t, path); got != kept {
			t.Fatalf("the stale write changed the file: %q", got)
		}
	})

	t.Run("a write with no If-Match answers 428 precondition-required", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		rec := agentLaneWrite(t, fx.app, http.MethodPost, "/documents/"+agentlane.DocumentID(path)+"/data",
			agentLaneDataBody("data", agentDataRename, "op-1"))
		wantAgentLaneRefusal(t, rec, http.StatusPreconditionRequired, "precondition-required")
		if got := agentLaneFileBytes(t, path); got != before {
			t.Fatalf("the refused write changed the file: %q", got)
		}
	})

	t.Run("a removal without a proving read answers the exact 428 read-required body", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		read := agentLaneDataOf(t, fx.app, path)
		versions := agentLaneVersionsOf(t, fx.app, path)

		rec := unprovenDataWrite(t, fx.app, path, "data", agentDataRemoval, "op-1", read.ETag)
		want := `{"error":{"code":"read-required","message":"` + agentDataReadNote + `","etag":"` + read.ETag +
			`","wouldRemove":{"items":1,"values":1,"paths":["api.note"]}}}` + "\n"
		if rec.Code != http.StatusPreconditionRequired || rec.Body.String() != want {
			t.Fatalf("the unproven removal = %d %s, want 428 %s", rec.Code, rec.Body.String(), want)
		}
		if got := rec.Header().Get("ETag"); got != read.ETag {
			t.Fatalf("the refusal's ETag header = %q, want %q", got, read.ETag)
		}

		// The owner's own data route gives a program that sends no If-Match the
		// same sentence, etag and count.
		var own struct {
			Message     string          `json:"message"`
			Code        string          `json:"code"`
			Etag        string          `json:"etag"`
			WouldRemove json.RawMessage `json:"wouldRemove"`
		}
		route := ownDataRoute(t, fx.app, path, agentDataRemoval, false)
		if route.Code != http.StatusPreconditionRequired || json.Unmarshal(route.Body.Bytes(), &own) != nil || own.Code != "read-required" {
			t.Fatalf("the owner's own route = %d %s, want its 428 read-required", route.Code, route.Body.String())
		}
		passed := `{"error":{"code":"read-required","message":"` + own.Message + `","etag":"` + own.Etag + `","wouldRemove":` + string(own.WouldRemove) + `}}` + "\n"
		if passed != want {
			t.Fatalf("the lane answered %s, and the owner's own body passed through is %s", want, passed)
		}

		if got := agentLaneFileBytes(t, path); got != before {
			t.Fatalf("the refused removal changed the file: %q", got)
		}
		if after := agentLaneVersionsOf(t, fx.app, path); len(after.Versions) != len(versions.Versions) {
			t.Fatalf("the refused removal kept a version: %+v", after.Versions)
		}

		// The same removal with the etag the program read is a proven read.
		wantAgentLaneWrite(t, agentLaneDataWrite(t, fx.app, path, "data", agentDataRemoval, "op-2", read.ETag), "op-2")
		if got := agentLaneFileBytes(t, path); strings.Contains(got, "beta") || !strings.Contains(got, "alpha") {
			t.Fatalf("the proven removal left %q, want the list without beta", got)
		}
	})

	t.Run("a write that names no version still keeps the document it replaced", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		read := agentLaneDataOf(t, fx.app, path)
		written := wantAgentLaneWrite(t, unprovenDataWrite(t, fx.app, path, "data", agentDataRename, "op-1", read.ETag), "op-1")
		if got := agentLaneFileBytes(t, path); !strings.Contains(got, "<h1>Renamed</h1>") {
			t.Fatalf("the file holds %q, want the new title", got)
		}
		if got := agentLaneVersioned(t, fx.app, path, *written.UndoVersionID); got != before {
			t.Fatalf("undo version %s holds %q, want the document the write replaced", *written.UndoVersionID, got)
		}
	})

	t.Run("data the rules cannot hold answers 400 bad-data and writes nothing", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		read := agentLaneDataOf(t, fx.app, path)
		rec := agentLaneDataWrite(t, fx.app, path, "data", `{"author":"nobody"}`, "op-1", read.ETag)
		refusal := wantAgentLaneRefusal(t, rec, http.StatusBadRequest, "bad-data")
		if !strings.Contains(refusal.Error.Message, "unknown key") {
			t.Fatalf("the refusal's sentence = %q, want the engine's", refusal.Error.Message)
		}
		if got := agentLaneFileBytes(t, path); got != before {
			t.Fatalf("the refused write changed the file: %q", got)
		}
	})

	t.Run("a repeated operation id answers the first answer and writes once", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		read := agentLaneDataOf(t, fx.app, path)
		first := agentLaneDataWrite(t, fx.app, path, "data", agentDataRename, "k1", read.ETag)
		written := wantAgentLaneWrite(t, first, "k1")

		wantAgentLaneWrite(t, agentLaneDataWrite(t, fx.app, path, "data", `{"title":"Later"}`, "k2", written.ETag), "k2")
		later := agentLaneFileBytes(t, path)
		if !strings.Contains(later, "<h1>Later</h1>") {
			t.Fatalf("the later write left %q", later)
		}

		again := agentLaneDataWrite(t, fx.app, path, "data", agentDataRename, "k1", read.ETag)
		if again.Code != http.StatusOK || again.Body.String() != first.Body.String() {
			t.Fatalf("the repeated id = %d %q, want the first answer %q", again.Code, again.Body.String(), first.Body.String())
		}
		if got := agentLaneFileBytes(t, path); got != later {
			t.Fatalf("the repeated id changed the file: %q, want the later write %q", got, later)
		}
		other := agentLaneDataWrite(t, fx.app, path, "data", `{"title":"Other"}`, "k1", read.ETag)
		wantAgentLaneRefusal(t, other, http.StatusBadRequest, "operation-reused")
	})

	t.Run("a version store that cannot keep the old bytes answers 503", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		read := agentLaneDataOf(t, fx.app, path)
		failingVersionStores(t, fx)

		rec := agentLaneDataWrite(t, fx.app, path, "data", agentDataRename, "op-1", read.ETag)
		refusal := wantAgentLaneRefusal(t, rec, http.StatusServiceUnavailable, "recovery-unavailable")
		if refusal.Error.Message != "The current version could not be stored in history, so nothing was written. Try again." {
			t.Fatalf("the refusal's sentence = %q", refusal.Error.Message)
		}
		if got := agentLaneFileBytes(t, path); got != before {
			t.Fatalf("the refused write changed the file: %q", got)
		}
	})

	t.Run("a page's own data write is as it was", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)

		// A page is never asked to read first: its removal lands with no If-Match.
		removed := ownDataRoute(t, fx.app, path, agentDataRemoval, true)
		if removed.Code != http.StatusOK || removed.Body.String() != agentDataRemoved {
			t.Fatalf("a page's removal = %d %s, want 200 and the data read back", removed.Code, removed.Body.String())
		}
		if got := agentLaneFileBytes(t, path); strings.Contains(got, "beta") || strings.Contains(got, "first note") {
			t.Fatalf("a page's removal left %q", got)
		}
		if read := agentLaneDataOf(t, fx.app, path); removed.Header().Get("ETag") != read.ETag || string(read.Data) != agentDataRemoved {
			t.Fatalf("the page's write stamped %q, and the lane reads %s stamped %q", removed.Header().Get("ETag"), read.Data, read.ETag)
		}
	})

	t.Run("a page's own data write with a failing version store is as it was", func(t *testing.T) {
		fx := newAgentDataFixture(t)
		path := pick(fx)
		before := agentLaneFileBytes(t, path)
		failingVersionStores(t, fx)

		// A served page's write is refused with its own 409 and a loose page's
		// write still lands, as both did before 7.41b.
		renamed := ownDataRoute(t, fx.app, path, agentDataRename, true)
		if served {
			var body struct {
				Code string `json:"code"`
			}
			if renamed.Code != http.StatusConflict || json.Unmarshal(renamed.Body.Bytes(), &body) != nil || body.Code != "backup-failed" {
				t.Fatalf("a served page's write = %d %s, want 409 backup-failed", renamed.Code, renamed.Body.String())
			}
			if got := agentLaneFileBytes(t, path); got != before {
				t.Fatalf("the refused page write changed the file: %q", got)
			}
			return
		}
		if renamed.Code != http.StatusOK {
			t.Fatalf("a loose page's write = %d %s, want 200", renamed.Code, renamed.Body.String())
		}
		if got := agentLaneFileBytes(t, path); !strings.Contains(got, "<h1>Renamed</h1>") {
			t.Fatalf("a loose page's write left %q", got)
		}
	})
}
