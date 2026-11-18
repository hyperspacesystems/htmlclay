package main

import (
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// updateState is what the host knows about updates on Windows and Linux, for
// the snapshot's update and the one menu's status row. The Mac app reads Sparkle.
type updateState struct {
	mu        sync.Mutex
	checking  bool
	manual    bool
	checkedAt time.Time
	ready     string
	url       string
	apply     func()
	checkNow  chan struct{}
}

func (a *app) setUpdateChecking(on bool) {
	a.updates.mu.Lock()
	a.updates.checking = on
	manual := false
	if !on {
		a.updates.checkedAt = time.Now().UTC()
		manual, a.updates.manual = a.updates.manual, false
	}
	found := a.updates.ready != ""
	a.updates.mu.Unlock()
	a.changed()
	if manual && !found {
		if err := a.informUser("HTML Clay", appcore.TextUpToDateNotice); err != nil {
			a.rt.logger.Printf("Could not show the update notice: %v", err)
		}
	}
}

func (a *app) setUpdateReady(version, url string, apply func()) {
	a.updates.mu.Lock()
	a.updates.ready, a.updates.url, a.updates.apply = version, url, apply
	a.updates.mu.Unlock()
	a.changed()
}

// updateFacts fills the snapshot's update from what the updater reported.
func (a *app) updateFacts(u *appcore.UpdateState) {
	a.updates.mu.Lock()
	defer a.updates.mu.Unlock()
	u.Checking = a.updates.checking
	if !a.updates.checkedAt.IsZero() {
		at := a.updates.checkedAt
		u.CheckedAt = &at
	}
	if a.updates.ready != "" {
		v := a.updates.ready
		u.Ready = &v
		u.DownloadURL = a.updates.url
	}
}

// checkForUpdates is update.check: it starts a check at once and notifies when
// nothing newer was found.
func (a *app) checkForUpdates() *appcore.Refusal {
	a.updates.mu.Lock()
	if a.updates.ready != "" || a.updates.checking {
		a.updates.mu.Unlock()
		return nil
	}
	a.updates.manual = true
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
	apply, url := a.updates.apply, a.updates.url
	a.updates.mu.Unlock()
	switch {
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
