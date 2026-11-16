package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/secret"
)

// stubServiceCommand builds tools/stubservice once per test and points
// HTMLCLAY_SERVICE_CMD at it, with every sync key line it is sent recorded to the
// file it answers.
func stubServiceCommand(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "stubservice")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "../../tools/stubservice")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the stub service: %v\n%s", err, out)
	}
	record := filepath.Join(t.TempDir(), "sync-lines")
	t.Setenv("HTMLCLAY_SERVICE_CMD", bin)
	t.Setenv("HTMLCLAY_STUB_SYNC_LOG", record)
	return record
}

func syncLinesSent(t *testing.T, record string) []string {
	t.Helper()
	data, err := os.ReadFile(record)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestM4KeyLine(t *testing.T) {
	key := "hcsk_" + strings.Repeat("00112233445566778899aabbccddeeff", 2)
	setSyncServer(t, "http://127.0.0.1:9")

	t.Run("no key in the store, no line", func(t *testing.T) {
		record := stubServiceCommand(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		a := newServiceTestApp(t, home, t.TempDir())
		a.rt.syncStore = secret.NewMemory()
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		if lines := syncLinesSent(t, record); len(lines) != 0 {
			t.Fatalf("sent %d sync lines with no key stored", len(lines))
		}
	})

	t.Run("a restarted service gets the key again", func(t *testing.T) {
		record := stubServiceCommand(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		a := newServiceTestApp(t, home, t.TempDir())
		a.rt.syncStore = secret.NewMemory()
		if err := a.rt.syncStore.Set(syncKeyName, key); err != nil {
			t.Fatal(err)
		}
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		want := `{"sync":{"v":1,"serverUrl":"http://127.0.0.1:9","key":"` + key + `"}}`
		if lines := syncLinesSent(t, record); len(lines) != 1 || lines[0] != want {
			t.Fatalf("lines after the first attach = %q, want one %q", lines, want)
		}

		before := a.service.process()
		before.Kill()
		deadline := time.Now().Add(15 * time.Second)
		for {
			after := a.service.process()
			if after != nil && after != before && a.service.state() == appcore.ServiceRunning {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the service did not come back as a new process")
			}
			time.Sleep(25 * time.Millisecond)
		}
		if lines := syncLinesSent(t, record); len(lines) != 2 || lines[1] != want {
			t.Fatalf("lines after the restart = %d, want 2 with the key again", len(lines))
		}
	})

	t.Run("the key line, the clear and a refusal through Process", func(t *testing.T) {
		record := stubServiceCommand(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		a := newServiceTestApp(t, home, t.TempDir())
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		proc := a.service.process()
		if err := proc.SendSyncKey("http://127.0.0.1:9", key, 5*time.Second); err != nil {
			t.Fatalf("SendSyncKey: %v", err)
		}
		if err := proc.ClearSyncKey(5 * time.Second); err != nil {
			t.Fatalf("ClearSyncKey: %v", err)
		}
		if err := proc.SendSyncKey("http://127.0.0.1:9", "not-a-key", 5*time.Second); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("a bad key = %v, want the refusal", err)
		} else if strings.Contains(err.Error(), "not-a-key") {
			t.Fatalf("the refusal carries the key: %v", err)
		}
		if lines := syncLinesSent(t, record); len(lines) != 3 || lines[1] != `{"sync":{"v":1,"key":null}}` {
			t.Fatalf("lines = %q", lines)
		}
	})
}
