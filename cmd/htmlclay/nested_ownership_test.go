package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
)

// nestedTestPort is the port every app here answers on. Nothing binds it: these
// tests call serveHost directly, so the address in an assertion is the address a
// browser would use without a socket being opened.
const nestedTestPort = 24192

// nestedNav is the request a browser makes for a typed address or a bookmark: a
// top-level document navigation that belongs to no site yet.
var nestedNav = map[string]string{
	"Sec-Fetch-Mode": "navigate",
	"Sec-Fetch-Dest": "document",
	"Sec-Fetch-Site": "none",
}

// nestedFixture is one home with a folder and a folder inside it, and the config
// directory a launch reads. Each subtest fills the config with one of step 7.0e's
// recorded states.
type nestedFixture struct {
	home    string
	parent  string
	child   string
	pdoc    string
	cdoc    string
	cfgBase string
}

func newNestedFixture(t *testing.T) *nestedFixture {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &nestedFixture{
		home:    home,
		parent:  filepath.Join(home, "proj"),
		cfgBase: t.TempDir(),
	}
	f.child = filepath.Join(f.parent, "child")
	f.pdoc = filepath.Join(f.parent, "p.htmlclay")
	f.cdoc = filepath.Join(f.child, "c.htmlclay")
	writeTestFile(t, f.pdoc, "<html><body>p</body></html>")
	writeTestFile(t, f.cdoc, "<html><body>c</body></html>")
	return f
}

// address is the path a document is reached at on any site: home-relative, the
// same whichever folder anchors it.
func (f *nestedFixture) address(abs string) string {
	rel, err := filepath.Rel(f.home, abs)
	if err != nil {
		panic(err)
	}
	return rel
}

// config writes body as this fixture's config.json. The recorded states carry a
// port; it is left out because these apps choose their own address.
func (f *nestedFixture) config(t *testing.T, body string) {
	t.Helper()
	dir := config.DirFrom(f.cfgBase)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// launch is a new process reading this fixture's config: what an upgrade and what
// a restart both are.
func (f *nestedFixture) launch(t *testing.T) *app {
	t.Helper()
	a := buildTestApp(t, f.home, f.cfgBase)
	a.rt.port = nestedTestPort
	t.Cleanup(a.shutdown)
	a.startSites()
	return a
}

// upgradeThenRestart checks the same recorded answers twice: on a build reading
// the config as it was written before this step, and on the process after it,
// which reads the config the first one left behind.
func (f *nestedFixture) upgradeThenRestart(t *testing.T, check func(*testing.T, *app)) {
	t.Helper()
	first := f.launch(t)
	check(t, first)
	first.shutdown()
	second := f.launch(t)
	check(t, second)
}

func nestedHost(slug string) string { return siteHost(slug, nestedTestPort) }

// nestedEditable asserts the document answers editable at host and returns the
// save token it was served with.
func nestedEditable(t *testing.T, a *app, host, rel string) string {
	t.Helper()
	rec := servedRequest(t, a, http.MethodGet, host, "/"+filepath.ToSlash(rel), "", nestedNav)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s at %s = %d, want an editable page (%s)", rel, host, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	const marker = `savetoken="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("GET %s at %s served no save token: %q", rel, host, body)
	}
	rest := body[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

// nestedRedirect asserts a typed navigation at host relocates to want.
func nestedRedirect(t *testing.T, a *app, host, rel, want string) {
	t.Helper()
	rec := servedRequest(t, a, http.MethodGet, host, "/"+filepath.ToSlash(rel), "", nestedNav)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
		t.Fatalf("navigation to %s at %s = %d, %q, want 302 to %q",
			rel, host, rec.Code, rec.Header().Get("Location"), want)
	}
}

// nestedRecovery asserts host holds nothing for rel: a plain request gets the
// recovery page and never relocates.
func nestedRecovery(t *testing.T, a *app, host, rel string) {
	t.Helper()
	rec := servedRequest(t, a, http.MethodGet, host, "/"+filepath.ToSlash(rel), "", nil)
	if rec.Code != http.StatusNotFound || rec.Header().Get("Location") != "" {
		t.Fatalf("GET %s at %s = %d, %q, want the recovery page", rel, host, rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), "Nothing is open at this address") {
		t.Fatalf("GET %s at %s served no recovery page: %q", rel, host, rec.Body.String())
	}
}

// oneSiteAnchoredAt asserts every live site is the one anchored at anchor, so one
// folder has one origin.
func oneSiteAnchoredAt(t *testing.T, a *app, anchor string) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sites {
		if s.anchor != anchor {
			t.Fatalf("%s has a site of its own (%s); only %s should", s.anchor, s.host, anchor)
		}
	}
	if a.siteAtLocked(anchor) == nil {
		t.Fatalf("no live site anchored at %s", anchor)
	}
}

// noAddressRecorded asserts the config gained no ownership record, which is what
// keeps every configuration written before this step routing exactly as it did.
func noAddressRecorded(t *testing.T, a *app) {
	t.Helper()
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.OwnAddress {
			t.Fatalf("%s gained ownAddress; a configuration that never had one must not", tf.Path)
		}
	}
}

// ownAddress reports whether the entry at path keeps its own address.
func ownAddress(t *testing.T, a *app, path string) bool {
	t.Helper()
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path == path {
			return tf.OwnAddress
		}
	}
	t.Fatalf("%s is not a trusted folder", path)
	return false
}

// oneSiteMaySave asserts exactly one live site holds the document, and that it is
// the site anchored at anchor. This is A's guard: a document has one address that
// may save it, never two.
func oneSiteMaySave(t *testing.T, a *app, abs, anchor string) {
	t.Helper()
	if n := registrations(t, a, abs); n != 1 {
		t.Fatalf("%s is registered in %d sites, want exactly 1", abs, n)
	}
	if got, want := hostOf(t, a, abs), liveSite(t, a, anchor); got != want {
		t.Fatalf("%s is held by %s, want the site anchored at %s (%s)", abs, got.host, anchor, want.host)
	}
}

// childOnly is the config of a person who has trusted the child folder alone.
func (f *nestedFixture) childOnly() string {
	return fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay" },
  "retiredSlugs": [], "nextSiteNumber": 2,
  "workspaceFolders": [ { "path": %q } ] }`, f.child, f.child)
}

// bothAddresses is the config child-trusted-then-parent leaves behind: both
// folders have a remembered address, and neither says it owns one.
func (f *nestedFixture) bothAddresses() string {
	return fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay", %q: "htmlclay1" },
  "retiredSlugs": [], "nextSiteNumber": 3,
  "workspaceFolders": [ { "path": %q }, { "path": %q } ] }`,
		f.child, f.parent, f.child, f.parent)
}

// A parent trusted around a folder that is already trusted never moves the inner
// folder's documents: they stay at the address the browser has their storage
// under, and the parent's site neither registers nor saves them. Every
// configuration that predates the ownership record routes exactly as it did.
func TestM3NestedOwnership(t *testing.T) {
	t.Run("state 1: the parent alone", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay" },
  "retiredSlugs": [], "nextSiteNumber": 2,
  "workspaceFolders": [ { "path": %q } ] }`, f.parent, f.parent))

		f.upgradeThenRestart(t, func(t *testing.T, a *app) {
			host := nestedHost("htmlclay")
			if nestedEditable(t, a, host, f.address(f.pdoc)) == nestedEditable(t, a, host, f.address(f.cdoc)) {
				t.Fatal("two documents were served the same save token")
			}
			oneSiteAnchoredAt(t, a, f.parent)
			oneSiteMaySave(t, a, f.pdoc, f.parent)
			oneSiteMaySave(t, a, f.cdoc, f.parent)
			noAddressRecorded(t, a)
		})
	})

	t.Run("state 2: the child trusted first, then the parent", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.childOnly())

		a := f.launch(t)
		if err := a.trustFolder(f.parent); err != nil {
			t.Fatalf("trust the parent: %v", err)
		}

		nestedEditable(t, a, nestedHost("htmlclay1"), f.address(f.pdoc))
		// The child already answered here, so its document stays here rather
		// than being handed to the folder that now encloses it.
		nestedEditable(t, a, nestedHost("htmlclay"), f.address(f.cdoc))
		if !ownAddress(t, a, f.child) {
			t.Fatalf("trusting the parent left %s without its address", f.child)
		}
	})

	t.Run("state 3: both addresses in the config", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.bothAddresses())

		check := func(t *testing.T, a *app) {
			parentHost, childHost := nestedHost("htmlclay1"), nestedHost("htmlclay")
			nestedEditable(t, a, parentHost, f.address(f.pdoc))
			nestedEditable(t, a, parentHost, f.address(f.cdoc))
			nestedRedirect(t, a, childHost, f.address(f.cdoc), fileURL(parentHost, f.address(f.cdoc)))
			nestedRecovery(t, a, childHost, f.address(f.cdoc))
			oneSiteAnchoredAt(t, a, f.parent)
			noAddressRecorded(t, a)
		}
		f.upgradeThenRestart(t, check)
	})

	t.Run("state 4: a child registration when the parent is trusted", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.childOnly())

		a := f.launch(t)
		childHost, parentHost := nestedHost("htmlclay"), nestedHost("htmlclay1")
		before := nestedEditable(t, a, childHost, f.address(f.cdoc))

		if err := a.trustFolder(f.parent); err != nil {
			t.Fatalf("trust the parent: %v", err)
		}

		if after := nestedEditable(t, a, childHost, f.address(f.cdoc)); after != before {
			t.Fatalf("the open child's token changed: %q -> %q", before, after)
		}
		oneSiteMaySave(t, a, f.cdoc, f.child)
		// The parent's own address hands the page back to the child's.
		nestedRedirect(t, a, parentHost, f.address(f.cdoc), fileURL(childHost, f.address(f.cdoc)))
	})

	t.Run("state 5: the same after a restart", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.bothAddresses())

		f.upgradeThenRestart(t, func(t *testing.T, a *app) {
			parentHost, childHost := nestedHost("htmlclay1"), nestedHost("htmlclay")
			nestedEditable(t, a, parentHost, f.address(f.cdoc))
			nestedRedirect(t, a, childHost, f.address(f.cdoc), fileURL(parentHost, f.address(f.cdoc)))
			nestedRecovery(t, a, childHost, f.address(f.cdoc))
			oneSiteAnchoredAt(t, a, f.parent)
			noAddressRecorded(t, a)
		})
	})

	t.Run("state 6: a retired child address", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay1" },
  "retiredSlugs": [ "htmlclay" ], "nextSiteNumber": 3,
  "workspaceFolders": [ { "path": %q }, { "path": %q } ] }`, f.parent, f.child, f.parent))

		f.upgradeThenRestart(t, func(t *testing.T, a *app) {
			parentHost, childHost := nestedHost("htmlclay1"), nestedHost("htmlclay")
			nestedEditable(t, a, parentHost, f.address(f.pdoc))
			nestedEditable(t, a, parentHost, f.address(f.cdoc))
			// A retired label never relocates, not even for a typed address.
			rec := servedRequest(t, a, http.MethodGet, childHost, "/"+filepath.ToSlash(f.address(f.cdoc)), "", nestedNav)
			if rec.Code != http.StatusNotFound || rec.Header().Get("Location") != "" {
				t.Fatalf("navigation to %s at %s = %d, %q, want the recovery page",
					f.address(f.cdoc), childHost, rec.Code, rec.Header().Get("Location"))
			}
			oneSiteAnchoredAt(t, a, f.parent)
			noAddressRecorded(t, a)
		})
	})

	// The step's own case: the child is open at its address, the parent is
	// trusted around it, and a new app on the same config still serves the
	// child's document at that address, and saves it there.
	t.Run("the child keeps its address across a restart", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.childOnly())

		first := f.launch(t)
		childHost := nestedHost("htmlclay")
		token := nestedEditable(t, first, childHost, f.address(f.cdoc))
		if err := first.trustFolder(f.parent); err != nil {
			t.Fatalf("trust the parent: %v", err)
		}
		if !ownAddress(t, first, f.child) {
			t.Fatalf("trusting the parent left %s without its address", f.child)
		}
		first.shutdown()

		second := f.launch(t)
		// The same address, a token minted fresh for this process.
		token = nestedEditable(t, second, childHost, f.address(f.cdoc))
		oneSiteMaySave(t, second, f.cdoc, f.child)

		// The save works at that address, with no hop to another origin.
		body := "<html><body>saved at the child's address</body></html>"
		save := servedRequest(t, second, http.MethodPost, childHost, "/_/save/"+token, body, map[string]string{
			"Content-Type":   "text/html",
			"Origin":         "http://" + childHost,
			"Sec-Fetch-Site": "same-origin",
		})
		if save.Code != http.StatusOK {
			t.Fatalf("saving %s at %s = %d, want 200 (%s)", f.address(f.cdoc), childHost, save.Code, save.Body.String())
		}
		written, err := os.ReadFile(f.cdoc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(written), "saved at the child's address") {
			t.Fatalf("the save at %s did not reach the document: %q", childHost, written)
		}
	})

	// A save for the child's document sent to the parent's address is refused,
	// and names the address it belongs to. One file, one writer.
	t.Run("the parent refuses the child's save", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.childOnly())

		first := f.launch(t)
		if err := first.trustFolder(f.parent); err != nil {
			t.Fatalf("trust the parent: %v", err)
		}
		first.shutdown()

		a := f.launch(t)
		childHost, parentHost := nestedHost("htmlclay"), nestedHost("htmlclay1")
		token := nestedEditable(t, a, childHost, f.address(f.cdoc))
		before, err := os.ReadFile(f.cdoc)
		if err != nil {
			t.Fatal(err)
		}

		write := servedRequest(t, a, http.MethodPost, parentHost, "/_/api/"+filepath.ToSlash(f.address(f.cdoc)), `{}`, map[string]string{
			"Content-Type": "application/json",
		})
		if write.Code == http.StatusOK {
			t.Fatalf("a write for %s at %s was accepted: %s", f.address(f.cdoc), parentHost, write.Body.String())
		}
		if want := fileURL(childHost, f.address(f.cdoc)); !strings.Contains(write.Body.String(), want) {
			t.Fatalf("the refusal = %d %q, want it to name %q", write.Code, write.Body.String(), want)
		}

		save := servedRequest(t, a, http.MethodPost, parentHost, "/_/save/"+token, "<html><body>wrong origin</body></html>", map[string]string{
			"Content-Type":   "text/html",
			"Origin":         "http://" + parentHost,
			"Sec-Fetch-Site": "same-origin",
		})
		if save.Code == http.StatusOK {
			t.Fatalf("a page save for %s at %s was accepted: %s", f.address(f.cdoc), parentHost, save.Body.String())
		}

		after, err := os.ReadFile(f.cdoc)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("a refused save changed %s: %q -> %q", f.cdoc, before, after)
		}
		if n := registrations(t, a, f.cdoc); n != 1 {
			t.Fatalf("%s is registered in %d sites, want exactly 1", f.cdoc, n)
		}
	})

	// One document, one site that may save it, whichever address it is asked on.
	t.Run("every document has one site that may save it", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.childOnly())

		first := f.launch(t)
		if err := first.trustFolder(f.parent); err != nil {
			t.Fatalf("trust the parent: %v", err)
		}
		first.shutdown()

		a := f.launch(t)
		nestedEditable(t, a, nestedHost("htmlclay"), f.address(f.cdoc))
		nestedEditable(t, a, nestedHost("htmlclay1"), f.address(f.pdoc))
		// Asking for the child's document at the parent's address changes nothing:
		// the page moves, the registration does not.
		nestedRedirect(t, a, nestedHost("htmlclay1"), f.address(f.cdoc),
			fileURL(nestedHost("htmlclay"), f.address(f.cdoc)))

		oneSiteMaySave(t, a, f.pdoc, f.parent)
		oneSiteMaySave(t, a, f.cdoc, f.child)
	})

	// The Trust sheet warns about the folders inside the one being trusted, and
	// that warning is unchanged by the address the child keeps.
	t.Run("the Trust sheet check for the parent carries contains-trusted", func(t *testing.T) {
		f := newNestedFixture(t)
		f.config(t, f.childOnly())

		a := f.launch(t)
		backend := shellBackend{a}
		preview, refusal := backend.Check(appcore.Action{
			ID:   "c-parent",
			Type: "folder.trust",
			Args: json.RawMessage(fmt.Sprintf(`{"path":%q}`, f.parent)),
		})
		if refusal != nil {
			t.Fatalf("Check of the folder holding a trusted folder refused: %+v", refusal)
		}
		for _, warning := range preview.Warnings {
			if warning.Code != "contains-trusted" {
				continue
			}
			if len(warning.Names) == 1 && warning.Names[0] == "child" {
				return
			}
		}
		t.Fatalf("warnings = %+v, want a contains-trusted warning naming child", preview.Warnings)
	})
}
