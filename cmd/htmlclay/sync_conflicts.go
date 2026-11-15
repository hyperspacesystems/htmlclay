package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// The engine keeps each session's held conflicts in conflicts.json in the
// session's meta dir: <config>/service/sync-meta/v2/<session id>/, or the dir a
// session moved from Hyperclay Local still uses (its row's legacyMetaDir). Go
// reads that file, at start and after a report that says it changed, and never
// writes it: the engine owns the records.

// heldConflict is one record of conflicts.json, keyed by node id or path.
type heldConflict struct {
	Kind       string      `json:"kind"`
	Path       string      `json:"path"`
	DetectedAt json.Number `json:"detectedAt"`
	// RemoteCopy is where the engine kept the server's bytes, relative to the
	// folder (.hyperclay/conflicts/<name>), or empty when none were kept.
	RemoteCopy string `json:"remoteCopy"`
}

// heldEntry is one held conflict with the session it belongs to.
type heldEntry struct {
	session config.SyncSession
	key     string
	record  heldConflict
}

// id is the conflict's id everywhere: its waiting item, the dashboard and
// conflict.resolve.
func (e heldEntry) id() string {
	return e.session.ID + ":" + e.key + ":" + e.record.DetectedAt.String()
}

// syncConflicts is what Go last read, per session: the waiting facts, and
// whether a file that did not parse has been logged.
type syncConflicts struct {
	facts     map[string][]appcore.EventFact
	badLogged map[string]bool
}

func (a *app) conflictsMetaDir(session config.SyncSession) string {
	if session.LegacyMetaDir != nil && *session.LegacyMetaDir != "" {
		return *session.LegacyMetaDir
	}
	return filepath.Join(a.rt.configDir, "service", "sync-meta", "v2", session.ID)
}

// loadSyncAtStart reads sync.json and every session's held conflicts, so both
// show before the service runs or reports.
func (a *app) loadSyncAtStart() {
	a.rt.syncMu.Lock()
	_, err := a.syncRecordLocked()
	a.rt.syncMu.Unlock()
	if err != nil {
		a.rt.logger.Printf("sync.json could not be read: %v", err)
		return
	}
	a.readHeldConflicts()
}

// readHeldConflicts reads every session's conflicts.json. A missing file is no
// conflicts; a file that does not parse keeps the list read before and is logged
// once until it reads again.
func (a *app) readHeldConflicts() {
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	var sessions []config.SyncSession
	if err == nil {
		sessions = append(sessions, record.SyncSessions...)
	}
	a.rt.syncMu.Unlock()
	if err != nil {
		return
	}
	type read struct {
		facts []appcore.EventFact
		bad   error
	}
	reads := make(map[string]read, len(sessions))
	for _, session := range sessions {
		facts, err := readConflictsFile(filepath.Join(a.conflictsMetaDir(session), "conflicts.json"), session)
		reads[session.ID] = read{facts: facts, bad: err}
	}

	a.rt.syncMu.Lock()
	previous := a.rt.syncConflicts
	next := syncConflicts{facts: make(map[string][]appcore.EventFact, len(reads)), badLogged: make(map[string]bool)}
	for id, r := range reads {
		if r.bad == nil {
			next.facts[id] = r.facts
			continue
		}
		next.facts[id] = previous.facts[id]
		next.badLogged[id] = true
		if !previous.badLogged[id] {
			a.rt.logger.Printf("The held conflicts of sync session %s did not read; keeping the last list: %v", id, r.bad)
		}
	}
	a.rt.syncConflicts = next
	known := map[string]bool{}
	for _, facts := range previous.facts {
		for _, f := range facts {
			known[f.ID] = true
		}
	}
	var fresh []appcore.EventFact
	for _, facts := range next.facts {
		for _, f := range facts {
			if !known[f.ID] {
				fresh = append(fresh, f)
			}
		}
	}
	a.rt.syncMu.Unlock()
	for _, f := range fresh {
		folder := f.SubjectID
		a.notice(shellapi.Notice{ID: f.ID, Kind: noticeConflict, FolderID: &folder, Name: f.Name, Detail: f.Detail})
	}
}

func readConflictsFile(file string, session config.SyncSession) ([]appcore.EventFact, error) {
	entries, err := readHeldEntries(file, session)
	if err != nil {
		return nil, err
	}
	facts := make([]appcore.EventFact, 0, len(entries))
	for _, e := range entries {
		facts = append(facts, appcore.EventFact{
			ID:        e.id(),
			Kind:      appcore.WaitingConflict,
			SubjectID: session.RootID,
			Name:      e.record.Path,
			Detail:    e.record.Kind,
		})
	}
	return facts, nil
}

// readHeldEntries reads one session's conflicts.json, in key order. A missing
// file is no conflicts.
func readHeldEntries(file string, session config.SyncSession) ([]heldEntry, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records map[string]heldConflict
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]heldEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, heldEntry{session: session, key: key, record: records[key]})
	}
	return entries, nil
}

// heldEntries reads every session's held conflicts from disk now. A file that
// does not read is left out; readHeldConflicts is the one that logs it.
func (a *app) heldEntries() []heldEntry {
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	var sessions []config.SyncSession
	if err == nil {
		sessions = append(sessions, record.SyncSessions...)
	}
	a.rt.syncMu.Unlock()
	var all []heldEntry
	for _, session := range sessions {
		entries, err := readHeldEntries(filepath.Join(a.conflictsMetaDir(session), "conflicts.json"), session)
		if err == nil {
			all = append(all, entries...)
		}
	}
	return all
}

// heldCopyFor is served.Options.HeldCopy: the kept server copy of rel's held
// conflict in folder rootID.
func (a *app) heldCopyFor(rootID, rel string) (string, bool) {
	for _, e := range a.heldEntries() {
		if e.session.RootID == rootID && e.record.Path == rel && e.record.RemoteCopy != "" {
			return e.record.RemoteCopy, true
		}
	}
	return "", false
}

// dashboardConflict is one held conflict as GET /api/app/conflicts lists it
// (contract C7): both versions' addresses, and the choices its kind allows.
type dashboardConflict struct {
	ID         string   `json:"id"`
	FolderID   string   `json:"folderID"`
	FolderName string   `json:"folderName"`
	Path       string   `json:"path"`
	Kind       string   `json:"kind"`
	DetectedAt string   `json:"detectedAt"`
	Keep       []string `json:"keep"`
	MineURL    *string  `json:"mineURL"`
	TheirsURL  *string  `json:"theirsURL"`
}

// dashboardConflicts lists every held conflict. mineURL is null for a file that
// is gone here, theirsURL when no server copy was kept; both are null for a
// folder that is not served.
func (a *app) dashboardConflicts() []dashboardConflict {
	folders := make(map[string]appcore.Folder)
	for _, f := range a.snapshot().Folders {
		folders[f.ID] = f
	}
	list := []dashboardConflict{}
	for _, e := range a.heldEntries() {
		folder := folders[e.session.RootID]
		item := dashboardConflict{
			ID:         e.id(),
			FolderID:   e.session.RootID,
			FolderName: folder.Name,
			Path:       e.record.Path,
			Kind:       e.record.Kind,
			Keep:       appcore.ConflictChoices(e.record.Kind),
		}
		if item.Keep == nil {
			item.Keep = []string{}
		}
		if ms, err := e.record.DetectedAt.Int64(); err == nil {
			item.DetectedAt = time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
		}
		if folder.URL != "" && folder.Path != "" {
			base := strings.TrimSuffix(folder.URL, "/") + "/"
			rel := escapePathSegments(e.record.Path)
			if fileExists(filepath.Join(folder.Path, filepath.FromSlash(e.record.Path))) {
				mine := base + rel
				item.MineURL = &mine
			}
			if e.record.RemoteCopy != "" && fileExists(filepath.Join(folder.Path, filepath.FromSlash(e.record.RemoteCopy))) {
				theirs := base + "_/sync/theirs/" + rel
				item.TheirsURL = &theirs
			}
		}
		list = append(list, item)
	}
	return list
}

func escapePathSegments(rel string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func fileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && info.Mode().IsRegular()
}

// conflictsChanged reports whether a report means the held conflicts changed: a
// session's count moved, or an event names a conflict or its end.
func conflictsChanged(previous *served.SyncReport, report served.SyncReport) bool {
	for _, event := range report.Events {
		if event.Action == "conflict" || event.Action == "resolved" {
			return true
		}
	}
	before := map[string]int{}
	if previous != nil {
		for _, session := range previous.Sessions {
			before[session.SessionID] = session.Conflicts
		}
	}
	for _, session := range report.Sessions {
		if before[session.SessionID] != session.Conflicts {
			return true
		}
		delete(before, session.SessionID)
	}
	for _, count := range before {
		if count != 0 {
			return true
		}
	}
	return false
}
