package specwire

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

func TestHostVectors(t *testing.T) {
	var file struct {
		Cases []struct {
			Fn          string            `json:"fn"`
			Name        string            `json:"name"`
			Args        []json.RawMessage `json:"args"`
			Expect      json.RawMessage   `json:"expect"`
			GoExpect    json.RawMessage   `json:"goExpect"`
			GoExpectWhy string            `json:"goExpectWhy"`
		} `json:"cases"`
	}
	testutil.LoadVectors(t, "spec-wire.json", &file)
	if len(file.Cases) == 0 {
		t.Fatal("spec-wire.json has no cases")
	}
	counts := map[string]int{}
	for _, c := range file.Cases {
		c := c
		counts[c.Fn]++
		t.Run(c.Fn+"/"+c.Name, func(t *testing.T) {
			expect := c.Expect
			if len(c.GoExpect) > 0 {
				if c.GoExpectWhy == "" {
					t.Fatalf("case carries goExpect with no goExpectWhy")
				}
				expect = c.GoExpect
			}
			switch c.Fn {
			case "documentEtag":
				if len(c.Args) != 1 {
					t.Fatalf("documentEtag wants 1 arg, got %d", len(c.Args))
				}
				stored := vectorBytes(t, c.Args[0])
				var want string
				if err := json.Unmarshal(expect, &want); err != nil {
					t.Fatalf("decode expect: %v", err)
				}
				if got := Etag(stored); got != want {
					t.Errorf("Etag(%q) = %q, want %q (JS)", stored, got, want)
				}
			case "ifMatchSatisfied":
				if len(c.Args) != 2 {
					t.Fatalf("ifMatchSatisfied wants 2 args, got %d", len(c.Args))
				}
				var field string
				if err := json.Unmarshal(c.Args[0], &field); err != nil {
					t.Fatalf("decode field: %v", err)
				}
				stored := vectorBytes(t, c.Args[1])
				var want bool
				if err := json.Unmarshal(expect, &want); err != nil {
					t.Fatalf("decode expect: %v", err)
				}
				if got := IfMatchSatisfied(field, stored); got != want {
					t.Errorf("IfMatchSatisfied(%q, %q) = %v, want %v (JS)", field, stored, got, want)
				}
			default:
				t.Fatalf("spec-wire.json has no Go mapping for fn %q", c.Fn)
			}
		})
	}
	for fn, n := range counts {
		t.Logf("spec-wire %s: %d cases", fn, n)
	}
}

func vectorBytes(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var obj struct {
		B64 *string `json:"$b64"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.B64 != nil {
		b, err := base64.StdEncoding.DecodeString(*obj.B64)
		if err != nil {
			t.Fatalf("decode $b64: %v", err)
		}
		return b
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode bytes: %v", err)
	}
	return []byte(s)
}
