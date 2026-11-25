package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/carried"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/wire"
)

// programsHost is the app's side of the programs wire helper: it reads a
// document's program tags with the trust of those exact bytes, shares the
// page's folder with ai-tools, and runs a program from the store in a worker.
type programsHost struct{ a *app }

func (a *app) programsHost() wire.ProgramsHost { return programsHost{a} }

// carriedStore is the store carried programs run from, or nil when it
// cannot be made.
func (a *app) carriedStore() *carried.Store {
	a.programsOnce.Do(func() {
		store, err := carried.OpenStore(filepath.Join(a.rt.configDir, "carried-programs"))
		if err != nil {
			a.rt.logger.Printf("Carried programs are off: %v", err)
			return
		}
		a.programStore = store
	})
	return a.programStore
}

func (h programsHost) Enabled() bool {
	return h.a.rt.cfg.AIEditEnabled() && h.a.carriedStore() != nil
}

func (h programsHost) HasFolder(document string) bool { return aiToolsHost{h.a}.HasFolder(document) }

func (h programsHost) Folder(ctx context.Context, document, program string, choose bool, level string, asking func()) (string, bool, error) {
	return h.a.pageFolder(ctx, document, program, choose, level, asking)
}

var errProgramFile = &carried.Error{Code: carried.CodeProgramMissing, Message: "HTML Clay cannot read this page's file."}

// Load reads the document once and takes the program tags from the same
// bytes the verdict was made for, so a save in between cannot slip in code
// nobody checked.
func (h programsHost) Load(document string) (wire.ProgramSet, error) {
	path, err := resolveSymlinks(document)
	if err != nil {
		return wire.ProgramSet{}, errProgramFile
	}
	v, data := h.a.readTier(path)
	if data == nil {
		return wire.ProgramSet{}, errProgramFile
	}
	return wire.ProgramSet{
		Trust: wire.Trust{Tier: v.Tier.String(), Ready: !v.Warn() && !v.Denied, Denied: v.Denied},
		Scan:  htmlutil.ReadPrograms(data),
	}, nil
}

// Run keeps every module in the store and runs the copies read back from
// it, in a worker started from this binary.
func (h programsHost) Run(ctx context.Context, job carried.Job) (carried.Result, error) {
	store := h.a.carriedStore()
	if store == nil {
		return carried.Result{}, &carried.Error{Code: carried.CodeUnavailable, Message: "Carried programs are not available in this copy of HTML Clay."}
	}
	kept, err := store.Keep(job.Modules)
	if err != nil {
		h.a.rt.logger.Printf("Could not keep a program to run: %v", err)
		return carried.Result{}, &carried.Error{Code: carried.CodeStoreFailed, Message: "HTML Clay could not keep a copy of the program to run."}
	}
	exe, err := os.Executable()
	if err != nil {
		return carried.Result{}, &carried.Error{Code: carried.CodeUnavailable, Message: "HTML Clay could not find itself to start the program."}
	}
	job.Modules = kept
	job.Worker = exe
	if s := h.a.aiToolsService(); s != nil {
		job.Gate = s.Gate()
	}
	return carried.Run(ctx, job)
}
