//go:build managedtrial

package managedtrial

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/panphora/htmlclay/internal/specwire"
)

// artifact-paths.js's TAILWIND_DIR: the one folder a site's generated
// stylesheets live in.
const tailwindDirName = ".hyperclay/tailwindcss"

// tailwindMaxCompiles is how many compiles may run at once. A save answers
// without waiting for its compile, so two saves in quick succession start two,
// and no more run beside them.
const tailwindMaxCompiles = 2

// tailwindSlots is the process-wide bound on compiles in flight.
var tailwindSlots = make(chan struct{}, tailwindMaxCompiles)

// tailwind-compile.cjs is the compiler this host runs: tailwind-hyperclay's own
// compileTailwind, loaded from the Local checkout named by --local-source, so
// the stylesheet is byte-identical to the one Local writes. HTML Clay's own
// `internal/tailwind` runs a different compiler with different plugins, which is
// why this host does not use it.
//
//go:embed tailwind-compile.cjs
var tailwindScript []byte

// tailwindScriptPath is the embedded script written once per process for node to
// run.
var (
	tailwindScriptOnce sync.Once
	tailwindScriptFile string
	tailwindScriptErr  error
)

// tailwindCompiler is the pinned compiler, replaceable by a test.
var tailwindCompiler = func(localSource, html string) (string, error) {
	return compileTailwindWithNode(localSource, html)
}

// compileTailwindWithNode is tailwind-hyperclay's compileTailwind over one
// document: the script on disk, the document on stdin, the CSS on stdout.
func compileTailwindWithNode(localSource, html string) (string, error) {
	script, err := tailwindScriptPath()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("node", script, "--local-source", localSource)
	cmd.Stdin = bytes.NewBufferString(html)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := stderr.String()
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", message)
	}
	return stdout.String(), nil
}

func tailwindScriptPath() (string, error) {
	tailwindScriptOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), fmt.Sprintf("htmlclay-managed-trial-%d", os.Getpid()))
		if err := os.MkdirAll(dir, 0o777); err != nil {
			tailwindScriptErr = err
			return
		}
		path := filepath.Join(dir, "tailwind-compile.cjs")
		if err := os.WriteFile(path, tailwindScript, 0o666); err != nil {
			tailwindScriptErr = err
			return
		}
		tailwindScriptFile = path
	})
	return tailwindScriptFile, tailwindScriptErr
}

// tailwindHost is one root's Tailwind work: the compiles a save starts, the
// compile a read starts when it misses, and the record of which stylesheets are
// being compiled, so a read can wait for one.
type tailwindHost struct {
	baseDir     string
	localSource string
	registry    *consentRegistry
	mu          sync.Mutex
	running     map[string]*tailwindRun
}

// tailwindRun is the compiles in flight for one stylesheet. done closes when the
// last of them finishes.
type tailwindRun struct {
	count int
	done  chan struct{}
}

func newTailwindHost(baseDir, localSource string, registry *consentRegistry) *tailwindHost {
	return &tailwindHost{
		baseDir:     baseDir,
		localSource: localSource,
		registry:    registry,
		running:     make(map[string]*tailwindRun),
	}
}

// refresh is a save's Tailwind refresh, from after the commit has published: the
// compile is registered before it starts, so a read that follows the save waits
// for it, and the save answers without waiting for the compile.
func (t *tailwindHost) refresh(content string) error {
	name := tailwindCSSName(content)
	if name == "" {
		return nil
	}
	t.begin(name)
	go func() {
		defer t.end(name)
		t.compile(name, content)
	}()
	return nil
}

// begin registers one compile of a stylesheet.
func (t *tailwindHost) begin(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	run := t.running[name]
	if run == nil {
		run = &tailwindRun{done: make(chan struct{})}
		t.running[name] = run
	}
	run.count++
}

// end retires one compile of a stylesheet, releasing the reads waiting on it.
func (t *tailwindHost) end(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	run := t.running[name]
	if run == nil {
		return
	}
	run.count--
	if run.count == 0 {
		delete(t.running, name)
		close(run.done)
	}
}

// wait blocks while a compile of this stylesheet is running.
func (t *tailwindHost) wait(name string) {
	t.mu.Lock()
	run := t.running[name]
	t.mu.Unlock()
	if run != nil {
		<-run.done
	}
}

// compile writes the stylesheet for the document a save published, and only
// while the document still holds the bytes it compiled from: a newer compile is
// coming or has already written its own. A failure leaves the old stylesheet, as
// in Local.
func (t *tailwindHost) compile(name, content string) {
	tailwindSlots <- struct{}{}
	defer func() { <-tailwindSlots }()

	css, err := tailwindCompiler(t.localSource, content)
	if err != nil {
		log.Printf("compileTailwind failed (non-fatal): %v", err)
		return
	}
	file, _ := splitPage(content)
	// The check and the write run under the document's lock, the lock a save
	// takes, so no save can land between them and be overtaken by this
	// stylesheet.
	sourcePath, err := t.sourceLockPath(name)
	if err != nil {
		log.Printf("compileTailwind failed (non-fatal): %v", err)
		return
	}
	unlock := fileLocks.lock(sourcePath)
	defer unlock()
	if !t.stillStored(name, file) {
		return
	}
	cssPath, err := t.writePath(name)
	if err != nil {
		log.Printf("compileTailwind failed (non-fatal): %v", err)
		return
	}
	if err := atomicWriteFile(cssPath, []byte(css), nil, true); err != nil {
		log.Printf("compileTailwind failed (non-fatal): %v", err)
	}
}

// stillStored reports whether the document this stylesheet is named for still
// holds the bytes this compile read. The file on disk is the same file part the
// save wrote, so its checksum is the guard.
func (t *tailwindHost) stillStored(name, file string) bool {
	stored, err := readTailwindDocument(t.baseDir, name)
	if err != nil {
		return false
	}
	return specwire.Etag(stored) == specwire.Etag([]byte(file))
}

// compileOnMiss is the read path's compile: the document is read and compiled
// now, and the stylesheet is re-checked first, since a compile this one queued
// behind may already have written a fresher one.
func (t *tailwindHost) compileOnMiss(cssPath, htmlPath string) (string, error) {
	tailwindSlots <- struct{}{}
	defer func() { <-tailwindSlots }()

	if css, err := os.ReadFile(cssPath); err == nil {
		return string(css), nil
	}
	html, err := os.ReadFile(htmlPath)
	if err != nil {
		return "", err
	}
	css, err := tailwindCompiler(t.localSource, string(html))
	if err != nil {
		return "", err
	}
	if err := atomicWriteFile(cssPath, []byte(css), nil, true); err != nil {
		return "", err
	}
	return css, nil
}

// writePath is server.js's resolveTailwindWrite: only the name comes from the
// document or the URL, so only the name is validated, and the reserved
// `.hyperclay/` prefix is added afterwards.
func (t *tailwindHost) writePath(name string) (string, error) {
	if _, err := validateSegments(name + ".css"); err != nil {
		return "", err
	}
	return resolveWritePath(t.registry, upathJoin(tailwindDirName, name+".css"))
}

// sourcePath is a stylesheet's source document through the same phase-2 and
// phase-4 pass as a user file.
func (t *tailwindHost) sourcePath(relPath string) (string, error) {
	if _, err := validateSegments(relPath); err != nil {
		return "", err
	}
	return resolveWritePath(t.registry, relPath)
}

// sourceLockPath is the path the commit lanes lock for the document a
// stylesheet is named for: its `.html`, or its `.htmlclay` when only that exists,
// as readTailwindDocument reads them.
func (t *tailwindHost) sourceLockPath(name string) (string, error) {
	htmlPath, err := t.sourcePath(name + ".html")
	if err != nil {
		return "", err
	}
	if _, statErr := os.Lstat(htmlPath); !os.IsNotExist(statErr) {
		return htmlPath, nil
	}
	return t.sourcePath(name + ".htmlclay")
}

// readTailwindDocument is the document a stylesheet's name stands for: the
// `.html` a site is called by, or its `.htmlclay`.
func readTailwindDocument(baseDir, name string) ([]byte, error) {
	raw, err := os.ReadFile(upathJoin(baseDir, name+".html"))
	if err == nil {
		return raw, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return os.ReadFile(upathJoin(baseDir, name+".htmlclay"))
}

// RefreshTailwind is the commit core's Tailwind hook (G6a). Local awaits the
// compile inside the save; this host starts it once the commit has published and
// answers without waiting.
func (h *liveHooks) RefreshTailwind(content string) error {
	if h.tailwind == nil {
		return nil
	}
	return h.tailwind.refresh(content)
}
