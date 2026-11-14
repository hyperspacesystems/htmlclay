package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/specwire"
)

const documentAccessDocument = "<!doctype html>\n<html><head><title>Reached</title></head><body><p>reached</p></body></html>\n"

const documentAccessVersion = "<!doctype html>\n<html><head><title>Reached</title></head><body><p>kept</p></body></html>\n"

// documentAccessFixture is one app with one served folder, one document in a live
// trusted folder that nobody opened, and one open loose file.
type documentAccessFixture struct {
	app     *app
	home    string
	served  string
	trusted string
	loose   string
}

func newDocumentAccessFixture(t *testing.T) *documentAccessFixture {
	t.Helper()
	home := evalTempHome(t)
	servedDir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(servedDir, "index.html"), servedTestDocument)
	trustedDir := filepath.Join(home, "proj")
	writeTestFile(t, filepath.Join(trustedDir, "page.html"), documentAccessDocument)
	loosePath := filepath.Join(home, "docs", "loose.html")
	writeTestFile(t, loosePath, documentAccessDocument)

	a := newServedTestApp(t, home, t.TempDir())
	_, refusal, err := a.serveFolder(servedDir, "notes")
	if err != nil {
		t.Fatalf("serveFolder: %v", err)
	}
	if refusal != nil {
		t.Fatalf("serveFolder refused: %+v", refusal)
	}
	if err := a.trustFolder(trustedDir); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	a.openForTest(t, loosePath)
	return &documentAccessFixture{
		app:     a,
		home:    home,
		served:  servedDir,
		trusted: trustedDir,
		loose:   loosePath,
	}
}

// documentAccessOf names one document's owner, refusing on the spot when it has
// none.
func documentAccessOf(t *testing.T, a *app, absPath string) DocumentAccess {
	t.Helper()
	access, err := a.documentAccess(absPath)
	if err != nil {
		t.Fatalf("documentAccess(%s): %v", absPath, err)
	}
	return access
}

// readDocumentAccess reads one document through the one lane and answers its
// bytes and stamp.
func readDocumentAccess(t *testing.T, a *app, absPath string) ([]byte, string) {
	t.Helper()
	html, etag, err := documentAccessOf(t, a, absPath).Read(context.Background())
	if err != nil {
		t.Fatalf("read %s: %v", absPath, err)
	}
	return html, etag
}

// registrationFingerprint is every site's anchor with the files it holds, so a
// read can be proved to register nothing.
func registrationFingerprint(t *testing.T, a *app) string {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	var lines []string
	for _, s := range a.sites {
		lines = append(lines, "site "+s.anchor)
		for _, info := range s.sessions.Registrations() {
			lines = append(lines, "file "+info.Path)
		}
	}
	return strings.Join(lines, "\n")
}

// documentAccessTarget is one document the fixture reaches, with the name its
// owner goes by in a failure message.
type documentAccessTarget struct {
	name string
	path string
}

func (fx *documentAccessFixture) targets() []documentAccessTarget {
	return []documentAccessTarget{
		{"served", filepath.Join(fx.served, "index.html")},
		{"trusted", filepath.Join(fx.trusted, "page.html")},
		{"open loose", fx.loose},
	}
}

// liveSiteAt is the live site anchored exactly at folder.
func liveSiteAt(t *testing.T, a *app, folder string) *site {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.siteAtLocked(folder)
	if s == nil {
		t.Fatalf("no live site at %s", folder)
	}
	return s
}

// siteOf answers the site absPath is registered in.
func siteOf(a *app, absPath string) (*site, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, _, ok := a.lookupLocked(absPath)
	return s, ok
}

// registerForSave opens a document through the app's own routing when it is not
// open yet, so its save lane has the token a save names. It is what a person's
// open does, and never what a read does.
func registerForSave(t *testing.T, a *app, absPath string) {
	t.Helper()
	if _, ok := siteOf(a, absPath); ok {
		return
	}
	owner, err := a.ResolveDocument(absPath)
	if err != nil {
		t.Fatalf("resolve %s: %v", absPath, err)
	}
	if owner.Kind == OwnerServed {
		return
	}
	a.openForTest(t, absPath)
}

// saveTarget is the host and path of the document's own save lane: the served
// host's /_/save for a served document, and the open file's token route for a
// loose one.
func saveTarget(t *testing.T, a *app, absPath string) (host, target string, headers map[string]string) {
	t.Helper()
	owner, err := a.ResolveDocument(absPath)
	if err != nil {
		t.Fatalf("resolve %s: %v", absPath, err)
	}
	if owner.Kind == OwnerServed {
		host = strings.TrimPrefix(owner.PublicOrigin, "http://")
		return host, "/_/save", map[string]string{"Document-URL": owner.URL()}
	}
	registerForSave(t, a, absPath)
	s, ok := siteOf(a, absPath)
	if !ok {
		t.Fatalf("%s is not registered in its site", absPath)
	}
	f, ok := s.sessions.LookupByPath(absPath)
	if !ok {
		t.Fatalf("%s has no save token in its site", absPath)
	}
	return s.host, "/_/save/" + f.Token, nil
}

// saveWithStamp posts body to the document's own save lane, naming the stamp the
// writer last saw.
func saveWithStamp(t *testing.T, a *app, absPath, body, ifMatch string) int {
	t.Helper()
	host, target, extra := saveTarget(t, a, absPath)
	headers := map[string]string{
		"Sec-Fetch-Site": "same-origin",
		"Origin":         "http://" + host,
		"Content-Type":   "text/html",
		"If-Match":       ifMatch,
	}
	for name, value := range extra {
		headers[name] = value
	}
	rec := servedRequest(t, a, http.MethodPost, host, target, body, headers)
	return rec.Code
}

// TestM3DocumentAccessReads is step 7.36: one way to reach a document, reads
// first. The same subtests run for a served document, a document in a live
// trusted folder and an open loose file.
func TestM3DocumentAccessReads(t *testing.T) {
	t.Run("the bytes equal the file", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		for _, target := range fx.targets() {
			onDisk, err := os.ReadFile(target.path)
			if err != nil {
				t.Fatalf("read %s: %v", target.path, err)
			}
			html, _ := readDocumentAccess(t, fx.app, target.path)
			if string(html) != string(onDisk) {
				t.Fatalf("%s: the lane answered %q, and the file holds %q", target.name, html, onDisk)
			}
		}
	})

	t.Run("the etag is what the owner's save lane compares", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		for _, target := range fx.targets() {
			registerForSave(t, fx.app, target.path)
			html, etag := readDocumentAccess(t, fx.app, target.path)
			if etag == "" {
				t.Fatalf("%s: the read answered no etag", target.name)
			}
			if want := specwire.Etag(html); etag != want {
				t.Fatalf("%s: etag = %q, want the one its own lane computes, %q", target.name, etag, want)
			}
			body := strings.Replace(servedTestDocument, "<p>first</p>", "<p>saved with the read stamp</p>", 1)
			if code := saveWithStamp(t, fx.app, target.path, body, etag); code != http.StatusOK {
				t.Fatalf("%s: a save naming the etag the read answered = %d, want 200", target.name, code)
			}
			// The stamp the read answered was the one the lane compared: the same
			// save again, with the same stamp, is now stale and writes nothing.
			if code := saveWithStamp(t, fx.app, target.path, body, etag); code == http.StatusOK {
				t.Fatalf("%s: the same stale stamp wrote twice", target.name)
			}
			_, next := readDocumentAccess(t, fx.app, target.path)
			if next == etag {
				t.Fatalf("%s: the etag did not move after a save", target.name)
			}
		}
	})

	t.Run("the wrong owner is never picked", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		if _, ok := documentAccessOf(t, fx.app, filepath.Join(fx.served, "index.html")).(servedDocumentAccess); !ok {
			t.Fatalf("a served document is not reached through the served owner")
		}
		for _, path := range []string{fx.loose, filepath.Join(fx.trusted, "page.html")} {
			access, ok := documentAccessOf(t, fx.app, path).(looseDocumentAccess)
			if !ok {
				t.Fatalf("%s is not reached through the loose owner", path)
			}
			if access.path != path {
				t.Fatalf("%s is read as %s", path, access.path)
			}
		}
		trusted := documentAccessOf(t, fx.app, filepath.Join(fx.trusted, "page.html")).(looseDocumentAccess)
		if want := liveSiteAt(t, fx.app, fx.trusted).srv; trusted.server != want {
			t.Fatalf("the trusted document is read through another site than the folder's own")
		}
	})

	t.Run("a read keeps the registrations and the version store", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		keepLooseVersion(t, fx.app, fx.loose, documentAccessVersion)
		keepLooseVersion(t, fx.app, filepath.Join(fx.trusted, "page.html"), documentAccessVersion)
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-07-890Z.html", documentAccessVersion)

		beforeTree := historyTree(t, fx.home, fx.app.rt.versions.BaseDir())
		beforeFiles := registrationFingerprint(t, fx.app)
		for _, target := range fx.targets() {
			readDocumentAccess(t, fx.app, target.path)
			page, _, err := documentAccessOf(t, fx.app, target.path).Versions(context.Background(), "", historyPageSize)
			if err != nil {
				t.Fatalf("%s: read its versions: %v", target.name, err)
			}
			if len(page) != 1 {
				t.Fatalf("%s: %d versions, want 1", target.name, len(page))
			}
			version, err := documentAccessOf(t, fx.app, target.path).Version(context.Background(), page[0].ID)
			if err != nil {
				t.Fatalf("%s: read version %s: %v", target.name, page[0].ID, err)
			}
			if string(version) != documentAccessVersion {
				t.Fatalf("%s: version %s holds %q", target.name, page[0].ID, version)
			}
		}
		sameHistoryTree(t, beforeTree, historyTree(t, fx.home, fx.app.rt.versions.BaseDir()))
		if after := registrationFingerprint(t, fx.app); after != beforeFiles {
			t.Fatalf("the read changed the registrations:\nbefore:\n%s\nafter:\n%s", beforeFiles, after)
		}
	})

	t.Run("a page of versions continues from the last id", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		older := "<!doctype html>\n<html><body><p>one</p></body></html>\n"
		newer := "<!doctype html>\n<html><body><p>two two</p></body></html>\n"
		keepLooseVersion(t, fx.app, fx.loose, older)
		keepLooseVersion(t, fx.app, fx.loose, newer)
		keepLooseVersion(t, fx.app, filepath.Join(fx.trusted, "page.html"), older)
		keepLooseVersion(t, fx.app, filepath.Join(fx.trusted, "page.html"), newer)
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-07-890Z.html", older)
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-08-890Z.html", newer)

		for _, target := range fx.targets() {
			access := documentAccessOf(t, fx.app, target.path)
			first, next, err := access.Versions(context.Background(), "", 1)
			if err != nil {
				t.Fatalf("%s: first page: %v", target.name, err)
			}
			if len(first) != 1 || next != first[0].ID {
				t.Fatalf("%s: first page = %+v, next = %q", target.name, first, next)
			}
			rest, last, err := access.Versions(context.Background(), next, 1)
			if err != nil {
				t.Fatalf("%s: second page: %v", target.name, err)
			}
			if len(rest) != 1 || rest[0].ID == first[0].ID || rest[0].Size != int64(len(older)) {
				t.Fatalf("%s: second page = %+v", target.name, rest)
			}
			if last != "" {
				t.Fatalf("%s: the last page still offers %q", target.name, last)
			}
		}
	})

	t.Run("a closed loose file is not reachable", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		closed := filepath.Join(fx.home, "elsewhere", "closed.html")
		writeTestFile(t, closed, documentAccessDocument)
		access, err := fx.app.documentAccess(closed)
		if access != nil || !errors.Is(err, ErrNotReachable) {
			t.Fatalf("documentAccess(%s) = %v, %v, want ErrNotReachable", closed, access, err)
		}
	})

	t.Run("a path that leaves its folder through a link is refused", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		outside := filepath.Join(fx.home, "outside", "secret.html")
		writeTestFile(t, outside, documentAccessDocument)
		link := filepath.Join(fx.served, "link.html")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("cannot plant a link: %v", err)
		}
		access, err := fx.app.documentAccess(link)
		if access != nil || !errors.Is(err, ErrNotReachable) {
			t.Fatalf("documentAccess(%s) = %v, %v, want ErrNotReachable", link, access, err)
		}
		if _, err := fx.app.documentAccess(outside); !errors.Is(err, ErrNotReachable) {
			t.Fatalf("the file the link names is reachable: %v", err)
		}
	})

	t.Run("a save and a restore are not built yet", func(t *testing.T) {
		fx := newDocumentAccessFixture(t)
		for _, target := range fx.targets() {
			access := documentAccessOf(t, fx.app, target.path)
			if _, err := access.Save(context.Background(), Save{HTML: []byte(servedTestDocument)}); !errors.Is(err, ErrNotBuilt) {
				t.Fatalf("%s: Save = %v, want ErrNotBuilt", target.name, err)
			}
			if _, err := access.Restore(context.Background(), "", "", ""); !errors.Is(err, ErrNotBuilt) {
				t.Fatalf("%s: Restore = %v, want ErrNotBuilt", target.name, err)
			}
		}
	})
}
