package recents_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/testutil"
)

func m3At(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
}

func m3Open(t *testing.T, dir string) *recents.Store {
	t.Helper()
	store, err := recents.Open(dir)
	if err != nil {
		t.Fatalf("Open(%q): %v", dir, err)
	}
	return store
}

func m3Touch(t *testing.T, store *recents.Store, entry recents.Entry, at time.Time) {
	t.Helper()
	if err := store.Touch(entry, at); err != nil {
		t.Fatalf("Touch(%q): %v", entry.Path, err)
	}
}

func m3Paths(entries []recents.Entry) []string {
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}
	return paths
}

// TestM3RecentsStore is the proof for step 7.15: the recents record is its own
// file in the config folder, ids survive a restart, a bad file never stops the
// app, and every write renames a temporary file into place.
func TestM3RecentsStore(t *testing.T) {
	at := m3At(t)

	t.Run("touch adds at the front and moves an existing entry", func(t *testing.T) {
		dir := t.TempDir()
		store := m3Open(t, dir)

		m3Touch(t, store, recents.Entry{Path: "/docs/a.clay", How: recents.HowFile}, at)
		m3Touch(t, store, recents.Entry{Path: "/docs/b.clay", How: recents.HowTrusted}, at.Add(time.Minute))
		m3Touch(t, store, recents.Entry{Path: "/docs/c.clay", How: recents.HowServed}, at.Add(2*time.Minute))

		got := m3Paths(store.List())
		want := []string{"/docs/c.clay", "/docs/b.clay", "/docs/a.clay"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("after three opens the record is %v, want %v", got, want)
		}

		m3Touch(t, store, recents.Entry{Path: "/docs/a.clay", Identity: "id-2", How: recents.HowServed}, at.Add(3*time.Minute))
		list := store.List()
		got = m3Paths(list)
		want = []string{"/docs/a.clay", "/docs/c.clay", "/docs/b.clay"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("after reopening a the record is %v, want %v", got, want)
		}
		if len(list) != 3 {
			t.Fatalf("reopening added a row: %d rows", len(list))
		}
		if list[0].Identity != "id-2" || list[0].How != recents.HowServed {
			t.Fatalf("the moved row kept the old details: %+v", list[0])
		}
		if !list[0].OpenedAt.Equal(at.Add(3 * time.Minute)) {
			t.Fatalf("the moved row's opened time is %v, want %v", list[0].OpenedAt, at.Add(3*time.Minute))
		}

		onDisk := m3Open(t, dir)
		if got := m3Paths(onDisk.List()); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("the record on disk is %v, want %v", got, want)
		}
	})

	t.Run("ids survive a restart", func(t *testing.T) {
		dir := t.TempDir()
		store := m3Open(t, dir)
		m3Touch(t, store, recents.Entry{Path: "/docs/a.clay"}, at)
		m3Touch(t, store, recents.Entry{Path: "/docs/b.clay"}, at)

		first := map[string]string{}
		for _, entry := range store.List() {
			if len(entry.ID) != 16 {
				t.Fatalf("id %q for %q is not 16 characters", entry.ID, entry.Path)
			}
			if _, err := hex.DecodeString(entry.ID); err != nil {
				t.Fatalf("id %q for %q is not hex: %v", entry.ID, entry.Path, err)
			}
			first[entry.Path] = entry.ID
		}

		restarted := m3Open(t, dir)
		if restarted.List()[0].ID != first["/docs/b.clay"] || restarted.List()[1].ID != first["/docs/a.clay"] {
			t.Fatalf("ids changed at a restart: %+v, want %v", restarted.List(), first)
		}

		m3Touch(t, restarted, recents.Entry{Path: "/docs/a.clay"}, at.Add(time.Hour))
		if got := restarted.List()[0].ID; got != first["/docs/a.clay"] {
			t.Fatalf("reopening made a new id %q for a row whose id was %q", got, first["/docs/a.clay"])
		}
	})

	t.Run("cap", func(t *testing.T) {
		dir := t.TempDir()
		store := m3Open(t, dir)
		for i := 0; i < 205; i++ {
			m3Touch(t, store, recents.Entry{Path: filepath.Join("/docs", m3Number(i)+".clay")}, at.Add(time.Duration(i)*time.Minute))
		}
		list := store.List()
		if len(list) != 200 {
			t.Fatalf("the record holds %d rows, want 200", len(list))
		}
		if got, want := list[0].Path, filepath.Join("/docs", "204.clay"); got != want {
			t.Fatalf("the newest row is %q, want %s", got, want)
		}
		if got, want := list[len(list)-1].Path, filepath.Join("/docs", "5.clay"); got != want {
			t.Fatalf("the oldest row is %q, want %s", got, want)
		}
		if got := len(m3Open(t, dir).List()); got != 200 {
			t.Fatalf("the record on disk holds %d rows, want 200", got)
		}
	})

	t.Run("a failed write leaves the old file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("a read-only folder does not stop root")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "recents.json")
		store := m3Open(t, dir)
		m3Touch(t, store, recents.Entry{Path: "/docs/a.clay"}, at)

		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the record: %v", err)
		}

		allowWrites := testutil.RefuseReplace(t, path)

		if err := store.Touch(recents.Entry{Path: "/docs/b.clay"}, at.Add(time.Minute)); err == nil {
			t.Fatal("Touch succeeded while the record could not be written")
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the record after the failed write: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("the failed write changed the file:\nbefore %s\nafter  %s", before, after)
		}

		got := m3Paths(store.List())
		if len(got) != 1 || got[0] != "/docs/a.clay" {
			t.Fatalf("the list after the failed write is %v, want only /docs/a.clay", got)
		}

		allowWrites()
		if got := m3Paths(m3Open(t, dir).List()); len(got) != 1 || got[0] != "/docs/a.clay" {
			t.Fatalf("the record on disk after the failed write is %v, want only /docs/a.clay", got)
		}
	})

	t.Run("a bad file is moved aside and the record starts empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "recents.json")
		if err := os.WriteFile(path, []byte("{ this is not json"), 0600); err != nil {
			t.Fatalf("write the bad record: %v", err)
		}

		store := m3Open(t, dir)
		if got := len(store.List()); got != 0 {
			t.Fatalf("a bad record left %d rows, want none", got)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("the bad record is still at %s", path)
		}
		aside, err := filepath.Glob(path + ".corrupt-*")
		if err != nil {
			t.Fatalf("look for the set-aside record: %v", err)
		}
		if len(aside) != 1 {
			t.Fatalf("found %d set-aside records, want one", len(aside))
		}
		data, err := os.ReadFile(aside[0])
		if err != nil {
			t.Fatalf("read the set-aside record: %v", err)
		}
		if string(data) != "{ this is not json" {
			t.Fatalf("the set-aside record changed: %q", data)
		}

		m3Touch(t, store, recents.Entry{Path: "/docs/a.clay"}, at)
		if got := m3Paths(m3Open(t, dir).List()); len(got) != 1 || got[0] != "/docs/a.clay" {
			t.Fatalf("the record after a bad file is %v, want only /docs/a.clay", got)
		}
	})

	t.Run("forget removes the row and touches nothing else", func(t *testing.T) {
		dir := t.TempDir()
		document := filepath.Join(t.TempDir(), "a.clay")
		if err := os.WriteFile(document, []byte("the person's document"), 0600); err != nil {
			t.Fatalf("write the document: %v", err)
		}

		store := m3Open(t, dir)
		m3Touch(t, store, recents.Entry{Path: document, Identity: "id-1", How: recents.HowFile}, at)
		m3Touch(t, store, recents.Entry{Path: "/docs/b.clay"}, at)

		gone, ok := store.Find(store.List()[1].ID)
		if !ok {
			t.Fatal("could not find the row to forget")
		}
		if err := store.Forget(gone.ID); err != nil {
			t.Fatalf("Forget(%q): %v", gone.ID, err)
		}
		if _, ok := store.Find(gone.ID); ok {
			t.Fatal("the forgotten row is still in the record")
		}
		if got := m3Paths(store.List()); len(got) != 1 || got[0] != "/docs/b.clay" {
			t.Fatalf("the record after forgetting is %v, want only /docs/b.clay", got)
		}

		if err := store.Forget("0000000000000000"); err != nil {
			t.Fatalf("Forget of an unknown id: %v", err)
		}

		data, err := os.ReadFile(document)
		if err != nil {
			t.Fatalf("read the document after forget: %v", err)
		}
		if string(data) != "the person's document" {
			t.Fatalf("forget changed the document: %q", data)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read the config folder: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "recents.json" {
			names := make([]string, len(entries))
			for i, entry := range entries {
				names[i] = entry.Name()
			}
			t.Fatalf("the config folder holds %v after forget, want only recents.json", names)
		}
	})

	t.Run("find and rebind follow a moved folder", func(t *testing.T) {
		dir := t.TempDir()
		store := m3Open(t, dir)
		native := filepath.FromSlash
		m3Touch(t, store, recents.Entry{Path: native("/work/site/a.clay")}, at)
		m3Touch(t, store, recents.Entry{Path: native("/work/site/pages/b.clay")}, at)
		m3Touch(t, store, recents.Entry{Path: native("/work/elsewhere.clay")}, at)

		entry, ok := store.Find(store.List()[2].ID)
		if !ok || entry.Path != native("/work/site/a.clay") {
			t.Fatalf("Find gave %+v, %v", entry, ok)
		}

		if err := store.Rebind(native("/work/site"), native("/work/moved")); err != nil {
			t.Fatalf("Rebind: %v", err)
		}
		got := m3Paths(store.List())
		want := []string{native("/work/elsewhere.clay"), native("/work/moved/pages/b.clay"), native("/work/moved/a.clay")}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("after the move the record is %v, want %v", got, want)
		}
		if entry, ok := store.Find(store.List()[2].ID); !ok || entry.Path != native("/work/moved/a.clay") {
			t.Fatalf("the moved row's id no longer finds it: %+v, %v", entry, ok)
		}
		if got := m3Paths(m3Open(t, dir).List()); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("the record on disk after the move is %v, want %v", got, want)
		}
	})
}

// m3Number spells i without strconv, to keep the test's imports about the record.
func m3Number(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
