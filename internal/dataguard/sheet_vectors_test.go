package dataguard

import (
	"encoding/json"
	"github.com/panphora/htmlclay/internal/testutil"
	"reflect"
	"testing"
)

func TestSheetGuardVectors(t *testing.T) {
	var vectors struct {
		Cases []struct {
			Name       string
			Base, Inc  json.RawMessage
			Provenance Provenance
			Pending    bool
			Lane       string
			IfMatch    *string
			D          Destruction
			Fire       bool
		}
		Metadata []struct {
			Name, Fn string
			Args     []json.RawMessage
			Expect   json.RawMessage
		}
	}
	testutil.LoadVectors(t, "sheet-guard.json", &vectors)
	if len(vectors.Cases) != 42 || len(vectors.Metadata) != 4 {
		t.Fatalf("unexpected vector counts: %d/%d", len(vectors.Cases), len(vectors.Metadata))
	}
	for _, c := range vectors.Cases {
		t.Run(c.Name, func(t *testing.T) {
			base, err := decodeIsland(c.Base)
			if err != nil {
				t.Fatal(err)
			}
			inc, err := decodeIsland(c.Inc)
			if err != nil {
				t.Fatal(err)
			}
			got := ClassifyDestruction(base, inc)
			if !reflect.DeepEqual(*got, c.D) {
				t.Fatalf("D = %+v, want %+v", *got, c.D)
			}
			matched := c.Lane == "agent" && c.IfMatch != nil && NamesExactVersion(*c.IfMatch)
			if fire := ShouldFire(c.Provenance, c.Pending && !matched, base, inc, got); fire != c.Fire {
				t.Fatalf("fire = %v, want %v", fire, c.Fire)
			}
		})
	}
	for _, c := range vectors.Metadata {
		t.Run(c.Name, func(t *testing.T) {
			args := make([]any, len(c.Args))
			for i, raw := range c.Args {
				v, err := decodeIsland(raw)
				if err != nil {
					t.Fatal(err)
				}
				args[i] = v
			}
			var got any
			switch c.Fn {
			case "islandHash":
				got = islandHash(args[0])
			case "islandsEqual":
				got = islandsEqual(args[0], args[1])
			case "isEmptyIsland":
				got = IsEmptyIsland(args[0])
			default:
				t.Fatalf("unknown function %s", c.Fn)
			}
			var want any
			if err := json.Unmarshal(c.Expect, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}
