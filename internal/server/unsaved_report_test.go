package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

type unsavedReportCall struct {
	path    string
	tab     string
	seq     int64
	unsaved bool
}

func postUnsavedThroughMux(t *testing.T, srv *Server, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Origin", "http://"+req.Host)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

func TestUnsavedReportRoute(t *testing.T) {
	srv, f, _ := setupHandlerTest(t)
	var got []unsavedReportCall
	srv.SetHooks(Hooks{UnsavedReport: func(absPath, tab string, seq int64, unsaved, frozen bool) {
		got = append(got, unsavedReportCall{path: absPath, tab: tab, seq: seq, unsaved: unsaved})
	}})

	w := postUnsavedThroughMux(t, srv, "/_/unsaved/"+f.Token, `{"tab":"t1","seq":1,"unsaved":true}`, nil)
	if w.Code != 204 {
		t.Fatalf("same-origin report = %d %s, want 204", w.Code, w.Body.String())
	}
	if len(got) != 1 || got[0] != (unsavedReportCall{path: f.AbsPath, tab: "t1", seq: 1, unsaved: true}) {
		t.Fatalf("recorder got %+v, want %s t1 1 true", got, f.AbsPath)
	}

	got = nil
	w = postUnsavedThroughMux(t, srv, "/_/unsaved/"+f.Token, `{"tab":"t1","seq":2,"unsaved":false}`,
		map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "http://evil.example"})
	if w.Code != 403 {
		t.Fatalf("cross-origin report = %d %s, want 403", w.Code, w.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("a cross-origin report reached the recorder: %+v", got)
	}

	w = postUnsavedThroughMux(t, srv, "/_/unsaved/"+strings.Repeat("a", 43), `{"tab":"t1","seq":1,"unsaved":true}`, nil)
	if w.Code != 401 {
		t.Fatalf("unknown token = %d %s, want 401", w.Code, w.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("an unknown token reached the recorder: %+v", got)
	}

	w = postUnsavedThroughMux(t, srv, "/_/unsaved/"+f.Token, `{"tab":"t1","seq":0,"unsaved":true}`, nil)
	if w.Code != 400 {
		t.Fatalf("bad body = %d %s, want 400", w.Code, w.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("a bad body reached the recorder: %+v", got)
	}

	meta := getThroughMux(t, srv, "/_/meta")
	var answer struct {
		Extensions []string `json:"extensions"`
	}
	if err := json.Unmarshal(meta.Body.Bytes(), &answer); err != nil {
		t.Fatalf("meta is not JSON: %v (%q)", err, meta.Body.String())
	}
	found := false
	for _, name := range answer.Extensions {
		found = found || name == "unsaved-report-v1"
	}
	if !found {
		t.Fatalf("/_/meta extensions = %v, want unsaved-report-v1", answer.Extensions)
	}
}
