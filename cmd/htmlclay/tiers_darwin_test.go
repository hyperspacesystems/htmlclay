//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/tier"
	"golang.org/x/sys/unix"
)

func TestUpgradeTrustsWhatWasOpenedBefore(t *testing.T) {
	a := newTestApp(t, evalTempHome(t))
	folder := real(t, t.TempDir())
	a.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))
	a.rt.configDir = filepath.Join(t.TempDir(), "htmlclay")
	if err := os.MkdirAll(a.rt.configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	opened, err := recents.Open(a.rt.configDir)
	if err != nil {
		t.Fatal(err)
	}
	made := filepath.Join(folder, "made.html")
	downloaded := filepath.Join(folder, "downloaded.html")
	later := filepath.Join(folder, "later.html")
	for _, path := range []string{made, downloaded, later} {
		writeTestFile(t, path, tierTestPage)
	}
	if err := unix.Setxattr(downloaded, "com.apple.quarantine", []byte("0081;6720a3b1;Safari;"), 0); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{made, downloaded} {
		if err := opened.Touch(recents.Entry{Path: path}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	store := a.openTiers(opened)
	if store == nil {
		t.Fatal("no trust store")
	}
	if rec, ok := store.PathTrust(made); !ok || rec.How != tier.HowUpgrade {
		t.Fatalf("a document opened before the upgrade = %+v, %v, want trusted by the upgrade", rec, ok)
	}
	if _, ok := store.PathTrust(downloaded); ok {
		t.Fatal("the upgrade trusted a download")
	}
	if u, ok := store.Upgrade(); !ok || !slices.Equal(u.Folders, []string{folder}) {
		t.Fatalf("upgrade record = %+v, %v, want the trusted folders", u, ok)
	}

	if err := opened.Touch(recents.Entry{Path: later}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if store = a.openTiers(opened); store == nil {
		t.Fatal("no trust store on the second launch")
	}
	if _, ok := store.PathTrust(later); ok {
		t.Fatal("a launch after the upgrade trusted a document by the recent list")
	}
}

func TestDownloadedFolderProgramNeverRuns(t *testing.T) {
	s := newTierStand(t)
	mine := filepath.Join(s.folder, "mine.html")
	writeTestFile(t, mine, helperPage)
	madeHere(t, s.app, mine)
	program := writeFolderProgram(t, s.app.rt.home, "search", "#!/bin/sh\necho ok\n")
	if !s.app.helperGranted(mine, "search", folderProgramPrefix+"search") {
		t.Fatal("fixture: the person's own page cannot start an unmarked folder program")
	}
	if err := unix.Setxattr(program, "com.apple.quarantine", []byte("0081;6720a3b1;Safari;"), 0); err != nil {
		t.Fatal(err)
	}
	if s.app.helperGranted(mine, "search", folderProgramPrefix+"search") {
		t.Fatal("a downloaded program in the programs folder may start")
	}
}
