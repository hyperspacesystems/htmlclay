package dataguard

import (
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
)

func provenReaderIsland(t *testing.T, raw string) dataapi.Value {
	t.Helper()
	if raw == "" {
		return nil
	}
	v, err := decodeIsland([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A program that named the exact version it read is only questioned when it
// nearly wipes the data (David, 2026-11-13).
func TestShouldFireProvenReader(t *testing.T) {
	const tags = `{"title":"keep","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc","dddddddddd","eeeeeeeeee"]}`
	const items = `{"title":"Reading list","items":[` +
		`{"id":"1","name":"alpha","note":"first note"},{"id":"2","name":"bravo","note":"second note"},` +
		`{"id":"3","name":"charlie","note":"third note"},{"id":"4","name":"delta","note":"fourth note"},` +
		`{"id":"5","name":"echo","note":"fifth note"}]}`
	for _, c := range []struct {
		name, base, inc string
		pending         bool
		provenReader    bool
		fire            bool
	}{
		{"remove one of five", tags, `{"title":"keep","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc","dddddddddd"]}`, false, true, false},
		{"remove two of five", tags, `{"title":"keep","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc"]}`, false, true, false},
		{"remove all five tags, title kept (5 of 6 values)", tags, `{"title":"keep","tags":[]}`, false, true, false},
		{"remove every value", tags, `{"title":"","tags":[]}`, false, true, true},
		{"unreadable", tags, "", false, true, true},
		{"modified value with ui work pending", tags, `{"title":"changed","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc","dddddddddd","eeeeeeeeee"]}`, true, true, false},
		{"remove one object of five", items, `{"title":"Reading list","items":[{"id":"1","name":"alpha","note":"first note"},{"id":"2","name":"bravo","note":"second note"},{"id":"3","name":"charlie","note":"third note"},{"id":"4","name":"delta","note":"fourth note"}]}`, false, true, false},
		{"remove four objects of five", items, `{"title":"Reading list","items":[{"id":"1","name":"alpha","note":"first note"}]}`, false, true, false},
		{"empty an object list, title kept", items, `{"title":"Reading list","items":[]}`, false, true, true},
		{"rewrite every object", items, `{"title":"Reading list","items":[{"id":"6","name":"foxtrot","note":"a new note"},{"id":"7","name":"golf","note":"another note"},{"id":"8","name":"hotel","note":"third new note"}]}`, false, true, false},
		{"empty a list in a stored island (bookkeeping ignored)", `{"@form":"stored","title":"keep","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc","dddddddddd","eeeeeeeeee","ffffffffff","gggggggggg","hhhhhhhhhh","iiiiiiiiii"]}`, `{"@form":"stored","title":"keep","tags":[]}`, false, true, true},
		{"exactly nine of ten values removed", `{"title":"keep","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc","dddddddddd","eeeeeeeeee","ffffffffff","gggggggggg","hhhhhhhhhh","iiiiiiiiii"]}`, `{"title":"keep","tags":[]}`, false, true, true},
		{"remove one by an unproven writer", tags, `{"title":"keep","tags":["aaaaaaaaaa","bbbbbbbbbb","cccccccccc","dddddddddd"]}`, false, false, true},
		{"remove one object by an unproven writer", items, `{"title":"Reading list","items":[{"id":"1","name":"alpha","note":"first note"},{"id":"2","name":"bravo","note":"second note"},{"id":"3","name":"charlie","note":"third note"},{"id":"4","name":"delta","note":"fourth note"}]}`, false, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			baseIsland := provenReaderIsland(t, c.base)
			inc := provenReaderIsland(t, c.inc)
			D := ClassifyDestruction(baseIsland, inc)
			if got := ShouldFire(External, c.pending, baseIsland, inc, D, c.provenReader); got != c.fire {
				t.Fatalf("fire = %v, want %v (D = %+v)", got, c.fire, *D)
			}
		})
	}
}
