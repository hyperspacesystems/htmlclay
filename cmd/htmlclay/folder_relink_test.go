package main

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// m3RelinkFixture is one document in one folder that can be trusted, moved,
// replaced, and served, so each row of TestM3FolderRelink sets up the folder it is
// about and then sends the Mac app's own folder.relink.
type m3RelinkFixture struct {
	app      *app
	cfgBase  string
	folder   string
	document string
}

func newM3RelinkFixture(t *testing.T) *m3RelinkFixture {
	t.Helper()
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, home, cfgBase)
	folder := filepath.Join(home, "docs")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	return &m3RelinkFixture{app: a, cfgBase: cfgBase, folder: folder, document: writeHelperPage(t, folder)}
}

// trust records the fixture's folder as trusted and answers the entry it keeps.
func (f *m3RelinkFixture) trust(t *testing.T) config.TrustedFolder {
	t.Helper()
	if err := f.app.trustFolder(f.folder); err != nil {
		t.Fatal(err)
	}
	return m3RelinkEntry(t, f.app, f.folder)
}

func (f *m3RelinkFixture) save(t *testing.T) {
	t.Helper()
	if err := f.app.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

// m3RelinkEntry answers the trusted entry the config holds for path.
func m3RelinkEntry(t *testing.T, a *app, path string) config.TrustedFolder {
	t.Helper()
	for _, entry := range a.rt.cfg.TrustedFolderList() {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("no trusted entry for %s: %+v", path, a.rt.cfg.TrustedFolderList())
	return config.TrustedFolder{}
}

// m3RelinkFolder answers the folder the snapshot lists with id.
func m3RelinkFolder(t *testing.T, a *app, id string) appcore.Folder {
	t.Helper()
	for _, folder := range a.snapshot().Folders {
		if folder.ID == id {
			return folder
		}
	}
	t.Fatalf("the snapshot does not list folder %s: %+v", id, a.snapshot().Folders)
	return appcore.Folder{}
}

// m3RelinkCopy copies the tree at src to dst, so the folder there is a different
// one: a copy has an identity of its own.
func m3RelinkCopy(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestM3FolderRelink is step 7.12: a moved folder is found again from the Mac app
// and keeps its id, name and address; a different folder at the new path needs the
// warnings acknowledged; and a team folder stays one.
func TestM3FolderRelink(t *testing.T) {
	t.Run("a moved folder is found again and keeps its id, name and address", func(t *testing.T) {
		f := newM3RelinkFixture(t)
		entry := f.trust(t)
		if _, err := f.app.rt.cfg.AddServedFolder(f.folder, platform.DirIdentity(f.folder), "docs"); err != nil {
			t.Fatal(err)
		}
		f.save(t)
		before := m3RelinkFolder(t, f.app, entry.ID)
		if before.Address == "" {
			t.Fatalf("fixture: the folder has no address: %+v", before)
		}

		moved := filepath.Join(f.app.rt.home, "projects", "docs")
		if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(f.folder, moved); err != nil {
			t.Fatal(err)
		}
		if gone := m3RelinkFolder(t, f.app, entry.ID); gone.State != appcore.FolderMissing {
			t.Fatalf("fixture: the moved folder's state = %s, want %s", gone.State, appcore.FolderMissing)
		}

		preview, refusal := shellBackend{f.app}.Check(m3ProgramAction(t, "folder.relink", map[string]any{
			"id": entry.ID, "path": moved,
		}))
		if refusal != nil {
			t.Fatalf("the Find it sheet for the moved folder = %+v", refusal)
		}
		if len(preview.Warnings) != 0 {
			t.Fatalf("the Find it sheet for the same folder, moved, shows %+v, want nothing to acknowledge", preview.Warnings)
		}
		if preview.Path != moved {
			t.Fatalf("the Find it sheet shows %q, want %q", preview.Path, moved)
		}

		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": moved, "acknowledged": []string{},
		}); refusal != nil {
			t.Fatalf("the relink of the same folder, moved = %+v", refusal)
		}

		after := m3RelinkFolder(t, f.app, entry.ID)
		if after.ID != before.ID || after.Path != moved || after.Address != before.Address || after.Name != before.Name {
			t.Fatalf("the relinked folder = %+v, want id %s at %s, named %s and served at %s", after, before.ID, moved, before.Name, before.Address)
		}
		if after.State != appcore.FolderOK {
			t.Fatalf("the relinked folder's state = %s, want %s", after.State, appcore.FolderOK)
		}
		if stored := m3RelinkEntry(t, f.app, moved); stored.ID != entry.ID || stored.Identity != platform.DirIdentity(moved) {
			t.Fatalf("the stored entry = %+v, want id %s at %s", stored, entry.ID, moved)
		}
		if served, ok := f.app.rt.cfg.ServedFolderByID(entry.ID); !ok || served.Path != moved || served.Name != "docs" {
			t.Fatalf("the served entry = %+v, want %s at %s", served, "docs", moved)
		}
	})

	t.Run("the folder's address answers from its new path", func(t *testing.T) {
		f := newM3RelinkFixture(t)
		entry := f.trust(t)
		f.app.rt.port = servedTestPort
		f.app.startServed()
		t.Cleanup(func() {
			if f.app.served != nil {
				f.app.served.Close()
			}
		})
		if _, err := f.app.rt.cfg.AddServedFolder(f.folder, platform.DirIdentity(f.folder), "docs"); err != nil {
			t.Fatal(err)
		}
		f.save(t)
		before := m3RelinkFolder(t, f.app, entry.ID)
		served, ok := f.app.rt.cfg.ServedFolderByID(entry.ID)
		if !ok {
			t.Fatalf("fixture: the folder has no served entry")
		}
		if err := f.app.mountServed(served); err != nil {
			t.Fatalf("fixture: mounting the folder: %v", err)
		}
		host := servedHostName("docs", servedTestPort)
		if rec := servedRequest(t, f.app, http.MethodGet, host, "/page.htmlclay", "", nil); rec.Code != http.StatusOK {
			t.Fatalf("fixture: GET before the move = %d (%s)", rec.Code, rec.Body.String())
		}

		moved := filepath.Join(f.app.rt.home, "projects", "docs")
		if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(f.folder, moved); err != nil {
			t.Fatal(err)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": moved, "acknowledged": []string{},
		}); refusal != nil {
			t.Fatalf("the relink of the served folder = %+v", refusal)
		}
		if after := m3RelinkFolder(t, f.app, entry.ID); after.Address != before.Address {
			t.Fatalf("the relinked folder's address = %q, want %q", after.Address, before.Address)
		}
		if rec := servedRequest(t, f.app, http.MethodGet, host, "/page.htmlclay", "", nil); rec.Code != http.StatusOK {
			t.Fatalf("GET from the moved folder = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("a different folder needs acknowledging", func(t *testing.T) {
		f := newM3RelinkFixture(t)
		entry := f.trust(t)
		other := filepath.Join(f.app.rt.home, "elsewhere")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(f.folder); err != nil {
			t.Fatal(err)
		}

		preview, refusal := shellBackend{f.app}.Check(m3ProgramAction(t, "folder.relink", map[string]any{
			"id": entry.ID, "path": other,
		}))
		if refusal != nil {
			t.Fatalf("the Find it sheet for a different folder = %+v", refusal)
		}
		var codes []string
		for _, warning := range preview.Warnings {
			codes = append(codes, warning.Code)
		}
		if !slices.Contains(codes, warnDifferentFolder) {
			t.Fatalf("the Find it sheet for a different folder shows %v, want %s", codes, warnDifferentFolder)
		}

		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": other, "acknowledged": []string{},
		}); refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("an unacknowledged relink to a different folder = %+v, want needs-confirmation", refusal)
		}
		if stored := m3RelinkEntry(t, f.app, f.folder); stored.Path != f.folder {
			t.Fatalf("the refused relink moved the entry to %s", stored.Path)
		}

		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": other, "acknowledged": []string{warnDifferentFolder},
		}); refusal != nil {
			t.Fatalf("an acknowledged relink to a different folder = %+v", refusal)
		}
		if after := m3RelinkFolder(t, f.app, entry.ID); after.Path != other || after.State != appcore.FolderOK {
			t.Fatalf("the relinked folder = %+v, want %s", after, other)
		}
	})

	t.Run("a relinked team folder is still a team folder", func(t *testing.T) {
		f := newM3RelinkFixture(t)
		entry := f.trust(t)
		if err := f.app.rt.cfg.MarkTeamFolder(config.TeamFolder{
			FolderID: entry.ID,
			Path:     f.folder,
			Identity: platform.DirIdentity(f.folder),
			Kind:     config.TeamFolderKindTeam,
			Key:      "acme",
		}); err != nil {
			t.Fatal(err)
		}
		f.save(t)
		team := config.Scope{Kind: config.ScopeTeam, Key: "team:acme"}
		if scope := f.app.scopeOf(f.document); scope != team {
			t.Fatalf("fixture: the document's scope = %+v, want %+v", scope, team)
		}

		moved := filepath.Join(f.app.rt.home, "acme-moved")
		m3RelinkCopy(t, f.folder, moved)
		if err := os.RemoveAll(f.folder); err != nil {
			t.Fatal(err)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": moved, "acknowledged": []string{warnDifferentFolder},
		}); refusal != nil {
			t.Fatalf("the relink of the team folder = %+v", refusal)
		}
		if scope := f.app.scopeOf(filepath.Join(moved, "page.htmlclay")); scope != team {
			t.Fatalf("the relinked folder's scope = %+v, want %+v", scope, team)
		}
		carried := false
		for _, recorded := range f.app.rt.cfg.TeamFolderList() {
			if recorded.Path == moved && recorded.Identity == platform.DirIdentity(moved) && recorded.Key == "acme" && recorded.FolderID == entry.ID {
				carried = true
			}
		}
		if !carried {
			t.Fatalf("the team record = %+v, want an entry for %s", f.app.rt.cfg.TeamFolderList(), moved)
		}
	})

	t.Run("a path another folder holds is refused", func(t *testing.T) {
		f := newM3RelinkFixture(t)
		entry := f.trust(t)
		other := filepath.Join(f.app.rt.home, "other")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := f.app.trustFolder(other); err != nil {
			t.Fatal(err)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": other, "acknowledged": []string{warnDifferentFolder},
		}); refusal == nil || refusal.Code != appcore.RefuseNotAllowedNow {
			t.Fatalf("a relink onto another folder's path = %+v, want not-allowed-now", refusal)
		}
		if stored := m3RelinkEntry(t, f.app, f.folder); stored.ID != entry.ID {
			t.Fatalf("the refused relink left the entry as %+v", stored)
		}
	})

	t.Run("a failed save changes nothing", func(t *testing.T) {
		f := newM3RelinkFixture(t)
		entry := f.trust(t)
		moved := filepath.Join(f.app.rt.home, "moved")
		if err := os.Rename(f.folder, moved); err != nil {
			t.Fatal(err)
		}
		blockConfigDir(t, f.cfgBase)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "folder.relink", map[string]any{
			"id": entry.ID, "path": moved, "acknowledged": []string{},
		}); refusal == nil || refusal.Code != appcore.RefuseFailed {
			t.Fatalf("a relink with an unwritable config = %+v, want failed", refusal)
		}
		if stored := m3RelinkEntry(t, f.app, f.folder); stored.ID != entry.ID || stored.Identity != entry.Identity {
			t.Fatalf("the failed relink left the entry as %+v, want %+v", stored, entry)
		}
	})
}
