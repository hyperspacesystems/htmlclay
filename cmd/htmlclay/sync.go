package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/secret"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/service"
)

// syncKeyName is the sync key's item name in the secret store.
const syncKeyName = "sync-key"

// defaultSyncServer is where sync goes. HTMLCLAY_SYNC_SERVER points a test at a
// stand-in; the service accepts plain http only on loopback.
const defaultSyncServer = "https://hyperclay.com"

func syncServerURL() string {
	if v := os.Getenv("HTMLCLAY_SYNC_SERVER"); v != "" {
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
	}
	return a.rt.syncRecord, nil
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
func (a *app) connectSync(ctx context.Context, e appcore.Effect) *appcore.Refusal {
	if !syncKeyShape.MatchString(e.Key) {
		return &appcore.Refusal{Code: appcore.RefuseSyncKeyShape, Field: "key"}
	}
	if a.rt.syncStore == nil {
		a.rt.logger.Printf("Could not connect sync: there is no secret store")
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	proc := a.runningService()
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
func (a *app) syncFacts() (appcore.SyncFacts, []appcore.FolderFact, []appcore.EventFact) {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil || record.Account == nil {
		return appcore.SyncFacts{}, nil, nil
	}
	facts := appcore.SyncFacts{
		Connected: true,
		Username:  record.Account.Username,
		FullName:  record.Account.FullName,
		KeySuffix: record.Account.KeySuffix,
		Paused:    record.Paused,
	}
	bound := make(map[int64]bool, len(record.SyncSessions))
	for _, session := range record.SyncSessions {
		if session.Enabled {
			bound[session.AccountID] = true
		}
	}
	var folders []appcore.FolderFact
	var events []appcore.EventFact
	for _, row := range record.Accounts {
		team := row.Kind == "team"
		viewer := row.Role == "viewer"
		if team && viewer {
			facts.ViewOnlyTeams++
		}
		if bound[row.AccountID] {
			continue
		}
		origin := appcore.Origin{Kind: appcore.OriginAccount, Username: row.Username}
		if team {
			origin = appcore.Origin{Kind: appcore.OriginTeam, Username: record.Account.Username, Team: row.Username}
		}
		name := row.DisplayName
		if name == "" {
			name = row.Username
		}
		folders = append(folders, appcore.FolderFact{
			ID:     row.FolderID,
			Name:   name,
			Origin: origin,
			Team:   team,
			Viewer: viewer,
		})
		if team {
			events = append(events, appcore.EventFact{ID: "teamToSetUp:" + row.FolderID, Kind: appcore.WaitingTeamToSetUp, SubjectID: row.FolderID, Name: name})
		}
	}
	if record.KeyRefused != nil {
		events = append(events, appcore.EventFact{ID: "syncKeyRefused:" + record.KeyRefused.Since, Kind: appcore.WaitingSyncKeyRefused})
	}
	return facts, folders, events
}

// setSyncPaused is sync.setPaused: the switch every session follows. Sessions
// read it when /sync/v1/configure arrives in slice 2.
func (a *app) setSyncPaused(paused bool) *appcore.Refusal {
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
