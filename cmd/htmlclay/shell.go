package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// shellShutdownBudget is how long the private listener waits for its open event
// streams to end once done closes. It is short: the Mac app is already gone.
const shellShutdownBudget = 2 * time.Second

// shellBackend is the app as the Mac app sees it. Every action arrives on
// ChannelShell: a person clicked a native control.
type shellBackend struct{ a *app }

func (b shellBackend) Snapshot() appcore.Snapshot { return b.a.snapshot() }

func (b shellBackend) Do(ctx context.Context, act appcore.Action) (appcore.Snapshot, *appcore.Refusal) {
	return b.a.do(ctx, act, ChannelShell)
}

// Check decides an action without running it: the plan do would run, and for
// folder.serve the address the folder would be served at.
func (b shellBackend) Check(act appcore.Action) (shellapi.Preview, *appcore.Refusal) {
	facts := b.a.facts()
	effects, refusal := appcore.Plan(facts, act)
	if refusal != nil {
		return shellapi.Preview{}, refusal
	}
	if act.Type != "folder.serve" {
		return shellapi.Preview{}, nil
	}
	for _, effect := range effects {
		if effect.Type != "folder.serve" {
			continue
		}
		if effect.FolderID != "" {
			return b.preview(servedNameOf(facts, effect.FolderID)), nil
		}
		entry, refusal := b.a.checkServe(effect.Path, effect.Name)
		if refusal != nil {
			return shellapi.Preview{}, refusal
		}
		return b.preview(entry.Name), nil
	}
	return shellapi.Preview{}, nil
}

// Answer is the remembered outcome of an action id in this epoch, so a reply the
// Mac app lost does not have to be sent again.
func (b shellBackend) Answer(id string) (appcore.Snapshot, *appcore.Refusal, bool) {
	b.a.initCore()
	b.a.core.mu.Lock()
	defer b.a.core.mu.Unlock()
	answer, ok := b.a.core.answers[id]
	if !ok {
		return appcore.Snapshot{}, nil, false
	}
	return answer.Snapshot, answer.Refusal, true
}

func (b shellBackend) Subscribe() (<-chan appcore.Snapshot, func()) { return b.a.subscribe() }

// preview is the address a folder served under name answers at.
func (b shellBackend) preview(name string) shellapi.Preview {
	return shellapi.Preview{Name: name, Address: servedHostName(name, b.a.rt.port)}
}

// servedNameOf is the name the facts give folder id: the served name it has, or its
// folder name when it has never been served.
func servedNameOf(facts appcore.Facts, id string) string {
	for _, folder := range facts.Folders {
		if folder.ID != id {
			continue
		}
		if folder.ServedName != "" {
			return folder.ServedName
		}
		return folder.Name
	}
	return ""
}

// startShell serves the Mac app's private listener on its own loopback port and
// prints the READY line the app waits for. in is the rest of stdin: end of file
// means the Mac app is gone, which shuts the host down.
func (a *app) startShell(secret string, in io.Reader) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatal("Could not bind the Mac app's private listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	a.shellDone = make(chan struct{})
	a.shellServer = &http.Server{
		Handler:           shellapi.New(shellBackend{a}, secret, port, a.shellDone),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go a.shellServer.Serve(ln)

	a.initCore()
	a.core.mu.Lock()
	epoch := a.core.epoch
	a.core.mu.Unlock()
	ready := shellapi.Ready{
		V:       1,
		Shell:   fmt.Sprintf("http://127.0.0.1:%d", port),
		Port:    a.rt.port,
		Epoch:   epoch,
		PID:     os.Getpid(),
		Version: version,
	}
	if err := shellapi.WriteReady(os.Stdout, ready); err != nil {
		a.rt.logger.Printf("Could not tell the Mac app the private listener is ready: %v", err)
	}
	a.rt.logger.Printf("Serving the Mac app at %s", ready.Shell)

	go func() {
		_, _ = io.Copy(io.Discard, in)
		a.rt.logger.Printf("The Mac app closed stdin; shutting down")
		a.shutdown()
		os.Exit(0)
	}()
}

// endShellStreams closes done, so every open event stream answers with its bye
// line. Shutdown runs it first, before anything else is torn down.
func (a *app) endShellStreams() {
	if a.shellDone == nil {
		return
	}
	a.shellStreamsOnce.Do(func() { close(a.shellDone) })
}

// closeShell shuts the private listener down last, after the rest of the app has
// settled, so a request the Mac app had already sent still gets its answer. The
// budget is short because the app is gone by then.
func (a *app) closeShell() {
	if a.shellServer == nil {
		return
	}
	a.shellServerOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shellShutdownBudget)
		defer cancel()
		if err := a.shellServer.Shutdown(ctx); err != nil {
			a.shellServer.Close()
		}
	})
}

var _ shellapi.Backend = shellBackend{}
