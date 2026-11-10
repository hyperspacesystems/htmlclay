//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/pageadmission"
)

// The golden answers come from ClayJS's own stampPageRev, pageFormatFloor and
// withPageFormatFloor, recorded by testdata/pagestamp-golden/gen.cjs. Every case
// is replayed through the same three functions in Go and compared exactly.

type pageStampGolden struct {
	GeneratedWith map[string]any        `json:"generatedWith"`
	Cases         []pageStampGoldenCase `json:"cases"`
}

type pageStampGoldenCase struct {
	Name            string          `json:"name"`
	HTML            json.RawMessage `json:"html"`
	Rev             string          `json:"rev"`
	Floor           int             `json:"floor"`
	Stamp           json.RawMessage `json:"stamp"`
	FloorOnly       json.RawMessage `json:"floorOnly"`
	PageFormatFloor int             `json:"pageFormatFloor"`
}

func TestPageStampGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pagestamp-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden pageStampGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Cases) == 0 {
		t.Fatal("golden.json holds no cases")
	}

	for _, want := range golden.Cases {
		t.Run(want.Name, func(t *testing.T) {
			source := pageGoldenInput(t, want.HTML)
			stamped, ok := stampPageRev(source, want.Rev, want.Floor)
			if expected := pageGoldenText(t, want.Stamp); stamped != expected {
				t.Errorf("stamp = %q, want %q", stamped, expected)
			}
			var stampedGolden *string
			if err := json.Unmarshal(want.Stamp, &stampedGolden); err != nil {
				t.Fatalf("golden stamp: %v", err)
			}
			if ok != (stampedGolden != nil) {
				t.Errorf("stamp wrote = %v, want %v", ok, stampedGolden != nil)
			}
			if expected := pageGoldenText(t, want.FloorOnly); pageadmission.WithPageFormatFloor(source, want.Floor) != expected {
				t.Errorf("withPageFormatFloor = %q, want %q", pageadmission.WithPageFormatFloor(source, want.Floor), expected)
			}
			if got := pageadmission.PageFormatFloor(source); got != want.PageFormatFloor {
				t.Errorf("pageFormatFloor = %d, want %d", got, want.PageFormatFloor)
			}
		})
	}
}
