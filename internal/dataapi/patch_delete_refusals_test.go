package dataapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

func TestPatchDeleteRefusals(t *testing.T) {
	const rules = `<script type="application/json" data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)"}</script>`
	const first = `<section item data-id="s1"><h2 bind="name">Budget</h2></section>`
	const second = `<section item data-id="s2"><h2 bind="name">Notes</h2></section>`
	base := rules + `<div items="sheets">` + first + second + `</div>`
	cases := []struct {
		name, source, patch, code, message string
		codec                              bool
	}{
		{"non-object", base, `[null]`, "bad_patch", "an operation is { sheet, range, values }, { sheet, append }, or { sheet, delete: true }", false},
		{"false", base, `[{"sheet":"Budget","delete":false}]`, "bad_patch", "delete must be exactly true", false},
		{"string", base, `[{"sheet":"Budget","delete":"true"}]`, "bad_patch", "delete must be exactly true", false},
		{"null", base, `[{"sheet":"Budget","delete":null}]`, "bad_patch", "delete must be exactly true", false},
		{"object", base, `[{"sheet":"Budget","delete":{}}]`, "bad_patch", "delete must be exactly true", false},
		{"missing-sheet", base, `[{"delete":true}]`, "bad_patch", "delete requires a sheet name or id", false},
		{"mixed-range", base, `[{"sheet":"Budget","delete":true,"range":null}]`, "bad_patch", "delete cannot include range, append, or values; send the delete on its own", false},
		{"mixed-append", base, `[{"sheet":"Budget","delete":true,"append":null}]`, "bad_patch", "delete cannot include range, append, or values; send the delete on its own", false},
		{"mixed-values", base, `[{"sheet":"Budget","delete":true,"values":null}]`, "bad_patch", "delete cannot include range, append, or values; send the delete on its own", false},
		{"name-conflict", base, `[{"sheet":"Budget","delete":true},{"sheet":"Budget","range":"A1","values":[[1]]}]`, "bad_patch", `another operation names sheet "Budget"; send the delete on its own`, false},
		{"id-conflict", base, `[{"sheet":"s1","range":"A1","values":[[1]]},{"sheet":"Budget","delete":true}]`, "bad_patch", `another operation names sheet "Budget"; send the delete on its own`, false},
		{"default-conflict", base, `[{"sheet":"Budget","delete":true},{"range":"A1","values":[[1]]}]`, "bad_patch", `another operation names sheet "Budget"; send the delete on its own`, false},
		{"default-conflict-before", base, `[{"range":"A1","values":[[1]]},{"sheet":"s1","delete":true}]`, "bad_patch", `another operation names sheet "Budget"; send the delete on its own`, false},
		{"duplicate-delete", base, `[{"sheet":"Budget","delete":true},{"sheet":"s1","delete":true}]`, "bad_patch", `another operation names sheet "Budget"; send the delete on its own`, false},
		{"last-sheet", rules + `<div items="sheets">` + first + `</div>`, `[{"sheet":"Budget","delete":true}]`, "last_sheet", "cannot delete the last sheet; keep at least one tab", false},
		{"delete-all", base, `[{"sheet":"Budget","delete":true},{"sheet":"Notes","delete":true}]`, "last_sheet", "cannot delete the last sheet; keep at least one tab", false},
		{"unknown-sheet", base, `[{"sheet":"Absent","delete":true}]`, "unknown_sheet", `no sheet named "Absent"; ask for shape=index to list them`, false},
		{"unclosed-section", strings.TrimSuffix(base, `</section></div>`), `[{"sheet":"Notes","delete":true}]`, "sheet-not-canonical", NotCanonicalMessage, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patch, err := decodeJSON(c.patch)
			if err != nil {
				t.Fatal(err)
			}
			input := []byte(c.source)
			result, err := PatchDocument(input, patch, PatchOptions{})
			if result != nil || err == nil {
				t.Fatalf("got result=%+v err=%v, want refusal %s", result, err, c.code)
			}
			if string(input) != c.source {
				t.Fatal("refused patch mutated its input")
			}
			var code, message string
			if c.codec {
				var refused *CodecRefused
				if !errors.As(err, &refused) {
					t.Fatalf("got %T, want CodecRefused", err)
				}
				code, message = refused.Code, refused.Message
			} else {
				var refused *gridcodec.QueryRefused
				if !errors.As(err, &refused) {
					t.Fatalf("got %T, want QueryRefused", err)
				}
				code, message = refused.Code, refused.Message
			}
			if code != c.code || message != c.message {
				t.Fatalf("got %s: %s, want %s: %s", code, message, c.code, c.message)
			}
		})
	}
}
