//go:build !darwin && !windows

package secret

import "path/filepath"

// Open answers this system's secret store for one HTML Clay profile. Until the
// Secret Service store lands (08-sync.md step 8.3), every other system keeps the
// key in a private file in the profile's config folder.
func Open(profile, configDir string) (Store, error) {
	if err := checkName(profile); err != nil {
		return nil, err
	}
	return NewFile(filepath.Join(configDir, "secrets")), nil
}
