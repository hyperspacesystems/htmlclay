package gridcodec

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

type patchErrorVector struct {
	Type    string  `json:"type"`
	Code    *string `json:"code"`
	Message string  `json:"message"`
}

type patchVector struct {
	Name    string            `json:"name"`
	Src     string            `json:"src"`
	Ops     []any             `json:"ops"`
	Bytes   string            `json:"bytes"`
	Changed *Changed          `json:"changed"`
	Error   *patchErrorVector `json:"error"`
}

type patchVectors struct {
	Patches []patchVector `json:"patches"`
	Keys    struct {
		Between [][]json.RawMessage `json:"between"`
		Spread  [][]json.RawMessage `json:"spread"`
	} `json:"keys"`
}

func loadPatchVectors(t *testing.T) patchVectors {
	t.Helper()
	var v patchVectors
	testutil.LoadVectors(t, "sheet-codec.json", &v)
	return v
}

func keyOf(t *testing.T, raw json.RawMessage) *string {
	t.Helper()
	if string(raw) == "null" {
		return nil
	}
	var key string
	if err := json.Unmarshal(raw, &key); err != nil {
		t.Fatalf("decode key %s: %v", raw, err)
	}
	return &key
}

func TestPatchVectors(t *testing.T) {
	v := loadPatchVectors(t)
	refusals := 0
	misses := 0
	for _, entry := range v.Patches {
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			n := 0
			counter := func() string {
				n++
				return fmt.Sprintf("n%d", n)
			}
			bytes, changed, err := PatchGrid(entry.Src, entry.Ops, counter)
			if entry.Error == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if bytes != entry.Bytes {
					t.Fatalf("bytes mismatch\n got: %s\nwant: %s", bytes, entry.Bytes)
				}
				if entry.Changed == nil {
					t.Fatalf("vector has no changed counts")
				}
				if changed != *entry.Changed {
					t.Fatalf("changed is %+v, want %+v", changed, *entry.Changed)
				}
				return
			}
			if err == nil {
				t.Fatalf("want %s %q, got no error", entry.Error.Type, entry.Error.Message)
			}
			if entry.Error.Type == "CodecMiss" {
				var miss *Miss
				if !errors.As(err, &miss) {
					t.Fatalf("want *Miss, got %T: %v", err, err)
				}
				return
			}
			var refused *QueryRefused
			if !errors.As(err, &refused) {
				t.Fatalf("want *QueryRefused, got %T: %v", err, err)
			}
			if entry.Error.Code == nil {
				t.Fatalf("refusal vector has no code")
			}
			if refused.Code != *entry.Error.Code {
				t.Fatalf("code is %q, want %q", refused.Code, *entry.Error.Code)
			}
			if refused.Message != entry.Error.Message {
				t.Fatalf("message is %q, want %q", refused.Message, entry.Error.Message)
			}
		})
		switch {
		case entry.Error == nil:
		case entry.Error.Type == "CodecMiss":
			misses++
		default:
			refusals++
		}
	}
	if len(v.Patches) < 25 {
		t.Fatalf("ran %d patch vectors, want at least 25", len(v.Patches))
	}
	if refusals < 10 {
		t.Fatalf("ran %d refusals, want at least 10", refusals)
	}
	if misses < 1 {
		t.Fatalf("ran %d misses, want at least one", misses)
	}
}

func TestOrderKeyVectors(t *testing.T) {
	v := loadPatchVectors(t)
	for _, entry := range v.Keys.Between {
		if len(entry) != 3 {
			t.Fatalf("between %v: want 3 fields, got %d", entry, len(entry))
		}
		a := keyOf(t, entry[0])
		b := keyOf(t, entry[1])
		var want string
		if err := json.Unmarshal(entry[2], &want); err != nil {
			t.Fatalf("decode key: %v", err)
		}
		got, err := KeyBetween(a, b)
		if err != nil {
			t.Errorf("KeyBetween(%v, %v): %v", a, b, err)
			continue
		}
		if got != want {
			t.Errorf("KeyBetween(%v, %v) is %q, want %q", a, b, got, want)
		}
	}
	for _, entry := range v.Keys.Spread {
		if len(entry) != 4 {
			t.Fatalf("spread %v: want 4 fields, got %d", entry, len(entry))
		}
		a := keyOf(t, entry[0])
		b := keyOf(t, entry[1])
		var n int
		if err := json.Unmarshal(entry[2], &n); err != nil {
			t.Fatalf("decode n: %v", err)
		}
		var want []string
		if err := json.Unmarshal(entry[3], &want); err != nil {
			t.Fatalf("decode keys: %v", err)
		}
		got, err := KeysBetween(a, b, n)
		if err != nil {
			t.Errorf("KeysBetween(%v, %v, %d): %v", a, b, n, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("KeysBetween(%v, %v, %d) is %v, want %v", a, b, n, got, want)
		}
	}
}

func TestPatchRandomIDs(t *testing.T) {
	v := loadPatchVectors(t)
	var golden *patchVector
	for i := range v.Patches {
		if v.Patches[i].Error == nil {
			golden = &v.Patches[i]
			break
		}
	}
	if golden == nil {
		t.Fatal("no accepted entry in the vectors")
	}
	bytes, changed, err := PatchGrid(golden.Src, []any{
		map[string]any{"append": []any{[]any{"appended"}}},
	}, nil)
	if err != nil {
		t.Fatalf("PatchGrid: %v", err)
	}
	if changed.Rows != 1 {
		t.Fatalf("changed rows is %d, want 1", changed.Rows)
	}
	id := regexp.MustCompile(`<tr data-id="([^"]*)" data-pos="[^"]*"><td>appended</td></tr>`)
	match := id.FindStringSubmatch(bytes)
	if match == nil {
		t.Fatalf("no appended row in %q", bytes)
	}
	if !regexp.MustCompile(`^r[0-9a-z]{6}$`).MatchString(match[1]) {
		t.Fatalf("row id %q is not r plus 6 base-36 characters", match[1])
	}
}
