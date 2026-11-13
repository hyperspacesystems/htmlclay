package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
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
