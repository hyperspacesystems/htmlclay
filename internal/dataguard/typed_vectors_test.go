package dataguard

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/testutil"
)

func TestTypedGuardVectors(t *testing.T) {
	var vectors struct {
		Now   int64
		Cases []struct {
			Name        string
			Op          string
			Prev        json.RawMessage
			Base        json.RawMessage
			Inc         json.RawMessage
			Island      json.RawMessage
			Current     json.RawMessage
			Recoverable json.RawMessage
			Typed       []TypedValue
			Gone        []TypedValue
			Now         int64
			Want        json.RawMessage
		}
	}
	testutil.LoadVectors(t, "typed-guard.json", &vectors)
	if len(vectors.Cases) != 33 {
		t.Fatalf("unexpected vector count: %d", len(vectors.Cases))
	}
	for _, c := range vectors.Cases {
		t.Run(c.Name, func(t *testing.T) {
			now := c.Now
			if now == 0 {
				now = vectors.Now
			}
			switch c.Op {
			case "typedLeaves":
				island, err := decodeIsland(c.Island)
				if err != nil {
					t.Fatal(err)
				}
				assertTypedList(t, typedLeaves(island), c.Want)
			case "updateTyped":
				prev, inc := decodePair(t, c.Prev, c.Inc)
				assertTypedList(t, UpdateTyped(prev, inc, c.Typed, now), c.Want)
			case "dropGone":
				inc, err := decodeIsland(c.Inc)
				if err != nil {
					t.Fatal(err)
				}
				assertTypedList(t, DropGone(c.Typed, inc), c.Want)
			case "judgeOutside":
				base, inc := decodePair(t, c.Base, c.Inc)
				d := ClassifyDestruction(base, inc)
				got := JudgeOutside(base, inc, d, c.Typed, now)
				var want struct {
					Fire   bool
					Reason string
					Gone   []TypedValue
				}
				decodeWant(t, c.Want, &want)
				if got.Fire != want.Fire || got.Reason != want.Reason {
					t.Fatalf("fire/reason = %v/%q, want %v/%q (gone %+v)", got.Fire, got.Reason, want.Fire, want.Reason, got.Gone)
				}
				assertTyped(t, got.Gone, want.Gone)
			case "putBack":
				current, recoverable := decodePair(t, c.Current, c.Recoverable)
				merged, placed := PutBack(current, c.Gone, recoverable)
				var want struct {
					Merged json.RawMessage
					Placed int
				}
				decodeWant(t, c.Want, &want)
				wantMerged, err := decodeIsland(want.Merged)
				if err != nil {
					t.Fatal(err)
				}
				if placed != want.Placed {
					t.Fatalf("placed = %d, want %d", placed, want.Placed)
				}
				if stableDigest(merged) != stableDigest(wantMerged) {
					t.Fatalf("merged = %s, want %s", Stringify(merged), Stringify(wantMerged))
				}
			default:
				t.Fatalf("unknown op %s", c.Op)
			}
		})
	}
}

func decodePair(t *testing.T, a, b json.RawMessage) (dataapi.Value, dataapi.Value) {
	t.Helper()
	first, err := decodeIsland(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := decodeIsland(b)
	if err != nil {
		t.Fatal(err)
	}
	return first, second
}

func decodeWant(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func assertTypedList(t *testing.T, got []TypedValue, wantRaw json.RawMessage) {
	t.Helper()
	var want []TypedValue
	decodeWant(t, wantRaw, &want)
	assertTyped(t, got, want)
}

func assertTyped(t *testing.T, got, want []TypedValue) {
	t.Helper()
	if got == nil {
		got = []TypedValue{}
	}
	if want == nil {
		want = []TypedValue{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("typed =\n%+v\nwant\n%+v", got, want)
	}
}
