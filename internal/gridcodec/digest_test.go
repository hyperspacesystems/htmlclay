package gridcodec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

type digestVectorSheet struct {
	Fixture string          `json:"fixture"`
	Sheet   int             `json:"sheet"`
	Table   string          `json:"table"`
	Digest  json.RawMessage `json:"digest"`
}

type digestVectorChange struct {
	Name          string          `json:"name"`
	Before        string          `json:"before"`
	After         string          `json:"after"`
	BeforeRow     json.RawMessage `json:"beforeRow"`
	AfterRow      json.RawMessage `json:"afterRow"`
	BeforeColumns []string        `json:"beforeColumns"`
	AfterColumns  []string        `json:"afterColumns"`
	Change        string          `json:"change"`
}

type digestVectors struct {
	Digest struct {
		FNV1a   [][]json.RawMessage  `json:"fnv1a"`
		Sheets  []digestVectorSheet  `json:"sheets"`
		Changes []digestVectorChange `json:"changes"`
	} `json:"digest"`
}

func loadDigestVectors(t *testing.T) digestVectors {
	t.Helper()
	var v digestVectors
	testutil.LoadVectors(t, "sheet-codec.json", &v)
	return v
}

func digestOfTable(t *testing.T, table string) [][]any {
	t.Helper()
	sheet, err := GridModel(table, 0)
	if err != nil {
		t.Fatalf("GridModel: %v", err)
	}
	return GridDigest(sheet)
}

func TestFNV1aVectors(t *testing.T) {
	v := loadDigestVectors(t)
	if len(v.Digest.FNV1a) == 0 {
		t.Fatal("no fnv1a vectors")
	}
	for _, pair := range v.Digest.FNV1a {
		if len(pair) != 2 {
			t.Fatalf("fnv1a vector has %d fields, want 2", len(pair))
		}
		var text, want string
		if err := json.Unmarshal(pair[0], &text); err != nil {
			t.Fatalf("decode text: %v", err)
		}
		if err := json.Unmarshal(pair[1], &want); err != nil {
			t.Fatalf("decode hex: %v", err)
		}
		if got := Hex(FNV1a(text, FNVOffset)); got != want {
			t.Errorf("FNV1a(%q) is %s, want %s", text, got, want)
		}
	}
}

func TestGridDigestVectors(t *testing.T) {
	v := loadDigestVectors(t)
	if len(v.Digest.Sheets) < 6 {
		t.Fatalf("ran %d sheet vectors, want at least 6", len(v.Digest.Sheets))
	}
	for _, entry := range v.Digest.Sheets {
		entry := entry
		name := fmt.Sprintf("%s/%d", entry.Fixture, entry.Sheet)
		t.Run(name, func(t *testing.T) {
			sheet, err := GridModel(entry.Table, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := normalized(t, TabDigest(sheet))
			want := normalize(t, entry.Digest)
			if !reflect.DeepEqual(got, want) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				t.Fatalf("digest mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestRowChangeVectors(t *testing.T) {
	v := loadDigestVectors(t)
	if len(v.Digest.Changes) != 11 {
		t.Fatalf("ran %d change vectors, want 11", len(v.Digest.Changes))
	}
	wantNames := map[string]bool{
		"overwrite": false, "clear": false, "clear-and-overwrite": false,
		"remove-filled-column": false, "remove-empty-column": false,
	}
	for _, entry := range v.Digest.Changes {
		if _, ok := wantNames[entry.Name]; ok {
			wantNames[entry.Name] = true
		}
		before := firstDigest(t, entry.Before)
		after := firstDigest(t, entry.After)

		gotBefore := normalized(t, before)
		wantBefore := normalize(t, entry.BeforeRow)
		if !reflect.DeepEqual(gotBefore, wantBefore) {
			gotJSON, _ := json.Marshal(gotBefore)
			wantJSON, _ := json.Marshal(wantBefore)
			t.Errorf("%s: beforeRow\n got: %s\nwant: %s", entry.Name, gotJSON, wantJSON)
		}
		gotAfter := normalized(t, after)
		wantAfter := normalize(t, entry.AfterRow)
		if !reflect.DeepEqual(gotAfter, wantAfter) {
			gotJSON, _ := json.Marshal(gotAfter)
			wantJSON, _ := json.Marshal(wantAfter)
			t.Errorf("%s: afterRow\n got: %s\nwant: %s", entry.Name, gotJSON, wantJSON)
		}
		if got := RowChange(before, after, entry.BeforeColumns, entry.AfterColumns); got != entry.Change {
			t.Errorf("%s: RowChange is %q, want %q", entry.Name, got, entry.Change)
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("change vectors have no %q entry", name)
		}
	}
}

func firstDigest(t *testing.T, table string) []any {
	t.Helper()
	digests := digestOfTable(t, table)
	if len(digests) == 0 {
		return nil
	}
	return digests[0]
}

func TestMassCountsUTF16AfterJSTrim(t *testing.T) {
	cols := []Col{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	row := Row{ID: "r1", Cells: []*Cell{
		{Text: " x "},
		{Text: "😀"},
		{Text: "\u0085y"},
	}}
	digest := RowDigest(row, cols)
	if digest == nil {
		t.Fatal("RowDigest is nil for a filled row")
	}
	if got := digest[2]; asInt(got) != 5 {
		t.Fatalf("mass is %v, want 5", got)
	}
}
