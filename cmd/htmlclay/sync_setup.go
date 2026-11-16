package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// syncBindTimeout bounds one first download. A bind the service is still running
// when this passes is left to resume on the next configure.
const syncBindTimeout = 6 * time.Hour

// syncBindAnswer is C3's /sync/v1/bind answer.
type syncBindAnswer struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
	Resumable bool   `json:"resumable"`
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
}

type syncBindRequest struct {
	DeviceID string             `json:"deviceId"`
	Session  config.SyncSession `json:"session"`
	Root     syncWantedRoot     `json:"root"`
	Mode     string             `json:"mode"`
	// madeTrust is set when this set-up trusted the folder, so a first download
	// that cannot resume gives the trust back.
	madeTrust bool
}

// setUpSyncFolder is folder.setUpTeam (08-sync.md, 8.20): an account's folder that
// is not on this computer yet becomes a served folder under the id the account row
// was given, and its first download starts. The order matters: a team folder is
// marked before it is served or anything lands in it (decision 10a), and the
// session row is held back from configure until the bind owns it, so the session
// never starts as an ordinary sync. The download runs after this returns.
func (a *app) setUpSyncFolder(e appcore.Effect) *appcore.Refusal {
	if a.runningService() == nil {
		return &appcore.Refusal{Code: appcore.RefuseServiceNeeded}
	}
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.syncMu.Unlock()
		a.rt.logger.Printf("Could not set up a synced folder: sync.json: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	connected := record.Account != nil && record.KeyRefused == nil
	deviceID := record.DeviceID
	var row config.SyncAccountRow
	found := false
	for _, candidate := range record.Accounts {
		if candidate.FolderID == e.FolderID {
			row, found = candidate, true
		}
	}
	a.rt.syncMu.Unlock()
	if !connected {
		return &appcore.Refusal{Code: appcore.RefuseNotConnected}
	}
	if !found {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
	}
	team := row.Kind == "team"
	if !filepath.IsAbs(e.Path) {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "path"}
	}

	path := filepath.Clean(e.Path)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			a.rt.logger.Printf("Could not make folder %s: %v", path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
		}
	} else if err != nil {
		a.rt.logger.Printf("Could not read folder %s: %v", path, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	resolved, name, err := a.servedPath(path, "")
	if err != nil {
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	if team || !e.Existing {
		empty, err := folderIsEmpty(resolved)
		if err != nil {
			a.rt.logger.Printf("Could not read folder %s: %v", resolved, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
		}
		if !empty {
			return &appcore.Refusal{Code: appcore.RefuseSyncFolderNotEmpty, SubjectID: e.FolderID, OtherPath: resolved}
		}
	}
	if _, refusal := a.serveFacts(resolved, name); refusal != nil {
		return refusal
	}
	// Syncing into files already there waits until the person has seen what it
	// will do (8.21): until then nothing is trusted, served or bound.
	if e.Existing && !e.Confirmed {
		counts, refusal := a.previewExisting(row.AccountID, resolved)
		if refusal != nil {
			return refusal
		}
		return &appcore.Refusal{Code: appcore.RefuseConfirmNeeded, SubjectID: e.FolderID, Counts: counts}
	}

	identity := platform.DirIdentity(resolved)
	madeTrust := !a.trustedAt(resolved)
	if team {
		if err := a.rt.cfg.MarkTeamFolder(config.TeamFolder{
			FolderID: row.FolderID,
			Path:     resolved,
			Identity: identity,
			Kind:     config.TeamFolderKindTeam,
			Key:      strconv.FormatInt(row.AccountID, 10),
		}); err != nil {
			a.rt.logger.Printf("Could not mark %s as a team folder: %v", resolved, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
		}
	}
	if err := a.rt.cfg.TrustFolderWithID(resolved, identity, row.FolderID); err != nil {
		a.rt.logger.Printf("Could not trust %s as folder %s: %v", resolved, row.FolderID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	a.saveConfig("trust synced folder " + name)
	entry, refusal, err := a.serveFolder(resolved, name)
	if refusal != nil {
		a.undoSetUp(row.FolderID, resolved, madeTrust)
		return refusal
	}
	if err != nil || entry.ID != row.FolderID {
		a.rt.logger.Printf("Could not serve synced folder %s as %s: id %s, %v", resolved, row.FolderID, entry.ID, err)
		a.undoSetUp(row.FolderID, resolved, madeTrust)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}

	session := config.SyncSession{
		ID:        newSyncSessionID(),
		RootID:    row.FolderID,
		AccountID: row.AccountID,
		Kind:      row.Kind,
		Cached:    config.SyncSessionCached{Username: row.Username, DisplayName: row.DisplayName, Role: row.Role},
		Enabled:   true,
	}
	a.rt.syncMu.Lock()
	record, err = a.syncRecordLocked()
	if err == nil {
		record.SyncSessions = append(record.SyncSessions, session)
		err = record.Save()
	}
	if err == nil {
		if a.rt.syncBinding == nil {
			a.rt.syncBinding = make(map[string]bool)
		}
		a.rt.syncBinding[session.ID] = true
	}
	a.rt.syncMu.Unlock()
	if err != nil {
		a.rt.logger.Printf("Could not set up a synced folder: sync.json: %v", err)
		a.undoSetUp(row.FolderID, resolved, madeTrust)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	mode := "empty"
	if e.Existing {
		mode = "existing"
	}
	request := syncBindRequest{DeviceID: deviceID, Session: session, Root: syncWantedRoot{ID: entry.ID, Path: entry.Path}, Mode: mode, madeTrust: madeTrust}
	a.rt.syncBinds.Add(1)
	go a.runSyncBind(request)
	return nil
}

// undoSetUp gives back what a set-up that did not finish took: it stops serving
// the folder and drops its served entry, and untrusts it when the set-up trusted
// it. The account row then reads as not on this computer again, so the person can
// set it up once more. The folder and its files stay; a team mark stays too, since
// nothing removes one (config.MarkTeamFolder).
func (a *app) undoSetUp(folderID, path string, madeTrust bool) {
	if _, ok := a.rt.cfg.ServedFolderByID(folderID); ok {
		if err := a.stopServing(folderID); err != nil {
			a.rt.logger.Printf("Could not stop serving folder %s after its set-up stopped: %v", folderID, err)
			return
		}
		a.rt.cfg.RemoveServedFolder(folderID)
		a.saveConfig("forget served folder " + folderID)
	}
	if madeTrust {
		if err := a.untrustFolder(path); err != nil {
			a.rt.logger.Printf("Could not untrust %s after its set-up stopped: %v", path, err)
		}
	}
	a.changed()
}

// previewExisting asks the service what syncing account into the folder at path
// would do. It reads the folder and the server and writes nothing.
func (a *app) previewExisting(accountID int64, path string) (*appcore.SyncCounts, *appcore.Refusal) {
	ctx, cancel := context.WithTimeout(context.Background(), syncCallTimeout)
	defer cancel()
	body := struct {
		AccountID int64          `json:"accountId"`
		Root      syncWantedRoot `json:"root"`
	}{AccountID: accountID, Root: syncWantedRoot{Path: path}}
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		appcore.SyncCounts
	}
	if err := a.served.CallService(ctx, "/sync/v1/preview", body, &answer); err != nil {
		a.rt.logger.Printf("Could not count what syncing %s would do: %v", path, err)
		return nil, &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if !answer.OK {
		if answer.Error == "offline" {
			return nil, &appcore.Refusal{Code: appcore.RefuseSyncOffline}
		}
		a.rt.logger.Printf("Could not count what syncing %s would do: %s", path, answer.Error)
		return nil, &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	counts := answer.SyncCounts
	return &counts, nil
}

// runSyncBind asks the service for the first download and acts on its answer. A
// bind that cannot resume drops the session row and stops serving the folder this
// setup served; it never deletes the folder or a file in it. A bind that can
// resume keeps its row, and the next configure picks it up.
func (a *app) runSyncBind(request syncBindRequest) {
	defer a.rt.syncBinds.Done()
	ctx, cancel := context.WithTimeout(context.Background(), syncBindTimeout)
	defer cancel()
	var answer syncBindAnswer
	err := a.served.CallService(ctx, "/sync/v1/bind", request, &answer)
	if err != nil {
		a.rt.logger.Printf("The first download for folder %s stopped before an answer, so it resumes later: %v", request.Root.ID, err)
		answer = syncBindAnswer{Error: "cancelled", Resumable: true}
	}
	dropped := false
	a.rt.syncMu.Lock()
	delete(a.rt.syncBinding, request.Session.ID)
	if !answer.OK && !answer.Resumable {
		if record, err := a.syncRecordLocked(); err == nil {
			kept := record.SyncSessions[:0]
			for _, row := range record.SyncSessions {
				if row.ID != request.Session.ID {
					kept = append(kept, row)
				}
			}
			record.SyncSessions = kept
			if err := record.Save(); err != nil {
				a.rt.logger.Printf("Could not drop the session of a failed first download: sync.json: %v", err)
			}
		}
		dropped = true
	}
	a.rt.syncMu.Unlock()
	if dropped {
		a.rt.logger.Printf("The first download for folder %s failed: %s", request.Root.ID, answer.Error)
		a.undoSetUp(request.Root.ID, request.Root.Path, request.madeTrust)
	} else if answer.OK {
		a.rt.logger.Printf("The first download for folder %s finished: %d files", request.Root.ID, answer.Files)
	}
	a.sendSyncWanted()
}

// folderIsEmpty reports whether dir holds nothing but the files a system puts in
// any folder it shows and sync's own .hyperclay folder: the engine's rule
// (first-bind.js IGNORED_ENTRIES), so the two never disagree.
func folderIsEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()
	for {
		names, err := f.Readdirnames(64)
		for _, name := range names {
			if name != ".DS_Store" && name != "desktop.ini" && name != "Thumbs.db" && name != ".hyperclay" {
				return false, nil
			}
		}
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func newSyncSessionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}

// trustedAt reports whether dir is itself a trusted folder.
func (a *app) trustedAt(dir string) bool {
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path == dir {
			return true
		}
	}
	return false
}
