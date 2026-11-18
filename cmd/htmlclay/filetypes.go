package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
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
		example, perr := a.welcomeExamplePath()
		if perr == nil {
			err = platform.MakeDefault(exe, example)
		} else {
			err = perr
		}
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
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// welcomeExamplePath writes the welcome example if it is missing and returns
// its path: Windows's chooser needs a .htmlclay file to ask about.
func (a *app) welcomeExamplePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, "htmlclay", "examples", "welcome.htmlclay")
	return path, ensureExampleFile(path)
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
