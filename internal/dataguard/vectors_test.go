package dataguard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
)

type vectorCase struct {
	Fn     string
	Name   string
	Args   []dataapi.Value
	Expect json.RawMessage
}

type vectorFile struct {
	Constants map[string]dataapi.Value
	Cases     []vectorCase
}

var vectorRunners = map[string]func(args []dataapi.Value) dataapi.Value{
	"stableDigest": func(args []dataapi.Value) dataapi.Value { return stableDigest(args[0]) },
	"islandHash":   func(args []dataapi.Value) dataapi.Value { return islandHash(args[0]) },
	"islandsEqual": func(args []dataapi.Value) dataapi.Value { return islandsEqual(args[0], args[1]) },
	"meaningful":   func(args []dataapi.Value) dataapi.Value { return Meaningful(args[0]) },
	"mass":         func(args []dataapi.Value) dataapi.Value { return Mass(args[0]) },
	"isPlaceholder": func(args []dataapi.Value) dataapi.Value {
		return IsPlaceholder(args[0])
	},
	"isEmptyIsland": func(args []dataapi.Value) dataapi.Value { return IsEmptyIsland(args[0]) },
	"diceCoefficient": func(args []dataapi.Value) dataapi.Value {
		a, _ := args[0].(string)
		b, _ := args[1].(string)
		return DiceCoefficient(a, b)
	},
	"itemSimilarity": func(args []dataapi.Value) dataapi.Value {
		return ItemSimilarity(args[0], args[1])
	},
	"classifyDestruction": func(args []dataapi.Value) dataapi.Value {
		return ClassifyDestruction(args[0], args[1])
	},
	"shouldFire": func(args []dataapi.Value) dataapi.Value {
		prov, _ := args[0].(string)
		pending, _ := args[1].(bool)
		D := ClassifyDestruction(args[2], args[3])
		return ShouldFire(Provenance(prov), pending, args[2], args[3], D, false)
	},
	"uiProvenance": func(args []dataapi.Value) dataapi.Value {
		var userDriven *bool
		if b, ok := args[0].(bool); ok {
			userDriven = &b
		}
		return string(UIProvenanceOf(userDriven))
	},
	"lossSummary": func(args []dataapi.Value) dataapi.Value {
		prov, _ := args[0].(string)
		return LossSummaryOf(Provenance(prov), ClassifyDestruction(args[1], args[2]))
	},
	"droppedItems": func(args []dataapi.Value) dataapi.Value {
		return DroppedItems(args[0], args[1])
	},
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func vectorsPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("DATAGUARD_VECTORS"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		t.Fatalf("DATAGUARD_VECTORS=%s does not exist", p)
	}
	p := filepath.Join(repoRoot(), "node_modules", "@panphora", "host-conformance", "vectors", "data-loss.json")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	t.Fatalf("vectors not found; run npm ci in the htmlclay root, or set DATAGUARD_VECTORS")
	return ""
}

// unwrapOrdered turns the vector file's ordered-object encoding into a plain
// object: {"$o":[["k",v],...]} is a JSON object whose key order the vectors
// depend on. Only the test loader knows that spelling. The production decoder
// reads every key literally, so a document field named $o keeps its guard rather
// than being mistaken for this encoding.
func unwrapOrdered(v dataapi.Value) dataapi.Value {
	switch val := v.(type) {
	case []dataapi.Value:
		for i := range val {
			val[i] = unwrapOrdered(val[i])
		}
		return val
	case *dataapi.Object:
		keys := val.Keys()
		if len(keys) == 1 && keys[0] == "$o" {
			if pairs, ok := unwrapOrderedPairs(val); ok {
				return pairs
			}
		}
		for _, k := range keys {
			item, _ := val.Get(k)
			val.Define(k, unwrapOrdered(item))
		}
		return val
	}
	return v
}

func unwrapOrderedPairs(obj *dataapi.Object) (*dataapi.Object, bool) {
	raw, _ := obj.Get("$o")
	list, ok := raw.([]dataapi.Value)
	if !ok {
		return nil, false
	}
	out := dataapi.NewObject()
	for _, p := range list {
		pair, ok := p.([]dataapi.Value)
		if !ok || len(pair) != 2 {
			return nil, false
		}
		k, ok := pair[0].(string)
		if !ok {
			return nil, false
		}
		out.Define(k, unwrapOrdered(pair[1]))
	}
	return out, true
}

func loadVectors(t *testing.T) *vectorFile {
	t.Helper()
	f, err := os.Open(vectorsPath(t))
	if err != nil {
		t.Fatalf("open vectors: %v", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if err := expectDelim(dec, '{'); err != nil {
		t.Fatalf("vectors: %v", err)
	}
	out := &vectorFile{Constants: map[string]dataapi.Value{}}
	for dec.More() {
		key, err := keyToken(dec)
		if err != nil {
			t.Fatalf("vectors: %v", err)
		}
		switch key {
		case "constants":
			obj, err := decodeObject(dec)
			if err != nil {
				t.Fatalf("vectors constants: %v", err)
			}
			for _, k := range obj.Keys() {
				v, _ := obj.Get(k)
				out.Constants[k] = unwrapOrdered(v)
			}
		case "cases":
			if err := expectDelim(dec, '['); err != nil {
				t.Fatalf("vectors cases: %v", err)
			}
			for dec.More() {
				c, err := decodeCase(dec)
				if err != nil {
					t.Fatalf("vectors case: %v", err)
				}
				out.Cases = append(out.Cases, c)
			}
			if err := expectDelim(dec, ']'); err != nil {
				t.Fatalf("vectors cases: %v", err)
			}
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				t.Fatalf("vectors %q: %v", key, err)
			}
		}
	}
	if err := expectDelim(dec, '}'); err != nil {
		t.Fatalf("vectors: %v", err)
	}
	return out
}

func decodeCase(dec *json.Decoder) (vectorCase, error) {
	var c vectorCase
	if err := expectDelim(dec, '{'); err != nil {
		return c, err
	}
	for dec.More() {
		key, err := keyToken(dec)
		if err != nil {
			return c, err
		}
		switch key {
		case "fn":
			if err := dec.Decode(&c.Fn); err != nil {
				return c, err
			}
		case "name":
			if err := dec.Decode(&c.Name); err != nil {
				return c, err
			}
		case "expect":
			if err := dec.Decode(&c.Expect); err != nil {
				return c, err
			}
		case "args":
			v, err := decodeValue(dec)
			if err != nil {
				return c, err
			}
			list, ok := unwrapOrdered(v).([]dataapi.Value)
			if !ok {
				return c, fmt.Errorf("args of %s is not an array", c.Name)
			}
			c.Args = list
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return c, err
			}
		}
	}
	return c, expectDelim(dec, '}')
}

func TestVectors(t *testing.T) {
	vf := loadVectors(t)
	ran := map[string]int{}
	run := 0
	for _, c := range vf.Cases {
		runner, ok := vectorRunners[c.Fn]
		if !ok {
			t.Errorf("%s/%s: no runner for function %q", c.Fn, c.Name, c.Fn)
			continue
		}
		ran[c.Fn]++
		run++
		c := c
		t.Run(c.Fn+"/"+c.Name, func(t *testing.T) {
			var want any
			if err := json.Unmarshal(c.Expect, &want); err != nil {
				t.Fatalf("decode expect %s: %v", c.Expect, err)
			}
			got := runner(c.Args)
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			var gotAny any
			if err := json.Unmarshal(raw, &gotAny); err != nil {
				t.Fatalf("decode result %s: %v", raw, err)
			}
			if !reflect.DeepEqual(want, gotAny) {
				t.Fatalf("got %s, want %s", raw, c.Expect)
			}
		})
	}
	t.Logf("ran %d of %d cases", run, len(vf.Cases))
	for fn := range vectorRunners {
		if ran[fn] == 0 {
			t.Errorf("%s: no cases ran", fn)
		}
	}
}
