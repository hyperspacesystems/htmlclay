package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
)

type folderAnswer struct {
	folder     string
	remembered bool
	err        error
}

func newAIToolsFolderApp(t *testing.T) (*app, string) {
	t.Helper()
	home := real(t, t.TempDir())
	a := newShellTestApp(t, home)
	t.Cleanup(func() {
		for _, p := range a.registry().Pending() {
			a.registry().Close(p.ID)
		}
		a.stopRequests()
	})
	doc := filepath.Join(home, "docs", "app.html")
	writeTestFile(t, doc, `<!doctype html><html><head></head><body><p>App</p></body></html>`)
	return a, doc
}

func setAIToolsFolder(t *testing.T, a *app, doc, folder, level string) {
	t.Helper()
	s := a.aiToolsService()
	if s == nil {
		t.Fatal("no ai-tools service")
	}
	if err := s.Store().SetFolder(aitools.FolderRecord{
		Document: closedPath(doc),
		Folder:   folder,
		Identity: platform.DirIdentity(folder),
		Level:    level,
		At:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func startFolder(a *app, doc, level string, choose bool) <-chan folderAnswer {
	out := make(chan folderAnswer, 1)
	go func() {
		folder, remembered, err := a.aiToolsHost().Folder(context.Background(), doc, choose, level, func() {})
		out <- folderAnswer{folder, remembered, err}
	}()
	return out
}

func readFolder(t *testing.T, out <-chan folderAnswer) folderAnswer {
	t.Helper()
	select {
	case got := <-out:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("the folder question never returned")
		return folderAnswer{}
	}
}

func TestAIToolsFolderLevelDecidesWhetherARememberedFolderIsReused(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	folder := real(t, t.TempDir())

	// A folder remembered for read is not reused for change: the person is asked
	// again, with the change warning.
	setAIToolsFolder(t, a, doc, folder, aitools.LevelRead)
	out := startFolder(a, doc, aitools.LevelChange, false)
	req := waitForSetupRequest(t, a, appcore.RequestAITools)
	if req.Detail != aitools.LevelChange {
		t.Fatalf("change question detail = %q, want %q", req.Detail, aitools.LevelChange)
	}
	if err := answerSetup(t, a, "answer-aitools-change", req, "choose-folder", folder); err != nil {
		t.Fatal(err)
	}
	got := readFolder(t, out)
	if got.err != nil || got.folder != folder || !got.remembered {
		t.Fatalf("change folder = (%q, %v, %v), want (%q, true, nil)", got.folder, got.remembered, got.err, folder)
	}
	rec, ok := a.aiToolsService().Store().Folder(closedPath(doc))
	if !ok || rec.Level != aitools.LevelChange {
		t.Fatalf("record after a change-level choice = %+v, %v; want the level recorded as change", rec, ok)
	}

	// A folder remembered for change is reused for both levels, with no question.
	setAIToolsFolder(t, a, doc, folder, aitools.LevelChange)
	for _, level := range []string{aitools.LevelRead, aitools.LevelChange} {
		asked := false
		gotFolder, remembered, err := a.aiToolsHost().Folder(context.Background(), doc, false, level, func() { asked = true })
		if err != nil || !remembered || gotFolder != folder || asked {
			t.Fatalf("%s folder = (%q, %v, %v, asked=%v), want (%q, true, nil, false)", level, gotFolder, remembered, err, asked, folder)
		}
	}
	if len(a.snapshot().Requests) != 0 {
		t.Fatalf("the reused folder raised %d questions", len(a.snapshot().Requests))
	}
}

func TestAIToolsRememberedFolderMustBeTheSameFolder(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	parent := real(t, t.TempDir())

	// A folder whose path is now a symlink to another folder is not reused.
	linked := filepath.Join(parent, "linked")
	if err := os.Mkdir(linked, 0o755); err != nil {
		t.Fatal(err)
	}
	setAIToolsFolder(t, a, doc, linked, aitools.LevelRead)
	other := filepath.Join(parent, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(linked); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, linked); err != nil {
		t.Fatal(err)
	}
	got := readFolder(t, startFolder(a, doc, aitools.LevelRead, false))
	var e *aitools.Error
	if !errors.As(got.err, &e) || e.Code != aitools.CodeFolderMissing || got.remembered {
		t.Fatalf("symlinked folder = (%q, %v, %v), want folder_missing and not remembered", got.folder, got.remembered, got.err)
	}

	// Another folder moved in at the same path is a different folder too. It
	// exists alongside the first until the move, so it cannot reuse its inode,
	// as a folder deleted and made again can on Linux.
	replaced := filepath.Join(parent, "replaced")
	if err := os.Mkdir(replaced, 0o755); err != nil {
		t.Fatal(err)
	}
	newcomer := filepath.Join(parent, "newcomer")
	if err := os.Mkdir(newcomer, 0o755); err != nil {
		t.Fatal(err)
	}
	setAIToolsFolder(t, a, doc, replaced, aitools.LevelRead)
	if err := os.RemoveAll(replaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newcomer, replaced); err != nil {
		t.Fatal(err)
	}
	got = readFolder(t, startFolder(a, doc, aitools.LevelRead, false))
	if !errors.As(got.err, &e) || e.Code != aitools.CodeFolderMissing || got.remembered {
		rec, _ := a.aiToolsService().Store().Folder(closedPath(doc))
		t.Fatalf("recreated folder = (%q, %v, %v), want folder_missing and not remembered (pinned %q, now %q)", got.folder, got.remembered, got.err, rec.Identity, platform.DirIdentity(replaced))
	}

	// Choosing the folder again asks and remembers it.
	out := startFolder(a, doc, aitools.LevelRead, true)
	req := waitForSetupRequest(t, a, appcore.RequestAITools)
	if err := answerSetup(t, a, "answer-aitools-again", req, "choose-folder", replaced); err != nil {
		t.Fatal(err)
	}
	got = readFolder(t, out)
	if got.err != nil || !got.remembered || got.folder != replaced {
		t.Fatalf("chosen folder = (%q, %v, %v), want (%q, true, nil)", got.folder, got.remembered, got.err, replaced)
	}
}

func TestAIToolsFolderChosenThroughALinkIsRememberedAndReused(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	parent := real(t, t.TempDir())
	target := filepath.Join(parent, "project")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "shortcut")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	out := startFolder(a, doc, aitools.LevelRead, false)
	req := waitForSetupRequest(t, a, appcore.RequestAITools)
	if err := answerSetup(t, a, "answer-aitools-link", req, "choose-folder", link); err != nil {
		t.Fatal(err)
	}
	if got := readFolder(t, out); got.err != nil || got.folder != target {
		t.Fatalf("chosen through a link = (%q, %v), want the folder itself %q", got.folder, got.err, target)
	}
	gotFolder, remembered, err := a.aiToolsHost().Folder(context.Background(), doc, false, aitools.LevelRead, func() { t.Error("asked again for a folder chosen through a link") })
	if err != nil || !remembered || gotFolder != target {
		t.Fatalf("second request = (%q, %v, %v), want the remembered folder %q", gotFolder, remembered, err, target)
	}
}
