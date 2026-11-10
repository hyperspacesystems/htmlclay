package htmlutil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

func TestHostVectors(t *testing.T) {
	var file struct {
		Cases []struct {
			Fn                string            `json:"fn"`
			Name              string            `json:"name"`
			Args              []json.RawMessage `json:"args"`
			Expect            json.RawMessage   `json:"expect"`
			HostVariant       string            `json:"hostVariant"`
			HostVariantExpect json.RawMessage   `json:"hostVariantExpect"`
			BytesOnly         bool              `json:"bytesOnly"`
			GoExpect          json.RawMessage   `json:"goExpect"`
			GoExpectWhy       string            `json:"goExpectWhy"`
		} `json:"cases"`
	}
	testutil.LoadVectors(t, "root-attrs.json", &file)
	if len(file.Cases) == 0 {
		t.Fatal("root-attrs.json has no cases")
	}
	counts := map[string]int{}
	for _, c := range file.Cases {
		c := c
		counts[c.Fn]++
		t.Run(c.Fn+"/"+c.Name, func(t *testing.T) {
			expect := c.Expect
			if c.HostVariant == "htmlclay" {
				expect = c.HostVariantExpect
			}
			if len(c.GoExpect) > 0 {
				if c.GoExpectWhy == "" {
					t.Fatalf("case carries goExpect with no goExpectWhy")
				}
				expect = c.GoExpect
			}
			switch c.Fn {
			case "stripToken":
				if len(c.Args) != 1 {
					t.Fatalf("stripToken wants 1 arg, got %d", len(c.Args))
				}
				in := hostVectorBytes(t, c.Args[0])
				got := StripToken(in)
				want := expectedBytes(t, expect)
				if !bytes.Equal(got, want) {
					t.Errorf("StripToken(%q) = %q, want %q (JS)", in, got, want)
				}
			case "injectDocumentEtag":
				if len(c.Args) != 2 {
					t.Fatalf("injectDocumentEtag wants 2 args, got %d", len(c.Args))
				}
				in := hostVectorBytes(t, c.Args[0])
				var stamp string
				if err := json.Unmarshal(c.Args[1], &stamp); err != nil {
					t.Fatalf("decode stamp: %v", err)
				}
				got := InjectDocumentETag(in, stamp)
				want := expectedBytes(t, expect)
				if !bytes.Equal(got, want) {
					t.Errorf("InjectDocumentETag(%q, %q) = %q, want %q (JS)", in, stamp, got, want)
				}
			default:
				t.Fatalf("root-attrs.json has no Go mapping for fn %q", c.Fn)
			}
		})
	}
	for fn, n := range counts {
		t.Logf("root-attrs %s: %d cases", fn, n)
	}
}

func hostVectorBytes(t *testing.T, raw json.RawMessage) []byte {
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

func expectedBytes(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	return hostVectorBytes(t, raw)
}
