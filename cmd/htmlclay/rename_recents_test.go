package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/tier"
)

// TestM3RenameRecentsFollow proves a rename inside a served folder carries the
// recent-documents rows with it: the row names the document where it is, and a
// folder rename carries the rows below it.
func TestM3RenameRecentsFollow(t *testing.T) {
	f := newM3RelinkFixture(t)
	f.trust(t)
	served, err := f.app.rt.cfg.AddServedFolder(f.folder, platform.DirIdentity(f.folder), "docs")
	if err != nil {
		t.Fatal(err)
	}
	store := f.app.recentsStore()
	if store == nil {
		t.Fatal("the app has no recent-documents record")
	}
	rows := []string{
		filepath.Join(served.Path, "old.html"),
		filepath.Join(served.Path, "sub", "old.html"),
	}
	for _, path := range rows {
		entry := recents.Entry{ID: recentID(path), Path: path, How: recents.HowServed}
		if err := store.Touch(entry, time.Now().UTC()); err != nil {
			t.Fatalf("record %s: %v", path, err)
		}
	}

	f.app.noteServedStructure(served.ID, "old.html", "new.html", false)
	f.app.noteServedStructure(served.ID, "sub", "moved", true)

	wanted := []string{
		filepath.Join(served.Path, "new.html"),
		filepath.Join(served.Path, "moved", "old.html"),
	}
	listed := store.List()
	for _, want := range wanted {
		found := false
		for _, entry := range listed {
			if entry.Path == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("the record has no row for %s: %+v", want, listed)
		}
	}
	for _, entry := range listed {
		if entry.Path == rows[0] || entry.Path == rows[1] {
			t.Fatalf("the record still names %s: %+v", entry.Path, listed)
		}
	}
}

// TestM3TrustFollowsAServedMoveAndCopy is Carbon 7: a rename inside a served
// folder carries the trust records with it, and a duplicate of a download is a
// download, not the person's own document.
func TestM3TrustFollowsAServedMoveAndCopy(t *testing.T) {
	f := newM3RelinkFixture(t)
	f.trust(t)
	served, err := f.app.rt.cfg.AddServedFolder(f.folder, platform.DirIdentity(f.folder), "docs")
	if err != nil {
		t.Fatal(err)
	}
	f.app.tierStore().SetDeveloper(false)
	mine := filepath.Join(served.Path, "notes.html")
	stranger := filepath.Join(served.Path, "downloaded.html")
	writeTestFile(t, mine, tierTestPage)
	writeTestFile(t, stranger, tierTestPage)
	madeHere(t, f.app, mine)
	downloadedHere(t, f.app, stranger)

	renamed := filepath.Join(served.Path, "renamed.html")
	if err := os.Rename(mine, renamed); err != nil {
		t.Fatal(err)
	}
	f.app.noteServedStructure(served.ID, "notes.html", "renamed.html", false)
	if v := f.app.tierOf(renamed); v.Tier != tier.Mine {
		t.Fatalf("a renamed document of the person's own is %s %s", v.Tier, v.Reason)
	}

	created := filepath.Join(served.Path, "created.html")
	writeTestFile(t, created, tierTestPage)
	f.app.noteServedCreate(served.ID, "created.html")
	if v := f.app.tierOf(created); v.Tier != tier.Mine {
		t.Fatalf("a document the workspace created in a served folder is %s %s, want the person's own", v.Tier, v.Reason)
	}

	copyPath := filepath.Join(served.Path, "copy.html")
	writeTestFile(t, copyPath, tierTestPage)
	f.app.noteServedCopy(served.ID, "downloaded.html", "copy.html")
	if v := f.app.tierOf(copyPath); v.Tier != tier.Unknown {
		t.Fatalf("a copy of a download is %s %s, want unknown", v.Tier, v.Reason)
	}
}
