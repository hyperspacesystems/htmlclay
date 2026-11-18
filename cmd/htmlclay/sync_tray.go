package main

import (
	"fmt"

	"github.com/panphora/htmlclay/internal/appcore"
)

const (
	syncTrayPause     = "pause"
	syncTrayResume    = "resume"
	syncTrayConflicts = "conflicts"
)

// syncTrayItem is one sync row the app can offer: Action is passed back to
// doSyncTray.
type syncTrayItem struct {
	Action string
	Title  string
}

// syncTrayItems is what the Windows and Linux tray shows for sync (08-sync.md
// 8.47): nothing until connected, then Pause or Resume, and the held conflicts
// when there are any, which open Needs you. The one menu's rows carry these now
// (appcore.MenuRows); this is what they offer a caller that reads the sync list
// directly.
func syncTrayItems(s appcore.Snapshot) []syncTrayItem {
	if !s.Sync.Connected {
		return nil
	}
	items := []syncTrayItem{{Action: syncTrayPause, Title: appcore.TextTrayPauseSyncing}}
	if s.Sync.Paused {
		items[0] = syncTrayItem{Action: syncTrayResume, Title: appcore.TextTrayResumeSyncing}
	}
	conflicts := 0
	for _, item := range s.Waiting {
		if item.Kind == appcore.WaitingConflict {
			conflicts++
		}
	}
	switch {
	case conflicts == 1:
		items = append(items, syncTrayItem{Action: syncTrayConflicts, Title: appcore.TextTrayOneConflict})
	case conflicts > 1:
		items = append(items, syncTrayItem{Action: syncTrayConflicts, Title: fmt.Sprintf(appcore.TextTrayConflicts, conflicts)})
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
	a.reportTrayRefusal(refusal)
}
