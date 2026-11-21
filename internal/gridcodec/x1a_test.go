package gridcodec

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func readX1aGolden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/x1a/" + name + ".html")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// An agent's write into a hidden row through the Go patch path keeps every
// hidden row and column hidden (hyper-html-api's patch-document test, same file).
func TestX1aPatchIntoAHiddenRowKeepsItHidden(t *testing.T) {
	var ops []any
	if err := json.Unmarshal([]byte(`[{"range":"A2","values":[["written"]]}]`), &ops); err != nil {
		t.Fatal(err)
	}
	out, _, _, err := PatchGrid(readX1aGolden(t, "hidden"), ops, func() string { return "n1" })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<tr data-id="h-r2" data-pos="G" hidden style="height:30px"><td>written</td>`,
		`<tr data-id="h-r1" data-pos="8" hidden>`,
		`<tr data-id="h-r4" data-pos="l" hidden></tr>`,
		`<col data-id="h-c0" data-pos="G" hidden style="width:90px"><col data-id="h-c1" data-pos="V" fmt="0%" hidden>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("patched bytes lost %q:\n%s", want, out)
		}
	}
}

func TestX1aGroupStartsNeverCutAMerge(t *testing.T) {
	merge := func(span int) Row { return Row{Cells: []*Cell{{Text: "m", Rowspan: span}}} }
	plain := Row{}
	cases := []struct {
		name string
		rows []Row
		want []int
	}{
		{"no rows", nil, []int{0}},
		{"no merge cuts every two rows", []Row{plain, plain, plain, plain, plain}, []int{0, 2, 4}},
		{"a merge across the cut moves it to the merge's end", []Row{plain, merge(3), plain, plain, plain}, []int{0, 4}},
		{"a merge ending at the cut leaves it", []Row{merge(2), plain, plain}, []int{0, 2}},
	}
	for _, c := range cases {
		if got := GroupStarts(c.rows, 2); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: GroupStarts = %v, want %v", c.name, got, c.want)
		}
	}
}
