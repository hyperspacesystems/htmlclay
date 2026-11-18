package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/testutil"
	"github.com/panphora/htmlclay/internal/update"
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

// The tray's Check for updates tells its result in a notification, because the
// tray has no page to show it on; a page's check shows its result in the
// snapshot only, so one click never shows the answer twice.
func TestTrayCheckForUpdatesSaysWhatItFoundAndAPageCheckDoesNot(t *testing.T) {
	var receipt atomic.Value
	receipt.Store(version)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		latest := receipt.Load().(string)
		if latest == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"latest": latest, "url": "https://example.invalid/dl"})
	}))
	t.Cleanup(srv.Close)
	saved := releaseInfoURL
	releaseInfoURL = srv.URL
	t.Cleanup(func() { releaseInfoURL = saved })

	a := newTestApp(t, t.TempDir())
	a.rt.cfg.SetSelfUpdateOff(true)
	var mu sync.Mutex
	var told []string
	a.rt.notify = func(title, body string) error {
		mu.Lock()
		told = append(told, title+": "+body)
		mu.Unlock()
		return nil
	}
	a.rt.launchBrowser = func(target string) error {
		t.Errorf("Check for updates opened %s", target)
		return nil
	}
	notices := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), told...)
	}
	go a.checkForUpdateLink()
	testutil.Eventually(t, 2*time.Second, "the update check waits for Check now", func() bool {
		a.updates.mu.Lock()
		defer a.updates.mu.Unlock()
		return a.updates.checkNow != nil
	})
	lastCheck := func() string {
		if c := a.facts().Update.LastCheck; c != nil {
			return c.Result
		}
		return ""
	}
	clearLast := func() {
		a.updates.mu.Lock()
		a.updates.lastCheck = nil
		a.updates.mu.Unlock()
	}

	if _, refusal := a.do(context.Background(), appcore.Action{ID: "page-check", Type: "update.check"}, ChannelDashboard); refusal != nil {
		t.Fatalf("a page's update.check = %+v", refusal)
	}
	testutil.Eventually(t, 5*time.Second, "the page's check to end", func() bool { return lastCheck() == appcore.UpdateLatest })
	time.Sleep(50 * time.Millisecond)
	if got := notices(); len(got) != 0 {
		t.Fatalf("a page's check showed %v; the page shows the result itself", got)
	}

	clearLast()
	a.doTrayMenu(appcore.MenuCheckUpdates)
	testutil.Eventually(t, 5*time.Second, "the tray's check to say it is the newest", func() bool { return len(notices()) == 1 })
	if want := appcore.TextUpdateLatestTitle + ": " + fmt.Sprintf(appcore.TextUpdateLatestBody, version); notices()[0] != want {
		t.Fatalf("the tray's check said %q, want %q", notices()[0], want)
	}

	receipt.Store("")
	clearLast()
	a.doTrayMenu(appcore.MenuCheckUpdates)
	testutil.Eventually(t, 5*time.Second, "the tray's check to say it failed", func() bool { return len(notices()) == 2 })
	if lastCheck() != appcore.UpdateFailed || notices()[1] != appcore.TextUpdateFailedTitle+": "+appcore.TextUpdateFailedBody {
		t.Fatalf("a failed check = %q, told %q", lastCheck(), notices()[1])
	}

	receipt.Store("99.0.0")
	clearLast()
	a.doTrayMenu(appcore.MenuCheckUpdates)
	testutil.Eventually(t, 5*time.Second, "the tray's check to say what it found", func() bool { return len(notices()) == 3 })
	if want := appcore.TextUpdateFoundTitle + ": " + fmt.Sprintf(appcore.TextUpdateAvailableBody, "99.0.0"); lastCheck() != appcore.UpdateFound || notices()[2] != want {
		t.Fatalf("a check that found 99.0.0 = %q, told %q", lastCheck(), notices()[2])
	}
}

// The status row says Downloading while a found update downloads, and the
// download's end clears it. The notice a tray check shows never holds up the
// download: on Windows it is a message box that waits for OK.
func TestTheStatusRowSaysDownloadingUntilTheCheckEnds(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	release := make(chan struct{})
	defer close(release)
	a.rt.notify = func(string, string) error { <-release; return nil }
	a.updates.mu.Lock()
	a.updates.announce = true
	a.updates.mu.Unlock()
	a.setUpdateChecking(true)
	found := make(chan struct{})
	go func() { a.setUpdateDownloading("9.9.9"); close(found) }()
	select {
	case <-found:
	case <-time.After(2 * time.Second):
		t.Fatal("the found notice held up the download until the person answered it")
	}
	s := a.snapshot()
	rows := appcore.MenuRows(s, appcore.PhaseOf(s))
	if s.Update.Downloading != "9.9.9" || rows[0].Kind != appcore.MenuStatusDownloading {
		t.Fatalf("while downloading: update.downloading = %q, status row %q", s.Update.Downloading, rows[0].Kind)
	}
	if got, want := appcore.MenuTitle(rows[0], s, appcore.PhaseOf(s)), fmt.Sprintf(appcore.TextMenuDownloading, "9.9.9"); got != want {
		t.Fatalf("status row = %q, want %q", got, want)
	}
	a.setUpdateChecking(false)
	a.updateChecked(update.CheckResult{Err: fmt.Errorf("the download broke")})
	s = a.snapshot()
	if s.Update.Downloading != "" || s.Update.LastCheck == nil || s.Update.LastCheck.Result != appcore.UpdateFailed {
		t.Fatalf("after a failed download: downloading %q, last check %+v", s.Update.Downloading, s.Update.LastCheck)
	}
}

// The Mac app checks for updates with Sparkle, so a host it started never checks
// itself, and only a host the Mac app started hears what Sparkle downloaded.
func TestOnlyTheMacAppChecksAndReportsItsUpdates(t *testing.T) {
	shell := newTestAppAs(t, t.TempDir(), t.TempDir(), true)
	if slices.Contains(shell.implemented(), "update.check") {
		t.Fatal("a host the Mac app started offers update.check")
	}
	own := newTestAppAs(t, t.TempDir(), t.TempDir(), false)
	if slices.Contains(own.implemented(), "update.prepared") {
		t.Fatal("a host without the Mac app offers update.prepared")
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

// On the Mac the browser pages offer Update for what Sparkle downloaded: the
// app reports the version, a page's Update asks the app to restart into it,
// and only the app may report a version.
func TestTheMacPagesOfferWhatSparkleDownloaded(t *testing.T) {
	a, _ := updateTestApp(t)
	a.shell = true
	a.setUpdateReady("9.9.9", "https://htmlclay.com/download", nil)
	if s := a.snapshot(); s.Update.Ready != nil {
		t.Fatalf("the host's own release check shows as ready on the Mac: %q", *s.Update.Ready)
	}
	frames, stop := a.rt.notices.subscribe()
	defer stop()

	if _, refusal := a.do(context.Background(), coreAction(t, "page", "update.prepared", map[string]string{"version": "2.1.0"}), ChannelDashboard); refusal == nil {
		t.Fatal("a browser page reported a downloaded version")
	}
	if _, refusal := a.do(context.Background(), coreAction(t, "app", "update.prepared", map[string]string{"version": "2.1.0"}), ChannelShell); refusal != nil {
		t.Fatalf("update.prepared from the Mac app: %+v", refusal)
	}
	s := a.snapshot()
	if s.Update.Ready == nil || *s.Update.Ready != "2.1.0" || s.Update.DownloadURL != "" || !s.Host.Can["update.start"] {
		t.Fatalf("update = %+v, can update.start %v; want 2.1.0 ready to start", s.Update, s.Host.Can["update.start"])
	}
	if _, refusal := a.do(context.Background(), appcore.Action{ID: "update", Type: "update.start"}, ChannelDashboard); refusal != nil {
		t.Fatalf("Update on a page: %+v", refusal)
	}
	select {
	case frame := <-frames:
		if frame.Type != "restartToUpdate" {
			t.Fatalf("the Mac app got %+v, want restartToUpdate", frame)
		}
	default:
		t.Fatal("Update on a page never reached the Mac app")
	}
}
