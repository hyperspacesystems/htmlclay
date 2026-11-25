package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyRequest(t *testing.T, srv *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

// Recovery copies (plan 09 H-r3): a live-room tab stores the edits it could not save, and they must
// outlive the window. Bugs it catches: a copy overwritten by a second write under its id; a repeat
// of the same write refused, so a retried PUT reports a loss; a read answered from anything but what
// was stored; a decision mark recorded twice or lost; a copy past the limits accepted; a copy deleted
// with the provisional history it sits in; a copy reachable without the document's token.
func TestOrderedRecoveryCopies(t *testing.T) {
	fx := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", openRecord))
	srv, tok := fx.srv, fx.file.Token
	base := "/_/copies/" + tok

	if w := copyRequest(t, srv, "GET", base, ""); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"copies":[]}` {
		t.Fatalf("no copies yet: %d %s", w.Code, w.Body.String())
	}
	if w := copyRequest(t, srv, "PUT", base+"/c1", "<p>held edits</p>"); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := copyRequest(t, srv, "PUT", base+"/c1", "<p>held edits</p>"); w.Code != 200 {
		t.Fatalf("the same bytes again: %d %s", w.Code, w.Body.String())
	}
	if w := copyRequest(t, srv, "PUT", base+"/c1", "<p>other edits</p>"); w.Code != 409 {
		t.Fatalf("different bytes under a stored id: %d %s", w.Code, w.Body.String())
	}
	if w := copyRequest(t, srv, "GET", base+"/c1", ""); w.Code != 200 || w.Body.String() != "<p>held edits</p>" {
		t.Fatalf("read: %d %q", w.Code, w.Body.String())
	}
	if w := copyRequest(t, srv, "GET", base+"/c2", ""); w.Code != 404 {
		t.Fatalf("an unknown copy: %d", w.Code)
	}
	if w := copyRequest(t, srv, "PUT", base+"/bad.id", "x"); w.Code != 400 {
		t.Fatalf("an id with a dot: %d", w.Code)
	}
	if w := copyRequest(t, srv, "PUT", base+"/big", strings.Repeat("x", copyLimit+1)); w.Code != 413 {
		t.Fatalf("a copy over 6 MiB: %d", w.Code)
	}
	for i := 0; i < 5; i++ {
		copyRequest(t, srv, "PUT", fmt.Sprintf("%s/fill%d", base, i), strings.Repeat(fmt.Sprint(i), copyLimit))
	}
	if w := copyRequest(t, srv, "PUT", base+"/last", strings.Repeat("y", copyLimit)); w.Code != 413 {
		t.Fatalf("a copy past 32 MiB for the document: %d", w.Code)
	}

	mark := `{"e":"t1.4","kind":"kept"}`
	for i := 0; i < 2; i++ {
		w := copyRequest(t, srv, "POST", base+"/c1/done", mark)
		var answer struct {
			ID   string     `json:"id"`
			Done []copyMark `json:"done"`
		}
		json.Unmarshal(w.Body.Bytes(), &answer)
		if w.Code != 200 || answer.ID != "c1" || len(answer.Done) != 1 || string(answer.Done[0].E) != `"t1.4"` || answer.Done[0].Kind != "kept" || answer.Done[0].At == 0 {
			t.Fatalf("mark %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	if w := copyRequest(t, srv, "POST", base+"/c1/done", `{"e":"t1.4","kind":"dropped"}`); !strings.Contains(w.Body.String(), `"dropped"`) || strings.Count(w.Body.String(), `"kind"`) != 2 {
		t.Fatalf("a second kind for one edit: %s", w.Body.String())
	}
	if w := copyRequest(t, srv, "POST", base+"/nope/done", mark); w.Code != 404 {
		t.Fatalf("a mark on an unknown copy: %d", w.Code)
	}
	w := copyRequest(t, srv, "GET", base, "")
	var list struct {
		Copies []copyEntry `json:"copies"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Copies) != 6 || list.Copies[0].ID != "c1" || list.Copies[0].Size != int64(len("<p>held edits</p>")) || len(list.Copies[0].Done) != 2 || list.Copies[0].Time == 0 {
		t.Fatalf("the list: %s", w.Body.String())
	}

	if w := copyRequest(t, srv, "GET", "/_/copies/not-a-token/c1", ""); w.Code == 200 {
		t.Fatalf("a copy read without the document's token: %d", w.Code)
	}
	put := httptest.NewRequest("PUT", base+"/c9", strings.NewReader("x"))
	put.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	put.Header.Set("Sec-Fetch-Site", "cross-site")
	pw := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(pw, put)
	if pw.Code != 403 {
		t.Fatalf("a cross-site copy write: %d", pw.Code)
	}

	folder, err := srv.versions.ExistingFolder(historyKey(fx.file), fx.file.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, copiesDir, "c1.html")); err != nil {
		t.Fatalf("the copy is not beside the versions: %v", err)
	}
}

// A copy stored for a document whose history is still a provisional first-open snapshot must make
// that history durable, or the sweep that deletes aged provisional histories deletes the copy too.
func TestOrderedRecoveryCopyKeepsItsHistory(t *testing.T) {
	fx := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", openRecord))
	key := historyKey(fx.file)
	if _, err := fx.srv.versions.Folder(key, fx.file.AbsPath); err != nil {
		t.Fatal(err)
	}
	if err := fx.srv.versions.SetProvisional(key, fx.file.AbsPath, true); err != nil {
		t.Fatal(err)
	}
	if w := copyRequest(t, fx.srv, "PUT", "/_/copies/"+fx.file.Token+"/c1", "<p>held</p>"); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	folder, err := fx.srv.versions.ExistingFolder(key, fx.file.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(folder, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"provisional":true`) {
		t.Fatalf("the history holding a copy is still provisional: %s", data)
	}
}

// What a crash or a failing disk leaves behind must not hide or lose a copy. Bugs it catches: a copy
// whose index entry was never written missing from the list and refusing its marks; a torn index
// refusing every later copy of the document; a retried PUT, after a read of the stored copy failed,
// deleting that copy; a mark whose edit id holds "<" stored again on every retry.
func TestOrderedRecoveryCopiesSurviveACrash(t *testing.T) {
	fx := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", openRecord))
	srv, base := fx.srv, "/_/copies/"+fx.file.Token
	if w := copyRequest(t, srv, "PUT", base+"/c1", "<p>one</p>"); w.Code != 201 {
		t.Fatalf("create: %d", w.Code)
	}
	folder, err := srv.versions.ExistingFolder(historyKey(fx.file), fx.file.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(folder, copiesDir)

	if err := os.WriteFile(filepath.Join(dir, "c2.html"), []byte("<p>two</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := copyRequest(t, srv, "GET", base, ""); !strings.Contains(w.Body.String(), `"id":"c2"`) {
		t.Fatalf("a copy with no index entry is not listed: %s", w.Body.String())
	}
	if w := copyRequest(t, srv, "POST", base+"/c2/done", `{"e":"t1:<4>","kind":"kept"}`); w.Code != 200 {
		t.Fatalf("a mark on a copy with no index entry: %d %s", w.Code, w.Body.String())
	}
	copyRequest(t, srv, "POST", base+"/c2/done", `{"e":"t1:<4>","kind":"kept"}`)
	if w := copyRequest(t, srv, "GET", base, ""); strings.Count(w.Body.String(), `"kind"`) != 1 {
		t.Fatalf("one mark sent twice is stored twice: %s", w.Body.String())
	}
	if w := copyRequest(t, srv, "POST", base+"/c2/done", `{"e":4,"kind":"kept"}`); w.Code != 400 {
		t.Fatalf("a mark whose e is not an edit id: %d", w.Code)
	}

	if err := os.WriteFile(filepath.Join(dir, copiesIndex), []byte(`{"c1":{"ti`), 0600); err != nil {
		t.Fatal(err)
	}
	if w := copyRequest(t, srv, "PUT", base+"/c3", "<p>three</p>"); w.Code != 201 {
		t.Fatalf("a copy after a torn index: %d %s", w.Code, w.Body.String())
	}
	if w := copyRequest(t, srv, "GET", base, ""); w.Code != 200 || strings.Count(w.Body.String(), `"id"`) != 3 {
		t.Fatalf("the list after a torn index: %d %s", w.Code, w.Body.String())
	}

	stored := filepath.Join(dir, "c1.html")
	if err := os.Chmod(stored, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(stored); err == nil {
		t.Skip("this user reads files with no permissions, so the read error cannot be made")
	}
	w := copyRequest(t, srv, "PUT", base+"/c1", "<p>one</p>")
	os.Chmod(stored, 0600)
	if w.Code != 500 {
		t.Fatalf("a retry while the stored copy cannot be read: %d", w.Code)
	}
	if data, err := os.ReadFile(stored); err != nil || string(data) != "<p>one</p>" {
		t.Fatalf("the retry lost the stored copy: %q %v", data, err)
	}
}

// A copy request can be the first request a .htmlclay sheet gets (a tab still open across an update
// restart). Bug it catches: the copy routes pinning the sheet to a path-derived history key, so its
// versions and copies split from the id-keyed history every later save uses.
func TestOrderedRecoveryCopyKeysByDocumentIdentity(t *testing.T) {
	fx := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", openRecord))
	if key := fx.file.HistoryKey(); key != "" {
		t.Skipf("the fixture already resolved key %s, so this cannot test the first request", key)
	}
	if w := copyRequest(t, fx.srv, "PUT", "/_/copies/"+fx.file.Token+"/c1", "<p>held</p>"); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if key := fx.file.HistoryKey(); !strings.HasPrefix(key, "id:") {
		t.Fatalf("the first copy request keyed the sheet %q, want its id", key)
	}
}
