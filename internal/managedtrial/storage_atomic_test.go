//go:build managedtrial

package managedtrial

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
)

func TestDirSyncUnsupported(t *testing.T) {
	cases := []struct {
		name string
		err  error
		goos string
		want bool
	}{
		{"an I/O error on macOS is a real durability failure", syscall.EIO, "darwin", false},
		{"EACCES on macOS cannot sync a directory", syscall.EACCES, "darwin", true},
		{"a wrapped permission error on Linux", &os.PathError{Op: "sync", Path: "/x", Err: fs.ErrPermission}, "linux", true},
		{"any directory sync failure on Windows", syscall.EIO, "windows", true},
	}
	for _, c := range cases {
		if got := dirSyncUnsupported(c.err, c.goos); got != c.want {
			t.Errorf("%s: dirSyncUnsupported = %v, want %v", c.name, got, c.want)
		}
	}
}
