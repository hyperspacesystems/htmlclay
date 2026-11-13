package gridcodec

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
	"golang.org/x/net/html"
)

func marshalJS(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode %v: %v", v, err)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact %s: %v", raw, err)
	}
	return buf.String()
}

func requireSameJSON(t *testing.T, name, got string, want []byte) {
	t.Helper()
	compactWant := compactJSON(t, want)
	compactGot := compactJSON(t, []byte(got))
	if compactGot != compactWant {
		t.Fatalf("%s\n got: %s\nwant: %s", name, got, compactWant)
	}
}

func goldenSrc(t *testing.T, v codecVectors, name string) string {
	t.Helper()
	for _, entry := range v.Scan {
		if entry.Name == name {
			if entry.Verdict != "accept" {
				t.Fatalf("%s has verdict %q", name, entry.Verdict)
			}
			return entry.Src
		}
	}
	t.Fatalf("no scan entry named %q", name)
	return ""
}

var shapeGoldens = []string{"tiny", "merged", "formulas-formatted"}

type readShape struct {
	Tab      json.RawMessage `json:"tab"`
	Index    json.RawMessage `json:"index"`
	Values   json.RawMessage `json:"values"`
	Formulas json.RawMessage `json:"formulas"`
	Cells    json.RawMessage `json:"cells"`
}

func loadReadShapes(t *testing.T) (map[string]readShape, []json.RawMessage) {
	t.Helper()
	var vectors struct {
		ReadShapes map[string]json.RawMessage `json:"readShapes"`
	}
	testutil.LoadVectors(t, "sheet-codec.json", &vectors)
	doc := vectors.ReadShapes
	shapes := map[string]readShape{}
	for _, name := range shapeGoldens {
		var shape readShape
		if err := json.Unmarshal(doc[name], &shape); err != nil {
			t.Fatalf("decode shape %s: %v", name, err)
		}
		shapes[name] = shape
	}
	var refusals []json.RawMessage
	if err := json.Unmarshal(doc["refusals"], &refusals); err != nil {
		t.Fatalf("decode refusals: %v", err)
	}
	return shapes, refusals
}

func TestReadShapes(t *testing.T) {
	v := loadCodecVectors(t)
	shapes, _ := loadReadShapes(t)
	for _, name := range shapeGoldens {
		name := name
		t.Run(name, func(t *testing.T) {
			src := goldenSrc(t, v, "golden "+name)
			sheet, err := GridModel(src, 0)
			if err != nil {
				t.Fatalf("GridModel: %v", err)
			}
			shape := shapes[name]
			requireSameJSON(t, "tab", marshalJS(t, Tab(sheet)), shape.Tab)
			requireSameJSON(t, "index", marshalJS(t, Index(sheet)), shape.Index)
			values, err := ReadRange(sheet, "A1:C3", nil)
			if err != nil {
				t.Fatalf("ReadRange values: %v", err)
			}
			requireSameJSON(t, "values", marshalJS(t, values), shape.Values)
			formulas, err := ReadRange(sheet, "B2:D9", "formulas")
			if err != nil {
				t.Fatalf("ReadRange formulas: %v", err)
			}
			requireSameJSON(t, "formulas", marshalJS(t, formulas), shape.Formulas)
			cells, err := ReadRange(sheet, "A5:C6", "cells")
			if err != nil {
				t.Fatalf("ReadRange cells: %v", err)
			}
			requireSameJSON(t, "cells", marshalJS(t, cells), shape.Cells)
		})
	}
}

func TestRangeRefusals(t *testing.T) {
	_, refusals := loadReadShapes(t)
	if len(refusals) == 0 {
		t.Fatal("no refusals in testdata/read-shapes.json")
	}
	for i, raw := range refusals {
		var fields []any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("refusal %d: decode: %v", i, err)
		}
		if len(fields) != 4 {
			t.Fatalf("refusal %d: want 4 fields, got %d", i, len(fields))
		}
		code, _ := fields[2].(string)
		message, _ := fields[3].(string)
		_, err := ReadRange(&Sheet{}, fields[0], fields[1])
		refused, ok := err.(*QueryRefused)
		if !ok {
			t.Fatalf("refusal %d (%v): want *QueryRefused, got %v", i, fields[0], err)
		}
		if refused.Code != code || refused.Message != message {
			t.Fatalf("refusal %d (%v): got %s %q, want %s %q", i, fields[0], refused.Code, refused.Message, code, message)
		}
	}
}

func TestWriterFixpoint(t *testing.T) {
	v := loadCodecVectors(t)
	accepted := 0
	for _, entry := range v.Scan {
		if entry.Verdict != "accept" {
			continue
		}
		accepted++
		rows := scannedRows(t, entry.Src)
		for i, row := range rows {
			var model Row
			if err := json.Unmarshal(row.Row, &model); err != nil {
				t.Fatalf("%s row %d: decode: %v", entry.Name, i, err)
			}
			got, err := RowHTML(model)
			if err != nil {
				t.Fatalf("%s row %d: RowHTML: %v", entry.Name, i, err)
			}
			if got != row.Bytes {
				t.Fatalf("%s row %d: RowHTML is\n %q\nwant\n %q", entry.Name, i, got, row.Bytes)
			}
		}
	}
	if accepted < 50 {
		t.Fatalf("checked %d accepted entries, want at least 50", accepted)
	}

	goldens := 0
	for _, entry := range v.Scan {
		if entry.Verdict != "accept" || !strings.HasPrefix(entry.Name, "golden ") {
			continue
		}
		goldens++
		sheet, err := GridModel(entry.Src, 0)
		if err != nil {
			t.Fatalf("%s: GridModel: %v", entry.Name, err)
		}
		got, err := TableHTML(sheet, 50)
		if err != nil {
			t.Fatalf("%s: TableHTML: %v", entry.Name, err)
		}
		if got != entry.Src {
			t.Fatalf("%s: TableHTML is\n %q\nwant\n %q", entry.Name, got, entry.Src)
		}
	}
	if goldens != 7 {
		t.Fatalf("checked %d goldens, want 7", goldens)
	}
}

func scannedRows(t *testing.T, src string) []vectorRow {
	t.Helper()
	rows := []vectorRow{}
	sink := &Sink{
		Row: func(row Row, start, end, index int) {
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatalf("encode row %d: %v", index, err)
			}
			rows = append(rows, vectorRow{Row: raw, Bytes: src[start:end]})
		},
	}
	if _, err := ScanGrid(src, 0, sink); err != nil {
		t.Fatalf("ScanGrid: %v", err)
	}
	return rows
}

func firstTable(t *testing.T, node *html.Node) *html.Node {
	t.Helper()
	if node.Type == html.ElementNode && node.Data == "table" {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstTable(t, child); found != nil {
			return found
		}
	}
	return nil
}

func parseTable(t *testing.T, src string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader("<!doctype html><html><body>" + src + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	table := firstTable(t, doc)
	if table == nil {
		t.Fatalf("no table in %q", src)
	}
	return table
}

func TestTreeRouteMatchesByteRoute(t *testing.T) {
	v := loadCodecVectors(t)
	goldens := 0
	for _, entry := range v.Scan {
		if entry.Verdict != "accept" || !strings.HasPrefix(entry.Name, "golden ") {
			continue
		}
		goldens++
		t.Run(entry.Name, func(t *testing.T) {
			table := parseTable(t, entry.Src)
			model, err := GridModel(entry.Src, 0)
			if err != nil {
				t.Fatalf("GridModel: %v", err)
			}
			tree := TreeModel(table)
			if !reflect.DeepEqual(tree, model) {
				t.Fatalf("tree model is not the byte model\n got: %s\nwant: %s", marshalJS(t, tree), marshalJS(t, model))
			}
			got, err := CanonicalGrid(table)
			if err != nil {
				t.Fatalf("CanonicalGrid: %v", err)
			}
			if got != entry.Src {
				t.Fatalf("CanonicalGrid is\n %q\nwant\n %q", got, entry.Src)
			}
		})
	}
	if goldens != 7 {
		t.Fatalf("checked %d goldens, want 7", goldens)
	}
}

func TestTreeRouteCanonicalizesHandEdits(t *testing.T) {
	table := parseTable(t, `<table grid><tr data-id="r1" data-pos="G"><td>a</td><td> b <b>c</b></td><td></td></tr></table>`)
	got, err := CanonicalGrid(table)
	if err != nil {
		t.Fatalf("CanonicalGrid: %v", err)
	}
	want := "<table grid><colgroup></colgroup>\n<tbody>\n<tr data-id=\"r1\" data-pos=\"G\"><td>a</td><td> b c</td></tr>\n</tbody></table>"
	if got != want {
		t.Fatalf("CanonicalGrid is\n %q\nwant\n %q", got, want)
	}
	if _, err := ScanGrid(got, 0, nil); err != nil {
		t.Fatalf("ScanGrid refuses the canonical output: %v", err)
	}
}

func TestWriterRefusals(t *testing.T) {
	if _, err := CellHTML(&Cell{Text: "a\rb"}); err == nil {
		t.Fatal("CellHTML accepted a carriage return")
	}
	if _, err := CellHTML(&Cell{Text: "x", Hidden: true}); err == nil {
		t.Fatal("CellHTML accepted a hidden cell with text")
	}
	if _, err := CellHTML(&Cell{Text: "x", Colspan: 1}); err == nil {
		t.Fatal("CellHTML accepted a colspan of 1")
	}
	sheet := &Sheet{Rows: []Row{
		{ID: "r1", Pos: "1"},
		{ID: "r2", Pos: "2", Frozen: true},
	}}
	if _, err := TableHTML(sheet, 50); err == nil {
		t.Fatal("TableHTML accepted a frozen row after a data row")
	}
}
