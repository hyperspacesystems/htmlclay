package dataguard

import (
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
)

func droppedItem(title string) *dataapi.Object {
	return orderedObject([2]any{"title", title})
}

func droppedList(items ...dataapi.Value) []dataapi.Value {
	return items
}

func TestDroppedItems(t *testing.T) {
	a, b, c, d, e := droppedItem("a"), droppedItem("b"), droppedItem("c"), droppedItem("d"), droppedItem("e")
	x, y, z := droppedItem("x"), droppedItem("y"), droppedItem("z")
	editedB := orderedObject([2]any{"title", "b-edited"})

	cases := []struct {
		name        string
		recoverable dataapi.Value
		current     dataapi.Value
		want        int
	}{
		{
			name:        "a wiped list refilled with two new items drops both",
			recoverable: droppedList(a, b, c, d, e),
			current:     droppedList(x, y),
			want:        2,
		},
		{
			name:        "the surviving pair stays, the three new items drop",
			recoverable: droppedList(a, b, c, d, e),
			current:     droppedList(a, b, x, y, z),
			want:        3,
		},
		{
			name:        "duplicates count once each",
			recoverable: droppedList(a, a),
			current:     droppedList(a, a, a),
			want:        1,
		},
		{
			name:        "an unchanged list drops nothing",
			recoverable: droppedList(a, b, c),
			current:     droppedList(a, b, c),
			want:        0,
		},
		{
			name:        "an edited item is overwritten by restore",
			recoverable: droppedList(a, b, c),
			current:     droppedList(a, editedB),
			want:        1,
		},
		{
			name:        "key order inside an item does not matter",
			recoverable: droppedList(orderedObject([2]any{"t", "a"}, [2]any{"n", 1.0})),
			current:     droppedList(orderedObject([2]any{"n", 1.0}, [2]any{"t", "a"})),
			want:        0,
		},
		{
			name:        "a current key that is not a list drops nothing",
			recoverable: orderedObject([2]any{"items", droppedList(a, b)}, [2]any{"title", "before"}),
			current:     orderedObject([2]any{"items", droppedList(a, b)}, [2]any{"title", "after"}),
			want:        0,
		},
		{
			name:        "wrapped islands count each list key",
			recoverable: orderedObject([2]any{"items", droppedList(a, b, c, d, e)}, [2]any{"title", "Board"}),
			current:     orderedObject([2]any{"items", droppedList(x, y)}, [2]any{"title", "Board"}),
			want:        2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DroppedItems(tc.recoverable, tc.current); got != tc.want {
				t.Fatalf("DroppedItems = %d, want %d", got, tc.want)
			}
		})
	}
}
