package pageadmission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
)

const hybrid = `{"blocks":{}}`
const yjs = `{"v":1,"y":{"seed":"opaque"}}`

func tag(name, value string) string {
	return `<script type="application/json" merge="` + name + `" clay="live">` + value + `</script>`
}

func page(tags ...string) string { return `<html><body>` + strings.Join(tags, "") + `</body></html>` }

func reason(t *testing.T, err error, want string) {
	t.Helper()
	refused, ok := err.(*Refusal)
	if !ok || refused.Reason != want {
		t.Fatalf("error = %v, want refusal %q", err, want)
	}
}

func parsed(t *testing.T, source string) dataapi.Value {
	t.Helper()
	v, err := dataapi.ParseStrict(source)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestHostedPageInspectionVectors(t *testing.T) {
	t.Run("skips parsing a large ordinary document without a merge attribute", func(t *testing.T) {
		if got := Inspect(strings.Repeat("ordinary content", 10000)); got.Kind != "none" {
			t.Fatal(got)
		}
	})
	fixture, err := os.ReadFile("testdata/page-conversion-inputs.json")
	if err != nil {
		t.Fatal(err)
	}
	var inputs []struct {
		Name string              `json:"name"`
		Tags [][]json.RawMessage `json:"tags"`
	}
	if err := json.Unmarshal(fixture, &inputs); err != nil {
		t.Fatal(err)
	}
	fixtureBaselines := map[string]string{
		"single paragraph":                     "31c0ff759ab43beb1b5cc06df456d3ac2d422c2200acb52b16e3e5bffae5f0ec",
		"ordered shards":                       "dd9125330c06b801f480b09bf754b8f12f64581e36f19ff29bcc1b2a2cc03e22",
		"integer keys, unicode and tombstones": "231c723616449a68e9ac372654f81196763415e518dcd2225f6edaa6594cd7ca",
	}
	for _, input := range inputs {
		t.Run("retains raw inputs and DOM order for "+input.Name, func(t *testing.T) {
			var source []string
			for _, pair := range input.Tags {
				var name string
				if err := json.Unmarshal(pair[0], &name); err != nil {
					t.Fatal(err)
				}
				source = append(source, tag(name, string(pair[1])))
			}
			got := Inspect(page(source...))
			if got.Kind != "hybrid" || len(got.Tags) != len(input.Tags) {
				t.Fatalf("inspection = %+v", got)
			}
			for i, pair := range input.Tags {
				var name string
				json.Unmarshal(pair[0], &name)
				if got.Tags[i].Name != name {
					t.Fatalf("tag %d name = %q, want %q", i, got.Tags[i].Name, name)
				}
			}
			base, err := Baseline(got.Tags)
			if err != nil || base != fixtureBaselines[input.Name] {
				t.Fatalf("baseline = %s, %v, want %s", base, err, fixtureBaselines[input.Name])
			}
		})
	}
	cases := []struct {
		name, source, kind, why string
	}{
		{"finds an uppercase merge attribute through the prefilter", page(`<script type="application/json" MERGE="blockeditor" clay="live">` + hybrid + `</script>`), "hybrid", ""},
		{"uses the live parser for encoded attributes, MIME types and relaxed JSON", page(`<script type="application/ld&#43;json; charset=utf-8" merge="blo&#99;keditor" clay="li&#118;e">{blocks: {p: {t: 'paragraph', pos: 'A', c: 'AAAA',}}, /* ok */}</script>`), "hybrid", ""},
		{"ignores non-live, external, inert, non-JSON and unrelated tags", page(`<template>`+tag("blockeditor", hybrid)+`</template>`, `<noscript>`+tag("blockeditor", hybrid)+`</noscript>`, `<script src="data.json" type="application/json" merge="blockeditor" clay="live">{}</script>`, `<script merge="blockeditor" type="application/json">{}</script>`, `<script merge="blockeditor" clay="live" type="text/plain">{}</script>`, tag("blockeditor.extra", hybrid), tag("blockeditor.-1", hybrid), tag("counter", `{}`)), "none", ""},
		{"detects duplicate names that ordinary live parsing deliberately skips", page(tag("blockeditor", hybrid), tag("blockeditor", yjs)), "invalid", "duplicate-tag"},
		{"does not silently drop a malformed page tag", page(tag("blockeditor", hybrid), tag("blockeditor.1", `{broken`)), "invalid", "invalid-json"},
		{"does not silently drop an ephemeral page tag", page(`<script type="application/json" merge="blockeditor" clay="live ephemeral">` + hybrid + `</script>`), "invalid", "ephemeral-tag"},
		{"describes unrelated store values without claiming page content", page(tag("blockeditor", `{"title":"generic"}`)), "other", ""},
		{"accepts one page envelope and preserves its opaque bytes", page(tag("blockeditor", yjs), tag("counter", `{}`)), "yjs", ""},
		{"reports a renamed Yjs capture", page(tag("blockeditor.1", yjs)), "invalid", "invalid-yjs"},
		{"reports mixed shards", page(tag("blockeditor", yjs), tag("blockeditor.1", hybrid)), "invalid", "invalid-yjs"},
		{"reports extra Yjs fields", page(tag("blockeditor", `{"v":1,"y":{},"blocks":{}}`)), "invalid", "invalid-yjs"},
		{"reports missing Yjs part", page(tag("blockeditor", `{"v":1,"y":{"seed.0.2":"part"}}`)), "invalid", "invalid-yjs"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := Inspect(test.source)
			if got.Kind != test.kind || got.Reason != test.why {
				t.Fatalf("inspection = %+v, want %s/%s", got, test.kind, test.why)
			}
		})
	}
	for _, value := range []string{`{}`, `[]`, `17`, `{"title":"generic"}`, `{"blocks":[],"extra":true}`} {
		t.Run("describes unrelated store values without claiming page content "+value, func(t *testing.T) {
			if got := Inspect(page(tag("blockeditor", value))); got.Kind != "other" {
				t.Fatalf("inspection = %+v", got)
			}
		})
	}
	for _, value := range []string{
		`{"v":1,"y":{},"deleted":{}}`,
		`{"v":1}`,
	} {
		t.Run("reports an ambiguous or malformed Yjs capture "+value, func(t *testing.T) {
			if got := Inspect(page(tag("blockeditor", value))); got.Kind != "invalid" || got.Reason != "invalid-yjs" {
				t.Fatalf("inspection = %+v", got)
			}
		})
	}
	if got := Inspect(page(tag("blockeditor", yjs), tag("blockeditor.1", yjs))); got.Kind != "invalid" || got.Reason != "invalid-yjs" {
		t.Fatalf("reports two Yjs page tags: %+v", got)
	}
}

func TestHostedYPageStructureVectors(t *testing.T) {
	for _, test := range []struct {
		name, source      string
		complete, partial bool
	}{
		{"does not add seed, g or binary-decoding requirements", `{"v":1,"g":null,"y":{"compacted":"opaque"}}`, true, true},
		{"accepts an empty Yjs map", `{"v":1,"y":{}}`, true, true},
		{"accepts complete groups", `{"v":1,"y":{"seed.0.2":"first","seed.1.2":"second"}}`, true, true},
		{"permits partial groups only when explicitly requested", `{"v":1,"y":{"seed.0.2":"first"}}`, false, true},
		{"rejects null", `null`, false, false},
		{"rejects arrays", `[]`, false, false},
		{"rejects new version", `{"v":2,"y":{}}`, false, false},
		{"rejects array y", `{"v":1,"y":[]}`, false, false},
		{"rejects non-string update", `{"v":1,"y":{"seed":5}}`, false, false},
		{"rejects out of range part", `{"v":1,"y":{"seed.2.2":"x"}}`, false, false},
		{"rejects zero part count", `{"v":1,"y":{"seed.0.0":"x"}}`, false, false},
		{"rejects unsafe part count", `{"v":1,"y":{"seed.0.9007199254740992":"x"}}`, false, false},
		{"rejects conflicting part counts", `{"v":1,"y":{"seed.0.2":"x","seed.1.3":"x"}}`, false, false},
		{"rejects duplicate numeric indices", `{"v":1,"y":{"seed.0.1":"x","seed.00.1":"x"}}`, false, false},
		{"rejects whole and multipart alias", `{"v":1,"y":{"seed":"x","seed.0.1":"x"}}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := parsed(t, test.source)
			if got := yPageValue(value, true); got != test.complete {
				t.Errorf("complete = %v, want %v", got, test.complete)
			}
			if got := yPageValue(value, false); got != test.partial {
				t.Errorf("partial = %v, want %v", got, test.partial)
			}
		})
	}
}

func TestHostedBaselineVectors(t *testing.T) {
	value := parsed(t, `{"blocks":{"p":{"t":"paragraph","pos":"A","c":"AAAA"}}}`)
	base, err := Baseline([]Tag{{"blockeditor", value}})
	if err != nil || base != "31c0ff759ab43beb1b5cc06df456d3ac2d422c2200acb52b16e3e5bffae5f0ec" {
		t.Fatalf("matches the independently executed ClayJS baseline vector: %q, %v", base, err)
	}
	integer := parsed(t, `{"blocks":{"2":{},"10":{}}}`)
	base, err = Baseline([]Tag{{"blockeditor", integer}})
	want := sha256.Sum256([]byte(`[["blockeditor",{"blocks":{"10":{},"2":{}}}]]`))
	if err != nil || base != hex.EncodeToString(want[:]) {
		t.Fatalf("sorts integer-shaped object keys lexicographically: %q, %v", base, err)
	}
	escapes := parsed(t, `{"blocks":{"x":-0,"u":"\u2028","n":1e-6,"huge":1e21}}`)
	base, err = Baseline([]Tag{{"blockeditor", escapes}})
	if err != nil || base != "1d4c0e5210d6c19f1c585d6961fb4a93f364c7ebe895d8f65133588751c7cb02" {
		t.Fatalf("matches hosted scalar JSON formatting: %q, %v", base, err)
	}
	left, _ := Baseline([]Tag{{"blockeditor", parsed(t, `{"blocks":{"a":1,"b":2}}`)}})
	right, _ := Baseline([]Tag{{"blockeditor", parsed(t, `{"blocks":{"b":2,"a":1}}`)}})
	if left != right {
		t.Fatal("ignores object key order")
	}
	changed, _ := Baseline([]Tag{{"blockeditor", parsed(t, `{"blocks":{"a":1,"b":3}}`)}})
	if left == changed {
		t.Fatal("retains raw blocks")
	}
	for _, tags := range [][]Tag{nil, {{"counter", value}}, {{"blockeditor", value}, {"blockeditor", value}}, {{"blockeditor", parsed(t, yjs)}}, {{"blockeditor", parsed(t, `{"blocks":[]}`)}}, {{"blockeditor", parsed(t, `{"blocks":{},"extra":true}`)}}, {{"blockeditor", parsed(t, `{"blocks":{},"deleted":[]}`)}}} {
		if _, err := Baseline(tags); err == nil {
			t.Errorf("refuses a missing, ambiguous or unsupported source: %+v", tags)
		}
	}
}

func TestHostedBaselineRetainsTagNamesOrderAndTombstones(t *testing.T) {
	first := Tag{"blockeditor", parsed(t, `{"blocks":{"a":1}}`)}
	second := Tag{"blockeditor.2", parsed(t, `{"blocks":{"b":2}}`)}
	ordered, _ := Baseline([]Tag{first, second})
	reversed, _ := Baseline([]Tag{second, first})
	renamed, _ := Baseline([]Tag{{"blockeditor.1", first.Value}, second})
	deleted, _ := Baseline([]Tag{{"blockeditor", parsed(t, `{"blocks":{"a":1},"deleted":{"old":true}}`)}, second})
	if ordered == reversed || ordered == renamed || ordered == deleted {
		t.Fatal("ignores object key order but retains tag order, names, raw blocks and tombstones")
	}
}

func TestHostedOrdinaryWriteMatrix(t *testing.T) {
	empty := page(`<article>without a page</article>`)
	hybridSource := page(tag("blockeditor", hybrid))
	other := page(tag("blockeditor", `{"title":"mine"}`))
	y := page(tag("blockeditor", yjs))
	targets := []string{
		page(tag("blockeditor", `{"v":7,"n":1}`)), page(tag("blockeditor", `{"y":4}`)),
		page(tag("blockeditor", `{"v":1,"y":[]}`)), other, hybridSource, empty,
		page(tag("blockeditor", `{broken`)), page(`<template>` + y + `</template>`),
		page(`<noscript>` + y + `</noscript>`),
		page(`<script src="page.json" type="application/json" merge="blockeditor" clay="live">` + yjs + `</script>`),
		page(tag("not-page", yjs)),
	}
	for _, source := range []string{empty, hybridSource, other} {
		for i, target := range targets {
			got, err := CheckWrite(source, target, "")
			if err != nil || got != Ordinary {
				t.Errorf("generic values and parser-inert tags retain ordinary behavior (%d): %s, %v", i, got, err)
			}
		}
	}
}

func TestHostedConvertedFormatMatrix(t *testing.T) {
	current := page(tag("blockeditor", yjs))
	broken := []struct{ name, target string }{
		{"hybrid value", page(tag("blockeditor", hybrid))},
		{"mixed value", page(tag("blockeditor", `{"v":1,"y":{},"blocks":{}}`))},
		{"deleted envelope", page(tag("blockeditor", `{"y":{}}`))},
		{"new version", page(tag("blockeditor", `{"v":2,"y":{}}`))},
		{"non-string update", page(tag("blockeditor", `{"v":1,"y":{"seed":{}}}`))},
		{"incomplete multipart group", page(tag("blockeditor", `{"v":1,"y":{"u.0.2":"first"}}`))},
		{"conflicting multipart count", page(tag("blockeditor", `{"v":1,"y":{"u.0.2":"first","u.1.3":"second"}}`))},
		{"whole and multipart alias", page(tag("blockeditor", `{"v":1,"y":{"u":"whole","u.0.1":"part"}}`))},
		{"duplicate page", page(tag("blockeditor", yjs), tag("blockeditor", yjs))},
		{"page shard", page(tag("blockeditor", yjs), tag("blockeditor.1", hybrid))},
		{"renamed page", page(tag("blockeditor.1", yjs))},
		{"ephemeral page", page(`<script type="application/json" merge="blockeditor" clay="live ephemeral">` + yjs + `</script>`)},
		{"malformed JSON", page(tag("blockeditor", `{broken`))},
	}
	for _, test := range broken {
		t.Run(test.name+" refuses as a page format conflict", func(t *testing.T) {
			_, err := CheckWrite(current, test.target, "")
			if _, ok := err.(*FormatError); !ok {
				t.Fatalf("error = %v, want page format", err)
			}
		})
	}
	for _, target := range []string{
		page(`<article>after</article>`),
		page(`<template>` + tag("blockeditor", yjs) + `</template>`),
		page(`<script type="application/json" merge="blockeditor" clay="live" src="page.json">` + yjs + `</script>`),
		page(`<script type="application/json" merge="blockeditor">` + yjs + `</script>`),
	} {
		if got, err := CheckWrite(current, target, ""); err != nil || got != Ordinary {
			t.Errorf("a converted file accepts an outside edit without an eligible page tag: %s, %v", got, err)
		}
	}
	for _, target := range []string{
		page(tag("blockeditor", `{"v":1,"y":{"m.0.2":"first","m.1.2":"second"}}`)),
		page(tag("blockeditor", `{"v":1,"g":"opaque","y":{"compacted":"different update"}}`)),
		page(tag("blockeditor", `{"v":1,"y":{}}`)),
		page(tag("blockeditor", `{"v":1,"y":{"large":"`+strings.Repeat("x", 1024*1024+1)+`"}}`)),
	} {
		if got, err := CheckWrite(current, target, ""); err != nil || got != Ordinary {
			t.Fatalf("complete multipart captures, compaction and optional generation fields remain legal: %s, %v", got, err)
		}
	}
}

func TestHostedConversionFilePreconditions(t *testing.T) {
	old := page(tag("blockeditor", hybrid))
	newPage := page(tag("blockeditor", yjs))
	base, _ := Baseline(Inspect(old).Tags)
	header := "v1; source=file; base=" + base
	for _, source := range []string{page(), newPage, page(tag("blockeditor", `{"counter":1}`)), page(tag("blockeditor", hybrid), tag("blockeditor", hybrid))} {
		_, err := CheckWrite(source, newPage, header)
		reason(t, err, "invalid-source")
	}
	for _, target := range []string{page(), old, page(tag("blockeditor", `{"v":1,"y":{},"blocks":{}}`)), page(tag("blockeditor", `{"v":1,"y":{"seed.0.2":"first"}}`)), page(tag("blockeditor", yjs), tag("blockeditor.1", hybrid))} {
		_, err := CheckWrite(old, target, header)
		reason(t, err, "invalid-capture")
	}
}

func TestHostedWriteAdmissionVectors(t *testing.T) {
	empty := page(`<article>without a page</article>`)
	old := page(tag("blockeditor", hybrid))
	converted := page(tag("blockeditor", yjs))
	base, err := Baseline(Inspect(old).Tags)
	if err != nil {
		t.Fatal(err)
	}
	header := "v1; source=file; base=" + base
	for _, source := range []string{old, page(tag("blockeditor", `{"blocks":{"p":{"c":"saved"}},"deleted":{"old":true}}`))} {
		_, err := CheckWrite(source, converted, "")
		reason(t, err, "implicit-conversion")
		actual, _ := Baseline(Inspect(source).Tags)
		if got, err := CheckWrite(source, converted, "v1; source=file; base="+actual); err != nil || got != Conversion {
			t.Fatalf("blank and populated hybrid sources require an explicit conversion: %s, %v", got, err)
		}
	}
	if got, err := CheckWrite(empty, converted, ""); err != nil || got != Initial {
		t.Fatalf("initial capture = %s, %v", got, err)
	}
	for _, test := range []struct{ name, source string }{
		{"custom object", page(tag("blockeditor", `{"title":"mine"}`))},
		{"generic version", page(tag("blockeditor", `{"v":7,"n":1}`))},
		{"empty object", page(tag("blockeditor", `{}`))},
		{"duplicate source", old + old},
		{"invalid source", page(tag("blockeditor", `{broken`))},
	} {
		t.Run(test.name+" cannot silently become a Yjs page", func(t *testing.T) {
			_, err := CheckWrite(test.source, converted, "")
			reason(t, err, "invalid-source")
		})
	}
	for _, test := range []struct{ name, target string }{
		{"mixed fields", page(tag("blockeditor", `{"v":1,"y":{},"blocks":{}}`))},
		{"non-string update", page(tag("blockeditor", `{"v":1,"y":{"seed":1}}`))},
		{"partial multipart", page(tag("blockeditor", `{"v":1,"y":{"seed.0.2":"one"}}`))},
		{"renamed page", page(tag("blockeditor.1", yjs))},
		{"mixed shards", page(tag("blockeditor", yjs), tag("blockeditor.1", hybrid))},
		{"duplicate page", page(tag("blockeditor", yjs), tag("blockeditor", yjs))},
		{"broken first sibling", page(tag("blockeditor", `{broken`), tag("blockeditor.1", yjs))},
		{"broken last sibling", page(tag("blockeditor", yjs), tag("blockeditor.1", `{broken`))},
		{"duplicate before claim", page(tag("blockeditor", hybrid), tag("blockeditor", hybrid), tag("blockeditor.1", yjs))},
		{"ephemeral page", page(`<script type="application/json" merge="blockeditor" clay="live ephemeral">` + yjs + `</script>`)},
	} {
		t.Run(test.name+" refuses even when inspection stopped before the Yjs tag", func(t *testing.T) {
			for _, source := range []string{empty, old, page(tag("blockeditor", `{"title":"mine"}`))} {
				_, err := CheckWrite(source, test.target, "")
				reason(t, err, "invalid-capture")
			}
		})
	}
	for _, target := range []string{empty, page(tag("blockeditor", hybrid)), page(tag("blockeditor", `{"title":"mine"}`)), page(tag("blockeditor", `{broken`)), page(`<template>` + tag("blockeditor", yjs) + `</template>`), page(`<noscript>` + tag("blockeditor", yjs) + `</noscript>`)} {
		if got, err := CheckWrite(empty, target, ""); err != nil || got != Ordinary {
			t.Errorf("generic values and parser-inert tags retain ordinary behavior: %s, %v", got, err)
		}
	}
	claimed := page(tag("blockeditor", `{"v":1,"y":{"scroll":{"top":0}}}`))
	if got, err := CheckWrite(claimed, `<p>Changed</p>`+claimed, ""); err != nil || got != Ordinary {
		t.Fatalf("an existing reserved-shape claim permits an unrelated markup edit: %s, %v", got, err)
	}
	_, err = CheckWrite(empty, claimed, "")
	reason(t, err, "invalid-capture")
	encoded := page(`<script type="application/vnd.page+json" merge="blo&#99;keditor" clay="live">{"v":1,"y":{"seed":"opaque",},}</script>`)
	if got, err := CheckWrite(empty, encoded, ""); err != nil || got != Initial {
		t.Fatalf("encoded names and relaxed JSON use the real live-tag parser: %s, %v", got, err)
	}
	for _, target := range []string{old, page(tag("blockeditor", `{"v":1,"y":{},"blocks":{}}`)), page(tag("blockeditor", yjs), tag("blockeditor.1", hybrid))} {
		_, err := CheckWrite(converted, target, "")
		if _, ok := err.(*FormatError); !ok {
			t.Errorf("already converted files retain one complete Yjs page value: %v", err)
		}
	}
	if got, err := CheckWrite(converted, `<p>changed</p>`+converted, ""); err != nil || got != Ordinary {
		t.Fatalf("surrounding HTML edits preserve a converted page: %s, %v", got, err)
	}
	_, err = CheckWrite(converted, converted, header)
	reason(t, err, "invalid-source")
	_, err = CheckWrite(old, converted, "v1; source=file; base="+strings.Repeat("0", 64))
	reason(t, err, "source-changed")
	_, err = CheckWrite(old, old, header)
	reason(t, err, "invalid-capture")
}

func TestHostedConversionMetadataVectors(t *testing.T) {
	base := strings.Repeat("a", 64)
	for _, header := range []string{
		"v1", "v2; source=file; base=" + base, "v1; source=file", "v1; source=file; base=" + base[1:],
		"v1; source=file; base=" + strings.Repeat("A", 64), "v1; source=other; base=" + base,
		"v1; source=file; base=" + base + "; epoch=epoch-1", "v1; source=file; base=" + base + "; source=file",
		"v1; source=file; base=" + base + "; base=" + base, "v1; source=file; base=" + base + "; restore=true",
		"v1; source=file; base=" + base + ";", "v1; source=file; base=" + base + "; extra=a=b",
		"v1; source=room; base=" + base, "v1; source=room; base=" + base + "; epoch=epoch-1",
	} {
		t.Run(fmt.Sprintf("refuses malformed or ambiguous metadata %q", header), func(t *testing.T) {
			_, err := conversionBase(header)
			reason(t, err, "invalid-header")
		})
	}
	if got, err := conversionBase(" v1 ; base = " + base + " ; source = file "); err != nil || got != base {
		t.Fatalf("reads a whitespace-normalized file baseline: %q, %v", got, err)
	}
}

func TestHostedWriteAdmissionWithoutStoredPageTag(t *testing.T) {
	article := `<article class="blockeditor" data-blockeditor="1"><h1 data-id="title"></h1></article>`
	tagless := page(article)
	converted := page(tag("blockeditor", yjs))
	base := strings.Repeat("0", 64)
	for _, test := range []struct {
		name, current, next, header string
		want                        Decision
		format                      bool
	}{
		{name: "a stored page tag accepts a save that keeps it", current: converted, next: converted, want: Ordinary},
		{name: "a stored page tag admits a save without one as an outside edit", current: converted, next: tagless, want: Ordinary},
		{name: "a stored page tag still refuses a hybrid capture", current: converted, next: page(article, tag("blockeditor", hybrid)), format: true},
		{name: "a stored tagless page document continues without a new capture", current: tagless, next: converted, want: Ordinary},
		{name: "a stored tagless non-page document still needs an initial capture", current: page(), next: converted, want: Initial},
		{name: "a conversion header still requires a hybrid source", current: tagless, next: converted, header: "v1; source=file; base=" + base, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := CheckWrite(test.current, test.next, test.header)
			if test.format {
				if _, ok := err.(*FormatError); !ok {
					t.Fatalf("error = %v, want page format", err)
				}
				return
			}
			if test.header != "" {
				reason(t, err, "invalid-source")
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("decision = %s, %v, want %s", got, err, test.want)
			}
		})
	}
}

func TestStripPageTagVectors(t *testing.T) {
	head := "<!doctype html>\n<html>\n<head>\n" + tag("page.tools", "{}") + "\n</head>\n<body>\n"
	tail := "\n</body>\n</html>\n"
	article := `<article class="blockeditor" data-blockeditor="1"><h1 data-id="title"></h1></article>`
	source := func(lines ...string) string { return head + strings.Join(lines, "\n") + tail }
	fixture, err := os.ReadFile("testdata/ypage-template.html")
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.SplitAfter(string(fixture), "\n") {
		if !strings.HasPrefix(line, `<script type="application/json" merge="blockeditor" `) {
			kept = append(kept, line)
		}
	}
	for _, test := range []struct{ name, source, want string }{
		{"removes a page tag alone on its line", source(tag("blockeditor", yjs), article), source(article)},
		{"removes only the element when the line holds other markup", source(`<p>before</p>`+tag("blockeditor", yjs)+`<p>after</p>`, article), source(`<p>before</p><p>after</p>`, article)},
		{"leaves a document with no page tag untouched", source(article), source(article)},
		{"leaves a hybrid page tag untouched", source(tag("blockeditor", hybrid), article), source(tag("blockeditor", hybrid), article)},
		{"leaves a renamed hybrid page tag untouched", source(tag("blockeditor.1", hybrid), article), source(tag("blockeditor.1", hybrid), article)},
		{"leaves a page tag inside a template untouched", source(`<template>`+tag("blockeditor", yjs)+`</template>`, article), source(`<template>`+tag("blockeditor", yjs)+`</template>`, article)},
		{"leaves a page tag with two articles untouched", source(tag("blockeditor", yjs), article, article), source(tag("blockeditor", yjs), article, article)},
		{"leaves a page tag inside the article untouched", source(`<article data-blockeditor="1">` + tag("blockeditor", yjs) + `</article>`), source(`<article data-blockeditor="1">` + tag("blockeditor", yjs) + `</article>`)},
		{"removes a page tag whose value contains a closing script sequence", source(tag("blockeditor", `{"v":1,"y":{"seed":"a</script"}}`), article), source(article)},
		{"removes the page tag from the editor template fixture", string(fixture), strings.Join(kept, "")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := StripPageTag(test.source); got != test.want {
				t.Fatalf("StripPageTag =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}

func untaggedTemplate(t *testing.T) (string, string) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/untagged-page-template.html")
	if err != nil {
		t.Fatal(err)
	}
	editor, err := os.ReadFile("testdata/ypage-template.html")
	if err != nil {
		t.Fatal(err)
	}
	tagLine := ""
	for _, line := range strings.SplitAfter(string(editor), "\n") {
		if strings.HasPrefix(line, `<script type="application/json" merge="blockeditor" `) {
			tagLine = line
		}
	}
	if tagLine == "" {
		t.Fatal("testdata/ypage-template.html has no page tag")
	}
	source := string(fixture)
	at := strings.Index(source, "<article")
	if at < 0 {
		t.Fatal("testdata/untagged-page-template.html has no article")
	}
	return source, source[:at] + tagLine + source[at:]
}

func TestUntaggedTemplateInspectsAsNone(t *testing.T) {
	fixture, _ := untaggedTemplate(t)
	if got := Inspect(fixture); got.Kind != "none" {
		t.Fatalf("inspection = %+v, want none", got)
	}
	if got := StripPageTag(fixture); got != fixture {
		t.Fatalf("StripPageTag =\n%q\nwant\n%q", got, fixture)
	}
}

func TestHostedWriteAdmissionIgnoresStrayNoscriptInsideTemplate(t *testing.T) {
	article := `<article class="blockeditor" data-blockeditor="1"><h1 data-id="title"></h1></article>`
	ghost := `<article data-blockeditor="ghost"></article>`
	current := page(`<template></noscript>`+ghost+`</template>`, article)
	next := page(tag("blockeditor", yjs), article)
	if _, _, articles := locatePage(current); articles != 1 {
		t.Fatalf("counted %d articles in the stored document, want 1", articles)
	}
	decision, err := CheckWrite(current, next, "")
	if err != nil {
		t.Fatalf("error = %v, want no refusal", err)
	}
	if decision != Ordinary {
		t.Fatalf("decision = %s, want %s", decision, Ordinary)
	}
}

func TestUntaggedTemplateFirstSaveIsOrdinary(t *testing.T) {
	current, next := untaggedTemplate(t)
	decision, err := CheckWrite(current, next, "")
	if err != nil {
		t.Fatalf("error = %v, want no refusal", err)
	}
	if decision != Ordinary {
		t.Fatalf("decision = %s, want %s", decision, Ordinary)
	}
}

func TestUntaggedTemplateSaveStripsSeededTag(t *testing.T) {
	fixture, next := untaggedTemplate(t)
	if got := StripPageTag(next); got != fixture {
		t.Fatalf("StripPageTag =\n%q\nwant\n%q", got, fixture)
	}
}
