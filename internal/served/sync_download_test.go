package served

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// breakVersions puts a plain file where the versions folder goes, so no version
// can be kept. A file, not a read-only folder: a box running as root writes
// through folder modes.
func breakVersions(t *testing.T, baseDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(baseDir, ".hyperclay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, filepath.FromSlash(versionsDirName)), []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestM4SyncDownloadKeepsReplaced(t *testing.T) {
	const old = "<!doctype html>\n<html><body><p>mine</p></body></html>\n"
	const incoming = "<!doctype html>\n<html><body><p>theirs</p></body></html>\n"

	download := func(t *testing.T, root *controlRoot, name string, bytes map[string]any) map[string]any {
		t.Helper()
		server := controlTestServer(t, root)
		status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
			"opId": "3333333333333333", "root": root.id, "name": name, "kind": "sync-download", "bytes": bytes,
		})
		if status != http.StatusOK {
			t.Fatalf("commit = %d %v", status, answer)
		}
		return answer
	}

	t.Run("no version, no write", func(t *testing.T) {
		root := controlTestRoot(t, "root-0")
		path := filepath.Join(root.baseDir, "a.html")
		if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		breakVersions(t, root.baseDir)
		answer := download(t, root, "a.html", map[string]any{"encoding": "utf8", "data": incoming})
		if answer["outcome"] != "refused" || answer["reason"] != "backup-failed" {
			t.Fatalf("answer = %v, want refused backup-failed", answer)
		}
		if data, _ := os.ReadFile(path); string(data) != old {
			t.Fatalf("the file was replaced: %q", data)
		}
	})

	t.Run("an attachment that cannot be kept is not replaced either", func(t *testing.T) {
		root := controlTestRoot(t, "root-0")
		path := filepath.Join(root.baseDir, "image.png")
		before := []byte{0x89, 'P', 'N', 'G', 1}
		if err := os.WriteFile(path, before, 0o644); err != nil {
			t.Fatal(err)
		}
		breakVersions(t, root.baseDir)
		answer := download(t, root, "image.png", map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 2})})
		if answer["outcome"] != "refused" || answer["reason"] != "backup-failed" {
			t.Fatalf("answer = %v, want refused backup-failed", answer)
		}
		if data, _ := os.ReadFile(path); string(data) != string(before) {
			t.Fatalf("the attachment was replaced: %v", data)
		}
	})

	t.Run("a download keeps what it replaces as a version", func(t *testing.T) {
		root := controlTestRoot(t, "root-0")
		path := filepath.Join(root.baseDir, "a.html")
		if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		answer := download(t, root, "a.html", map[string]any{"encoding": "utf8", "data": incoming})
		if answer["outcome"] != "committed" {
			t.Fatalf("answer = %v", answer)
		}
		if data, _ := os.ReadFile(path); string(data) != incoming {
			t.Fatalf("the file holds %q", data)
		}
		versions, err := os.ReadDir(filepath.Join(root.baseDir, filepath.FromSlash(versionsDirName), "a"))
		if err != nil {
			t.Fatal(err)
		}
		var held bool
		for _, v := range versions {
			if data, _ := os.ReadFile(filepath.Join(root.baseDir, filepath.FromSlash(versionsDirName), "a", v.Name())); strings.Contains(string(data), "<p>mine</p>") {
				held = true
			}
		}
		if !held {
			t.Fatalf("no version holds the replaced bytes among %d", len(versions))
		}
	})

	t.Run("a program's first edit is refused when its copy cannot be kept", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, wireDocumentName), []byte("<!doctype html><p>wire</p>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		handler, err := wireTestHost(t).AddRoot(RootSpec{ID: wireRootID, Path: dir, PublicHost: wireRootHost, WirePolicy: framesOnly{}, WireLogger: wireTestLogger(t)})
		if err != nil {
			t.Fatal(err)
		}
		root := handler.(*rootServer)
		docs := &wireDocuments{root: root, internalDir: root.internalDir}
		real, err := filepath.EvalSymlinks(filepath.Join(dir, wireDocumentName))
		if err != nil {
			t.Fatal(err)
		}
		doc, ok := docs.At(real)
		if !ok {
			t.Fatal("not a wire document")
		}
		breakVersions(t, root.baseDir)
		if err := docs.OpenHistory(doc); err == nil {
			t.Fatal("OpenHistory answered nil with no version kept")
		}
	})
}
