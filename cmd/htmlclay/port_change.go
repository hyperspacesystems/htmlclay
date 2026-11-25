package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/update"
)

// hostRestartStatus is the exit status that asks the Mac app to start the host
// again at once (HostProcess.restartStatus in Swift).
const hostRestartStatus = 75

// portSyncLimit is how long a port change waits for sync to go quiet before it
// is refused for now.
const portSyncLimit = 5 * time.Second

type formerPort struct {
	port int
	srv  *http.Server
}

// workARestartStops names what a restart would stop now: "work" for an AI
// edit or a document's program, "sync" for changes sync has not sent.
func (a *app) workARestartStops() []string {
	var running []string
	if a.runningWork() > 0 {
		running = append(running, "work")
	}
	if !a.syncQuietForUpdate(portSyncLimit) {
		running = append(running, "sync")
	}
	return running
}

// setPort is host.setPort: check the port is free and nothing is unsaved,
// record it with the old port as a former port, and restart on it.
func (a *app) setPort(ctx context.Context, port int, confirmed bool) *appcore.Refusal {
	if slices.Contains(oldDefaultPorts, port) {
		return &appcore.Refusal{Code: appcore.RefusePortInvalid, Field: "port"}
	}
	// A former port is held by this app to forward old visits, and the restart
	// frees it, so moving back to it is always possible.
	if !a.isFormerPort(port) {
		b, err := bindLoopback(port)
		if err != nil {
			return &appcore.Refusal{Code: appcore.RefusePortTaken, Field: "port", OtherName: strconv.Itoa(port)}
		}
		for _, ln := range []net.Listener{b.v4, b.v6} {
			if ln != nil {
				ln.Close()
			}
		}
	}
	if names, answered := a.unsavedDocuments(); !answered || len(names) > 0 {
		return &appcore.Refusal{Code: appcore.RefusePortUnsaved, Documents: names}
	}
	// The restart stops a running AI edit or a document's program, and a delete
	// or an upload the sync engine still holds, which the server then sends back.
	// The person may restart anyway; unsaved edits above are never overridden.
	if !confirmed {
		if running := a.workARestartStops(); len(running) > 0 {
			return &appcore.Refusal{Code: appcore.RefuseRestartStopsWork, Running: running}
		}
	} else {
		// Restart anyway still gives sync its few seconds to send what it holds,
		// then goes on whatever it answers.
		a.syncQuietForUpdate(portSyncLimit)
	}
	if !a.updating.CompareAndSwap(false, true) {
		return &appcore.Refusal{Code: appcore.RefuseStillRunning}
	}
	// Ask open sheets to save before closing writes, matching selfupdate.go:
	// once writesClosedForPort is set, shutdown's own flush would be rejected.
	if a.rt.ls != nil {
		a.rt.ls.FlushOpenSheets(server.FlushWait)
	}
	if names, answered := a.unsavedDocuments(); !answered || len(names) > 0 {
		a.updating.Store(false)
		return &appcore.Refusal{Code: appcore.RefusePortUnsaved, Documents: names}
	}
	a.rt.writesMu.Lock()
	a.rt.writesClosedForPort = true
	a.rt.writesMu.Unlock()
	held := heldWrites(ctx)
	if !waitQuietTo(&a.rt.writesInFlight, held, updateQuietFor, updateQuietLimit) || !a.drainWritesTo(held, updateQuietLimit) {
		a.reopenWrites()
		a.updating.Store(false)
		return &appcore.Refusal{Code: appcore.RefuseStillRunning}
	}
	oldPort, oldFormer := a.rt.cfg.Port(), a.rt.cfg.FormerPortList()
	a.rt.cfg.MovePort(port)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.RestorePort(oldPort, oldFormer)
		a.reopenWrites()
		a.updating.Store(false)
		a.rt.logger.Printf("Could not save the new port %d: %v", port, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.rt.logger.Printf("Moving from port %d to %d; restarting", oldPort, port)
	restart := a.restartForPort
	if a.rt.restartHost != nil {
		restart = a.rt.restartHost
	}
	go restart()
	return nil
}

// restartForPort ends this process so the next one binds the new port. The Mac
// app starts its host again when it exits with hostRestartStatus; on Windows and
// Linux a detached copy waits for this process to exit and takes over.
func (a *app) restartForPort() {
	if !a.shell {
		exe, err := os.Executable()
		if err == nil {
			if a.rt.startDetached != nil {
				err = a.rt.startDetached(exe, portRestartArgs(os.Getpid(), a.noTray)...)
			} else {
				_, err = update.StartDetached(exe, portRestartArgs(os.Getpid(), a.noTray)...)
			}
		}
		if err != nil {
			a.reopenWrites()
			a.updating.Store(false)
			a.rt.logger.Printf("Could not restart for the new port: %v", err)
			if nerr := a.informUser("HTML Clay", appcore.TextPortRestartFailed); nerr != nil {
				a.rt.logger.Printf("Could not show the port notice: %v", nerr)
			}
			return
		}
	}
	a.shutdown()
	if a.shell {
		os.Exit(hostRestartStatus)
	}
	os.Exit(0)
}

// portRestartArgs are the arguments of the copy that takes over after a port
// change on Windows and Linux. A run with its tray reopens Settings on the new
// port, because the page that asked cannot follow: its session ended with this
// process, and a new one starts only from a page the person opened.
func portRestartArgs(pid int, noTray bool) []string {
	args := []string{"--after-pid=" + strconv.Itoa(pid)}
	if noTray {
		return append(args, "--no-tray")
	}
	return append(args, "--open-dashboard=/settings/advanced")
}

// startFormerPorts holds every former port that is free, so bookmarks and open
// tabs at an old address find the app.
func (a *app) startFormerPorts() {
	for _, port := range a.rt.cfg.FormerPortList() {
		if port == a.rt.port {
			continue
		}
		b, err := bindLoopback(port)
		if err != nil {
			a.rt.logger.Printf("Former port %d is taken; not forwarding it: %v", port, err)
			continue
		}
		srv := &http.Server{
			Handler:           a.formerPortHandler(port),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		a.mu.Lock()
		a.former = append(a.former, formerPort{port: port, srv: srv})
		a.mu.Unlock()
		for _, ln := range []net.Listener{b.v4, b.v6} {
			if ln == nil {
				continue
			}
			go func(ln net.Listener) {
				if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
					a.rt.logger.Printf("Former port listener error on %s: %v", ln.Addr(), err)
				}
			}(ln)
		}
		a.rt.logger.Printf("Holding former port %d; page visits go on to port %d", port, a.rt.port)
	}
}

func (a *app) isFormerPort(port int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.ContainsFunc(a.former, func(f formerPort) bool { return f.port == port })
}

// formerPortHandler sends a page visit on to the same address on the current
// port, and answers anything else (a save, a fetch) with 409 port-moved, so a
// page open at the old address says where the app went instead of failing silently.
func (a *app) formerPortHandler(from int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !a.ownsOrigin(r.Host) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		name, _, _ := net.SplitHostPort(r.Host)
		to := "http://" + net.JoinHostPort(name, strconv.Itoa(a.rt.port))
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && server.EligibleNavigation(r, a.ownsOrigin) {
			w.Header().Set("Referrer-Policy", "strict-origin")
			http.Redirect(w, r, to+r.URL.RequestURI(), http.StatusFound)
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"code": "port-moved",
			"msg":  fmt.Sprintf(appcore.TextPortMoved, to+"/"),
			"to":   to + r.URL.RequestURI(),
		})
	})
}
