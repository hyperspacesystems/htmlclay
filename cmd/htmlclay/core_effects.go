package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/browser"
	"github.com/panphora/htmlclay/internal/platform"
)

// runEffect runs one effect with the code the app already has for that change.
// An effect whose code is not here yet answers not-yet.
func (a *app) runEffect(ctx context.Context, e appcore.Effect) *appcore.Refusal {
	switch e.Type {
	case "folder.trust":
		canonical, err := a.rt.policy.Canonical(e.Path)
		if err != nil {
			a.rt.logger.Printf("Could not trust folder %s: %v", e.Path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
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
	case "folder.reveal":
		path := a.folderPath(e.FolderID)
		if path == "" {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
		}
		if err := browser.Reveal(path); err != nil {
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
	case "openAtLogin":
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
