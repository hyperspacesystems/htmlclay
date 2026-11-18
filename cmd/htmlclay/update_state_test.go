package main

import (
	"context"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// The download-page row opens the URL the release receipt named.
func TestUpdateStartOpensTheDownloadPage(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	var opened string
	a.rt.launchBrowser = func(target string) error {
		opened = target
		return nil
	}
	a.setUpdateReady("9.9.9", "https://example.invalid/dl", nil)
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "update.start"}); refusal != nil {
		t.Fatalf("update.start = %+v", refusal)
	}
	if opened != "https://example.invalid/dl" {
		t.Fatalf("opened %q, want the download page", opened)
	}
	f := a.facts()
	if f.Update.Ready == nil || *f.Update.Ready != "9.9.9" {
		t.Fatalf("update.ready = %v, want 9.9.9", f.Update.Ready)
	}
	if f.Update.DownloadURL != "https://example.invalid/dl" {
		t.Fatalf("update.downloadURL = %q", f.Update.DownloadURL)
	}
}

// A build with no updater behind it has nothing to check.
func TestUpdateCheckWithoutAnUpdaterIsNotYet(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "update.check"}); refusal == nil || refusal.Code != appcore.RefuseNotYet {
		t.Fatalf("update.check = %+v, want %s", refusal, appcore.RefuseNotYet)
	}
}

// The Mac app runs Sparkle, so a host it started offers neither update action.
func TestShellHostDoesNotOfferUpdates(t *testing.T) {
	a := newTestAppAs(t, t.TempDir(), t.TempDir(), true)
	for _, typ := range a.implemented() {
		if typ == "update.check" || typ == "update.start" {
			t.Fatalf("a shell host offers %s", typ)
		}
	}
}

func TestAutoUpdateSwitchOnWindowsAndLinux(t *testing.T) {
	orig := updateNowOS
	t.Cleanup(func() { updateNowOS = orig })
	updateNowOS = "linux"
	a, _ := updateTestApp(t)
	a.updates.checkNow = make(chan struct{}, 1)
	set := func(id string, on bool) *appcore.Refusal {
		action := coreAction(t, id, "prefs.set", map[string]any{"key": "autoUpdate", "value": on})
		_, refusal := a.do(context.Background(), action, ChannelDashboard)
		return refusal
	}

	if refusal := set("p1", false); refusal != nil {
		t.Fatalf("turning updates off was refused: %+v", refusal)
	}
	if !a.rt.cfg.SelfUpdateOff() || a.snapshot().Update.Automatic {
		t.Fatal("updates are still automatic after the switch went off")
	}
	select {
	case <-a.updates.checkNow:
		t.Fatal("turning updates off started a check")
	default:
	}

	if refusal := set("p2", true); refusal != nil {
		t.Fatalf("turning updates on was refused: %+v", refusal)
	}
	if a.rt.cfg.SelfUpdateOff() || !a.snapshot().Update.Automatic {
		t.Fatal("updates are not automatic after the switch went on")
	}
	select {
	case <-a.updates.checkNow:
	default:
		t.Fatal("turning updates on did not check at once")
	}

	updateNowOS = "darwin"
	if refusal := set("p3", false); refusal == nil || refusal.Code != appcore.RefuseNotYet {
		t.Fatalf("on macOS the switch belongs to the app, got %+v", refusal)
	}
}

func TestUpdateNowOnLinuxStartsTheDownloadedUpdate(t *testing.T) {
	origOS, origVersion := updateNowOS, version
	t.Cleanup(func() { updateNowOS, version = origOS, origVersion })
	updateNowOS, version = "linux", "9.0.0"
	a, _ := updateTestApp(t)
	a.updateOff = ""
	a.updates.checkNow = make(chan struct{}, 1)

	if got := a.updateNow("9.0.1"); got != updateNowChecking {
		t.Fatalf("update-now with nothing downloaded = %q, want %q", got, updateNowChecking)
	}
	select {
	case <-a.updates.checkNow:
	default:
		t.Fatal("update-now with nothing downloaded did not check now")
	}

	applied := make(chan struct{}, 1)
	a.setUpdateReady("9.0.1", "", func() { applied <- struct{}{} })
	if got := a.updateNow("9.0.2"); got != "refused the update HTML Clay has downloaded is v9.0.1, not v9.0.2" {
		t.Fatalf("update-now for another version = %q", got)
	}
	if got := a.updateNow("9.0.1"); got != "accepted" {
		t.Fatalf("update-now for the downloaded version = %q", got)
	}
	select {
	case <-applied:
	case <-time.After(5 * time.Second):
		t.Fatal("update-now never ran the restart")
	}
}
