package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

// handoffFixture is one app's worth of open documents: two files in a trusted
// folder and one file opened ad hoc with a read grant beside it, every token and
// host captured, and the app shut down. The handoff, when there is one, is
// written before the shutdown exactly as the updater will write it. Both apps
// keep their tokens in one save-tokens.json, as one install does.
type handoffFixture struct {
	home, cfgBase          string
	page, other, loose     string
	assets                 string
	pageTok, otherTok      string
	looseTok               string
	trustedHost, looseHost string
	handoff                string
	kept                   string
	port                   int
}

// handoffOpts changes how the fixture's first app ends.
type handoffOpts struct {
	// beforeQuit runs on the first app once every document is open and every
	// token captured.
	beforeQuit func(t *testing.T, first *app, fx handoffFixture)
	// crash ends the first app without the record's final write at quit, the way
	// a forced quit or a power loss does, once the record has caught up.
	crash bool
}

func newHandoffFixture(t *testing.T, targetVersion string) handoffFixture {
	t.Helper()
	return newHandoffFixtureWith(t, targetVersion, handoffOpts{})
}

func newHandoffFixtureWith(t *testing.T, targetVersion string, opts handoffOpts) handoffFixture {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	fx := handoffFixture{
		home:    home,
		cfgBase: t.TempDir(),
		page:    filepath.Join(home, "proj", "index.htmlclay"),
		other:   filepath.Join(home, "proj", "other.htmlclay"),
		loose:   filepath.Join(home, "notes", "loose.htmlclay"),
		assets:  filepath.Join(home, "assets"),
		handoff: filepath.Join(t.TempDir(), updateHandoffFile),
		kept:    filepath.Join(t.TempDir(), saveTokensFile),
	}
	writeTestFile(t, fx.page, "<html><body>index</body></html>")
	writeTestFile(t, fx.other, "<html><body>other</body></html>")
	writeTestFile(t, fx.loose, "<html><body>loose</body></html>")
	writeTestFile(t, filepath.Join(fx.assets, "logo.txt"), "logo")

	first := newTestAppWithConfigDir(t, home, fx.cfgBase)
	first.startKeptTokens(fx.kept)
	if err := first.trustFolder(filepath.Join(home, "proj")); err != nil {
		t.Fatalf("trust: %v", err)
	}
	trusted, _ := first.openForTest(t, fx.page)
	if again, _ := first.openForTest(t, fx.other); again != trusted {
		t.Fatal("both trusted files should share one site")
	}
	adHoc, _ := first.openForTest(t, fx.loose)
	if err := adHoc.sessions.GrantCanonicalRoot(fx.assets); err != nil {
		t.Fatalf("grant: %v", err)
	}
	fx.trustedHost, fx.looseHost = trusted.host, adHoc.host
	fx.pageTok = tokenFor(t, trusted, fx.page)
	fx.otherTok = tokenFor(t, trusted, fx.other)
	fx.looseTok = tokenFor(t, adHoc, fx.loose)
	fx.port = first.rt.port

	if opts.beforeQuit != nil {
		opts.beforeQuit(t, first, fx)
	}
	if targetVersion != "" {
		if err := first.writeUpdateHandoff(fx.handoff, targetVersion); err != nil {
			t.Fatalf("write handoff: %v", err)
		}
	}
	if opts.crash {
		waitKeptCurrent(t, first, fx.kept)
		first.tokens.Swap(nil).halt()
	}
	first.shutdown()
	requirePortFree(t, fx.port)
	return fx
}

func readKept(path string) (savedTokens, error) {
	var rec savedTokens
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &rec)
	}
	return rec, err
}

// waitKeptCurrent waits until the record on disk names exactly the live
// registrations, each with its file's identity now: what a crash at this moment
// leaves behind.
func waitKeptCurrent(t *testing.T, a *app, path string) {
	t.Helper()
	want := map[string]string{}
	a.mu.Lock()
	sites := append([]*site(nil), a.sites...)
	a.mu.Unlock()
	for _, s := range sites {
		for _, r := range s.sessions.Snapshot().Registrations {
			want[r.Token] = r.Path
		}
	}
	waitFor(t, "the kept record to match the live sessions", func() bool {
		rec, err := readKept(path)
		if err != nil {
			return false
		}
		got := map[string]string{}
		for _, site := range rec.Sites {
			for _, f := range site.Files {
				if f.Identity != platform.DirIdentity(f.Path) {
					return false
				}
				got[f.Token] = f.Path
			}
		}
		return maps.Equal(got, want)
	})
}

func editKept(t *testing.T, path string, edit func(*savedTokens)) {
	t.Helper()
	rec, err := readKept(path)
	if err != nil {
		t.Fatal(err)
	}
	edit(&rec)
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func tokenFor(t *testing.T, s *site, path string) string {
	t.Helper()
	f, ok := s.sessions.LookupByPath(path)
	if !ok {
		t.Fatalf("%s is not registered on %s", path, s.host)
	}
	return f.Token
}

// restart starts a fresh app on the same config, the way startup_test does, and
// runs the startup steps main runs before argv: the trusted sites, then the
// handoff.
func (fx handoffFixture) restart(t *testing.T) (*app, int, error) {
	t.Helper()
	second := newTestAppWithConfigDir(t, fx.home, fx.cfgBase)
	if second.rt.port != fx.port {
		t.Fatalf("the port moved across the restart: %d, want %d", second.rt.port, fx.port)
	}
	second.startSites()
	n, err := second.restoreUpdateHandoff(fx.handoff)
	second.startKeptTokens(fx.kept)
	return second, n, err
}

func saveWith(t *testing.T, host, token, body string) (int, string) {
	t.Helper()
	return postSameOrigin(t, "http://"+host+"/_/save/"+token, "text/html", body)
}

func requireGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the handoff must be deleted on read, stat err = %v", err)
	}
}

// The acceptance test for the update handoff: tabs open before an update
// restart keep saving afterwards with the tokens they already hold, at the
// addresses they are already on, for a trusted folder and for an ad-hoc file,
// and the ad-hoc file's read grant comes back with it.
func TestUpdateHandoffKeepsOpenTabsSaving(t *testing.T) {
	fx := newHandoffFixture(t, version)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(fx.handoff)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("the handoff holds save tokens and must be 0600, got %o", perm)
		}
	}

	second, n, err := fx.restart(t)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if n != 3 {
		t.Fatalf("restored %d registrations, want 3", n)
	}
	requireGone(t, fx.handoff)

	for _, c := range []struct{ host, token, path, body string }{
		{fx.trustedHost, fx.pageTok, fx.page, "<html><body>index, after the update</body></html>"},
		{fx.trustedHost, fx.otherTok, fx.other, "<html><body>other, after the update</body></html>"},
		{fx.looseHost, fx.looseTok, fx.loose, "<html><body>loose, after the update</body></html>"},
	} {
		if code, out := saveWith(t, c.host, c.token, c.body); code != 200 {
			t.Fatalf("save to %s with the old token = %d: %s", c.host, code, out)
		}
		saved, _ := os.ReadFile(c.path)
		if !strings.Contains(string(saved), "after the update") {
			t.Fatalf("the save to %s did not reach disk", c.path)
		}
	}

	s := liveSiteWithHost(t, second, fx.looseHost)
	if _, _, ok := s.sessions.AssetRoot(filepath.Join(fx.assets, "logo.txt")); !ok {
		t.Fatal("the ad-hoc site's read grant did not come back")
	}
}

func liveSiteWithHost(t *testing.T, a *app, host string) *site {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sites {
		if s.host == host {
			return s
		}
	}
	t.Fatalf("no live site at %s", host)
	return nil
}

// An ordinary restart keeps open tabs saving, whether the app quit or died:
// the tokens kept in save-tokens.json come back with their checks, a trusted
// folder's on its first request and an ad-hoc file's when a request reaches its
// address. A read grant does not come back: "Allow once" lasts until the
// person quits. Nothing is rebuilt before a request asks, so no
// folder is held open for a tab nobody still has.
func TestOrdinaryRestartKeepsOpenTabsSaving(t *testing.T) {
	for _, crash := range []bool{false, true} {
		name := "quit"
		if crash {
			name = "crash"
		}
		t.Run(name, func(t *testing.T) {
			fx := newHandoffFixtureWith(t, "", handoffOpts{crash: crash})
			if runtime.GOOS != "windows" {
				info, err := os.Stat(fx.kept)
				if err != nil {
					t.Fatal(err)
				}
				if perm := info.Mode().Perm(); perm != 0600 {
					t.Fatalf("the record holds save tokens and must be 0600, got %o", perm)
				}
			}

			second, n, err := fx.restart(t)
			if err != nil || n != 0 {
				t.Fatalf("restore with no handoff = (%d, %v), want (0, nil)", n, err)
			}
			looseSlug, _, _ := strings.Cut(fx.looseHost, ".")
			second.hostMu.RLock()
			_, built := second.bySlug[looseSlug]
			second.hostMu.RUnlock()
			if built {
				t.Fatal("the ad-hoc site was rebuilt before any request reached it")
			}

			for _, c := range []struct{ host, token, path string }{
				{fx.trustedHost, fx.pageTok, fx.page},
				{fx.trustedHost, fx.otherTok, fx.other},
				{fx.looseHost, fx.looseTok, fx.loose},
			} {
				if code, out := saveWith(t, c.host, c.token, "<html><body>after the restart</body></html>"); code != 200 {
					t.Fatalf("save to %s with the kept token = %d: %s", c.host, code, out)
				}
				saved, _ := os.ReadFile(c.path)
				if !strings.Contains(string(saved), "after the restart") {
					t.Fatalf("the save to %s did not reach disk", c.path)
				}
			}
			s := liveSiteWithHost(t, second, fx.looseHost)
			if _, _, ok := s.sessions.AssetRoot(filepath.Join(fx.assets, "logo.txt")); ok {
				t.Fatal("the read grant came back after a quit; Allow once lasts until the person quits")
			}
		})
	}
}

// Stop trusting ends a kept token at once: the record drops the folder's tokens
// before any quit, so a crash right after cannot bring them back, and the old
// address no longer saves.
func TestStopTrustingEndsKeptTokens(t *testing.T) {
	fx := newHandoffFixtureWith(t, "", handoffOpts{crash: true, beforeQuit: func(t *testing.T, first *app, fx handoffFixture) {
		if err := first.untrustFolder(filepath.Join(fx.home, "proj")); err != nil {
			t.Fatalf("untrust: %v", err)
		}
	}})
	rec, err := readKept(fx.kept)
	if err != nil {
		t.Fatal(err)
	}
	for _, site := range rec.Sites {
		for _, f := range site.Files {
			if f.Token == fx.pageTok || f.Token == fx.otherTok {
				t.Fatalf("the record still holds a token of the untrusted folder for %s", f.Path)
			}
		}
	}
	fx.restart(t)
	if code, _ := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>x</body></html>"); code != 404 {
		t.Fatalf("the untrusted folder's old address must not save after a restart, got %d", code)
	}
}

// A kept token belongs to the file the tab opened. A file replaced while the
// app was down is refused; the rest of its folder still saves.
func TestKeptTokenRefusedForAReplacedFile(t *testing.T) {
	fx := newHandoffFixture(t, "")
	replacement := fx.page + ".new"
	writeTestFile(t, replacement, "<html><body>replaced</body></html>")
	if err := os.Rename(replacement, fx.page); err != nil {
		t.Fatal(err)
	}
	fx.restart(t)
	if code, _ := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>x</body></html>"); code != 401 {
		t.Fatalf("a replaced file's kept token must be refused, got %d", code)
	}
	if code, out := saveWith(t, fx.trustedHost, fx.otherTok, "<html><body>x</body></html>"); code != 200 {
		t.Fatalf("the folder's other file must still save, got %d: %s", code, out)
	}
}

// Every save replaces the file through a temp file and a rename, so its identity
// changes, and the record follows it: a crash after a save still restores that tab.
func TestKeptTokenFollowsASaveAcrossACrash(t *testing.T) {
	fx := newHandoffFixtureWith(t, "", handoffOpts{crash: true, beforeQuit: func(t *testing.T, first *app, fx handoffFixture) {
		// Settle the writes the opens queued, so only the save can bring the record
		// up to date before the crash.
		waitKeptCurrent(t, first, fx.kept)
		before := platform.DirIdentity(fx.page)
		if code, out := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>saved before the crash</body></html>"); code != 200 {
			t.Fatalf("save = %d: %s", code, out)
		}
		if platform.DirIdentity(fx.page) == before {
			t.Fatal("the save kept the file's identity, so this test proves nothing")
		}
	}})
	fx.restart(t)
	if code, out := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>after the crash</body></html>"); code != 200 {
		t.Fatalf("save after the crash = %d: %s", code, out)
	}
}

// A kept token lasts keptUnusedDays from its last use, and only on the port its
// addresses were on.
func TestKeptTokensExpireAndNeedTheSamePort(t *testing.T) {
	t.Run("unused too long", func(t *testing.T) {
		fx := newHandoffFixture(t, "")
		old := dayString(session.UnixDay(time.Now()) - keptUnusedDays - 1)
		editKept(t, fx.kept, func(rec *savedTokens) {
			for i := range rec.Sites {
				for j := range rec.Sites[i].Files {
					if rec.Sites[i].Files[j].Path == fx.loose {
						rec.Sites[i].Files[j].Used = old
					}
				}
			}
		})
		fx.restart(t)
		if code, _ := saveWith(t, fx.looseHost, fx.looseTok, "<html><body>x</body></html>"); code != 404 {
			t.Fatalf("a token unused for %d days must not come back, got %d", keptUnusedDays+1, code)
		}
		if code, out := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>x</body></html>"); code != 200 {
			t.Fatalf("a recently used token must come back, got %d: %s", code, out)
		}
	})
	t.Run("another port", func(t *testing.T) {
		fx := newHandoffFixture(t, "")
		editKept(t, fx.kept, func(rec *savedTokens) { rec.Port++ })
		fx.restart(t)
		if code, _ := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>x</body></html>"); code != 401 {
			t.Fatalf("a record for another port must restore nothing, trusted save = %d", code)
		}
		if code, _ := saveWith(t, fx.looseHost, fx.looseTok, "<html><body>x</body></html>"); code != 404 {
			t.Fatalf("a record for another port must restore nothing, ad-hoc save = %d", code)
		}
	})
}

// A handoff that is too old, or written for another version, is deleted and
// ignored whole: nothing it names comes back.
func TestStaleOrMismatchedHandoffIsIgnoredAndDeleted(t *testing.T) {
	cases := []struct {
		name   string
		target string
		age    time.Duration
	}{
		{"stale", version, 2 * updateHandoffMaxAge},
		{"other version", version + "-other", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := newHandoffFixture(t, c.target)
			if c.age > 0 {
				backdateHandoff(t, fx.handoff, c.age)
			}
			// This tests the handoff alone; the kept record would restore the same tokens.
			if err := os.Remove(fx.kept); err != nil {
				t.Fatal(err)
			}
			_, n, err := fx.restart(t)
			if err == nil || n != 0 {
				t.Fatalf("restore = (%d, %v), want the handoff refused", n, err)
			}
			requireGone(t, fx.handoff)
			if code, _ := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>x</body></html>"); code != 401 {
				t.Fatalf("a refused handoff restored a trusted token, save = %d", code)
			}
			if code, _ := saveWith(t, fx.looseHost, fx.looseTok, "<html><body>x</body></html>"); code != 404 {
				t.Fatalf("a refused handoff restored the ad-hoc site, save = %d", code)
			}
		})
	}
}

func backdateHandoff(t *testing.T, path string, by time.Duration) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var h updateHandoff
	if err := json.Unmarshal(data, &h); err != nil {
		t.Fatal(err)
	}
	h.Created = h.Created.Add(-by)
	out, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		t.Fatal(err)
	}
}

// A file deleted while the app was down is skipped, and everything else still
// comes back. A site whose every file is gone is not published at all, so its
// address keeps answering the recovery page.
func TestHandoffSkipsADeletedFileAndRestoresTheRest(t *testing.T) {
	fx := newHandoffFixture(t, version)
	for _, gone := range []string{fx.other, fx.loose} {
		if err := os.Remove(gone); err != nil {
			t.Fatal(err)
		}
	}

	second, n, err := fx.restart(t)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if n != 1 {
		t.Fatalf("restored %d registrations, want 1", n)
	}
	if code, out := saveWith(t, fx.trustedHost, fx.pageTok, "<html><body>index, kept</body></html>"); code != 200 {
		t.Fatalf("the surviving file should still save, got %d: %s", code, out)
	}
	if code, _ := saveWith(t, fx.trustedHost, fx.otherTok, "<html><body>x</body></html>"); code != 401 {
		t.Fatalf("the deleted file's token must not come back, got %d", code)
	}
	if code, _ := saveWith(t, fx.looseHost, fx.looseTok, "<html><body>x</body></html>"); code != 404 {
		t.Fatalf("a site with no surviving file must not be published, got %d", code)
	}
	if _, err := os.Stat(fx.loose); !os.IsNotExist(err) {
		t.Fatal("a refused save recreated the deleted file")
	}
	second.mu.Lock()
	defer second.mu.Unlock()
	if len(second.sites) != 1 {
		t.Fatalf("want only the trusted folder's site live, got %d", len(second.sites))
	}
}

// helperUpdateFixture is a trusted folder holding one page that declares a
// helper the person already allowed, with that helper's dispatcher attached the
// way a normal open attaches it, and a handoff written for the version that
// starts after the update.
type helperUpdateFixture struct {
	home, cfgBase, document, handoff, marker string
	token, host                              string
	port                                     int
}

func newHelperUpdateFixture(t *testing.T) helperUpdateFixture {
	t.Helper()
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, "proj")
	if err := os.MkdirAll(folder, 0755); err != nil {
		t.Fatal(err)
	}
	fx := helperUpdateFixture{
		home:     home,
		cfgBase:  t.TempDir(),
		document: writeHelperPage(t, folder, "search"),
		handoff:  filepath.Join(t.TempDir(), updateHandoffFile),
	}
	programPath, marker := attachmentProgram(t, "restored-search")
	fx.marker = marker

	first := newTestAppWithConfigDir(t, home, fx.cfgBase)
	if err := first.trustFolder(folder); err != nil {
		t.Fatalf("trust: %v", err)
	}
	program, err := first.rt.cfg.AddHelperProgram("search", programPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.rt.cfg.DecideHelper(config.HelperDecision{Document: fx.document, Name: "search", Program: program.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	if err := first.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s, _ := first.openForTest(t, fx.document)
	first.applyHelperPlan(s.srv.Core, fx.document, first.helpersForNavigation(fx.document))
	fx.token, fx.host, fx.port = tokenFor(t, s, fx.document), s.host, first.rt.port
	if err := first.writeUpdateHandoff(fx.handoff, version); err != nil {
		t.Fatalf("write handoff: %v", err)
	}
	first.shutdown()
	requirePortFree(t, fx.port)
	return fx
}

// A page left open across an update keeps its helper dispatcher, not just its
// save token: the restored registration runs the program the person already
// allowed with the old token, and neither reopening nor a dialog is needed.
func TestUpdateHandoffRestoresHelpers(t *testing.T) {
	t.Run("restored page runs its allowed program", func(t *testing.T) {
		fx := newHelperUpdateFixture(t)
		second := newTestAppWithConfigDir(t, fx.home, fx.cfgBase)
		second.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
			t.Fatal("restoring the update handoff raised the helper setup dialog")
			return platform.ChoiceNone, nil
		}
		second.startSites()
		n, err := second.restoreUpdateHandoff(fx.handoff)
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		if n != 1 {
			t.Fatalf("restored %d registrations, want 1", n)
		}
		s := liveSiteWithHost(t, second, fx.host)
		if got := tokenFor(t, s, fx.document); got != fx.token {
			t.Fatalf("the restored registration has token %q, want the original %q", got, fx.token)
		}
		if !s.srv.HelpersBound(fx.document) {
			t.Fatal("the restored registration has no helper dispatcher")
		}
		if state := helperDiscoveryState(t, s, fx.document); state != "ready" {
			t.Fatalf("helper state %q after restore, want ready", state)
		}
		terminal := attachmentRequestTerminal(t, s, fx.document, fx.token, "restored-helper-request")
		if terminal.Type != "wire/done" || terminal.Payload.Program != "restored-search" {
			t.Fatalf("the restored page could not use its program: %+v", terminal)
		}
		if _, err := os.Stat(fx.marker); err != nil {
			t.Fatalf("the restored program did not execute: %v", err)
		}
	})

	t.Run("revoked grant stays revoked", func(t *testing.T) {
		fx := newHelperUpdateFixture(t)
		second := newTestAppWithConfigDir(t, fx.home, fx.cfgBase)
		second.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
			t.Fatal("restoring the update handoff raised the helper setup dialog")
			return platform.ChoiceNone, nil
		}
		if _, _, err := second.rt.cfg.RevokeHelper(fx.document, "search"); err != nil {
			t.Fatal(err)
		}
		if err := second.rt.cfg.Save(); err != nil {
			t.Fatal(err)
		}
		second.startSites()
		n, err := second.restoreUpdateHandoff(fx.handoff)
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		if n != 1 {
			t.Fatalf("restored %d registrations, want 1", n)
		}
		s := liveSiteWithHost(t, second, fx.host)
		if state := helperDiscoveryState(t, s, fx.document); state != "denied" {
			t.Fatalf("helper state %q after restore, want denied", state)
		}
		if _, err := os.Stat(fx.marker); !os.IsNotExist(err) {
			t.Fatalf("the revoked program ran during restore or stat failed: %v", err)
		}
	})
}

// Restarts where something changed before a dormant site's first request.
type keptRestart struct {
	home, cfgBase, kept string
	port                int
}

func newKeptRestart(t *testing.T) (*app, keptRestart) {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	r := keptRestart{home: home, cfgBase: t.TempDir(), kept: filepath.Join(t.TempDir(), saveTokensFile)}
	a := newTestAppWithConfigDir(t, home, r.cfgBase)
	a.startKeptTokens(r.kept)
	r.port = a.rt.port
	return a, r
}

func (r keptRestart) again(t *testing.T) *app {
	t.Helper()
	second := newTestAppWithConfigDir(t, r.home, r.cfgBase)
	if second.rt.port != r.port {
		t.Fatalf("port moved: %d want %d", second.rt.port, r.port)
	}
	second.startSites()
	second.restoreUpdateHandoff(filepath.Join(t.TempDir(), updateHandoffFile))
	second.startKeptTokens(r.kept)
	return second
}

func sitesHolding(t *testing.T, a *app, path string) []string {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	var hosts []string
	for _, s := range a.sites {
		if _, ok := s.sessions.LookupByPath(path); ok {
			hosts = append(hosts, s.host)
		}
	}
	return hosts
}

// A file in a subfolder of an ad-hoc site, opened again after a restart before
// the old tab's first request, comes back at its old address with its old token,
// as it would with no restart: one file, one site, one token.
func TestKeptTokenFollowsAFileOpenedAgain(t *testing.T) {
	first, r := newKeptRestart(t)
	top := filepath.Join(r.home, "notes", "top.htmlclay")
	deep := filepath.Join(r.home, "notes", "sub", "deep.htmlclay")
	writeTestFile(t, top, "<html><body>top</body></html>")
	writeTestFile(t, deep, "<html><body>deep</body></html>")

	s1, _ := first.openForTest(t, top)
	s2, _ := first.openForTest(t, deep)
	if s1 != s2 {
		t.Fatalf("precondition: deep landed on its own site (%s vs %s)", s1.host, s2.host)
	}
	if h := sitesHolding(t, first, deep); len(h) != 1 {
		t.Fatalf("precondition: live app holds deep in %v", h)
	}
	oldHost, oldTok := s1.host, tokenFor(t, s1, deep)
	first.shutdown()
	requirePortFree(t, r.port)

	second := r.again(t)
	s3, _ := second.openForTest(t, deep)
	if s3.host != oldHost || tokenFor(t, s3, deep) != oldTok {
		t.Fatalf("reopened deep at %s with a new token, want its old address %s and old token", s3.host, oldHost)
	}
	if code, out := saveWith(t, oldHost, oldTok, "<html><body>old tab</body></html>"); code != 200 {
		t.Fatalf("old tab save at %s = %d %s", oldHost, code, out)
	}
	if holders := sitesHolding(t, second, deep); len(holders) != 1 {
		t.Fatalf("deep.htmlclay is registered in %d sites %v, want one", len(holders), holders)
	}
}

// Trust-inside-trusted after a restart, before the outer site's first request.
func TestKeptTokenFollowsTrustInside(t *testing.T) {
	first, r := newKeptRestart(t)
	proj := filepath.Join(r.home, "proj")
	inner := filepath.Join(proj, "inner")
	page := filepath.Join(inner, "page.htmlclay")
	writeTestFile(t, page, "<html><body>page</body></html>")
	if err := first.trustFolder(proj); err != nil {
		t.Fatal(err)
	}
	s, _ := first.openForTest(t, page)
	oldHost, oldTok := s.host, tokenFor(t, s, page)
	first.shutdown()
	requirePortFree(t, r.port)

	second := r.again(t)
	n := 0
	if refusal := trustAction(t, second, &n, inner, warnInsideTrusted); refusal != nil {
		t.Fatalf("trust inside = %+v", refusal)
	}
	code, out := saveWith(t, oldHost, oldTok, "<html><body>old tab after the move</body></html>")
	t.Logf("old tab save at the outer address %s = %d %s", oldHost, code, out)
	holders := sitesHolding(t, second, page)
	t.Logf("page is registered in %v", holders)
	if code == 200 {
		t.Errorf("the outer address still saves a document trust-inside moved to its own address")
	}
	again, _ := second.openForTest(t, page)
	t.Logf("a fresh open of the moved document lands at %s (inner folder's own address is %s)", again.host, siteHost(second.rt.cfg.SiteSlug(inner), second.rt.port))
}

// A read grant given by the read prompt's "Trust this folder" button: the grant
// and the trust come from one click, and Stop trusting takes both back from every
// live site (closeSitesLocked). After a restart, it must not come back either.
func keptGrantFixture(t *testing.T) (*app, keptRestart, string, string, string) {
	t.Helper()
	first, r := newKeptRestart(t)
	loose := filepath.Join(r.home, "notes", "loose.htmlclay")
	assets := filepath.Join(r.home, "assets")
	writeTestFile(t, loose, "<html><body>loose</body></html>")
	writeTestFile(t, filepath.Join(assets, "logo.txt"), "logo")
	s, _ := first.openForTest(t, loose)
	if err := s.sessions.GrantCanonicalRoot(assets); err != nil {
		t.Fatal(err)
	}
	if err := first.trustFolder(assets); err != nil {
		t.Fatal(err)
	}
	return first, r, loose, assets, s.host
}

func TestStopTrustingEndsAKeptReadGrant(t *testing.T) {
	first, r, loose, assets, host := keptGrantFixture(t)
	tok := tokenFor(t, liveSiteWithHost(t, first, host), loose)
	first.shutdown()
	requirePortFree(t, r.port)

	second := r.again(t)
	if err := second.untrustFolder(assets); err != nil {
		t.Fatal(err)
	}
	code, out := saveWith(t, host, tok, "<html><body>after</body></html>")
	t.Logf("old tab save = %d %s", code, out)
	s := liveSiteWithHost(t, second, host)
	if _, _, ok := s.sessions.AssetRoot(filepath.Join(assets, "logo.txt")); ok {
		t.Errorf("the read grant on %s came back after Stop trusting took it away", assets)
	}
}

// Trusting an ad-hoc document's own folder after a restart, before its tab's
// first request, keeps the tab saving at the same address, as a running app
// does when it adopts the site in place.
func TestKeptTokenSurvivesTrustingItsOwnFolder(t *testing.T) {
	first, r := newKeptRestart(t)
	proj := filepath.Join(r.home, "proj")
	page := filepath.Join(proj, "page.htmlclay")
	writeTestFile(t, page, "<html><body>page</body></html>")
	s1, _ := first.openForTest(t, page)
	oldHost, oldTok := s1.host, tokenFor(t, s1, page)
	first.shutdown()
	requirePortFree(t, r.port)

	second := r.again(t)
	if err := second.trustFolder(proj); err != nil {
		t.Fatalf("trust: %v", err)
	}
	if code, out := saveWith(t, oldHost, oldTok, "<html><body>after trusting</body></html>"); code != 200 {
		t.Fatalf("the open tab's save after trusting its folder = %d: %s", code, out)
	}
}
