package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/shellapi"
	"github.com/panphora/htmlclay/internal/update"
)

// updateState is what the host knows about updates on Windows and Linux, for
// the snapshot's update and the one menu's status row. The Mac app reads Sparkle.
type updateState struct {
	mu        sync.Mutex
	checking  bool
	checkedAt time.Time
	ready     string
	url       string
	apply     func()
	checkNow  chan struct{}
	// downloading is the newer version the updater is downloading now.
	downloading string
	lastCheck   *appcore.UpdateCheck
	// announce is set by the tray's Check for updates: the tray has no page,
	// so the check's result is told in a notification.
	announce bool
	// sparkle is the version the Mac app's updater has downloaded and will
	// install at its next quit, as the app last reported it; "" for none.
	sparkle string
}

func (a *app) setUpdateChecking(on bool) {
	a.updates.mu.Lock()
	a.updates.checking = on
	if !on {
		a.updates.checkedAt = time.Now().UTC()
	}
	a.updates.mu.Unlock()
	a.changed()
}

func (a *app) setUpdateReady(version, url string, apply func()) {
	a.updates.mu.Lock()
	a.updates.ready, a.updates.url, a.updates.apply = version, url, apply
	a.updates.mu.Unlock()
	a.changed()
}

// setUpdateDownloading is the updater finding a newer version before its
// download starts. A check the tray asked for says so now, not after the
// download.
func (a *app) setUpdateDownloading(version string) {
	a.updates.mu.Lock()
	a.updates.downloading = version
	announce := a.updates.announce
	a.updates.mu.Unlock()
	a.changed()
	if announce {
		a.announceUpdate(appcore.TextUpdateFoundTitle, fmt.Sprintf(appcore.TextUpdateDownloadingBody, version))
	}
}

// updateChecked records how a check ended, which the Updates page shows, and
// tells a person who asked from the tray.
func (a *app) updateChecked(r update.CheckResult) {
	result := appcore.UpdateLatest
	switch {
	case r.Err != nil:
		result = appcore.UpdateFailed
	case r.Found != "":
		result = appcore.UpdateFound
	}
	a.updates.mu.Lock()
	told := a.updates.downloading != ""
	a.updates.downloading = ""
	a.updates.lastCheck = &appcore.UpdateCheck{At: time.Now().UTC(), Result: result}
	announce := a.updates.announce
	a.updates.announce = false
	a.updates.mu.Unlock()
	a.changed()
	if !announce {
		return
	}
	switch {
	case result == appcore.UpdateFailed:
		a.announceUpdate(appcore.TextUpdateFailedTitle, appcore.TextUpdateFailedBody)
	case result == appcore.UpdateLatest:
		a.announceUpdate(appcore.TextUpdateLatestTitle, fmt.Sprintf(appcore.TextUpdateLatestBody, version))
	case !told:
		a.announceUpdate(appcore.TextUpdateFoundTitle, fmt.Sprintf(appcore.TextUpdateAvailableBody, r.Found))
	}
}

// checkForUpdatesFromTray is the tray's Check for updates: update.check, with
// its result told in a notification. An update already downloaded is told at once.
func (a *app) checkForUpdatesFromTray() *appcore.Refusal {
	a.updates.mu.Lock()
	ready, url := a.updates.ready, a.updates.url
	a.updates.announce = ready == ""
	a.updates.mu.Unlock()
	if ready != "" {
		body := appcore.TextUpdateReadyBody
		if url != "" {
			body = appcore.TextUpdateAvailableBody
		}
		a.announceUpdate(appcore.TextUpdateFoundTitle, fmt.Sprintf(body, ready))
		return nil
	}
	refusal := a.trayDo("update.check", map[string]any{})
	if refusal != nil {
		a.updates.mu.Lock()
		a.updates.announce = false
		a.updates.mu.Unlock()
	}
	return refusal
}

func (a *app) announceUpdate(title, body string) {
	// On Windows the notice is a message box that waits for OK; the updater
	// goes on downloading meanwhile.
	go func() {
		if err := a.informUser(title, body); err != nil {
			a.rt.logger.Printf("Could not show the update check's result: %v", err)
		}
	}()
}

// updateFacts fills the snapshot's update from what the updater reported.
func (a *app) updateFacts(u *appcore.UpdateState) {
	a.updates.mu.Lock()
	defer a.updates.mu.Unlock()
	u.Checking = a.updates.checking
	u.Downloading = a.updates.downloading
	if a.updates.lastCheck != nil {
		last := *a.updates.lastCheck
		u.LastCheck = &last
	}
	if !a.updates.checkedAt.IsZero() {
		at := a.updates.checkedAt
		u.CheckedAt = &at
	}
	// On the Mac only Sparkle's download is ready: the host's own release
	// check there links to the website, and the app shows nothing of it.
	if a.shell {
		if a.updates.sparkle != "" {
			v := a.updates.sparkle
			u.Ready = &v
		}
		return
	}
	if a.updates.ready != "" {
		v := a.updates.ready
		u.Ready = &v
		u.DownloadURL = a.updates.url
		u.Finishing = a.updating.Load()
	}
}

// setSparklePrepared is update.prepared: the Mac app reports what its updater
// has downloaded, so the browser pages can offer Update.
func (a *app) setSparklePrepared(version string) {
	a.updates.mu.Lock()
	a.updates.sparkle = version
	a.updates.mu.Unlock()
}

// setAutoUpdate is prefs.set autoUpdate. On Windows and Linux off stops the
// timed checks and Check now still works; on turns them on and checks at once.
// On macOS the host persists the preference so the Settings switch always works
// (even on Candidate/unsigned builds) and the Mac app mirrors it to Sparkle.
func (a *app) setAutoUpdate(on bool) *appcore.Refusal {
	a.rt.cfg.SetSelfUpdateOff(!on)
	a.saveConfig("preference autoUpdate")
	if a.shell || updateNowOS == "darwin" || !on {
		return nil
	}
	a.updates.mu.Lock()
	ch, busy := a.updates.checkNow, a.updates.ready != "" || a.updates.checking
	a.updates.mu.Unlock()
	if ch != nil && !busy {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return nil
}

// checkForUpdates is update.check: it starts a check at once. The Updates page
// shows what it found; the tray's check is told in a notification.
func (a *app) checkForUpdates() *appcore.Refusal {
	a.updates.mu.Lock()
	if a.updates.ready != "" || a.updates.checking {
		a.updates.mu.Unlock()
		return nil
	}
	ch := a.updates.checkNow
	a.updates.mu.Unlock()
	if ch == nil {
		return &appcore.Refusal{Code: appcore.RefuseNotYet}
	}
	select {
	case ch <- struct{}{}:
	default:
	}
	return nil
}

// startUpdate is update.start: restart into a staged update, or open the
// download page when this build cannot install it itself.
func (a *app) startUpdate() *appcore.Refusal {
	a.updates.mu.Lock()
	apply, url, sparkle := a.updates.apply, a.updates.url, a.updates.sparkle
	a.updates.mu.Unlock()
	switch {
	case a.shell:
		// The Mac app restarts into the update its updater holds.
		if sparkle == "" {
			return &appcore.Refusal{Code: appcore.RefuseNothingReady}
		}
		if !a.rt.notices.deliver(shellapi.Notice{Type: shellapi.FrameRestartToUpdate}) {
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
	case apply != nil:
		go apply()
	case url != "":
		a.openURL(url)
	default:
		return &appcore.Refusal{Code: appcore.RefuseNothingReady}
	}
	return nil
}

// openURL opens u in the person's browser through the runtime seam.
func (a *app) openURL(u string) {
	a.launchBrowser(u)
}
