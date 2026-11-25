package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// RefuseReplace makes every write that replaces the file at path fail, for a
// writer that writes a temporary file beside it and renames that into place.
// The file stays where it is and readable. The returned call lets writes
// through again, and the test's cleanup runs it too.
//
// A read-only folder stops the temporary file. Windows ignores a folder's
// mode, so there the file is held open, which refuses the rename onto it.
func RefuseReplace(t testing.TB, path string) (allow func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		held, err := os.Open(path)
		if err != nil {
			t.Fatalf("hold %s open: %v", path, err)
		}
		allow = func() { held.Close() }
		t.Cleanup(allow)
		return allow
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("read the mode of %s: %v", dir, err)
	}
	allow = func() { os.Chmod(dir, info.Mode().Perm()) }
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("make %s read-only: %v", dir, err)
	}
	t.Cleanup(allow)
	return allow
}
