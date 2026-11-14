package dataguard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
)

func typedEntry(path string, v dataapi.Value, at int64) TypedValue {
	return TypedValue{
		Path:    path,
		Digest:  stableDigest(v),
		Preview: truncatePreview(jsStringOf(v)),
		At:      at,
	}
}

func sortTyped(list []TypedValue) []TypedValue {
	out := slices.Clone(list)
	slices.SortStableFunc(out, func(a, b TypedValue) int {
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		if a.Digest != b.Digest {
			return strings.Compare(a.Digest, b.Digest)
		}
		switch {
		case a.At < b.At:
			return -1
		case a.At > b.At:
			return 1
		}
		return 0
	})
	return out
}

func typedPaths(list []TypedValue) []string {
	out := make([]string, len(list))
	for i, e := range list {
		out[i] = e.Path
	}
	return out
}

func assertTypedValues(t *testing.T, label string, got, want []TypedValue) {
	t.Helper()
	if !reflect.DeepEqual(sortTyped(got), sortTyped(want)) {
		t.Fatalf("%s:\n got %+v\nwant %+v", label, got, want)
	}
}

func TestTypedLeaves(t *testing.T) {
	for _, c := range []struct {
		name   string
		island string
		paths  []string
		digest []string
		prev   []string
	}{
		{
			"identity path from item ids",
			`{"items":[{"id":"5","name":"alpha","note":"hello"}]}`,
			[]string{"items[id:5].name", "items[id:5].note"},
			[]string{`"alpha"`, `"hello"`},
			[]string{"alpha", "hello"},
		},
		{
			"numeric item ids",
			`{"items":[{"id":1,"note":"x"}]}`,
			[]string{"items[id:1].note"},
			[]string{`"x"`},
			[]string{"x"},
		},
		{
			"item list without ids",
			`{"items":[{"note":"hello"}]}`,
			[]string{"items[*].note"},
			[]string{`"hello"`},
			[]string{"hello"},
		},
		{
			"scalar array",
			`{"tags":["alpha","bravo"]}`,
			[]string{"tags[*]", "tags[*]"},
			[]string{`"alpha"`, `"bravo"`},
			[]string{"alpha", "bravo"},
		},
		{
			"nested object under a top-level key",
			`{"profile":{"city":"Lisbon"}}`,
			[]string{"profile.city"},
			[]string{`"Lisbon"`},
			[]string{"Lisbon"},
		},
		{
			"bookkeeping keys skipped",
			`{"@form":"stored","@codecs":{"note":{"kind":"text"}},"title":"hi"}`,
			[]string{"title"},
			[]string{`"hi"`},
			[]string{"hi"},
		},
		{
			"blank strings skipped, numbers and booleans kept",
			`{"a":"","b":"   ","c":3,"d":false,"e":true}`,
			[]string{"c", "d", "e"},
			[]string{"3", "false", "true"},
			[]string{"3", "false", "true"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := typedLeaves(provenReaderIsland(t, c.island))
			if paths := typedPaths(got); !slices.Equal(paths, c.paths) {
				t.Fatalf("paths = %v, want %v", paths, c.paths)
			}
			for i, e := range got {
				if e.Digest != c.digest[i] {
					t.Fatalf("digest of %s = %q, want %q", e.Path, e.Digest, c.digest[i])
				}
				if e.Preview != c.prev[i] {
					t.Fatalf("preview of %s = %q, want %q", e.Path, e.Preview, c.prev[i])
				}
				if e.At != 0 {
					t.Fatalf("leaf %s carries time %d, want 0", e.Path, e.At)
				}
			}
		})
	}
}

func TestTypedLeavesPreviewCut(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := typedLeaves(provenReaderIsland(t, fmt.Sprintf(`{"note":%q}`, long)))
	if len(got) != 1 {
		t.Fatalf("leaves = %d, want 1", len(got))
	}
	if n := len(codeUnits(got[0].Preview)); n != previewLimit {
		t.Fatalf("preview holds %d UTF-16 units, want %d", n, previewLimit)
	}
	if !strings.HasSuffix(got[0].Preview, "\u2026") {
		t.Fatalf("preview %q has no ellipsis", got[0].Preview)
	}
}

func TestUpdateTyped(t *testing.T) {
	t.Run("first save records every leaf", func(t *testing.T) {
		inc := provenReaderIsland(t, `{"title":"hi","items":[{"id":"1","note":"a note"}],"count":3}`)
		got := UpdateTyped(nil, inc, nil, 1000)
		assertTypedValues(t, "first save", got, []TypedValue{
			typedEntry("title", "hi", 1000),
			typedEntry("items[id:1].note", "a note", 1000),
			typedEntry("count", json.Number("3"), 1000),
		})
	})

	t.Run("second save records only the new note", func(t *testing.T) {
		first := provenReaderIsland(t, `{"title":"hi"}`)
		typed := UpdateTyped(nil, first, nil, 1000)
		second := provenReaderIsland(t, `{"title":"hi","extra":"new note"}`)
		got := UpdateTyped(first, second, typed, 2000)
		assertTypedValues(t, "second save", got, []TypedValue{
			typedEntry("title", "hi", 1000),
			typedEntry("extra", "new note", 2000),
		})
	})

	t.Run("clearing a note drops it", func(t *testing.T) {
		first := provenReaderIsland(t, `{"title":"hi","note":"mine"}`)
		typed := UpdateTyped(nil, first, nil, 1000)
		second := provenReaderIsland(t, `{"title":"hi","note":""}`)
		got := UpdateTyped(first, second, typed, 2000)
		assertTypedValues(t, "cleared note", got, []TypedValue{
			typedEntry("title", "hi", 1000),
		})
	})

	t.Run("reordering a list without ids changes nothing", func(t *testing.T) {
		first := provenReaderIsland(t, `{"tags":["a","b","c"]}`)
		typed := UpdateTyped(nil, first, nil, 1000)
		second := provenReaderIsland(t, `{"tags":["c","a","b"]}`)
		got := UpdateTyped(first, second, typed, 2000)
		assertTypedValues(t, "reordered", got, []TypedValue{
			typedEntry("tags[*]", "a", 1000),
			typedEntry("tags[*]", "b", 1000),
			typedEntry("tags[*]", "c", 1000),
		})
	})

	t.Run("a duplicated value in two items counts twice", func(t *testing.T) {
		first := provenReaderIsland(t, `{"items":[{"id":"1","note":"same"}]}`)
		typed := UpdateTyped(nil, first, nil, 1000)
		second := provenReaderIsland(t, `{"items":[{"id":"1","note":"same"},{"id":"2","note":"same"}]}`)
		grown := UpdateTyped(first, second, typed, 2000)
		assertTypedValues(t, "grown", grown, []TypedValue{
			typedEntry("items[id:1].note", "same", 1000),
			typedEntry("items[id:2].note", "same", 2000),
		})
		shrunk := UpdateTyped(second, first, grown, 3000)
		assertTypedValues(t, "one leaf satisfies one entry", shrunk, []TypedValue{
			typedEntry("items[id:1].note", "same", 1000),
		})
	})

	t.Run("cap keeps the newest", func(t *testing.T) {
		n := TypedMax + 5
		fields := make([]string, n)
		typed := make([]TypedValue, n)
		for i := 0; i < n; i++ {
			value := fmt.Sprintf("v%d", i)
			fields[i] = fmt.Sprintf("%q:%q", fmt.Sprintf("f%d", i), value)
			typed[i] = typedEntry(fmt.Sprintf("f%d", i), value, 1000+int64(i))
		}
		inc := provenReaderIsland(t, "{"+strings.Join(fields, ",")+"}")
		got := UpdateTyped(nil, inc, typed, 9000)
		if len(got) != TypedMax {
			t.Fatalf("record holds %d entries, want %d", len(got), TypedMax)
		}
		kept := map[string]bool{}
		for _, e := range got {
			kept[e.Path] = true
		}
		for i := 0; i < 5; i++ {
			if kept[fmt.Sprintf("f%d", i)] {
				t.Fatalf("oldest entry f%d was kept", i)
			}
		}
		for _, path := range []string{"f5", fmt.Sprintf("f%d", n-1)} {
			if !kept[path] {
				t.Fatalf("newer entry %s was dropped", path)
			}
		}
	})
}

func TestJudgeOutside(t *testing.T) {
	const now = int64(1_000_000_000)
	recent := now - 2*60*1000
	later := now - 60*1000
	old := now - 2*60*60*1000
	items := `{"items":[{"id":"1","note":"first"},{"id":"2","note":"second"},{"id":"3","note":"third"},{"id":"4","note":"fourth"}]}`

	for _, c := range []struct {
		name   string
		base   string
		inc    string
		typed  []TypedValue
		fire   bool
		reason string
		gone   []string
	}{
		{
			name: "carried the recent answer and removed an old one",
			base: items,
			inc:  `{"items":[{"id":"1","note":"first"},{"id":"3","note":"third"},{"id":"4","note":"fourth"}]}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", recent),
				typedEntry("items[id:2].note", "second", old),
				typedEntry("items[id:3].note", "third", old),
			},
			fire: false, reason: "", gone: []string{},
		},
		{
			name: "removed the only recent answer, carried nothing typed after it",
			base: items,
			inc:  `{"items":[{"id":"2","note":"second"},{"id":"3","note":"third"},{"id":"4","note":"fourth"}]}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", recent),
				typedEntry("items[id:2].note", "second", old),
			},
			fire: true, reason: "recent", gone: []string{"items[id:1].note"},
		},
		{
			name: "removed a recent answer but carried one typed later",
			base: `{"items":[{"id":"1","note":"first"},{"id":"2","note":"second"}]}`,
			inc:  `{"items":[{"id":"2","note":"second"}]}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", recent),
				typedEntry("items[id:2].note", "second", later),
			},
			fire: false, reason: "", gone: []string{},
		},
		{
			name: "replaced a recent answer at the same identity path",
			base: `{"items":[{"id":"1","note":"first"}]}`,
			inc:  `{"items":[{"id":"1","note":"edited by the writer"}]}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", recent),
			},
			fire: false, reason: "", gone: []string{},
		},
		{
			name: "removed one old answer, carried another old one",
			base: items,
			inc:  `{"items":[{"id":"2","note":"second"},{"id":"3","note":"third"},{"id":"4","note":"fourth"}]}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", old),
				typedEntry("items[id:2].note", "second", old),
			},
			fire: false, reason: "", gone: []string{"items[id:1].note"},
		},
		{
			name: "removed all four old answers",
			base: items,
			inc:  `{"items":[]}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", old),
				typedEntry("items[id:2].note", "second", old),
				typedEntry("items[id:3].note", "third", old),
				typedEntry("items[id:4].note", "fourth", old),
			},
			fire: true, reason: "all-typed", gone: []string{
				"items[id:1].note", "items[id:2].note", "items[id:3].note", "items[id:4].note",
			},
		},
		{
			name: "two old answers removed, below the all-typed floor",
			base: `{"title":"keep","items":[{"id":"1","note":"first"},{"id":"2","note":"second"}]}`,
			inc:  `{"title":"keep"}`,
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", old),
				typedEntry("items[id:2].note", "second", old),
			},
			fire: false, reason: "", gone: []string{"items[id:1].note", "items[id:2].note"},
		},
		{
			name: "nine of ten values removed, none of them typed",
			base: `{"title":"keep","tags":["a","b","c","d","e","f","g","h","i"]}`,
			inc:  `{"title":"keep","tags":[]}`,
			fire: true, reason: "emptied", gone: []string{},
		},
		{
			name: "unreadable data",
			base: items,
			inc:  "",
			typed: []TypedValue{
				typedEntry("items[id:1].note", "first", old),
			},
			fire: true, reason: "unreadable", gone: []string{},
		},
		{
			name: "empty typing record and a small removal",
			base: `{"title":"keep","tags":["aaa","bbb","ccc"]}`,
			inc:  `{"title":"keep","tags":["aaa","bbb"]}`,
			fire: false, reason: "", gone: []string{},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			base := provenReaderIsland(t, c.base)
			inc := provenReaderIsland(t, c.inc)
			D := ClassifyDestruction(base, inc)
			got := JudgeOutside(base, inc, D, c.typed, now)
			if got.Fire != c.fire {
				t.Fatalf("fire = %v, want %v (verdict = %+v)", got.Fire, c.fire, got)
			}
			if got.Reason != c.reason {
				t.Fatalf("reason = %q, want %q (verdict = %+v)", got.Reason, c.reason, got)
			}
			if paths := slices.Sorted(slices.Values(typedPaths(got.Gone))); !slices.Equal(paths, slices.Sorted(slices.Values(c.gone))) {
				t.Fatalf("gone = %v, want %v (verdict = %+v)", paths, c.gone, got)
			}
		})
	}
}
