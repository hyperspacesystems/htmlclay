//go:build !managedtrial && !servedfaults

package served

import (
	"strings"
	"testing"
)

// TestUntaggedBuildHasNoTrialDefaults keeps the developer-only live folder and the
// Node Tailwind compile out of a build without tags.
func TestUntaggedBuildHasNoTrialDefaults(t *testing.T) {
	if liveRoot != nil {
		t.Fatal("an untagged build names a live folder")
	}
	if reachesLiveRoot("/anything") {
		t.Fatal("an untagged build refuses symlinks against a live folder")
	}
	if defaultTailwindCompile != nil {
		t.Fatal("an untagged build has the Node Tailwind compile")
	}
	_, err := NewHost(Options{}).AddRoot(RootSpec{ID: "a", Path: t.TempDir(), PublicHost: "a.htmlclay.localhost:1212"})
	if err == nil || !strings.Contains(err.Error(), "no Tailwind compiler") {
		t.Fatalf("a root with no Tailwind compiler: err = %v", err)
	}
}
