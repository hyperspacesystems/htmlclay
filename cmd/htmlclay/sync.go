package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/secret"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/service"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// syncKeyName is the sync key's item name in the secret store.
const syncKeyName = "sync-key"

// defaultSyncServer is where sync goes, fixed in the build: an override a shipped
// binary honoured would let anything that can set an environment variable send
// the key elsewhere. Tests in this package set syncServerForTest; only a binary
// built with the tag synctest also reads an address from its environment
// (sync_server_synctest.go). The service accepts plain http only on loopback.
const defaultSyncServer = "https://hyperclay.com"

var syncServerForTest string

func syncServerURL() string {
	if syncServerForTest != "" {
		return syncServerForTest
	}
	if v := syncServerFromBuild(); v != "" {
		return v
	}
	return defaultSyncServer
}

// syncProfile names this install's items in the secret store: "default" for the
// standard config folder, and a short hash of the folder for any other, such as
// a candidate build or HTMLCLAY_CONFIG_BASE, so two installs never share a key.
func syncProfile(configDir string) string {
	if base, err := os.UserConfigDir(); err == nil {
		standard := filepath.Join(base, "htmlclay")
		if resolved, err := filepath.EvalSymlinks(standard); err == nil {
			standard = resolved
		}
		if configDir == standard {
			return "default"
		}
	}
	sum := sha256.Sum256([]byte(configDir))
	return "c" + hex.EncodeToString(sum[:6])
}

// openSyncStore answers this install's secret store, or nil with the failure
// logged: sync then stays off, and nothing else changes. HTMLCLAY_SECRET_STORE=file
// keeps the key in a private file in the config folder instead, so a test or a
// throwaway profile never touches the system's store.
func (a *app) openSyncStore() secret.Store {
	if os.Getenv("HTMLCLAY_SECRET_STORE") == "file" {
		return secret.NewFile(filepath.Join(a.rt.configDir, "secrets"))
	}
	store, err := secret.Open(syncProfile(a.rt.configDir), a.rt.configDir)
	if err != nil {
		a.rt.logger.Printf("The sync key store could not be opened: %v", err)
		return nil
	}
	return store
}

// syncKey answers the server and the key the service should hold, when the
// store has one. A failed read is logged without any part of the key.
func (a *app) syncKey() (serverURL, key string, ok bool) {
	if a.rt.syncStore == nil {
		return "", "", false
	}
	key, found, err := a.rt.syncStore.Get(syncKeyName)
	if err != nil {
		a.rt.logger.Printf("The sync key could not be read: %v", err)
		return "", "", false
	}
	if !found {
		return "", "", false
	}
	return syncServerURL(), key, true
}

// syncKeyShape is a sync key as hyperclay.com makes it.
var syncKeyShape = regexp.MustCompile(`^hcsk_[0-9a-f]{64}$`)

// syncCallTimeout bounds one call to the service's sync routes.
const syncCallTimeout = 30 * time.Second

// syncDiscovery is the service's /sync/v1/discover answer (08-sync.md, C3).
type syncDiscovery struct {
	OK     bool   `json:"ok"`
	Code   string `json:"code"`
	Status int    `json:"status"`
	Actor  struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		FullName string `json:"fullName"`
	} `json:"actor"`
	Accounts []struct {
		ID          int64   `json:"id"`
		Kind        string  `json:"kind"`
		Username    string  `json:"username"`
		DisplayName string  `json:"displayName"`
		Role        string  `json:"role"`
		WebURL      *string `json:"webUrl"`
		Sync        struct {
			Enabled bool    `json:"enabled"`
			Reason  *string `json:"reason"`
		} `json:"sync"`
	} `json:"accounts"`
}

// syncRecordLocked answers sync.json, read once. The caller holds a.rt.syncMu.
func (a *app) syncRecordLocked() (*config.Sync, error) {
	if a.rt.syncRecord == nil {
		record, err := config.LoadSync(a.rt.configDir)
		if err != nil {
			return nil, err
		}
		a.rt.syncRecord = record
		a.rt.syncHeld.Store(record.Account != nil)
	}
	return a.rt.syncRecord, nil
}

// startServiceForSync asks for the service while a connect holds syncConnecting
// and waits for it to run, or answers nil when this build has none or it does not
// start in time.
func (a *app) startServiceForSync() *service.Process {
	if !a.service.enabled() {
		return nil
	}
	a.service.want()
	deadline := time.Now().Add(serviceSpawnTimeout)
	for time.Now().Before(deadline) {
		if proc := a.runningService(); proc != nil {
			return proc
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// runningService answers the live-room service process when it is running, or
// nil: sync runs inside it.
func (a *app) runningService() *service.Process {
	if a.service == nil || a.service.state() != appcore.ServiceRunning {
		return nil
	}
	return a.service.process()
}

// connectSync is sync.connect: the key goes to the service, which asks the server
// who it belongs to; only a key the server accepts, for the account the person
// named (if they named one), is stored. Any failure leaves the service holding
// the key it held before and stores nothing.
// connectSync sends the wanted sessions whatever the outcome: the service stops
// every engine when it is handed a different key, so a connect that fails after
// sending the new key and puts the previous one back must start them again.
func (a *app) connectSync(ctx context.Context, e appcore.Effect) *appcore.Refusal {
	refusal := a.connectSyncKey(ctx, e)
	a.sendSyncWanted()
	return refusal
}

func (a *app) connectSyncKey(ctx context.Context, e appcore.Effect) *appcore.Refusal {
	if !syncKeyShape.MatchString(e.Key) {
		return &appcore.Refusal{Code: appcore.RefuseSyncKeyShape, Field: "key"}
	}
	if a.rt.syncStore == nil {
		a.rt.logger.Printf("Could not connect sync: there is no secret store")
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	// A first connect may come before any folder is served, when the service is
	// not running: it starts for the connect and stays while the key is held.
	a.rt.syncConnecting.Add(1)
	defer func() {
		a.rt.syncConnecting.Add(-1)
		a.service.want()
	}()
	proc := a.runningService()
	if proc == nil {
		proc = a.startServiceForSync()
	}
	if proc == nil {
		return &appcore.Refusal{Code: appcore.RefuseServiceNeeded}
	}
	a.rt.syncActionMu.Lock()
	defer a.rt.syncActionMu.Unlock()
	restore := func() {
		var err error
		if serverURL, previous, ok := a.syncKey(); ok {
			err = proc.SendSyncKey(serverURL, previous, serviceSpawnTimeout)
		} else {
			err = proc.ClearSyncKey(serviceSpawnTimeout)
		}
		if err != nil {
			a.rt.logger.Printf("The live-room service did not take back its previous sync key: %v", err)
			a.service.restart()
		}
	}
	if err := proc.SendSyncKey(syncServerURL(), e.Key, serviceSpawnTimeout); err != nil {
		a.rt.logger.Printf("Could not connect sync: %v", err)
		restore()
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	callCtx, cancel := context.WithTimeout(ctx, syncCallTimeout)
	defer cancel()
	var found syncDiscovery
	if err := a.served.CallService(callCtx, "/sync/v1/discover", struct{}{}, &found); err != nil {
		restore()
		if errors.Is(err, served.ErrServiceUnavailable) {
			return &appcore.Refusal{Code: appcore.RefuseServiceNeeded}
		}
		a.rt.logger.Printf("Could not connect sync: discovery: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if !found.OK {
		restore()
		switch found.Code {
		case "key-refused":
			return &appcore.Refusal{Code: appcore.RefuseSyncKeyRefused, Field: "key"}
		case "offline":
			return &appcore.Refusal{Code: appcore.RefuseSyncOffline}
		case "server-update-required":
			return &appcore.Refusal{Code: appcore.RefuseSyncServerUpdate}
		}
		a.rt.logger.Printf("Could not connect sync: discovery answered %q", found.Code)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	typed := strings.TrimPrefix(strings.TrimSpace(e.Username), "@")
	if typed != "" && !strings.EqualFold(typed, found.Actor.Username) {
		restore()
		return &appcore.Refusal{Code: appcore.RefuseSyncWrongAccount, Field: "username"}
	}
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.logger.Printf("Could not connect sync: sync.json: %v", err)
		restore()
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if err := a.rt.syncStore.Set(syncKeyName, e.Key); err != nil {
		a.rt.logger.Printf("Could not connect sync: storing the key: %v", err)
		restore()
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	record.Account = &config.SyncAccount{
		ActorID:     found.Actor.ID,
		Username:    found.Actor.Username,
		FullName:    found.Actor.FullName,
		KeySuffix:   e.Key[len(e.Key)-4:],
		KeyStore:    a.rt.syncStore.Kind(),
		ConnectedAt: time.Now().UTC().Format(time.RFC3339),
	}
	record.KeyRefused = nil
	a.rt.syncHeld.Store(true)
	record.Accounts = a.mergeAccountRows(record.Accounts, found)
	if err := record.Save(); err != nil {
		a.rt.logger.Printf("Could not connect sync: sync.json: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// mergeAccountRows answers discovery's rows in its order, each keeping the folder
// id it was given the first time it was seen.
func (a *app) mergeAccountRows(previous []config.SyncAccountRow, found syncDiscovery) []config.SyncAccountRow {
	folderOf := make(map[int64]string, len(previous))
	for _, row := range previous {
		folderOf[row.AccountID] = row.FolderID
	}
	rows := make([]config.SyncAccountRow, 0, len(found.Accounts))
	for _, account := range found.Accounts {
		folderID := folderOf[account.ID]
		if folderID == "" {
			id, err := a.rt.cfg.NewFolderID()
			if err != nil {
				a.rt.logger.Printf("Could not give account %s a folder id: %v", account.Username, err)
				continue
			}
			folderID = id
		}
		rows = append(rows, config.SyncAccountRow{
			AccountID:   account.ID,
			Kind:        account.Kind,
			Username:    account.Username,
			DisplayName: account.DisplayName,
			Role:        account.Role,
			FolderID:    folderID,
			Sync:        config.SyncAccountSync{Enabled: account.Sync.Enabled, Reason: account.Sync.Reason},
			WebURL:      account.WebURL,
		})
	}
	return rows
}

// disconnectSync is sync.disconnect: the service drops the key, the store
// forgets it, and the account goes. Every session row stays, switched off, so
// each folder's marker stays known for a later reconnect.
func (a *app) disconnectSync() *appcore.Refusal {
	// With no folder served, the service is no longer needed once the key is gone.
	defer a.service.want()
	a.rt.syncActionMu.Lock()
	defer a.rt.syncActionMu.Unlock()
	if proc := a.runningService(); proc != nil {
		if err := proc.ClearSyncKey(serviceSpawnTimeout); err != nil {
			a.rt.logger.Printf("The live-room service did not drop the sync key, so it restarts: %v", err)
			a.service.restart()
		}
	}
	if a.rt.syncStore != nil {
		if err := a.rt.syncStore.Delete(syncKeyName); err != nil {
			a.rt.logger.Printf("Could not disconnect sync: removing the key: %v", err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
	}
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.logger.Printf("Could not disconnect sync: sync.json: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	record.Account = nil
	record.KeyRefused = nil
	a.rt.syncHeld.Store(false)
	for i := range record.SyncSessions {
		record.SyncSessions[i].Enabled = false
	}
	if err := record.Save(); err != nil {
		a.rt.logger.Printf("Could not disconnect sync: sync.json: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// syncFacts answers what sync adds to the facts: the account, its account and
// team rows as folders not on this computer, and the events sync owes the
// person (a team to set up, a refused key). A sync.json this build cannot read
// reads as not connected.
func (a *app) syncFacts() (appcore.SyncFacts, []appcore.FolderFact, []appcore.EventFact, map[string]syncedFolder) {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil || record.Account == nil {
		return appcore.SyncFacts{}, nil, nil, nil
	}
	facts := appcore.SyncFacts{
		Connected: true,
		Username:  record.Account.Username,
		FullName:  record.Account.FullName,
		KeySuffix: record.Account.KeySuffix,
		KeyStore:  record.Account.KeyStore,
		Paused:    record.Paused,
	}
	// A session row puts its account's folder on this computer, synced or stopped:
	// that folder is listed from the config, so the row is not listed again.
	onComputer := make(map[string]bool, len(record.SyncSessions))
	enabled := make(map[string]bool, len(record.SyncSessions))
	pausedRow := make(map[string]string, len(record.SyncSessions))
	downloading := make(map[string]bool, len(record.SyncSessions))
	for _, session := range record.SyncSessions {
		onComputer[session.RootID] = true
		enabled[session.RootID] = enabled[session.RootID] || session.Enabled
		if session.Paused != nil {
			pausedRow[session.RootID] = session.Paused.Reason
		}
		if a.rt.syncBinding[session.ID] {
			downloading[session.RootID] = true
		}
	}
	reported := make(map[string]served.SyncSessionReport)
	if a.rt.syncLast != nil {
		for _, session := range a.rt.syncLast.Sessions {
			reported[session.RootID] = session
			facts.SyncingFiles += session.PendingCount
			if at := syncTime(session.LastSyncAt); at != nil && (facts.LastFinishedAt == nil || at.After(*facts.LastFinishedAt)) {
				facts.LastFinishedAt = at
			}
		}
	}
	synced := make(map[string]syncedFolder, len(onComputer))
	var folders []appcore.FolderFact
	var events []appcore.EventFact
	for _, row := range record.Accounts {
		team := row.Kind == "team"
		viewer := row.Role == "viewer"
		if team && viewer {
			facts.ViewOnlyTeams++
		}
		origin := appcore.Origin{Kind: appcore.OriginAccount, Username: row.Username}
		if team {
			origin = appcore.Origin{Kind: appcore.OriginTeam, Username: record.Account.Username, Team: row.Username}
		}
		if onComputer[row.FolderID] {
			report, hasReport := reported[row.FolderID]
			reason := pausedRow[row.FolderID]
			if hasReport {
				if pause := syncPauseReason(report.Paused); pause != "" {
					reason = pause
				}
			}
			// A demotion pauses the session with viewer at once; the account rows
			// learn the new role only at the next discovery, up to five minutes later.
			role := row.Role
			if reason == "viewer" && !viewer {
				viewer, role = true, "viewer"
				if team {
					facts.ViewOnlyTeams++
				}
			}
			state := appcore.FolderSyncUpToDate
			switch {
			case !enabled[row.FolderID]:
				state = appcore.FolderSyncOff
			case record.Paused || reason != "":
				state = appcore.FolderSyncPaused
			case hasReport && report.Conflicts > 0:
				state = appcore.FolderSyncConflicts
			case downloading[row.FolderID] || hasReport && (report.Status == "syncing" || report.PendingCount > 0):
				state = appcore.FolderSyncSyncing
			}
			folderSync := appcore.FolderSync{State: state, Role: role}
			folderSync.UploadedToday, folderSync.DownloadedToday = a.historyStore().Today(row.FolderID, time.Now())
			if hasReport {
				folderSync.LastFinishedAt = syncTime(report.LastSyncAt)
				if p := report.Progress; p != nil && p.Phase == "download" && p.Total > 0 && p.Done < p.Total {
					folderSync.Download = &appcore.SyncDownload{Done: p.Done, Total: p.Total, BytesDone: p.BytesDone, BytesTotal: p.BytesTotal}
				}
			}
			synced[row.FolderID] = syncedFolder{Enabled: enabled[row.FolderID], Origin: origin, Viewer: viewer, Sync: folderSync}
			name := row.DisplayName
			if name == "" {
				name = row.Username
			}
			// A file too big for the plan waits for the person once per folder. A
			// missing folder is already a waiting item from the folder facts (the disk
			// check), so its pause only sets the state here; a revoked key is one item
			// for the whole app, below, however many folders it paused.
			if enabled[row.FolderID] && hasReport {
				notText, nameClash := report.BlockedBy["not-text"], report.BlockedBy["case-twin"]
				if tooBig := report.Blocked - notText - nameClash; tooBig > 0 {
					events = append(events, appcore.EventFact{ID: "fileTooBig:" + row.FolderID, Kind: appcore.WaitingFileTooBig, SubjectID: row.FolderID, Name: name, Detail: strconv.Itoa(tooBig)})
				}
				if notText > 0 {
					events = append(events, appcore.EventFact{ID: "fileNotText:" + row.FolderID, Kind: appcore.WaitingFileNotText, SubjectID: row.FolderID, Name: name, Detail: strconv.Itoa(notText)})
				}
				if nameClash > 0 {
					events = append(events, appcore.EventFact{ID: "nameClash:" + row.FolderID, Kind: appcore.WaitingNameClash, SubjectID: row.FolderID, Name: name, Detail: strconv.Itoa(nameClash)})
				}
			}
			// Many attachments removed at once stay on the server until the person
			// chooses. A new hold is a new item, so its id names the paths.
			if held := report.RemovalHeld; enabled[row.FolderID] && hasReport && held != nil && held.Files > 0 {
				sum := sha256.Sum256([]byte(strings.Join(held.Paths, "\x00")))
				events = append(events, appcore.EventFact{ID: "removalHeld:" + row.FolderID + ":" + hex.EncodeToString(sum[:6]), Kind: appcore.WaitingRemovalHeld, SubjectID: row.FolderID, Name: name, Detail: strconv.Itoa(held.Files)})
			}
			continue
		}
		name := row.DisplayName
		if name == "" {
			name = row.Username
		}
		// A row the server does not sync (a viewer's team, a lapsed plan) cannot
		// be set up here, so it reads paused and offers nothing.
		ff := appcore.FolderFact{
			ID:     row.FolderID,
			Name:   name,
			Origin: origin,
			Team:   team,
			Viewer: viewer,
		}
		if !row.Sync.Enabled {
			ff.Sync.State = appcore.FolderSyncPaused
		}
		folders = append(folders, ff)
		if team && row.Sync.Enabled {
			events = append(events, appcore.EventFact{ID: "teamToSetUp:" + row.FolderID, Kind: appcore.WaitingTeamToSetUp, SubjectID: row.FolderID, Name: name})
		}
	}
	if record.KeyRefused != nil {
		events = append(events, appcore.EventFact{ID: "syncKeyRefused:" + record.KeyRefused.Since, Kind: appcore.WaitingSyncKeyRefused})
	}
	for _, session := range record.SyncSessions {
		events = append(events, a.rt.syncConflicts.facts[session.ID]...)
	}
	return facts, folders, events, synced
}

// syncedFolder is what sync knows about a folder on this computer that has a
// session row: whether it syncs now, and the account it belongs to.
type syncedFolder struct {
	Enabled bool
	Origin  appcore.Origin
	Viewer  bool
	Sync    appcore.FolderSync
}

// syncTime reads a report's ISO time, or nil.
func syncTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil
	}
	return &at
}

// syncPauseReason reads a report's paused field: null, or {"reason": "…"}.
func syncPauseReason(raw json.RawMessage) string {
	var pause struct {
		Reason string `json:"reason"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &pause) != nil {
		return ""
	}
	return pause.Reason
}

// setSyncPaused is sync.setPaused: the switch every session follows. Sessions
// read it when /sync/v1/configure arrives in slice 2.
func (a *app) setSyncPaused(paused bool) *appcore.Refusal {
	refusal := a.saveSyncPaused(paused)
	if refusal == nil {
		a.sendSyncWanted()
	}
	return refusal
}

func (a *app) saveSyncPaused(paused bool) *appcore.Refusal {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.logger.Printf("Could not pause sync: sync.json: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	record.Paused = paused
	if err := record.Save(); err != nil {
		a.rt.logger.Printf("Could not pause sync: sync.json: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// ownSyncRoots answers the rootId of every folder this app syncs, stopped ones
// included: their markers are this app's, not Hyperclay Local's. A sync.json this
// build cannot read answers none, so every marker is treated as Local's.
func (a *app) ownSyncRoots() []string {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		return nil
	}
	roots := make([]string, 0, len(record.SyncSessions))
	for _, session := range record.SyncSessions {
		roots = append(roots, session.RootID)
	}
	return roots
}

// syncWanted is C3's configure body: the whole wanted state. The service runs each
// enabled session whose root is listed and stops every other one.
type syncWanted struct {
	DeviceID string               `json:"deviceId"`
	Paused   bool                 `json:"paused"`
	Sessions []config.SyncSession `json:"sessions"`
	Roots    []syncWantedRoot     `json:"roots"`
}

type syncWantedRoot struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// sendSyncWanted sends the service the wanted state, after an attach and after each
// change. Sends are one at a time, each built when it starts, so the last one sent
// is the newest state. A folder named in skip is left out of the roots: it is
// about to stop being served. With no account nothing is sent; with a refused key
// the sessions are empty, so every one stops while the key stays stored.
func (a *app) sendSyncWanted(skip ...string) {
	if a.served == nil || a.runningService() == nil {
		return
	}
	a.rt.syncWantedMu.Lock()
	defer a.rt.syncWantedMu.Unlock()
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.syncMu.Unlock()
		a.rt.logger.Printf("Could not send the sync state: sync.json: %v", err)
		return
	}
	if record.Account == nil {
		a.rt.syncMu.Unlock()
		return
	}
	body := syncWanted{DeviceID: record.DeviceID, Paused: record.Paused, Sessions: []config.SyncSession{}, Roots: []syncWantedRoot{}}
	if record.KeyRefused == nil {
		for _, session := range record.SyncSessions {
			if !a.rt.syncBinding[session.ID] {
				body.Sessions = append(body.Sessions, session)
			}
		}
	}
	a.rt.syncMu.Unlock()
	a.servedMu.RLock()
	for _, m := range a.servedByName {
		if !slices.Contains(skip, m.folder.ID) {
			body.Roots = append(body.Roots, syncWantedRoot{ID: m.folder.ID, Path: m.folder.Path})
		}
	}
	a.servedMu.RUnlock()
	sort.Slice(body.Roots, func(i, j int) bool { return body.Roots[i].ID < body.Roots[j].ID })
	ctx, cancel := context.WithTimeout(context.Background(), syncCallTimeout)
	defer cancel()
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := a.served.CallService(ctx, "/sync/v1/configure", body, &answer); err != nil {
		if !errors.Is(err, served.ErrServiceUnavailable) {
			a.rt.logger.Printf("The live-room service did not take the sync state: %v", err)
		}
		return
	}
	if !answer.OK && answer.Error != "not-connected" {
		a.rt.logger.Printf("The live-room service refused the sync state: %s", answer.Error)
	}
}

// syncReportRecord is the part of a report's session row Go keeps. Every other
// field of the row, enabled above all, is Go's own and a report never changes it.
type syncReportRecord struct {
	ID            string                    `json:"id"`
	Paused        *config.SyncPause         `json:"paused"`
	AccountID     int64                     `json:"accountId"`
	Cached        *config.SyncSessionCached `json:"cached"`
	LegacyMetaDir *string                   `json:"legacyMetaDir"`
}

// takeSyncReport is the served host's sink for the service's reports (C4). Each
// session's record updates its row's paused, accountId, cached and legacyMetaDir.
// keyRefused marks the key refused and stops every session; the key stays stored
// until the person replaces it or disconnects.
func (a *app) takeSyncReport(report served.SyncReport) {
	stop := false
	a.rt.syncMu.Lock()
	rereadConflicts := conflictsChanged(a.rt.syncLast, report)
	keyRevoked := newlyKeyRevoked(a.rt.syncLast, report)
	a.rt.syncLast = &report
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.syncMu.Unlock()
		a.rt.logger.Printf("Could not keep a sync report: sync.json: %v", err)
		return
	}
	changed := false
	for _, session := range report.Sessions {
		if len(session.Record) == 0 || string(session.Record) == "null" {
			continue
		}
		var row syncReportRecord
		if err := json.Unmarshal(session.Record, &row); err != nil {
			a.rt.logger.Printf("A sync report's row for session %s did not read: %v", session.SessionID, err)
			continue
		}
		for i := range record.SyncSessions {
			kept := &record.SyncSessions[i]
			if kept.ID != session.SessionID || row.ID != session.SessionID {
				continue
			}
			next := *kept
			next.Paused = row.Paused
			if row.AccountID != 0 {
				next.AccountID = row.AccountID
			}
			if row.Cached != nil {
				next.Cached = *row.Cached
			}
			next.LegacyMetaDir = row.LegacyMetaDir
			if !reflect.DeepEqual(next, *kept) {
				*kept = next
				changed = true
			}
		}
	}
	var notices []shellapi.Notice
	if rows, ok := a.reportedAccountRows(record, report.Accounts); ok {
		notices = append(notices, addedTeams(record.Accounts, rows)...)
		record.Accounts = rows
		changed = true
	}
	if (report.KeyRefused || keyRevoked) && record.Account != nil && record.KeyRefused == nil {
		record.KeyRefused = &config.SyncKeyRefused{Since: time.Now().UTC().Format(time.RFC3339)}
		changed = true
		stop = true
		notices = append(notices, shellapi.Notice{ID: "keyStopped:" + record.KeyRefused.Since, Kind: noticeKeyStopped})
	}
	if changed {
		if err := record.Save(); err != nil {
			a.rt.logger.Printf("Could not keep a sync report: sync.json: %v", err)
		}
	}
	a.rt.syncMu.Unlock()
	for _, n := range notices {
		a.notice(n)
	}
	a.keepSyncHistory(report.Events)
	if rereadConflicts {
		a.readHeldConflicts()
	}
	if stop {
		go a.sendSyncWanted()
	}
	// Folder states, counts and waiting items come from the report, so every
	// open screen gets a new snapshot.
	a.changed()
}

// newlyKeyRevoked reports whether a session in report has just paused with
// key-revoked. The server ends a revoked key's streams that way, which pauses
// each session without a refused discovery: it is the same news. Only the
// change counts, so a report sent before a new key took over sets nothing again.
func newlyKeyRevoked(previous *served.SyncReport, report served.SyncReport) bool {
	before := map[string]bool{}
	if previous != nil {
		for _, session := range previous.Sessions {
			before[session.SessionID] = syncPauseReason(session.Paused) == "key-revoked"
		}
	}
	for _, session := range report.Sessions {
		if syncPauseReason(session.Paused) == "key-revoked" && !before[session.SessionID] {
			return true
		}
	}
	return false
}

// reportedAccountRows merges a report's new discovery answer into the account
// rows, as a connect does, so a role the server changed reaches the folder's row.
// A row whose account left the answer stays while a session syncs its folder: the
// session's own pause says why. ok is false when nothing changed.
func (a *app) reportedAccountRows(record *config.Sync, raw json.RawMessage) ([]config.SyncAccountRow, bool) {
	if record.Account == nil || len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var found syncDiscovery
	if err := json.Unmarshal(raw, &found.Accounts); err != nil {
		a.rt.logger.Printf("A sync report's accounts did not read: %v", err)
		return nil, false
	}
	rows := a.mergeAccountRows(record.Accounts, found)
	listed := make(map[string]bool, len(rows))
	for _, row := range rows {
		listed[row.FolderID] = true
	}
	for _, previous := range record.Accounts {
		if listed[previous.FolderID] {
			continue
		}
		for _, session := range record.SyncSessions {
			if session.RootID == previous.FolderID {
				rows = append(rows, previous)
				break
			}
		}
	}
	if reflect.DeepEqual(rows, record.Accounts) {
		return nil, false
	}
	return rows, true
}

// setFolderSynced is folder.setSynced: off keeps the folder's session row with
// enabled false, so its held conflicts and its metadata stay for a later on; on
// turns the same row back on. Neither ever binds: the folder's files are already
// there, and the session starts with an ordinary comparison.
func (a *app) setFolderSynced(folderID string, on bool) *appcore.Refusal {
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.syncMu.Unlock()
		a.rt.logger.Printf("Could not change sync for folder %s: sync.json: %v", folderID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	found := false
	for i := range record.SyncSessions {
		if record.SyncSessions[i].RootID == folderID {
			record.SyncSessions[i].Enabled = on
			found = true
		}
	}
	if !found {
		a.rt.syncMu.Unlock()
		return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: folderID}
	}
	err = record.Save()
	a.rt.syncMu.Unlock()
	if err != nil {
		a.rt.logger.Printf("Could not change sync for folder %s: sync.json: %v", folderID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.sendSyncWanted()
	return nil
}
