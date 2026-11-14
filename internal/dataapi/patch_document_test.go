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

const patchDocumentRules = `<script type="application/json" data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)"}</script>`

const patchDocumentGrid = `<table grid><colgroup><col data-id="c1" data-pos="V"></colgroup><tbody><tr data-id="r1" data-pos="V"><td>value</td></tr></tbody></table>`

func patchDocumentTab(id, name, extra, table string) string {
	return `<section item ` + extra + ` data-id="` + id + `"><h2 bind="name">` + name + `</h2>` + table + `</section>`
}

func decodePatch(t *testing.T, raw string) Value {
	t.Helper()
	patch, err := decodeJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return patch
}

func TestPatchDocumentDeletesTabsAndReadsExcludeSeeds(t *testing.T) {
	for _, seedAttribute := range []string{"cms-template", `template=""`} {
		t.Run(seedAttribute, func(t *testing.T) {
			seed := patchDocumentTab("seed", "Seed", seedAttribute, patchDocumentGrid)
			budget := patchDocumentTab("s1", "Budget", "", patchDocumentGrid)
			notes := patchDocumentTab("s2", "Notes", "", patchDocumentGrid)
			source := patchDocumentRules + `<div items="sheets">` + seed + budget + notes + `</div>`
			want := patchDocumentRules + `<div items="sheets">` + seed + notes + `</div>`

			result, err := PatchDocument([]byte(source), decodePatch(t, `[{"sheet":"Budget","delete":true}]`), PatchOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if string(result.HTML) != want {
				t.Fatal(firstByteDifference(result.HTML, []byte(want)))
			}
			if result.Changed != (gridcodec.Changed{Sheets: 1}) || result.Stale {
				t.Fatalf("result = changed %+v, stale %v", result.Changed, result.Stale)
			}

			read, err := ReadDocument(result.HTML, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			data, ok := read.Data.(*Object)
			if !ok {
				t.Fatalf("read data = %#v, want an object", read.Data)
			}
			rawTabs, _ := data.Get("sheets")
			tabs, ok := rawTabs.([]Value)
			if !ok || len(tabs) != 1 {
				t.Fatalf("read tabs = %#v, want one non-seed tab", read.Data)
			}
			tab, ok := tabs[0].(*Object)
			name, _ := tab.Get("name")
			if !ok || name != "Notes" {
				t.Fatalf("read tab = %#v, want Notes", tabs[0])
			}
		})
	}
}

func TestPatchDocumentSeedDoesNotPreventLastSheetRefusal(t *testing.T) {
	seed := patchDocumentTab("seed", "Seed", "cms-template", patchDocumentGrid)
	budget := patchDocumentTab("s1", "Budget", "", patchDocumentGrid)
	source := patchDocumentRules + `<div items="sheets">` + seed + budget + `</div>`

	_, err := PatchDocument([]byte(source), decodePatch(t, `[{"sheet":"Budget","delete":true}]`), PatchOptions{})
	var refused *gridcodec.QueryRefused
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v (%T), want *gridcodec.QueryRefused", err, err)
	}
	if refused.Code != "last_sheet" || refused.Message != "cannot delete the last sheet; keep at least one tab" {
		t.Fatalf("refused = %q %q", refused.Code, refused.Message)
	}
}

func TestPatchDocumentDeletePreservesBOMAndNonCanonicalSurvivor(t *testing.T) {
	budget := patchDocumentTab("s1", "Budget", "", `<table grid class="wide"><tr><td><b>7</b></td></tr></table>`)
	notes := patchDocumentTab("s2", "Notes", "", `<table grid data-note='keep &amp; me'><tr><td>untouched</td></tr></table>`)
	body := patchDocumentRules + `<body data-x=value/>before<div items="sheets">` + budget + notes + `</div>after</body>`
	wantBody := patchDocumentRules + `<body data-x=value/>before<div items="sheets">` + notes + `</div>after</body>`
	source := append(append([]byte{}, utf8BOM...), []byte(body)...)
	want := append(append([]byte{}, utf8BOM...), []byte(wantBody)...)

	result, err := PatchDocument(source, decodePatch(t, `[{"sheet":"Budget","delete":true}]`), PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.HTML) != string(want) {
		t.Fatal(firstByteDifference(result.HTML, want))
	}
	if result.Changed != (gridcodec.Changed{Sheets: 1}) || result.Stale {
		t.Fatalf("result = changed %+v, stale %v", result.Changed, result.Stale)
	}
}

func TestPatchDocumentDeleteMarksFormulaSurvivorsStale(t *testing.T) {
	deleted := patchDocumentTab("s1", "Budget", "", patchDocumentGrid)
	cases := []struct {
		name      string
		start     string
		wantStart string
		formula   bool
		wantStale bool
	}{
		{"slash in unquoted value", `<table grid data-x=value/>`, `<table grid data-x=value/ stale>`, true, true},
		{"self-closing", `<table grid/>`, `<table grid stale/>`, true, true},
		{"spaced self-closing", `<table grid data-x="x" />`, `<table grid data-x="x"  stale/>`, true, true},
		{"existing stale", `<table grid stale>`, `<table grid stale>`, true, true},
		{"formula-free", `<table grid>`, `<table grid>`, false, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cell := `<td>plain</td>`
			if test.formula {
				cell = `<td f="=Budget!A1" v="7">7</td>`
			}
			table := test.start + `<tbody><tr data-id="r1" data-pos="V">` + cell + `</tr></tbody></table>`
			wantTable := test.wantStart + `<tbody><tr data-id="r1" data-pos="V">` + cell + `</tr></tbody></table>`
			notes := patchDocumentTab("s2", "Notes", "", table)
			wantNotes := patchDocumentTab("s2", "Notes", "", wantTable)
			source := patchDocumentRules + `<div items="sheets">` + deleted + notes + `</div>`
			want := patchDocumentRules + `<div items="sheets">` + wantNotes + `</div>`

			result, err := PatchDocument([]byte(source), decodePatch(t, `[{"sheet":"Budget","delete":true}]`), PatchOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if string(result.HTML) != want {
				t.Fatal(firstByteDifference(result.HTML, []byte(want)))
			}
			if result.Changed != (gridcodec.Changed{Sheets: 1}) || result.Stale != test.wantStale {
				t.Fatalf("result = changed %+v, stale %v", result.Changed, result.Stale)
			}
		})
	}
}
