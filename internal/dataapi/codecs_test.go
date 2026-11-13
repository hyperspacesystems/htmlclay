package dataapi

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

type upperCodec struct{ seen *CodecContext }

func (c upperCodec) Read(d *Document, node *html.Node, ctx CodecContext) (Value, error) {
	if c.seen != nil {
		*c.seen = ctx
	}
	return strings.ToUpper(trimmedText(node)), nil
}

func (upperCodec) Write(*Document, *html.Node, Value, CodecContext) error { return nil }

func codecDoc(t *testing.T, version, rules string) (*Document, *RulesTag) {
	t.Helper()
	d, err := ParseBytes([]byte(`<html><body><h1>Hello</h1><p class="i">one</p><p class="i">two</p>` +
		`<script data-rules-name="api" data-rules-version="` + version + `">` + rules + `</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	tag, err := d.FindRulesIn("api")
	if err != nil {
		t.Fatalf("FindRulesIn: %v", err)
	}
	return d, tag
}

func TestVersion2PlainRulesReadAsVersion1(t *testing.T) {
	d, tag := codecDoc(t, "2", `{title:"h1",items:".i[]"}`)
	if tag.Version != "2" {
		t.Fatalf("Version = %q, want 2", tag.Version)
	}
	got, err := d.ExtractWith(tag.Rules, ExtractOptions{Version: tag.Version})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := Marshal(got)
	if string(out) != `{"title":"Hello","items":["one","two"]}` {
		t.Fatalf("got %s", out)
	}
}

func TestCodecRuleRefusals(t *testing.T) {
	codecs := map[string]Codec{"upper": upperCodec{}}
	cases := []struct {
		name, version, rules, want string
	}{
		{"under version 1", "1", `{shout:"h1@codec(upper)"}`, needsVersion2},
		{"nested", "2", `{post:{shout:"h1@codec(upper)"}}`, needsTopLevel},
		{"in a list shape", "2", `{posts:["p",{shout:"h1@codec(upper)"}]}`, needsTopLevel},
		{"star", "2", `{shout:"h1[*]@codec(upper)"}`, codecNoStar},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, tag := codecDoc(t, c.version, c.rules)
			_, err := d.ExtractWith(tag.Rules, ExtractOptions{Version: tag.Version, Codecs: codecs})
			var parse *RulesParseError
			if !errors.As(err, &parse) || parse.Message != c.want {
				t.Fatalf("err = %v, want RulesParseError %q", err, c.want)
			}
		})
	}

	t.Run("unknown codec", func(t *testing.T) {
		d, tag := codecDoc(t, "2", `{shout:"h1@codec(nope)"}`)
		_, err := d.ExtractWith(tag.Rules, ExtractOptions{Version: tag.Version, Codecs: codecs})
		var unknown *UnknownCodec
		if !errors.As(err, &unknown) || unknown.Name != "nope" {
			t.Fatalf("err = %v, want UnknownCodec nope", err)
		}
		if err.Error() != `unknown codec "nope": no codec by that name is given to this call` {
			t.Fatalf("message = %q", err.Error())
		}
	})

	t.Run("plain Extract treats the tree as version 1", func(t *testing.T) {
		d, tag := codecDoc(t, "2", `{shout:"h1@codec(upper)"}`)
		_, err := d.Extract(tag.Rules)
		var parse *RulesParseError
		if !errors.As(err, &parse) || parse.Message != needsVersion2 {
			t.Fatalf("err = %v, want RulesParseError %q", err, needsVersion2)
		}
	})
}

func TestCodecRuleReadsThroughTheCodec(t *testing.T) {
	var seen CodecContext
	d, tag := codecDoc(t, "2", `{title:"h1",shout:"h1@codec(upper)"}`)
	got, err := d.ExtractWith(tag.Rules, ExtractOptions{
		Version: tag.Version,
		Codecs:  map[string]Codec{"upper": upperCodec{seen: &seen}},
		Query:   "q",
		View:    "digest",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := Marshal(got)
	if string(out) != `{"title":"Hello","shout":"HELLO"}` {
		t.Fatalf("got %s", out)
	}
	if seen.Query != "q" || seen.View != "digest" {
		t.Fatalf("codec saw %+v", seen)
	}
}

func TestWriteRefusesCodecRules(t *testing.T) {
	page := func(version string) []byte {
		return []byte(`<html><body><h1>Hello</h1><script data-rules-name="api" data-rules-version="` +
			version + `">{shout:"h1@codec(upper)"}</script></body></html>`)
	}
	data, err := decodeJSON(`{"shout":"x"}`)
	if err != nil {
		t.Fatal(err)
	}

	res, err := WriteDocument(page("2"), data, "api")
	var unknown *UnknownCodec
	if !errors.As(err, &unknown) || res != nil {
		t.Fatalf("version 2: res = %v, err = %v, want UnknownCodec and no output", res, err)
	}

	res, err = WriteDocument(page("1"), data, "api")
	var parse *RulesParseError
	if !errors.As(err, &parse) || parse.Message != needsVersion2 || res != nil {
		t.Fatalf("version 1: res = %v, err = %v, want RulesParseError", res, err)
	}

	rules, err := ParseRelaxed(`{shout:"h1@codec(upper)"}`)
	if err != nil {
		t.Fatal(err)
	}
	res, err = WriteDocumentWithRules(page("1"), data, rules)
	if !errors.As(err, &parse) || parse.Message != needsVersion2 || res != nil {
		t.Fatalf("caller rules: res = %v, err = %v, want RulesParseError", res, err)
	}
}

func TestWriteThroughVersion2PlainTag(t *testing.T) {
	src := []byte(`<html><body><h1>Hello</h1><script data-rules-name="api" data-rules-version="2">{title:"h1"}</script></body></html>`)
	data, err := decodeJSON(`{"title":"Bye"}`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := WriteDocument(src, data, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.HTML), "<h1>Bye</h1>") {
		t.Fatalf("got %s", res.HTML)
	}
}
