//go:build managedtrial

package managedtrial

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRestructureLocksEveryPath proves a structure change waits for a write on
// every path it names: a save holding the destination's lock delays a move onto
// it, and the move is judged against the state that save left behind.
func TestRestructureLocksEveryPath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := []byte("<!doctype html>\n<html><body><h1>One</h1></body></html>\n")
	writeCommitFile(t, root, "a.html", first)

	previousClock := versionClock
	versionClock = func() int64 { return 1762000000000 }
	t.Cleanup(func() { versionClock = previousClock })

	locked := make(chan struct{})
	release := make(chan struct{})
	hooks := &blockingCommitHooks{after: func() {
		close(locked)
		<-release
	}}
	host := newCommitHost(root, "root-structure-lock", newConsentRegistry(root, nil), hooks, nil)

	saved := make(chan commitResult, 1)
	go func() {
		result, _ := host.commit(&commitRequest{
			Kind:  "browser-save",
			Name:  "b.html",
			Bytes: []byte("<!doctype html>\n<html><body><h1>Two</h1></body></html>\n"),
			Text:  true,
		})
		saved <- result
	}()
	select {
	case <-locked:
	case <-time.After(10 * time.Second):
		t.Fatal("the save never reached the document's lock")
	}

	moved := make(chan structureResult, 1)
	go func() {
		result, _ := host.restructure("move", []structureEntry{{
			FromPath: filepath.Join(root, "a.html"),
			ToPath:   filepath.Join(root, "b.html"),
		}})
		moved <- result
	}()

	select {
	case result := <-moved:
		t.Fatalf("the move did not wait for the save on its destination: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case result := <-saved:
		if result.Outcome != "committed" {
			t.Fatalf("the save got %q, want committed", result.Outcome)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the save never finished")
	}
	var result structureResult
	select {
	case result = <-moved:
	case <-time.After(10 * time.Second):
		t.Fatal("the move never finished")
	}
	if result.Outcome != "committed" || result.Completed != 1 {
		t.Fatalf("the move got %+v, want committed after the save", result)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.html")); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(data, first) {
		t.Errorf("the move did not replace the saved file: %q", data)
	}
}

// TestStructureGateWaitsForADescendantWrite proves a saved file anywhere under
// the tree a structure change moves blocks that change: the change takes the
// whole gate, so no write can run alongside it and recreate the folder it
// renames away.
func TestStructureGateWaitsForADescendantWrite(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	previousClock := versionClock
	versionClock = func() int64 { return 1762000000000 }
	t.Cleanup(func() { versionClock = previousClock })

	locked := make(chan struct{})
	release := make(chan struct{})
	hooks := &blockingCommitHooks{after: func() {
		close(locked)
		<-release
	}}
	host := newCommitHost(root, "root-structure-gate", newConsentRegistry(root, nil), hooks, nil)

	saved := []byte("<!doctype html>\n<html><body><h1>Nested</h1></body></html>\n")
	savedDone := make(chan commitResult, 1)
	go func() {
		result, _ := host.commit(&commitRequest{
			Kind:  "browser-save",
			Name:  "a/b.html",
			Bytes: saved,
			Text:  true,
		})
		savedDone <- result
	}()
	select {
	case <-locked:
	case <-time.After(10 * time.Second):
		t.Fatal("the save never reached the document's lock")
	}

	moved := make(chan structureResult, 1)
	go func() {
		result, _ := host.restructure("move", []structureEntry{{
			FromPath: filepath.Join(root, "a"),
			ToPath:   filepath.Join(root, "c"),
			IsDir:    true,
		}})
		moved <- result
	}()

	select {
	case result := <-moved:
		t.Fatalf("the move did not wait for the save inside the folder it moved: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case result := <-savedDone:
		if result.Outcome != "committed" {
			t.Fatalf("the save got %q, want committed", result.Outcome)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the save never finished")
	}
	var result structureResult
	select {
	case result = <-moved:
	case <-time.After(10 * time.Second):
		t.Fatal("the move never finished")
	}
	if result.Outcome != "committed" || result.Completed != 1 {
		t.Fatalf("the move got %+v, want committed after the save", result)
	}
	if data, err := os.ReadFile(filepath.Join(root, "c", "b.html")); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(data, saved) {
		t.Errorf("the move did not carry the saved file: %q", data)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Errorf("the move left the folder it renamed behind: %v", err)
	}
}

// TestRestructureCarriesPageState proves a step's blockeditor-state move or drop runs
// for every completed step, the folder form for a directory, and never for a
// step that did not complete.
func TestRestructureCarriesPageState(t *testing.T) {
	newRoot := func(t *testing.T) (string, *commitHost) {
		t.Helper()
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return root, newCommitHost(root, "root-page-state", newConsentRegistry(root, nil), newGuardHooks(nil), nil)
	}

	t.Run("move", func(t *testing.T) {
		root, host := newRoot(t)
		writeCommitFile(t, root, "p.html", []byte("<!doctype html>\n<html><body><h1>P</h1></body></html>\n"))
		if err := writePageState(root, "p.html", &PageRecord{V: 1, Rev: "abc", Tag: "tag"}, false); err != nil {
			t.Fatal(err)
		}
		result, err := host.restructure("move", []structureEntry{{
			FromPath:  filepath.Join(root, "p.html"),
			ToPath:    filepath.Join(root, "q.html"),
			FromName:  "p.html",
			ToName:    "q.html",
			PageState: "move",
		}})
		if err != nil || result.Outcome != "committed" || result.PageStateErr != nil {
			t.Fatalf("the move got %+v / %v, want committed with its page state", result, err)
		}
		record, err := readPageState(root, "q.html", PageStateReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if record == nil || record.Rev != "abc" {
			t.Fatalf("q.html reads back %+v, want the moved record", record)
		}
		if record, err := readPageState(root, "p.html", PageStateReadOptions{}); err != nil {
			t.Fatal(err)
		} else if record != nil {
			t.Fatalf("p.html still reads back %+v, want nothing", record)
		}
	})

	t.Run("move-dir", func(t *testing.T) {
		root, host := newRoot(t)
		writeCommitFile(t, root, filepath.Join("d", "x.html"), []byte("<!doctype html>\n<html><body><h1>X</h1></body></html>\n"))
		if err := writePageState(root, "d/x.html", &PageRecord{V: 1, Rev: "abc", Tag: "tag"}, false); err != nil {
			t.Fatal(err)
		}
		result, err := host.restructure("move", []structureEntry{{
			FromPath:  filepath.Join(root, "d"),
			ToPath:    filepath.Join(root, "e"),
			IsDir:     true,
			FromName:  "d",
			ToName:    "e",
			PageState: "move",
		}})
		if err != nil || result.Outcome != "committed" || result.PageStateErr != nil {
			t.Fatalf("the folder move got %+v / %v, want committed with its page state", result, err)
		}
		record, err := readPageState(root, "e/x.html", PageStateReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if record == nil || record.Rev != "abc" {
			t.Fatalf("e/x.html reads back %+v, want the moved record", record)
		}
	})

	t.Run("trash-drop", func(t *testing.T) {
		root, host := newRoot(t)
		writeCommitFile(t, root, "p.html", []byte("<!doctype html>\n<html><body><h1>P</h1></body></html>\n"))
		if err := writePageState(root, "p.html", &PageRecord{V: 1, Rev: "abc", Tag: "tag"}, false); err != nil {
			t.Fatal(err)
		}
		result, err := host.restructure("trash", []structureEntry{{
			FromPath:  filepath.Join(root, "p.html"),
			ToPath:    filepath.Join(root, ".trash", "p.html"),
			FromName:  "p.html",
			ToName:    ".trash/p.html",
			PageState: "drop",
		}})
		if err != nil || result.Outcome != "committed" || result.PageStateErr != nil {
			t.Fatalf("the trash got %+v / %v, want committed with its page state dropped", result, err)
		}
		target, err := pageStatePath(root, "p.html")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("the trash left the sidecar behind: %v", err)
		}
	})

	t.Run("failed-step", func(t *testing.T) {
		root, host := newRoot(t)
		if err := writePageState(root, "q.html", &PageRecord{V: 1, Rev: "kept", Tag: "tag"}, false); err != nil {
			t.Fatal(err)
		}
		result, err := host.restructure("move", []structureEntry{{
			FromPath:  filepath.Join(root, "missing.html"),
			ToPath:    filepath.Join(root, "q.html"),
			FromName:  "missing.html",
			ToName:    "q.html",
			PageState: "move",
		}})
		if err != nil || result.Outcome != "failed" || result.Completed != 0 {
			t.Fatalf("the move got %+v / %v, want failed with nothing completed", result, err)
		}
		if result.PageStateErr != nil {
			t.Fatalf("the failed move touched page state: %v", result.PageStateErr)
		}
		record, err := readPageState(root, "q.html", PageStateReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if record == nil || record.Rev != "kept" {
			t.Fatalf("q.html reads back %+v, want the untouched record", record)
		}
	})
}

// TestRestructureFolderAndEmptyTrash proves the two workspace kinds: folder
// makes one directory per step and never its parents, and empty-trash clears
// the root's `.trash/` while keeping the folder.
func TestRestructureFolderAndEmptyTrash(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host := newCommitHost(root, "root-workspace-kinds", newConsentRegistry(root, nil), newGuardHooks(nil), nil)

	result, err := host.restructure("folder", []structureEntry{{ToPath: filepath.Join(root, "new"), ToName: "new"}})
	if err != nil || result.Outcome != "committed" || result.Completed != 1 {
		t.Fatalf("the folder got %+v / %v, want committed", result, err)
	}
	if info, err := os.Stat(filepath.Join(root, "new")); err != nil || !info.IsDir() {
		t.Fatalf("the folder made no directory: %v", err)
	}
	result, err = host.restructure("folder", []structureEntry{{ToPath: filepath.Join(root, "new"), ToName: "new"}})
	if err != nil || result.Outcome != "failed" || result.Reason != "EEXIST" {
		t.Fatalf("the second folder got %+v / %v, want failed/EEXIST", result, err)
	}
	result, err = host.restructure("folder", []structureEntry{{ToPath: filepath.Join(root, "missing", "new"), ToName: "missing/new"}})
	if err != nil || result.Outcome != "failed" || result.Reason != "ENOENT" {
		t.Fatalf("the nested folder got %+v / %v, want failed/ENOENT", result, err)
	}

	writeCommitFile(t, root, filepath.Join(".trash", "a.html"), []byte("<!doctype html>\n"))
	writeCommitFile(t, root, filepath.Join(".trash", "dir", "b.html"), []byte("<!doctype html>\n"))
	result, err = host.restructure("empty-trash", nil)
	if err != nil || result.Outcome != "committed" || result.Completed != 2 {
		t.Fatalf("empty-trash got %+v / %v, want committed with two entries", result, err)
	}
	if info, err := os.Stat(filepath.Join(root, ".trash")); err != nil || !info.IsDir() {
		t.Fatalf("empty-trash removed the trash folder itself: %v", err)
	}
	if names, err := os.ReadDir(filepath.Join(root, ".trash")); err != nil {
		t.Fatal(err)
	} else if len(names) != 0 {
		t.Fatalf("empty-trash left %d entries behind", len(names))
	}

	empty, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	emptyHost := newCommitHost(empty, "root-no-trash", newConsentRegistry(empty, nil), newGuardHooks(nil), nil)
	result, err = emptyHost.restructure("empty-trash", nil)
	if err != nil || result.Outcome != "committed" || result.Completed != 0 {
		t.Fatalf("empty-trash without a trash got %+v / %v, want committed with nothing", result, err)
	}

	fileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, fileRoot, ".trash", []byte("not a folder\n"))
	fileHost := newCommitHost(fileRoot, "root-file-trash", newConsentRegistry(fileRoot, nil), newGuardHooks(nil), nil)
	result, err = fileHost.restructure("empty-trash", nil)
	if err != nil || result.Outcome != "failed" || result.Reason != "ENOTDIR" {
		t.Fatalf("empty-trash over a file got %+v / %v, want failed/ENOTDIR", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(fileRoot, ".trash")); err != nil || string(data) != "not a folder\n" {
		t.Fatalf("empty-trash changed a .trash that is a file: %q / %v", data, err)
	}
}
