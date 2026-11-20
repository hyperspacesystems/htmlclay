package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChangedWithin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if !ChangedWithin(path, 10*time.Minute, now) {
		t.Fatal("a file written just now must count as changed")
	}
	if ChangedWithin(path, 10*time.Minute, now.Add(time.Hour)) {
		t.Fatal("a file written an hour before now must not count as changed")
	}
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if !ChangedWithin(path, 10*time.Minute, now) {
		t.Fatal("a backdated modification time must not hide a file that just arrived")
	}
	if !ChangedWithin(filepath.Join(t.TempDir(), "missing"), time.Minute, now) {
		t.Fatal("an unreadable file must count as changed")
	}
}

func TestDownloadedPlainFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if Downloaded(path) {
		t.Fatal("a file this test wrote is not a download")
	}
}

// TestWrittenAtIgnoresTheChangeTime guards the upgrade's "unchanged since"
// rule: writing a file's attributes moves the change time while its bytes stay
// the same, so a document that was here before the upgrade must not look new.
func TestWrittenAtIgnoresTheChangeTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.html")
	if err := os.WriteFile(path, []byte("<p>old</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	mark := time.Now()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if WrittenAt(info).After(mark) {
		t.Fatalf("an attribute write made an old file look written at %v", WrittenAt(info))
	}
}
