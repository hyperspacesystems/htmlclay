package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/update"
)

// appBundleFor returns the .app that holds exe, an executable at
// X.app/Contents/MacOS/name, and false when exe is not inside one.
func appBundleFor(exe string) (string, bool) {
	macos := path.Dir(exe)
	contents := path.Dir(macos)
	bundle := path.Dir(contents)
	if path.Base(macos) != "MacOS" || path.Base(contents) != "Contents" || path.Ext(bundle) != ".app" {
		return "", false
	}
	return bundle, true
}

// updatesOffReason is why the gate turns self-update off when config says so.
const updatesOffReason = `config sets "updates": "off"`

// prepareShellUpdate is update-prepare: the Swift app is about to let Sparkle
// install target and asks the host, which is the only side that knows what is
// unsaved and can stop admitting saves, to close writes, write the session
// handoff for the open tabs and start the recovery supervisor. appPID is the
// app that asked. force skips the questions about running work and unsaved
// edits.
//
// Every answer except "prepared …" leaves the app saving as it was; after it,
// writes stay closed and the app is marked updating until it exits or
// update-abort.
func (a *app) prepareShellUpdate(target string, appPID int, quit, force bool) string {
	if !a.shell {
		return "failed not-shell"
	}
	if appPID != os.Getppid() {
		return "failed bad-pid"
	}
	if !update.IsStrictVersion(target) {
		return "failed bad-version"
	}
	exe, err := shellInstallPath()
	bundle, ok := appBundleFor(exe)
	if err != nil || !ok {
		return "failed not-in-bundle"
	}
	if !a.updating.CompareAndSwap(false, true) {
		return "failed busy"
	}
	kept := false
	defer func() {
		if !kept {
			a.reopenWrites()
			a.updating.Store(false)
		}
	}()
	if !force {
		if a.runningWork() > 0 {
			return "refused running-work"
		}
		lines, saved := a.unsavedBeforeQuit()
		if !saved {
			data, err := json.Marshal(lines)
			if err != nil {
				return "failed unsaved"
			}
			return "refused unsaved " + string(data)
		}
	}
	if !waitQuiet(&a.rt.writesInFlight, updateQuietFor, updateQuietLimit) {
		return "failed busy"
	}
	shellUpdateStep("quiet")
	if !a.drainWrites(updateQuietLimit) {
		return "failed busy"
	}
	shellUpdateStep("drained")

	handoff := ""
	if !quit {
		handoff = filepath.Join(a.rt.configDir, updateHandoffFile)
		if err := a.writeUpdateHandoff(handoff, target); err != nil {
			os.Remove(handoff)
			return "failed handoff"
		}
		shellUpdateStep("handoff")
	}

	shellUpdateStep("begin")
	rec, err := beginSparkle(update.UpdatesDir(a.rt.configDir), update.SparkleParams{
		From:        version,
		To:          target,
		Bundle:      bundle,
		Executable:  filepath.Join(bundle, "Contents", "MacOS", "htmlclay"),
		Helpers:     []string{"Contents/MacOS/htmlclay-host"},
		OldPID:      appPID,
		HandoffPath: handoff,
		Quit:        quit,
	})
	kept = true
	if err != nil {
		a.rt.logger.Printf("Installing v%s without recovery: %v", target, err)
		return "prepared -"
	}
	a.rt.logger.Printf("Recovery for v%s runs from %s", target, rec.PreviousPath)
	return "prepared " + rec.Txid
}

// abortShellUpdate is update-abort: the Swift app gave up on the update it
// prepared, so the host takes its handoff and recovery state back and saves
// again. A txid of "-" means no supervisor was started, so no state to clear.
func (a *app) abortShellUpdate(txid string) string {
	if !a.shell {
		return "failed not-shell"
	}
	a.reopenWrites()
	os.Remove(filepath.Join(a.rt.configDir, updateHandoffFile))
	if txid != "-" {
		update.ClearState(update.UpdatesDir(a.rt.configDir), txid)
	}
	os.RemoveAll(filepath.Join(update.UpdatesDir(a.rt.configDir), "previous"))
	a.updating.Store(false)
	return "aborted"
}

// shellUpdateStatus answers update-status: whether this host may update at all,
// and the verdict an earlier update's supervisor left behind.
func (a *app) shellUpdateStatus() string {
	if a.rt.cfg != nil && a.rt.cfg.SelfUpdateOff() {
		return "off " + updatesOffReason
	}
	st, err := update.ReadState(update.UpdatesDir(a.rt.configDir))
	if err != nil || st == nil || st.Failed == nil {
		return "ok"
	}
	return fmt.Sprintf("failed %s %s", st.Failed.Version, st.Failed.Reason)
}
