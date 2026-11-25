package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// m3ProgramFixture is one document in one folder, with one registered program
// that writes a marker file when it runs. Every row of TestM3ProgramScope sets up
// the folder and the grant differently, then asks the page's own request path to
// start that program: the marker is the proof a real process did or did not start.
type m3ProgramFixture struct {
	app      *app
	document string
	folder   string
	program  config.HelperProgram
	marker   string
	cfgBase  string
}

func newM3ProgramFixture(t *testing.T) *m3ProgramFixture {
	t.Helper()
	settledSetup(t)
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
	document := writeHelperPage(t, folder, "search")
	path, marker := attachmentProgram(t, "m3")
	program, err := a.rt.cfg.AddHelperProgram("search", path)
	if err != nil {
		t.Fatal(err)
	}
	f := &m3ProgramFixture{
		app:      a,
		document: document,
		folder:   folder,
		program:  program,
		marker:   marker,
		cfgBase:  cfgBase,
	}
	a.openForTest(t, document)
	return f
}

// trust records the document's folder as trusted, the way the read prompt does.
func (f *m3ProgramFixture) trust(t *testing.T) {
	t.Helper()
	if err := f.app.trustFolder(f.folder); err != nil {
		t.Fatal(err)
	}
}

// markTeam records the folder as a team folder under key, whose scope key is
// team:<key>.
func (f *m3ProgramFixture) markTeam(t *testing.T, key, folderID string) {
	t.Helper()
	if err := f.app.rt.cfg.MarkTeamFolder(config.TeamFolder{
		FolderID: folderID,
		Path:     f.folder,
		Identity: platform.DirIdentity(f.folder),
		Kind:     config.TeamFolderKindTeam,
		Key:      key,
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *m3ProgramFixture) save(t *testing.T) {
	t.Helper()
	if err := f.app.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func (f *m3ProgramFixture) allowForAnyDocument(t *testing.T) {
	t.Helper()
	if _, ok := f.app.rt.cfg.SetHelperAnyDocument(f.program.ID, true); !ok {
		t.Fatal("fixture could not grant the program for any document")
	}
	f.save(t)
}

func (f *m3ProgramFixture) allowDocument(t *testing.T, scope config.Scope) {
	t.Helper()
	if _, _, err := f.app.rt.cfg.DecideHelper(config.HelperDecision{
		Document: f.document,
		Name:     "search",
		Program:  f.program.ID,
		Allowed:  true,
		Scope:    scope,
	}); err != nil {
		t.Fatal(err)
	}
	f.save(t)
}

// attach runs the same reattachment the app runs after a grant changes.
func (f *m3ProgramFixture) attach(t *testing.T) {
	t.Helper()
	f.app.refreshHelperDispatchers()
}

func (f *m3ProgramFixture) scope(t *testing.T) config.Scope {
	t.Helper()
	return f.app.scopeOf(f.document)
}

// requireLocalTrusted and requireTeam fail the row when its own fixture did not
// reach the state the row is about, so a row can never pass by accident.
func (f *m3ProgramFixture) requireLocalTrusted(t *testing.T) {
	t.Helper()
	if !f.app.trustedLive(f.document) {
		t.Fatal("fixture: the document's folder is not trusted")
	}
	if scope := f.scope(t); scope.Kind != config.ScopeLocal {
		t.Fatalf("fixture: scope = %+v, want local", scope)
	}
}

func (f *m3ProgramFixture) requireTeam(t *testing.T, key string) {
	t.Helper()
	if scope := f.scope(t); scope != (config.Scope{Kind: config.ScopeTeam, Key: key}) {
		t.Fatalf("fixture: scope = %+v, want team %s", scope, key)
	}
}

// request sends the request a page sends to use the program and reports whether a
// process started. A refused or un-granted request never starts one, and a grant
// that reached the page starts it immediately, so the marker is the answer.
func (f *m3ProgramFixture) request(t *testing.T) bool {
	t.Helper()
	f.app.mu.Lock()
	var site *site
	for _, candidate := range f.app.sites {
		if _, ok := candidate.sessions.LookupByPath(f.document); ok {
			site = candidate
			break
		}
	}
	f.app.mu.Unlock()
	if site == nil {
		t.Fatal("the document is open in no site")
	}
	registration, ok := site.sessions.LookupByPath(f.document)
	if !ok {
		t.Fatal("the document is not registered")
	}
	wireHelperRequest(t, site, registration.Token, "m3-scope", f.document)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(f.marker); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// TestM3ProgramScope is step 7.5: the scope decides what a program may do, at the
// three moments a question is built, a dispatcher attaches, and a process starts.
// Every row tries to start a harmless test program and checks whether it started.
func TestM3ProgramScope(t *testing.T) {
	t.Run("local and trusted starts the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowForAnyDocument(t)
		f.attach(t)
		f.requireLocalTrusted(t)
		if !f.request(t) {
			t.Fatal("the broad grant did not start the program in a trusted local folder")
		}
	})

	t.Run("local and not trusted does not start the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.allowForAnyDocument(t)
		f.attach(t)
		if f.request(t) {
			t.Fatal("the broad grant started the program in an untrusted folder")
		}
	})

	t.Run("a team folder, connected, does not start the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.markTeam(t, "acme", "0123456789abcdef")
		f.allowForAnyDocument(t)
		f.attach(t)
		f.requireTeam(t, "team:acme")
		if f.request(t) {
			t.Fatal("the broad grant started the program in a team folder")
		}
	})

	t.Run("a team folder whose served entry was removed does not start the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		identity := platform.DirIdentity(f.folder)
		served, err := f.app.rt.cfg.AddServedFolder(f.folder, identity, "team")
		if err != nil {
			t.Fatal(err)
		}
		f.markTeam(t, "acme", served.ID)
		// Disconnecting removes the served entry and the trusted one; the folder is
		// trusted again afterwards, and the team record keeps it a team folder.
		if _, ok := f.app.rt.cfg.RemoveServedFolder(served.ID); !ok {
			t.Fatal("fixture removed no served entry")
		}
		if _, ok := f.app.rt.cfg.RemoveTrustedFolder(f.folder); !ok {
			t.Fatal("fixture removed no trusted folder")
		}
		if !f.app.rt.cfg.AddTrustedFolder(f.folder, identity) {
			t.Fatal("fixture could not trust the folder again")
		}
		f.allowForAnyDocument(t)
		f.attach(t)
		f.requireTeam(t, "team:acme")
		if !f.app.trustedLive(f.document) {
			t.Fatal("fixture: the disconnected folder should still be trusted")
		}
		if f.request(t) {
			t.Fatal("the broad grant started the program in a disconnected team folder")
		}
	})

	t.Run("a team folder keyed root:<folder id> does not start the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.markTeam(t, "root:0123456789abcdef", "0123456789abcdef")
		f.allowForAnyDocument(t)
		f.attach(t)
		f.requireTeam(t, "team:root:0123456789abcdef")
		if f.request(t) {
			t.Fatal("a root-keyed team folder took the broad grant")
		}
	})

	t.Run("an unreadable team record does not start the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowForAnyDocument(t)
		f.attach(t)
		f.requireLocalTrusted(t)
		// The record is written and then lost, which reads as unknown, not as local.
		f.markTeam(t, "acme", "0123456789abcdef")
		f.save(t)
		if err := os.Remove(filepath.Join(config.DirFrom(f.cfgBase), "team-folders.json")); err != nil {
			t.Fatal(err)
		}
		// No reattachment: the dispatcher still holds the grant it resolved while the
		// folder was a trusted local one, so only the check before the process starts
		// can stop it.
		if scope := f.scope(t); scope.Kind != config.ScopeUnknown {
			t.Fatalf("fixture: scope = %+v, want unknown", scope)
		}
		if f.request(t) {
			t.Fatal("the broad grant started the program on a machine whose team record cannot be read")
		}
	})

	t.Run("a document whose access was removed does not start the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowForAnyDocument(t)
		f.attach(t)
		f.requireLocalTrusted(t)
		if _, _, err := f.app.rt.cfg.RevokeHelper(f.document, "search"); err != nil {
			t.Fatal(err)
		}
		f.save(t)
		f.attach(t)
		if f.request(t) {
			t.Fatal("a revoked document still started the program")
		}
	})

	t.Run("a relinked program starts the file it now points at", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowDocument(t, config.Scope{Kind: config.ScopeLocal})
		f.attach(t)
		oldMarker := f.marker
		movedPath, movedMarker := attachmentProgram(t, "m3-relinked")
		if _, ok := f.app.rt.cfg.SetHelperProgramPath(f.program.ID, movedPath); !ok {
			t.Fatal("fixture could not relink the program")
		}
		f.save(t)
		f.attach(t)
		f.marker = movedMarker
		if !f.request(t) {
			t.Fatal("a relinked program did not start its new file")
		}
		if _, err := os.Stat(oldMarker); err == nil {
			t.Fatal("the old file ran after the program was relinked")
		}
	})

	t.Run("an allow stored as local does not start once the folder is team", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowDocument(t, config.Scope{Kind: config.ScopeLocal})
		f.attach(t)
		// No reattachment, deliberately: the dispatcher attached while the folder was
		// local still holds the allow, so only the check before the process starts
		// can keep the program from running.
		f.markTeam(t, "acme", "0123456789abcdef")
		f.requireTeam(t, "team:acme")
		if f.request(t) {
			t.Fatal("a local allow ran a program in a folder that became a team folder")
		}
	})

	t.Run("an old-format local decision still starts the program", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowDocument(t, config.Scope{})
		f.attach(t)
		f.requireLocalTrusted(t)
		if !f.request(t) {
			t.Fatal("a decision stored before scopes existed stopped working")
		}
	})
}
