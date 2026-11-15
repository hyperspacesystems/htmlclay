package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/secret"
)

// syncKeyName is the sync key's item name in the secret store.
const syncKeyName = "sync-key"

// defaultSyncServer is where sync goes. HTMLCLAY_SYNC_SERVER points a test at a
// stand-in; the service accepts plain http only on loopback.
const defaultSyncServer = "https://hyperclay.com"

func syncServerURL() string {
	if v := os.Getenv("HTMLCLAY_SYNC_SERVER"); v != "" {
		return v
	}
	return defaultSyncServer
}

// syncProfile names this install's items in the secret store: "default" for the
// standard config folder, and a short hash of the folder for any other, such as
// a candidate build or HTMLCLAY_CONFIG_BASE, so two installs never share a key.
func syncProfile(configDir string) string {
	if base, err := os.UserConfigDir(); err == nil {
		standard := filepath.Join(base, "htmlclay")
		if resolved, err := filepath.EvalSymlinks(standard); err == nil {
			standard = resolved
		}
		if configDir == standard {
			return "default"
		}
	}
	sum := sha256.Sum256([]byte(configDir))
	return "c" + hex.EncodeToString(sum[:6])
}

// openSyncStore answers this install's secret store, or nil with the failure
// logged: sync then stays off, and nothing else changes.
func (a *app) openSyncStore() secret.Store {
	store, err := secret.Open(syncProfile(a.rt.configDir), a.rt.configDir)
	if err != nil {
		a.rt.logger.Printf("The sync key store could not be opened: %v", err)
		return nil
	}
	return store
}

// syncKey answers the server and the key the service should hold, when the
// store has one. A failed read is logged without any part of the key.
func (a *app) syncKey() (serverURL, key string, ok bool) {
	if a.rt.syncStore == nil {
		return "", "", false
	}
	key, found, err := a.rt.syncStore.Get(syncKeyName)
	if err != nil {
		a.rt.logger.Printf("The sync key could not be read: %v", err)
		return "", "", false
	}
	if !found {
		return "", "", false
	}
	return syncServerURL(), key, true
}
