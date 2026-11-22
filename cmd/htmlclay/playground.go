package main

import (
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/session"
)

// seedDefaultPlayground creates and serves an empty ~/htmlclay/workspace
// strictly once on a fresh install (when config.json did not exist, or on the
// first Candidate launch, when PlaygroundSeeded is false, and no folders are
// trusted or served yet). Existing 1.x upgraders and users who later untrust or
// delete the workspace are never re-seeded.
func (a *app) seedDefaultPlayground() {
	if !a.loaded.FreshInstall && os.Getenv("HTMLCLAY_CANDIDATE_FRESH") != "1" {
		return
	}
	if a.rt.cfg.PlaygroundSeededDone() {
		return
	}
	if len(a.rt.cfg.TrustedFolderList()) > 0 || len(a.rt.cfg.ServedFolderList()) > 0 {
		return
	}
	if os.Getenv("HTMLCLAY_NO_DEFAULT_PLAYGROUND") == "1" {
		return
	}

	dir := filepath.Join(a.rt.home, "htmlclay", "workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.rt.logger.Printf("Could not create default workspace folder %s: %v", dir, err)
		return
	}
	realDir, err := resolveSymlinks(dir)
	if err != nil {
		a.rt.logger.Printf("Could not resolve default workspace folder %s: %v", dir, err)
		return
	}
	if _, ok := session.ContainWithinHome(a.rt.home, realDir); !ok {
		a.rt.logger.Printf("Skipping default workspace %s: it leads outside home to %s", dir, realDir)
		return
	}

	if _, refusal, err := a.serveFolderNamed(realDir, "workspace"); err != nil {
		a.rt.logger.Printf("Could not serve default workspace %s: %v", realDir, err)
		return
	} else if refusal != nil {
		a.rt.logger.Printf("Could not serve default workspace %s: %s", realDir, refusal.Code)
		return
	}
	a.rt.cfg.SetPlaygroundSeeded(true)
	a.saveConfig("seed default workspace")
}
