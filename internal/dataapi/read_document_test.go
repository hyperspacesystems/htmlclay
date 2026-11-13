package dataapi

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

type countingSheetCodec struct{ Codec }

func (countingSheetCodec) Read(_ *Document, node *html.Node, _ CodecContext) (Value, error) {
	cells, err := Find(node, "td", FindOpts{})
	return float64(len(cells)), err
}

func TestReadDocumentCustomCodecUsesTree(t *testing.T) {
	source := []byte(`<script data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)"}</script><div items="sheets"><section item data-id="s1"><h2 bind="name">Budget</h2><table grid><colgroup></colgroup><tbody><tr data-id="r1" data-pos="V"><td>Saved</td></tr></tbody></table></section></div>`)
	read, err := ReadDocument(source, ReadOptions{Codecs: map[string]Codec{"sheet": countingSheetCodec{SheetCodec}}})
	if err != nil {
		t.Fatal(err)
	}
	value, _ := read.Data.(*Object).Get("sheets")
	if value != float64(1) || read.Fast {
		t.Fatalf("custom codec = %+v", read)
	}
}

func TestReadDocumentCanonicalFast(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<!doctype html><html><head><script data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)"}</script></head><body><div items="sheets"><section item data-id="s1"><h2 bind="name">Budget</h2><table grid><colgroup>`)
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, `<col data-id="c%d" data-pos="V">`, i)
	}
	body.WriteString(`</colgroup><tbody>`)
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&body, `<tr data-id="r%d" data-pos="V">%s</tr>`, i, strings.Repeat(`<td>Saved cell</td>`, 40))
	}
	body.WriteString(`</tbody></table></section></div></body></html>`)
	source := []byte(body.String())
	doc, err := ParseBytes(source)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := doc.FindRulesIn("api")
	if err != nil || tag == nil {
		t.Fatalf("rules = %+v, %v", tag, err)
	}
	tree, err := doc.ExtractWith(tag.Rules, ExtractOptions{Version: tag.Version, Codecs: map[string]Codec{"sheet": SheetCodec}, View: "digest"})
	if err != nil {
		t.Fatal(err)
	}
	want, err := Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	var durations []time.Duration
	for i := 0; i < 8; i++ {
		start := time.Now()
		read, err := ReadDocument(source, ReadOptions{View: "digest"})
		elapsed := time.Since(start)
		if err != nil || !read.Fast {
			t.Fatalf("canonical byte read = %+v, %v", read, err)
		}
		got, err := Marshal(read.Data)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("canonical byte read differs from the full tree")
		}
		if i >= 3 {
			durations = append(durations, elapsed)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	t.Logf("20,000 cells median %s", durations[len(durations)/2])
}
