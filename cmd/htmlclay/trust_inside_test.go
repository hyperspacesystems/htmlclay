package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/shellapi"
	"github.com/panphora/htmlclay/internal/testutil"
)

// insideFixture is the home every subtest starts from: the folder around is
// trusted and answers at its address, the folder inside it is not trusted, and
// the inner folder's document is open at the address of the folder around it,
// which is the state the Trust sheet warns about.
type insideFixture struct {
	*nestedFixture
	a          *app
	aroundHost string
	rel        string
	oldToken   string
	actions    int
}

func newInsideFixture(t *testing.T) *insideFixture {
	t.Helper()
	f := newNestedFixture(t)
	a := f.launch(t)
	if err := a.trustFolder(f.parent); err != nil {
		t.Fatalf("trust the folder around: %v", err)
	}
	in := &insideFixture{nestedFixture: f, a: a, aroundHost: nestedHost(a.rt.cfg.SiteSlug(f.parent)), rel: f.address(f.cdoc)}
	in.oldToken = nestedEditable(t, a, in.aroundHost, in.rel)
	oneSiteMaySave(t, a, f.cdoc, f.parent)
	return in
}

// trust sends folder.trust the way the Trust sheet does, with the codes the
// person accepted.
func (f *insideFixture) trust(t *testing.T, path string, acknowledged ...string) *appcore.Refusal {
	t.Helper()
	return trustAction(t, f.a, &f.actions, path, acknowledged...)
}

func trustAction(t *testing.T, a *app, sent *int, path string, acknowledged ...string) *appcore.Refusal {
	t.Helper()
	*sent++
	args := map[string]any{"path": path}
	if acknowledged != nil {
		args["acknowledged"] = acknowledged
	}
	_, refusal := a.do(context.Background(), coreAction(t, fmt.Sprintf("trust-%d", *sent), "folder.trust", args), ChannelShell)
	return refusal
}

func (f *insideFixture) configBytes(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(config.DirFrom(f.cfgBase), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// nothingMoved asserts a refused trust left everything where it was: the config
// file, the trusted list, the addresses, and the one address that saves the
// inner folder's document.
func (f *insideFixture) nothingMoved(t *testing.T, configBefore string) {
	t.Helper()
	if after := f.configBytes(t); after != configBefore {
		t.Fatalf("the refused trust rewrote the config:\n%s\nwant\n%s", after, configBefore)
	}
	if _, listed := f.a.trustedEntry(f.child); listed {
		t.Fatalf("the refused trust put %s on the trusted list", f.child)
	}
	if slug := f.a.rt.cfg.SiteSlug(f.child); slug != "" {
		t.Fatalf("the refused trust gave %s the address %s", f.child, slug)
	}
	oneSiteAnchoredAt(t, f.a, f.parent)
	saveAt(t, f.a, f.aroundHost, f.rel, f.cdoc, "<html><body>still saved at the old address</body></html>")
	oneSiteMaySave(t, f.a, f.cdoc, f.parent)
}

// trustCheck is the Trust sheet's check of path, and its inside-trusted warning
// when it carries one.
func trustCheck(t *testing.T, a *app, path string) (shellapi.Preview, *shellapi.Warning) {
	t.Helper()
	preview, refusal := shellBackend{a}.Check(coreAction(t, "check-"+filepath.Base(path), "folder.trust", map[string]string{"path": path}))
	if refusal != nil {
		t.Fatalf("Check of %s refused: %+v", path, refusal)
	}
	for i := range preview.Warnings {
		if preview.Warnings[i].Code == warnInsideTrusted {
			return preview, &preview.Warnings[i]
		}
	}
	return preview, nil
}

// TestM3TrustInsideTrusted proves step 7.45: a folder trusted inside a trusted
// folder gets an address of its own only after the person acknowledged the
// warning, and then its documents answer there and the old address redirects.
func TestM3TrustInsideTrusted(t *testing.T) {
	t.Run("without the acknowledgement nothing changes", func(t *testing.T) {
		f := newInsideFixture(t)
		before := f.configBytes(t)
		files := fileDigests(t, f.home)

		refusal := f.trust(t, f.child)
		if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("an unacknowledged trust inside a trusted folder = %+v, want %s", refusal, appcore.RefuseNeedsConfirmation)
		}
		// Another warning's code is not this one's acceptance.
		refusal = f.trust(t, f.child, "contains-trusted", "personal-folder")
		if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("a trust that acknowledged other codes = %+v, want %s", refusal, appcore.RefuseNeedsConfirmation)
		}
		for path, sum := range fileDigests(t, f.home) {
			if files[path] != sum {
				t.Fatalf("%s changed during a refused trust", path)
			}
		}
		f.nothingMoved(t, before)
	})

	t.Run("with it the folder's documents answer at the new address and the old address redirects", func(t *testing.T) {
		f := newInsideFixture(t)
		// The sheet acknowledges the code the check showed it.
		_, warning := trustCheck(t, f.a, f.child)
		if warning == nil {
			t.Fatal("the check of the folder inside carries no inside-trusted warning")
		}
		// The page open at the old address had its program channel bound there.
		around := liveSite(t, f.a, f.parent)
		around.srv.Core.BindHelpers(f.cdoc)

		if refusal := f.trust(t, f.child, warning.Code); refusal != nil {
			t.Fatalf("the acknowledged trust refused: %+v", refusal)
		}
		if !ownAddress(t, f.a, f.child) {
			t.Fatal("the new entry does not keep its own address")
		}
		slug := f.a.rt.cfg.SiteSlug(f.child)
		if slug == "" || slug == f.a.rt.cfg.SiteSlug(f.parent) {
			t.Fatalf("the folder inside has the address %q, want one of its own", slug)
		}
		newHost := nestedHost(slug)

		nestedRedirect(t, f.a, f.aroundHost, f.rel, fileURL(newHost, f.rel))
		nestedEditable(t, f.a, newHost, f.rel)
		oneSiteMaySave(t, f.a, f.cdoc, f.child)

		// The page that was open at the old address can no longer save there.
		stale := servedRequest(t, f.a, http.MethodPost, f.aroundHost, "/_/save/"+f.oldToken, "<html><body>from the old address</body></html>", map[string]string{
			"Content-Type":   "text/html",
			"Origin":         "http://" + f.aroundHost,
			"Sec-Fetch-Site": "same-origin",
		})
		if stale.Code == http.StatusOK {
			t.Fatalf("the old address still saved the document: %d %s", stale.Code, stale.Body.String())
		}
		if written, err := os.ReadFile(f.cdoc); err != nil || string(written) != "<html><body>c</body></html>" {
			t.Fatalf("the document after a save at the old address = %q, %v", written, err)
		}
		if around.srv.Core.HelpersBound(f.cdoc) {
			t.Fatal("the old address still holds the program channel of a document that left it")
		}

		saveAt(t, f.a, newHost, f.rel, f.cdoc, "<html><body>saved at the new address</body></html>")
		saveAt(t, f.a, f.aroundHost, f.address(f.pdoc), f.pdoc, "<html><body>the folder around still saves</body></html>")
		oneSiteMaySave(t, f.a, f.pdoc, f.parent)

		// The folder is listed under the folder around it, which is what offers Merge.
		snapshot := f.a.snapshot()
		if got, want := snapshotFolderAt(t, snapshot, f.child).ParentID, snapshotFolderAt(t, snapshot, f.parent).ID; got == nil || *got != want {
			t.Fatalf("the folder inside has parent %v, want the folder around it %q", got, want)
		}
	})

	t.Run("a dirty page that cannot flush stops it with unsaved-edits and the config is unchanged", func(t *testing.T) {
		f := newInsideFixture(t)
		before := f.configBytes(t)
		f.a.service = unsavedService(t, f.a)

		refusal := f.trust(t, f.child, warnInsideTrusted)
		if refusal == nil || refusal.Code != appcore.RefuseUnsavedEdits {
			t.Fatalf("a trust with a page that cannot save = %+v, want %s", refusal, appcore.RefuseUnsavedEdits)
		}
		if len(refusal.Documents) == 0 {
			t.Fatal("the refusal names no document")
		}
		f.nothingMoved(t, before)
	})

	t.Run("the check answers inside-trusted for a nested folder and not for a sibling that shares a prefix", func(t *testing.T) {
		home := evalTempHome(t)
		notes := filepath.Join(home, "a", "notes")
		nested := filepath.Join(notes, "drafts")
		deeper := filepath.Join(nested, "week")
		sibling := filepath.Join(home, "a", "notes-old")
		for _, dir := range []string{deeper, sibling} {
			writeTestFile(t, filepath.Join(dir, "index.htmlclay"), servedTestDocument)
		}
		cfgBase := t.TempDir()
		a := buildTestApp(t, home, cfgBase)
		if err := a.trustFolder(notes); err != nil {
			t.Fatalf("trustFolder(%s): %v", notes, err)
		}
		listed := a.rt.cfg.TrustedFolderList()
		onDisk, err := os.ReadFile(filepath.Join(config.DirFrom(cfgBase), "config.json"))
		if err != nil {
			t.Fatal(err)
		}

		for _, dir := range []string{nested, deeper} {
			preview, warning := trustCheck(t, a, dir)
			if warning == nil {
				t.Fatalf("Check of %s = %+v, want an inside-trusted warning", dir, preview.Warnings)
			}
			if len(warning.Names) != 1 || warning.Names[0] != "notes" {
				t.Fatalf("inside-trusted for %s names %v, want the folder around it, notes", dir, warning.Names)
			}
			if want := fmt.Sprintf(appcore.TextWarnInsideTrusted, "notes"); warning.Text != want {
				t.Fatalf("inside-trusted text = %q, want %q", warning.Text, want)
			}
		}
		if preview, warning := trustCheck(t, a, sibling); warning != nil || len(preview.Warnings) != 0 {
			t.Fatalf("Check of the sibling %s = %+v, want no warning", sibling, preview.Warnings)
		}
		if preview, warning := trustCheck(t, a, notes); warning != nil {
			t.Fatalf("Check of the trusted folder itself = %+v, want no inside-trusted", preview.Warnings)
		}
		if preview, warning := trustCheck(t, a, filepath.Join(home, "a")); warning != nil {
			t.Fatalf("Check of the folder around the trusted one = %+v, want no inside-trusted", preview.Warnings)
		}

		after := a.rt.cfg.TrustedFolderList()
		if len(after) != len(listed) || after[0] != listed[0] {
			t.Fatalf("trusted folders = %+v after the checks, want %+v unchanged", after, listed)
		}
		for _, dir := range []string{nested, deeper, sibling} {
			if slug := a.rt.cfg.SiteSlug(dir); slug != "" {
				t.Fatalf("a check gave %s the address %s", dir, slug)
			}
		}
		if written, err := os.ReadFile(filepath.Join(config.DirFrom(cfgBase), "config.json")); err != nil || string(written) != string(onDisk) {
			t.Fatalf("a check rewrote the config: %s, %v", written, err)
		}
	})

	t.Run("a failed config write changes nothing", func(t *testing.T) {
		f := newInsideFixture(t)
		before := f.configBytes(t)
		cfgDir := config.DirFrom(f.cfgBase)
		allowWrites := testutil.RefuseReplace(t, filepath.Join(cfgDir, "config.json"))

		refusal := f.trust(t, f.child, warnInsideTrusted)
		allowWrites()
		if refusal == nil || refusal.Code != appcore.RefuseFailed {
			t.Fatalf("a trust the config write could not record = %+v, want %s", refusal, appcore.RefuseFailed)
		}
		f.nothingMoved(t, before)

		// The failed try left no entry and no address in memory either, so the
		// same trust goes through once the config can be written.
		if refusal := f.trust(t, f.child, warnInsideTrusted); refusal != nil {
			t.Fatalf("the trust after the failed write refused: %+v", refusal)
		}
		nestedEditable(t, f.a, nestedHost(f.a.rt.cfg.SiteSlug(f.child)), f.rel)
	})

	t.Run("a restart keeps the new address and the old address still redirects", func(t *testing.T) {
		f := newInsideFixture(t)
		if refusal := f.trust(t, f.child, warnInsideTrusted); refusal != nil {
			t.Fatalf("the acknowledged trust refused: %+v", refusal)
		}
		newHost := nestedHost(f.a.rt.cfg.SiteSlug(f.child))
		f.a.shutdown()

		second := f.launch(t)
		if !ownAddress(t, second, f.child) {
			t.Fatal("the config on disk does not give the folder inside its own address")
		}
		nestedEditable(t, second, newHost, f.rel)
		nestedRedirect(t, second, f.aroundHost, f.rel, fileURL(newHost, f.rel))
		oneSiteMaySave(t, second, f.cdoc, f.child)
	})

	t.Run("a folder with no document open answers at its new address at once", func(t *testing.T) {
		f := newNestedFixture(t)
		a := f.launch(t)
		if err := a.trustFolder(f.parent); err != nil {
			t.Fatalf("trust the folder around: %v", err)
		}
		sent := 0
		if refusal := trustAction(t, a, &sent, f.child, warnInsideTrusted); refusal != nil {
			t.Fatalf("the acknowledged trust refused: %+v", refusal)
		}
		site := liveSite(t, a, f.child)
		if want := nestedHost(a.rt.cfg.SiteSlug(f.child)); site.host != want {
			t.Fatalf("the folder's site answers at %s, want %s", site.host, want)
		}
		nestedEditable(t, a, site.host, f.address(f.cdoc))
		oneSiteMaySave(t, a, f.cdoc, f.child)
	})

	t.Run("every file's bytes are identical before and after", func(t *testing.T) {
		f := newInsideFixture(t)
		before := fileDigests(t, f.home)
		if refusal := f.trust(t, f.child, warnInsideTrusted); refusal != nil {
			t.Fatalf("the acknowledged trust refused: %+v", refusal)
		}
		after := fileDigests(t, f.home)
		if len(after) != len(before) {
			t.Fatalf("the trust added or removed files: %d before, %d after", len(before), len(after))
		}
		for path, sum := range before {
			if after[path] != sum {
				t.Fatalf("%s changed during the trust", path)
			}
		}
	})

	t.Run("a folder already on the list is approved again without the warning and keeps its address", func(t *testing.T) {
		f := newNestedFixture(t)
		// Both folders listed, neither keeping an address: every configuration
		// written before nested folders kept one.
		f.config(t, f.bothAddresses())
		a := f.launch(t)
		sent := 0

		if preview, warning := trustCheck(t, a, f.child); warning != nil {
			t.Fatalf("Check of a folder already trusted = %+v, want no inside-trusted", preview.Warnings)
		}
		if refusal := trustAction(t, a, &sent, f.child); refusal != nil {
			t.Fatalf("approving a listed folder again refused: %+v", refusal)
		}
		if ownAddress(t, a, f.child) {
			t.Fatal("approving a listed folder again gave it its own address")
		}
		oneSiteAnchoredAt(t, a, f.parent)
		nestedEditable(t, a, nestedHost("htmlclay2"), f.address(f.cdoc))
	})

	t.Run("a folder around that is no longer the one that was trusted asks for no acknowledgement", func(t *testing.T) {
		f := newInsideFixture(t)
		if _, ok := f.a.rt.cfg.SetTrustedIdentity(f.parent, "not-the-folder-on-disk"); !ok {
			t.Fatal("the folder around is not on the trusted list")
		}

		if preview, warning := trustCheck(t, f.a, f.child); warning != nil {
			t.Fatalf("Check inside a folder that failed its identity = %+v, want no inside-trusted", preview.Warnings)
		}
		if refusal := f.trust(t, f.child); refusal != nil {
			t.Fatalf("a trust inside a folder that failed its identity refused: %+v", refusal)
		}
		if _, listed := f.a.trustedEntry(f.child); !listed {
			t.Fatal("the folder was not trusted")
		}
	})

	t.Run("a folder inside a served folder is trusted without the warning and gets no address", func(t *testing.T) {
		home := evalTempHome(t)
		served := filepath.Join(home, "notes")
		inner := filepath.Join(served, "inner")
		writeTestFile(t, filepath.Join(inner, "index.html"), servedTestDocument)
		a := newServedTestApp(t, home, t.TempDir())
		if _, refusal, err := a.serveFolder(served, "notes"); err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		if _, listed := a.trustedEntry(served); !listed {
			t.Fatal("serving the folder did not trust it, so this subtest proves nothing")
		}
		sent := 0

		if preview, warning := trustCheck(t, a, inner); warning != nil {
			t.Fatalf("Check inside a served folder = %+v, want no inside-trusted", preview.Warnings)
		}
		if refusal := trustAction(t, a, &sent, inner); refusal != nil {
			t.Fatalf("a trust inside a served folder refused: %+v", refusal)
		}
		if ownAddress(t, a, inner) {
			t.Fatal("a folder inside a served folder was given its own address")
		}
	})

	t.Run("an acknowledgement nobody was asked for does not give a folder its own address", func(t *testing.T) {
		f := newInsideFixture(t)
		other := filepath.Join(f.home, "other")
		writeTestFile(t, filepath.Join(other, "o.htmlclay"), "<html><body>o</body></html>")

		if refusal := f.trust(t, other, warnInsideTrusted); refusal != nil {
			t.Fatalf("a plain trust that carried the acknowledgement refused: %+v", refusal)
		}
		if ownAddress(t, f.a, other) {
			t.Fatal("a folder inside nothing was marked as keeping its own address")
		}
	})
}
