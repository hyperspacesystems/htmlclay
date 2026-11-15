package served

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestM4SyncFile(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)
	opSeq := 0
	send := func(op, rel string, data *string) (int, map[string]any) {
		t.Helper()
		opSeq++
		body := map[string]any{"opId": hex.EncodeToString([]byte{0, 0, 0, 0, 0, 0, 0, byte(opSeq)}), "root": "root-0", "op": op, "rel": rel}
		if data != nil {
			body["bytes"] = map[string]any{"encoding": "utf8", "data": *data}
		}
		return controlJSON(t, server, controlSecret, http.MethodPost, "/v1/sync/file", body)
	}
	text := func(s string) *string { return &s }
	refusedNotAllowed := func(t *testing.T, status int, answer map[string]any) {
		t.Helper()
		if status != http.StatusOK || answer["outcome"] != "refused" || answer["reason"] != "not-allowed" {
			t.Fatalf("answer = %d %v, want refused not-allowed", status, answer)
		}
	}

	t.Run("the marker is written and answered with its checksum", func(t *testing.T) {
		marker := `{"rootId":"0123456789abcdef"}`
		status, answer := send("put", ".hyperclay/sync-root.json", text(marker))
		sum := sha256.Sum256([]byte(marker))
		if status != http.StatusOK || answer["outcome"] != "committed" || answer["sha256"] != hex.EncodeToString(sum[:]) {
			t.Fatalf("answer = %d %v", status, answer)
		}
		if data, _ := os.ReadFile(filepath.Join(root.baseDir, ".hyperclay", "sync-root.json")); string(data) != marker {
			t.Fatalf("marker holds %q", data)
		}
		status, answer = send("remove", ".hyperclay/sync-root.json", nil)
		refusedNotAllowed(t, status, answer)
	})

	t.Run("a kept copy three folders deep", func(t *testing.T) {
		status, answer := send("put", ".hyperclay/conflicts/s1/apps/notes/index (theirs).html", text("<p>theirs</p>"))
		if status != http.StatusOK || answer["outcome"] != "committed" {
			t.Fatalf("answer = %d %v", status, answer)
		}
		if _, err := os.Stat(filepath.Join(root.baseDir, ".hyperclay", "conflicts", "s1", "apps", "notes", "index (theirs).html")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("anything else is refused", func(t *testing.T) {
		for _, rel := range []string{
			".hyperclay/conflicts/../versions/a/x.html",
			".hyperclay/conflicts/.hidden/x.html",
			".hyperclay/conflicts/a//x.html",
			".hyperclay/conflicts/",
			".hyperclay/conflicts",
			".hyperclay/sync-root.json.tmp",
			"index.html",
			"../outside.html",
		} {
			status, answer := send("put", rel, text("x"))
			refusedNotAllowed(t, status, answer)
		}
	})

	t.Run("the versions folder is not writable here", func(t *testing.T) {
		status, answer := send("put", ".hyperclay/versions/x", text("x"))
		refusedNotAllowed(t, status, answer)
		if _, err := os.Stat(filepath.Join(root.baseDir, ".hyperclay", "versions", "x")); !os.IsNotExist(err) {
			t.Fatalf("a file was written in the versions folder: %v", err)
		}
	})

	t.Run("a link that leaves the folder is refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("links need privileges on Windows")
		}
		other := controlTestRoot(t, "root-1")
		otherServer := controlTestServer(t, other)
		outside := t.TempDir()
		if err := os.MkdirAll(filepath.Join(other.baseDir, ".hyperclay"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(other.baseDir, ".hyperclay", "conflicts")); err != nil {
			t.Fatal(err)
		}
		status, answer := controlJSON(t, otherServer, controlSecret, http.MethodPost, "/v1/sync/file", map[string]any{
			"opId": "00000000000000ff", "root": "root-1", "op": "put", "rel": ".hyperclay/conflicts/x.html",
			"bytes": map[string]any{"encoding": "utf8", "data": "x"},
		})
		refusedNotAllowed(t, status, answer)
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Fatalf("a file was written outside the folder: %v", entries)
		}
	})

	t.Run("removing a kept copy, and one that is not there", func(t *testing.T) {
		rel := ".hyperclay/conflicts/s1/apps/notes/index (theirs).html"
		if status, answer := send("remove", rel, nil); status != http.StatusOK || answer["outcome"] != "committed" {
			t.Fatalf("remove = %d %v", status, answer)
		}
		if _, err := os.Stat(filepath.Join(root.baseDir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("still there: %v", err)
		}
		if status, answer := send("remove", ".hyperclay/conflicts/never.html", nil); status != http.StatusOK || answer["outcome"] != "committed" {
			t.Fatalf("remove of a missing file = %d %v", status, answer)
		}
	})

	t.Run("a reused opId", func(t *testing.T) {
		body := map[string]any{"opId": "00000000000000aa", "root": "root-0", "op": "remove", "rel": ".hyperclay/conflicts/a.html"}
		if status, _ := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/sync/file", body); status != http.StatusOK {
			t.Fatalf("first = %d", status)
		}
		if status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/sync/file", body); status != http.StatusConflict || answer["error"] != "op-reused" {
			t.Fatalf("second = %d %v, want 409 op-reused", status, answer)
		}
	})
}

// TestM4ControlWritesStayOutOfBookkeeping is hunt finding H2-c (8.54): a server
// folder named .hyperclay made the engine send a sync download named
// .hyperclay/versions/x.html, and Go wrote it among the kept versions. Every
// control write now refuses .hyperclay/, in any case and through a link.
func TestM4ControlWritesStayOutOfBookkeeping(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)
	if err := os.MkdirAll(filepath.Join(root.baseDir, ".hyperclay", "versions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(root.baseDir, ".hyperclay"), filepath.Join(root.baseDir, "lnk")); err != nil {
			t.Fatal(err)
		}
	}
	seq := 0
	opID := func() string {
		seq++
		return hex.EncodeToString([]byte{0x7a, 0, 0, 0, 0, 0, 0, byte(seq)})
	}
	download := func(name string) (int, map[string]any) {
		return controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
			"opId": opID(), "root": "root-0", "name": name, "kind": "sync-download",
			"bytes": map[string]any{"encoding": "utf8", "data": "<p>planted</p>"},
		})
	}

	if status, answer := download("fine.html"); status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("an ordinary download answered %d %v", status, answer)
	}
	names := []string{".hyperclay/versions/x.html", ".HyperClay/versions/x.html", "a/.hyperclay/x.html"}
	if runtime.GOOS != "windows" {
		names = append(names, "lnk/versions/y.html")
	}
	for _, name := range names {
		if status, answer := download(name); status == http.StatusOK && answer["outcome"] == "committed" {
			t.Errorf("a download named %s was committed: %v", name, answer)
		}
	}
	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"opId": opID(), "root": "root-0", "kind": "sync-move",
		"steps": []map[string]any{{"from": "fine.html", "to": ".hyperclay/versions/fine.html"}},
	})
	if status == http.StatusOK && answer["outcome"] == "committed" {
		t.Errorf("a move into .hyperclay/ was committed: %v", answer)
	}
	err := filepath.WalkDir(filepath.Join(root.baseDir, ".hyperclay"), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if data, readErr := os.ReadFile(path); readErr == nil && strings.Contains(string(data), "planted") {
				t.Errorf("server bytes landed in Go's bookkeeping at %s", path)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root.baseDir, "fine.html")); err != nil {
		t.Errorf("the refused move took fine.html away: %v", err)
	}
}
