package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
)

// mergeFixture is the home every merge subtest starts from: the child folder is
// trusted and answers at its own address, and the parent is trusted around it,
// which is the state a merge is offered in.
type mergeFixture struct {
	*nestedFixture
	a          *app
	childHost  string
	parentHost string
	childID    string
}

func newMergeFixture(t *testing.T) *mergeFixture {
	t.Helper()
	f := newNestedFixture(t)
	f.config(t, f.childOnly())
	a := f.launch(t)
	if err := a.trustFolder(f.parent); err != nil {
		t.Fatalf("trust the parent: %v", err)
	}
	childID := snapshotFolderAt(t, a.snapshot(), f.child).ID
	return &mergeFixture{nestedFixture: f, a: a, childHost: nestedHost("htmlclay1"), parentHost: nestedHost("htmlclay2"), childID: childID}
}

func (m *mergeFixture) merge(t *testing.T, ch Channel) *appcore.Refusal {
	t.Helper()
	_, refusal := m.a.do(context.Background(), coreAction(t, "merge-1", "folder.merge", map[string]string{"id": m.childID}), ch)
	return refusal
}

// saveAt saves body at host and asserts the document on disk holds it.
func saveAt(t *testing.T, a *app, host, rel, abs, body string) {
	t.Helper()
	token := nestedEditable(t, a, host, rel)
	save := servedRequest(t, a, http.MethodPost, host, "/_/save/"+token, body, map[string]string{
		"Content-Type":   "text/html",
		"Origin":         "http://" + host,
		"Sec-Fetch-Site": "same-origin",
	})
	if save.Code != http.StatusOK {
		t.Fatalf("saving %s at %s = %d, want 200 (%s)", rel, host, save.Code, save.Body.String())
	}
	written, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), body) {
		t.Fatalf("the save at %s did not reach %s: %q", host, rel, written)
	}
}

// fileDigests is every file under dir and the hash of its bytes.
func fileDigests(t *testing.T, dir string) map[string]string {
	t.Helper()
	digests := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		digests[path] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return digests
}

func TestM3Merge(t *testing.T) {
	t.Run("the dashboard cannot merge", func(t *testing.T) {
		m := newMergeFixture(t)
		refusal := m.merge(t, ChannelDashboard)
		if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("a merge from the dashboard = %+v, want %s", refusal, appcore.RefuseNeedsConfirmation)
		}
		if !ownAddress(t, m.a, m.child) {
			t.Fatal("a refused dashboard merge took the child's address")
		}
		// Both addresses still answer, and both documents still save where they are.
		saveAt(t, m.a, m.childHost, m.address(m.cdoc), m.cdoc, "<html><body>child</body></html>")
		saveAt(t, m.a, m.parentHost, m.address(m.pdoc), m.pdoc, "<html><body>parent</body></html>")
	})

	t.Run("a dirty page that cannot flush stops the merge and both addresses still save", func(t *testing.T) {
		m := newMergeFixture(t)
		before := fileDigests(t, m.home)
		m.a.service = unsavedService(t, m.a)

		refusal := m.merge(t, ChannelShell)
		if refusal == nil || refusal.Code != appcore.RefuseUnsavedEdits {
			t.Fatalf("a merge with a page that cannot save = %+v, want %s", refusal, appcore.RefuseUnsavedEdits)
		}
		if len(refusal.Documents) == 0 {
			t.Fatal("the refusal names no document")
		}
		if !ownAddress(t, m.a, m.child) {
			t.Fatal("a refused merge took the child's address")
		}
		saveAt(t, m.a, m.childHost, m.address(m.cdoc), m.cdoc, "<html><body>child</body></html>")
		saveAt(t, m.a, m.parentHost, m.address(m.pdoc), m.pdoc, "<html><body>parent</body></html>")
		if after := fileDigests(t, m.home); len(after) != len(before) {
			t.Fatalf("a refused merge changed the files: %d before, %d after", len(before), len(after))
		}
	})

	t.Run("a failed config write changes nothing", func(t *testing.T) {
		m := newMergeFixture(t)
		cfgDir := config.DirFrom(m.cfgBase)
		onDisk, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(cfgDir, 0500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(cfgDir, 0755) })

		refusal := m.merge(t, ChannelShell)
		if err := os.Chmod(cfgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if refusal == nil || refusal.Code != appcore.RefuseFailed {
			t.Fatalf("a merge the config write could not record = %+v, want %s", refusal, appcore.RefuseFailed)
		}
		written, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		if string(written) != string(onDisk) {
			t.Fatalf("the failed write rewrote the config: %s", written)
		}
		if !ownAddress(t, m.a, m.child) {
			t.Fatal("the failed write cleared the child's address")
		}
		saveAt(t, m.a, m.childHost, m.address(m.cdoc), m.cdoc, "<html><body>child</body></html>")
		saveAt(t, m.a, m.parentHost, m.address(m.pdoc), m.pdoc, "<html><body>parent</body></html>")
	})

	t.Run("after a merge the old address redirects and the parent's address saves the document", func(t *testing.T) {
		m := newMergeFixture(t)
		// Open the child's document at the child's own address first, so the merge
		// has a page to route again.
		nestedEditable(t, m.a, m.childHost, m.address(m.cdoc))

		if refusal := m.merge(t, ChannelShell); refusal != nil {
			t.Fatalf("merge refused: %+v", refusal)
		}
		rel := m.address(m.cdoc)
		nestedRedirect(t, m.a, m.childHost, rel, fileURL(m.parentHost, rel))
		nestedEditable(t, m.a, m.parentHost, rel)
		if ownAddress(t, m.a, m.child) {
			t.Fatal("the merge left the child keeping its own address")
		}
		if slug := m.a.rt.cfg.SiteSlug(m.child); slug != "htmlclay1" {
			t.Fatalf("the child's slug = %q, want it kept as the old address that redirects", slug)
		}
		oneSiteAnchoredAt(t, m.a, m.parent)
		saveAt(t, m.a, m.parentHost, m.address(m.cdoc), m.cdoc, "<html><body>saved after the merge</body></html>")
		if n := registrations(t, m.a, m.cdoc); n != 1 {
			t.Fatalf("%s is registered in %d sites, want one", rel, n)
		}
		if got, want := hostOf(t, m.a, m.cdoc), liveSite(t, m.a, m.parent); got != want {
			t.Fatalf("%s is held by %s, want the parent's site %s", rel, got.host, want.host)
		}
		// The parent's own document is untouched by the merge.
		saveAt(t, m.a, m.parentHost, m.address(m.pdoc), m.pdoc, "<html><body>parent</body></html>")
	})

	t.Run("a restart after a merge keeps the old address redirecting", func(t *testing.T) {
		m := newMergeFixture(t)
		if refusal := m.merge(t, ChannelShell); refusal != nil {
			t.Fatalf("merge refused: %+v", refusal)
		}
		m.a.shutdown()
		second := m.launch(t)
		if ownAddress(t, second, m.child) {
			t.Fatal("the config on disk still gives the child its own address")
		}
		rel := m.address(m.cdoc)
		nestedRedirect(t, second, m.childHost, rel, fileURL(m.parentHost, rel))
		nestedEditable(t, second, m.parentHost, rel)
		oneSiteAnchoredAt(t, second, m.parent)
	})

	t.Run("every file's bytes are identical before and after", func(t *testing.T) {
		m := newMergeFixture(t)
		nestedEditable(t, m.a, m.childHost, m.address(m.cdoc))
		before := fileDigests(t, m.home)
		if refusal := m.merge(t, ChannelShell); refusal != nil {
			t.Fatalf("merge refused: %+v", refusal)
		}
		after := fileDigests(t, m.home)
		if len(after) != len(before) {
			t.Fatalf("the merge added or removed files: %d before, %d after", len(before), len(after))
		}
		for path, sum := range before {
			if after[path] != sum {
				t.Fatalf("%s changed during the merge", path)
			}
		}
	})
}
