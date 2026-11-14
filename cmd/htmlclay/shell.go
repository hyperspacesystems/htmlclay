package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/pathrel"
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
	if act.Type == "folder.trust" {
		return b.trustPreview(effects[0].Path)
	}
	if act.Type == "program.add" || act.Type == "program.relink" {
		return b.programPreview(effects[0])
	}
	if act.Type != "folder.serve" {
		return shellapi.Preview{}, nil
	}
	for _, effect := range effects {
		if effect.Type != "folder.serve" {
			continue
		}
		path, name := effect.Path, effect.Name
		if effect.FolderID != "" {
			// Serving a stopped folder again resumes its entry and keeps its name
			// (serveFolder), so a name typed for a known folder is not used.
			if served := servedNameOf(facts, effect.FolderID); served != "" {
				return b.preview(served), nil
			}
			// A folder never served is served as any other folder: the name the
			// runner would give it, checked by the same rules.
			path = folderPathOf(facts, effect.FolderID)
		}
		if name == "" {
			name = appcore.DefaultServedName(filepath.Base(path), b.a.rt.cfg.ServedNameAvailable)
		}
		entry, refusal := b.a.checkServe(path, name)
		if refusal != nil {
			return shellapi.Preview{}, refusal
		}
		return b.preview(entry.Name), nil
	}
	return shellapi.Preview{}, nil
}

// trustPreview is the Trust sheet's answer: the folder as it will be stored, or
// why it cannot be trusted, and what the person should know first.
func (b shellBackend) trustPreview(path string) (shellapi.Preview, *appcore.Refusal) {
	canonical, err := b.a.rt.policy.Canonical(path)
	if err != nil {
		return shellapi.Preview{}, &appcore.Refusal{Code: appcore.RefuseCannotTrust}
	}
	preview := shellapi.Preview{Path: canonical, Name: filepath.Base(canonical)}
	if b.a.rt.policy.IsPersonal(canonical) {
		preview.Warnings = append(preview.Warnings, shellapi.Warning{Code: "personal-folder", Text: appcore.TextWarnPersonalFolder})
	}
	var inside []string
	for _, tf := range b.a.rt.cfg.TrustedFolderList() {
		if tf.Path != canonical && pathrel.SameOrBelow(tf.Path, canonical) {
			inside = append(inside, filepath.Base(tf.Path))
		}
	}
	if len(inside) > 0 {
		preview.Warnings = append(preview.Warnings, shellapi.Warning{
			Code:  "contains-trusted",
			Text:  fmt.Sprintf(appcore.TextWarnContainsTrusted, strings.Join(inside, ", ")),
			Names: inside,
		})
	}
	return preview, nil
}

// programPreview is what the add and relink sheets show: the file as it will be
// stored and the codes the person has to acknowledge, which are the same ones the
// effect checks again.
func (b shellBackend) programPreview(effect appcore.Effect) (shellapi.Preview, *appcore.Refusal) {
	codes, refusal := b.a.programFileWarnings(effect.Path)
	if refusal != nil {
		return shellapi.Preview{}, refusal
	}
	preview := shellapi.Preview{Path: effect.Path}
	for _, code := range codes {
		preview.Warnings = append(preview.Warnings, shellapi.Warning{Code: code})
	}
	return preview, nil
}

// Answer is the remembered outcome of an action id in this epoch, so a reply the
// Mac app lost does not have to be sent again.
func (b shellBackend) Answer(id string) (appcore.Snapshot, *appcore.Refusal, bool) {
	b.a.initCore()
	b.a.core.mu.Lock()
	answer, ok := b.a.core.answers[id]
	b.a.core.mu.Unlock()
	if !ok {
		return appcore.Snapshot{}, nil, false
	}
	if answer.Running {
		return b.a.snapshot(), &appcore.Refusal{Code: appcore.RefuseStillRunning}, true
	}
	return answer.Snapshot, answer.Refusal, true
}

func (b shellBackend) Subscribe() (<-chan appcore.Snapshot, func()) { return b.a.subscribe() }

// preview is the address a folder served under name answers at.
func (b shellBackend) preview(name string) shellapi.Preview {
	return shellapi.Preview{Name: name, Address: servedHostName(name, b.a.rt.port)}
}

// servedNameOf is the served name the facts give folder id, or "" when the folder
// has never been served.
func servedNameOf(facts appcore.Facts, id string) string {
	for _, folder := range facts.Folders {
		if folder.ID == id {
			return folder.ServedName
		}
	}
	return ""
}

// folderPathOf is the path the facts give folder id, or "" when they do not list it.
func folderPathOf(facts appcore.Facts, id string) string {
	for _, folder := range facts.Folders {
		if folder.ID == id {
			return folder.Path
		}
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
// line. Shutdown runs it right after the live-room service's final save.
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
