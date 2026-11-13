package served

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
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
}

// RootSpec is one served folder.
type RootSpec struct {
	ID         string
	Path       string
	PublicHost string
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
	server, err := newRootServer(rootSpec{id: spec.ID, path: spec.Path, publicHost: spec.PublicHost}, h.cfg)
	if err != nil {
		return nil, err
	}
	h.roots = append(h.roots, server)
	h.byID[spec.ID] = server
	if h.started {
		server.control.watcher.start()
	}
	return server, nil
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
	if h.cfg.service != nil {
		h.cfg.service.dropRoot(id)
	}
	return true
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
