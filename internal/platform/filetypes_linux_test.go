//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetHTMLOpenWithRewritesTheDesktopFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	path := filepath.Join(dir, "applications", desktopID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "[Desktop Entry]\nName=HTML Clay\nMimeType=application/x-htmlclay;text/html;\nExec=htmlclay %U\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setHTMLOpenWith("", false); err != nil {
		t.Fatalf("setHTMLOpenWith(false): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "MimeType=application/x-htmlclay;text/html;", "MimeType=application/x-htmlclay;", 1)
	if string(data) != want {
		t.Fatalf("desktop file after off:\n%s\nwant:\n%s", data, want)
	}

	if err := setHTMLOpenWith("", true); err != nil {
		t.Fatalf("setHTMLOpenWith(true): %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("desktop file after on:\n%s\nwant:\n%s", data, original)
	}
}
