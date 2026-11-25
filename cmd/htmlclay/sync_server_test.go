//go:build !synctest

package main

import "testing"

func TestM4SyncServerFixed(t *testing.T) {
	t.Setenv("HTMLCLAY_SYNC_SERVER", "http://127.0.0.1:9")
	if got := syncServerURL(); got != defaultSyncServer {
		t.Fatalf("a release build sends the key to %q", got)
	}
}
