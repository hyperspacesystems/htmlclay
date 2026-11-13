package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

// sheetVector is one case out of the shared data-api.json corpus, reduced to what the server tests
// read: the page, the recorded artifacts, and the parameters the case sends.
type sheetVector struct {
	Name   string          `json:"name"`
	HTML   string          `json:"html"`
	Face   string          `json:"face"`
	Params *string         `json:"params"`
	Query  json.RawMessage `json:"query"`
	Expect struct {
		Files map[string]json.RawMessage `json:"files"`
	} `json:"expect"`
}

func sheetVectors(t *testing.T) map[string]sheetVector {
	t.Helper()
	var cases []sheetVector
	testutil.LoadVectors(t, "data-api.json", &cases)
	byName := map[string]sheetVector{}
	for _, c := range cases {
		byName[c.Name] = c
	}
	return byName
}

// vectorJSON is the case's recorded artifact as compact JSON text, so a body can be compared
// against it without copying the page or the expected data into this file.
func vectorJSON(t *testing.T, c sheetVector, suffix string) string {
	t.Helper()
	raw, ok := c.Expect.Files[suffix]
	if !ok {
		t.Fatalf("%s has no %s", c.Name, suffix)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("%s.%s: %v", c.Name, suffix, err)
	}
	return buf.String()
}

func compactBody(t *testing.T, body []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, body); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, body)
	}
	return buf.String()
}

// getAPIQuery drives the handler with a raw query string, the way the mux hands it over.
func getAPIQuery(t *testing.T, srv *Server, name, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/_/api/"+name+"?"+rawQuery, nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.SetPathValue("path", name)
	w := httptest.NewRecorder()
	srv.handleDataAPI(w, req)
	return w
}

func TestDataAPISheetWholeRead(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	cases := sheetVectors(t)
	twoTabs := cases["sheet-read-two-tabs"]
	writeHome(t, srv, "test.htmlclay", twoTabs.HTML)

	w := getAPIQuery(t, srv, "test.htmlclay", "")
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got, want := compactBody(t, w.Body.Bytes()), vectorJSON(t, twoTabs, "expected.json"); got != want {
		t.Errorf("body = %s\nwant  = %s", got, want)
	}
}

func TestDataAPISheetIndexShape(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	cases := sheetVectors(t)
	twoTabs := cases["sheet-read-two-tabs"]
	index := cases["sheet-read-index"]
	if index.HTML != twoTabs.HTML {
		t.Fatalf("sheet-read-index's page is not the two-tab page")
	}
	writeHome(t, srv, "test.htmlclay", twoTabs.HTML)

	w := getAPIQuery(t, srv, "test.htmlclay", "shape=index")
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got, want := compactBody(t, w.Body.Bytes()), vectorJSON(t, index, "expected.json"); got != want {
		t.Errorf("body = %s\nwant  = %s", got, want)
	}
}

func TestDataAPISheetQueryRefusedBeforeTheFileIsResolved(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	cases := sheetVectors(t)
	writeHome(t, srv, "test.htmlclay", cases["sheet-read-two-tabs"].HTML)

	want := `{"error":"Invalid sheet query","message":"range appears 2 times; send it once","code":"bad_query"}`
	for _, name := range []string{"test.htmlclay", "missing.htmlclay"} {
		w := getAPIQuery(t, srv, name, "range=A1&range=B2")
		if w.Code != 400 {
			t.Fatalf("%s: status = %d: %s", name, w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != want {
			t.Errorf("%s: body = %s, want %s", name, got, want)
		}
	}
}

func TestDataAPISheetUnknownSheet(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	cases := sheetVectors(t)
	writeHome(t, srv, "test.htmlclay", cases["sheet-read-two-tabs"].HTML)

	w := getAPIQuery(t, srv, "test.htmlclay", "sheet=Nope")
	if w.Code != 400 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	body := decodeError(t, w)
	if body.Error != "Invalid sheet query" || body.Code != "unknown_sheet" {
		t.Errorf("error = %q, code = %q: %s", body.Error, body.Code, w.Body.String())
	}
}

func TestDataAPISheetVersionOneCodec(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	c := sheetVectors(t)["sheet-read-version-1-codec"]
	writeHome(t, srv, "test.htmlclay", c.HTML)

	w := getAPIQuery(t, srv, "test.htmlclay", "")
	if w.Code != 400 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	body := decodeError(t, w)
	if body.Error != "Malformed api rules tag" {
		t.Errorf("error = %q: %s", body.Error, w.Body.String())
	}
}

func TestDataAPICacheBusterIsIgnored(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	writeHome(t, srv, "test.htmlclay",
		`<html><head><title>T</title></head><body><h1>Hi</h1>`+
			`<script data-rules-name="api" data-rules-version="1">{"t":"h1"}</script></body></html>`)

	plain := getAPIQuery(t, srv, "test.htmlclay", "")
	busted := getAPIQuery(t, srv, "test.htmlclay", "v=7")
	if plain.Code != 200 {
		t.Fatalf("plain status = %d: %s", plain.Code, plain.Body.String())
	}
	if busted.Code != plain.Code {
		t.Fatalf("status = %d, want %d", busted.Code, plain.Code)
	}
	if busted.Body.String() != plain.Body.String() {
		t.Errorf("body = %s, want %s", busted.Body.String(), plain.Body.String())
	}
	if busted.Header().Get("ETag") != plain.Header().Get("ETag") {
		t.Errorf("ETag = %q, want %q", busted.Header().Get("ETag"), plain.Header().Get("ETag"))
	}
}
