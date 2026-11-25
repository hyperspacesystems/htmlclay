package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
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
	moveSkipTrustedHere    = "folder-trusted-here"
	moveSkipAccountGone    = "account-gone"
)

// localProcessAlive reports whether pid is a running Hyperclay Local. The numbers
// Local writes down outlive a crash or a power loss, and the system may give one
// to another program after a restart, so a live number alone is not Local.
// Windows has no ps: there the number alone answers, as before.
var localProcessAlive = func(pid int) bool {
	if !wirePIDAlive(pid) {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return isLocalCommand(string(out))
}

// isLocalCommand reports whether a process command names Hyperclay Local: its
// Mac and Windows executable (HyperclayLocal), its helpers, or the Linux
// package's (hyperclay-local-electron, cut to 15 characters by ps).
func isLocalCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	return strings.Contains(command, "hyperclaylocal") || strings.Contains(command, "hyperclay-local") || strings.Contains(command, "hyperclay local")
}

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
	_, move.LocalRunning = localimport.RunningPID(userData, localProcessAlive)
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
	pid, running := localimport.RunningPID(userData, localProcessAlive)
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
		if _, running := localimport.RunningPID(userData, localProcessAlive); !running {
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
	if pid, running := localimport.RunningPID(userData, localProcessAlive); running {
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
	tried := 0
	for _, root := range settings.Roots {
		if len(e.IDs) > 0 && !slices.Contains(e.IDs, root.ID) {
			continue
		}
		var prev *localimport.ReceiptRoot
		if found := receipt.Root(root.ID); found != nil {
			if found.State == localimport.StateDone {
				continue
			}
			copied := *found
			prev = &copied
		}
		tried++
		entry := a.moveRoot(userData, settings, root, receipt)
		if prev != nil && prev.State == localimport.StateStarted && entry.State == localimport.StateSkipped && prev.FolderID != "" {
			// A move that began stays begun: an early stop on a later try keeps what it recorded.
			kept := *prev
			kept.Skip = entry.Skip
			entry = kept
		}
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
	if len(moved) > 0 {
		a.moveGrants(settings, moved)
	}
	a.rt.moveMu.Lock()
	a.rt.moveRunning = false
	a.rt.moveMoved = append(a.rt.moveMoved, moved...)
	a.rt.moveMu.Unlock()
	a.sendSyncWanted()
	a.changed()
	if tried > 0 && len(moved) == 0 {
		return &appcore.Refusal{Code: appcore.RefuseNothingMoved}
	}
	return nil
}

// moveGrants carries the programs the person registered in Local, and the
// documents they allowed inside the folders that moved. IDs are kept, so each
// allow still names its program. A program already here, a name a built-in
// program owns, and a document already decided here are left as they are.
func (a *app) moveGrants(settings *localimport.Settings, moved []string) {
	var folders []string
	for _, root := range settings.Roots {
		if !slices.Contains(moved, root.ID) {
			continue
		}
		folder := filepath.Clean(root.Path)
		if resolved, err := filepath.EvalSymlinks(folder); err == nil {
			folder = resolved
		}
		folders = append(folders, folder)
	}
	programs := 0
	for _, p := range settings.Programs {
		if builtin.Is(p.Name) {
			continue
		}
		if a.rt.cfg.RestoreHelperProgram(config.HelperProgram{ID: p.ID, Name: p.Name, Path: p.Path, AnyDocument: p.AnyDocument, AddedAt: p.AddedAt * int64(time.Millisecond)}) {
			programs++
		}
	}
	var rows []config.HelperDecision
	for _, d := range settings.AllowsIn(folders) {
		if builtin.Is(d.Name) {
			continue
		}
		rows = append(rows, config.HelperDecision{Document: d.Document, Name: d.Name, Program: d.Program, Allowed: true, DecidedAt: d.DecidedAt * int64(time.Millisecond), Scope: a.scopeOf(d.Document)})
	}
	allows := a.rt.cfg.RestoreHelperDecisions(rows)
	if programs == 0 && allows == 0 {
		return
	}
	a.saveConfig("carry programs from Hyperclay Local")
	a.rt.logger.Printf("Carried %d programs and %d document permissions from Hyperclay Local", programs, allows)
}

// moveRoot moves one of Local's folders and answers its receipt entry.
func (a *app) moveRoot(userData string, settings *localimport.Settings, root localimport.Root, receipt *localimport.Receipt) localimport.ReceiptRoot {
	entry := localimport.ReceiptRoot{LocalRootID: root.ID, Path: root.Path, LocalPort: root.Port, State: localimport.StateSkipped}
	skip := func(code string, err error) localimport.ReceiptRoot {
		a.rt.logger.Printf("Folder %s stays with Hyperclay Local (%s): %v", root.Path, code, err)
		entry.State, entry.Skip = localimport.StateSkipped, code
		return entry
	}
	session := settings.SessionFor(root.ID)
	resolved, name, err := a.servedPath(root.Path, "")
	if err != nil {
		return skip(moveSkipMissing, err)
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return skip(moveSkipMissing, err)
	}
	// Whatever would refuse serving this folder refuses the move, before anything is
	// written. Local's own marker counts as this folder's: the move is what replaces it.
	if refusal := a.moveServeRefusal(resolved, name, root.ID); refusal != nil {
		return skip(refusal.Code, nil)
	}
	// A folder this app already trusts under another id would be refused at the
	// trust step, after Local's marker was handed over. It stays, with a reason.
	if id, trusted := a.trustedFolderID(resolved); trusted && session != nil {
		if row, ok := a.moveRow(*session); !ok || row.FolderID != id {
			return skip(moveSkipTrustedHere, nil)
		}
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
	if session == nil {
		return a.moveServedOnly(root, resolved, name, identity, markerID, marked, entry, skip, release)
	}

	row, connected, listed, committed, taken := a.moveAccount(*session)
	if !connected {
		return skip(appcore.RefuseNotConnected, nil)
	}
	if !listed {
		return skip(moveSkipAccountGone, nil)
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
		if id, trusted := a.trustedFolderID(resolved); !trusted || id != row.FolderID {
			if err := a.rt.cfg.TrustFolderWithID(resolved, identity, row.FolderID); err != nil {
				return skip(appcore.RefuseFailed, err)
			}
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
		if _, running := localimport.RunningPID(userData, localProcessAlive); running {
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
		id, trusted := a.trustedFolderID(resolved)
		if !trusted {
			fresh, err := a.rt.cfg.NewFolderID()
			if err != nil {
				return skip(appcore.RefuseFailed, err)
			}
			id = fresh
		}
		if err := a.rt.cfg.MarkTeamFolder(config.TeamFolder{FolderID: id, Path: resolved, Identity: identity, Kind: config.TeamFolderKindTeam, Key: "moved:" + root.ID}); err != nil {
			return skip(appcore.RefuseFailed, err)
		}
		if !trusted {
			if err := a.rt.cfg.TrustFolderWithID(resolved, identity, id); err != nil {
				return skip(appcore.RefuseFailed, err)
			}
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
func (a *app) moveAccount(session localimport.Session) (row config.SyncAccountRow, connected, listed, committed, taken bool) {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil || record.Account == nil || record.KeyRefused != nil {
		return row, false, false, false, false
	}
	connected = true
	row, listed = moveRowIn(record, session)
	for _, existing := range record.SyncSessions {
		if existing.ID == session.ID {
			committed = true
		} else if listed && existing.RootID == row.FolderID {
			taken = true
		}
	}
	return row, connected, listed, committed, taken
}

// moveRowIn answers the account row a Local session syncs. A personal session
// from before Local's version 2 has no account id: it is the connected person's
// own account, as Local's own lookup answers it.
func moveRowIn(record *config.Sync, session localimport.Session) (config.SyncAccountRow, bool) {
	for _, candidate := range record.Accounts {
		if session.AccountID != 0 && candidate.AccountID == session.AccountID {
			return candidate, true
		}
		if session.AccountID == 0 && session.Kind == "personal" && candidate.Kind == "personal" {
			return candidate, true
		}
	}
	return config.SyncAccountRow{}, false
}

// moveRow answers the account row a session would move into, if this app lists it.
func (a *app) moveRow(session localimport.Session) (config.SyncAccountRow, bool) {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil || record.Account == nil {
		return config.SyncAccountRow{}, false
	}
	return moveRowIn(record, session)
}

// moveServeRefusal answers what would refuse serving the folder, with Local's
// own marker (its root id) counted as this folder's.
func (a *app) moveServeRefusal(resolved, name, localRootID string) *appcore.Refusal {
	facts, _ := a.serveFacts(resolved, name)
	facts.OwnSyncRoots = append(facts.OwnSyncRoots, localRootID)
	// The move runs only while Local is closed, and Local served these folders, so
	// the list it wrote (stale after a crash) says nothing about them now.
	facts.LocalServed = nil
	return appcore.CheckServe(facts)
}

// trustedFolderID answers the id this app trusts the folder under, if it does.
func (a *app) trustedFolderID(resolved string) (string, bool) {
	for _, f := range a.rt.cfg.TrustedFolderList() {
		if f.Path == resolved {
			return f.ID, true
		}
	}
	return "", false
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
