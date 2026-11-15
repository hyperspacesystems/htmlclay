package served

import (
	"os"
	"path/filepath"
	"testing"
)

// A file and a folder of the same name share one history directory: the file's
// versions are the files in it, and each document inside the folder has its own
// subdirectory. Renaming the file moves only its own versions.
func TestM3RenameMovesOnlyTheFilesHistory(t *testing.T) {
	write := func(t *testing.T, dir, name, body string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T, dir, name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s/%s: %v", dir, name, err)
		}
		return string(data)
	}
	const version = "2026-11-01-10-00-00-000.html"

	t.Run("a folder of the old name keeps its documents' history", func(t *testing.T) {
		base := t.TempDir()
		notes, todo := versionsDirFor(base, "notes.html"), versionsDirFor(base, "notes/todo.html")
		write(t, notes, version, "notes v1")
		write(t, todo, version, "todo v1")
		write(t, todo, version+pinSuffix, "")

		if err := moveDocumentHistory(base, "notes.html", "journal.html"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, todo, version); got != "todo v1" {
			t.Fatalf("notes/todo.html's version = %q", got)
		}
		read(t, todo, version+pinSuffix)
		if got := read(t, versionsDirFor(base, "journal.html"), version); got != "notes v1" {
			t.Fatalf("journal.html's version = %q", got)
		}
		if _, err := os.Stat(filepath.Join(versionsDirFor(base, "journal.html"), "todo")); err == nil {
			t.Fatal("notes/todo.html's history moved under journal.html")
		}
	})

	t.Run("a folder of the new name keeps its documents' history", func(t *testing.T) {
		base := t.TempDir()
		draft, journal, x := versionsDirFor(base, "draft.html"), versionsDirFor(base, "journal.html"), versionsDirFor(base, "journal/x.html")
		write(t, draft, version, "draft v1")
		write(t, journal, "2026-10-01-10-00-00-000.html", "old journal v1")
		write(t, x, version, "x v1")
		write(t, x, version+pinSuffix, "")

		if err := moveDocumentHistory(base, "draft.html", "journal.html"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, x, version); got != "x v1" {
			t.Fatalf("journal/x.html's version = %q", got)
		}
		read(t, x, version+pinSuffix)
		if got := read(t, journal, version); got != "draft v1" {
			t.Fatalf("journal.html's version = %q", got)
		}
		if _, err := os.Stat(filepath.Join(journal, "2026-10-01-10-00-00-000.html")); err == nil {
			t.Fatal("the old journal.html's version shares the renamed document's history")
		}
		entries, err := os.ReadDir(filepath.Dir(journal))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range entries {
			if e.IsDir() && len(e.Name()) > len("journal-") && e.Name()[:len("journal-")] == "journal-" {
				if got := read(t, filepath.Join(filepath.Dir(journal), e.Name()), "2026-10-01-10-00-00-000.html"); got == "old journal v1" {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("the old journal.html's version was not set aside")
		}
	})
}
