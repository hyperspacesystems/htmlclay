package dataapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

func TestIsPatchBody(t *testing.T) {
	rulesWithSheets, err := decodeJSON(`{"title":"h1","sheets":"[items=sheets]@codec(sheet)"}`)
	if err != nil {
		t.Fatal(err)
	}
	rulesWithPatch, err := decodeJSON(`{"patch":"#patch","sheets":"[items=sheets]@codec(sheet)"}`)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		body  string
		rules Value
		want  bool
	}{
		{"empty list", `{"patch":[]}`, rulesWithSheets, true},
		{"operation id beside a patch", `{"operationId":"x","patch":[{}]}`, rulesWithSheets, true},
		{"patch is not a list", `{"patch":"x"}`, rulesWithSheets, false},
		{"another top level key", `{"patch":[],"other":1}`, rulesWithSheets, false},
		{"a list body", `[]`, rulesWithSheets, false},
		{"a null body", `null`, rulesWithSheets, false},
		{"rules define patch", `{"patch":[]}`, rulesWithPatch, false},
		{"no rules", `{"patch":[]}`, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := decodeJSON(c.body)
			if err != nil {
				t.Fatalf("decode %s: %v", c.body, err)
			}
			if got := IsPatchBody(body, c.rules); got != c.want {
				t.Errorf("IsPatchBody(%s) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

func patchCaseSource(t *testing.T, name string) string {
	t.Helper()
	source, ok := readCaseFile(t, name, ".html")
	if !ok {
		t.Fatalf("%s has no .html", name)
	}
	return source
}

// TestPatchDocumentRefusesInfinity covers the one value a host can send that JSON.parse turns into
// Infinity: the sheet refuses it with a QueryRefused, not with a raw formatting error.
func TestPatchDocumentRefusesInfinity(t *testing.T) {
	source := patchCaseSource(t, "sheet-patch-range")
	patch, err := decodeJSON(`[{"sheet":"Budget","range":"B3","values":[[1e400]]}]`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = PatchDocument([]byte(source), patch, PatchOptions{})
	var refused *gridcodec.QueryRefused
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v (%T), want *gridcodec.QueryRefused", err, err)
	}
	if refused.Code != "bad_value" || refused.Message != "Infinity is not a finite number" {
		t.Errorf("refused = %q %q", refused.Code, refused.Message)
	}
}

// TestPatchDocumentRefusesNonCanonicalStartTag pins the tree route: the reference records every
// table's source offset, so an uppercase start tag scans and misses, and the tab has no region to
// patch. The Go port marks each "<table grid" instead, and a table whose start tag the parser kept
// but the scanner will not accept has no mark to find.
func TestPatchDocumentRefusesNonCanonicalStartTag(t *testing.T) {
	source := patchCaseSource(t, "sheet-patch-range")
	if !strings.Contains(source, "<table grid") {
		t.Fatalf("the case no longer holds a canonical table")
	}
	upper := strings.Replace(source, "<table grid", "<table GRID", 1)
	patch, err := decodeJSON(`[{"sheet":"Budget","range":"B3","values":[[450]]}]`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = PatchDocument([]byte(upper), patch, PatchOptions{})
	var refused *CodecRefused
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v (%T), want *CodecRefused", err, err)
	}
	if refused.Code != "sheet-not-canonical" || refused.Message != NotCanonicalMessage {
		t.Errorf("refused = %q %q", refused.Code, refused.Message)
	}
}
