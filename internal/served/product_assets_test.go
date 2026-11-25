package served

import (
	"testing"

	"github.com/panphora/htmlclay/internal/clayruntime"
	"github.com/panphora/htmlclay/internal/served/assets"
)

// TestProductFSLoadsInTheHost proves a served root in the product can start: the
// one assets view reads Local's files and HTML Clay's own ClayJS releases.
func TestProductFSLoadsInTheHost(t *testing.T) {
	runtime, err := loadRuntimeReleases(assets.ProductFS())
	if err != nil {
		t.Fatalf("loadRuntimeReleases(assets.ProductFS()): %v", err)
	}
	if runtime.Selected != clayruntime.Selected {
		t.Fatalf("the host selects %q, clayruntime selects %q", runtime.Selected, clayruntime.Selected)
	}
	if _, ok := runtime.Releases[runtime.Selected]; !ok {
		t.Fatalf("the host did not load the selected release %q", runtime.Selected)
	}
}
