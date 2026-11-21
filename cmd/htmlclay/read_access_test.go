package main

import (
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

// readAccessFor finds the one row of the read permissions list for folder.
func readAccessFor(t *testing.T, a *app, folder string) (appcore.ReadAccess, bool) {
	t.Helper()
	var found []appcore.ReadAccess
	for _, r := range a.snapshot().ReadAccess {
		if r.Folder == folder {
			found = append(found, r)
		}
	}
	if len(found) > 1 {
		t.Fatalf("the list shows %s %d times", folder, len(found))
	}
	if len(found) == 0 {
		return appcore.ReadAccess{}, false
	}
	return found[0], true
}

// The list shows a live grant with the page it belongs to, and Remove from the
// browser Settings ends it: the page can no longer read the folder.
func TestReadAccessRemovesALiveGrant(t *testing.T) {
	a, r := newKeptRestart(t)
	loose := filepath.Join(r.home, "notes", "loose.htmlclay")
	assets := filepath.Join(r.home, "assets")
	writeTestFile(t, loose, "<html><body>loose</body></html>")
	writeTestFile(t, filepath.Join(assets, "logo.txt"), "logo")
	s, _ := a.openForTest(t, loose)
	if err := s.sessions.GrantCanonicalRoot(assets); err != nil {
		t.Fatal(err)
	}
	row, ok := readAccessFor(t, a, assets)
	if !ok {
		t.Fatal("the read permissions list does not show the grant")
	}
	if row.Place != "~/assets" {
		t.Errorf("the grant's place = %q, want ~/assets", row.Place)
	}
	if len(row.Pages) != 1 || row.Pages[0] != loose {
		t.Fatalf("the grant's pages = %v, want [%s]", row.Pages, loose)
	}
	if refusal := m3ProgramDo(t, a, ChannelDashboard, "readAccess.remove", map[string]any{"id": row.ID}); refusal != nil {
		t.Fatalf("remove: %+v", refusal)
	}
	if _, _, ok := s.sessions.AssetRoot(filepath.Join(assets, "logo.txt")); ok {
		t.Error("the page still reads the folder after Remove")
	}
	if _, ok := readAccessFor(t, a, assets); ok {
		t.Error("the list still shows the removed grant")
	}
}

// A grant kept across a quit is listed before its tab comes back, and Remove
// takes it out of the record: the tab's first request after that restores the
// tab without it.
func TestReadAccessRemovesAKeptGrant(t *testing.T) {
	first, r := newKeptRestart(t)
	loose := filepath.Join(r.home, "notes", "loose.htmlclay")
	assets := filepath.Join(r.home, "assets")
	writeTestFile(t, loose, "<html><body>loose</body></html>")
	writeTestFile(t, filepath.Join(assets, "logo.txt"), "logo")
	s, _ := first.openForTest(t, loose)
	if err := s.sessions.GrantCanonicalRoot(assets); err != nil {
		t.Fatal(err)
	}
	host, tok := s.host, tokenFor(t, s, loose)
	first.shutdown()
	requirePortFree(t, r.port)

	second := r.again(t)
	row, ok := readAccessFor(t, second, assets)
	if !ok {
		t.Fatal("the list does not show a grant kept across the quit")
	}
	if refusal := m3ProgramDo(t, second, ChannelShell, "readAccess.remove", map[string]any{"id": row.ID}); refusal != nil {
		t.Fatalf("remove: %+v", refusal)
	}
	if code, out := saveWith(t, host, tok, "<html><body>after</body></html>"); code != 200 {
		t.Fatalf("the old tab's save after the restart = %d: %s", code, out)
	}
	restored := liveSiteWithHost(t, second, host)
	if _, _, ok := restored.sessions.AssetRoot(filepath.Join(assets, "logo.txt")); ok {
		t.Error("the removed grant came back with its tab")
	}
	if _, ok := readAccessFor(t, second, assets); ok {
		t.Error("the list still shows the removed grant")
	}
}
