package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// addressFixture is one home and the config directory a launch reads. docs holds
// one document in a folder of its own and other holds a second folder's, so a
// config can give each folder a label.
type addressFixture struct {
	home    string
	cfgBase string
	docs    string
	doc     string
	other   string
	odoc    string
	opened  []string
}

// newAddressFixture makes the home. folder names the first document's folder.
func newAddressFixture(t *testing.T, folder string) *addressFixture {
	t.Helper()
	home := evalTempHome(t)
	f := &addressFixture{
		home:    home,
		cfgBase: t.TempDir(),
		docs:    filepath.Join(home, folder),
		other:   filepath.Join(home, "other"),
	}
	f.doc = filepath.Join(f.docs, "a.htmlclay")
	f.odoc = filepath.Join(f.other, "o.htmlclay")
	writeTestFile(t, f.doc, "<html><body>a</body></html>")
	writeTestFile(t, f.odoc, "<html><body>o</body></html>")
	return f
}

func (f *addressFixture) rel(abs string) string {
	rel, err := filepath.Rel(f.home, abs)
	if err != nil {
		panic(err)
	}
	return rel
}

func (f *addressFixture) configPath() string {
	return filepath.Join(config.DirFrom(f.cfgBase), "config.json")
}

func (f *addressFixture) seed(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(config.DirFrom(f.cfgBase), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.configPath(), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// firstSite is the config a build from before this step left behind: key answers
// at htmlclay.localhost, a second folder has the next label, and a third label
// was retired. trusted lists the folders the person trusted.
func (f *addressFixture) firstSite(key string, trusted ...string) string {
	folders := []string{}
	for _, path := range trusted {
		folders = append(folders, fmt.Sprintf(`{ "path": %q }`, path))
	}
	return fmt.Sprintf(`{ "siteSlugs": { %q: "htmlclay", %q: "htmlclay1" },
  "retiredSlugs": [ "htmlclay2" ], "nextSiteNumber": 4,
  "workspaceFolders": [ %s ] }`, key, f.other, strings.Join(folders, ", "))
}

func (f *addressFixture) bytes(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.configPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// addressConfig is what the tests read back from config.json.
type addressConfig struct {
	SiteSlugs      map[string]string `json:"siteSlugs"`
	RetiredSlugs   []string          `json:"retiredSlugs"`
	NextSiteNumber int               `json:"nextSiteNumber"`
	MovedFirstSite string            `json:"movedFirstSite"`
}

func (f *addressFixture) onDisk(t *testing.T) addressConfig {
	t.Helper()
	var c addressConfig
	if err := json.Unmarshal([]byte(f.bytes(t)), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// upgrade is the first half of a start, as main runs it: the config is loaded
// and the upgrade it implies is finished. Nothing answers yet.
func (f *addressFixture) upgrade(t *testing.T) *app {
	t.Helper()
	a := buildTestApp(t, f.home, f.cfgBase)
	cfg, res, err := config.LoadFrom(f.cfgBase, platform.DirIdentity)
	if err != nil {
		t.Fatal(err)
	}
	a.rt.cfg, a.loaded = cfg, res
	a.rt.port = nestedTestPort
	a.rt.launchBrowser = func(target string) error {
		f.opened = append(f.opened, target)
		return nil
	}
	t.Cleanup(a.shutdown)
	a.finishUpgrade()
	return a
}

// launch is a whole start: upgrade, then the trusted folders' sites and the
// dashboard, in main's order.
func (f *addressFixture) launch(t *testing.T) *app {
	t.Helper()
	a := f.upgrade(t)
	a.startSites()
	a.startServed()
	return a
}

// open opens abs the way a double-click does and returns the address the browser
// was sent to.
func (f *addressFixture) open(t *testing.T, a *app, abs string) string {
	t.Helper()
	before := len(f.opened)
	if refusal := a.openPath(abs); refusal != nil {
		t.Fatalf("opening %s refused: %+v", abs, refusal)
	}
	if len(f.opened) != before+1 {
		t.Fatalf("opening %s sent the browser to %q, want one address", abs, f.opened[before:])
	}
	return f.opened[before]
}

// dashboardAddress is the dashboard's host on these apps' port.
var dashboardAddress = fmt.Sprintf("htmlclay.localhost:%d", nestedTestPort)

// navigate is a typed address or a bookmark for target at host.
func navigate(t *testing.T, a *app, host, target string) *httptest.ResponseRecorder {
	t.Helper()
	return servedRequest(t, a, http.MethodGet, host, target, "", nestedNav)
}

// dashboardAnswers asserts a person who types the dashboard's address is let in
// and gets the workspace page there.
func dashboardAnswers(t *testing.T, a *app) {
	t.Helper()
	first := navigate(t, a, dashboardAddress, "/")
	entry := first.Header().Get("Location")
	if first.Code != http.StatusFound || !strings.HasPrefix(entry, "/enter?ticket=") {
		t.Fatalf("GET / at %s = %d to %q, want the dashboard's entry", dashboardAddress, first.Code, entry)
	}
	entered := navigate(t, a, dashboardAddress, entry)
	cookies := entered.Result().Cookies()
	if entered.Code != http.StatusFound || entered.Header().Get("Location") != "/" || len(cookies) != 1 {
		t.Fatalf("the entry at %s = %d to %q with %d cookies", dashboardAddress, entered.Code, entered.Header().Get("Location"), len(cookies))
	}
	headers := map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value}
	for name, value := range nestedNav {
		headers[name] = value
	}
	page := servedRequest(t, a, http.MethodGet, dashboardAddress, "/", "", headers)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<div id="workspace-root"></div>`) {
		t.Fatalf("the dashboard at %s = %d: %s", dashboardAddress, page.Code, page.Body.String())
	}
}

// dashboardKeeps asserts the dashboard's host answers target itself, with status,
// and sends it nowhere.
func dashboardKeeps(t *testing.T, rec *httptest.ResponseRecorder, target string, status int) {
	t.Helper()
	if rec.Code != status || rec.Header().Get("Location") != "" {
		t.Fatalf("%s at %s = %d to %q, want the dashboard's own %d", target, dashboardAddress, rec.Code, rec.Header().Get("Location"), status)
	}
}

// The dashboard answers at htmlclay.localhost. A document site never does: new
// ones number from htmlclay1, and the one a config from before this step has
// there moves to the next free number once, with its old links sent after it.
func TestM3DashboardAtHTMLClay(t *testing.T) {
	t.Run("a fresh config: the first document gets htmlclay1 and the dashboard answers at htmlclay.localhost", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		a := f.launch(t)
		if a.loaded.MovedFirstSite || a.rt.cfg.Writes() != 0 {
			t.Fatalf("a first launch moved a site (%v) or wrote the config %d times", a.loaded.MovedFirstSite, a.rt.cfg.Writes())
		}

		first := nestedHost("htmlclay1")
		if got, want := f.open(t, a, f.doc), fileURL(first, f.rel(f.doc)); got != want {
			t.Fatalf("the first document opened at %s, want %s", got, want)
		}
		nestedEditable(t, a, first, f.rel(f.doc))
		if got, want := f.open(t, a, f.odoc), fileURL(nestedHost("htmlclay2"), f.rel(f.odoc)); got != want {
			t.Fatalf("the second folder's document opened at %s, want %s", got, want)
		}
		for key, slug := range a.rt.cfg.SiteSlugList() {
			if slug == "htmlclay" {
				t.Fatalf("%s was given the label htmlclay", key)
			}
		}

		dashboardAnswers(t, a)
		want := "http://" + dashboardAddress
		if got := a.snapshot().Host.DashboardURL; got != want+"/" {
			t.Fatalf("the snapshot's dashboardURL = %q, want %q", got, want+"/")
		}
		if _, refusal := a.do(context.Background(), coreAction(t, "open-1", "dashboard.open", map[string]string{"route": ""}), ChannelShell); refusal != nil {
			t.Fatalf("dashboard.open refused: %+v", refusal)
		}
		if got := f.opened[len(f.opened)-1]; !strings.HasPrefix(got, want+"/enter?ticket=") {
			t.Fatalf("dashboard.open sent the browser to %s, want an entry at %s", got, want)
		}

		// Nothing moved, so no document's path leaves the dashboard's host.
		dashboardKeeps(t, navigate(t, a, dashboardAddress, "/"+filepath.ToSlash(f.rel(f.doc))), f.rel(f.doc), http.StatusNotFound)
		if moved := f.onDisk(t).MovedFirstSite; moved != "" {
			t.Fatalf("a config that never had the label records a moved site: %q", moved)
		}
	})

	t.Run("a migrated config: the old label is gone, the document answers at its new label, the old address redirects there, one config write", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		f.seed(t, f.firstSite(f.docs))
		a := f.launch(t)

		if !a.loaded.MovedFirstSite {
			t.Fatal("the load did not report moving the site at htmlclay")
		}
		if got := a.rt.cfg.Writes(); got != 1 {
			t.Fatalf("the start wrote the config %d times, want exactly 1", got)
		}
		disk := f.onDisk(t)
		for key, slug := range disk.SiteSlugs {
			if slug == "htmlclay" {
				t.Fatalf("%s still has the label htmlclay on disk", key)
			}
		}
		// htmlclay1 is the other folder's and htmlclay2 was retired, so the next
		// free number is 3.
		if disk.SiteSlugs[f.docs] != "htmlclay3" || disk.NextSiteNumber != 5 || disk.MovedFirstSite != f.docs {
			t.Fatalf("on disk: label %q, next number %d, moved site %q; want htmlclay3, 5, %q",
				disk.SiteSlugs[f.docs], disk.NextSiteNumber, disk.MovedFirstSite, f.docs)
		}
		if disk.SiteSlugs[f.other] != "htmlclay1" || !slices.Equal(disk.RetiredSlugs, []string{"htmlclay2"}) {
			t.Fatalf("the move touched another label: %+v, retired %v", disk.SiteSlugs, disk.RetiredSlugs)
		}

		rel := f.rel(f.doc)
		moved := nestedHost("htmlclay3")
		// The old link is sent on before the document is open again; what answers
		// at the new address is that address's business.
		nestedRedirect(t, a, dashboardAddress, rel, fileURL(moved, rel))
		nestedRecovery(t, a, moved, rel)

		if got, want := f.open(t, a, f.doc), fileURL(moved, rel); got != want {
			t.Fatalf("the document opened at %s, want %s", got, want)
		}
		nestedEditable(t, a, moved, rel)
		nestedRedirect(t, a, dashboardAddress, rel, fileURL(moved, rel))

		query := servedRequest(t, a, http.MethodGet, dashboardAddress, "/"+filepath.ToSlash(rel)+"?data=%7Bt%3Ah1%7D", "", nestedNav)
		if want := fileURL(moved, rel) + "?data=%7Bt%3Ah1%7D"; query.Code != http.StatusFound || query.Header().Get("Location") != want {
			t.Fatalf("the old link with a query = %d to %q, want 302 to %q", query.Code, query.Header().Get("Location"), want)
		}
		dashboardAnswers(t, a)
	})

	t.Run("a restart changes nothing more", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		f.seed(t, f.firstSite(f.docs))
		first := f.launch(t)
		rel := f.rel(f.doc)
		moved := nestedHost("htmlclay3")
		f.open(t, first, f.doc)
		first.shutdown()
		settled := f.bytes(t)

		second := f.launch(t)
		if second.loaded.MovedFirstSite || second.rt.cfg.Writes() != 0 {
			t.Fatalf("the restart moved a site again (%v) or wrote the config %d times", second.loaded.MovedFirstSite, second.rt.cfg.Writes())
		}
		// The record of the move is in the config, so the old link still goes on.
		nestedRedirect(t, second, dashboardAddress, rel, fileURL(moved, rel))
		if got, want := f.open(t, second, f.doc), fileURL(moved, rel); got != want {
			t.Fatalf("after the restart the document opened at %s, want %s", got, want)
		}
		nestedEditable(t, second, moved, rel)
		dashboardAnswers(t, second)
		second.shutdown()
		if after := f.bytes(t); after != settled {
			t.Fatalf("the restart changed the config:\n%s\nwas\n%s", after, settled)
		}
	})

	t.Run("a trusted folder moves the same way and every document in it follows", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		inner := filepath.Join(f.docs, "deep", "b.htmlclay")
		writeTestFile(t, inner, "<html><body>b</body></html>")
		f.seed(t, f.firstSite(f.docs, f.docs))

		a := f.upgrade(t)
		// The move is one write. A trusted folder's site saves once more when it
		// is built at start, as it always has, and that write changes nothing.
		if got := a.rt.cfg.Writes(); got != 1 {
			t.Fatalf("the upgrade wrote the config %d times, want exactly 1", got)
		}
		upgraded := f.bytes(t)
		a.startSites()
		a.startServed()
		if after := f.bytes(t); after != upgraded {
			t.Fatalf("building the sites changed the config again:\n%s\nwas\n%s", after, upgraded)
		}

		moved := nestedHost("htmlclay3")
		for _, abs := range []string{f.doc, inner} {
			nestedEditable(t, a, moved, f.rel(abs))
			nestedRedirect(t, a, dashboardAddress, f.rel(abs), fileURL(moved, f.rel(abs)))
		}
		oneSiteAnchoredAt(t, a, f.docs)
		dashboardAnswers(t, a)
	})

	t.Run("a file loose in home moves the same way", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		loose := filepath.Join(f.home, "loose.htmlclay")
		writeTestFile(t, loose, "<html><body>loose</body></html>")
		f.seed(t, f.firstSite(loose))
		a := f.launch(t)

		moved := nestedHost("htmlclay3")
		nestedRedirect(t, a, dashboardAddress, "loose.htmlclay", fileURL(moved, "loose.htmlclay"))
		if got, want := f.open(t, a, loose), fileURL(moved, "loose.htmlclay"); got != want {
			t.Fatalf("the loose file opened at %s, want %s", got, want)
		}
		nestedEditable(t, a, moved, "loose.htmlclay")
		// Its key is the one file, so a file beside it was never that site's.
		beside := filepath.Join(f.home, "beside.htmlclay")
		writeTestFile(t, beside, "<html><body>beside</body></html>")
		dashboardKeeps(t, navigate(t, a, dashboardAddress, "/beside.htmlclay"), "beside.htmlclay", http.StatusNotFound)
	})

	t.Run("only a navigation for a document that site held is sent on", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		writeTestFile(t, filepath.Join(f.docs, "picture.png"), "png")
		f.seed(t, f.firstSite(f.docs))
		a := f.launch(t)
		rel := "/" + filepath.ToSlash(f.rel(f.doc))

		// Another site's document, a file that is no document, the site's own
		// routes, and a path that only reaches the document through dot segments
		// were never an old link to a document of the moved site.
		for _, target := range []string{
			"/" + filepath.ToSlash(f.rel(f.odoc)),
			"/notes/picture.png",
			"/_/api" + rel,
			"/other/../notes/a.htmlclay",
			"/notes/.hidden/a.htmlclay",
		} {
			dashboardKeeps(t, navigate(t, a, dashboardAddress, target), target, http.StatusNotFound)
		}
		// A script and a page's own fetch are not a person following a link.
		dashboardKeeps(t, servedRequest(t, a, http.MethodGet, dashboardAddress, rel, "", nil), rel, http.StatusNotFound)
		fetch := map[string]string{"Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty", "Sec-Fetch-Site": "same-origin"}
		dashboardKeeps(t, servedRequest(t, a, http.MethodGet, dashboardAddress, rel, "", fetch), rel, http.StatusNotFound)
		// A save sent to the old address is not replayed anywhere.
		save := servedRequest(t, a, http.MethodPost, dashboardAddress, "/_/save/token", "<html></html>", map[string]string{
			"Origin": "http://" + dashboardAddress, "Sec-Fetch-Site": "same-origin",
		})
		dashboardKeeps(t, save, "/_/save/token", http.StatusMethodNotAllowed)
	})

	t.Run("a start that cannot write the config still moves the site, and the next start records it", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("a read-only folder does not stop root")
		}
		if runtime.GOOS == "windows" {
			t.Skip("Windows ignores a folder's read-only mode")
		}
		f := newAddressFixture(t, "notes")
		seeded := f.firstSite(f.docs)
		f.seed(t, seeded)
		cfgDir := config.DirFrom(f.cfgBase)
		if err := os.Chmod(cfgDir, 0500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(cfgDir, 0755) })

		first := f.launch(t)
		rel := f.rel(f.doc)
		moved := nestedHost("htmlclay3")
		nestedRedirect(t, first, dashboardAddress, rel, fileURL(moved, rel))
		if got := f.bytes(t); got != seeded || first.rt.cfg.Writes() != 0 {
			t.Fatalf("a failed write left the config changed (%d writes): %s", first.rt.cfg.Writes(), got)
		}
		first.shutdown()
		if err := os.Chmod(cfgDir, 0755); err != nil {
			t.Fatal(err)
		}

		second := f.launch(t)
		if got := second.rt.cfg.Writes(); got != 1 {
			t.Fatalf("the next start wrote the config %d times, want exactly 1", got)
		}
		if disk := f.onDisk(t); disk.SiteSlugs[f.docs] != "htmlclay3" || disk.MovedFirstSite != f.docs {
			t.Fatalf("on disk after the next start: label %q, moved site %q", disk.SiteSlugs[f.docs], disk.MovedFirstSite)
		}
		nestedRedirect(t, second, dashboardAddress, rel, fileURL(moved, rel))
	})

	t.Run("the old dashboard host redirects to the same path at htmlclay.localhost", func(t *testing.T) {
		f := newAddressFixture(t, "notes")
		a := f.launch(t)
		old := fmt.Sprintf("dashboard.%s:%d", appcore.ServedParent, nestedTestPort)
		for _, target := range []string{
			"/",
			"/recent",
			"/roots/0011223344556677/files",
			"/recent/d0123456789abcd/versions?before=2026-01-02-03-04-05-006Z.html",
			"/enter?ticket=abc",
		} {
			rec := navigate(t, a, old, target)
			if want := "http://" + dashboardAddress + target; rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
				t.Fatalf("%s at %s = %d to %q, want 302 to %q", target, old, rec.Code, rec.Header().Get("Location"), want)
			}
		}
		// The bookmark lands on a dashboard page, which lets the person in.
		landed := navigate(t, a, dashboardAddress, "/recent")
		if landed.Code != http.StatusFound || !strings.HasPrefix(landed.Header().Get("Location"), "/enter?ticket=") {
			t.Fatalf("/recent at %s = %d to %q, want the dashboard's entry", dashboardAddress, landed.Code, landed.Header().Get("Location"))
		}
		if entry, err := a.dashboard.EntryURL("/recent"); err != nil || !strings.HasPrefix(entry, "http://"+dashboardAddress+"/enter?ticket=") {
			t.Fatalf("the dashboard's entry = %q, %v, want one at %s", entry, err, dashboardAddress)
		}
	})

	t.Run("a dashboard path is never redirected to a document", func(t *testing.T) {
		// The moved site's folder is named api, so its document's old link is a
		// path the dashboard answers itself.
		f := newAddressFixture(t, "api")
		f.seed(t, f.firstSite(f.docs))
		a := f.launch(t)
		rel := f.rel(f.doc)
		moved := nestedHost("htmlclay3")

		if !a.dashboard.Owns("/" + filepath.ToSlash(rel)) {
			t.Fatalf("/%s is not a dashboard path, so this proves nothing", rel)
		}
		dashboardKeeps(t, navigate(t, a, dashboardAddress, "/"+filepath.ToSlash(rel)), rel, http.StatusForbidden)
		for _, own := range []string{"/", "/recent", "/needs-you", "/roots/0011223344556677/files"} {
			rec := navigate(t, a, dashboardAddress, own)
			if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/enter?ticket=") {
				t.Fatalf("%s at %s = %d to %q, want the dashboard's entry", own, dashboardAddress, rec.Code, rec.Header().Get("Location"))
			}
		}
		dashboardAnswers(t, a)

		// The document did move, and its new address is the only way to it.
		if got, want := f.open(t, a, f.doc), fileURL(moved, rel); got != want {
			t.Fatalf("the document opened at %s, want %s", got, want)
		}
		nestedEditable(t, a, moved, rel)
		dashboardKeeps(t, navigate(t, a, dashboardAddress, "/"+filepath.ToSlash(rel)), rel, http.StatusForbidden)
	})
}
