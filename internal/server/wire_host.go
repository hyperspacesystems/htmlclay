package server

import (
	"errors"
	"github.com/panphora/htmlclay/internal/wire"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/versions"
)

// handleHelperSetup is the page asking HTML Clay to connect one program it
// declares. The page names only the program; HTML Clay finds the file, shows
// the native dialog, and answers with a coarse state. A page can call this
// without a click, so what keeps it in check is the dialog itself (no default
// button, the real file first), one dialog per name at a time, a minute of
// quiet after a dialog that granted nothing, and Deny lasting until the person
// opens the document again.
func (s *Server) handleHelperSetup(w http.ResponseWriter, r *http.Request) {
	f, ok := s.actionDocument(w, r)
	if !ok {
		return
	}
	s.HandleSetup(w, r, wire.Document{Path: f.AbsPath, Name: f.RelPath, Ref: f})
}

// wireCaller classifies who is asking and whether the origin gate admits them.
//
// A browser attests at least one of Sec-Fetch-Site or Origin on every fetch and
// every EventSource and cannot forge either; a local process attests neither.
// That remains sufficient for a plain wire. A helper-bound channel adds the
// target document's save token after target resolution, including for callers
// classified as local processes.
//
// Origin is checked only WHEN PRESENT, never required. Chrome omits it on
// same-origin GETs, including EventSource's stream GET, and requiring it is what
// silently 403'd every live-sync stream from v1.3.0 until v1.5.0. The subscribe
// route below is the same kind of GET and would break identically.
//
// same-site is rejected rather than admitted: htmlclay serves one loopback origin
// per project tree and every one of them is same-site with every other, so
// admitting it would let one project's page drive another project's wire.
//
// The isBrowser answer is RETURNED rather than discarded, because it is what
// makes the handler role safe. See handleWireSubscribe.
func wireCaller(r *http.Request) (isBrowser, ok bool) {
	site := r.Header.Get("Sec-Fetch-Site")
	origin := r.Header.Get("Origin")
	if site == "" && origin == "" {
		return false, true
	}
	if site != "" && site != "same-origin" {
		return true, false
	}
	if origin != "" && origin != "http://"+r.Host {
		return true, false
	}
	return true, true
}

// wireTarget resolves which file this request is about.
//
// The two callers are not symmetric and must not be merged. A browser NEVER names
// a path: its file comes from the page's own URL, through the same funnel
// handleLiveSyncSave uses, and any supplied file field is discarded. A local
// process has no page, so it names an absolute path, which is then validated.
//
// Note that origin-wide trust is inherited here, not introduced: on an unbound
// channel resolvePageURL already lets one served page name another registered
// path on the same origin (livesync.go). A helper-bound channel adds the target's
// save token after this function returns. "One wire per file" is addressing,
// not isolation.
func (s *Server) wireTarget(r *http.Request, isBrowser bool, supplied string) (*session.File, bool) {
	if isBrowser {
		return s.resolvePageURL(r, wire.PageURL(r))
	}
	return s.resolveWireFile(supplied)
}

// resolveWireFile admits an absolute path from a local process.
//
// ValidatePath cannot be called on it directly: that helper takes a URL-relative
// path and rejects anything starting with "/", so the address is made relative to
// the home directory first and then runs the same containment it does.
//
// Ordering is the serve path's discipline: containment and internal/hidden
// refusal are string and memory work and happen FIRST, so an out-of-scope path is
// refused identically whether or not anything exists at it. Only a path already
// registered in this site is admitted. Admitting an unregistered path under a
// live trusted anchor is the subscribe-before-open case; it needs the site's own
// anchor to compare against (answering "is it under ANY trusted folder" would let
// one project's origin open a wire into another's) and is deliberately not built
// here yet.
func (s *Server) resolveWireFile(raw string) (*session.File, bool) {
	if raw == "" || !filepath.IsAbs(raw) || strings.Contains(raw, "\x00") {
		return nil, false
	}
	home := s.sessions.HomeDir()
	canonical, ok := session.ContainWithinHome(home, filepath.Clean(raw))
	if !ok || s.isInternal(canonical) || session.HasHiddenComponent(home, canonical) {
		return nil, false
	}
	return s.sessions.LookupByPath(canonical)
}

// openHistoryForHandler turns version history on for a file an agent is about to
// start editing, and tells the watcher what is already there.
//
// The history key is resolved HERE rather than in the watcher, through the same
// seam serving uses. versions.Backup needs a key, a key is only ever set by
// serving or saving, and the handler lease exists precisely to watch files no tab
// has served, where the key is still empty. Attaching an agent therefore becomes
// the act that turns version history on for a file nobody has opened, with a
// baseline backup of what was there first, so the agent's own first write is not
// the earliest state history remembers. A freshly minted identity is marked
// provisional exactly as a first-open snapshot is, so attaching an agent to a file
// that then never changes does not permanently claim an id for it.
//
// Only lastStableObservation is seeded, and only when nothing has observed the
// file yet. Marking the file OBSERVED here would let a wire subscription do what
// the watcher is structurally forbidden from doing (session.go derives Observed
// from lastServerWrite for that reason), and the file's first real GET would then
// skip both its first-open snapshot and the seeding that makes its first save
// comparable. Overwriting an existing observation would suppress a change already
// waiting to be published.
//
// It runs BEFORE the lease is raised, so the watcher's first poll never sees a
// file with no observation on record and reports its existing content as a change.
func (s *Server) openHistoryForHandler(f *session.File) error {
	// Bulk pruning takes the store lock, and the lock order is file lock before
	// store lock, so it runs only once the seeding has released f.Lock.
	key, err := s.seedHandlerHistory(f)
	if key != "" {
		s.versions.MaybePrune(key, f.AbsPath)
	}
	return err
}

func (s *Server) seedHandlerHistory(f *session.File) (string, error) {
	f.Lock()
	defer f.Unlock()

	data, err := os.ReadFile(f.AbsPath)
	if errors.Is(err, fs.ErrNotExist) {
		// A registered file can be gone by now, and the agent may be about to put it
		// back. There are no bytes to seed an observation from and no embedded id to
		// adopt, so the identity resolves from the path alone. Doing that here is
		// what lets the watcher version the agent's very first write.
		s.ensureHistoryKeyLocked(f, nil)
		return "", nil
	}
	if err != nil {
		// An existing file this server cannot read is a refusal, not a degraded
		// attach. Its bytes may carry an id a later serve should adopt, so minting
		// one now would fork its history, and leaving the key unresolved would mean
		// versioning nothing the agent writes.
		return "", err
	}
	key, provisional := s.ensureHistoryKeyLocked(f, data)
	if f.LastStableObservation() == "" {
		f.RecordStableObservation(versions.Hash(data))
	}
	if _, bErr := s.versions.Backup(key, f.AbsPath, data); bErr != nil {
		s.Logger.Printf("Baseline backup for wire handler on %s failed: %v", f.RelPath, bErr)
	} else if provisional {
		if pErr := s.versions.SetProvisional(key, f.AbsPath, true); pErr != nil {
			s.Logger.Printf("Could not mark provisional history for %s: %v", f.RelPath, pErr)
		}
	}
	return key, nil
}

// looseWire is the loose host's side of the wire: the documents this server
// serves and the decisions its hooks make. Every method is one line over what the
// wire called before the seam existed.
type looseWire struct{ s *Server }

func (l looseWire) Caller(r *http.Request) (isBrowser, ok bool) { return wireCaller(r) }

func (l looseWire) Target(r *http.Request, isBrowser bool, supplied string) (wire.Document, bool) {
	f, ok := l.s.wireTarget(r, isBrowser, supplied)
	if !ok {
		return wire.Document{}, false
	}
	return wire.Document{Path: f.AbsPath, Name: f.RelPath, Ref: f}, true
}

func (l looseWire) At(path string) (wire.Document, bool) {
	f, ok := l.s.sessions.LookupByPath(path)
	if !ok {
		return wire.Document{}, false
	}
	return wire.Document{Path: f.AbsPath, Name: f.RelPath, Ref: f}, true
}

func (l looseWire) MayUsePrograms(doc wire.Document, isBrowser bool, presented string) bool {
	return presented == doc.Ref.(*session.File).Token
}

func (l looseWire) OpenHistory(doc wire.Document) error {
	return l.s.openHistoryForHandler(doc.Ref.(*session.File))
}

func (l looseWire) Lease(doc wire.Document)   { l.s.coord.lease(doc.Ref.(*session.File)) }
func (l looseWire) Unlease(doc wire.Document) { l.s.coord.unlease(doc.Ref.(*session.File)) }
func (l looseWire) Poke(doc wire.Document)    { l.s.coord.pokeWatcher(doc.Ref.(*session.File)) }

func (l looseWire) Internal(absPath string) bool { return l.s.isInternal(absPath) }

func (l looseWire) CanAskProgram() bool { return l.s.hooks.AskHelper != nil }

func (l looseWire) AskProgram(file, name string) bool { return l.s.hooks.AskHelper(file, name) }

func (l looseWire) ProgramGranted(file, name, programID string) bool {
	granted := l.s.hooks.HelperGranted
	return granted == nil || granted(file, name, programID)
}

func (l looseWire) AIEditOffered() bool { return l.s.hooks.AIEditFor != nil }

func (l looseWire) AIEditFor(document string) (on, ask bool) { return l.s.hooks.AIEditFor(document) }

func (l looseWire) AskAIEdit(document string) {
	if l.s.hooks.AskAIEdit != nil {
		l.s.hooks.AskAIEdit(document)
	}
}

func (l looseWire) AIEditFiles(document string) bool {
	return l.s.hooks.AIEditFiles != nil && l.s.hooks.AIEditFiles(document)
}

func (l looseWire) AIEditEngines() (string, map[string][]string) {
	if l.s.hooks.AIEditEngines == nil {
		return "", nil
	}
	return l.s.hooks.AIEditEngines()
}

func (l looseWire) AIEditModels() map[string]string {
	if l.s.hooks.AIEditModels == nil {
		return nil
	}
	return l.s.hooks.AIEditModels()
}

func (l looseWire) AIEditEfforts() map[string]string {
	if l.s.hooks.AIEditEfforts == nil {
		return nil
	}
	return l.s.hooks.AIEditEfforts()
}

func (l looseWire) BuiltinFor(document, program string) builtin.Access {
	if l.s.hooks.BuiltinFor == nil {
		return builtin.Access{State: builtin.Off}
	}
	return l.s.hooks.BuiltinFor(document, program)
}

func (l looseWire) AskBuiltin(document, program string) {
	if l.s.hooks.AskBuiltin != nil {
		l.s.hooks.AskBuiltin(document, program)
	}
}

func (l looseWire) RevealFile(path string) error {
	if l.s.hooks.Reveal == nil {
		return errors.New("Showing files in your file manager is not available here.")
	}
	return l.s.hooks.Reveal(path)
}
