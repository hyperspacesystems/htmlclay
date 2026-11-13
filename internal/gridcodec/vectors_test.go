package gridcodec

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

type vectorRow struct {
	Row   json.RawMessage `json:"row"`
	Bytes string          `json:"bytes"`
}

type scanVector struct {
	Name    string          `json:"name"`
	Src     string          `json:"src"`
	Verdict string          `json:"verdict"`
	Table   json.RawMessage `json:"table"`
	Cols    json.RawMessage `json:"cols"`
	Rows    []vectorRow     `json:"rows"`
}

type codecVectors struct {
	Scan   []scanVector `json:"scan"`
	Values struct {
		Cells   [][]json.RawMessage `json:"cells"`
		Shown   [][]json.RawMessage `json:"shown"`
		Numbers [][]json.RawMessage `json:"numbers"`
	} `json:"values"`
}

func loadCodecVectors(t *testing.T) codecVectors {
	t.Helper()
	var v codecVectors
	testutil.LoadVectors(t, "sheet-codec.json", &v)
	return v
}

func normalize(t *testing.T, raw []byte) any {
	t.Helper()
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

func normalized(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %v: %v", value, err)
	}
	return normalize(t, raw)
}

func TestScanVectors(t *testing.T) {
	v := loadCodecVectors(t)
	accepted := 0
	for _, entry := range v.Scan {
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			var table *Table
			var cols []Col
			var rows []vectorRow
			sink := &Sink{
				Table: func(got Table) { table = &got },
				Col:   func(col Col) { cols = append(cols, col) },
				Row: func(row Row, start, end, index int) {
					if start < 0 || end > len(entry.Src) || start > end {
						t.Fatalf("row %d has range [%d, %d) outside %d bytes", index, start, end, len(entry.Src))
					}
					raw, err := json.Marshal(row)
					if err != nil {
						t.Fatalf("encode row %d: %v", index, err)
					}
					rows = append(rows, vectorRow{Row: raw, Bytes: entry.Src[start:end]})
				},
			}
			end, err := ScanGrid(entry.Src, 0, sink)
			switch entry.Verdict {
			case "miss":
				var miss *Miss
				if !errors.As(err, &miss) {
					t.Fatalf("want *Miss for verdict miss, got err=%v end=%d", err, end)
				}
				return
			case "accept":
			default:
				t.Fatalf("unknown verdict %q", entry.Verdict)
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if end != len(entry.Src) {
				t.Fatalf("end is %d, want %d", end, len(entry.Src))
			}
			if table == nil {
				t.Fatalf("no table reported")
			}
			if cols == nil {
				cols = []Col{}
			}
			if rows == nil {
				rows = []vectorRow{}
			}
			want := map[string]any{
				"table": normalize(t, entry.Table),
				"cols":  normalize(t, entry.Cols),
				"rows":  normalize(t, mustMarshal(t, entry.Rows)),
			}
			got := map[string]any{
				"table": normalized(t, *table),
				"cols":  normalized(t, cols),
				"rows":  normalized(t, rows),
			}
			if !reflect.DeepEqual(got, want) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				t.Fatalf("model mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
			accepted++
		})
	}
	if len(v.Scan) < 400 {
		t.Fatalf("ran %d scan vectors, want at least 400", len(v.Scan))
	}
	if accepted < 50 {
		t.Fatalf("accepted %d entries, want at least 50", accepted)
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %v: %v", value, err)
	}
	return raw
}

func TestValueVectors(t *testing.T) {
	v := loadCodecVectors(t)
	for _, entry := range v.Values.Cells {
		name := string(entry[0])
		if len(entry) != 3 {
			t.Fatalf("%s: want 3 fields, got %d", name, len(entry))
		}
		var cell *Cell
		if string(entry[1]) != "null" {
			cell = &Cell{}
			if err := json.Unmarshal(entry[1], cell); err != nil {
				t.Fatalf("%s: decode cell: %v", name, err)
			}
		}
		got := normalized(t, CellValue(cell))
		want := normalize(t, entry[2])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: CellValue is %v, want %v", name, got, want)
		}
	}
	for _, entry := range v.Values.Shown {
		if len(entry) != 2 {
			t.Fatalf("shown %s: want 2 fields, got %d", entry[0], len(entry))
		}
		var value any
		if err := json.Unmarshal(entry[0], &value); err != nil {
			t.Fatalf("decode value: %v", err)
		}
		var want string
		if err := json.Unmarshal(entry[1], &want); err != nil {
			t.Fatalf("decode text: %v", err)
		}
		got, err := Shown(value)
		if err != nil {
			t.Errorf("Shown(%v): %v", value, err)
			continue
		}
		if got != want {
			t.Errorf("Shown(%v) is %q, want %q", value, got, want)
		}
	}
	for _, entry := range v.Values.Numbers {
		if len(entry) != 3 {
			t.Fatalf("number %s: want 3 fields, got %d", entry[0], len(entry))
		}
		var number float64
		if err := json.Unmarshal(entry[0], &number); err != nil {
			t.Fatalf("decode number: %v", err)
		}
		var text string
		if err := json.Unmarshal(entry[1], &text); err != nil {
			t.Fatalf("decode text: %v", err)
		}
		got, err := Shown(number)
		if err != nil {
			t.Errorf("Shown(%v): %v", number, err)
			continue
		}
		if got != text {
			t.Errorf("Shown(%v) is %q, want %q", number, got, text)
		}
		typed := normalized(t, Typed(text, false))
		want := normalize(t, entry[2])
		if !reflect.DeepEqual(typed, want) {
			t.Errorf("Typed(%q) is %v, want %v", text, typed, want)
		}
	}
	if len(v.Values.Numbers) < 1000 {
		t.Fatalf("ran %d number vectors, want at least 1000", len(v.Values.Numbers))
	}
}

func TestShownRoundsTiesUp(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{1234567890123445.0, "1234567890123450"},
		{1234567890123455.0, "1234567890123460"},
		{-1234567890123455.0, "-1234567890123460"},
		{9007199254740985.0, "9007199254740990"},
		{100000000000000.5, "100000000000001"},
	}
	for _, c := range cases {
		got, err := Shown(c.value)
		if err != nil {
			t.Fatalf("Shown(%v): %v", c.value, err)
		}
		if got != c.want {
			t.Fatalf("Shown(%v) is %q, want %q", c.value, got, c.want)
		}
	}
}

func TestScanNeverPanicsOnPrefixes(t *testing.T) {
	v := loadCodecVectors(t)
	var golden string
	for _, entry := range v.Scan {
		if entry.Verdict == "accept" {
			golden = entry.Src
			break
		}
	}
	if golden == "" {
		t.Fatal("no accepted entry in the vectors")
	}
	for k := 0; k < len(golden); k++ {
		end, err := ScanGrid(golden[:k], 0, nil)
		var miss *Miss
		if !errors.As(err, &miss) {
			t.Fatalf("prefix of %d bytes: want *Miss, got err=%v end=%d", k, err, end)
		}
	}
}

func TestCutAndJoin(t *testing.T) {
	v := loadCodecVectors(t)
	var golden []string
	for _, entry := range v.Scan {
		if entry.Verdict == "accept" {
			golden = append(golden, entry.Src)
			if len(golden) == 2 {
				break
			}
		}
	}
	if len(golden) < 2 {
		t.Fatal("need two accepted entries in the vectors")
	}
	g1, g2 := golden[0], golden[1]
	doc := "<p>a</p>" + g1 + "<!-- <table grid -->" + g2 + "<table grid class=x></table>"

	marker := HoleMarker(doc)
	if marker[0] != 'h' || len(marker) != 25 {
		t.Fatalf("marker %q is not h plus 24 hex digits", marker)
	}
	if strings.Contains(doc, marker) {
		t.Fatalf("marker %q occurs in the document", marker)
	}

	rest, regions, err := CutGrids(doc, marker)
	if err != nil {
		t.Fatalf("CutGrids: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("CutGrids found %d regions, want 2", len(regions))
	}
	for n, region := range regions {
		hole := HoleHTML(marker, n)
		if !strings.Contains(rest, hole) {
			t.Fatalf("rest has no hole %d (%s): %q", n, hole, rest)
		}
		if strings.Count(rest, hole) != 1 {
			t.Fatalf("rest has hole %d %d times", n, strings.Count(rest, hole))
		}
		if doc[region.Start:region.End] != golden[n] {
			t.Fatalf("region %d is not the golden table", n)
		}
	}
	if !strings.Contains(rest, "<table grid class=x></table>") {
		t.Fatalf("rest dropped the non-canonical table: %q", rest)
	}
	if !strings.Contains(rest, "<!-- <table grid -->") {
		t.Fatalf("rest dropped the comment: %q", rest)
	}
	if strings.Count(rest, `<table grid data-hole="`+marker) != 2 {
		t.Fatalf("rest has the wrong number of holes: %q", rest)
	}

	joined, err := JoinGrids(rest, marker, doc, regions, nil)
	if err != nil {
		t.Fatalf("JoinGrids: %v", err)
	}
	if joined != doc {
		t.Fatalf("join did not restore the document:\n got %q\nwant %q", joined, doc)
	}

	replaced, err := JoinGrids(rest, marker, doc, regions, map[int]string{1: "X"})
	if err != nil {
		t.Fatalf("JoinGrids with a replacement: %v", err)
	}
	want := "<p>a</p>" + g1 + "<!-- <table grid -->" + "X" + "<table grid class=x></table>"
	if replaced != want {
		t.Fatalf("replacement join is wrong:\n got %q\nwant %q", replaced, want)
	}

	for _, bad := range []string{"bad marker!", "p"} {
		_, _, err := CutGrids(doc, bad)
		if err == nil {
			t.Fatalf("CutGrids(%q) returned no error", bad)
		}
		var miss *Miss
		if errors.As(err, &miss) {
			t.Fatalf("CutGrids(%q) returned a *Miss: %v", bad, err)
		}
	}
}
