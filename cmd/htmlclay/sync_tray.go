package main

import (
	"fmt"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/tray"
)

const (
	syncTrayPause     = "pause"
	syncTrayResume    = "resume"
	syncTrayConflicts = "conflicts"
)

// syncTrayItems is what the Windows and Linux tray shows for sync (08-sync.md
// 8.47): nothing until connected, then Pause or Resume, and the held conflicts
// when there are any, which open Needs you.
func syncTrayItems(s appcore.Snapshot) []tray.SyncItem {
	if !s.Sync.Connected {
		return nil
	}
	items := []tray.SyncItem{{Action: syncTrayPause, Title: appcore.TextTrayPauseSyncing}}
	if s.Sync.Paused {
		items[0] = tray.SyncItem{Action: syncTrayResume, Title: appcore.TextTrayResumeSyncing}
	}
	conflicts := 0
	for _, item := range s.Waiting {
		if item.Kind == appcore.WaitingConflict {
			conflicts++
		}
	}
	switch {
	case conflicts == 1:
		items = append(items, tray.SyncItem{Action: syncTrayConflicts, Title: appcore.TextTrayOneConflict})
	case conflicts > 1:
		items = append(items, tray.SyncItem{Action: syncTrayConflicts, Title: fmt.Sprintf(appcore.TextTrayConflicts, conflicts)})
	}
	return items
}

// doSyncTray runs a sync tray item through the same actions the Mac app sends.
func (a *app) doSyncTray(action string) {
	var refusal *appcore.Refusal
	switch action {
	case syncTrayPause:
		refusal = a.trayDo("sync.setPaused", map[string]any{"paused": true})
	case syncTrayResume:
		refusal = a.trayDo("sync.setPaused", map[string]any{"paused": false})
	case syncTrayConflicts:
		refusal = a.trayDo("dashboard.open", map[string]any{"route": "/needs-you"})
	default:
		return
	}
	if refusal != nil {
		a.rt.logger.Printf("Tray %s: %s", action, appcore.RefusalMessage(refusal))
		_ = a.notifyUser(appcore.RefusalMessage(refusal), "")
	}
}
