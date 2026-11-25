package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
)

type fileTypeCache struct {
	mu    sync.Mutex
	state platform.FileTypeState
}

// managesFileTypes is true where the host owns file types: Windows and Linux.
// The Mac app sets its own.
func (a *app) managesFileTypes() bool {
	return !a.shell && (runtime.GOOS == "windows" || runtime.GOOS == "linux")
}

// refreshFileTypeState asks the system again and publishes a snapshot.
func (a *app) refreshFileTypeState() {
	if !a.managesFileTypes() {
		return
	}
	state := platform.ReadFileTypes()
	a.fileTypes.mu.Lock()
	a.fileTypes.state = state
	a.fileTypes.mu.Unlock()
	a.changed()
}

func (a *app) fileTypeFacts() appcore.FileTypes {
	if !a.managesFileTypes() {
		return appcore.FileTypes{Default: appcore.DefaultAppUnknown}
	}
	a.fileTypes.mu.Lock()
	state := a.fileTypes.state
	a.fileTypes.mu.Unlock()
	if state.Default == "" {
		state.Default = appcore.DefaultAppUnknown
	}
	return appcore.FileTypes{
		Managed:      true,
		Default:      state.Default,
		HTMLOpenWith: state.HTMLOpenWith,
		Stopped:      a.rt.cfg.StoppedOpeningFiles(),
	}
}

func (a *app) runFileTypeEffect(e appcore.Effect) *appcore.Refusal {
	if !a.managesFileTypes() {
		return &appcore.Refusal{Code: appcore.RefuseNotOnThisSystem}
	}
	exe, err := os.Executable()
	if err != nil {
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	switch e.Type {
	case "fileTypes.makeDefault":
		a.rt.cfg.SetStopOpeningFiles(false)
		err = platform.MakeDefault(exe)
	case "fileTypes.stopOpening":
		if err = platform.StopOpening(); err == nil {
			a.rt.cfg.SetStopOpeningFiles(true)
		}
	case "fileTypes.setHTMLOpenWith":
		if err = platform.SetHTMLOpenWith(exe, e.On); err == nil {
			a.rt.cfg.SetHTMLOpenWith(e.On)
		}
	}
	a.saveConfig("file types")
	go a.refreshFileTypeState()
	if err != nil {
		a.rt.logger.Printf("Could not change file types (%s): %v", e.Type, err)
		return fileTypeRefusal(err)
	}
	return nil
}

// fileTypeRefusal is the answer to a file type change that did not finish:
// Make default on Windows sending the person to Settings, or a failure.
func fileTypeRefusal(err error) *appcore.Refusal {
	if errors.Is(err, platform.ErrFinishInSettings) {
		return &appcore.Refusal{Code: appcore.RefuseFinishInSettings}
	}
	return &appcore.Refusal{Code: appcore.RefuseFailed}
}

// welcomeExampleName is the welcome example that Make default wrote into
// ~/htmlclay/examples before 2.0, for Windows's chooser; the examples folder
// check still expects it there.
const welcomeExampleName = "welcome.htmlclay"

// welcomeExamplePath writes the welcome example if it is missing and returns
// its path. A welcome example HTML Clay wrote is the person's own.
func (a *app) welcomeExamplePath() (string, error) {
	path := filepath.Join(a.appDataDir(), "examples", welcomeExampleName)
	_, statErr := os.Stat(path)
	missing := errors.Is(statErr, os.ErrNotExist)
	if err := ensureExampleFile(path); err != nil {
		return "", err
	}
	if missing {
		if err := a.recordMadeDocument(path, tier.HowCreated); err != nil {
			a.rt.logger.Printf("Could not record %s as yours: %v", path, err)
		}
	}
	return path, nil
}

// fileTypesEvery is how often the system's file type choice is read again, so a
// person who changed the default in the system's own settings sees it here.
const fileTypesEvery = 60 * time.Second

// watchFileTypes keeps the file type state fresh until stop closes.
func (a *app) watchFileTypes(stop <-chan struct{}) {
	t := time.NewTicker(fileTypesEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			a.refreshFileTypeState()
		}
	}
}
