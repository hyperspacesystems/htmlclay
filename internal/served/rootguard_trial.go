//go:build managedtrial

package served

import (
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/pathrel"
)

// DefaultLiveRoot is the live Hyperclay Local apps folder for this user.
func DefaultLiveRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return pathrel.RealPath(filepath.Join(home, "Documents", "GitHub", "hyper", "LOCAL_APPS"))
}

func init() { liveRoot = DefaultLiveRoot }
