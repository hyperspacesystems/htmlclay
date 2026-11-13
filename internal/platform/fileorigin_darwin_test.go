//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDownloadedReadsQuarantineNotProvenance(t *testing.T) {
	dir := t.TempDir()
	quarantined := filepath.Join(dir, "quarantined")
	provenance := filepath.Join(dir, "provenance")
	for _, p := range []string{quarantined, provenance} {
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := unix.Setxattr(quarantined, "com.apple.quarantine", []byte("0081;6720a3b1;Safari;"), 0); err != nil {
		t.Skipf("cannot set quarantine here: %v", err)
	}
	if err := unix.Setxattr(provenance, "com.apple.provenance", []byte{1, 2, 3}, 0); err != nil && err != unix.EPERM {
		t.Logf("provenance not settable (%v); checking the plain file only", err)
	}
	if !Downloaded(quarantined) {
		t.Fatal("quarantined file must count as downloaded")
	}
	if Downloaded(provenance) {
		t.Fatal("provenance alone must not count as downloaded")
	}
}
