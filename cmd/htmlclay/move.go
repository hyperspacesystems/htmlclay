package main

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/folderlock"
	"github.com/panphora/htmlclay/internal/localimport"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/server"
)

// Reasons a folder stays with Hyperclay Local, kept in the receipt's skip field.
const (
	moveSkipMissing        = "folder-missing"
	moveSkipInUse          = "folder-in-use"
	moveSkipUnfinishedBind = "local-first-download-unfinished"
	moveSkipAlreadySynced  = "account-already-synced"
)

// moveQuitWait is how long quitLocal waits for Hyperclay Local to go.
var moveQuitWait = 10 * time.Second

// localUserData is Hyperclay Local's own data folder. HTMLCLAY_LOCAL_USERDATA
// names another one for tests and rehearsals, which never read a person's real one.
func localUserData() string {
	if dir := os.Getenv("HTMLCLAY_LOCAL_USERDATA"); dir != "" {
		return dir
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "Hyperclay Local")
}

func (a *app) moveReceiptFile() string {
	return filepath.Join(a.rt.configDir, "local-import.json")
}

// moveFacts answers the offer to move from Hyperclay Local, its progress, or nil
// when there is nothing to move.
func (a *app) moveFacts() *appcore.Move {
	userData := localUserData()
	if userData == "" {
		return nil
	}
	settings, err := localimport.ReadSettings(userData)
	if err != nil {
		return nil
	}
	receipt, err := localimport.LoadReceipt(a.moveReceiptFile(), userData)
	if err != nil {
		return nil
	}
	a.rt.moveMu.Lock()
	running, moved := a.rt.moveRunning, slices.Clone(a.rt.moveMoved)
	a.rt.moveMu.Unlock()
	move := &appcore.Move{State: appcore.MoveOffered}
	if running {
		move.State = appcore.MoveRunning
	}
	_, move.LocalRunning = localimport.RunningPID(userData, wirePIDAlive)
	for _, root := range settings.Roots {
		entry := receipt.Root(root.ID)
		if entry != nil && entry.State == localimport.StateDone {
			continue
		}
		folder := a.moveFolder(root)
		folder.Synced = settings.SessionFor(root.ID) != nil
		if entry != nil {
			folder.Skip = entry.Skip
		}
		move.NeedsKey = move.NeedsKey || folder.Synced
		move.Folders = append(move.Folders, folder)
	}
	move.NeedsKey = move.NeedsKey && !a.moveConnected()
	if running || len(move.Folders) > 0 {
		return move
	}
	if len(moved) == 0 {
		return nil
	}
	move.State = appcore.MoveDone
	for _, root := range settings.Roots {
		if slices.Contains(moved, root.ID) {
			move.Folders = append(move.Folders, a.moveFolder(root))
		}
	}
	return move
}

func (a *app) moveFolder(root localimport.Root) appcore.MoveFolder {
	folder := appcore.MoveFolder{ID: root.ID, Name: filepath.Base(root.Path), Port: root.Port}
	if resolved, name, err := a.servedPath(root.Path, ""); err == nil {
		folder.Address = a.servedAddress(config.ServedFolder{Name: name, Path: resolved})
	}
	return folder
}

// moveConnected reports whether this app holds a sync key Hyperclay accepts.
func (a *app) moveConnected() bool {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	return err == nil && record.Account != nil && record.KeyRefused == nil
}

// quitLocal asks Hyperclay Local to quit, the way its own Quit does, and waits
// for it to go. It signals only a process that listens on a port Local wrote
// down, so a number another program got after a crash is never signalled. It
// never forces: a Local that will not quit stays, and the person quits it.
func (a *app) quitLocal() *appcore.Refusal {
	userData := localUserData()
	pid, running := localimport.RunningPID(userData, wirePIDAlive)
	if !running {
		a.changed()
		return nil
	}
	isLocal := false
	for _, port := range localimport.ServedPorts(userData) {
		if owner, _, known := a.lookupPortOwner(port); known && owner == pid {
			isLocal = true
			break
		}
	}
	if !isLocal {
		a.rt.logger.Printf("Not asking PID %d to quit: it listens on none of Hyperclay Local's ports", pid)
		return &appcore.Refusal{Code: appcore.RefuseLocalRunning}
	}
	quit := platform.QuitProcess
	if a.rt.quitProcess != nil {
		quit = a.rt.quitProcess
	}
	if err := quit(pid, false); err != nil {
		a.rt.logger.Printf("Could not ask Hyperclay Local (PID %d) to quit: %v", pid, err)
		return &appcore.Refusal{Code: appcore.RefuseLocalRunning}
	}
	for deadline := time.Now().Add(moveQuitWait); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if _, running := localimport.RunningPID(userData, wirePIDAlive); !running {
			a.changed()
			return nil
		}
	}
	a.changed()
	return &appcore.Refusal{Code: appcore.RefuseLocalRunning}
}

// startMove moves the folders named in e.IDs, or every folder offered, from
// Hyperclay Local. Each folder either moves whole or stays with Local, and the
// receipt says which and why.
func (a *app) startMove(e appcore.Effect) *appcore.Refusal {
	userData := localUserData()
	settings, err := localimport.ReadSettings(userData)
	if err != nil {
		return &appcore.Refusal{Code: appcore.RefuseNothingToMove}
	}
	if pid, running := localimport.RunningPID(userData, wirePIDAlive); running {
		a.rt.logger.Printf("The move waits: Hyperclay Local is running (pid %d)", pid)
		return &appcore.Refusal{Code: appcore.RefuseLocalRunning}
	}
	receipt, err := localimport.LoadReceipt(a.moveReceiptFile(), userData)
	if err != nil {
		a.rt.logger.Printf("Could not read the move receipt: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.rt.moveMu.Lock()
	if a.rt.moveRunning {
		a.rt.moveMu.Unlock()
		return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow}
	}
	a.rt.moveRunning = true
	a.rt.moveMu.Unlock()
	a.changed()

	var moved []string
	for _, root := range settings.Roots {
		if len(e.IDs) > 0 && !slices.Contains(e.IDs, root.ID) {
			continue
		}
		if entry := receipt.Root(root.ID); entry != nil && entry.State == localimport.StateDone {
			continue
		}
		entry := a.moveRoot(userData, settings, root, receipt)
		receipt.Put(entry)
		if err := receipt.Save(a.moveReceiptFile()); err != nil {
			a.rt.logger.Printf("Could not save the move receipt: %v", err)
		}
		if entry.State == localimport.StateDone {
			moved = append(moved, root.ID)
			a.holdLocalPort(entry.FolderID, root.Port)
			a.rt.logger.Printf("Moved %s from Hyperclay Local as folder %s", root.Path, entry.FolderID)
		}
	}
	a.rt.moveMu.Lock()
	a.rt.moveRunning = false
	a.rt.moveMoved = append(a.rt.moveMoved, moved...)
	a.rt.moveMu.Unlock()
	a.sendSyncWanted()
	a.changed()
	return nil
}

// moveRoot moves one of Local's folders and answers its receipt entry.
func (a *app) moveRoot(userData string, settings *localimport.Settings, root localimport.Root, receipt *localimport.Receipt) localimport.ReceiptRoot {
	entry := localimport.ReceiptRoot{LocalRootID: root.ID, Path: root.Path, LocalPort: root.Port, State: localimport.StateSkipped}
	skip := func(code string, err error) localimport.ReceiptRoot {
		a.rt.logger.Printf("Folder %s stays with Hyperclay Local (%s): %v", root.Path, code, err)
		entry.State, entry.Skip = localimport.StateSkipped, code
		return entry
	}
	resolved, name, err := a.servedPath(root.Path, "")
	if err != nil {
		return skip(moveSkipMissing, err)
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return skip(moveSkipMissing, err)
	}
	// Whatever else would refuse serving this folder refuses the move too. Local's
	// own marker is expected here: the move is what replaces it.
	if _, refusal := a.serveFacts(resolved, name); refusal != nil && refusal.Code != appcore.RefuseBoundToLocalSync {
		return skip(refusal.Code, nil)
	}
	lock, err := folderlock.Acquire(a.rt.folderLockDir, resolved)
	if err != nil {
		return skip(moveSkipInUse, err)
	}
	release := func() {
		if lock != nil {
			lock.Release()
			lock = nil
		}
	}
	defer release()

	identity := platform.DirIdentity(resolved)
	markerID, marked := localimport.MarkerRootID(resolved)
	session := settings.SessionFor(root.ID)
	if session == nil {
		return a.moveServedOnly(root, resolved, name, identity, markerID, marked, entry, skip, release)
	}

	row, connected, committed, taken := a.moveAccount(*session)
	if !connected {
		return skip(appcore.RefuseNotConnected, nil)
	}
	if taken {
		return skip(moveSkipAlreadySynced, nil)
	}
	entry.FolderID, entry.SessionID = row.FolderID, session.ID
	committed = committed && marked && markerID == row.FolderID
	if !committed {
		if marked && markerID != root.ID {
			return skip(appcore.RefuseBoundToLocalSync, nil)
		}
		if localimport.UnfinishedBind(userData, *session) {
			return skip(moveSkipUnfinishedBind, nil)
		}
		// On disk before anything changes, so a crash leaves a record of what began.
		entry.State = localimport.StateStarted
		receipt.Put(entry)
		if err := receipt.Save(a.moveReceiptFile()); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		if err := localimport.CopyBookkeeping(userData, filepath.Join(a.rt.configDir, "service"), *session, row.FolderID); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		if row.Kind == "team" || root.WasTeam() {
			if err := a.rt.cfg.MarkTeamFolder(config.TeamFolder{FolderID: row.FolderID, Path: resolved, Identity: identity, Kind: config.TeamFolderKindTeam, Key: strconv.FormatInt(row.AccountID, 10)}); err != nil {
				return skip(appcore.RefuseFailed, err)
			}
		}
		if err := a.rt.cfg.TrustFolderWithID(resolved, identity, row.FolderID); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		a.saveConfig("trust moved folder " + name)
		// The row is held back from the service, like a first download, until the
		// marker names this app.
		if err := a.moveAddSession(config.SyncSession{
			ID: session.ID, RootID: row.FolderID, AccountID: row.AccountID, Kind: row.Kind,
			Cached:        config.SyncSessionCached{Username: row.Username, DisplayName: row.DisplayName, Role: row.Role},
			LegacyMetaDir: session.LegacyMetaDir, Enabled: true,
		}); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		// The hand-over. Before this write the folder is Local's. After it Local refuses it.
		if _, running := localimport.RunningPID(userData, wirePIDAlive); running {
			a.moveDropSession(session.ID)
			return skip(appcore.RefuseLocalRunning, nil)
		}
		if marked {
			if err := localimport.RewriteMarker(resolved, row.FolderID); err != nil {
				a.moveDropSession(session.ID)
				return skip(appcore.RefuseFailed, err)
			}
		}
	}
	release()
	served, refusal, err := a.serveFolder(resolved, name)
	a.rt.syncMu.Lock()
	delete(a.rt.syncBinding, session.ID)
	a.rt.syncMu.Unlock()
	if refusal != nil || err != nil || served.ID != row.FolderID {
		// The folder is this app's now. It stays in the receipt as started, so the
		// next run serves it without copying anything again.
		entry.State = localimport.StateStarted
		code := appcore.RefuseFailed
		if refusal != nil {
			code = refusal.Code
		}
		entry.Skip = code
		a.rt.logger.Printf("Folder %s moved but is not served (%s): %v", root.Path, code, err)
		return entry
	}
	entry.State, entry.Skip = localimport.StateDone, ""
	return entry
}

// moveServedOnly moves a folder Local served and did not sync.
func (a *app) moveServedOnly(root localimport.Root, resolved, name, identity, markerID string, marked bool, entry localimport.ReceiptRoot, skip func(string, error) localimport.ReceiptRoot, release func()) localimport.ReceiptRoot {
	if marked {
		// A marker Local left behind when it stopped syncing. Another id is not Local's to give away.
		if markerID != root.ID {
			return skip(appcore.RefuseBoundToLocalSync, nil)
		}
		if err := localimport.SetMarkerAside(resolved); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
	}
	if root.WasTeam() {
		// A folder that once held a team's pages keeps that mark here, before any page is served.
		id, err := a.rt.cfg.NewFolderID()
		if err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		if err := a.rt.cfg.MarkTeamFolder(config.TeamFolder{FolderID: id, Path: resolved, Identity: identity, Kind: config.TeamFolderKindTeam, Key: "moved:" + root.ID}); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		if err := a.rt.cfg.TrustFolderWithID(resolved, identity, id); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		a.saveConfig("trust moved folder " + name)
	} else if err := a.trustFolder(resolved); err != nil {
		return skip(appcore.RefuseFailed, err)
	}
	release()
	served, refusal, err := a.serveFolder(resolved, name)
	if refusal != nil {
		return skip(refusal.Code, nil)
	}
	if err != nil {
		return skip(appcore.RefuseFailed, err)
	}
	entry.FolderID, entry.State = served.ID, localimport.StateDone
	return entry
}

// moveAccount answers the account row a Local session belongs to, whether this
// app is connected and lists that account, whether this session is already in
// sync.json, and whether another session already syncs that account's folder.
func (a *app) moveAccount(session localimport.Session) (row config.SyncAccountRow, connected, committed, taken bool) {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil || record.Account == nil || record.KeyRefused != nil {
		return row, false, false, false
	}
	for _, candidate := range record.Accounts {
		if candidate.AccountID == session.AccountID {
			row, connected = candidate, true
		}
	}
	for _, existing := range record.SyncSessions {
		if existing.ID == session.ID {
			committed = true
		} else if connected && existing.RootID == row.FolderID {
			taken = true
		}
	}
	return row, connected, committed, taken
}

// moveAddSession records a moved session and holds it back from the service. A
// row an earlier, interrupted move left for the same session is replaced.
func (a *app) moveAddSession(session config.SyncSession) error {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		return err
	}
	record.SyncSessions = slices.DeleteFunc(record.SyncSessions, func(s config.SyncSession) bool { return s.ID == session.ID })
	record.SyncSessions = append(record.SyncSessions, session)
	if err := record.Save(); err != nil {
		return err
	}
	if a.rt.syncBinding == nil {
		a.rt.syncBinding = make(map[string]bool)
	}
	a.rt.syncBinding[session.ID] = true
	return nil
}

// moveDropSession takes back a session whose folder stayed with Local.
func (a *app) moveDropSession(id string) {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	delete(a.rt.syncBinding, id)
	record, err := a.syncRecordLocked()
	if err != nil {
		return
	}
	record.SyncSessions = slices.DeleteFunc(record.SyncSessions, func(s config.SyncSession) bool { return s.ID == id })
	if err := record.Save(); err != nil {
		a.rt.logger.Printf("Could not take back session %s: sync.json: %v", id, err)
	}
}

// holdLocalPort answers at the port Hyperclay Local served a moved folder on. A
// page navigation there is sent on to the same path at the folder's address now,
// so a bookmark made in Local still opens its page. Like every parked port it
// serves no file and never reads the disk. A port that is taken is skipped.
func (a *app) holdLocalPort(folderID string, port int) bool {
	if port <= 0 || port == a.rt.port {
		return false
	}
	a.mu.Lock()
	for _, p := range a.parked {
		if p.port == port {
			a.mu.Unlock()
			return p.kind == recoveryMoved
		}
	}
	a.mu.Unlock()
	return a.parkPortTo("moved:"+folderID, port, recoveryPage, recoveryMoved, func(r *http.Request) (string, bool) {
		return a.localPortTarget(folderID, r)
	})
}

// localPortTarget answers where a page navigation to one of Local's old
// addresses goes: the same path at the address the moved folder is served on.
func (a *app) localPortTarget(folderID string, r *http.Request) (string, bool) {
	if !server.EligibleNavigation(r, a.ownsOrigin) {
		return "", false
	}
	rel := strings.TrimPrefix(r.URL.Path, "/")
	if strings.Contains(rel, "\\") || strings.HasPrefix(rel, "_/") {
		return "", false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || part == "." {
			return "", false
		}
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case "", ".html", ".htm", ".htmlclay":
	default:
		return "", false
	}
	folder, ok := a.rt.cfg.ServedFolderByID(folderID)
	if !ok {
		return "", false
	}
	target := a.servedAddress(folder) + r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	return target, true
}

// holdLocalPorts holds, at launch, the old port of every folder the receipt says moved.
func (a *app) holdLocalPorts() {
	receipt, err := localimport.LoadReceipt(a.moveReceiptFile(), "")
	if err != nil {
		return
	}
	for _, entry := range receipt.Roots {
		if entry.State == localimport.StateDone {
			a.holdLocalPort(entry.FolderID, entry.LocalPort)
		}
	}
}
