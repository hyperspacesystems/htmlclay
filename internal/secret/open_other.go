//go:build !darwin && !windows && !linux

package secret

import "path/filepath"

// Open answers this system's secret store for one HTML Clay profile. A system with
// none of the stores HTML Clay knows keeps the key in a private file in the
// profile's config folder.
func Open(profile, configDir string) (Store, error) {
	if err := checkName(profile); err != nil {
		return nil, err
	}
	return NewFile(filepath.Join(configDir, "secrets")), nil
}
