package gridcodec

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
)

type standardTab struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Rows int    `json:"rows"`
	Cols int    `json:"cols"`
}

type standardVector struct {
	F     string `json:"f"`
	Home  string `json:"home"`
	Want  string `json:"want"`
	Error string `json:"error"`
}

type standardVectorsFile struct {
	About   string           `json:"about"`
	Tabs    []standardTab    `json:"tabs"`
	Vectors []standardVector `json:"vectors"`
}

func TestStandardMatchesSapjs(t *testing.T) {
	raw, err := os.ReadFile("testdata/formula/standard.json")
	if err != nil {
		t.Fatalf("read standard.json: %v", err)
	}
	var file standardVectorsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("decode standard.json: %v", err)
	}

	sheets := make([]FrameSheet, 0, len(file.Tabs))
	for _, tab := range file.Tabs {
		rows := make([]string, tab.Rows)
		for i := range rows {
			rows[i] = strconv.Itoa(i)
		}
		cols := make([]string, tab.Cols)
		for i := range cols {
			cols[i] = strconv.Itoa(i)
		}
		sheets = append(sheets, FrameSheet{ID: tab.ID, Name: tab.Name, Rows: rows, Cols: cols})
	}
	frame := NewFrame(sheets)

	for i, v := range file.Vectors {
		got, err := Standard(v.F, v.Home, frame)
		if v.Error != "" {
			if !errors.Is(err, ErrFormulaTooDeep) {
				t.Errorf("vector %d: f=%q home=%q: want %s, got value %q with error %v", i, v.F, v.Home, v.Error, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("vector %d: f=%q home=%q: want %q, got error %v", i, v.F, v.Home, v.Want, err)
			continue
		}
		if got != v.Want {
			t.Errorf("vector %d: f=%q home=%q: want %q, got %q", i, v.F, v.Home, v.Want, got)
		}
	}
}

// A whole-column formula names rows past a million. Bug it catches: writing those rows the way Go
// prints floats ("B1e+06"), which the sheet reads as #ERROR!; JavaScript writes plain digits.
func TestStandardWritesLargeRowsInDigits(t *testing.T) {
	frame := NewFrame([]FrameSheet{{ID: "s1", Name: "Budget", Rows: []string{"0"}, Cols: []string{"0"}}})
	for f, want := range map[string]string{
		"=sum(b2:b1000000)": "=SUM(B2:B1000000)",
		"=a1048576":         "=A1048576",
		"=b999999":          "=B999999",
	} {
		got, err := Standard(f, "s1", frame)
		if err != nil || got != want {
			t.Errorf("Standard(%q) = %q, %v; want %q", f, got, err, want)
		}
	}
}

// A typo'd row of twenty digits is past every sheet. Bug it catches: converting it to an int before
// the bounds check, which wraps negative and panics the write that carries it.
func TestStandardKeepsAnImpossibleRowAsTyped(t *testing.T) {
	frame := NewFrame([]FrameSheet{{ID: "s1", Name: "Budget", Rows: []string{"0"}, Cols: []string{"0"}}})
	for _, f := range []string{"=a99999999999999999999", "=abcdefghijklmnop1"} {
		if _, err := Standard(f, "s1", frame); err != nil {
			t.Errorf("Standard(%q): %v", f, err)
		}
	}
}
