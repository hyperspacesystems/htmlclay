//go:build synctest

package main

import "os"

// syncServerFromBuild reads the stand-in server's address in a synctest build,
// the one the pair and round-trip runs use (08-sync.md decision 8).
func syncServerFromBuild() string { return os.Getenv("HTMLCLAY_SYNC_SERVER") }

// A synctest build reads its release check's address from the environment, so a
// round-trip run never reaches htmlclay.com: "updates": "off" still checks for
// the download link.
func init() {
	if v := os.Getenv("HTMLCLAY_RELEASE_INFO_URL"); v != "" {
		releaseInfoURL = v
	}
}
