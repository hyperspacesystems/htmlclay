//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

// TestDownloadedCountsOnlyInternetOrigins is Carbon 14: a browser's recorded
// origin marks a download only when it came from the internet, not when a file
// was saved from this machine or from a page served on it.
func TestDownloadedCountsOnlyInternetOrigins(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		mark string
		want bool
	}{
		{"file:///x", false},
		{"http://localhost:8080/a.html", false},
		{"http://app.localhost/a", false},
		{"http://127.0.0.1/a", false},
		{"http://192.168.1.4/a", false},
		{"https://example.com/a.html", true},
	}
	for i, c := range cases {
		path := filepath.Join(dir, "mark-"+strconv.Itoa(i)+".html")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := unix.Setxattr(path, downloadMarkAttr, []byte(c.mark), 0); err != nil {
			t.Fatalf("Setxattr %q: %v", c.mark, err)
		}
		if got := Downloaded(path); got != c.want {
			t.Errorf("Downloaded with origin %q = %v, want %v", c.mark, got, c.want)
		}
	}
	plain := filepath.Join(dir, "plain.html")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Downloaded(plain) {
		t.Error("a file with no origin mark counts as a download")
	}
}
