package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/commitrecord"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/versions"
)

const sheetRules = `<script type="application/json" data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)"}</script>`

const openRecord = `{"v":2,"doc":"d1","gen":"1.00000005","epoch":1,"commit":"c00000000009","parent":null,"through":0,"room":null,"mode":"file","lineage":[],"cut":null,"floor":-1,"seen":{"tab.0":[2,1]}}`

// sheetPage is a one-tab workbook as ClayJS's sheet writes it, with its commit record after the tabs.
func sheetPage(title, cell, record string) string {
	return `<!DOCTYPE html><html><head><title>` + title + `</title></head><body>` + sheetRules +
		`<div items="sheets" sheet-workbook=""><section item data-id="s1" ><h2 bind="name">Budget</h2><table grid><colgroup><col data-id="c1" data-pos="V"></colgroup><tbody><tr data-id="r1" data-pos="V"><td>` + cell + `</td></tr></tbody></table></section>` +
		`<script type="application/json" sheet-commit="">` + record + `</script></div></body></html>`
}

func nextRecord(parent, commit string) string {
	return `{"v":2,"doc":"d1","gen":"1.00000005","epoch":1,"commit":"` + commit + `","parent":"` + parent + `","through":0,"room":null,"mode":"file","lineage":[],"cut":null,"floor":-1,"seen":{"tab.0":[3,1]}}`
}

func saveWithIfMatch(t *testing.T, srv *Server, f *session.File, body, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/_/save/"+f.Token, strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.SetPathValue("token", f.Token)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	w := httptest.NewRecorder()
	srv.handleSave(w, req)
	return w
}

func storedFile(t *testing.T, path string) (string, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return specwire.Etag(data), data
}

// savedFrame is the next frame the saved lane sends for path, decoded.
func savedFrame(t *testing.T, sub *subscriber) map[string]any {
	t.Helper()
	select {
	case q := <-sub.ch:
		data := string(q.frame)
		at := strings.Index(data, "data: ")
		if at < 0 {
			t.Fatalf("frame without data: %q", data)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(data[at+len("data: "):])), &payload); err != nil {
			t.Fatalf("frame %q: %v", data, err)
		}
		return payload
	case <-time.After(5 * time.Second):
		t.Fatal("no saved-lane frame")
		return nil
	}
}

// The file contract on a sheet that carries a commit record (plan 09 H-r1). Bugs it catches: a save
// with no If-Match, or one that drops or forges the record, rewinding a sheet other windows are
// saving to; a refusal that still writes; a saved-lane push without the stored bytes' ETag, which
// the file-only client needs to take the push as the file; any of this reaching a document with no
// record.
func TestOrderedFileSaves(t *testing.T) {
	fx := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", openRecord))
	srv, f := fx.srv, fx.file
	sub := newSubscriber(f.AbsPath, laneSaved)
	srv.hub.add(sub)
	t.Cleanup(func() { srv.hub.remove(sub) })

	etag, before := storedFile(t, f.AbsPath)
	refusals := []struct {
		name, body, ifMatch string
		status              int
		code                string
	}{
		{"no If-Match", sheetPage("Sheet", "b", nextRecord("c00000000009", "c00000000010")), "", http.StatusPreconditionRequired, "if-match-required"},
		{"a wildcard If-Match", sheetPage("Sheet", "b", nextRecord("c00000000009", "c00000000010")), "*", http.StatusPreconditionRequired, "if-match-required"},
		{"the record dropped", strings.Replace(sheetPage("Sheet", "b", openRecord), `<script type="application/json" sheet-commit="">`+openRecord+`</script>`, "", 1), etag, http.StatusConflict, "not-next-commit"},
		{"a forged parent", sheetPage("Sheet", "b", nextRecord("c00000000001", "c00000000010")), etag, http.StatusConflict, "not-next-commit"},
		{"another document's record", sheetPage("Sheet", "b", strings.Replace(nextRecord("c00000000009", "c00000000010"), `"doc":"d1"`, `"doc":"d2"`, 1)), etag, http.StatusConflict, "not-next-commit"},
	}
	for _, c := range refusals {
		w := saveWithIfMatch(t, srv, f, c.body, c.ifMatch)
		var answer struct {
			Code string `json:"code"`
		}
		json.Unmarshal(w.Body.Bytes(), &answer)
		if w.Code != c.status || answer.Code != c.code {
			t.Fatalf("%s: %d %s, want %d %s", c.name, w.Code, w.Body.String(), c.status, c.code)
		}
		if got := w.Header().Get("ETag"); got != etag {
			t.Errorf("%s: refusal ETag %q, want the stored %q", c.name, got, etag)
		}
		if _, now := storedFile(t, f.AbsPath); string(now) != string(before) {
			t.Fatalf("%s: the refused save changed the file", c.name)
		}
	}
	if w := saveWithIfMatch(t, srv, f, sheetPage("Sheet", "b", nextRecord("c00000000009", "c00000000010")), `"stale"`); w.Code != http.StatusPreconditionFailed {
		t.Fatalf("a stale If-Match: %d, want 412", w.Code)
	}

	if w := saveWithIfMatch(t, srv, f, sheetPage("Sheet", "b", nextRecord("c00000000009", "c00000000010")), etag); w.Code != 200 {
		t.Fatalf("the next commit: %d %s", w.Code, w.Body.String())
	}
	stored, _ := storedFile(t, f.AbsPath)
	if frame := savedFrame(t, sub); frame["etag"] != stored {
		t.Fatalf("the saved frame carries etag %v, want the stored bytes' %s", frame["etag"], stored)
	}

	if w := saveWithIfMatch(t, srv, f, sheetPage("Renamed", "b", nextRecord("c00000000009", "c00000000010")), stored); w.Code != 200 {
		t.Fatalf("a page save that carries the file's record unchanged: %d %s", w.Code, w.Body.String())
	}
	savedFrame(t, sub)

	plain := setupFileTest(t, "notes.htmlclay", "<!DOCTYPE html><html><body><p>one</p></body></html>")
	plainSub := newSubscriber(plain.file.AbsPath, laneSaved)
	plain.srv.hub.add(plainSub)
	t.Cleanup(func() { plain.srv.hub.remove(plainSub) })
	if w := saveWithIfMatch(t, plain.srv, plain.file, "<!DOCTYPE html><html><body><p>two</p></body></html>", ""); w.Code != 200 {
		t.Fatalf("a document with no record, saved without If-Match: %d %s", w.Code, w.Body.String())
	}
	if frame := savedFrame(t, plainSub); frame["etag"] != nil {
		t.Fatalf("a document with no record got an etag on its saved frame: %v", frame["etag"])
	}
}

func readRecord(t *testing.T, path string) *commitrecord.Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := commitrecord.Find(data)
	if !ok {
		t.Fatalf("no record in %s", data)
	}
	rec, err := commitrecord.Read(data, tag)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// A version restore and a sheet patch are writers other than a tab: each folds the record into a new
// generation (plan 09 H-r2), so an open tab reads the file, sees the cut, and judges its unsaved
// edits against the new state. Bugs they catch: a restore that writes the old version's record back,
// so a tab whose save the old record never counted saves its edit again over the restored version
// (the clayjs file-mode test "a version restored after a save whose answer was lost stays
// restored"); a patch that leaves the generation alone.
func TestOrderedFileFoldsOnRestoreAndDataWrite(t *testing.T) {
	oldRecord := `{"v":2,"doc":"d1","gen":"0.00000002","epoch":0,"commit":"c00000000005","parent":null,"through":0,"room":null,"mode":"file","lineage":[],"cut":null,"floor":-1,"seen":{"old.0":[1,0]}}`
	fx := setupFileTest(t, "sheet.html", sheetPage("Sheet", "a", openRecord))
	key := versions.Key(fx.file.AbsPath, []byte(sheetPage("Sheet", "a", openRecord)))
	if _, err := fx.srv.versions.Backup(key, fx.file.AbsPath, []byte(sheetPage("Sheet", "old", oldRecord))); err != nil {
		t.Fatal(err)
	}
	var name string
	for _, entry := range fx.history(t) {
		if strings.Contains(fx.versionBody(t, entry.Name), ">old<") {
			name = entry.Name
		}
	}
	req := httptest.NewRequest("POST", "/_/restore/"+fx.file.Token+"/"+name, nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", fx.srv.port)
	req.SetPathValue("token", fx.file.Token)
	req.SetPathValue("name", name)
	w := httptest.NewRecorder()
	fx.srv.handleRestoreVersion(w, req)
	if w.Code != 200 {
		t.Fatalf("restore: %d %s", w.Code, w.Body.String())
	}
	rec := readRecord(t, fx.file.AbsPath)
	if rec.Epoch != 2 || !strings.HasPrefix(rec.Gen, "2.") || rec.Parent == nil || *rec.Parent != "c00000000009" {
		t.Fatalf("restored record: epoch %d gen %s parent %v, want a new generation after c00000000009", rec.Epoch, rec.Gen, rec.Parent)
	}
	data, _ := os.ReadFile(fx.file.AbsPath)
	for _, want := range []string{`"restored":"c00000000005"`, `"cut":["1.00000005",0,{}]`, `"tab.0":[2,1]`, `"old.0":[1,0]`, ">old<"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("restored file lacks %s:\n%s", want, data)
		}
	}

	srv, _, _ := setupHandlerTest(t)
	path := writeHome(t, srv, "book.htmlclay", sheetPage("Sheet", "a", openRecord))
	if _, err := srv.sessions.Register(path, session.ViaOsOpen); err != nil {
		t.Fatal(err)
	}
	etag, _ := storedFile(t, path)
	w = postDataWrite(t, srv, "/_/api/book.htmlclay", `{"patch":[{"sheet":"Budget","range":"A1","values":[["=sum(a1)"]]}]}`, map[string]string{"If-Match": etag})
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	rec = readRecord(t, path)
	if rec.Epoch != 2 || rec.Parent == nil || *rec.Parent != "c00000000009" {
		t.Fatalf("patched record: epoch %d parent %v, want a new generation after c00000000009", rec.Epoch, rec.Parent)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), `<td f="=SUM(A1)">`) {
		t.Fatalf("the patch did not land: %s", data)
	}
}

// H7: a quit asks every open sheet to save, and waits for it. Bugs it catches: no flush frame, so a
// window's last edit has no host to save to; a flush frame sent to a document with no record, whose
// client does not know it; a quit that does not wait for an open sheet; a quit that keeps waiting
// after every sheet's stream has closed.
func TestFlushOpenSheetsBeforeQuit(t *testing.T) {
	sheet := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", openRecord))
	srv := sheet.srv
	plainPath := writeHome(t, srv, "notes.htmlclay", "<!DOCTYPE html><html><body><p>one</p></body></html>")
	ls := &LiveSync{hub: srv.hub}

	plainSub := newSubscriber(plainPath, laneSaved)
	srv.hub.add(plainSub)
	t.Cleanup(func() { srv.hub.remove(plainSub) })

	// A tab that answers the flush by closing its stream: the quit returns without using its wait.
	closing := newSubscriber(sheet.file.AbsPath, laneSaved)
	srv.hub.add(closing)
	go func() {
		q := <-closing.ch
		if strings.Contains(string(q.frame), `"type":"flush"`) {
			srv.hub.remove(closing)
		}
	}()
	done := make(chan struct{})
	go func() {
		ls.FlushOpenSheets(time.Hour)
		close(done)
	}()
	waitFor(t, 10*time.Second, "the quit to return once the sheet's stream closed", func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	select {
	case q := <-plainSub.ch:
		t.Fatalf("a document with no record got a frame: %s", q.frame)
	default:
	}

	// A tab that never answers: the quit waits for the whole wait, then goes on.
	silent := newSubscriber(sheet.file.AbsPath, laneSaved)
	srv.hub.add(silent)
	t.Cleanup(func() { srv.hub.remove(silent) })
	start := time.Now()
	ls.FlushOpenSheets(150 * time.Millisecond)
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("the quit returned after %v with a sheet's stream still open, want at least the wait", elapsed)
	}
	if frame := savedFrame(t, silent); frame["type"] != "flush" {
		t.Fatalf("the open sheet got %v, want a flush frame", frame)
	}
}

// A sheet whose record is not a version 2 commit (the unreleased template's {"rev":1} placeholder,
// an empty {}) is not under the file contract. Bug it catches: refusing every save of such a page,
// with 428 for no If-Match or 409 forever because no next record can name its parent.
func TestOrderedFileIgnoresAPlaceholderRecord(t *testing.T) {
	for _, record := range []string{`{"rev":1}`, `{}`} {
		fx := setupFileTest(t, "sheet.htmlclay", sheetPage("Sheet", "a", record))
		if w := saveWithIfMatch(t, fx.srv, fx.file, sheetPage("Sheet", "b", record), ""); w.Code != 200 {
			t.Fatalf("record %s: a save without If-Match answered %d %s", record, w.Code, w.Body.String())
		}
		etag, _ := storedFile(t, fx.file.AbsPath)
		if w := saveWithIfMatch(t, fx.srv, fx.file, sheetPage("Sheet", "c", openRecord), etag); w.Code != 200 {
			t.Fatalf("record %s: the first real commit answered %d %s", record, w.Code, w.Body.String())
		}
	}
}
