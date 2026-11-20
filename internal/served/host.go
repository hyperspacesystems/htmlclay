package served

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"sync"

	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/wire"
)

// Options is what a Host needs from the program that runs it.
type Options struct {
	// Assets is Local's static files in Local's own layout (assets/, src/main/templates/,
	// src/main/assets/, src/main/offline-runtime/).
	Assets fs.FS
	// CompileTailwind turns one document into its stylesheet. Nil means the Node compile
	// over LocalSource.
	CompileTailwind func(html string) (string, error)
	// LocalSource is the Local checkout the Node Tailwind compile reads. Only the trial sets it.
	LocalSource string
	// PersonName is the name written into new versions' author field, as --person-name does today.
	PersonName string
	// Service makes the host consult a live-room service. It starts with no peer,
	// so semantic writes answer 503 until SetServicePeer.
	Service bool
	// Dashboard is the workspace's origin, such as http://htmlclay.localhost:1212:
	// the only page that may frame a document loaded with workspace-frame.
	Dashboard string
	// WorkspaceEntry answers the address that opens a root in the workspace, or
	// "" when the workspace cannot open it now. A top-level page load of a root's
	// "/" goes there; everything else keeps the folder listing.
	WorkspaceEntry func(rootID string) string
	// OnPageLoad hears about a served document that answered a top-level page
	// load, which is a page a person asked the browser for. An asset, an API read
	// and a watcher event never reach it. Nil hears nothing.
	OnPageLoad func(rootID, rel string)
	// OnUnsavedReport hears an open tab of a served document say whether it
	// holds edits not saved yet (unsaved-report-v1), with the document's
	// absolute path. Nil hears nothing.
	OnUnsavedReport func(absPath, tab string, seq int64, unsaved, frozen bool)
	// BeforePage hears about a top-level page load of a served document before
	// any of the page is written, with the document's absolute path, so the app
	// can bind the document's programs before the page's first request. Nil
	// hears nothing.
	BeforePage func(rootID, absPath string)
	// OnStructure hears about each structure step that moved a name, so a
	// record that follows a document by its path can follow the move. Nil hears
	// nothing.
	OnStructure func(rootID, from, to string, isDir bool)
	// OnCreate hears about a document the workspace created in a served
	// folder, so the app can record it as the person's own. Nil hears nothing.
	OnCreate func(rootID, rel string)
	// OnCopy hears about each completed duplicate, with the names it came from
	// and went to, so a record that follows a document by its path can start
	// the copy with the source's facts. Nil hears nothing.
	OnCopy func(rootID, from, to string)
	// Stranger reports a document nobody vouches for. Its page is served with
	// its scripts off, because this origin can read the whole folder. Nil
	// counts no document as one.
	Stranger func(absPath string) bool
	// OnRetire hears about each structure step that retired a name, a trash
	// move, so a record that follows a document by its path can forget what it
	// held there. Nil hears nothing.
	OnRetire func(rootID, name string, isDir bool)
	// InternalDir is the app's own data folder. Nothing under it, and nothing
	// under a dot-prefixed folder of a root, is a wire target.
	InternalDir string
	// HeldCopy answers where sync kept the server's version of rel while a
	// conflict over it is held: the record's remoteCopy, relative to the root.
	// GET /_/sync/theirs/<rel> serves it. Nil serves nothing there.
	HeldCopy func(rootID, rel string) (string, bool)
}

// RootSpec is one served folder.
type RootSpec struct {
	ID           string
	Path         string
	PublicHost   string
	AllowFraming bool
	// WirePolicy turns the wire on for this root and decides what it grants.
	// Nil leaves /_/wire/ unanswered, as before.
	WirePolicy wire.Policy
	// WireError writes the wire's refusals, so they have the loose host's bytes.
	WireError func(w http.ResponseWriter, status int, message string)
	// WireLogger receives the wire's log lines. Nil uses logging.NewStdout().
	WireLogger *logging.Logger
}

// Host owns every served folder's handler, the shared event ring, the watchers and the
// snapshot sweep.
type Host struct {
	cfg   config
	mu    sync.Mutex
	roots []*rootServer
	byID  map[string]*rootServer
	// stopSweep stops the snapshot sweep Start began.
	stopSweep func()
	// started is whether Start has begun the roots' watchers, so a root added
	// afterwards starts its own.
	started bool
}

func NewHost(o Options) *Host {
	cfg := config{
		localSource:     o.LocalSource,
		assets:          o.Assets,
		compileTailwind: o.CompileTailwind,
		personName:      o.PersonName,
		dashboard:       o.Dashboard,
		workspaceEntry:  o.WorkspaceEntry,
		onPageLoad:      o.OnPageLoad,
		onUnsavedReport: o.OnUnsavedReport,
		beforePage:      o.BeforePage,
		onStructure:     o.OnStructure,
		onCreate:        o.OnCreate,
		stranger:        o.Stranger,
		onCopy:          o.OnCopy,
		onRetire:        o.OnRetire,
		internalDir:     o.InternalDir,
		heldCopy:        o.HeldCopy,
		events:          newEventRing(),
	}
	if o.Service {
		cfg.service = newServiceLink(cfg.events, servicePeer{})
	}
	return newHostFromConfig(cfg)
}

func newHostFromConfig(cfg config) *Host {
	return &Host{cfg: cfg, byID: make(map[string]*rootServer)}
}

func (h *Host) AddRoot(spec RootSpec) (http.Handler, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.compileTailwind == nil && defaultTailwindCompile == nil {
		return nil, errors.New("served: no Tailwind compiler; set Options.CompileTailwind")
	}
	if _, taken := h.byID[spec.ID]; taken {
		return nil, fmt.Errorf("root %q is served twice", spec.ID)
	}
	server, err := newRootServer(rootSpec{id: spec.ID, path: spec.Path, publicHost: spec.PublicHost, allowFraming: spec.AllowFraming}, h.cfg)
	if err != nil {
		return nil, err
	}
	server.wirePolicy = spec.WirePolicy
	server.internalDir = h.cfg.internalDir
	if spec.WirePolicy != nil {
		logger := spec.WireLogger
		if logger == nil {
			logger = logging.NewStdout()
		}
		writeError := spec.WireError
		if writeError == nil {
			writeError = func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) }
		}
		server.wire = wire.NewCore(&wireDocuments{root: server, internalDir: server.internalDir}, spec.WirePolicy, logger, writeError)
		server.wireMux = server.wire.WireMux()
	}
	h.roots = append(h.roots, server)
	h.byID[spec.ID] = server
	if h.started {
		server.control.watcher.start()
	}
	return server, nil
}

// SetAllowFraming flips whether one root's answers may be framed by another site
// (step 7.14). An unknown id changes nothing.
func (h *Host) SetAllowFraming(rootID string, allow bool) {
	h.mu.Lock()
	server, ok := h.byID[rootID]
	h.mu.Unlock()
	if !ok {
		return
	}
	server.allowFraming.Store(allow)
}

// Versions reads one page of a served document's kept versions, newest first,
// in the same item shape the loose store answers. rel is the document's path
// under the root. A document that is gone still answers its history, and the
// read creates nothing: no version, no folder and no document.
func (h *Host) Versions(rootID, rel, before string, limit int) ([]VersionRow, error) {
	h.mu.Lock()
	server, ok := h.byID[rootID]
	h.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("served: root %q is not served", rootID)
	}
	return server.versions(rel, before, limit)
}

// RemoveRoot stops serving one root: its watcher stops and Handler no longer finds it.
// Requests already inside its handler finish on their own; the caller ends long-lived
// ones (live streams) by cancelling the contexts it handed them.
func (h *Host) RemoveRoot(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	server, ok := h.byID[id]
	if !ok {
		return false
	}
	delete(h.byID, id)
	for i, root := range h.roots {
		if root == server {
			h.roots = append(h.roots[:i], h.roots[i+1:]...)
			break
		}
	}
	if h.started {
		server.control.watcher.stop()
	}
	if server.wire != nil {
		server.wire.Shutdown()
	}
	if h.cfg.service != nil {
		h.cfg.service.dropRoot(id)
	}
	return true
}

// Wire answers a served root's wire core, or false when the root is not served
// or carries no wire.
func (h *Host) Wire(rootID string) (*wire.Core, bool) {
	h.mu.Lock()
	server, ok := h.byID[rootID]
	h.mu.Unlock()
	if !ok || server.wire == nil {
		return nil, false
	}
	return server.wire, true
}

func (h *Host) Handler(id string) (http.Handler, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	server, ok := h.byID[id]
	if !ok {
		return nil, false
	}
	return server, true
}

// ControlHandler answers the control listener. It looks the roots up per
// request, so a root added after it is called is found too.
func (h *Host) ControlHandler(secret string) http.Handler {
	return newControlHandler(secret, h, h.cfg.events, h.cfg.service)
}

// SetServicePeer points the link at a running service. The values must pass
// validServicePeer (boot.go).
func (h *Host) SetServicePeer(url, secret, epoch string) error {
	if h.cfg.service == nil {
		return errors.New("served: the host has no service link")
	}
	peer := servicePeer{URL: url, Secret: secret, Epoch: epoch}
	if !validServicePeer(peer) {
		return errors.New("served: not a service peer: a loopback address, a 64-hex secret and a 16-hex epoch")
	}
	h.cfg.service.setPeer(peer)
	return nil
}

// ErrServiceUnavailable is a service call that got no answer: no service
// attached, the link switched off, a refused connection or an expired deadline.
var ErrServiceUnavailable = errServiceUnavailable

// ErrNotSyncRoute refuses a CallService path outside /sync/v1/.
var ErrNotSyncRoute = errors.New("served: only /sync/v1/ routes may be called")

// syncRoute is one route name under /sync/v1/. No dot, so no ".." can walk the
// service's URL parser back to a control route.
var syncRoute = regexp.MustCompile(`^/sync/v1/[a-z][a-z-]*$`)

// CallService posts one JSON request to a route the service mounted under
// /sync/v1/ and decodes its answer, with the bearer and both epochs a control
// call carries. Only sync routes: every other route on that listener is the
// service's half of the document protocol, which only the host itself drives.
func (h *Host) CallService(ctx context.Context, path string, body, out any) error {
	if !syncRoute.MatchString(path) {
		return ErrNotSyncRoute
	}
	link := h.cfg.service.active()
	if link == nil {
		return ErrServiceUnavailable
	}
	return link.call(ctx, path, body, out)
}

// ClearServicePeer forgets the peer: the service is starting or restarting.
func (h *Host) ClearServicePeer() {
	if h.cfg.service == nil {
		return
	}
	h.cfg.service.setPeer(servicePeer{})
}

// DetachService switches the link off: every request runs Go's native lanes.
// It is a no-op without Options.Service.
func (h *Host) DetachService() {
	if h.cfg.service == nil {
		return
	}
	h.cfg.service.detached.Store(true)
}

// AttachService switches the link back on. It is a no-op without Options.Service.
func (h *Host) AttachService() {
	if h.cfg.service == nil {
		return
	}
	h.cfg.service.detached.Store(false)
}

// ServiceDetached reports the switch, for tests and the snapshot.
func (h *Host) ServiceDetached() bool {
	return h.cfg.service != nil && h.cfg.service.detached.Load()
}

// controlRoot is the control API's lookup for one root by its id.
func (h *Host) controlRoot(id string) (*controlRoot, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	server, ok := h.byID[id]
	if !ok {
		return nil, false
	}
	return server.control, true
}

// controlRootList is every served root, in the order they were added.
func (h *Host) controlRootList() []*controlRoot {
	h.mu.Lock()
	defer h.mu.Unlock()
	sites := make([]*controlRoot, 0, len(h.roots))
	for _, root := range h.roots {
		sites = append(sites, root.control)
	}
	return sites
}

func (h *Host) Epoch() string {
	return h.cfg.events.epoch
}

// Start begins every root's watcher: each root reports what changes on disk from here
// on, so a document written outside this host reaches its guard and the event stream.
// Start starts every root's watcher, so outside edits reach the guard and the
// event stream, and the sweep that drops snapshot entries nobody uploaded.
func (h *Host) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, root := range h.roots {
		root.control.watcher.start()
	}
	h.started = true
	h.stopSweep = startSnapshotSweep()
}

// Close stops the sweep and then the watchers, in the order the trial has always
// stopped them.
func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopSweep != nil {
		h.stopSweep()
		h.stopSweep = nil
	}
	roots := append([]*rootServer(nil), h.roots...)
	for _, root := range roots {
		root.control.watcher.stop()
		if root.wire != nil {
			root.wire.Shutdown()
		}
	}
}

// UnconfirmedCount is how many page documents wait for the service to reconcile
// them, for tests and the snapshot.
func (h *Host) UnconfirmedCount() int {
	if h.cfg.service == nil {
		return 0
	}
	return len(h.cfg.service.unconfirmed.list())
}
