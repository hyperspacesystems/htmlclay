package served

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestM3RenameHistory proves a rename carries the document's history with it: the
// versions a person kept read back under the new name, a folder rename carries
// every document's, history already at the destination is kept under a dated
// sibling name rather than deleted or mixed in, and a failure between the file
// move and the history move loses nothing.
func TestM3RenameHistory(t *testing.T) {
	newRoot := func(t *testing.T) (string, *commitHost) {
		t.Helper()
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return root, newCommitHost(root, "root-rename-history", newConsentRegistry(root, nil), newGuardHooks(nil), nil)
	}
	doc := func(title string) []byte {
		return []byte("<!doctype html>\n<html><body><h1>" + title + "</h1></body></html>\n")
	}
	versionBytes := func(t *testing.T, dir string) []string {
		t.Helper()
		found := []string{}
		for _, entry := range listVersionEntries(dir, false) {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name))
			if err != nil {
				t.Fatal(err)
			}
			found = append(found, string(data))
		}
		return found
	}
	move := func(t *testing.T, host *commitHost, root, from, to string, isDir bool) structureResult {
		t.Helper()
		result, err := host.restructure("move", []structureEntry{{
			FromPath:  filepath.Join(root, from),
			ToPath:    filepath.Join(root, to),
			IsDir:     isDir,
			FromName:  from,
			ToName:    to,
			PageState: "move",
		}})
		if err != nil {
			t.Fatalf("the move %s -> %s failed: %v", from, to, err)
		}
		return result
	}

	t.Run("old-version-reads-under-the-new-name", func(t *testing.T) {
		root, host := newRoot(t)
		writeCommitFile(t, root, "a.html", doc("One"))
		if published := CreateBackup(root, "a", []byte("the version kept before"), nil); published == nil {
			t.Fatal("the backup was not written")
		}
		if got := versionBytes(t, versionsDirFor(root, "a.html")); len(got) != 1 {
			t.Fatalf("a.html holds %v before the rename, want the one kept version", got)
		}

		if result := move(t, host, root, "a.html", "b.html", false); result.Outcome != "committed" {
			t.Fatalf("the rename got %+v, want committed", result)
		}
		if result := move(t, host, root, "b.html", "c.html", false); result.Outcome != "committed" {
			t.Fatalf("the second rename got %+v, want committed", result)
		}

		got := versionBytes(t, versionsDirFor(root, "c.html"))
		if len(got) != 1 || got[0] != "the version kept before" {
			t.Fatalf("c.html reads back %v, want the version kept before the rename", got)
		}
		if entries := listVersionEntries(versionsDirFor(root, "a.html"), false); len(entries) != 0 {
			t.Fatalf("a.html still holds %d version(s) after the rename", len(entries))
		}
	})

	t.Run("folder-carries-every-documents-history", func(t *testing.T) {
		root, host := newRoot(t)
		writeCommitFile(t, root, "docs/one.html", doc("One"))
		writeCommitFile(t, root, "docs/two.html", doc("Two"))
		writeCommitFile(t, root, "docs.html", doc("The folder's own name"))
		if CreateBackup(root, "docs/one", []byte("one's kept version"), nil) == nil {
			t.Fatal("one's backup was not written")
		}
		if CreateBackup(root, "docs/two", []byte("two's kept version"), nil) == nil {
			t.Fatal("two's backup was not written")
		}
		if CreateBackup(root, "docs", []byte("the document named after the folder"), nil) == nil {
			t.Fatal("docs.html's backup was not written")
		}

		if result := move(t, host, root, "docs", "archive", true); result.Outcome != "committed" {
			t.Fatalf("the folder rename got %+v, want committed", result)
		}

		for name, want := range map[string]string{
			"archive/one.html": "one's kept version",
			"archive/two.html": "two's kept version",
		} {
			got := versionBytes(t, versionsDirFor(root, name))
			if len(got) != 1 || got[0] != want {
				t.Errorf("%s reads back %v, want %q", name, got, want)
			}
		}
		// docs.html did not move, so its history stays beside it.
		got := versionBytes(t, versionsDirFor(root, "docs.html"))
		if len(got) != 1 || got[0] != "the document named after the folder" {
			t.Errorf("docs.html reads back %v, want its own version left in place", got)
		}
	})

	t.Run("leftover-history-at-the-destination-survives", func(t *testing.T) {
		root, host := newRoot(t)
		writeCommitFile(t, root, "a.html", doc("One"))
		if CreateBackup(root, "a", []byte("the renamed document's version"), nil) == nil {
			t.Fatal("a's backup was not written")
		}
		// A document that once had the destination's name kept a version there.
		leftover := versionsDirFor(root, "b.html")
		if err := os.MkdirAll(leftover, 0o777); err != nil {
			t.Fatal(err)
		}
		if _, err := PublishVersion(leftover, ".html", []byte("the deleted document's version")); err != nil {
			t.Fatal(err)
		}

		if result := move(t, host, root, "a.html", "b.html", false); result.Outcome != "committed" {
			t.Fatalf("the rename got %+v, want committed", result)
		}

		got := versionBytes(t, versionsDirFor(root, "b.html"))
		if len(got) != 1 || got[0] != "the renamed document's version" {
			t.Fatalf("b.html reads back %v, want only the renamed document's version", got)
		}
		versionsRoot := filepath.Join(root, filepath.FromSlash(versionsDirName))
		kept := ""
		err := filepath.WalkDir(versionsRoot, func(target string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			data, readErr := os.ReadFile(target)
			if readErr != nil {
				return readErr
			}
			if bytes.Equal(data, []byte("the deleted document's version")) {
				kept, _ = filepath.Rel(versionsRoot, target)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if kept == "" {
			t.Fatalf("the history the destination already held is gone from %s", versionsRoot)
		}
		if !strings.HasPrefix(kept, "b-2") {
			t.Fatalf("the leftover is kept at %q, want a dated sibling of b", kept)
		}
	})

	t.Run("failure-after-the-file-move-loses-nothing", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root writes into a read-only versions folder")
		}
		if runtime.GOOS == "windows" {
			t.Skip("Windows ignores a folder's read-only mode")
		}
		root, host := newRoot(t)
		writeCommitFile(t, root, "a.html", doc("One"))
		if CreateBackup(root, "a", []byte("the renamed document's version"), nil) == nil {
			t.Fatal("a's backup was not written")
		}
		leftover := versionsDirFor(root, "b.html")
		if err := os.MkdirAll(leftover, 0o777); err != nil {
			t.Fatal(err)
		}
		if _, err := PublishVersion(leftover, ".html", []byte("the deleted document's version")); err != nil {
			t.Fatal(err)
		}
		versionsRoot := filepath.Join(root, filepath.FromSlash(versionsDirName))
		if err := os.Chmod(versionsRoot, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(versionsRoot, 0o755) })

		result := move(t, host, root, "a.html", "b.html", false)
		if result.Outcome != "committed" || result.HistoryErr == nil {
			t.Fatalf("the rename got %+v, want committed with its history reported as not moved", result)
		}
		_ = os.Chmod(versionsRoot, 0o755)

		if data, err := os.ReadFile(filepath.Join(root, "b.html")); err != nil || !bytes.Contains(data, []byte("One")) {
			t.Fatalf("b.html = %q %v, want the moved file", data, err)
		}
		if got := versionBytes(t, versionsDirFor(root, "a.html")); len(got) != 1 || got[0] != "the renamed document's version" {
			t.Fatalf("a.html's history reads back %v, want it kept where it was", got)
		}
		if got := versionBytes(t, versionsDirFor(root, "b.html")); len(got) != 1 || got[0] != "the deleted document's version" {
			t.Fatalf("b.html's history reads back %v, want the leftover kept", got)
		}
	})
}
