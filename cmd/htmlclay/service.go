package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/service"
)

const (
	// serviceSpawnTimeout is how long the service has to start, print READY and,
	// later, print ATTACHED.
	serviceSpawnTimeout = 20 * time.Second
	// serviceStopGrace is how long a stopping service has to exit on its own
	// before it is killed.
	serviceStopGrace = 5 * time.Second
	// serviceCloseWait is how long shutdown waits for the process to be gone.
	serviceCloseWait = 6 * time.Second
	// serviceRootDeadline is the deadline the root and flush calls carry.
	serviceRootDeadline = 5000
	// serviceCallGrace is how long a call waits beyond its body's deadline.
	serviceCallGrace = 10 * time.Second
	// serviceBodyLimit is the largest answer a supervisor call reads.
	serviceBodyLimit = 1 << 20
)

// errNoService is a supervisor call made with no attached service to answer it.
var errNoService = errors.New("no live-room service is attached")

// errUnsavedEdits is a stop serving the service kept: the folder still has
// documents with edits that are not saved, so the folder stays served.
type errUnsavedEdits struct {
	Names []string
}

func (e errUnsavedEdits) Error() string {
	return fmt.Sprintf("%d document(s) have edits that are not saved yet", len(e.Names))
}

// unsavedDoc is one document the service has edits for that it could not save.
type unsavedDoc struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// serviceClient is a running service's address and secret. The zero value is no
// service.
type serviceClient struct {
	URL    string
	Secret string
}

// rootsAnswer is the service's answer to POST /v1/roots.
type rootsAnswer struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Kept    []struct {
		ID       string       `json:"id"`
		Unsaved  []unsavedDoc `json:"unsaved"`
		TimedOut bool         `json:"timedOut"`
	} `json:"kept"`
}

// flushAnswer is the service's answer to POST /v1/flush.
type flushAnswer struct {
	Unsaved  []unsavedDoc `json:"unsaved"`
	TimedOut bool         `json:"timedOut"`
}

// serviceHTTPClient is the client every supervisor call uses: loopback only, so
// no proxy, and no compression to unwrap.
var serviceHTTPClient = &http.Client{Transport: &http.Transport{Proxy: nil, DisableCompression: true}}

// serviceRunner drives one service process with the supervisor's state machine.
type serviceRunner struct {
	a       *app
	command []string                 // from HTMLCLAY_SERVICE_CMD; nil in a bundled build
	install func() ([]string, error) // extracts the bundled program; nil without one
	dataDir string                   // <config dir>/service

	mu      sync.Mutex
	machine service.Machine
	proc    *service.Process
	peer    serviceClient // the running process's URL and secret, zero when none
	timer   *time.Timer

	controlSecret string       // 64 hex, made once at startup
	controlURL    string       // http://127.0.0.1:<port>
	control       *http.Server // serves a.served.ControlHandler(controlSecret)
}

// newServiceRunner builds the runner for this process. A bundled build runs
// the program it carries and ignores HTMLCLAY_SERVICE_CMD; any other build runs
// that command when it is set. With neither there is no service, and every call
// below is a no-op.
func newServiceRunner(a *app) *serviceRunner {
	r := &serviceRunner{
		a:             a,
		dataDir:       filepath.Join(a.rt.configDir, "service"),
		controlSecret: newControlSecret(),
	}
	if service.Bundled() {
		root := filepath.Join(a.rt.configDir, "service-runtime")
		r.install = func() ([]string, error) { return service.Install(root) }
	} else {
		r.command = service.CommandFromEnv()
	}
	return r
}

// newControlSecret answers 64 lowercase hex characters for the control listener.
func newControlSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// enabled reports whether this build has a service program to run.
func (r *serviceRunner) enabled() bool {
	return r != nil && (len(r.command) > 0 || r.install != nil)
}

// startControl listens on a free loopback port and serves the host's control API
// there, so an attached service can reach this process. Without a service
// command there is nothing to listen for.
func (r *serviceRunner) startControl() {
	if !r.enabled() {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		r.a.rt.logger.Printf("Could not start the live-room service's control listener: %v", err)
		return
	}
	server := &http.Server{
		Handler:           r.a.served.ControlHandler(r.controlSecret),
		ReadHeaderTimeout: 10 * time.Second,
	}
	r.mu.Lock()
	r.control = server
	r.controlURL = "http://" + listener.Addr().String()
	r.mu.Unlock()
	go server.Serve(listener)
}

// state is what the snapshot shows as host.service. Stopping shows as off: no
// folder is served any more.
func (r *serviceRunner) state() string {
	if r == nil {
		return appcore.ServiceOff
	}
	r.mu.Lock()
	state := r.machine.State()
	r.mu.Unlock()
	switch state {
	case service.Starting:
		return appcore.ServiceStarting
	case service.Running:
		return appcore.ServiceRunning
	case service.Restarting:
		return appcore.ServiceRestarting
	case service.Failed:
		return appcore.ServiceFailed
	}
	return appcore.ServiceOff
}

// step feeds one event to the machine and runs the commands it answers. The
// machine's state is read and written under mu; every command runs after mu is
// released, so no I/O ever happens while the runner is locked.
func (r *serviceRunner) step(ev service.Event) {
	r.mu.Lock()
	before := r.machine.State()
	cmds := r.machine.Step(time.Now(), ev)
	after := r.machine.State()
	r.mu.Unlock()
	for _, cmd := range cmds {
		r.run(cmd)
	}
	if before != after {
		r.a.changed()
	}
}

func (r *serviceRunner) run(cmd service.Command) {
	switch cmd.Kind {
	case service.Spawn:
		go r.spawn()
	case service.Stop:
		if proc := r.process(); proc != nil {
			go proc.Close(serviceStopGrace)
		}
	case service.Kill:
		if proc := r.process(); proc != nil {
			proc.Kill()
		}
	case service.WakeAt:
		r.wakeAt(cmd.At)
	case service.Detach:
		r.a.served.DetachService()
		r.a.rt.logger.Printf("the live-room service stopped five times in two minutes; documents save directly until it is restarted")
	case service.Reattach:
		// Every page document Go saved while detached is already marked
		// unconfirmed (served commitHost.noteDetachedWrite); the new service
		// reconciles those before it admits a semantic write to them.
		r.a.served.AttachService()
	}
}

// wakeAt asks for a Tick at the machine's next deadline. The timer is not I/O,
// so it is set under mu with the rest of the runner's state.
func (r *serviceRunner) wakeAt(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(time.Until(at), func() {
		r.step(service.Event{Kind: service.Tick})
	})
}

// process is the running process, or nil.
func (r *serviceRunner) process() *service.Process {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.proc
}

// spawn starts one process, attaches it to this host and reports the exit that
// follows. It runs on a goroutine of its own: Start and Attach wait on the
// process, which must never happen under mu.
func (r *serviceRunner) spawn() {
	command := r.command
	if r.install != nil {
		installed, err := r.install()
		if err != nil {
			r.a.rt.logger.Printf("The live-room service could not be unpacked: %v", err)
			r.step(service.Event{Kind: service.Exited})
			return
		}
		command = installed
	}
	secret := newControlSecret()
	log := func(line string) { r.a.rt.logger.Printf("service: %s", line) }
	started := time.Now()
	proc, ready, err := service.Start(context.Background(), service.Spec{
		Command: command,
		DataDir: r.dataDir,
		Log:     log,
	}, secret, serviceSpawnTimeout)
	if err != nil {
		r.a.rt.logger.Printf("The live-room service did not start: %v", err)
		if proc != nil {
			proc.Kill()
		}
		r.step(service.Event{Kind: service.Exited})
		return
	}

	r.mu.Lock()
	r.proc = proc
	r.peer = serviceClient{URL: ready.URL, Secret: secret}
	controlURL, controlSecret := r.controlURL, r.controlSecret
	r.mu.Unlock()
	r.a.rt.logger.Printf("The live-room service started in %v (pid %d)", time.Since(started).Round(time.Millisecond), ready.PID)
	r.step(service.Event{Kind: service.Spawned})
	go r.awaitExit(proc)

	if err := r.a.served.SetServicePeer(ready.URL, secret, ready.Epoch); err != nil {
		r.a.rt.logger.Printf("The live-room service's address was refused: %v", err)
	}
	host := service.Host{
		URL:    controlURL,
		Secret: controlSecret,
		Epoch:  r.a.served.Epoch(),
		Roots:  r.roots(),
	}
	if err := proc.Attach(host, serviceSpawnTimeout); err != nil {
		r.a.rt.logger.Printf("The live-room service did not attach: %v", err)
		proc.Kill()
		return
	}
	r.step(service.Event{Kind: service.Attached})
}

// awaitExit reports one process's exit to the machine. A process that is no
// longer the runner's own is a stale one: its exit says nothing about the
// process running now, so it is dropped.
func (r *serviceRunner) awaitExit(proc *service.Process) {
	<-proc.Done()
	r.mu.Lock()
	current := r.proc == proc
	if current {
		r.proc = nil
		r.peer = serviceClient{}
	}
	r.mu.Unlock()
	if !current {
		return
	}
	r.a.rt.logger.Printf("The live-room service exited (code %d)", proc.ExitCode())
	r.a.served.ClearServicePeer()
	r.step(service.Event{Kind: service.Exited})
}

// roots lists every mounted folder as the service knows it.
func (r *serviceRunner) roots() []service.Root {
	r.a.servedMu.RLock()
	defer r.a.servedMu.RUnlock()
	roots := make([]service.Root, 0, len(r.a.servedByName))
	for _, m := range r.a.servedByName {
		roots = append(roots, service.Root{ID: m.folder.ID, Path: m.folder.Path, URL: "http://" + m.host})
	}
	return roots
}

// want tells the machine whether any folder is served now.
func (r *serviceRunner) want() {
	if !r.enabled() {
		return
	}
	r.a.servedMu.RLock()
	on := len(r.a.servedByName) > 0
	r.a.servedMu.RUnlock()
	r.step(service.Event{Kind: service.Want, On: on})
}

// restart starts the service again after it failed, on a person's request.
func (r *serviceRunner) restart() {
	if !r.enabled() {
		return
	}
	r.step(service.Event{Kind: service.Restart})
}

// call posts one JSON request to the attached service and decodes its answer.
// It is a no-op error when nothing is attached.
func (r *serviceRunner) call(ctx context.Context, path string, body, out any) error {
	r.mu.Lock()
	peer := r.peer
	r.mu.Unlock()
	if peer.URL == "" {
		return errNoService
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, peer.URL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+peer.Secret)
	response, err := serviceHTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, serviceBodyLimit))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("service %s answered %d: %s", path, response.StatusCode, bytes.TrimSpace(raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// addRoot tells a running service about one more served folder. A failure is
// only logged: the next attach lists every mounted folder anyway.
func (r *serviceRunner) addRoot(m *servedMount) {
	if !r.enabled() || m == nil || r.state() != appcore.ServiceRunning {
		return
	}
	body := struct {
		Add []service.Root `json:"add"`
	}{Add: []service.Root{{ID: m.folder.ID, Path: m.folder.Path, URL: "http://" + m.host}}}
	ctx, cancel := context.WithTimeout(context.Background(), serviceRootDeadline*time.Millisecond+serviceCallGrace)
	defer cancel()
	if err := r.call(ctx, "/v1/roots", body, nil); err != nil {
		r.a.rt.logger.Printf("The live-room service did not take folder %s: %v", m.folder.Path, err)
	}
}

// removeRoot asks a running service to let go of one folder, flushing what it
// has first. A folder it kept still has unsaved edits, and its documents are
// the answer.
func (r *serviceRunner) removeRoot(id string) ([]unsavedDoc, error) {
	if !r.enabled() || r.state() != appcore.ServiceRunning {
		return nil, nil
	}
	body := struct {
		Remove     []string `json:"remove"`
		DeadlineMs int      `json:"deadlineMs"`
	}{Remove: []string{id}, DeadlineMs: serviceRootDeadline}
	ctx, cancel := context.WithTimeout(context.Background(), serviceRootDeadline*time.Millisecond+serviceCallGrace)
	defer cancel()
	var answer rootsAnswer
	if err := r.call(ctx, "/v1/roots", body, &answer); err != nil {
		return nil, err
	}
	for _, kept := range answer.Kept {
		if kept.ID == id {
			return kept.Unsaved, nil
		}
	}
	return nil, nil
}

// flush asks a running service to save what it still holds and answers the
// documents it could not.
func (r *serviceRunner) flush() ([]unsavedDoc, error) {
	if !r.enabled() || r.state() != appcore.ServiceRunning {
		return nil, nil
	}
	body := struct {
		DeadlineMs int `json:"deadlineMs"`
	}{DeadlineMs: serviceRootDeadline}
	ctx, cancel := context.WithTimeout(context.Background(), serviceRootDeadline*time.Millisecond+serviceCallGrace)
	defer cancel()
	var answer flushAnswer
	if err := r.call(ctx, "/v1/flush", body, &answer); err != nil {
		return nil, err
	}
	return answer.Unsaved, nil
}

// close saves what the service still holds, stops it and waits for it to be
// gone, then stops the control listener. It runs once, on shutdown.
func (r *serviceRunner) close() {
	if !r.enabled() {
		return
	}
	if unsaved, err := r.flush(); err != nil {
		if !errors.Is(err, errNoService) {
			r.a.rt.logger.Printf("The live-room service did not answer before quitting: %v", err)
		}
	} else {
		for _, doc := range unsaved {
			r.a.rt.logger.Printf("Quitting with unsaved edits in %s: %s", doc.Root, doc.Name)
		}
	}
	r.step(service.Event{Kind: service.Want, On: false})
	deadline := time.Now().Add(serviceCloseWait)
	for time.Now().Before(deadline) {
		if r.process() == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.mu.Lock()
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	server := r.control
	r.control = nil
	r.mu.Unlock()
	if server != nil {
		server.Close()
	}
}
