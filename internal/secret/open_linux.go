//go:build linux

package secret

import (
	"path/filepath"

	"github.com/godbus/dbus/v5"
)

// Open answers the Secret Service for one HTML Clay profile. With no session bus,
// no Secret Service on it, or a collection that stays locked, the key goes in a
// private file in the profile's config folder instead, and Kind says so.
func Open(profile, configDir string) (Store, error) {
	if err := checkName(profile); err != nil {
		return nil, err
	}
	if store, err := openSecretService(profile); err == nil {
		return store, nil
	}
	return NewFile(filepath.Join(configDir, "secrets")), nil
}

func openSecretService(profile string) (Store, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	store, err := newSecretService(conn, profile)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return store, nil
}
