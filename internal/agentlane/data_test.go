package agentlane

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func (f fakeAccess) ReadData(ctx context.Context) ([]byte, string, error) {
	return nil, "", ErrNoAPIRules
}

func (f fakeAccess) WriteData(ctx context.Context, write DataWrite) (WriteResult, error) {
	return WriteResult{}, ErrNotReachable
}

func (s *scriptedAccess) ReadData(ctx context.Context) ([]byte, string, error) {
	return nil, "", ErrNoAPIRules
}

func (s *scriptedAccess) WriteData(ctx context.Context, write DataWrite) (WriteResult, error) {
	return WriteResult{}, ErrNotReachable
}

// dataAccess is one document's data, scripted by the test: a read answers the
// data or the error the test set, and every write is recorded and answers the
// result or the error the test set.
type dataAccess struct {
	fakeAccess
	mu      sync.Mutex
	data    string
	etag    string
	readErr error
	writes  []DataWrite
	result  WriteResult
	err     error
}

func (d *dataAccess) ReadData(ctx context.Context) ([]byte, string, error) {
	if d.readErr != nil {
		return nil, "", d.readErr
	}
	return []byte(d.data), d.etag, nil
}

func (d *dataAccess) WriteData(ctx context.Context, write DataWrite) (WriteResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = append(d.writes, write)
	return d.result, d.err
}

func (d *dataAccess) writesSoFar() []DataWrite {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DataWrite{}, d.writes...)
}

// dataHost is an app whose one document is the scripted data access, or whose
// access is refused before it is reached.
type dataHost struct {
	access    *dataAccess
	accessErr error
}

func (h dataHost) Version() string { return "9.9.9" }

func (h dataHost) List(ctx context.Context, limit int) ([]Document, bool, error) {
	return nil, false, nil
}

func (h dataHost) Access(ctx context.Context, path string) (Access, Document, error) {
	if h.accessErr != nil {
		return nil, Document{}, h.accessErr
	}
	return h.access, Document{Path: path}, nil
}

var agentDataTarget = "/_/agent/v1/documents/" + DocumentID(agentServedPath) + "/data"

// dataPosted drives one data write of the served document.
func dataPosted(t *testing.T, host http.Handler, tag, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	return writeRequest(t, host, http.MethodPost, agentDataTarget, body, append([]string{"If-Match", tag}, headers...)...)
}

// wantDataRefusal checks a refusal's status, its code and that the owner saw the
// number of writes the test expects.
func wantDataRefusal(t *testing.T, rec *httptest.ResponseRecorder, access *dataAccess, status int, code string, writes int) {
	t.Helper()
	if rec.Code != status || agentRefusal(t, rec) != code {
		t.Fatalf("answer = %d %s, want %d %s", rec.Code, rec.Body.String(), status, code)
	}
	if got := len(access.writesSoFar()); got != writes {
		t.Fatalf("the owner got %d writes, want %d", got, writes)
	}
}

// TestM3AgentData is step 7.41b: the lane's data routes over a scripted owner,
// with the door a save has, one answer per operation id, and every refusal a
// data read or write answers.
func TestM3AgentData(t *testing.T) {
	goodData := `{"data":{"title":"B"},"operationId":"op-1"}`
	goodPatch := `{"patch":[{"sheet":"Budget","delete":true}],"operationId":"op-2"}`

	t.Run("a read answers the data and the stamp its write compares", func(t *testing.T) {
		access := &dataAccess{data: `{"title":"A","items":[{"name":"<one>"}]}`, etag: "e1"}
		rec := agentCall(t, Handler(dataHost{access: access}, agentTestPort), http.MethodGet, agentDataTarget, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("read = %d %s", rec.Code, rec.Body.String())
		}
		body := decodeAgentBody[struct {
			Data struct {
				Title string `json:"title"`
				Items []struct {
					Name string `json:"name"`
				} `json:"items"`
			} `json:"data"`
			ETag string `json:"etag"`
		}](t, rec)
		if body.Data.Title != "A" || len(body.Data.Items) != 1 || body.Data.Items[0].Name != "<one>" || body.ETag != "e1" {
			t.Fatalf("read = %s", rec.Body.String())
		}
		if got := rec.Header().Get("ETag"); got != "e1" {
			t.Fatalf("ETag header = %q, want e1", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", got)
		}
	})

	t.Run("a read is refused with the code its error names", func(t *testing.T) {
		cases := []struct {
			name    string
			host    dataHost
			status  int
			code    string
			message string
		}{
			{"no api rules", dataHost{access: &dataAccess{readErr: ErrNoAPIRules}}, 400, "no-api-rules", textNoAPIRules},
			{"rules the engine cannot use", dataHost{access: &dataAccess{readErr: &DataError{Status: 400, Rules: true, Message: "The api rules tag body is not valid JSON."}}}, 400, "bad-api-rules", "The api rules tag body is not valid JSON."},
			{"a sheet the engine refuses", dataHost{access: &dataAccess{readErr: &DataError{Status: 409, Code: "sheet-not-canonical", Message: "Open the sheet in the editor and save once."}}}, 409, "sheet-not-canonical", "Open the sheet in the editor and save once."},
			{"a document that is gone", dataHost{access: &dataAccess{readErr: ErrNotFound}}, 404, "not-found", textNotFound},
			{"a document no owner reaches", dataHost{accessErr: ErrNotReachable}, 404, "not-reachable", textNotReachable},
			{"an error with no name", dataHost{access: &dataAccess{readErr: errors.New("disk")}}, 500, "internal", textInternal},
		}
		for _, c := range cases {
			rec := agentCall(t, Handler(c.host, agentTestPort), http.MethodGet, agentDataTarget, nil)
			body := decodeAgentBody[envelope](t, rec)
			if rec.Code != c.status || body.Error.Code != c.code || body.Error.Message != c.message {
				t.Fatalf("%s = %d %s, want %d %s %q", c.name, rec.Code, rec.Body.String(), c.status, c.code, c.message)
			}
		}
	})

	t.Run("a data write hands the owner the data and the etag, and answers four fields", func(t *testing.T) {
		access := &dataAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7", Stale: true}}
		rec := dataPosted(t, Handler(dataHost{access: access}, agentTestPort), `"e1"`, goodData)
		wantWriteAnswer(t, rec, "e2", "op-1", "v7")
		if got := rec.Body.String(); got != `{"changed":true,"etag":"e2","operationId":"op-1","undoVersionId":"v7"}`+"\n" {
			t.Fatalf("a data write answered %s, want the four fields and no stale", got)
		}
		writes := access.writesSoFar()
		if len(writes) != 1 {
			t.Fatalf("the owner got %d writes, want 1", len(writes))
		}
		if string(writes[0].Data) != `{"title":"B"}` || writes[0].Patch != nil || writes[0].IfMatch != `"e1"` {
			t.Fatalf("the owner got %+v, want the data as sent, no patch and the etag", writes[0])
		}
	})

	t.Run("a write that changed nothing answers a null undo version", func(t *testing.T) {
		access := &dataAccess{result: WriteResult{ETag: "e1"}}
		rec := dataPosted(t, Handler(dataHost{access: access}, agentTestPort), `"e1"`, goodData)
		wantWriteAnswer(t, rec, "e1", "op-1", "")
		if decodeAgentBody[writeAnswer](t, rec).Changed {
			t.Fatalf("changed = true for a write that moved nothing: %s", rec.Body.String())
		}
	})

	t.Run("a patch hands the owner its operations and answers stale", func(t *testing.T) {
		access := &dataAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7", Stale: true}}
		rec := dataPosted(t, Handler(dataHost{access: access}, agentTestPort), `"e1"`, goodPatch)
		wantWriteAnswer(t, rec, "e2", "op-2", "v7")
		if got := rec.Body.String(); got != `{"changed":true,"etag":"e2","operationId":"op-2","stale":true,"undoVersionId":"v7"}`+"\n" {
			t.Fatalf("a patch answered %s, want the four fields and stale", got)
		}
		writes := access.writesSoFar()
		if len(writes) != 1 || string(writes[0].Patch) != `[{"sheet":"Budget","delete":true}]` || writes[0].Data != nil || writes[0].IfMatch != `"e1"` {
			t.Fatalf("the owner got %+v, want the operations as sent and no data", writes)
		}
	})

	t.Run("a Save-ID header names the operation, and must agree with the body", func(t *testing.T) {
		access := &dataAccess{result: WriteResult{ETag: "e2", Changed: true}}
		handler := Handler(dataHost{access: access}, agentTestPort)
		rec := dataPosted(t, handler, `"e1"`, `{"data":{"title":"B"}}`, "Save-ID", "op-header")
		wantWriteAnswer(t, rec, "e2", "op-header", "")
		rec = dataPosted(t, handler, `"e1"`, `{"data":{"title":"B"},"operationId":"op-same"}`, "Save-ID", "op-same")
		wantWriteAnswer(t, rec, "e2", "op-same", "")
		rec = dataPosted(t, handler, `"e1"`, goodData, "Save-ID", "op-other")
		wantDataRefusal(t, rec, access, http.StatusBadRequest, "bad-request", 2)
	})

	t.Run("a write with no If-Match answers 428 and reaches no owner", func(t *testing.T) {
		access := &dataAccess{}
		rec := writeRequest(t, Handler(dataHost{access: access}, agentTestPort), http.MethodPost, agentDataTarget, goodData)
		wantDataRefusal(t, rec, access, http.StatusPreconditionRequired, "precondition-required", 0)
	})

	t.Run("an If-Match that is not one strong etag answers 400 and reaches no owner", func(t *testing.T) {
		access := &dataAccess{}
		handler := Handler(dataHost{access: access}, agentTestPort)
		for _, tag := range []string{"*", `W/"e1"`, `"e1", "e2"`, ""} {
			wantDataRefusal(t, dataPosted(t, handler, tag, goodData), access, http.StatusBadRequest, "invalid-match", 0)
		}
		rec := writeRequest(t, handler, http.MethodPost, agentDataTarget, goodData, "If-Match", `"e1"`, "If-Match", `"e2"`)
		wantDataRefusal(t, rec, access, http.StatusBadRequest, "invalid-match", 0)
	})

	t.Run("a body that does not name one of data and patch answers 400 and reaches no owner", func(t *testing.T) {
		access := &dataAccess{}
		handler := Handler(dataHost{access: access}, agentTestPort)
		bodies := map[string]string{
			"both":            `{"data":{"title":"B"},"patch":[{"sheet":"Budget","delete":true}],"operationId":"op-1"}`,
			"neither":         `{"operationId":"op-1"}`,
			"a null data":     `{"data":null,"operationId":"op-1"}`,
			"a null patch":    `{"patch":null,"operationId":"op-1"}`,
			"html":            `{"html":"<!doctype html><html></html>","operationId":"op-1"}`,
			"no operation id": `{"data":{"title":"B"}}`,
			"not JSON":        `{"data":`,
			"nothing":         ``,
		}
		for name, body := range bodies {
			rec := dataPosted(t, handler, `"e1"`, body)
			if rec.Code != http.StatusBadRequest || agentRefusal(t, rec) != "bad-request" {
				t.Fatalf("%s = %d %s, want 400 bad-request", name, rec.Code, rec.Body.String())
			}
			if message := decodeAgentBody[envelope](t, rec).Error.Message; message != textBadDataBody {
				t.Fatalf("%s said %q, want the data body's sentence", name, message)
			}
		}
		if got := len(access.writesSoFar()); got != 0 {
			t.Fatalf("the owner got %d writes, want 0", got)
		}
	})

	t.Run("a stale etag answers 412 with the current etag", func(t *testing.T) {
		access := &dataAccess{err: &StaleError{ETag: "e9", SaveID: "op-earlier"}}
		rec := dataPosted(t, Handler(dataHost{access: access}, agentTestPort), `"e1"`, goodData)
		wantDataRefusal(t, rec, access, http.StatusPreconditionFailed, "conflict", 1)
		body := decodeAgentBody[envelope](t, rec)
		if body.Error.ETag != "e9" || body.Error.SaveID != "op-earlier" || rec.Header().Get("ETag") != "e9" {
			t.Fatalf("conflict = %s with ETag %q, want the current etag and the save that wrote it", rec.Body.String(), rec.Header().Get("ETag"))
		}
	})

	t.Run("the data guard's refusal answers the exact 428 read-required body", func(t *testing.T) {
		access := &dataAccess{err: &ReadRequiredError{
			ETag:        "e9",
			Message:     "This write would remove data. Read the document first and send its etag as If-Match.",
			WouldRemove: WouldRemove{Items: 1, Values: 2, Paths: []string{"/items/0"}},
		}}
		rec := dataPosted(t, Handler(dataHost{access: access}, agentTestPort), `"e1"`, goodData)
		want := `{"error":{"code":"read-required","message":"This write would remove data. Read the document first and send its etag as If-Match.","etag":"e9","wouldRemove":{"items":1,"values":2,"paths":["/items/0"]}}}` + "\n"
		if rec.Code != http.StatusPreconditionRequired || rec.Body.String() != want {
			t.Fatalf("read-required = %d %s, want 428 %s", rec.Code, rec.Body.String(), want)
		}
		if got := rec.Header().Get("ETag"); got != "e9" {
			t.Fatalf("ETag header = %q, want e9", got)
		}
	})

	t.Run("a write is refused with the code its error names", func(t *testing.T) {
		cases := []struct {
			name    string
			host    dataHost
			body    string
			status  int
			code    string
			message string
		}{
			{"a version that could not be kept", dataHost{access: &dataAccess{err: ErrBackup}}, goodData, 503, "recovery-unavailable", textRecoveryUnavailable},
			{"no api rules", dataHost{access: &dataAccess{err: ErrNoAPIRules}}, goodData, 400, "no-api-rules", textNoAPIRules},
			{"a patch its owner does not take", dataHost{access: &dataAccess{err: ErrPatchUnsupported}}, goodPatch, 400, "patch-unsupported", textPatchUnsupported},
			{"data the rules cannot hold", dataHost{access: &dataAccess{err: &DataError{Status: 400, Message: "write rejected: 1 unknown key(s), 0 rule(s) with no matching element"}}}, goodData, 400, "bad-data", "write rejected: 1 unknown key(s), 0 rule(s) with no matching element"},
			{"rules the engine cannot use", dataHost{access: &dataAccess{err: &DataError{Status: 400, Rules: true, Message: "The api rules tag body is not valid JSON."}}}, goodData, 400, "bad-api-rules", "The api rules tag body is not valid JSON."},
			{"the owner's own code", dataHost{access: &dataAccess{err: &DataError{Status: 400, Code: "last_sheet", Message: "A document keeps at least one sheet."}}}, goodPatch, 400, "last_sheet", "A document keeps at least one sheet."},
			{"the owner's own status", dataHost{access: &dataAccess{err: &DataError{Status: 409, Code: "sheet-not-canonical", Message: "Open the sheet in the editor and save once."}}}, goodPatch, 409, "sheet-not-canonical", "Open the sheet in the editor and save once."},
			{"a status that is not a refusal", dataHost{access: &dataAccess{err: &DataError{Status: 500, Message: "refused"}}}, goodData, 400, "bad-data", "refused"},
			{"data too large", dataHost{access: &dataAccess{err: ErrTooLarge}}, goodData, 413, "too-large", textDataTooLarge},
			{"a document that is gone", dataHost{access: &dataAccess{err: ErrNotFound}}, goodData, 404, "not-found", textNotFound},
			{"a document no owner reaches", dataHost{accessErr: ErrNotReachable}, goodData, 404, "not-reachable", textNotReachable},
			{"an error with no name", dataHost{access: &dataAccess{err: errors.New("disk")}}, goodData, 500, "internal", textWriteFailed},
		}
		for _, c := range cases {
			rec := dataPosted(t, Handler(c.host, agentTestPort), `"e1"`, c.body)
			body := decodeAgentBody[envelope](t, rec)
			if rec.Code != c.status || body.Error.Code != c.code || body.Error.Message != c.message {
				t.Fatalf("%s = %d %s, want %d %s %q", c.name, rec.Code, rec.Body.String(), c.status, c.code, c.message)
			}
			if body.Error.WouldRemove != nil {
				t.Fatalf("%s carries wouldRemove: %s", c.name, rec.Body.String())
			}
		}
	})

	t.Run("a repeated operation id answers the first answer and writes once", func(t *testing.T) {
		access := &dataAccess{result: WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}}
		handler := Handler(dataHost{access: access}, agentTestPort)
		first := dataPosted(t, handler, `"e1"`, goodData)
		access.mu.Lock()
		access.result = WriteResult{ETag: "e3", Changed: true, UndoVersion: "v8"}
		access.mu.Unlock()
		second := dataPosted(t, handler, `"e1"`, goodData)
		if first.Code != http.StatusOK || second.Code != first.Code || second.Body.String() != first.Body.String() {
			t.Fatalf("the repeat answered %d %s, want the first answer %d %s", second.Code, second.Body.String(), first.Code, first.Body.String())
		}
		if got := second.Header().Get("ETag"); got != "e2" {
			t.Fatalf("the repeat's ETag header = %q, want e2", got)
		}
		if got := len(access.writesSoFar()); got != 1 {
			t.Fatalf("the owner got %d writes, want 1", got)
		}
	})

	t.Run("a refusal is replayed too, so a guard's answer is not asked for twice", func(t *testing.T) {
		access := &dataAccess{err: &ReadRequiredError{ETag: "e9", Message: "read first", WouldRemove: WouldRemove{Items: 1, Paths: []string{}}}}
		handler := Handler(dataHost{access: access}, agentTestPort)
		first := dataPosted(t, handler, `"e1"`, goodData)
		second := dataPosted(t, handler, `"e1"`, goodData)
		if first.Code != http.StatusPreconditionRequired || second.Body.String() != first.Body.String() {
			t.Fatalf("the repeat answered %d %s, want the first answer %d %s", second.Code, second.Body.String(), first.Code, first.Body.String())
		}
		if got := len(access.writesSoFar()); got != 1 {
			t.Fatalf("the owner got %d writes, want 1", got)
		}
	})

	t.Run("an operation id sent again with another write answers 400 operation-reused", func(t *testing.T) {
		access := &dataAccess{result: WriteResult{ETag: "e2", Changed: true}}
		handler := Handler(dataHost{access: access}, agentTestPort)
		dataPosted(t, handler, `"e1"`, `{"data":[{"sheet":"Budget","delete":true}],"operationId":"op-1"}`)
		others := map[string]*httptest.ResponseRecorder{
			"other data":             dataPosted(t, handler, `"e1"`, `{"data":{"title":"C"},"operationId":"op-1"}`),
			"another etag":           dataPosted(t, handler, `"e0"`, `{"data":[{"sheet":"Budget","delete":true}],"operationId":"op-1"}`),
			"the same JSON as patch": dataPosted(t, handler, `"e1"`, `{"patch":[{"sheet":"Budget","delete":true}],"operationId":"op-1"}`),
		}
		for name, rec := range others {
			if rec.Code != http.StatusBadRequest || agentRefusal(t, rec) != "operation-reused" {
				t.Fatalf("%s = %d %s, want 400 operation-reused", name, rec.Code, rec.Body.String())
			}
		}
		if got := len(access.writesSoFar()); got != 1 {
			t.Fatalf("the owner got %d writes, want 1", got)
		}
	})

	t.Run("a 503 is not kept, so the same operation id runs again", func(t *testing.T) {
		access := &dataAccess{err: ErrBackup}
		handler := Handler(dataHost{access: access}, agentTestPort)
		wantDataRefusal(t, dataPosted(t, handler, `"e1"`, goodData), access, http.StatusServiceUnavailable, "recovery-unavailable", 1)
		access.mu.Lock()
		access.err, access.result = nil, WriteResult{ETag: "e2", Changed: true, UndoVersion: "v7"}
		access.mu.Unlock()
		wantWriteAnswer(t, dataPosted(t, handler, `"e1"`, goodData), "e2", "op-1", "v7")
		if got := len(access.writesSoFar()); got != 2 {
			t.Fatalf("the owner got %d writes, want 2", got)
		}
	})

	t.Run("the data route answers GET and POST only, and has nothing below it", func(t *testing.T) {
		access := &dataAccess{data: `{}`, etag: "e1"}
		handler := Handler(dataHost{access: access}, agentTestPort)
		for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
			rec := writeRequest(t, handler, method, agentDataTarget, goodData, "If-Match", `"e1"`)
			wantDataRefusal(t, rec, access, http.StatusMethodNotAllowed, "method-not-allowed", 0)
		}
		rec := writeRequest(t, handler, http.MethodPost, agentDataTarget+"/title", goodData, "If-Match", `"e1"`)
		wantDataRefusal(t, rec, access, http.StatusNotFound, "not-found", 0)
		rec = writeRequest(t, handler, http.MethodPost, "/_/agent/v1/documents/not-an-id/data", goodData, "If-Match", `"e1"`)
		wantDataRefusal(t, rec, access, http.StatusBadRequest, "bad-request", 0)
		rec = agentCall(t, handler, http.MethodGet, "/_/agent/v1/documents/not-an-id/data", nil)
		wantDataRefusal(t, rec, access, http.StatusBadRequest, "bad-request", 0)
	})

	t.Run("a page is refused before the owner is reached", func(t *testing.T) {
		access := &dataAccess{data: `{}`, etag: "e1"}
		handler := Handler(dataHost{access: access}, agentTestPort)
		rec := dataPosted(t, handler, `"e1"`, goodData, "Origin", "http://notes.htmlclay.localhost:24190")
		wantDataRefusal(t, rec, access, http.StatusForbidden, "forbidden", 0)
		rec = agentCall(t, handler, http.MethodGet, agentDataTarget, map[string]string{"Sec-Fetch-Site": "same-origin"})
		wantDataRefusal(t, rec, access, http.StatusForbidden, "forbidden", 0)
	})
}
