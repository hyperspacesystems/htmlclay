package dataapi

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

// The reference-generated cases exercise parser repairs and data-region exclusions independently
// of the Go source-position algorithm. Regenerate against the JS candidate, never this port.
func TestPatchDeleteReferenceParity(t *testing.T) {
	var fixtures []struct {
		Name   string          `json:"name"`
		Source string          `json:"source"`
		Patch  json.RawMessage `json:"patch"`
		Want   map[string]any  `json:"want"`
		Error  map[string]any  `json:"error"`
	}
	raw, err := os.ReadFile("testdata/sheet-delete-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 31 {
		t.Fatalf("reference fixtures = %d, want at least 31", len(fixtures))
	}
	for _, c := range fixtures {
		t.Run(c.Name, func(t *testing.T) {
			patch, err := decodeJSON(string(c.Patch))
			if err != nil {
				t.Fatal(err)
			}
			input := []byte(c.Source)
			got, err := PatchDocument(input, patch, PatchOptions{})
			if string(input) != c.Source {
				t.Fatal("patch mutated input")
			}
			if err != nil {
				var query *gridcodec.QueryRefused
				var codec *CodecRefused
				var kind, code, message string
				switch {
				case errors.As(err, &query):
					kind, code, message = "QueryRefused", query.Code, query.Message
				case errors.As(err, &codec):
					kind, code, message = "CodecRefused", codec.Code, codec.Message
				default:
					t.Fatalf("unexpected error: %v", err)
				}
				if c.Error == nil || c.Error["type"] != kind || c.Error["code"] != code || c.Error["message"] != message {
					t.Fatalf("Go refused %s %s: %s; reference error=%v result=%v", kind, code, message, c.Error, c.Want)
				}
				return
			}
			if got == nil {
				t.Fatal("nil result without error")
			}
			if c.Error != nil {
				t.Fatalf("Go accepted; reference refused %v", c.Error)
			}
			encoded, err := json.Marshal(map[string]any{"html": string(got.HTML), "changed": got.Changed, "stale": got.Stale})
			if err != nil {
				t.Fatal(err)
			}
			var normalized map[string]any
			if err := json.Unmarshal(encoded, &normalized); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(normalized, c.Want) {
				t.Fatalf("Go=%s\nreference=%v", encoded, c.Want)
			}
		})
	}
}
