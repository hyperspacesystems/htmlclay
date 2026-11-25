package appcore

import (
	"errors"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
)

// nameChecker is config.(*Config).ServedNameAvailable: valid, not taken, not retired.
func nameChecker(taken ...string) func(string) error {
	takenSet := map[string]bool{}
	for _, name := range taken {
		takenSet[name] = true
	}
	return func(name string) error {
		if err := config.ValidServedName(name); err != nil {
			return err
		}
		if takenSet[name] {
			return config.ErrServedNameTaken
		}
		return nil
	}
}

func TestDefaultServedName(t *testing.T) {
	long := strings.Repeat("a", 60)
	cases := []struct {
		folder    string
		taken     []string
		want      string
		wantEmpty bool
	}{
		{folder: "Notes", want: "notes"},
		{folder: "My Notes", want: "my-notes"},
		{folder: "My Project", want: "my-project"},
		{folder: "Café Notes", want: "caf-notes"},
		{folder: "---", want: "folder"},
		{folder: "", want: "folder"},
		{folder: "www", want: "www-2"},
		{folder: "notes", taken: []string{"notes"}, want: "notes-2"},
		{folder: "notes", taken: []string{"notes", "notes-2", "notes-3"}, want: "notes-4"},
		{folder: long, want: strings.Repeat("a", 40)},
		{folder: "a  b---c", want: "a-b-c"},
		{folder: "9 Lives", want: "9-lives"},
	}
	for _, tc := range cases {
		got := DefaultServedName(tc.folder, nameChecker(tc.taken...))
		if got != tc.want {
			t.Errorf("DefaultServedName(%q, taken %q) = %q, want %q", tc.folder, tc.taken, got, tc.want)
			continue
		}
		if err := config.ValidServedName(got); err != nil {
			t.Errorf("DefaultServedName(%q) = %q, which config refuses: %v", tc.folder, got, err)
		}
	}
}

func TestDefaultServedNameSuffixFitsTheLimit(t *testing.T) {
	long := strings.Repeat("b", 60)
	got := DefaultServedName(long, nameChecker(strings.Repeat("b", 40)))
	want := strings.Repeat("b", 38) + "-2"
	if got != want {
		t.Fatalf("DefaultServedName of a 60-character name taken as is = %q, want %q", got, want)
	}
	if err := config.ValidServedName(got); err != nil {
		t.Fatalf("result %q is refused: %v", got, err)
	}
}

func TestDefaultServedNameGivesUpAfter99(t *testing.T) {
	got := DefaultServedName("notes", func(string) error { return errors.New("taken") })
	if got != "" {
		t.Fatalf("DefaultServedName when nothing is available = %q, want empty", got)
	}
}
