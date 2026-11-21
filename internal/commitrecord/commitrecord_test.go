package commitrecord

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type counterIDs map[string]int64

func (c counterIDs) Next(counter string) int64 {
	c[counter]++
	return c[counter]
}

func page(record string) []byte {
	return []byte(`<!doctype html><title>t</title><div items="sheets" sheet-workbook=""><section item="" data-id="s1" data-pos="V"><h2 bind="name">A</h2><table grid></table></section>
<script type="application/json" sheet-commit="">` + record + `</script></div><p>after</p>`)
}

// The Go fold writes the same record bytes ClayJS's outsideFold writes, for the vectors made from
// clayjs's own fold (testdata/fold.json, made by fold-vectors.mjs). Bug it catches: any difference
// in the ledger merge, its trim order, the epoch, the cut or the escaping, which would leave an open
// tab judging its edits against a ledger the JavaScript host would not have written.
func TestOutsideFoldMatchesClayJS(t *testing.T) {
	var file struct {
		Vectors []struct {
			Name    string           `json:"name"`
			Source  string           `json:"source"`
			Current string           `json:"current"`
			Start   map[string]int64 `json:"start"`
			Want    string           `json:"want"`
		} `json:"vectors"`
	}
	raw, err := os.ReadFile("testdata/fold.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Vectors) < 12 {
		t.Fatalf("%d vectors, want at least 12", len(file.Vectors))
	}
	for _, v := range file.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			source := page(v.Source)
			ids := counterIDs{}
			for name, start := range v.Start {
				ids[name] = start
			}
			out, err := OutsideFold(source, page(v.Current), ids)
			if err != nil {
				t.Fatal(err)
			}
			tag, ok := Find(out)
			if !ok {
				t.Fatal("no record in the folded page")
			}
			if got := string(out[tag.BodyStart:tag.BodyEnd]); got != v.Want {
				t.Fatalf("record\n got: %s\nwant: %s", got, v.Want)
			}
			if string(out[:tag.BodyStart]) != string(source[:tag.BodyStart]) || !strings.HasSuffix(string(out), "</script></div><p>after</p>") {
				t.Fatal("bytes outside the record changed")
			}
		})
	}
}

// A version from before the sheet had a record still gets one, so the open tabs see a new
// generation; a page with no workbook is written as it is. Bug it catches: a restore of an old
// version that leaves no record, after which a tab's next save passes the host's checks with a
// ledger that no longer matches the file.
func TestOutsideFoldIntoAVersionWithNoRecord(t *testing.T) {
	current := page(`{"v":2,"doc":"d","gen":"1.00000001","epoch":1,"commit":"c00000000002","parent":null,"through":0,"lineage":[],"cut":null,"floor":-1,"seen":{"a.0":[3,1]}}`)
	old := []byte(`<div items="sheets" sheet-workbook=""><section item="" data-id="s1"><div>x</div></section></div><div>after</div>`)
	out, err := OutsideFold(old, current, counterIDs{})
	if err != nil {
		t.Fatal(err)
	}
	want := `<div items="sheets" sheet-workbook=""><section item="" data-id="s1"><div>x</div></section><script type="application/json" sheet-commit="">{"v":2,"doc":"d","gen":"2.00000001","epoch":2,"commit":"c00000000001","parent":"c00000000002","through":0,"room":null,"mode":"file","lineage":[],"cut":["1.00000001",0,{}],"floor":-1,"seen":{"a.0":[3,1]}}</script></div><div>after</div>`
	if string(out) != want {
		t.Fatalf("got  %s\nwant %s", out, want)
	}
	plain := []byte(`<p>no workbook</p>`)
	if out, err := OutsideFold(plain, current, counterIDs{}); err != nil || string(out) != string(plain) {
		t.Fatalf("a page with no workbook changed: %s %v", out, err)
	}
}

// The quick check in front of the tokenizer. Bug it catches: a check that misses the attribute
// written in another letter case, which the HTML parser (and the sheet codec) still reads.
func TestFindReadsTheAttributeInAnyCase(t *testing.T) {
	doc := []byte(`<div sheet-workbook><SCRIPT TYPE="application/json" Sheet-Commit="">{}</SCRIPT></div>`)
	if !Has(doc) {
		t.Fatal("a record written in upper case was not found")
	}
	if Has([]byte(`<p data-x="sheet-commi">sheet-</p>`)) {
		t.Fatal("a document with no record was found tagged")
	}
}
