package dataguard_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
)

func TestSheetGuardExtraction(t *testing.T) {
	raw, err := os.ReadFile("testdata/sheet-guard-extraction.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name, HTML string
			Island     json.RawMessage
			Data       json.RawMessage
		}
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Cases) != 11 {
		t.Fatalf("got %d extraction cases", len(vectors.Cases))
	}
	for _, v := range vectors.Cases {
		t.Run(v.Name, func(t *testing.T) {
			api, err := dataapi.ParseStrict(string(v.Data))
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				read, err := dataapi.ReadDocument([]byte(v.HTML), dataapi.ReadOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if !dataguard.IslandsEqual(read.Data, api) {
					got, _ := dataapi.Marshal(read.Data)
					t.Fatalf("API data mismatch: %s", got)
				}
			}
			want, err := dataapi.ParseStrict(string(v.Island))
			if err != nil {
				t.Fatal(err)
			}
			first, err := dataguard.ExtractIsland([]byte(v.HTML))
			if err != nil {
				t.Fatal(err)
			}
			second, err := dataguard.ExtractIsland([]byte(v.HTML))
			if err != nil {
				t.Fatal(err)
			}
			if !dataguard.IslandsEqual(first, want) || !dataguard.IslandsEqual(first, second) {
				got, _ := dataapi.Marshal(first)
				t.Fatalf("island mismatch or unstable extraction: %s", got)
			}
			h := newHarness(t)
			h.observe([]byte(v.HTML), []byte(v.HTML), dataguard.UIGestured)
			if event := h.observe([]byte(v.HTML), []byte(v.HTML), dataguard.External); event != nil {
				t.Fatalf("unchanged source warned: %+v", event)
			}
		})
	}
}
