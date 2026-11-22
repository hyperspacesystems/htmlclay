package main

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/tier"
	servicefiles "github.com/panphora/htmlclay/service"
)

//go:embed playground_welcome.html
var playgroundWelcomeHTML []byte

var playgroundStarterTemplates = []struct {
	templateID string
	filename   string
}{
	{templateID: "journal", filename: "journal.html"},
	{templateID: "kanban", filename: "kanban.html"},
	{templateID: "freelance-rate-calculator", filename: "freelance-rate-calculator.html"},
}

// seedDefaultPlayground creates and serves ~/htmlclay/playground strictly once
// on a fresh install (when config.json did not exist, PlaygroundSeeded is false,
// and no folders are trusted or served yet). Existing 1.x upgraders and users
// who later untrust or delete Playground are never re-seeded.
func (a *app) seedDefaultPlayground() {
	if !a.loaded.FreshInstall || a.rt.cfg.PlaygroundSeededDone() {
		return
	}
	if len(a.rt.cfg.TrustedFolderList()) > 0 || len(a.rt.cfg.ServedFolderList()) > 0 {
		return
	}
	if os.Getenv("HTMLCLAY_NO_DEFAULT_PLAYGROUND") == "1" {
		return
	}

	dir := filepath.Join(a.rt.home, "htmlclay", "playground")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.rt.logger.Printf("Could not create default Playground folder %s: %v", dir, err)
		return
	}
	realDir, err := resolveSymlinks(dir)
	if err != nil {
		a.rt.logger.Printf("Could not resolve default Playground folder %s: %v", dir, err)
		return
	}
	if _, ok := session.ContainWithinHome(a.rt.home, realDir); !ok {
		a.rt.logger.Printf("Skipping default Playground %s: it leads outside home to %s", dir, realDir)
		return
	}

	writeStarter := func(filename string, data []byte) {
		target := filepath.Join(realDir, filename)
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			return
		}
		if err != nil {
			a.rt.logger.Printf("Could not write Playground starter %s: %v", target, err)
			return
		}
		_, werr := f.Write(data)
		if cerr := errors.Join(werr, f.Close()); cerr != nil {
			_ = os.Remove(target)
			a.rt.logger.Printf("Could not save Playground starter %s: %v", target, cerr)
			return
		}
		if err := a.recordMadeDocument(target, tier.HowCreated); err != nil {
			a.rt.logger.Printf("Could not record Playground starter %s as yours: %v", target, err)
		}
	}

	writeStarter("welcome.html", playgroundWelcomeHTML)
	for _, starter := range playgroundStarterTemplates {
		if data, ok := servicefiles.Template(starter.templateID); ok {
			writeStarter(starter.filename, data)
		}
	}

	if _, refusal, err := a.serveFolderNamed(realDir, "playground"); err != nil {
		a.rt.logger.Printf("Could not serve default Playground %s: %v", realDir, err)
		return
	} else if refusal != nil {
		a.rt.logger.Printf("Could not serve default Playground %s: %s", realDir, refusal.Code)
		return
	}
	a.rt.cfg.SetPlaygroundSeeded(true)
	a.saveConfig("seed default playground")
}
