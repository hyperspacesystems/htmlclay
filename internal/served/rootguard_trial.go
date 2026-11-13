//go:build managedtrial

package served

import (
	"os"
	"path/filepath"
)

// DefaultLiveRoot is the live Hyperclay Local apps folder for this user.
func DefaultLiveRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return realPath(filepath.Join(home, "Documents", "GitHub", "hyper", "LOCAL_APPS"))
}

func init() { liveRoot = DefaultLiveRoot }
