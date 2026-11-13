//go:build windows

package served

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestAtomicWriteRetriesWhileFileHeld: Defender or the Search indexer opening a
// document for a moment, with no sharing, must not fail a save; the rename is
// retried until they let go.
func TestAtomicWriteRetriesWhileFileHeld(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("hold the file: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(60 * time.Millisecond)
		syscall.CloseHandle(handle)
		close(released)
	}()
	if err := atomicWriteFile(target, []byte("new"), nil, true); err != nil {
		t.Fatalf("a save while another program held the file for 60 ms failed: %v", err)
	}
	<-released
	if data, _ := os.ReadFile(target); string(data) != "new" {
		t.Fatalf("doc.html = %q, want the saved bytes", data)
	}
}
