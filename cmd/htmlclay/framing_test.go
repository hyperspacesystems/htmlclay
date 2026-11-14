package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// m3FramingFixture is one served folder with a document in it, so each row of
// TestM3SetFraming sends the action the folder page sends and reads the answer.
type m3FramingFixture struct {
	app     *app
	home    string
	cfgBase string
	lockDir string
	folder  string
	entry   config.ServedFolder
}

func newM3FramingFixture(t *testing.T) *m3FramingFixture {
	t.Helper()
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfgBase := t.TempDir()
	lockDir := t.TempDir()
	a := buildTestApp(t, home, cfgBase)
	a.rt.folderLockDir = lockDir
	a.rt.port = servedTestPort
	a.startServed()
	t.Cleanup(func() {
		if a.served != nil {
			a.served.Close()
		}
	})
	folder := filepath.Join(home, "docs")
	writeTestFile(t, filepath.Join(folder, "index.html"), servedTestDocument)
	entry, refusal, err := a.serveFolder(folder, "docs")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}
	return &m3FramingFixture{app: a, home: home, cfgBase: cfgBase, lockDir: lockDir, folder: folder, entry: entry}
}

// policy is the frame rule the folder's next answer carries, "" when it carries none.
func (f *m3FramingFixture) policy(t *testing.T) string {
	t.Helper()
	host := servedHostName(f.entry.Name, servedTestPort)
	rec := servedRequest(t, f.app, http.MethodGet, host, "/index.html", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET index.html = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	return rec.Header().Get("Content-Security-Policy")
}

// stored is the folder's served entry as the config holds it now.
func (f *m3FramingFixture) stored(t *testing.T) config.ServedFolder {
	t.Helper()
	entry, ok := f.app.rt.cfg.ServedFolderByID(f.entry.ID)
	if !ok {
		t.Fatalf("the config has no served folder %s", f.entry.ID)
	}
	return entry
}

// TestM3SetFraming is step 7.14's host half: the Mac app lets one folder's pages be
// framed and the very next answer proves it, with no restart; the dashboard is
// refused the grant and can only take it away again; a restart keeps the choice;
// and a team folder may not allow frames at all.
func TestM3SetFraming(t *testing.T) {
	t.Run("the Mac app allows frames and the next answer has none", func(t *testing.T) {
		f := newM3FramingFixture(t)
		if policy := f.policy(t); !strings.Contains(policy, "frame-ancestors") {
			t.Fatalf("the frame policy before the switch = %q, want frame-ancestors", policy)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": true,
		}); refusal != nil {
			t.Fatalf("the Mac app allowing frames = %+v", refusal)
		}
		if policy := f.policy(t); policy != "" {
			t.Fatalf("the frame policy after allowing frames = %q, want none", policy)
		}
		if stored := f.stored(t); !stored.AllowFraming {
			t.Fatalf("the stored entry = %+v, want allowFraming", stored)
		}
		folder := m3RelinkFolder(t, f.app, f.entry.ID)
		if !folder.AllowsFraming || !folder.Can.SetFraming {
			t.Fatalf("the snapshot's folder = %+v, want framing allowed and settable", folder)
		}
	})

	t.Run("the dashboard's allow is refused and the header is still there", func(t *testing.T) {
		f := newM3FramingFixture(t)
		before := f.policy(t)
		if before == "" {
			t.Fatal("fixture: the folder starts with no frame policy")
		}
		if refusal := m3ProgramDo(t, f.app, ChannelDashboard, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": true,
		}); refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("the dashboard allowing frames = %+v, want needs-confirmation", refusal)
		}
		if policy := f.policy(t); policy != before {
			t.Fatalf("the frame policy after the dashboard's refusal = %q, want %q", policy, before)
		}
		if stored := f.stored(t); stored.AllowFraming {
			t.Fatalf("the dashboard's refusal stored the choice: %+v", stored)
		}
	})

	t.Run("the dashboard can refuse frames again", func(t *testing.T) {
		f := newM3FramingFixture(t)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": true,
		}); refusal != nil {
			t.Fatalf("the Mac app allowing frames = %+v", refusal)
		}
		if policy := f.policy(t); policy != "" {
			t.Fatalf("fixture: the frame policy after allowing frames = %q, want none", policy)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelDashboard, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": false,
		}); refusal != nil {
			t.Fatalf("the dashboard refusing frames = %+v", refusal)
		}
		if policy := f.policy(t); !strings.Contains(policy, "frame-ancestors") {
			t.Fatalf("the frame policy after the dashboard refused frames = %q, want frame-ancestors", policy)
		}
		if stored := f.stored(t); stored.AllowFraming {
			t.Fatalf("the stored entry = %+v, want framing refused", stored)
		}
	})

	t.Run("a restart keeps the choice", func(t *testing.T) {
		f := newM3FramingFixture(t)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": true,
		}); refusal != nil {
			t.Fatalf("the Mac app allowing frames = %+v", refusal)
		}
		f.app.servedMu.RLock()
		mount := f.app.servedByName[f.entry.Name]
		f.app.servedMu.RUnlock()
		if mount == nil || mount.lock == nil {
			t.Fatal("the served folder has no mount holding its lock")
		}
		if err := mount.lock.Release(); err != nil {
			t.Fatalf("releasing the mount's lock: %v", err)
		}

		restarted := buildTestApp(t, f.home, f.cfgBase)
		restarted.rt.folderLockDir = f.lockDir
		restarted.rt.port = servedTestPort
		restarted.startServed()
		t.Cleanup(func() {
			if restarted.served != nil {
				restarted.served.Close()
			}
		})
		host := servedHostName(f.entry.Name, servedTestPort)
		rec := servedRequest(t, restarted, http.MethodGet, host, "/index.html", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET index.html after the restart = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if policy := rec.Header().Get("Content-Security-Policy"); policy != "" {
			t.Fatalf("the frame policy after the restart = %q, want none", policy)
		}
	})

	t.Run("a team folder is refused", func(t *testing.T) {
		f := newM3FramingFixture(t)
		if err := f.app.rt.cfg.MarkTeamFolder(config.TeamFolder{
			FolderID: f.entry.ID,
			Path:     f.folder,
			Identity: platform.DirIdentity(f.folder),
			Kind:     config.TeamFolderKindTeam,
			Key:      "acme",
		}); err != nil {
			t.Fatal(err)
		}
		if err := f.app.rt.cfg.Save(); err != nil {
			t.Fatal(err)
		}
		folder := m3RelinkFolder(t, f.app, f.entry.ID)
		if folder.Can.SetFraming {
			t.Fatalf("the team folder's switch = %+v, want it refused", folder.Can)
		}
		before := f.policy(t)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": true,
		}); refusal == nil || refusal.Code != appcore.RefuseNotAllowedNow {
			t.Fatalf("allowing frames for a team folder = %+v, want not-allowed-now", refusal)
		}
		if policy := f.policy(t); policy != before {
			t.Fatalf("the frame policy after the team folder's refusal = %q, want %q", policy, before)
		}
		if stored := f.stored(t); stored.AllowFraming {
			t.Fatalf("the team folder stored the choice: %+v", stored)
		}
	})

	t.Run("a failed save changes neither", func(t *testing.T) {
		f := newM3FramingFixture(t)
		before := f.policy(t)
		blockConfigDir(t, f.cfgBase)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.setFraming", map[string]any{
			"id": f.entry.ID, "allow": true,
		}); refusal == nil || refusal.Code != appcore.RefuseFailed {
			t.Fatalf("allowing frames with an unwritable config = %+v, want failed", refusal)
		}
		if stored := f.stored(t); stored.AllowFraming {
			t.Fatalf("the failed save left the choice as %+v", stored)
		}
		if policy := f.policy(t); policy != before {
			t.Fatalf("the frame policy after the failed save = %q, want %q", policy, before)
		}
	})
}
