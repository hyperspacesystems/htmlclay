package main

import (
	"fmt"
	"os"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tray"
	"github.com/panphora/htmlclay/internal/trust"
)

// selectFolder shows the native folder picker through the runtime seam, so a
// test answers it instead of opening one.
func (a *app) selectFolder(prompt string) (string, bool, error) {
	if a.rt.selectFolder != nil {
		return a.rt.selectFolder(prompt)
	}
	return platform.SelectFolder(prompt)
}

// copyText puts text on the clipboard through the runtime seam, so a test reads
// what was copied instead of writing to the user's clipboard.
func (a *app) copyText(text string) error {
	if a.rt.copyText != nil {
		return a.rt.copyText(text)
	}
	return platform.CopyText(text)
}

// servedFolderRows returns the Served Folders menu's rows: one per folder that is
// being served, labelled with its name and path, and marked when the folder is
// gone or no longer the one that was served.
func (a *app) servedFolderRows() []tray.Row {
	list := a.rt.cfg.ServedFolderList()
	out := make([]tray.Row, 0, len(list))
	for _, f := range list {
		if !f.Serving {
			continue
		}
		label := f.Name + "  " + a.tildePath(f.Path)
		if info, err := os.Stat(f.Path); err != nil || !info.IsDir() || !trust.IdentityOK(f.Path, f.Identity, a.rt.home) {
			label += " " + appcore.TextFolderMissing
		}
		out = append(out, tray.Row{Path: f.ID, Label: label, Serving: true})
	}
	return out
}

// pickAndServeFolder is the tray's serve hook: it asks for a folder, serves it
// under a default name, and says where it now answers. Serving changes what owns
// the folder's files, so the person hears about the result whether it worked or
// not; the notification is detached because it may block.
func (a *app) pickAndServeFolder() []tray.Row {
	dir, ok, err := a.selectFolder(appcore.TextPickServeFolder)
	if err != nil {
		a.rt.logger.Printf("Folder picker failed: %v", err)
		return a.servedFolderRows()
	}
	if !ok {
		return a.servedFolderRows()
	}
	entry, refusal, err := a.serveFolderNamed(dir, "")
	if refusal != nil || err != nil {
		msg := ""
		if refusal != nil {
			msg = appcore.RefusalMessage(refusal)
		} else {
			msg = err.Error()
		}
		a.rt.logger.Printf("Could not serve folder %s: %s", dir, msg)
		go func() {
			if nErr := a.notifyUser(appcore.TextCannotServe, msg); nErr != nil {
				a.rt.logger.Printf("Could not show notification: %v", nErr)
			}
		}()
		return a.servedFolderRows()
	}
	address := a.servedAddress(entry)
	go func() {
		if nErr := a.notifyUser("HTML Clay", fmt.Sprintf(appcore.TextNowServed, entry.Name, address)); nErr != nil {
			a.rt.logger.Printf("Could not show notification: %v", nErr)
		}
	}()
	a.launchBrowser(address + "/")
	return a.servedFolderRows()
}

// openServedFolder opens a served folder's address in the browser.
func (a *app) openServedFolder(id string) {
	f, ok := a.rt.cfg.ServedFolderByID(id)
	if !ok {
		return
	}
	a.launchBrowser(a.servedAddress(f) + "/")
}

// copyServedAddress puts a served folder's address on the clipboard, logging the
// failure when no clipboard tool is installed.
func (a *app) copyServedAddress(id string) {
	f, ok := a.rt.cfg.ServedFolderByID(id)
	if !ok {
		return
	}
	if err := a.copyText(a.servedAddress(f) + "/"); err != nil {
		a.rt.logger.Printf("Could not copy the address of served folder %s: %v", f.Name, err)
	}
}

// stopServedFromTray stops serving a folder after asking first, because its
// address stops answering and its pages need opening again.
func (a *app) stopServedFromTray(id string) []tray.Row {
	f, ok := a.rt.cfg.ServedFolderByID(id)
	if !ok {
		return a.servedFolderRows()
	}
	proceed, err := a.confirmTrayChange(appcore.TextStopServingTitle, fmt.Sprintf(appcore.TextStopServingMessage, a.servedAddress(f)), appcore.TextStopServing)
	if err != nil || !proceed {
		return a.servedFolderRows()
	}
	if err := a.stopServing(id); err != nil {
		a.rt.logger.Printf("Could not stop serving folder %s: %v", f.Path, err)
	}
	return a.servedFolderRows()
}
