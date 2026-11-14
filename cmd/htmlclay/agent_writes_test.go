package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/agentlane"
)

// agentLaneWriteAnswer is a write's success answer.
type agentLaneWriteAnswer struct {
	ETag          string  `json:"etag"`
	Changed       bool    `json:"changed"`
	OperationID   string  `json:"operationId"`
	UndoVersionID *string `json:"undoVersionId"`
}

// agentLaneWriteRefusal is a write refusal's envelope.
type agentLaneWriteRefusal struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		ETag    string `json:"etag"`
		SaveID  string `json:"saveId"`
	} `json:"error"`
}

// agentLaneWrite sends one write through the app's one handler on the lane's bare
// host, with a body and any number of name/value headers, added rather than set.
func agentLaneWrite(t *testing.T, a *app, method, target, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, "http://"+agentLaneHost+agentLanePrefix+target, reader)
	} else {
		req = httptest.NewRequest(method, "http://"+agentLaneHost+agentLanePrefix+target, nil)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Add(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	a.serveHost(rec, req)
	return rec
}

// agentLaneSaveBody is a save's JSON body.
func agentLaneSaveBody(t *testing.T, html, operationID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"html": html, "operationId": operationID})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// agentLaneSave sends one save of the document at path.
func agentLaneSave(t *testing.T, a *app, path, html, operationID, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	return agentLaneWrite(t, a, http.MethodPut, "/documents/"+agentlane.DocumentID(path),
		agentLaneSaveBody(t, html, operationID), "If-Match", ifMatch)
}

// agentLaneRestore sends one restore of versionID in the document at path.
func agentLaneRestore(t *testing.T, a *app, path, versionID, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	return agentLaneWrite(t, a, http.MethodPost,
		"/documents/"+agentlane.DocumentID(path)+"/versions/"+versionID+"/restore", "",
		"If-Match", ifMatch, "Save-ID", "r-"+versionID)
}

// wantAgentLaneWrite is one write that answered 200 with the expected stamp, id and
// undo version.
func wantAgentLaneWrite(t *testing.T, rec *httptest.ResponseRecorder, operationID string) agentLaneWriteAnswer {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s = %d %s, want 200", operationID, rec.Code, rec.Body.String())
	}
	var body agentLaneWriteAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%d answered %q, which is not JSON: %v", rec.Code, rec.Body.String(), err)
	}
	if body.OperationID != operationID || !body.Changed || body.ETag == "" || body.UndoVersionID == nil || *body.UndoVersionID == "" {
		t.Fatalf("the write = %+v, want operationId %q, changed, a stamp and an undo version", body, operationID)
	}
	if got := rec.Header().Get("ETag"); got != body.ETag {
		t.Fatalf("ETag header = %q, want %q", got, body.ETag)
	}
	return body
}

// wantAgentLaneRefusal is one write that answered status with code.
func wantAgentLaneRefusal(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) agentLaneWriteRefusal {
	t.Helper()
	var refusal agentLaneWriteRefusal
	if rec.Code != status || json.Unmarshal(rec.Body.Bytes(), &refusal) != nil || refusal.Error.Code != code {
		t.Fatalf("the write = %d %s, want %d %s", rec.Code, rec.Body.String(), status, code)
	}
	return refusal
}

// agentLaneVersioned asks the lane for one kept version's bytes.
func agentLaneVersioned(t *testing.T, a *app, path, versionID string) string {
	t.Helper()
	rec := agentLaneRequest(t, a, http.MethodGet, "/documents/"+agentlane.DocumentID(path)+"/versions/"+versionID, nil)
	var one struct {
		HTML string `json:"html"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &one) != nil {
		t.Fatalf("GET version %s = %d %s", versionID, rec.Code, rec.Body.String())
	}
	return one.HTML
}

// agentLaneVersionsOf asks the lane for a document's kept versions.
func agentLaneVersionsOf(t *testing.T, a *app, path string) agentLaneVersions {
	t.Helper()
	rec := agentLaneRequest(t, a, http.MethodGet, "/documents/"+agentlane.DocumentID(path)+"/versions", nil)
	var page agentLaneVersions
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("GET versions = %d %s", rec.Code, rec.Body.String())
	}
	return page
}

// agentLaneHasVersion reports whether a page names one version.
func agentLaneHasVersion(page agentLaneVersions, id string) bool {
	for _, version := range page.Versions {
		if version.ID == id {
			return true
		}
	}
	return false
}

// TestM3AgentLane is step 7.41a through the real app: a save writes the file and
// keeps the bytes it replaced, an old stamp is refused, one operation id writes
// once, and a restore puts a kept version back.
func TestM3AgentLane(t *testing.T) {
	t.Run("served", func(t *testing.T) {
		testAgentLaneWrites(t, func(fx *agentLaneFixture) string { return fx.served })
	})
	t.Run("loose", func(t *testing.T) {
		testAgentLaneWrites(t, func(fx *agentLaneFixture) string { return fx.open })
	})
}

func testAgentLaneWrites(t *testing.T, pick func(*agentLaneFixture) string) {
	t.Helper()

	t.Run("a save writes the document and keeps the bytes it replaced", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		incoming := strings.Replace(doc.HTML, "</body>", "<p>saved</p></body>", 1)
		saved := wantAgentLaneWrite(t, agentLaneSave(t, fx.app, path, incoming, "op-1", doc.ETag), "op-1")
		if got := agentLaneFileBytes(t, path); got != incoming {
			t.Fatalf("the file holds %q, want the saved bytes", got)
		}
		if saved.ETag == doc.ETag {
			t.Fatal("the save kept the stamp it was given")
		}
		if !agentLaneHasVersion(agentLaneVersionsOf(t, fx.app, path), *saved.UndoVersionID) {
			t.Fatalf("the versions list does not name the undo version %s", *saved.UndoVersionID)
		}
		if got := agentLaneVersioned(t, fx.app, path, *saved.UndoVersionID); got != doc.HTML {
			t.Fatalf("undo version %s holds %q, want the bytes the save replaced", *saved.UndoVersionID, got)
		}
	})

	t.Run("a stale stamp is refused and the file is unchanged", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		saved := wantAgentLaneWrite(t, agentLaneSave(t, fx.app, path,
			strings.Replace(doc.HTML, "</body>", "<p>first save</p></body>", 1), "op-1", doc.ETag), "op-1")
		kept := agentLaneFileBytes(t, path)

		late := agentLaneSave(t, fx.app, path, strings.Replace(doc.HTML, "</body>", "<p>late</p></body>", 1), "op-2", doc.ETag)
		refusal := wantAgentLaneRefusal(t, late, http.StatusPreconditionFailed, "conflict")
		current, _ := agentLaneGet(t, fx.app, path)
		if refusal.Error.ETag != current.ETag || refusal.Error.ETag != saved.ETag {
			t.Fatalf("the refusal names %q, and the document is stamped %q", refusal.Error.ETag, current.ETag)
		}
		if got := agentLaneFileBytes(t, path); got != kept {
			t.Fatalf("the stale save changed the file: %q", got)
		}
	})

	t.Run("a save without If-Match answers 428", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		rec := agentLaneWrite(t, fx.app, http.MethodPut, "/documents/"+agentlane.DocumentID(path),
			agentLaneSaveBody(t, strings.Replace(doc.HTML, "</body>", "<p>no stamp</p></body>", 1), "op-1"))
		wantAgentLaneRefusal(t, rec, http.StatusPreconditionRequired, "precondition-required")
		if got := agentLaneFileBytes(t, path); got != doc.HTML {
			t.Fatalf("the refused save changed the file: %q", got)
		}
	})

	t.Run("an If-Match that is not one strong etag answers 400", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		body := agentLaneSaveBody(t, strings.Replace(doc.HTML, "</body>", "<p>weak</p></body>", 1), "op-1")
		target := "/documents/" + agentlane.DocumentID(path)
		for _, tag := range []string{`W/"x"`, "*", `"a", "b"`} {
			rec := agentLaneWrite(t, fx.app, http.MethodPut, target, body, "If-Match", tag)
			wantAgentLaneRefusal(t, rec, http.StatusBadRequest, "invalid-match")
		}
		if got := agentLaneFileBytes(t, path); got != doc.HTML {
			t.Fatalf("a refused save changed the file: %q", got)
		}
	})

	t.Run("a repeated operation id answers the first answer and writes once", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		incoming := strings.Replace(doc.HTML, "</body>", "<p>once</p></body>", 1)
		first := agentLaneSave(t, fx.app, path, incoming, "k1", doc.ETag)
		saved := wantAgentLaneWrite(t, first, "k1")

		later := strings.Replace(incoming, "</body>", "<p>later</p></body>", 1)
		if code := saveWithStamp(t, fx.app, path, later, saved.ETag); code != http.StatusOK {
			t.Fatalf("the later save = %d, want 200", code)
		}
		if got := agentLaneFileBytes(t, path); got != later {
			t.Fatalf("the later save left %q", got)
		}

		again := agentLaneSave(t, fx.app, path, incoming, "k1", doc.ETag)
		if again.Code != http.StatusOK || again.Body.String() != first.Body.String() {
			t.Fatalf("the repeated key = %d %q, want the first answer", again.Code, again.Body.String())
		}
		if got := agentLaneFileBytes(t, path); got != later {
			t.Fatalf("the repeated key changed the file: %q, want the later change %q", got, later)
		}
	})

	t.Run("the same operation id with other html answers 400", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		wantAgentLaneWrite(t, agentLaneSave(t, fx.app, path,
			strings.Replace(doc.HTML, "</body>", "<p>one</p></body>", 1), "k1", doc.ETag), "k1")
		rec := agentLaneSave(t, fx.app, path, strings.Replace(doc.HTML, "</body>", "<p>two</p></body>", 1), "k1", doc.ETag)
		wantAgentLaneRefusal(t, rec, http.StatusBadRequest, "operation-reused")
	})

	t.Run("a version store that cannot keep the old bytes answers 503", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root writes into a read-only history folder")
		}
		if runtime.GOOS == "windows" {
			t.Skip("Windows ignores a folder's read-only mode")
		}
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		readOnly := func(dir string) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		}
		readOnly(servedHistoryDir(filepath.Dir(fx.served), "index.html", "versions"))
		readOnly(fx.app.rt.versions.BaseDir())

		doc, _ := agentLaneGet(t, fx.app, path)
		rec := agentLaneSave(t, fx.app, path, strings.Replace(doc.HTML, "</body>", "<p>never written</p></body>", 1), "op-1", doc.ETag)
		refusal := wantAgentLaneRefusal(t, rec, http.StatusServiceUnavailable, "recovery-unavailable")
		if refusal.Error.Message != "The current version could not be stored in history, so nothing was written. Try again." {
			t.Fatalf("the refusal's sentence = %q", refusal.Error.Message)
		}
		if got := agentLaneFileBytes(t, path); got != doc.HTML {
			t.Fatalf("the refused save changed the file: %q", got)
		}
	})

	t.Run("a restore puts a kept version back", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		path := pick(fx)
		doc, _ := agentLaneGet(t, fx.app, path)
		saved := wantAgentLaneWrite(t, agentLaneSave(t, fx.app, path,
			strings.Replace(doc.HTML, "</body>", "<p>to be undone</p></body>", 1), "op-1", doc.ETag), "op-1")
		kept := agentLaneFileBytes(t, path)

		stale := agentLaneRestore(t, fx.app, path, *saved.UndoVersionID, doc.ETag)
		wantAgentLaneRefusal(t, stale, http.StatusPreconditionFailed, "conflict")
		if got := agentLaneFileBytes(t, path); got != kept {
			t.Fatalf("the stale restore changed the file: %q", got)
		}

		restored := agentLaneWrite(t, fx.app, http.MethodPost,
			"/documents/"+agentlane.DocumentID(path)+"/versions/"+*saved.UndoVersionID+"/restore", "",
			"If-Match", saved.ETag, "Save-ID", "r-1")
		answer := wantAgentLaneWrite(t, restored, "r-1")
		if got := agentLaneFileBytes(t, path); got != doc.HTML {
			t.Fatalf("the restore left %q, want the version's bytes %q", got, doc.HTML)
		}
		if !agentLaneHasVersion(agentLaneVersionsOf(t, fx.app, path), *answer.UndoVersionID) {
			t.Fatalf("the versions list does not name the restore's undo version %s", *answer.UndoVersionID)
		}
	})
}
