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
	"github.com/panphora/htmlclay/internal/served"
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
	// The restart is offered from the tray, so a run without one has no way to
	// apply what it would download.
	if a.noTray {
		a.updateOff = "HTML Clay is running without its tray"
		return false
	}
	if updateNowOS == "darwin" {
		// On macOS the HTML Clay app runs Sparkle; a host started on its own keeps the download-page link.
		a.updateOff = "on macOS the HTML Clay app updates itself"
		return false
	}
	installPath, err := update.InstallPath()
	if err == nil {
		err = selfUpdateGate(updateChannel, version, updateNowOS, len(update.TrustedKeys), installPath, update.CheckWritable)
	}
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
	go u.Run(nil, func(r update.Ready) {
		a.setUpdateReady(r.Version, "", func() { a.clickApplyUpdate(r, installPath) })
	})
	return true
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
	msg := fmt.Sprintf("HTML Clay could not restart into v%s just now, so it is still running v%s. Try Restart to Update again in a moment.", target, version)
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
			info := update.Check(version, releaseInfoURL)
			if info != nil {
				a.setUpdateReady(info.Version, info.URL, nil)
			}
			a.setUpdateChecking(false)
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
		msg := fmt.Sprintf("Restart HTML Clay now to update to v%s?\n\nAn AI edit or a program started by a document is still running, and restarting stops it.", target)
		ok, err := a.confirmTrayChange("HTML Clay", msg, "Restart")
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

// runningWork counts the AI edits and helper program requests running across
// every live site.
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
		if !a.admitWrite() {
			refuseWriteWhileRestarting(w)
			return
		}
		defer a.rt.writesInFlight.Add(-1)
		next.ServeHTTP(w, r)
	})
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
	a.rt.writesMu.Lock()
	a.rt.writesClosed = true
	a.rt.writesMu.Unlock()
	deadline := time.Now().Add(limit)
	for a.rt.writesInFlight.Load() != 0 {
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
	a.rt.writesMu.Unlock()
}

// refuseWriteWhileRestarting answers a write that arrived after the restart
// closed admission. ClayJS waits Retry-After and sends the same save again, also
// through the refused connections while no version listens, so it lands on the
// new version a few seconds later (clayjs src/core/save-core.js, `restarting`).
func refuseWriteWhileRestarting(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "2")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      false,
		"msg":     "HTML Clay is restarting to finish an update, so nothing was saved. Try again in a few seconds.",
		"msgType": "error",
		"code":    "restarting",
	})
}

// waitQuiet reports whether inFlight stayed at zero for quiet, waiting at
// most limit for that to happen.
func waitQuiet(inFlight *atomic.Int64, quiet, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	var since time.Time
	for {
		now := time.Now()
		if inFlight.Load() == 0 {
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
