//go:build darwin

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/platform"
	"golang.org/x/sys/unix"
)

func TestSaveKeepsTheDownloadMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte("<p>one</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(path, "com.apple.quarantine", []byte("0081;6720a3b1;Chrome;"), 0); err != nil {
		t.Skipf("cannot set quarantine here: %v", err)
	}
	if err := atomicWriteFile(path, []byte("<p>two</p>")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "<p>two</p>" {
		t.Fatalf("saved %q", got)
	}
	if !platform.Downloaded(path) {
		t.Fatal("a save erased the download mark")
	}
}
