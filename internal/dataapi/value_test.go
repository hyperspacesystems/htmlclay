package dataapi

import "testing"

func obj(pairs ...any) *Object {
	o := NewObject()
	for i := 0; i < len(pairs); i += 2 {
		o.Set(pairs[i].(string), pairs[i+1])
	}
	return o
}

func marshal(t *testing.T, v Value) string {
	t.Helper()
	b, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b)
}

// The ordering rule is measured, not inferred. Reference, run against node with the same
// insertion order as the case below:
//
//	Object.keys(o) → ["0","1","2","10","4294967294","b","a","00","01","4294967295"]
//
// Canonical array indices sort first ascending; everything else keeps insertion order. "00",
// "01" and "4294967295" are NOT indices: the first two do not round-trip through ToString, and
// the last is the maximum array *length*, one past the maximum index.
func TestObjectKeyOrderMatchesJS(t *testing.T) {
	o := NewObject()
	for _, k := range []string{"b", "2", "0", "a", "00", "01", "4294967294", "4294967295", "10", "1"} {
		o.Set(k, k)
	}
	want := []string{"0", "1", "2", "10", "4294967294", "b", "a", "00", "01", "4294967295"}
	got := o.Keys()
	if len(got) != len(want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Keys() = %v, want %v", got, want)
		}
	}
}

// Overwriting keeps the key where it first appeared, matching JS assignment.
func TestSetPreservesFirstInsertionPosition(t *testing.T) {
	o := obj("a", "1", "b", "2", "a", "3")
	if got := marshal(t, o); got != `{"a":"3","b":"2"}` {
		t.Errorf("Marshal = %s, want {\"a\":\"3\",\"b\":\"2\"}", got)
	}
}

// JSON.stringify does not escape < > &; Go's default encoder does. Extracted values carry HTML
// constantly, so this is not an edge case.
func TestMarshalDoesNotEscapeHTML(t *testing.T) {
	o := obj("a", `<b>&"x"</b>`)
	want := `{"a":"<b>&\"x\"</b>"}`
	if got := marshal(t, o); got != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}

func TestMarshalShapes(t *testing.T) {
	cases := []struct {
		name string
		in   Value
		want string
	}{
		{"nil", nil, "null"},
		{"string", "hi", `"hi"`},
		{"empty string", "", `""`},
		{"empty object", NewObject(), "{}"},
		{"empty list", []Value{}, "[]"},
		{"list of strings", []Value{"a", "b"}, `["a","b"]`},
		{"list with nils", []Value{nil, "a"}, `[null,"a"]`},
		{"nested", obj("a", []Value{obj("b", "c")}), `{"a":[{"b":"c"}]}`},
		{"null value", obj("a", nil), `{"a":null}`},
	}
	for _, c := range cases {
		if got := marshal(t, c.in); got != c.want {
			t.Errorf("%s: Marshal = %s, want %s", c.name, got, c.want)
		}
	}
}

// json.Encoder appends a newline to everything it writes; nothing downstream wants it, and a
// stray one would break byte comparison against the corpus.
func TestMarshalHasNoTrailingNewline(t *testing.T) {
	b, err := Marshal(obj("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if b[len(b)-1] == '\n' {
		t.Errorf("Marshal ends with a newline: %q", b)
	}
}
