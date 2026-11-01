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
