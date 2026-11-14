package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/browser"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// implementedActions is every action type runEffect has a case for. The snapshot
// reports it as host.can; TestImplementedActionsMatchRunEffect keeps the two equal.
var implementedActions = []string{
	"dashboard.open",
	"document.open",
	"document.reveal",
	"file.open",
	"folder.copyAddress",
	"folder.reveal",
	"folder.serve",
	"folder.stopServing",
	"folder.stopTrusting",
	"folder.trust",
	"host.quit",
	"prefs.set",
	"service.restart",
	"waiting.markSeen",
}

// revealPath shows a folder in the file manager through the runtime seam, so a
// test reads what would be revealed instead of opening a Finder window.
func (a *app) revealPath(path string) error {
	if a.rt.reveal != nil {
		return a.rt.reveal(path)
	}
	return browser.Reveal(path)
}

// runEffect runs one effect with the code the app already has for that change.
// An effect whose code is not here yet answers not-yet.
func (a *app) runEffect(ctx context.Context, e appcore.Effect) *appcore.Refusal {
	switch e.Type {
	case "folder.trust":
		canonical, err := a.rt.policy.Canonical(e.Path)
		if err != nil {
			a.rt.logger.Printf("Could not trust folder %s: %v", e.Path, err)
			return &appcore.Refusal{Code: appcore.RefuseCannotTrust}
		}
		if err := a.trustFolder(canonical); err != nil {
			a.rt.logger.Printf("Could not trust folder %s: %v", canonical, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.stopTrusting":
		path := a.folderPath(e.FolderID)
		if path == "" {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
		}
		if err := a.untrustFolder(path); err != nil {
			a.rt.logger.Printf("Could not stop trusting folder %s: %v", path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.serve":
		_, refusal, err := a.serveFolderNamed(e.Path, e.Name)
		if refusal != nil {
			return refusal
		}
		if err != nil {
			a.rt.logger.Printf("Could not serve folder %s: %v", e.Path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.stopServing":
		if err := a.stopServing(e.FolderID); err != nil {
			var unsaved errUnsavedEdits
			if errors.As(err, &unsaved) {
				return &appcore.Refusal{Code: appcore.RefuseUnsavedEdits, SubjectID: e.FolderID}
			}
			a.rt.logger.Printf("Could not stop serving folder %s: %v", e.FolderID, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "service.restart":
		a.service.restart()
		return nil
	case "host.quit":
		if !e.Force {
			names, answered := a.unsavedDocuments()
			if !answered || len(names) > 0 {
				return &appcore.Refusal{Code: appcore.RefuseQuitUnsaved, Documents: names}
			}
		}
		go func() {
			a.shutdown()
			os.Exit(0)
		}()
		return nil
	case "folder.reveal":
		path := a.folderPath(e.FolderID)
		if path == "" {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
		}
		if err := a.revealPath(path); err != nil {
			a.rt.logger.Printf("Could not show folder %s: %v", path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "folder.copyAddress":
		if err := a.copyServedAddressText(e.FolderID); err != nil {
			a.rt.logger.Printf("Could not copy the address of folder %s: %v", e.FolderID, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "prefs.set":
		return a.setPreference(e)
	case "waiting.markSeen":
		a.core.mu.Lock()
		if a.core.seen == nil {
			a.core.seen = map[string]bool{}
		}
		for _, id := range e.IDs {
			a.core.seen[id] = true
		}
		a.core.mu.Unlock()
		return nil
	case "dashboard.open":
		if a.dashboard == nil {
			return &appcore.Refusal{Code: appcore.RefuseNotYet}
		}
		entry, err := a.dashboard.EntryURL(e.Route)
		if err != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments}
		}
		a.launchBrowser(entry)
		return nil
	case "document.open":
		for _, doc := range a.facts().Documents {
			if doc.ID != e.DocumentID {
				continue
			}
			if doc.URL == "" {
				return &appcore.Refusal{Code: appcore.RefuseNotYet, SubjectID: doc.ID}
			}
			a.launchBrowser(doc.URL)
			return nil
		}
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	case "document.reveal":
		for _, doc := range a.facts().Documents {
			if doc.ID != e.DocumentID {
				continue
			}
			if err := a.revealPath(doc.Path); err != nil {
				a.rt.logger.Printf("Could not show document %s: %v", doc.Path, err)
				return &appcore.Refusal{Code: appcore.RefuseFailed}
			}
			return nil
		}
		return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: e.DocumentID}
	case "file.open":
		return a.openPath(e.Path)
	}
	return &appcore.Refusal{Code: appcore.RefuseNotYet}
}

// folderPath answers the path the facts give a folder id, or "".
func (a *app) folderPath(id string) string {
	for _, ff := range a.facts().Folders {
		if ff.ID == id {
			return ff.Path
		}
	}
	return ""
}

// setPreference applies a prefs.set effect.
func (a *app) setPreference(e appcore.Effect) *appcore.Refusal {
	switch e.Key {
	case "aiEditing":
		var on bool
		if json.Unmarshal(e.Value, &on) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		a.rt.cfg.SetAIEditEnabled(on)
		a.saveConfig("preference aiEditing")
		return nil
	case "appearance":
		var v string
		if json.Unmarshal(e.Value, &v) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		a.rt.cfg.SetAppearance(v)
		a.saveConfig("preference appearance")
		return nil
	case "openAtLogin":
		// The Mac app takes over Open at Login in milestone 7; until then a host it started never writes the login item.
		if a.shell {
			return &appcore.Refusal{Code: appcore.RefuseNotYet}
		}
		var on bool
		if json.Unmarshal(e.Value, &on) != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		if err := a.setLoginItem(on); err != nil {
			a.rt.logger.Printf("Could not set Start on Login: %v", err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		return nil
	case "useName", "yourName":
		profile := a.rt.cfg.ProfileState()
		switch e.Key {
		case "useName":
			if json.Unmarshal(e.Value, &profile.Enabled) != nil {
				return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
			}
			// The tray mints the id when sharing is first turned on; the Mac app's
			// switch needs the same, or turning it on is refused for a missing id.
			if profile.Enabled && !config.ValidProfileID(profile.ID) {
				id, err := config.NewProfileID()
				if err != nil {
					return &appcore.Refusal{Code: appcore.RefuseFailed}
				}
				profile.ID = id
			}
		case "yourName":
			if json.Unmarshal(e.Value, &profile.Name) != nil {
				return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
			}
		}
		if err := a.rt.cfg.UpdateProfile(profile); err != nil {
			return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
		}
		return nil
	}
	return &appcore.Refusal{Code: appcore.RefuseNotYet}
}

// setLoginItem records the Start on Login preference the way the tray's toggle
// does: the system login item first, then the config, and both back if the save
// fails.
func (a *app) setLoginItem(on bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	if execPath == "" {
		return fmt.Errorf("the executable path is empty")
	}
	if err := platform.SetLoginItem(on, execPath); err != nil {
		return err
	}
	previous := a.rt.cfg.StartOnLoginEnabled()
	a.rt.cfg.SetStartOnLogin(on)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetStartOnLogin(previous)
		if backErr := platform.SetLoginItem(previous, execPath); backErr != nil {
			a.rt.logger.Printf("Could not put the login item back: %v", backErr)
		}
		return err
	}
	return nil
}
