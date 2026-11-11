package pageadmission

import (
	"os"
	"strings"
	"testing"
)

func oldFormatDocument(parts ...string) string {
	return "<!doctype html><html><body>" + strings.Join(parts, "") + "</body></html>"
}

func TestOldBlockeditorFormat(t *testing.T) {
	article := `<article class="page" data-page data-rev="r1"></article>`
	blockarticle := `<article class="blockeditor" data-blockeditor data-rev="r1"></article>`
	for _, test := range []struct {
		name   string
		source string
		want   bool
	}{
		{"a page store holding a Yjs envelope", oldFormatDocument(tag("page", `{"v":1,"y":{}}`), article), true},
		{"a renamed page store holding a Yjs envelope", oldFormatDocument(tag("page.2", `{"v":1}`)), true},
		{"a page store holding only y", oldFormatDocument(tag("page", `{"y":{}}`)), true},
		{"an author store named page", oldFormatDocument(tag("page", `{"current":3}`)), false},
		{"a bare pagination article", oldFormatDocument(`<article data-page="2">Chapter two</article>`), false},
		{"the new block editor name", oldFormatDocument(tag("blockeditor", `{"v":1,"y":{}}`), blockarticle), false},
		{"a store named pages", oldFormatDocument(tag("pages", `{"v":1,"y":{}}`)), false},
		{"no tags", oldFormatDocument(`<p>no tags</p>`), false},
		{"empty source", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := OldBlockeditorFormat(test.source); got != test.want {
				t.Fatalf("OldBlockeditorFormat = %v, want %v for %q", got, test.want, test.source)
			}
		})
	}
	if OldFormatMessage != "This document now uses the new Block editor format, so this change was not saved. Copy any text you need from this tab, then reload." {
		t.Fatalf("OldFormatMessage = %q", OldFormatMessage)
	}
	if OldFormatCode != "blockeditor-old-format" {
		t.Fatalf("OldFormatCode = %q", OldFormatCode)
	}
}

func TestBlockeditorTemplateRoundTrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/ypage-template.html")
	if err != nil {
		t.Fatal(err)
	}
	fixture := string(raw)
	if got := Inspect(fixture); got.Kind != "yjs" {
		t.Fatalf("inspection = %+v, want yjs", got)
	}
	pageTag, ok := PageTag(fixture)
	if !ok || pageTag == "" {
		t.Fatalf("PageTag = %q, %v", pageTag, ok)
	}
	stripped := StripPageTag(fixture)
	if stripped == fixture || strings.Contains(stripped, `merge="blockeditor" `) {
		t.Fatalf("StripPageTag left the page tag: %q", stripped)
	}
	composed := InsertPageTag(stripped, pageTag)
	if !strings.Contains(composed, pageTag+"\n<article class=\"blockeditor\"") {
		t.Fatalf("InsertPageTag did not put the tag back before the article:\n%q", composed)
	}
	if again := InsertPageTag(StripPageTag(composed), pageTag); again != composed {
		t.Fatalf("a composed template does not round-trip byte for byte:\n%q\nwant\n%q", again, composed)
	}
	if got, ok := PageTag(composed); !ok || got != pageTag {
		t.Fatalf("composed PageTag = %q, %v, want %q", got, ok, pageTag)
	}
	if got := Inspect(composed); got.Kind != "yjs" {
		t.Fatalf("composed inspection = %+v, want yjs", got)
	}
	if got := StripPageTag(composed); got != stripped {
		t.Fatalf("composed strips back to\n%q\nwant\n%q", got, stripped)
	}
}
