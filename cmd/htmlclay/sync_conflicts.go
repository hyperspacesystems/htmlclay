package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/served"
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
	defer a.rt.syncMu.Unlock()
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
}

func readConflictsFile(file string, session config.SyncSession) ([]appcore.EventFact, error) {
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
	facts := make([]appcore.EventFact, 0, len(keys))
	for _, key := range keys {
		r := records[key]
		facts = append(facts, appcore.EventFact{
			ID:        session.ID + ":" + key + ":" + r.DetectedAt.String(),
			Kind:      appcore.WaitingConflict,
			SubjectID: session.RootID,
			Name:      r.Path,
			Detail:    r.Kind,
		})
	}
	return facts, nil
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
