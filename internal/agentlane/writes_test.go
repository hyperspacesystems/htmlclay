package agentlane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// oneWrite is one call the lane made on a scripted access.
type oneWrite struct {
	html      string
	versionID string
	ifMatch   string
	saveID    string
}

// scriptedAccess is one document's writes, scripted by the test: every call is
// recorded and answers the result or the error the test set.
type scriptedAccess struct {
	mu       sync.Mutex
	saves    []oneWrite
	restores []oneWrite
	result   WriteResult
	err      error
}

func (s *scriptedAccess) Read(ctx context.Context) ([]byte, string, error) {
	return []byte(agentServedHTML), `"e1"`, nil
}

func (s *scriptedAccess) Versions(ctx context.Context, before string, limit int) ([]Version, string, error) {
	return nil, "", nil
}

func (s *scriptedAccess) Version(ctx context.Context, id string) ([]byte, error) {
	return []byte(agentVersionHTML), nil
}

func (s *scriptedAccess) Save(ctx context.Context, html []byte, ifMatch, saveID string) (WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves = append(s.saves, oneWrite{html: string(html), ifMatch: ifMatch, saveID: saveID})
	return s.result, s.err
}

func (s *scriptedAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restores = append(s.restores, oneWrite{versionID: versionID, ifMatch: ifMatch, saveID: saveID})
	return s.result, s.err
}

// writesSoFar is every write the lane made, saves first.
func (s *scriptedAccess) writesSoFar() []oneWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]oneWrite, 0, len(s.saves)+len(s.restores))
	out = append(out, s.saves...)
	return append(out, s.restores...)
}

// writeHost is an app whose one document is the scripted access, or whose access
// is refused before it is reached.
type writeHost struct {
	access    *scriptedAccess
	accessErr error
}

func (h writeHost) Version() string { return "9.9.9" }

func (h writeHost) List(ctx context.Context, limit int) ([]Document, bool, error) {
	return nil, false, nil
}

func (h writeHost) Access(ctx context.Context, path string) (Access, Document, error) {
	if h.accessErr != nil {
		return nil, Document{}, h.accessErr
	}
	return h.access, Document{Path: path}, nil
}

// writeAnswer is a write's success answer.
type writeAnswer struct {
	ETag          string  `json:"etag"`
	Changed       bool    `json:"changed"`
	OperationID   string  `json:"operationId"`
	UndoVersionID *string `json:"undoVersionId"`
}

// writeRequest drives the lane's handler directly with a body and any number of
// name/value headers, added rather than set.
func writeRequest(t *testing.T, host http.Handler, method, target, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "http://"+agentTestHost+target, nil)
	} else {
		req = httptest.NewRequest(method, "http://"+agentTestHost+target, strings.NewReader(body))
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Add(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	host.ServeHTTP(rec, req)
	return rec
}

// writeSaved drives one save of the served document.
func writeSaved(t *testing.T, host http.Handler, tag, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	return writeRequest(t, host, http.MethodPut, "/_/agent/v1/documents/"+DocumentID(agentServedPath), body,
		append([]string{"If-Match", tag}, headers...)...)
}

func wantWriteAnswer(t *testing.T, rec *httptest.ResponseRecorder, etag, operationID string, undo string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("write = %d %s, want 200", rec.Code, rec.Body.String())
	}
	var body writeAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%d answered %q, which is not JSON: %v", rec.Code, rec.Body.String(), err)
	}
	if body.ETag != etag || body.OperationID != operationID {
		t.Fatalf("write = %+v, want etag %q and operationId %q", body, etag, operationID)
	}
	if undo == "" {
		if body.UndoVersionID != nil {
			t.Fatalf("undoVersionId = %v, want null", *body.UndoVersionID)
		}
	} else if body.UndoVersionID == nil || *body.UndoVersionID != undo {
		t.Fatalf("undoVersionId = %v, want %q", body.UndoVersionID, undo)
	}
	if got := rec.Header().Get("ETag"); got != etag {
		t.Fatalf("ETag header = %q, want %q", got, etag)
	}
}

// TestM3AgentWrites is step 7.41a: the lane's save and restore routes over a
// scripted owner, with one strong If-Match per write and one answer per
// operation id.
func TestM3AgentWrites(t *testing.T) {
	goodSave := `{"html":"<!doctype html><html></html>","operationId":"op-1"}`
	otherSave := `{"html":"<!doctype html><html><body>other</body></html>","operationId":"op-2"}`

	t.Run("a save answers the new stamp and the version it replaced", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		rec := writeSaved(t, handler, `"e1"`, goodSave)
		wantWriteAnswer(t, rec, "e2", "op-1", "v7")
		writes := access.writesSoFar()
		if len(writes) != 1 {
			t.Fatalf("the owner got %d writes, want 1", len(writes))
		}
		if writes[0].ifMatch != `"e1"` || writes[0].saveID != "op-1" || writes[0].html != "<!doctype html><html></html>" {
			t.Fatalf("the owner got %+v, want If-Match %q, saveID op-1 and the body's html", writes[0], `"e1"`)
		}
	})

	t.Run("a save without If-Match answers 428 and reaches no owner", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		rec := writeRequest(t, handler, http.MethodPut, "/_/agent/v1/documents/"+DocumentID(agentServedPath), goodSave)
		if rec.Code != http.StatusPreconditionRequired || agentRefusal(t, rec) != "precondition-required" {
			t.Fatalf("a save without If-Match = %d %s, want 428 precondition-required", rec.Code, rec.Body.String())
		}
		if writes := access.writesSoFar(); len(writes) != 0 {
			t.Fatalf("the owner got %d writes without an If-Match, want none", len(writes))
		}
	})

	t.Run("an If-Match that is not one strong etag answers 400", func(t *testing.T) {
		for name, headers := range map[string][]string{
			"star":   {"If-Match", "*"},
			"weak":   {"If-Match", `W/"e1"`},
			"a list": {"If-Match", `"a", "b"`},
			"two":    {"If-Match", `"e1"`, "If-Match", `"e2"`},
		} {
			access := &scriptedAccess{result: WriteResult{ETag: "e2"}}
			handler := Handler(writeHost{access: access}, agentTestPort)
			rec := writeRequest(t, handler, http.MethodPut, "/_/agent/v1/documents/"+DocumentID(agentServedPath), goodSave, headers...)
			if rec.Code != http.StatusBadRequest || agentRefusal(t, rec) != "invalid-match" {
				t.Fatalf("an If-Match that is %s = %d %s, want 400 invalid-match", name, rec.Code, rec.Body.String())
			}
			if writes := access.writesSoFar(); len(writes) != 0 {
				t.Fatalf("an If-Match that is %s reached the owner %d times, want none", name, len(writes))
			}
		}
	})

	t.Run("a stale stamp answers 412 with the document's stamp and the save that wrote it", func(t *testing.T) {
		access := &scriptedAccess{err: &StaleError{ETag: "e9", SaveID: "op-0"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		rec := writeSaved(t, handler, `"e1"`, goodSave)
		if rec.Code != http.StatusPreconditionFailed {
			t.Fatalf("a stale save = %d %s, want 412", rec.Code, rec.Body.String())
		}
		body := decodeAgentBody[envelope](t, rec)
		if body.Error.Code != "conflict" || body.Error.ETag != "e9" || body.Error.SaveID != "op-0" {
			t.Fatalf("a stale save = %+v, want conflict with etag e9 and saveId op-0", body.Error)
		}
		if got := rec.Header().Get("ETag"); got != "e9" {
			t.Fatalf("ETag header = %q, want e9", got)
		}
	})

	t.Run("a repeated operation id answers the first answer and writes once", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		first := writeSaved(t, handler, `"e1"`, goodSave)
		again := writeSaved(t, handler, `"e1"`, goodSave)
		if first.Code != http.StatusOK || again.Code != http.StatusOK {
			t.Fatalf("the two answers = %d and %d, want 200", first.Code, again.Code)
		}
		if first.Body.String() != again.Body.String() {
			t.Fatalf("the repeat answered %q, want the first answer %q", again.Body.String(), first.Body.String())
		}
		if writes := access.writesSoFar(); len(writes) != 1 {
			t.Fatalf("the repeat wrote %d times in total, want 1", len(writes))
		}
	})

	t.Run("a repeat after another write still answers the first answer", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		first := writeSaved(t, handler, `"e1"`, goodSave)
		second := writeSaved(t, handler, `"e1"`, otherSave)
		if second.Code != http.StatusOK {
			t.Fatalf("the other write = %d %s", second.Code, second.Body.String())
		}
		third := writeSaved(t, handler, `"e1"`, goodSave)
		if third.Body.String() != first.Body.String() || third.Code != first.Code {
			t.Fatalf("the late repeat = %d %q, want the first answer %d %q", third.Code, third.Body.String(), first.Code, first.Body.String())
		}
		if writes := access.writesSoFar(); len(writes) != 2 {
			t.Fatalf("the owner got %d writes, want 2", len(writes))
		}
	})

	t.Run("the same operation id with another body answers 400 and writes once", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		writeSaved(t, handler, `"e1"`, goodSave)
		reused := writeSaved(t, handler, `"e1"`, `{"html":"<!doctype html><html><body>changed</body></html>","operationId":"op-1"}`)
		if reused.Code != http.StatusBadRequest || agentRefusal(t, reused) != "operation-reused" {
			t.Fatalf("the same operation id with another body = %d %s, want 400 operation-reused", reused.Code, reused.Body.String())
		}
		if writes := access.writesSoFar(); len(writes) != 1 {
			t.Fatalf("the refused reuse left %d writes, want 1", len(writes))
		}
	})

	t.Run("a Save-ID header names the operation id", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		rec := writeSaved(t, handler, `"e1"`, `{"html":"<!doctype html><html></html>"}`, "Save-ID", "h-1")
		wantWriteAnswer(t, rec, "e2", "h-1", "v7")
		if writes := access.writesSoFar(); len(writes) != 1 || writes[0].saveID != "h-1" {
			t.Fatalf("the owner got %+v, want one write named h-1", writes)
		}
	})

	t.Run("a body that names no operation id is refused", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		for name, c := range map[string]struct {
			body    string
			headers []string
		}{
			"a header and a body naming different ids": {goodSave, []string{"Save-ID", "other-1"}},
			"no key at all": {`{"html":"<!doctype html><html></html>"}`, nil},
		} {
			rec := writeSaved(t, handler, `"e1"`, c.body, c.headers...)
			if rec.Code != http.StatusBadRequest || agentRefusal(t, rec) != "bad-request" {
				t.Fatalf("a write with %s = %d %s, want 400 bad-request", name, rec.Code, rec.Body.String())
			}
		}
		for name, body := range map[string]string{
			"a body without html":     `{"operationId":"op-1"}`,
			"a body that is not JSON": "not json",
		} {
			rec := writeSaved(t, handler, `"e1"`, body)
			if rec.Code != http.StatusBadRequest || agentRefusal(t, rec) != "bad-request" {
				t.Fatalf("a save with %s = %d %s, want 400 bad-request", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("a version store that fails answers 503 and the retry runs again", func(t *testing.T) {
		access := &scriptedAccess{err: ErrBackup}
		handler := Handler(writeHost{access: access}, agentTestPort)
		for i := 0; i < 2; i++ {
			rec := writeSaved(t, handler, `"e1"`, goodSave)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("a save with no version store = %d %s, want 503", rec.Code, rec.Body.String())
			}
			body := decodeAgentBody[envelope](t, rec)
			if body.Error.Code != "recovery-unavailable" || body.Error.Message != "The current version could not be stored in history, so nothing was written. Try again." {
				t.Fatalf("a save with no version store = %+v, want recovery-unavailable with the contract sentence", body.Error)
			}
		}
		if writes := access.writesSoFar(); len(writes) != 2 {
			t.Fatalf("the owner got %d writes, want 2, because a 503 is not replayed", len(writes))
		}
	})

	t.Run("each write refusal is named by its own code", func(t *testing.T) {
		for name, want := range map[string]struct {
			status int
			code   string
		}{
			"an oversized document": {http.StatusRequestEntityTooLarge, "too-large"},
			"a fragment":            {http.StatusUnprocessableEntity, "invalid-document"},
			"a not found document":  {http.StatusNotFound, "not-found"},
		} {
			access := &scriptedAccess{err: errFor(name)}
			handler := Handler(writeHost{access: access}, agentTestPort)
			rec := writeSaved(t, handler, `"e1"`, goodSave)
			if rec.Code != want.status || agentRefusal(t, rec) != want.code {
				t.Fatalf("a save of %s = %d %s, want %d %s", name, rec.Code, rec.Body.String(), want.status, want.code)
			}
		}
	})

	t.Run("a document nothing reaches answers not-reachable", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2"}}
		handler := Handler(writeHost{access: access, accessErr: ErrNotReachable}, agentTestPort)
		rec := writeSaved(t, handler, `"e1"`, goodSave)
		if rec.Code != http.StatusNotFound || agentRefusal(t, rec) != "not-reachable" {
			t.Fatalf("a save of a document nothing reaches = %d %s, want 404 not-reachable", rec.Code, rec.Body.String())
		}
	})

	t.Run("any other failure answers 500 and the retry runs again", func(t *testing.T) {
		access := &scriptedAccess{err: errors.New("boom")}
		handler := Handler(writeHost{access: access}, agentTestPort)
		for i := 0; i < 2; i++ {
			rec := writeSaved(t, handler, `"e1"`, goodSave)
			if rec.Code != http.StatusInternalServerError || agentRefusal(t, rec) != "internal" {
				t.Fatalf("a save that failed = %d %s, want 500 internal", rec.Code, rec.Body.String())
			}
		}
		if writes := access.writesSoFar(); len(writes) != 2 {
			t.Fatalf("the owner got %d writes, want 2, because a 500 is not replayed", len(writes))
		}
	})

	t.Run("a restore answers the new stamp and replays its operation id", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v8"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		target := "/_/agent/v1/documents/" + DocumentID(agentServedPath) + "/versions/v3/restore"
		first := writeRequest(t, handler, http.MethodPost, target, "", "If-Match", `"e1"`, "Save-ID", "r-1")
		wantWriteAnswer(t, first, "e2", "r-1", "v8")
		writes := access.writesSoFar()
		if len(writes) != 1 || writes[0].versionID != "v3" || writes[0].ifMatch != `"e1"` || writes[0].saveID != "r-1" {
			t.Fatalf("the owner got %+v, want a restore of v3 with If-Match %q and Save-ID r-1", writes, `"e1"`)
		}
		again := writeRequest(t, handler, http.MethodPost, target, "", "If-Match", `"e1"`, "Save-ID", "r-1")
		if again.Body.String() != first.Body.String() {
			t.Fatalf("the repeat answered %q, want the first answer %q", again.Body.String(), first.Body.String())
		}
		if writes := access.writesSoFar(); len(writes) != 1 {
			t.Fatalf("the repeat restored %d times in total, want 1", len(writes))
		}
		reused := writeRequest(t, handler, http.MethodPost, target, "", "If-Match", `"e0"`, "Save-ID", "r-1")
		if reused.Code != http.StatusBadRequest || agentRefusal(t, reused) != "operation-reused" {
			t.Fatalf("the same key with another If-Match = %d %s, want 400 operation-reused", reused.Code, reused.Body.String())
		}
	})

	t.Run("a restore without If-Match answers 428", func(t *testing.T) {
		access := &scriptedAccess{result: WriteResult{ETag: "e2"}}
		handler := Handler(writeHost{access: access}, agentTestPort)
		target := "/_/agent/v1/documents/" + DocumentID(agentServedPath) + "/versions/v3/restore"
		rec := writeRequest(t, handler, http.MethodPost, target, "", "Save-ID", "r-1")
		if rec.Code != http.StatusPreconditionRequired || agentRefusal(t, rec) != "precondition-required" {
			t.Fatalf("a restore without If-Match = %d %s, want 428 precondition-required", rec.Code, rec.Body.String())
		}
		if writes := access.writesSoFar(); len(writes) != 0 {
			t.Fatalf("the owner got %d restores without an If-Match, want none", len(writes))
		}
	})
}

// errFor is the owner error each named refusal stands for.
func errFor(name string) error {
	switch name {
	case "an oversized document":
		return ErrTooLarge
	case "a fragment":
		return ErrNotDocument
	}
	return ErrNotFound
}
