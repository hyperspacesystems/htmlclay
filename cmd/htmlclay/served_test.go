package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/folderlock"
	"github.com/panphora/htmlclay/internal/session"
)

const servedTestPort = 24190

const servedTestDocument = "<!doctype html>\n<html><head><title>Notes</title></head><body><p>first</p></body></html>\n"

func newServedTestApp(t *testing.T, home, cfgBase string) *app {
	t.Helper()
	a := buildTestApp(t, home, cfgBase)
	a.rt.port = servedTestPort
	a.startServed()
	t.Cleanup(func() {
		if a.served != nil {
			a.served.Close()
		}
	})
	return a
}

func servedRequest(t *testing.T, a *app, method, host, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, "http://"+host+path, reader)
	} else {
		req = httptest.NewRequest(method, "http://"+host+path, nil)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	a.serveHost(rec, req)
	return rec
}

func trustedFolderPaths(a *app) []string {
	paths := []string{}
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		paths = append(paths, tf.Path)
	}
	return paths
}

func TestServedServeAndRead(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := a.serveFolder(dir, "notes")
	if err != nil {
		t.Fatalf("serveFolder: %v", err)
	}
	if refusal != nil {
		t.Fatalf("serveFolder refused: %+v", refusal)
	}

	host := servedHostName("notes", servedTestPort)
	rec := servedRequest(t, a, http.MethodGet, host, "/index.html", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET index.html = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "first") {
		t.Fatalf("GET index.html body = %q, want the document", body)
	}

	stored, ok := a.rt.cfg.ServedFolderByID(entry.ID)
	if !ok {
		t.Fatalf("config has no served folder %q", entry.ID)
	}
	if stored.Path != dir || stored.Name != "notes" || !stored.Serving {
		t.Fatalf("stored entry = %+v, want %s as notes, serving", stored, dir)
	}
	trusted := false
	for _, path := range trustedFolderPaths(a) {
		if path == dir {
			trusted = true
		}
	}
	if !trusted {
		t.Fatalf("served folder %s is not trusted", dir)
	}
}

func TestServedSaveKeepsAVersion(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if _, refusal, err := a.serveFolder(dir, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	host := servedHostName("notes", servedTestPort)
	changed := strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1)
	rec := servedRequest(t, a, http.MethodPost, host, "/_/save", changed, map[string]string{
		"Document-URL": "http://" + host + "/index.html",
		"Content-Type": "text/html",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /_/save = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onDisk), "<p>second</p>") {
		t.Fatalf("index.html on disk = %q, want the saved bytes", onDisk)
	}

	versions := servedRequest(t, a, http.MethodGet, host, "/_/versions/index.html", "", nil)
	if versions.Code != http.StatusOK {
		t.Fatalf("GET /_/versions/index.html = %d, want 200 (%s)", versions.Code, versions.Body.String())
	}
	var answer struct {
		OK       bool              `json:"ok"`
		Versions []json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(versions.Body.Bytes(), &answer); err != nil {
		t.Fatalf("versions answer is not JSON: %v (%s)", err, versions.Body.String())
	}
	if !answer.OK || len(answer.Versions) == 0 {
		t.Fatalf("versions answer = %s, want at least one version", versions.Body.String())
	}
}

func TestServedStop(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := a.serveFolder(dir, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}
	if err := a.stopServing(entry.ID); err != nil {
		t.Fatalf("stopServing: %v", err)
	}

	host := servedHostName("notes", servedTestPort)
	rec := servedRequest(t, a, http.MethodGet, host, "/index.html", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after stop = %d, want 404", rec.Code)
	}
	if body := rec.Body.String(); body != string(recoveryPage) {
		t.Fatalf("GET after stop body = %q, want the recovery page", body)
	}

	stored, ok := a.rt.cfg.ServedFolderByID(entry.ID)
	if !ok || stored.Serving {
		t.Fatalf("stored entry after stop = %+v (found %v), want Serving false", stored, ok)
	}
	trusted := false
	for _, path := range trustedFolderPaths(a) {
		if path == dir {
			trusted = true
		}
	}
	if !trusted {
		t.Fatalf("stopped folder %s is no longer trusted", dir)
	}
}

func TestServedRefusals(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(notes, "index.html"), servedTestDocument)
	other := filepath.Join(home, "other")
	writeTestFile(t, filepath.Join(other, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())

	if _, refusal, err := a.serveFolder(home, "home"); err != nil || refusal == nil || refusal.Code != appcore.RefuseIsHome {
		t.Fatalf("serving home: refusal=%+v err=%v, want is-home", refusal, err)
	}
	if _, refusal, err := a.serveFolder(notes, "notes"); err != nil || refusal != nil {
		t.Fatalf("serving notes: refusal=%+v err=%v", refusal, err)
	}
	if _, refusal, err := a.serveFolder(notes, "another-name"); err != nil || refusal == nil || refusal.Code != appcore.RefuseOverlapsServed {
		t.Fatalf("serving notes twice: refusal=%+v err=%v, want overlaps-served", refusal, err)
	}
	if _, refusal, err := a.serveFolder(other, "notes"); err != nil || refusal == nil || refusal.Code != appcore.RefuseNameTaken {
		t.Fatalf("serving other as notes: refusal=%+v err=%v, want name-taken", refusal, err)
	}
}

func TestServedComesBackAfterRestart(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	cfgBase := t.TempDir()

	first := newServedTestApp(t, home, cfgBase)
	if _, refusal, err := first.serveFolder(dir, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	second := newServedTestApp(t, home, cfgBase)
	host := servedHostName("notes", servedTestPort)
	rec := servedRequest(t, second, http.MethodGet, host, "/index.html", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after restart = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "first") {
		t.Fatalf("GET after restart body = %q, want the document", body)
	}
}

func TestServedNotMountedTwice(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	cfgBase := t.TempDir()

	first := newServedTestApp(t, home, cfgBase)
	if _, refusal, err := first.serveFolder(dir, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	second := buildTestApp(t, home, cfgBase)
	second.rt.port = servedTestPort
	second.startServed()
	t.Cleanup(func() {
		if second.served != nil {
			second.served.Close()
		}
	})
	second.startSites()

	second.hostMu.RLock()
	defer second.hostMu.RUnlock()
	for _, s := range second.bySlug {
		if s.key == dir {
			t.Fatalf("startSites built a trusted-folder site for the served folder %s", dir)
		}
	}
}

func TestServedRouterRefusesBadNames(t *testing.T) {
	a := newServedTestApp(t, t.TempDir(), t.TempDir())

	rec := servedRequest(t, a, http.MethodGet, "Bad_Name.htmlclay.localhost:"+strconv.Itoa(servedTestPort), "/", "", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET Bad_Name = %d, want 403", rec.Code)
	}

	rec = servedRequest(t, a, http.MethodGet, servedHostName("nobody", servedTestPort), "/", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET nobody = %d, want 404", rec.Code)
	}
	if body := rec.Body.String(); body != string(recoveryPage) {
		t.Fatalf("GET nobody body = %q, want the recovery page", body)
	}

	rec = servedRequest(t, a, http.MethodGet, servedHostName("nobody", servedTestPort+1), "/", "", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET on the wrong port = %d, want 403", rec.Code)
	}
}

func servedFileURL(name, rel string) string {
	return "http://" + servedHostName(name, servedTestPort) + "/" + filepath.ToSlash(rel)
}

func TestServedResolveDocument(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	served := filepath.Join(home, "notes")
	servedFile := filepath.Join(served, "index.htmlclay")
	trusted := filepath.Join(home, "proj")
	trustedFile := filepath.Join(trusted, "page.htmlclay")
	loose := filepath.Join(home, "loose.htmlclay")
	writeTestFile(t, servedFile, servedTestDocument)
	writeTestFile(t, trustedFile, servedTestDocument)
	writeTestFile(t, loose, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(trusted); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	entry, refusal, err := a.serveFolder(served, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	owner, err := a.ResolveDocument(loose)
	if err != nil {
		t.Fatalf("ResolveDocument(%s): %v", loose, err)
	}
	if owner.Kind != OwnerLoose {
		t.Fatalf("a loose file resolved to kind %v, want OwnerLoose (%+v)", owner.Kind, owner)
	}

	owner, err = a.ResolveDocument(trustedFile)
	if err != nil {
		t.Fatalf("ResolveDocument(%s): %v", trustedFile, err)
	}
	if owner.Kind != OwnerTrusted || owner.FolderPath != trusted || owner.RelativePath != "page.htmlclay" {
		t.Fatalf("a trusted file resolved to %+v, want OwnerTrusted in %s", owner, trusted)
	}

	owner, err = a.ResolveDocument(servedFile)
	if err != nil {
		t.Fatalf("ResolveDocument(%s): %v", servedFile, err)
	}
	want := servedFileURL("notes", "index.htmlclay")
	if owner.Kind != OwnerServed || owner.FolderID != entry.ID || owner.FolderPath != served {
		t.Fatalf("a served file resolved to %+v, want OwnerServed for %s", owner, served)
	}
	if owner.URL() != want {
		t.Fatalf("served file URL = %q, want %q", owner.URL(), want)
	}

	link := filepath.Join(home, "via-link")
	if err := os.Symlink(served, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	owner, err = a.ResolveDocument(filepath.Join(link, "index.htmlclay"))
	if err != nil {
		t.Fatalf("ResolveDocument through a link: %v", err)
	}
	if owner.Kind != OwnerServed || owner.URL() != want {
		t.Fatalf("a served file reached through a symlinked folder resolved to %+v (%q), want %q", owner, owner.URL(), want)
	}

	if _, err := os.Stat(filepath.Join(home, "NOTES")); err != nil {
		t.Skip("this volume is case-sensitive")
	}
	owner, err = a.ResolveDocument(filepath.Join(home, "NOTES", "index.htmlclay"))
	if err != nil {
		t.Fatalf("ResolveDocument in another letter case: %v", err)
	}
	if owner.Kind != OwnerServed || owner.URL() != want {
		t.Fatalf("a served file in another letter case resolved to %+v (%q), want %q", owner, owner.URL(), want)
	}
}

func TestServedRegistrationRefused(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "proj")
	writeTestFile(t, filepath.Join(proj, "page.htmlclay"), servedTestDocument)
	served := filepath.Join(home, "notes")
	servedFile := filepath.Join(served, "index.htmlclay")
	writeTestFile(t, servedFile, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(proj); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	s := liveSite(t, a, proj)
	if _, refusal, err := a.serveFolder(served, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	if _, err := s.sessions.Register(servedFile, session.ViaTrusted); !errors.Is(err, session.ErrServedPath) {
		t.Fatalf("Register of a served file = %v, want ErrServedPath", err)
	}
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := s.sessions.RestoreRegistration(servedFile, session.ViaTrusted, token); !errors.Is(err, session.ErrServedPath) {
		t.Fatalf("RestoreRegistration of a served file = %v, want ErrServedPath", err)
	}
}

func TestServedOpenGoesToServedAddress(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	served := filepath.Join(home, "notes")
	servedFile := filepath.Join(served, "index.htmlclay")
	writeTestFile(t, servedFile, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if _, refusal, err := a.serveFolder(served, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	var launched []string
	a.rt.launchBrowser = func(target string) error {
		launched = append(launched, target)
		return nil
	}
	a.openFile(servedFile)

	want := servedFileURL("notes", "index.htmlclay")
	if len(launched) != 1 || launched[0] != want {
		t.Fatalf("opening a served file launched %q, want exactly %q", launched, want)
	}
	if n := registrations(t, a, servedFile); n != 0 {
		t.Fatalf("opening a served file registered it in %d sites", n)
	}
	a.hostMu.RLock()
	sites := len(a.bySlug)
	a.hostMu.RUnlock()
	if sites != 0 {
		t.Fatalf("opening a served file created %d sites", sites)
	}
}

func TestServedOldAddressRedirects(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "proj")
	page := filepath.Join(proj, "notes.htmlclay")
	writeTestFile(t, page, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(proj); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	s, rel := a.openForTest(t, page)
	oldHost := s.host
	oldURL := s.url(rel)

	if _, refusal, err := a.serveFolder(proj, "proj"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	want := servedFileURL("proj", filepath.Base(page))
	rec := servedRequest(t, a, http.MethodGet, oldHost, "/"+filepath.ToSlash(rel), "", map[string]string{
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
		"Sec-Fetch-Site": "none",
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("navigating to the old address %s = %d, want 302 (%q)", oldURL, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location from the old address = %q, want %q", got, want)
	}

	query := "?editmode=true&label=a"
	rec = servedRequest(t, a, http.MethodGet, oldHost, "/"+filepath.ToSlash(rel)+query, "", map[string]string{
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
		"Sec-Fetch-Site": "none",
	})
	if got := rec.Header().Get("Location"); got != want+query {
		t.Fatalf("Location with a query = %q, want %q", got, want+query)
	}
}

func TestServedBareHostAPI(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	served := filepath.Join(home, "notes")
	servedFile := filepath.Join(served, "index.htmlclay")
	writeTestFile(t, servedFile, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if _, refusal, err := a.serveFolder(served, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	req := httptest.NewRequest(http.MethodGet, "/_/api/notes/index.htmlclay", nil)
	req.Host = "localhost:" + strconv.Itoa(servedTestPort)
	rec := httptest.NewRecorder()
	a.serveHost(rec, req)

	want := servedFileURL("notes", "index.htmlclay")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("the bare-host data API for a served file = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
	var answer struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v (%q)", err, rec.Body.String())
	}
	if answer.Error != "Served elsewhere" || !strings.Contains(answer.Message, "served at its own address") {
		t.Fatalf("the answer = %+v, want the Served elsewhere refusal", answer)
	}
	if answer.Address != want {
		t.Fatalf("address = %q, want %q", answer.Address, want)
	}
}

func TestServedClosesOlderSites(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "proj")
	page := filepath.Join(proj, "index.htmlclay")
	writeTestFile(t, page, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(proj); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	s, rel := a.openForTest(t, page)
	slug, oldHost := s.slug, s.host
	oldURL := s.url(rel)

	if _, refusal, err := a.serveFolder(proj, "proj"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	a.hostMu.RLock()
	_, live := a.bySlug[slug]
	a.hostMu.RUnlock()
	if live {
		t.Fatalf("the older site %s is still live after serving its folder", slug)
	}
	if n := registrations(t, a, page); n != 0 {
		t.Fatalf("the served folder's file is still registered in %d sites", n)
	}

	changed := strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1)
	save := servedRequest(t, a, http.MethodPost, oldHost, "/_/save", changed, map[string]string{
		"Document-URL": oldURL,
		"Content-Type": "text/html",
	})
	if save.Code == http.StatusOK {
		t.Fatalf("a save to the old address succeeded: %d %q", save.Code, save.Body.String())
	}
	onDisk, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(onDisk), "<p>second</p>") {
		t.Fatalf("a save to the old address reached the file: %q", onDisk)
	}

	served := servedRequest(t, a, http.MethodGet, servedHostName("proj", servedTestPort), "/"+filepath.Base(page), "", nil)
	if served.Code != http.StatusOK {
		t.Fatalf("GET on the served address = %d, want 200 (%s)", served.Code, served.Body.String())
	}
	if body := served.Body.String(); !strings.Contains(body, "first") {
		t.Fatalf("GET on the served address = %q, want the document", body)
	}
}

func TestServedLockRefusesASecondProfile(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	first := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := first.serveFolder(dir, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("first serveFolder: refusal=%+v err=%v", refusal, err)
	}

	second := newServedTestApp(t, home, t.TempDir())
	second.rt.folderLockDir = first.rt.folderLockDir

	_, refusal, err = second.serveFolder(dir, "notes")
	if err != nil {
		t.Fatalf("second serveFolder: %v", err)
	}
	if refusal == nil || refusal.Code != appcore.RefuseServedElsewhere {
		t.Fatalf("second serveFolder refusal = %+v, want served-elsewhere", refusal)
	}
	if refusal != nil && refusal.OtherPath != dir {
		t.Fatalf("refusal OtherPath = %q, want %q", refusal.OtherPath, dir)
	}
	if served := second.rt.cfg.ServedFolderList(); len(served) != 0 {
		t.Fatalf("second config has served folder(s) %+v, want none", served)
	}

	if err := first.stopServing(entry.ID); err != nil {
		t.Fatalf("stopServing: %v", err)
	}
	if _, refusal, err := second.serveFolder(dir, "notes"); err != nil || refusal != nil {
		t.Fatalf("second serveFolder after the first stopped: refusal=%+v err=%v", refusal, err)
	}
}

func TestServedLockSkippedAtStartup(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	cfgBase := t.TempDir()
	lockDir := t.TempDir()

	seeded := buildTestApp(t, home, cfgBase)
	seeded.rt.folderLockDir = lockDir
	seeded.rt.port = servedTestPort
	seeded.startServed()
	t.Cleanup(func() {
		if seeded.served != nil {
			seeded.served.Close()
		}
	})
	entry, refusal, err := seeded.serveFolder(dir, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	seeded.servedMu.RLock()
	mount := seeded.servedByName["notes"]
	seeded.servedMu.RUnlock()
	if mount == nil || mount.lock == nil {
		t.Fatalf("the served folder has no mount holding its lock")
	}
	if err := mount.lock.Release(); err != nil {
		t.Fatalf("releasing the mount's lock: %v", err)
	}

	held, err := folderlock.Acquire(lockDir, dir)
	if err != nil {
		t.Fatalf("holding the folder's lock: %v", err)
	}
	t.Cleanup(func() { held.Release() })

	fresh := buildTestApp(t, home, cfgBase)
	fresh.rt.folderLockDir = lockDir
	fresh.rt.port = servedTestPort
	fresh.startServed()
	t.Cleanup(func() {
		if fresh.served != nil {
			fresh.served.Close()
		}
	})

	rec := servedRequest(t, fresh, http.MethodGet, servedHostName("notes", servedTestPort), "/index.html", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET a folder held elsewhere = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != string(recoveryPage) {
		t.Fatalf("GET a folder held elsewhere body = %q, want the recovery page", body)
	}

	stored, ok := fresh.rt.cfg.ServedFolderByID(entry.ID)
	if !ok || !stored.Serving {
		t.Fatalf("stored entry = %+v (found %v), want it to keep Serving true", stored, ok)
	}
}
