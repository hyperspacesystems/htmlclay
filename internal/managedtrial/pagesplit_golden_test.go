//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The golden answers come from ClayJS's own splitPage and composePage, recorded
// by testdata/pagesplit-golden/gen.cjs. Every case is replayed through both Go
// functions and compared exactly.

type pageSplitGolden struct {
	GeneratedWith map[string]any          `json:"generatedWith"`
	Cases         []pageSplitGoldenCase   `json:"cases"`
	ComposeOnly   []pageComposeGoldenCase `json:"composeOnly"`
}

type pageSplitGoldenCase struct {
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
	Split        pageSplitAnswer `json:"split"`
	Compose      json.RawMessage `json:"compose"`
	ComposeStale json.RawMessage `json:"composeStale"`
}

type pageSplitAnswer struct {
	File  json.RawMessage `json:"file"`
	State json.RawMessage `json:"state"`
}

type pageComposeGoldenCase struct {
	Name  string          `json:"name"`
	File  string          `json:"file"`
	State *PageRecord     `json:"state"`
	Out   json.RawMessage `json:"out"`
}

func TestPageSplitGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pagesplit-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden pageSplitGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Cases) == 0 || len(golden.ComposeOnly) == 0 {
		t.Fatal("golden.json holds no cases")
	}

	for _, want := range golden.Cases {
		t.Run(want.Name, func(t *testing.T) {
			input := pageGoldenInput(t, want.Input)
			file, state := splitPage(input)
			if expected := pageGoldenText(t, want.Split.File); file != expected {
				t.Errorf("split file = %q, want %q", file, expected)
			}
			comparePageJSON(t, "split state", state, want.Split.State)
			if expected := pageGoldenText(t, want.Compose); composePage(file, state) != expected {
				t.Errorf("compose = %q, want %q", composePage(file, state), expected)
			}
			if expected := pageGoldenText(t, want.ComposeStale); composePage(file, pageStaleRecord(state)) != expected {
				t.Errorf("composeStale = %q, want %q", composePage(file, pageStaleRecord(state)), expected)
			}
		})
	}

	for _, want := range golden.ComposeOnly {
		t.Run(want.Name, func(t *testing.T) {
			got := composePage(want.File, want.State)
			if expected := pageGoldenText(t, want.Out); got != expected {
				t.Errorf("compose = %q, want %q", got, expected)
			}
		})
	}
}

// pageGoldenInput reads a recorded input. ClayJS takes any value and hands a
// non-string straight back; the Go functions take a string, so a non-string
// input replays as the empty document it produces.
func pageGoldenInput(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return ""
}

// pageGoldenText reads a recorded document. A null document is the non-string
// input case, which replays as the empty string.
func pageGoldenText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var text *string
	if err := json.Unmarshal(raw, &text); err != nil {
		t.Fatalf("golden document: %v", err)
	}
	if text == nil {
		return ""
	}
	return *text
}

// pageStaleRecord is the generator's composePage(file, { ...state, rev: 'other' }).
func pageStaleRecord(state *PageRecord) *PageRecord {
	stale := &PageRecord{Rev: "other"}
	if state != nil {
		*stale = *state
		stale.Rev = "other"
	}
	return stale
}

func comparePageJSON(t *testing.T, field string, got any, want json.RawMessage) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%s = %s, want %s", field, gotJSON, want)
	}
}
