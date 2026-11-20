//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestKeepDownloadMarkCopiesQuarantine(t *testing.T) {
	dir := t.TempDir()
	from, to, plain := filepath.Join(dir, "from.html"), filepath.Join(dir, "to.html"), filepath.Join(dir, "plain.html")
	for _, p := range []string{from, to, plain} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mark := []byte("0081;6720a3b1;Chrome;")
	if err := unix.Setxattr(from, downloadMarkAttr, mark, 0); err != nil {
		t.Skipf("cannot set quarantine here: %v", err)
	}
	if err := KeepDownloadMark(from, to); err != nil {
		t.Fatal(err)
	}
	if got, _ := readXattr(to, downloadMarkAttr); string(got) != string(mark) {
		t.Fatalf("mark = %q, want %q", got, mark)
	}
	if !Downloaded(to) {
		t.Fatal("the copy must read as downloaded")
	}
	if err := KeepDownloadMark(plain, filepath.Join(dir, "plain-copy.html")); err != nil {
		t.Fatalf("a file with no mark must copy nothing and fail nothing: %v", err)
	}
}
