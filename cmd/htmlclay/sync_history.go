package main

import (
	"errors"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/synchistory"
)

// historyKinds maps the engine's file-synced actions (service/sync.cjs
// EVENT_ACTIONS) to a history entry's kind. create is left out: the engine
// reports it only for folders, without saying which side made them.
var historyKinds = map[string]string{
	"download":      synchistory.Downloaded,
	"upload":        synchistory.Uploaded,
	"relocate":      synchistory.Renamed,
	"trash":         synchistory.Removed,
	"delete":        synchistory.Removed,
	"conflict":      synchistory.Conflict,
	"conflict-copy": synchistory.Conflict,
	"resolved":      synchistory.Settled,
}

func (a *app) historyStore() *synchistory.Store {
	a.rt.syncHistoryOnce.Do(func() {
		a.rt.syncHistory = synchistory.New(filepath.Join(a.rt.configDir, "sync-history"))
	})
	return a.rt.syncHistory
}

// keepSyncHistory adds a report's events to each folder's history (8.38). The
// event's id is the entry's, so a report the service sends again adds nothing.
func (a *app) keepSyncHistory(events []served.SyncEvent) {
	byFolder := map[string][]synchistory.Entry{}
	var order []string
	for _, e := range events {
		kind, ok := historyKinds[e.Action]
		if !ok || e.RootID == "" || e.ID == "" {
			continue
		}
		if _, seen := byFolder[e.RootID]; !seen {
			order = append(order, e.RootID)
		}
		byFolder[e.RootID] = append(byFolder[e.RootID], synchistory.Entry{ID: e.ID, At: e.At, Kind: kind, Path: e.File, Detail: e.Action})
	}
	for _, folder := range order {
		if _, err := a.historyStore().Append(folder, byFolder[folder]...); err != nil {
			a.rt.logger.Printf("Could not keep sync history for folder %s: %v", folder, err)
		}
	}
}

// syncHistoryPageSize is how many entries one page of a folder's history holds.
const syncHistoryPageSize = 50

// folderHistory is one page of a synced folder's history, for the dashboard's
// GET /api/app/folders/<id>/history and the Mac app's
// /shell/v1/folders/<id>/history. A folder no sync session keeps is not there.
func (a *app) folderHistory(folderID, before string) (synchistory.Page, bool) {
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	a.rt.syncMu.Unlock()
	if err != nil {
		return synchistory.Page{}, false
	}
	found := false
	for _, s := range record.SyncSessions {
		found = found || s.RootID == folderID
	}
	if !found {
		return synchistory.Page{}, false
	}
	entries, next, err := a.historyStore().Page(folderID, before, syncHistoryPageSize)
	if errors.Is(err, synchistory.ErrFolderID) {
		return synchistory.Page{}, false
	}
	if err != nil {
		a.rt.logger.Printf("Could not read sync history for folder %s: %v", folderID, err)
		entries, next = []synchistory.Entry{}, ""
	}
	page := synchistory.Page{Entries: entries}
	if page.Entries == nil {
		page.Entries = []synchistory.Entry{}
	}
	if next != "" {
		page.Next = &next
	}
	return page, true
}

// syncLogDir is where the service's sync engine writes its daily logs.
func (a *app) syncLogDir() string {
	return filepath.Join(a.rt.configDir, "service", "logs", "sync")
}
