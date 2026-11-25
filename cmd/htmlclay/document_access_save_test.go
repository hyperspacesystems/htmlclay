package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/specwire"
)

// The page fixture is the served save golden's: an article at one revision and
// the page state stored beside the file for that revision.
const (
	documentAccessPageRev = "093360993130d818"
	documentAccessPageTag = `<script type="application/json" merge="blockeditor" clay="live">{"v":1,"g":0,"y":{"seed":"AAACQQMEAQAIQgALJwAoAQUAJwAoAIUkHGF0dHJzdGl0bGVsZXZlbHRmbG93YXR0cnNwMXRFAQEEBQIBBwEAAAEBAQACQQACQQEBBwB9AXcHaGVhZGluZ3YBAWJ3BXRpdGxldwlwYXJhZ3JhcGh2AQFidwJwMQA="}}</script>`
	documentAccessPage    = "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>Doc</title>\n</head>\n<body>\n" +
		`<article class="blockeditor" clay="freeze" data-blockeditor="1" data-rev="` + documentAccessPageRev + `"><h1 data-id="title">Title</h1>` + "\n" +
		`<p data-id="p1">Body</p></article>` + "\n</body>\n</html>\n"
)

func fileBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// savedThroughLane edits one document through the one lane and answers the
// bytes it wrote and what the lane answered.
func savedThroughLane(t *testing.T, a *app, target documentAccessTarget, marker string) (string, Result) {
	t.Helper()
	html, etag := readDocumentAccess(t, a, target.path)
	incoming := strings.Replace(string(html), "</body>", "<p>"+marker+"</p></body>", 1)
	result, err := documentAccessOf(t, a, target.path).Save(context.Background(), Save{HTML: []byte(incoming), IfMatch: etag, SaveID: "save-" + marker})
	if err != nil {
		t.Fatalf("%s: Save: %v", target.name, err)
	}
	return incoming, result
}

func versionThroughLane(t *testing.T, a *app, target documentAccessTarget, id string) string {
	t.Helper()
	data, err := documentAccessOf(t, a, target.path).Version(context.Background(), id)
	if err != nil {
		t.Fatalf("%s: read version %q: %v", target.name, id, err)
	}
	return string(data)
}

// TestM3DocumentAccessSave is step 7.40 where the two owners meet: the same
// save and restore for a served document, a document in a live trusted folder
// and an open loose file.
func TestM3DocumentAccessSave(t *testing.T) {
	t.Run("a save writes the document and names the version it replaced", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		for _, target := range fx.targets() {
			original := fileBytes(t, target.path)
			incoming, result := savedThroughLane(t, fx.app, target, "one")
			if got := fileBytes(t, target.path); got != incoming {
				t.Fatalf("%s: the file holds %q, want the saved bytes", target.name, got)
			}
			if !result.Changed || result.ETag != specwire.Etag([]byte(incoming)) {
				t.Fatalf("%s: result = %+v, want changed and the stamp of the saved bytes", target.name, result)
			}
			if _, etag := readDocumentAccess(t, fx.app, target.path); etag != result.ETag {
				t.Fatalf("%s: the next read stamps %q, and the save answered %q", target.name, etag, result.ETag)
			}
			if result.UndoVersion == "" {
				t.Fatalf("%s: the save named no undo version", target.name)
			}
			if got := versionThroughLane(t, fx.app, target, result.UndoVersion); got != original {
				t.Fatalf("%s: undo version %s holds %q, want the bytes the save replaced", target.name, result.UndoVersion, got)
			}
			page, _, err := documentAccessOf(t, fx.app, target.path).Versions(context.Background(), "", historyPageSize)
			if err != nil {
				t.Fatalf("%s: read its versions: %v", target.name, err)
			}
			if !slices.ContainsFunc(page, func(v Version) bool { return v.ID == result.UndoVersion }) {
				t.Fatalf("%s: the versions list %+v does not name the undo version %s", target.name, page, result.UndoVersion)
			}
		}
	})

	t.Run("a stale stamp is ErrStale and writes nothing", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		for _, target := range fx.targets() {
			_, old := readDocumentAccess(t, fx.app, target.path)
			current, saved := savedThroughLane(t, fx.app, target, "one")

			late := strings.Replace(current, "</body>", "<p>late</p></body>", 1)
			_, err := documentAccessOf(t, fx.app, target.path).Save(context.Background(), Save{HTML: []byte(late), IfMatch: old, SaveID: "save-late"})
			if got := fileBytes(t, target.path); got != current {
				t.Fatalf("%s: the stale save changed the file: %q", target.name, got)
			}
			var stale ErrStale
			if !errors.As(err, &stale) {
				t.Fatalf("%s: a save with the stamp of an older version = %v, want ErrStale", target.name, err)
			}
			if stale.ETag != saved.ETag || stale.SaveID != "save-one" {
				t.Fatalf("%s: ErrStale = %+v, want the document's stamp %q and the id of the save that stored it", target.name, stale, saved.ETag)
			}
			// The page's own save lane gives the same verdict on the same stamp.
			if code := saveWithStamp(t, fx.app, target.path, late, old); code != http.StatusPreconditionFailed {
				t.Fatalf("%s: a page's save with the stamp of an older version = %d, want 412", target.name, code)
			}
			if code := saveWithStamp(t, fx.app, target.path, late, saved.ETag); code != http.StatusOK {
				t.Fatalf("%s: a page's save with the stamp the lane's save answered = %d, want 200", target.name, code)
			}
		}
	})

	t.Run("a restore puts a version back and keeps the bytes it replaced", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		for _, target := range fx.targets() {
			original := fileBytes(t, target.path)
			_, old := readDocumentAccess(t, fx.app, target.path)
			incoming, saved := savedThroughLane(t, fx.app, target, "one")
			access := documentAccessOf(t, fx.app, target.path)

			_, err := access.Restore(context.Background(), saved.UndoVersion, old, "")
			var stale ErrStale
			if !errors.As(err, &stale) || stale.ETag != saved.ETag {
				t.Fatalf("%s: a restore with the stamp of an older version = %v, want ErrStale carrying %q", target.name, err, saved.ETag)
			}
			if got := fileBytes(t, target.path); got != incoming {
				t.Fatalf("%s: the stale restore changed the file: %q", target.name, got)
			}

			restored, err := access.Restore(context.Background(), saved.UndoVersion, saved.ETag, "restore-one")
			if err != nil {
				t.Fatalf("%s: Restore: %v", target.name, err)
			}
			if got := fileBytes(t, target.path); got != original {
				t.Fatalf("%s: the file after the restore holds %q, want the version's bytes", target.name, got)
			}
			if !restored.Changed || restored.ETag != specwire.Etag([]byte(original)) {
				t.Fatalf("%s: restored = %+v, want changed and the stamp of the restored bytes", target.name, restored)
			}
			if restored.UndoVersion == "" {
				t.Fatalf("%s: the restore named no undo version", target.name)
			}
			if got := versionThroughLane(t, fx.app, target, restored.UndoVersion); got != incoming {
				t.Fatalf("%s: undo version %s holds %q, want the bytes the restore replaced", target.name, restored.UndoVersion, got)
			}
		}
	})

	t.Run("a version that cannot be kept is ErrBackup and the bytes are unchanged", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root writes into a read-only history folder")
		}
		if runtime.GOOS == "windows" {
			t.Skip("Windows ignores a folder's read-only mode")
		}
		fx := newDocumentAccessFixture(t)
		readOnly := func(dir string) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		}
		readOnly(servedHistoryDir(fx.served, "index.html", "versions"))
		readOnly(fx.app.rt.versions.BaseDir())

		for _, target := range fx.targets() {
			original := fileBytes(t, target.path)
			html, etag := readDocumentAccess(t, fx.app, target.path)
			incoming := strings.Replace(string(html), "</body>", "<p>never written</p></body>", 1)
			result, err := documentAccessOf(t, fx.app, target.path).Save(context.Background(), Save{HTML: []byte(incoming), IfMatch: etag})
			if !errors.Is(err, ErrBackup) {
				t.Fatalf("%s: a save whose replaced version cannot be stored = %+v, %v, want ErrBackup", target.name, result, err)
			}
			if got := fileBytes(t, target.path); got != original {
				t.Fatalf("%s: the refused save changed the file: %q", target.name, got)
			}
		}
	})

	t.Run("a fragment and an oversized body are refused", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		oversized := []byte(strings.Replace(servedTestDocument, "first", strings.Repeat("x", 51<<20), 1))
		for _, target := range fx.targets() {
			original := fileBytes(t, target.path)
			_, etag := readDocumentAccess(t, fx.app, target.path)
			access := documentAccessOf(t, fx.app, target.path)
			if _, err := access.Save(context.Background(), Save{HTML: []byte("<p>a fragment</p>"), IfMatch: etag}); !errors.Is(err, ErrNotDocument) {
				t.Fatalf("%s: a save of a fragment = %v, want ErrNotDocument", target.name, err)
			}
			if _, err := access.Save(context.Background(), Save{HTML: oversized, IfMatch: etag}); !errors.Is(err, ErrTooLarge) {
				t.Fatalf("%s: a save over the page's limit = %v, want ErrTooLarge", target.name, err)
			}
			if got := fileBytes(t, target.path); got != original {
				t.Fatalf("%s: a refused save changed the file: %q", target.name, got)
			}
		}
	})

	t.Run("a save registers a trusted folder's document in the folder's own site", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		path := filepath.Join(fx.trusted, "page.html")
		readDocumentAccess(t, fx.app, path)
		if _, registered := siteOf(fx.app, path); registered {
			t.Fatal("the read registered the document")
		}
		savedThroughLane(t, fx.app, documentAccessTarget{"trusted", path}, "one")
		registered, ok := siteOf(fx.app, path)
		if !ok {
			t.Fatal("the save left the document unregistered, so it took no file lock")
		}
		if want := liveSiteAt(t, fx.app, fx.trusted); registered != want {
			t.Fatal("the save registered the document in another site than its folder's own")
		}
	})

	t.Run("an owner that changed since the document was reached is not written to", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		servedPath := filepath.Join(fx.served, "index.html")
		trustedPath := filepath.Join(fx.trusted, "page.html")
		servedOwner, err := fx.app.ResolveDocument(servedPath)
		if err != nil {
			t.Fatal(err)
		}
		changes := map[string]func(){
			servedPath: func() {
				if err := fx.app.stopServing(servedOwner.FolderID); err != nil {
					t.Fatalf("stopServing: %v", err)
				}
			},
			trustedPath: func() {
				if err := fx.app.untrustFolder(fx.trusted); err != nil {
					t.Fatalf("untrustFolder: %v", err)
				}
			},
			fx.loose: func() {
				if _, refusal, err := fx.app.serveFolder(filepath.Dir(fx.loose), "docs"); err != nil || refusal != nil {
					t.Fatalf("serveFolder: %v, %+v", err, refusal)
				}
			},
		}
		for _, target := range fx.targets() {
			original := fileBytes(t, target.path)
			html, etag := readDocumentAccess(t, fx.app, target.path)
			access := documentAccessOf(t, fx.app, target.path)
			changes[target.path]()

			incoming := strings.Replace(string(html), "</body>", "<p>never written</p></body>", 1)
			if _, err := access.Save(context.Background(), Save{HTML: []byte(incoming), IfMatch: etag}); !errors.Is(err, ErrNotReachable) {
				t.Fatalf("%s: a save through the owner it had before = %v, want ErrNotReachable", target.name, err)
			}
			if _, err := access.Restore(context.Background(), "2026-03-04-05-06-07-890Z.html", etag, ""); !errors.Is(err, ErrNotReachable) {
				t.Fatalf("%s: a restore through the owner it had before = %v, want ErrNotReachable", target.name, err)
			}
			if got := fileBytes(t, target.path); got != original {
				t.Fatalf("%s: the refused write changed the file: %q", target.name, got)
			}
		}
		// The loose file's folder is served now, and the lane reaches it there.
		incoming, saved := savedThroughLane(t, fx.app, documentAccessTarget{"newly served", fx.loose}, "served")
		if _, ok := documentAccessOf(t, fx.app, fx.loose).(servedDocumentAccess); !ok {
			t.Fatal("a file in a folder that is served now is not reached through the served owner")
		}
		if got := fileBytes(t, fx.loose); got != incoming || !saved.Changed {
			t.Fatalf("the save through the new owner left %q, %+v", got, saved)
		}
	})

	t.Run("a served page with stored page state saves with the stamp it was read with", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		path := filepath.Join(fx.served, "paged.html")
		writeTestFile(t, path, documentAccessPage)
		tag, err := json.Marshal(documentAccessPageTag)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(fx.served, ".hyperclay", "blockeditor-state", "paged.html.json"),
			`{"v":1,"rev":"`+documentAccessPageRev+`","tag":`+string(tag)+`}`)
		target := documentAccessTarget{"served page", path}

		html, etag := readDocumentAccess(t, fx.app, path)
		if string(html) != documentAccessPage {
			t.Fatalf("the read answered %q, want the file's own bytes", html)
		}
		incoming := strings.Replace(documentAccessPage, "<title>Doc</title>", "<title>Doc through the lane</title>", 1)
		saved, err := documentAccessOf(t, fx.app, path).Save(context.Background(), Save{HTML: []byte(incoming), IfMatch: etag})
		if err != nil {
			t.Fatalf("a save naming the stamp the read answered = %v, want it written", err)
		}
		if etag == specwire.Etag(html) {
			t.Fatal("the page's stored state was no part of its stamp, so this round trip proved nothing about it")
		}
		if got := fileBytes(t, path); got != incoming {
			t.Fatalf("the file holds %q, want the saved page", got)
		}
		if got := versionThroughLane(t, fx.app, target, saved.UndoVersion); got != documentAccessPage {
			t.Fatalf("undo version %s holds %q, want the page the save replaced", saved.UndoVersion, got)
		}
		_, next := readDocumentAccess(t, fx.app, path)
		if next != saved.ETag {
			t.Fatalf("the next read stamps %q, and the save answered %q", next, saved.ETag)
		}
		again := strings.Replace(incoming, "through the lane", "through the page", 1)
		if code := saveWithStamp(t, fx.app, path, again, next); code != http.StatusOK {
			t.Fatalf("a page's save naming the stamp the read answered = %d, want 200", code)
		}
	})
}
