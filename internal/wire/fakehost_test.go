package wire

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/logging"
)

// The test binary doubles as the registered program. The dispatcher starts a
// program with no arguments and passes this process's environment through, so
// the behaviour is chosen by environment variable.
const (
	wireHelperMode = "HTMLCLAY_WIRE_HELPER_MODE"
	wireHelperArg  = "HTMLCLAY_WIRE_HELPER_ARG"
)

// TestMain pins HOME for the whole package so nothing here touches the real
// ~/htmlclay, including every helper process this binary re-runs itself as.
func TestMain(m *testing.M) {
	if mode := os.Getenv(wireHelperMode); mode != "" {
		runWireHelperProcess(mode, os.Getenv(wireHelperArg))
		return
	}
	home, err := os.MkdirTemp("", "htmlclay-test-home")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test home:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func structuredHelper(t *testing.T, mode string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(wireHelperMode, mode)
	return exe
}

func runWireHelperProcess(mode, arg string) {
	switch mode {
	case "marker":
		io.ReadAll(os.Stdin)
		if err := os.WriteFile(arg, []byte("ran\n"), 0644); err != nil {
			fmt.Println(`{"type":"error","code":"marker_failed","message":"cannot write the marker"}`)
			return
		}
		fmt.Println(`{"type":"result","value":null}`)
	}
}

func writeHelperDocument(t *testing.T, path string, names ...string) {
	t.Helper()
	var metas strings.Builder
	for _, name := range names {
		fmt.Fprintf(&metas, "<meta name=\"htmlclay-helper\" content=\"%s\">", name)
	}
	if err := os.WriteFile(path, []byte("<!doctype html><head>"+metas.String()+"</head><body></body>"), 0644); err != nil {
		t.Fatal(err)
	}
}

func observer(t *testing.T, c *Core, file string) *Sub {
	t.Helper()
	sub := &Sub{Key: file, Ch: make(chan []byte, 64), Done: make(chan struct{})}
	if _, _, err := c.Hub.Add(sub, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Hub.Remove(sub) })
	return sub
}

func receiveHelperEnvelope(t *testing.T, sub *Sub) Envelope {
	t.Helper()
	select {
	case raw := <-sub.Ch:
		env, err := DecodeEnvelope(raw)
		if err != nil {
			t.Fatal(err)
		}
		return env
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for helper frame")
		return Envelope{}
	}
}

func receiveHelperType(t *testing.T, sub *Sub, typ, id string) Envelope {
	t.Helper()
	for i := 0; i < 8; i++ {
		env := receiveHelperEnvelope(t, sub)
		if env.Type == typ && env.ID == id {
			return env
		}
	}
	t.Fatalf("did not receive %s for %s", typ, id)
	return Envelope{}
}

func helperErrorCode(t *testing.T, env Envelope) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Code
}

func helperProgram(name, path string) config.HelperProgram {
	return config.HelperProgram{ID: "program-" + name, Name: name, Path: path}
}

type fakeDocs struct {
	root   string
	mu     sync.Mutex
	mayUse bool
	leases int
	pokes  int
}

func (f *fakeDocs) Caller(r *http.Request) (isBrowser, ok bool) {
	if r.Header.Get("Sec-Fetch-Site") == "same-origin" {
		return true, true
	}
	if r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Origin") == "" {
		return false, true
	}
	return false, false
}

func (f *fakeDocs) Target(r *http.Request, isBrowser bool, supplied string) (Document, bool) {
	if isBrowser {
		u, err := url.Parse(PageURL(r))
		if err != nil || u.Path == "" {
			return Document{}, false
		}
		return f.At(filepath.Join(f.root, path.Base(u.Path)))
	}
	return f.At(supplied)
}

func (f *fakeDocs) At(p string) (Document, bool) {
	if filepath.Dir(p) != f.root {
		return Document{}, false
	}
	if _, err := os.Stat(p); err != nil {
		return Document{}, false
	}
	return Document{Path: p, Name: filepath.Base(p)}, true
}

func (f *fakeDocs) MayUsePrograms(doc Document, isBrowser bool, presented string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mayUse
}

func (f *fakeDocs) OpenHistory(doc Document) error { return nil }
func (f *fakeDocs) Lease(doc Document)             { f.mu.Lock(); f.leases++; f.mu.Unlock() }
func (f *fakeDocs) Unlease(doc Document)           { f.mu.Lock(); f.leases--; f.mu.Unlock() }
func (f *fakeDocs) Poke(doc Document)              { f.mu.Lock(); f.pokes++; f.mu.Unlock() }
func (f *fakeDocs) Internal(absPath string) bool   { return false }

type fakePolicy struct {
	mu        sync.Mutex
	granted   bool
	aiOffered bool
	aiOn      bool
	aiAsk     bool
	asked     []string

	builtin      builtin.Access
	askedBuiltin []string
	revealed     []string
}

func (p *fakePolicy) CanAskProgram() bool               { return false }
func (p *fakePolicy) AskProgram(file, name string) bool { return false }
func (p *fakePolicy) ProgramGranted(file, name, id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.granted
}
func (p *fakePolicy) AIEditOffered() bool           { return p.aiOffered }
func (p *fakePolicy) AIEditFor(string) (bool, bool) { return p.aiOn, p.aiAsk }
func (p *fakePolicy) AskAIEdit(document string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, document)
}
func (p *fakePolicy) AIEditEngines() (string, map[string][]string) { return "", nil }
func (p *fakePolicy) AIEditModels() map[string]string              { return nil }
func (p *fakePolicy) AIEditEfforts() map[string]string             { return nil }

func (p *fakePolicy) BuiltinFor(string, string) builtin.Access { return p.builtin }

func (p *fakePolicy) AskBuiltin(document, program string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.askedBuiltin = append(p.askedBuiltin, program+":"+document)
}

func (p *fakePolicy) RevealFile(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.revealed = append(p.revealed, path)
	return nil
}

func newFakeCore(t *testing.T) (*Core, *fakeDocs, *fakePolicy, string) {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	docs := &fakeDocs{root: root, mayUse: true}
	policy := &fakePolicy{granted: true}
	core := NewCore(docs, policy, logging.NewStdout(), func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
	t.Cleanup(core.Shutdown)
	file := filepath.Join(root, "doc.htmlclay")
	writeHelperDocument(t, file, "marker")
	return core, docs, policy, file
}
