package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
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
