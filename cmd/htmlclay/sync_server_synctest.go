//go:build synctest

package main

import "os"

// syncServerFromBuild reads the stand-in server's address in a synctest build,
// the one the pair and round-trip runs use (08-sync.md decision 8).
func syncServerFromBuild() string { return os.Getenv("HTMLCLAY_SYNC_SERVER") }
