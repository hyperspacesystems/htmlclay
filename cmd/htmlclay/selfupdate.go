package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/mirror"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/tray"
	"github.com/panphora/htmlclay/internal/update"
)

// updateChannel is set by the release workflow's Windows and Linux builds with
// -X main.updateChannel=stable. A local build leaves it empty, so it can never
// replace itself with a published release. macOS updates through Sparkle.
var updateChannel = ""

// releaseInfoURL is the receipt the download-page link reads. Tests point it
// at a local server.
var releaseInfoURL = update.DefaultVersionURL

// The restart waits for a moment with no write request in flight, so a save
// already on the wire lands in this process rather than failing against a
// closed port. Then it stops admitting writes and waits for the ones already
// admitted to finish. A request still running after updateQuietLimit cancels
// the restart instead of being cut off, and admission reopens; the person can
// click again.
var (
	updateQuietFor   = 500 * time.Millisecond
	updateQuietLimit = 5 * time.Second
)

// updateSyncLimit is how long an update restart waits for sync to finish what
// it holds before it asks the person whether to restart anyway.
var updateSyncLimit = 30 * time.Second

var (
	errUpdateCancelled = errors.New("the restart was cancelled")
	errUpdateBusy      = errors.New("requests were still in flight")
)

// selfUpdateGate returns nil only when this build may download and apply its
// own updates. Every other answer leaves the tray on the download-page link.
func selfUpdateGate(channel, ver, goos string, trustedKeys int, installPath string, writable func(string) error) error {
	switch {
	case channel == "":
		return errors.New("this build has no update channel")
	case !update.IsStrictVersion(ver):
		return fmt.Errorf("version %q is not a release version", ver)
	case goos != "windows" && goos != "linux":
		return fmt.Errorf("the self-updater runs on Windows and Linux, not %s", goos)
	case trustedKeys == 0:
		return errors.New("no feed signing key is built in")
	}
	return writable(installPath)
}

// startSelfUpdater starts the background check when the gate allows it and
// reports whether it did. When it returns false the caller keeps the plain
// release-info check that only links to the website.
func (a *app) startSelfUpdater() bool {
	installPath, err := a.selfUpdateInstallPath()
	if err != nil {
		a.rt.logger.Printf("Self-update is off: %v", err)
		a.updateOff = err.Error()
		return false
	}
	feed := update.DefaultFeedURL
	if v := os.Getenv("HTMLCLAY_UPDATE_FEED"); v != "" {
		feed = v
	}
	u := &update.Updater{
		FeedURL:     feed,
		Keys:        update.TrustedKeys,
		Client:      &http.Client{Timeout: 10 * time.Minute},
		Current:     version,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		Dir:         update.UpdatesDir(a.rt.configDir),
		InstallPath: installPath,
		Logf:        a.rt.logger.Printf,
		Automatic:   func() bool { return !a.rt.cfg.SelfUpdateOff() },
	}
	a.rt.logger.Printf("Self-update is on, checking %s", feed)
	a.updates.mu.Lock()
	a.updates.checkNow = make(chan struct{}, 1)
	checkNow := a.updates.checkNow
	a.updates.mu.Unlock()
	u.CheckNow = checkNow
	u.Checking = a.setUpdateChecking
	u.Found = a.setUpdateDownloading
	u.Checked = a.updateChecked
	go u.Run(nil, func(r update.Ready) {
		a.setUpdateReady(r.Version, "", func() { a.clickApplyUpdate(r, installPath) })
	})
	return true
}

// selfUpdateInstallPath returns the binary this run may replace with an update,
// or why it may not.
func (a *app) selfUpdateInstallPath() (string, error) {
	// The restart is offered from the tray, so a run without one has no way to
	// apply what it would download.
	if a.noTray {
		return "", errors.New("HTML Clay is running without its tray")
	}
	if updateNowOS == "darwin" {
		// On macOS the HTML Clay app runs Sparkle; a host started on its own keeps the download-page link.
		return "", errors.New("on macOS the HTML Clay app updates itself")
	}
	installPath, err := update.InstallPath()
	if err == nil {
		err = selfUpdateGate(updateChannel, version, updateNowOS, len(update.TrustedKeys), installPath, update.CheckWritable)
	}
	return installPath, err
}

// openAfterUpdateFile carries the files a launch was asked to open across an
// update installed at that launch: the supervisor starts the new version with
// no arguments, and the new version opens them.
const (
	openAfterUpdateFile = "open-after-update.json"
	openAfterUpdateFor  = 2 * time.Minute
)

type openAfterUpdate struct {
	At    time.Time `json:"at"`
	Files []string  `json:"files"`
}

// installWaitingUpdate installs the update an earlier run downloaded before
// this run starts anything, and reports whether it did. The caller then exits,
// and the supervisor starts the new version and judges it as it does after
// Restart to update. A run another update is still judging installs nothing.
func (a *app) installWaitingUpdate(files []string) bool {
	installPath, err := a.selfUpdateInstallPath()
	return err == nil && a.installWaiting(installPath, files)
}

func (a *app) installWaiting(installPath string, files []string) bool {
	dir := update.UpdatesDir(a.rt.configDir)
	if update.Unjudged(dir) {
		return false
	}
	r := update.Waiting(dir, version, installPath)
	if r == nil {
		return false
	}
	carry := filepath.Join(a.rt.configDir, openAfterUpdateFile)
	if len(files) > 0 {
		data, _ := json.Marshal(openAfterUpdate{At: time.Now().UTC(), Files: files})
		if err := os.WriteFile(carry, data, 0o600); err != nil {
			a.rt.logger.Printf("Not installing update %s at start: could not keep the files to open: %v", r.Version, err)
			return false
		}
	}
	if _, err := update.Apply(dir, update.ApplyParams{From: version, To: r.Version, InstallPath: installPath,
		StagedPath: r.StagedPath, OldPID: os.Getpid()}); err != nil {
		os.Remove(carry)
		a.rt.logger.Printf("Could not install update %s at start: %v", r.Version, err)
		return false
	}
	a.rt.logger.Printf("Installing update %s, downloaded before the last quit", r.Version)
	return true
}

// takeOpenAfterUpdate returns the files the launch that installed this version
// was asked to open, once, and only while they are recent.
func (a *app) takeOpenAfterUpdate() []string {
	path := filepath.Join(a.rt.configDir, openAfterUpdateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	os.Remove(path)
	var carried openAfterUpdate
	if json.Unmarshal(data, &carried) != nil || time.Since(carried.At) > openAfterUpdateFor {
		return nil
	}
	return carried.Files
}

// clickApplyUpdate is the tray's Restart to Update row. A failure leaves this
// version running and the row in place, so the person can try again.
func (a *app) clickApplyUpdate(r update.Ready, installPath string) {
	err := a.applyUpdate(r, installPath)
	if err == nil || errors.Is(err, errUpdateCancelled) {
		return
	}
	a.reportRestartFailure(r.Version, err)
}

// reportRestartFailure logs a restart that did not happen and tells the person
// this version is still running.
func (a *app) reportRestartFailure(target string, err error) {
	a.rt.logger.Printf("Update to v%s not applied, still running v%s: %v", target, version, err)
	msg := fmt.Sprintf("HTML Clay could not restart into %s just now, so it is still running %s. Try Restart to update again in a moment.", target, version)
	if notifyErr := a.informUser("HTML Clay", msg); notifyErr != nil {
		a.rt.logger.Printf("Could not show the update notice: %v", notifyErr)
	}
}

// checkForUpdateLink is the check every build falls back to when it cannot
// update itself: it only offers the download page.
func (a *app) checkForUpdateLink() {
	a.updates.mu.Lock()
	a.updates.checkNow = make(chan struct{}, 1)
	ch := a.updates.checkNow
	a.updates.mu.Unlock()
	check := !a.rt.cfg.SelfUpdateOff()
	for {
		if check {
			a.setUpdateChecking(true)
			info, err := update.Check(version, releaseInfoURL)
			result := update.CheckResult{Err: err}
			if info != nil {
				result.Found = info.Version
				a.setUpdateReady(info.Version, info.URL, nil)
			}
			a.setUpdateChecking(false)
			a.updateChecked(result)
			if info != nil {
				a.rt.logger.Printf("Update available: v%s at %s", info.Version, info.URL)
				return
			}
		}
		<-ch
		check = true
	}
}

// applyUpdate restarts into a staged update: restartForUpdate, then swap the
// binary, start the supervisor and quit through the ordinary shutdown.
func (a *app) applyUpdate(r update.Ready, installPath string) error {
	return a.restartForUpdate(r.Version, func(handoff string) error {
		_, err := update.Apply(update.UpdatesDir(a.rt.configDir), update.ApplyParams{
			From:        version,
			To:          r.Version,
			InstallPath: installPath,
			StagedPath:  r.StagedPath,
			OldPID:      os.Getpid(),
			HandoffPath: handoff,
		})
		if err != nil {
			return err
		}
		a.rt.logger.Printf("Restarting to finish the update to v%s", r.Version)
		if a.rt.quitForUpdate != nil {
			a.rt.quitForUpdate()
		} else {
			tray.Quit()
		}
		return nil
	})
}

// restartForUpdate is what every update restart shares. In order: ask when an
// AI edit or a program is running, wait for sync to finish what it holds, wait
// for in-flight writes to land and stop admitting new ones, hand the open
// sessions to target in the handoff file, then
// call install with its path. Until install returns nil every failure removes
// the handoff, reopens writes and leaves this process running as it was; after
// that the app stays marked as updating with writes closed, so a second click
// cannot start another restart and no save can land after the handoff.
func (a *app) restartForUpdate(target string, install func(handoffPath string) error) error {
	if !a.updating.CompareAndSwap(false, true) {
		return errors.New("an update restart is already under way")
	}
	a.changed()
	exiting := false
	defer func() {
		if !exiting {
			a.reopenWrites()
			a.updating.Store(false)
			a.changed()
		}
	}()

	if a.runningWork() > 0 {
		msg := fmt.Sprintf("Restart HTML Clay now to update to HTML Clay %s?\n\nAn AI edit or a program started by a document is still running, and restarting stops it.", target)
		ok, err := a.confirmTrayChange("HTML Clay", msg, appcore.TextUpdateSyncBusyRestart)
		if err != nil || !ok {
			return errUpdateCancelled
		}
	}
	if !a.confirmQuit() {
		return errUpdateCancelled
	}
	// A delete or an upload the sync engine still holds is lost by a restart, and
	// the server then sends the old file back. Wait for it, and ask before going on without it.
	if !a.syncQuietForUpdate(updateSyncLimit) {
		ok, err := a.confirmTrayChange(appcore.TextUpdateSyncBusyTitle, fmt.Sprintf(appcore.TextUpdateSyncBusyBody, target), appcore.TextUpdateSyncBusyRestart)
		if err != nil || !ok {
			return errUpdateCancelled
		}
	}
	// Open sheets save what they hold now: drainWrites below closes the server to saves before
	// shutdown's own flush runs.
	a.rt.ls.FlushOpenSheets(server.FlushWait)
	if !waitQuiet(&a.rt.writesInFlight, updateQuietFor, updateQuietLimit) {
		return errUpdateBusy
	}
	if !a.drainWrites(updateQuietLimit) {
		return errUpdateBusy
	}

	handoff := filepath.Join(a.rt.configDir, updateHandoffFile)
	if err := a.writeUpdateHandoff(handoff, target); err != nil {
		os.Remove(handoff)
		return fmt.Errorf("write the session handoff: %w", err)
	}
	if err := install(handoff); err != nil {
		os.Remove(handoff)
		return err
	}
	exiting = true
	return nil
}

// syncQuietForUpdate waits up to limit until the sync engine holds nothing a
// restart would drop: no first download, no session starting, nothing queued.
// With no service running there is no sync to wait for. A service that cannot
// be asked does not hold the update back, because the update may be its repair.
func (a *app) syncQuietForUpdate(limit time.Duration) bool {
	if a.rt.syncQuiet != nil {
		return a.rt.syncQuiet(limit)
	}
	if a.served == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit+5*time.Second)
	defer cancel()
	body := struct {
		TimeoutMs int64 `json:"timeoutMs"`
		ForUpdate bool  `json:"forUpdate"`
	}{limit.Milliseconds(), true}
	var answer struct {
		Idle bool `json:"idle"`
	}
	if err := a.served.CallService(ctx, "/sync/v1/idle", body, &answer); err != nil {
		if !errors.Is(err, served.ErrServiceUnavailable) {
			a.rt.logger.Printf("Could not ask sync to finish before the update: %v", err)
		}
		return true
	}
	return answer.Idle
}

// runningWork counts the AI edits, helper program requests and built-in
// program requests running across every live site and served folder, the same
// set cancelAIEdits and cancelBuiltins stop.
func (a *app) runningWork() int {
	a.mu.Lock()
	sites := append([]*site(nil), a.sites...)
	a.mu.Unlock()
	n := 0
	for _, s := range sites {
		if s.srv != nil {
			n += s.srv.RunningWork()
		}
	}
	for _, core := range a.servedWireCores() {
		n += core.RunningWork()
	}
	return n
}

// countWrites wraps the one HTTP handler and counts requests that can change
// a file: saves, uploads, restores. Reads are left out on purpose, because the
// live-sync and wire streams are GETs that stay open for as long as a tab
// does, and counting them would mean the app is never quiet. While an update
// restart has writes closed, a write is refused with a retriable 503 instead.
func (a *app) countWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		// Offline copies are a cache: a hand-over cut short loses nothing, and counting
		// them would keep an update or a port change waiting on a page's fetches.
		if mirror.Owns(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !a.admitWrite() {
			refuseWriteWhileRestarting(w, a.restartingText())
			return
		}
		defer a.rt.writesInFlight.Add(-1)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), heldWriteKey{}, true)))
	})
}

type heldWriteKey struct{}

// heldWrites is 1 when ctx belongs to a request countWrites admitted as a
// write, such as the browser's action that changes the port. A restart it asks
// for waits for every other write, never for the request asking.
func heldWrites(ctx context.Context) int64 {
	if held, _ := ctx.Value(heldWriteKey{}).(bool); held {
		return 1
	}
	return 0
}

// admitWrite counts one write in, unless writes are closed. The check and the
// count happen under one lock, so once drainWrites has closed admission the
// count can only fall.
func (a *app) admitWrite() bool {
	a.rt.writesMu.Lock()
	defer a.rt.writesMu.Unlock()
	if a.rt.writesClosed {
		return false
	}
	a.rt.writesInFlight.Add(1)
	return true
}

// drainWrites closes admission and waits up to limit for every admitted write
// to finish. On timeout it reopens admission and reports false, so the app
// keeps running as it was.
func (a *app) drainWrites(limit time.Duration) bool {
	return a.drainWritesTo(0, limit)
}

// drainWritesTo is drainWrites for a caller that is itself one of held admitted
// writes: it waits for the count to fall to held, not to zero.
func (a *app) drainWritesTo(held int64, limit time.Duration) bool {
	a.rt.writesMu.Lock()
	a.rt.writesClosed = true
	a.rt.writesMu.Unlock()
	deadline := time.Now().Add(limit)
	for a.rt.writesInFlight.Load() > held {
		if time.Now().After(deadline) {
			a.reopenWrites()
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// waitWritesIdle waits up to limit for the writes already admitted to finish. It
// never closes admission, so unlike drainWrites the app keeps taking writes while
// it waits: it is for a change that must not run headlong into a save already on
// its way, not for a restart.
func (a *app) waitWritesIdle(limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for a.rt.writesInFlight.Load() != 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func (a *app) reopenWrites() {
	a.rt.writesMu.Lock()
	a.rt.writesClosed = false
	a.rt.writesClosedForPort = false
	a.rt.writesMu.Unlock()
}

// restartingText is the sentence a refused write gets while writes are closed.
func (a *app) restartingText() string {
	a.rt.writesMu.Lock()
	defer a.rt.writesMu.Unlock()
	if a.rt.writesClosedForPort {
		return appcore.TextSaveRefusedPortRestart
	}
	return appcore.TextSaveRefusedUpdateRestart
}

// refuseWriteWhileRestarting answers a write that arrived after the restart
// closed admission. ClayJS waits Retry-After and sends the same save again, also
// through the refused connections while no version listens, so it lands on the
// new version a few seconds later (clayjs src/core/save-core.js, `restarting`).
func refuseWriteWhileRestarting(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "2")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      false,
		"msg":     text,
		"msgType": "error",
		"code":    "restarting",
	})
}

// waitQuiet reports whether inFlight stayed at zero for quiet, waiting at
// most limit for that to happen.
func waitQuiet(inFlight *atomic.Int64, quiet, limit time.Duration) bool {
	return waitQuietTo(inFlight, 0, quiet, limit)
}

// waitQuietTo is waitQuiet for a caller that is itself one of held admitted writes.
func waitQuietTo(inFlight *atomic.Int64, held int64, quiet, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	var since time.Time
	for {
		now := time.Now()
		if inFlight.Load() <= held {
			if since.IsZero() {
				since = now
			}
			if now.Sub(since) >= quiet {
				return true
			}
		} else {
			since = time.Time{}
		}
		if now.After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
