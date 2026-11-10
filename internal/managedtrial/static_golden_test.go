//go:build managedtrial

package managedtrial

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStaticGolden pins the three routes a page served by this host reads its
// runtime, its template CSS and its icons from to Local's real answers.
func TestStaticGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and these routes read that checkout's own assets, templates and bundled runtime release")
	}
	runGoldenCases(t, filepath.Join("testdata", "static-golden"), localSource, nil)
}
