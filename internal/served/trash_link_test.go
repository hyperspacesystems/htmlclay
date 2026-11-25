package served

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestEmptyTrashStaysInsideTheRoot is empty-trash's guard, the one command that
// deletes: a .trash that is a link to another folder is refused with that folder
// untouched, and a link inside the Trash is removed without touching what it
// points at.
func TestEmptyTrashStaysInsideTheRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("making symlinks on Windows needs Developer Mode")
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, outside, "keep.html", []byte("<!doctype html>\n"))

	linked, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linked, ".trash")); err != nil {
		t.Fatal(err)
	}
	host := newCommitHost(linked, "root-linked-trash", newConsentRegistry(linked, nil), newGuardHooks(nil), nil)
	result, err := host.restructure("empty-trash", nil)
	if err != nil || result.Outcome != "failed" || result.Reason != "ENOTDIR" {
		t.Fatalf("empty-trash through a linked .trash got %+v / %v, want failed/ENOTDIR", result, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.html")); err != nil {
		t.Fatalf("empty-trash reached through the link: %v", err)
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, root, filepath.Join(".trash", "old.html"), []byte("<!doctype html>\n"))
	if err := os.Symlink(outside, filepath.Join(root, ".trash", "link")); err != nil {
		t.Fatal(err)
	}
	host = newCommitHost(root, "root-link-inside", newConsentRegistry(root, nil), newGuardHooks(nil), nil)
	result, err = host.restructure("empty-trash", nil)
	if err != nil || result.Outcome != "committed" || result.Completed != 2 {
		t.Fatalf("empty-trash with a link inside got %+v / %v, want committed with two entries", result, err)
	}
	if names, _ := os.ReadDir(filepath.Join(root, ".trash")); len(names) != 0 {
		t.Fatalf("empty-trash left %d entries", len(names))
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.html")); err != nil {
		t.Fatalf("removing the link removed what it points at: %v", err)
	}
}
