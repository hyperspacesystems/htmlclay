package localimport

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	oldRoot = "0b6a4c2e-7b1d-4f0a-9d53-2f9c1e7a4b10"
	newRoot = "a1b2c3d4e5f60718"
	session = "f3a1c7d2-58a4-4c0b-8a57-6d3e2b1a9c44"
)

func write(t *testing.T, file, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, file string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// local makes a disposable Hyperclay Local data folder with one synced root.
func local(t *testing.T) (userData, folder string) {
	t.Helper()
	userData, folder = t.TempDir(), t.TempDir()
	settings := map[string]any{
		"settingsVersion": 2, "deviceId": "device-1", "actor": map[string]any{"id": 3, "username": "sam"},
		"roots":        []any{map[string]any{"id": oldRoot, "kind": "team", "path": folder, "port": 4322}},
		"syncSessions": []any{map[string]any{"id": session, "rootId": oldRoot, "accountId": 7, "kind": "team", "legacyMetaDir": nil}},
	}
	raw, _ := json.Marshal(settings)
	write(t, filepath.Join(userData, "settings.json"), string(raw))
	meta := filepath.Join(userData, "sync-meta", "v2", session)
	write(t, filepath.Join(meta, "identity.json"), `{"rootId":"`+oldRoot+`","rootMarker":true,"rootDir":"123:456","accountId":7}`)
	write(t, filepath.Join(meta, "node-map.json"), `{"2":{"path":"board.html"}}`)
	write(t, filepath.Join(meta, "conflicts.json"), `{}`)
	write(t, filepath.Join(folder, ".hyperclay", "sync-root.json"), `{"rootId":"`+oldRoot+`","createdAt":"2026-10-01T00:00:00.000Z"}`)
	return userData, folder
}

func TestSettingsAndRunning(t *testing.T) {
	userData, folder := local(t)
	s, err := ReadSettings(userData)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Roots) != 1 || s.Roots[0].Path != folder || !s.Roots[0].WasTeam() || s.SessionFor(oldRoot).ID != session {
		t.Fatalf("settings read as %+v", s)
	}
	if _, err := ReadSettings(t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no settings answered %v, want ErrNotFound", err)
	}
	old := t.TempDir()
	write(t, filepath.Join(old, "settings.json"), `{"selectedFolder":"/x"}`)
	if _, err := ReadSettings(old); !errors.Is(err, ErrOldSettings) {
		t.Fatalf("version 1 settings answered %v, want ErrOldSettings", err)
	}
	if _, running := RunningPID(userData, func(int) bool { return true }); running {
		t.Fatal("no served-roots.json must read as not running")
	}
	write(t, filepath.Join(userData, "served-roots.json"), `{"v":1,"pid":4242,"roots":[]}`)
	if pid, running := RunningPID(userData, func(pid int) bool { return pid == 4242 }); !running || pid != 4242 {
		t.Fatalf("a live pid read as %d, %v", pid, running)
	}
	if _, running := RunningPID(userData, func(int) bool { return false }); running {
		t.Fatal("a dead pid must read as not running")
	}
	if ports := ServedPorts(userData); len(ports) != 0 {
		t.Fatalf("no served roots read as ports %v", ports)
	}
	// A Local that runs without serving still holds Electron's single-instance link.
	quiet := t.TempDir()
	if err := os.Symlink("some-host.local-5151", filepath.Join(quiet, "SingletonLock")); err != nil {
		t.Skip("no symlinks here")
	}
	if pid, running := RunningPID(quiet, func(pid int) bool { return pid == 5151 }); !running || pid != 5151 {
		t.Fatalf("the single-instance link read as %d, %v", pid, running)
	}
}

func TestCopyBookkeepingRewritesOnlyTheRootID(t *testing.T) {
	userData, _ := local(t)
	serviceData := t.TempDir()
	s, _ := ReadSettings(userData)
	sess := *s.SessionFor(oldRoot)
	if err := CopyBookkeeping(userData, serviceData, sess, newRoot); err != nil {
		t.Fatal(err)
	}
	target := TargetMeta(serviceData, sess)
	identity := readJSON(t, filepath.Join(target, "identity.json"))
	if identity["rootId"] != newRoot || identity["rootDir"] != "123:456" || identity["rootMarker"] != true || identity["accountId"] != float64(7) {
		t.Fatalf("identity.json after the copy: %v", identity)
	}
	if got := readJSON(t, filepath.Join(SourceMeta(userData, sess), "identity.json"))["rootId"]; got != oldRoot {
		t.Fatalf("Local's own identity.json changed to %v", got)
	}
	if same, err := SameBookkeeping(SourceMeta(userData, sess), target); err != nil || !same {
		t.Fatalf("the copy differs from its source: %v %v", same, err)
	}
	if _, err := os.Stat(target + ".import"); !os.IsNotExist(err) {
		t.Fatal("the staging folder was left behind")
	}
	// Local ran again and changed its baseline: the copy is stale until copied again.
	write(t, filepath.Join(SourceMeta(userData, sess), "node-map.json"), `{"2":{"path":"board.html"},"3":{"path":"new.html"}}`)
	if same, _ := SameBookkeeping(SourceMeta(userData, sess), target); same {
		t.Fatal("a changed source must read as different")
	}
	if err := CopyBookkeeping(userData, serviceData, sess, newRoot); err != nil {
		t.Fatal(err)
	}
	if same, _ := SameBookkeeping(SourceMeta(userData, sess), target); !same {
		t.Fatal("a second copy must replace the first")
	}
}

func TestCopyBookkeepingRefusesWhatItCannotProve(t *testing.T) {
	userData, _ := local(t)
	s, _ := ReadSettings(userData)
	sess := *s.SessionFor(oldRoot)
	if err := os.Remove(filepath.Join(SourceMeta(userData, sess), "identity.json")); err != nil {
		t.Fatal(err)
	}
	serviceData := t.TempDir()
	if err := CopyBookkeeping(userData, serviceData, sess, newRoot); err == nil {
		t.Fatal("a version 2 session with no identity.json must be refused")
	}
	if _, err := os.Stat(TargetMeta(serviceData, sess)); !os.IsNotExist(err) {
		t.Fatal("a refused copy must leave nothing in place")
	}
	missing := Session{ID: "nope"}
	if err := CopyBookkeeping(userData, serviceData, missing, newRoot); err == nil {
		t.Fatal("a session with no bookkeeping must be refused")
	}
	legacyDir := "3f9a1c2b4d5e"
	legacy := Session{ID: "legacy-1", LegacyMetaDir: &legacyDir}
	write(t, filepath.Join(userData, "sync-meta", legacyDir, "node-map.json"), `{}`)
	if err := CopyBookkeeping(userData, serviceData, legacy, newRoot); err != nil {
		t.Fatalf("a legacy session copies as it is: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serviceData, "sync-meta", legacyDir, "node-map.json")); err != nil {
		t.Fatal("the legacy bookkeeping did not arrive")
	}
	write(t, filepath.Join(SourceMeta(userData, sess), "bind-in-progress.json"), `{}`)
	if !UnfinishedBind(userData, sess) {
		t.Fatal("an unfinished first download must be seen")
	}
}

func TestMarkerAndReceipt(t *testing.T) {
	_, folder := local(t)
	if id, ok := MarkerRootID(folder); !ok || id != oldRoot {
		t.Fatalf("marker read as %q, %v", id, ok)
	}
	if err := RewriteMarker(folder, newRoot); err != nil {
		t.Fatal(err)
	}
	marker := readJSON(t, filepath.Join(folder, ".hyperclay", "sync-root.json"))
	if marker["rootId"] != newRoot || marker["createdAt"] != "2026-10-01T00:00:00.000Z" {
		t.Fatalf("marker after the rewrite: %v", marker)
	}
	if err := SetMarkerAside(folder); err != nil {
		t.Fatal(err)
	}
	if _, ok := MarkerRootID(folder); ok {
		t.Fatal("a marker set aside must not read as a marker")
	}

	file := filepath.Join(t.TempDir(), "local-import.json")
	r, err := LoadReceipt(file, "/local")
	if err != nil || len(r.Roots) != 0 {
		t.Fatalf("no receipt read as %+v, %v", r, err)
	}
	r.Put(ReceiptRoot{LocalRootID: oldRoot, Path: folder, FolderID: newRoot, SessionID: session, LocalPort: 4322, State: StateStarted})
	r.Put(ReceiptRoot{LocalRootID: oldRoot, Path: folder, FolderID: newRoot, SessionID: session, LocalPort: 4322, State: StateDone})
	if err := r.Save(file); err != nil {
		t.Fatal(err)
	}
	again, err := LoadReceipt(file, "/local")
	if err != nil || len(again.Roots) != 1 || again.Root(oldRoot).State != StateDone || again.Root("other") != nil {
		t.Fatalf("receipt read back as %+v, %v", again, err)
	}
}
