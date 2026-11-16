package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

func TestM4SyncCLISetup(t *testing.T) {
	previous := serveOS
	serveOS = "linux"
	t.Cleanup(func() { serveOS = previous })

	scratch := t.TempDir()
	bindLog := filepath.Join(scratch, "bind")
	preview := filepath.Join(scratch, "preview")
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", bindLog)
	t.Setenv("HTMLCLAY_STUB_PREVIEW", preview)
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	setup := func(stdin string, args ...string) (int, string, *appSI) {
		si := &appSI{app: a}
		var out, errOut bytes.Buffer
		code := runSync(si, a.rt.configDir, append([]string{"setup"}, args...), strings.NewReader(stdin), &out, &errOut)
		return code, out.String() + errOut.String(), si
	}

	t.Run("answering no sends nothing", func(t *testing.T) {
		writeTestFile(t, preview, `{"ok":true,"files":31,"bytes":482113}`)
		dir := filepath.Join(a.rt.home, "Studio files")
		code, out, si := setup("n\n", "studio", dir)
		if code != serveExitRefused || !strings.Contains(out, "31 files (482 KB)") || !strings.Contains(out, dir) {
			t.Fatalf("no = %d %q", code, out)
		}
		if len(si.sent) != 1 || !strings.HasPrefix(si.sent[0], "sync-preview ") {
			t.Fatalf("frames = %q", si.sent)
		}
		if got := bindsSent(t, bindLog); len(got) != 0 {
			t.Fatalf("binds = %+v", got)
		}
		if got := savedSync(t, a).SyncSessions; len(got) != 0 {
			t.Fatalf("rows = %+v", got)
		}
	})

	t.Run("yes sets the team folder up", func(t *testing.T) {
		dir := filepath.Join(a.rt.home, "Studio files")
		code, out, _ := setup("y\n", "@studio", dir)
		if code != serveExitOK {
			t.Fatalf("yes = %d %q", code, out)
		}
		waitForBinds(t, a)
		binds := bindsSent(t, bindLog)
		if len(binds) != 1 || binds[0].Mode != "empty" || binds[0].Session.AccountID != 40 || binds[0].Root.Path != dir {
			t.Fatalf("binds = %+v", binds)
		}
	})

	t.Run("--existing shows what moves and sends the confirmed setup", func(t *testing.T) {
		writeTestFile(t, preview, `{"ok":true,"files":3,"bytes":30,"toUpload":2,"toDownload":1,"differ":1}`)
		dir := filepath.Join(a.rt.home, "Maya")
		writeTestFile(t, filepath.Join(dir, "notes.html"), "<p>mine</p>")
		code, out, _ := setup("yes\n", "maya", dir, "--existing")
		if code != serveExitOK || !strings.Contains(out, fmt.Sprintf(appcore.TextSyncSetupExisting, 2, 1, 1)) {
			t.Fatalf("existing = %d %q", code, out)
		}
		waitForBinds(t, a)
		binds := bindsSent(t, bindLog)
		if len(binds) != 2 || binds[1].Mode != "existing" || binds[1].Session.AccountID != 12 {
			t.Fatalf("binds = %+v", binds)
		}
	})

	t.Run("an unknown account is refused before anything is sent but the preview", func(t *testing.T) {
		code, out, si := setup("y\n", "nobody", filepath.Join(a.rt.home, "Nobody"))
		if code != serveExitRefused || !strings.Contains(out, "nobody") || len(si.sent) != 1 {
			t.Fatalf("unknown = %d %q %q", code, out, si.sent)
		}
	})
}
