package dataapi

import (
	"errors"
	"testing"
)

// The tokenizer that finds source offsets and the HTML parser disagree inside an inline
// SVG whose <style> is never closed. A delete then has no known start for a section or a
// table, and must refuse as not canonical instead of cutting from byte 0 or panicking
// (a panic in the route would leave the document's save lock held).
func TestPatchDeleteRefusesWhenOffsetsAreUnknown(t *testing.T) {
	cases := []struct{ name, source, patch string }{
		{"unclosed-svg-style-then-closed-delete-next", "<!doctype html><html><head><script type=\"application/json\" data-rules-name=\"api\" data-rules-version=\"2\">{\"sheets\":\"[items=sheets]@codec(sheet)\"}</script></head><body><div items=\"sheets\">\n<section item data-id=\"s0\"><h2 bind=\"name\">Budget</h2><svg viewBox=\"0 0 8 8\"><style>.a{fill:red}</svg><table grid><colgroup><col data-id=\"c1\" data-pos=\"V\"></colgroup><tbody><tr data-id=\"r1\" data-pos=\"V\"><td>value</td></tr></tbody></table></section>\n<section item data-id=\"s1\"><h2 bind=\"name\">Notes</h2><svg viewBox=\"0 0 8 8\"><style>.b{fill:blue}</style></svg><table grid><colgroup><col data-id=\"c1\" data-pos=\"V\"></colgroup><tbody><tr data-id=\"r1\" data-pos=\"V\"><td>value</td></tr></tbody></table></section>\n<section item data-id=\"s2\"><h2 bind=\"name\">Archive</h2><table grid><colgroup><col data-id=\"c1\" data-pos=\"V\"></colgroup><tbody><tr data-id=\"r1\" data-pos=\"V\"><td>value</td></tr></tbody></table></section>\n</div></body></html>", "[{\"sheet\": \"Notes\", \"delete\": true}]"},
		{"unclosed-svg-style-survivor-formula", "<!doctype html><html><head><script type=\"application/json\" data-rules-name=\"api\" data-rules-version=\"2\">{\"sheets\":\"[items=sheets]@codec(sheet)\"}</script></head><body><div items=\"sheets\">\n<section item data-id=\"s0\"><h2 bind=\"name\">Budget</h2><table grid><colgroup><col data-id=\"c1\" data-pos=\"V\"></colgroup><tbody><tr data-id=\"r1\" data-pos=\"V\"><td>value</td></tr></tbody></table></section>\n<section item data-id=\"s1\"><h2 bind=\"name\">Notes</h2><svg viewBox=\"0 0 8 8\"><style>.a{fill:red}</svg><table grid><colgroup><col data-id=\"c1\" data-pos=\"V\"></colgroup><tbody><tr data-id=\"r1\" data-pos=\"V\"><td>value</td></tr></tbody></table></section>\n<section item data-id=\"s2\"><h2 bind=\"name\">Archive</h2><table grid><tbody><tr><td f=\"=Budget!A1\" v=\"7\">7</td></tr></tbody></table></section>\n</div></body></html>", "[{\"sheet\": \"Budget\", \"delete\": true}]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			patch, err := decodeJSON(c.patch)
			if err != nil {
				t.Fatal(err)
			}
			_, err = PatchDocument([]byte(c.source), patch, PatchOptions{})
			var refused *CodecRefused
			if !errors.As(err, &refused) || refused.Code != "sheet-not-canonical" {
				t.Fatalf("want sheet-not-canonical, got %v", err)
			}
		})
	}
}
