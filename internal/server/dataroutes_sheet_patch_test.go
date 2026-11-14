package server

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/testutil"
)

// sheetPatchVector is a patch case out of data-api.json: the page, the body the case sends, and the
// recorded artifacts (the patched page and the change count).
type sheetPatchVector struct {
	Name   string `json:"name"`
	HTML   string `json:"html"`
	Expect struct {
		Data  json.RawMessage            `json:"data"`
		Files map[string]json.RawMessage `json:"files"`
	} `json:"expect"`
}

func sheetPatchVectors(t *testing.T) map[string]sheetPatchVector {
	t.Helper()
	var cases []sheetPatchVector
	testutil.LoadVectors(t, "data-api.json", &cases)
	byName := map[string]sheetPatchVector{}
	for _, c := range cases {
		byName[c.Name] = c
	}
	return byName
}

type sheetPatchAnswer struct {
	Etag    string `json:"etag"`
	Changed struct {
		Cells int `json:"cells"`
		Rows  int `json:"rows"`
	} `json:"changed"`
	Recalculated bool `json:"recalculated"`
	Stale        bool `json:"stale"`
}

func patchBody(t *testing.T, c sheetPatchVector) string {
	t.Helper()
	return `{"patch":` + string(c.Expect.Data) + `}`
}

// TestSheetPatchAppliesPostsThePatchAndCommitsIt covers the success answer and the bytes on disk: the
// changed cell, the table's stale mark, the stamp of what was written, and a plain read afterwards.
func TestSheetPatchAppliesPostsThePatchAndCommitsIt(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-range"]
	path := writeHome(t, srv, "test.htmlclay", c.HTML)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", patchBody(t, c), nil)
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Body.String(), `{"etag":`) {
		t.Errorf("body does not start with the etag: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"sheets":`) {
		t.Errorf("cell-only patch includes a sheet count: %s", w.Body.String())
	}
	var answer sheetPatchAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatalf("body is not the patch answer: %v: %s", err, w.Body.String())
	}
	var wantWrite struct {
		Changed struct {
			Cells int `json:"cells"`
			Rows  int `json:"rows"`
		} `json:"changed"`
		Stale bool `json:"stale"`
	}
	if err := json.Unmarshal(c.Expect.Files["write.json"], &wantWrite); err != nil {
		t.Fatalf("write.json: %v", err)
	}
	if answer.Changed != wantWrite.Changed {
		t.Errorf("changed = %+v, want %+v", answer.Changed, wantWrite.Changed)
	}
	if answer.Recalculated {
		t.Errorf("recalculated = true, want false")
	}
	if !answer.Stale {
		t.Errorf("stale = false, want true")
	}

	stored := readDoc(t, path)
	if want := specwire.Etag([]byte(stored)); answer.Etag != want {
		t.Errorf("etag = %q, want the stamp of the stored bytes %q", answer.Etag, want)
	}
	if got := w.Header().Get("ETag"); got != answer.Etag {
		t.Errorf("ETag header = %q, want %q", got, answer.Etag)
	}

	var after string
	if err := json.Unmarshal(c.Expect.Files["after.html"], &after); err != nil {
		t.Fatalf("after.html: %v", err)
	}
	start := after[strings.Index(after, "<table grid"):]
	start = start[:strings.Index(start, ">")+1]
	if !strings.Contains(stored, start) {
		t.Errorf("stored page does not contain %q", start)
	}
	row := after[strings.Index(after, `<tr data-id="r3"`):]
	row = row[:strings.Index(row, "</tr>")+len("</tr>")]
	if !strings.Contains(stored, row) {
		t.Errorf("stored page does not contain %q", row)
	}

	read := getAPI(t, srv, "test.htmlclay")
	if read.Code != 200 {
		t.Fatalf("read status = %d: %s", read.Code, read.Body.String())
	}
	var doc struct {
		Sheets []struct {
			Rows []struct {
				Cells []json.RawMessage `json:"cells"`
			} `json:"rows"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &doc); err != nil {
		t.Fatalf("read body: %v: %s", err, read.Body.String())
	}
	var cell float64
	if len(doc.Sheets) == 0 || len(doc.Sheets[0].Rows) < 3 || len(doc.Sheets[0].Rows[2].Cells) < 2 {
		t.Fatalf("read body has no B3: %s", read.Body.String())
	}
	if err := json.Unmarshal(doc.Sheets[0].Rows[2].Cells[1], &cell); err != nil || cell != 450 {
		t.Errorf("B3 = %v (%v), want 450", cell, err)
	}
}

// TestSheetPatchAcceptsOperationID keeps the one other key a JavaScript patch sends.
func TestSheetPatchAcceptsOperationID(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-range"]
	writeHome(t, srv, "test.htmlclay", c.HTML)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"operationId":"abc","patch":`+string(c.Expect.Data)+`}`, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
}

func TestSheetPatchDeleteResponseAndStoredTabs(t *testing.T) {
	for _, name := range []string{"sheet-patch-delete-by-name", "sheet-patch-delete-formula-stale", "sheet-patch-delete-only"} {
		t.Run(name, func(t *testing.T) {
			fixture := func(suffix string) string {
				t.Helper()
				raw, err := os.ReadFile(filepath.Join("..", "dataapi", "testdata", "conformance", "sheet", name+suffix))
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			}
			srv, _, _ := setupHandlerTest(t)
			path := writeHome(t, srv, "test.htmlclay", fixture(".html"))
			before := readDoc(t, path)
			w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"patch":`+fixture(".data.json")+`}`, nil)
			if name == "sheet-patch-delete-only" {
				if w.Code != 400 || decodeWriteError(t, w).Code != "last_sheet" {
					t.Fatalf("last sheet refusal = %d %s", w.Code, w.Body.String())
				}
				if readDoc(t, path) != before {
					t.Fatal("refused delete changed the stored document")
				}
				return
			}
			if w.Code != 200 {
				t.Fatalf("delete status = %d: %s", w.Code, w.Body.String())
			}
			var answer struct {
				Etag         string         `json:"etag"`
				Changed      map[string]int `json:"changed"`
				Recalculated bool           `json:"recalculated"`
				Stale        bool           `json:"stale"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			if len(answer.Changed) != 3 || answer.Changed["sheets"] != 1 || answer.Changed["cells"] != 0 || answer.Changed["rows"] != 0 {
				t.Fatalf("changed = %#v, want only one sheet removed", answer.Changed)
			}
			if answer.Recalculated || answer.Stale != (name == "sheet-patch-delete-formula-stale") {
				t.Fatalf("recalculated=%v stale=%v", answer.Recalculated, answer.Stale)
			}
			stored := readDoc(t, path)
			if answer.Etag != specwire.Etag([]byte(stored)) || w.Header().Get("ETag") != answer.Etag {
				t.Fatal("response does not identify the stored document")
			}
			read := getAPI(t, srv, "test.htmlclay")
			var got, want any
			if read.Code != 200 {
				t.Fatalf("read status = %d: %s", read.Code, read.Body.String())
			}
			if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(fixture(".after.json")), &want); err != nil {
				t.Fatal(err)
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("stored tabs = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestSheetPatchBodyClassification pins the leader's three data cases: only a lone patch list is a
// patch, every other body stays a data write.
func TestSheetPatchBodyClassification(t *testing.T) {
	cases := sheetPatchVectors(t)
	page := cases["sheet-patch-range"]
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay", page.HTML)

	for _, body := range []string{
		`{"patch":` + string(page.Expect.Data) + `,"other":1}`,
		`{"patch":"x"}`,
	} {
		w := postDataWrite(t, srv, "/_/api/test.htmlclay", body, nil)
		if w.Code != 400 {
			t.Fatalf("%s: status = %d: %s", body, w.Code, w.Body.String())
		}
		if got := decodeWriteError(t, w); got.Error != "Write rejected" {
			t.Errorf("%s: error = %q: %s", body, got.Error, w.Body.String())
		}
	}

	defined := cases["body-patch-key-in-rules"]
	srv2, _, _ := setupHandlerTest(t)
	writeHome(t, srv2, "test.htmlclay", defined.HTML)
	w := postDataWrite(t, srv2, "/_/api/test.htmlclay", string(defined.Expect.Data), nil)
	if strings.Contains(w.Body.String(), "recalculated") {
		t.Errorf("a rules-defined patch key took the patch branch: %d %s", w.Code, w.Body.String())
	}
}

// TestSheetPatchIfMatch covers the conditional write: a stale stamp answers 412 with the current
// stamp and leaves the file alone; the current stamp lets the patch through.
func TestSheetPatchIfMatch(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-range"]
	path := writeHome(t, srv, "test.htmlclay", c.HTML)
	before := readDoc(t, path)

	current := getAPI(t, srv, "test.htmlclay").Header().Get("ETag")
	if current == "" {
		t.Fatal("getAPI sent no ETag")
	}

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", patchBody(t, c), map[string]string{"If-Match": "stale-stamp"})
	if w.Code != 412 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("ETag"); got != current {
		t.Errorf("412 ETag = %q, want %q", got, current)
	}
	if after := readDoc(t, path); after != before {
		t.Errorf("a refused patch changed the file")
	}

	w = postDataWrite(t, srv, "/_/api/test.htmlclay", patchBody(t, c), map[string]string{"If-Match": current})
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
}

// TestSheetPatchNotCanonical covers the codec's 409: a table edited outside the sheet editor is
// refused before anything is written.
func TestSheetPatchNotCanonical(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-not-canonical"]
	path := writeHome(t, srv, "test.htmlclay", c.HTML)
	before := readDoc(t, path)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", patchBody(t, c), nil)
	if w.Code != 409 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Error != "Sheet refused" || body.Code != "sheet-not-canonical" {
		t.Errorf("error = %q, code = %q: %s", body.Error, body.Code, w.Body.String())
	}
	if after := readDoc(t, path); after != before {
		t.Errorf("a refused patch changed the file")
	}
}

// TestSheetPatchBadRange covers a query the codec refuses on its way to the write.
func TestSheetPatchBadRange(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-range"]
	path := writeHome(t, srv, "test.htmlclay", c.HTML)
	before := readDoc(t, path)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"patch":[{"sheet":"Budget","range":"NOPE","values":[[1]]}]}`, nil)
	if w.Code != 400 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Error != "Invalid sheet query" || body.Code != "bad_range" {
		t.Errorf("error = %q, code = %q: %s", body.Error, body.Code, w.Body.String())
	}
	if after := readDoc(t, path); after != before {
		t.Errorf("a refused patch changed the file")
	}
}

// TestSheetWholeValueWriteRefused covers the data path's whole-sheet refusal: replacing the sheets
// list as a value is a codec refusal, and the write face answers 409 with its code.
func TestSheetWholeValueWriteRefused(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-range"]
	path := writeHome(t, srv, "test.htmlclay", c.HTML)
	before := readDoc(t, path)

	w := postDataWrite(t, srv, "/_/api/test.htmlclay", `{"sheets":[]}`, nil)
	if w.Code != 409 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	body := decodeWriteError(t, w)
	if body.Error != "Sheet refused" || body.Code != "sheet-write-unavailable" {
		t.Errorf("error = %q, code = %q: %s", body.Error, body.Code, w.Body.String())
	}
	if after := readDoc(t, path); after != before {
		t.Errorf("a refused write changed the file")
	}
}

// TestSheetPatchQueryFaceDoesNotPatch keeps the ?data= caller on its own rules: a patch-shaped body
// is a data write there, never a patch.
func TestSheetPatchQueryFaceDoesNotPatch(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetPatchVectors(t)["sheet-patch-range"]
	writeHome(t, srv, "test.htmlclay", c.HTML)

	rules := url.QueryEscape(`{"title":"h1","sheets":"[items=sheets]@codec(sheet)"}`)
	w := postDataWrite(t, srv, "/_/api/test.htmlclay?data="+rules, patchBody(t, c), nil)
	if strings.Contains(w.Body.String(), "recalculated") {
		t.Errorf("a ?data= caller took the patch branch: %d %s", w.Code, w.Body.String())
	}
}
