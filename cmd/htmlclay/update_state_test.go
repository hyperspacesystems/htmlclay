package main

import (
	"context"
	"testing"

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
