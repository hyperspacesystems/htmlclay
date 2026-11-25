package synchistory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const folder = "0dd3adfe2b553e75"

func entry(n int) Entry {
	return Entry{ID: fmt.Sprintf("s1:r-%d", n), At: fmt.Sprintf("2026-11-15T10:00:%02dZ", n%60), Kind: Uploaded, Path: fmt.Sprintf("f%d.html", n)}
}

func lines(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, folder+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

func ids(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

// TestM4History is 08-sync.md step 8.38.
func TestM4History(t *testing.T) {
	t.Run("a report sent twice adds one line", func(t *testing.T) {
		dir := t.TempDir()
		s := New(dir)
		if added, err := s.Append(folder, entry(1), entry(2)); err != nil || added != 2 {
			t.Fatalf("first = %d, %v", added, err)
		}
		if added, err := s.Append(folder, entry(2), entry(3)); err != nil || added != 1 {
			t.Fatalf("again = %d, %v", added, err)
		}
		if added, _ := s.Append(folder, entry(3), entry(3)); added != 0 {
			t.Fatalf("a repeat added %d", added)
		}
		if n := lines(t, dir); n != 3 {
			t.Fatalf("lines = %d, want 3", n)
		}
	})

	t.Run("pages run newest first and the cursor reaches the oldest", func(t *testing.T) {
		s := New(t.TempDir())
		for n := 1; n <= 7; n++ {
			if _, err := s.Append(folder, entry(n)); err != nil {
				t.Fatal(err)
			}
		}
		page, next, err := s.Page(folder, "", 3)
		if err != nil || strings.Join(ids(page), ",") != "s1:r-7,s1:r-6,s1:r-5" || next != "s1:r-5" {
			t.Fatalf("first page %v next %q err %v", ids(page), next, err)
		}
		page, next, _ = s.Page(folder, next, 3)
		if strings.Join(ids(page), ",") != "s1:r-4,s1:r-3,s1:r-2" || next != "s1:r-2" {
			t.Fatalf("second page %v next %q", ids(page), next)
		}
		page, next, _ = s.Page(folder, next, 3)
		if strings.Join(ids(page), ",") != "s1:r-1" || next != "" {
			t.Fatalf("last page %v next %q", ids(page), next)
		}
		if page, next, _ = s.Page(folder, "s1:gone", 3); len(page) != 0 || next != "" {
			t.Fatalf("unknown cursor %v %q", ids(page), next)
		}
	})

	t.Run("only the newest lines are kept", func(t *testing.T) {
		dir := t.TempDir()
		s := New(dir)
		s.keep = 4
		for n := 1; n <= 6; n++ {
			if _, err := s.Append(folder, entry(n)); err != nil {
				t.Fatal(err)
			}
		}
		page, _, _ := s.Page(folder, "", 10)
		if strings.Join(ids(page), ",") != "s1:r-6,s1:r-5,s1:r-4,s1:r-3" || lines(t, dir) != 4 {
			t.Fatalf("kept %v", ids(page))
		}
	})

	t.Run("a folder with no history is an empty page", func(t *testing.T) {
		page, next, err := New(t.TempDir()).Page(folder, "", 10)
		if err != nil || len(page) != 0 || next != "" {
			t.Fatalf("%v %q %v", page, next, err)
		}
	})

	t.Run("a folder id that is not one the app makes is refused", func(t *testing.T) {
		dir := t.TempDir()
		s := New(dir)
		for _, bad := range []string{"../escape", "", "0DD3ADFE2B553E75", "0dd3adfe2b553e7/"} {
			if _, err := s.Append(bad, entry(1)); !errors.Is(err, ErrFolderID) {
				t.Errorf("Append(%q) = %v", bad, err)
			}
			if _, _, err := s.Page(bad, "", 1); !errors.Is(err, ErrFolderID) {
				t.Errorf("Page(%q) = %v", bad, err)
			}
		}
	})
}
