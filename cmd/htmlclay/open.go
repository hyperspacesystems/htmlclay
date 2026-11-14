package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/browser"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/session"
)

// openFile is the OS open: it logs, and tells the person with a notification when
// a file outside home is refused.
func (a *app) openFile(filePath string) {
	refusal := a.openPath(filePath)
	if refusal == nil || refusal.Code != appcore.RefuseFileOutsideHome {
		return
	}
	absPath, _ := filepath.Abs(filePath)
	msg := fmt.Sprintf("%s is outside your home folder. HTML Clay only opens files inside %s.",
		filepath.Base(absPath), a.rt.home)
	go func() {
		if nErr := a.notifyUser("HTML Clay can't open this file", msg); nErr != nil {
			a.rt.logger.Printf("Could not show notification: %v", nErr)
		}
	}()
}

// openPath opens one file the way an OS open does and says why not. The Mac app
// shows the refusal itself (file.open), so this sends no notification.
func (a *app) openPath(filePath string) *appcore.Refusal {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		a.rt.logger.Printf("Error resolving path: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		a.rt.logger.Printf("File not found: %s", absPath)
		return &appcore.Refusal{Code: appcore.RefuseFileMissing}
	}

	absPath, err = resolveSymlinks(absPath)
	if err != nil {
		a.rt.logger.Printf("Error resolving symlinks: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}

	// Refuse an out-of-home file before any site exists. Registration would
	// reject it anyway, but by then a port was bound and a server started.
	if _, ok := session.ContainWithinHome(a.rt.home, absPath); !ok {
		a.rt.logger.Printf("Refusing file outside home: %s", absPath)
		return &appcore.Refusal{Code: appcore.RefuseFileOutsideHome}
	}
	// A file inside a served folder is opened where it is served, not by a site of
	// its own: the served host is its only writer.
	if owner, err := a.ResolveDocument(absPath); err == nil && owner.Kind == OwnerServed {
		a.rt.logger.Printf("Serving %s at %s", filepath.Base(absPath), owner.URL())
		a.noteOpen(absPath, recents.HowServed, true)
		document := filepath.Join(owner.FolderPath, filepath.FromSlash(owner.RelativePath))
		if core, ok := a.served.Wire(owner.FolderID); ok {
			a.applyHelperPlan(core, document, a.helpersForOpen(document, true))
		}
		a.launchBrowser(owner.URL())
		return nil
	}

	helpers := a.helpersForOpen(absPath, true)

	s, rel, ok := a.route(absPath, session.ViaOsOpen)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.applyHelperPlan(s.srv.Core, absPath, helpers)
	a.noteOpen(absPath, recents.HowFile, true)

	target := s.url(rel)
	a.rt.logger.Printf("Serving %s at %s", filepath.Base(absPath), target)
	a.launchBrowser(target)
	return nil
}

func ensureExampleFile(path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(path, exampleHTML, 0644)
	}
	return err
}

func (a *app) openExample() {
	home, err := os.UserHomeDir()
	if err != nil {
		a.rt.logger.Printf("Error resolving home dir: %v", err)
		return
	}
	path := filepath.Join(home, "htmlclay", "examples", "welcome.htmlclay")
	if err := ensureExampleFile(path); err != nil {
		a.rt.logger.Printf("Error creating example file: %v", err)
		return
	}
	a.openFile(path)
}

// openBackups opens the versions folder in the platform file manager. This is the
// discovery mechanism that makes the plain-folder backup design usable: a version
// can be double-clicked straight from Finder.
func (a *app) openBackups() {
	dir, err := a.rt.versions.Dir()
	if err != nil {
		a.rt.logger.Printf("Error resolving backups dir: %v", err)
		return
	}
	if err := browser.OpenURL(dir); err != nil {
		a.rt.logger.Printf("Error opening backups folder: %v", err)
	}
}

func (a *app) launchBrowser(targetURL string) {
	a.rt.logger.Printf("Opening in default browser: %s", targetURL)
	open := browser.OpenURL
	if a.rt.launchBrowser != nil {
		open = a.rt.launchBrowser
	}
	if err := open(targetURL); err != nil {
		a.rt.logger.Printf("Error opening browser: %v", err)
	}
}
