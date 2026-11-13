//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestTailwindGolden pins GET /tailwindcss/<name>.css to Local's own answers: the
// compiled stylesheet on a miss, the stored one after that, a hand-written one
// served as stored, a nested name, a name no page stands for, a traversal sent
// raw, and the stylesheet a save's compile publishes.
//
// The data guard's records under `.hyperclay/guard/` are left out for the reason
// TestSaveGolden leaves them out: G7 owns them.
func TestTailwindGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and the handler reads that checkout's bundled runtime release")
	}
	runGoldenCasesWith(t, filepath.Join("testdata", "tailwind-golden"), localSource, goldenOptions{
		skipFile: func(path string) bool {
			return path == ".hyperclay/guard" || strings.HasPrefix(path, ".hyperclay/guard/")
		},
	})
}

// TestTailwindCssName checks the ported getTailwindCssName against
// tailwind-hyperclay's own, over every page the Tailwind golden holds.
func TestTailwindCssName(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the JS getTailwindCssName lives in that checkout")
	}
	root, err := filepath.Abs(filepath.Join("testdata", "tailwind-golden", "root"))
	if err != nil {
		t.Fatal(err)
	}
	pages := []string{"tw.html", "sub/page.html", "plain.html", "stale.html"}

	paths := make([]string, len(pages))
	for i, page := range pages {
		paths[i] = filepath.Join(root, filepath.FromSlash(page))
	}
	script := `
const fs = require('fs')
const path = require('path')
const { getTailwindCssName } = require(path.join(process.argv[1], 'node_modules', 'tailwind-hyperclay'))
const pages = JSON.parse(process.argv[2])
process.stdout.write(JSON.stringify(pages.map((page) => getTailwindCssName(fs.readFileSync(page, 'utf8')))))
`
	cmd := exec.Command("node", "-e", script, localSource, mustJSON(paths))
	answer, err := cmd.Output()
	if err != nil {
		message := err.Error()
		if exit, ok := err.(*exec.ExitError); ok {
			message += ": " + string(exit.Stderr)
		}
		t.Fatalf("node getTailwindCssName failed: %s", message)
	}
	var want []*string
	if err := json.Unmarshal(answer, &want); err != nil {
		t.Fatalf("node getTailwindCssName answered %q: %v", answer, err)
	}

	for i, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got := tailwindCSSName(string(raw))
		expected := ""
		if want[i] != nil {
			expected = *want[i]
		}
		if got != expected {
			t.Errorf("tailwindCSSName(%s) = %q, JS = %q", pages[i], got, expected)
		}
	}
}

// TestTailwindNewerBytesWin is P11's newer-bytes guard: the first save's compile
// is blocked until the second save's compile has finished, and the stylesheet
// the first would write is dropped because the document no longer holds its
// bytes.
func TestTailwindNewerBytesWin(t *testing.T) {
	localSource := requireTailwindLocalSource(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, root, "tw.html", []byte(tailwindPage("flex p-4")))

	first, second := tailwindPage("flex p-4 text-red-500"), tailwindPage("flex p-4 text-emerald-600")
	firstStarted := make(chan struct{})
	secondFinished := make(chan struct{})
	releaseFirst := make(chan struct{})
	stubTailwindCompiler(t, firstStarted, secondFinished, releaseFirst)

	handler, err := newRootHandler(rootSpec{id: "root-tailwind-newer", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	address, port := serveTestServer(t, handler)

	tailwindSave(t, address, port, "tw.html", first)
	<-firstStarted
	tailwindSave(t, address, port, "tw.html", second)
	// The second compile's write has landed before the first is let go, so a
	// first compile that writes now would be the one left on disk.
	waitForTailwindCSS(t, tailwindCSSPath(root, "tw"), tailwindCSSFor(second))
	close(releaseFirst)
	handler.tailwind.wait("tw")

	if got := readTailwindCSS(t, root, "tw"); got != tailwindCSSFor(second) {
		t.Errorf("stylesheet = %q, want the second save's %q", got, tailwindCSSFor(second))
	}
}

// TestTailwindInstallHoldsTheDocumentLock is P11's ordering rule at its narrowest:
// a compile checks that its document is still stored and writes its stylesheet
// under the document's lock, so a save that lands between the check and the
// write cannot be overtaken by the older compile's stylesheet.
func TestTailwindInstallHoldsTheDocumentLock(t *testing.T) {
	localSource := requireTailwindLocalSource(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, second := tailwindPage("flex p-4 text-red-500"), tailwindPage("flex p-4 text-emerald-600")
	writeCommitFile(t, root, "tw.html", []byte(first))

	previous := tailwindCompiler
	tailwindCompiler = func(localSource, html string) (string, error) { return tailwindCSSFor(html), nil }
	t.Cleanup(func() { tailwindCompiler = previous })

	handler, err := newRootHandler(rootSpec{id: "root-tailwind-lock", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	docPath := upathJoin(root, "tw.html")

	// The test is the save: it holds the document's lock while the first
	// compile runs, and replaces the document before letting go.
	unlock := fileLocks.lock(docPath)
	if err := handler.tailwind.refresh(first); err != nil {
		unlock()
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := os.WriteFile(docPath, []byte(second), 0o644); err != nil {
		unlock()
		t.Fatal(err)
	}
	unlock()
	handler.tailwind.wait("tw")

	if _, err := os.Stat(tailwindCSSPath(root, "tw")); !os.IsNotExist(err) {
		t.Errorf("the first compile wrote a stylesheet for bytes no longer stored (stat err %v): %q", err, readTailwindCSS(t, root, "tw"))
	}
}

// TestTailwindReadWaits is P11's no-stale-reads rule: a read that arrives while a
// save's compile is running waits for it, so it answers the new stylesheet
// rather than the one already on disk.
func TestTailwindReadWaits(t *testing.T) {
	localSource := requireTailwindLocalSource(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved := tailwindPage("flex p-4 text-emerald-600")
	writeCommitFile(t, root, "tw.html", []byte(tailwindPage("flex p-4")))
	writeCommitFile(t, root, filepath.Join(".hyperclay", "tailwindcss", "tw.css"), []byte("/* stale */\n"))

	firstStarted := make(chan struct{})
	secondFinished := make(chan struct{})
	releaseFirst := make(chan struct{})
	stubTailwindCompiler(t, firstStarted, secondFinished, releaseFirst)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()

	handler, err := newRootHandler(rootSpec{id: "root-tailwind-read", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	address, port := serveTestServer(t, handler)

	tailwindSave(t, address, port, "tw.html", saved)
	<-firstStarted

	type answer struct {
		body string
		err  error
	}
	answered := make(chan answer, 1)
	go func() {
		response, err := http.Get(fmt.Sprintf("http://%s/tailwindcss/tw.css", address))
		if err != nil {
			answered <- answer{err: err}
			return
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		answered <- answer{body: string(body), err: err}
	}()

	select {
	case got := <-answered:
		t.Fatalf("the read answered %q before the save's compile finished", got.body)
	case <-time.After(300 * time.Millisecond):
	}

	release()
	got := <-answered
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.body != tailwindCSSFor(saved) {
		t.Errorf("read = %q, want the save's %q", got.body, tailwindCSSFor(saved))
	}
}

func requireTailwindLocalSource(t *testing.T) string {
	t.Helper()
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the handler reads that checkout's bundled runtime release")
	}
	return localSource
}

// stubTailwindCompiler replaces the pinned compiler for one test with an answer
// it can hold open, so the order two compiles finish in is the test's to decide.
func stubTailwindCompiler(t *testing.T, firstStarted, secondFinished, releaseFirst chan struct{}) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	previous := tailwindCompiler
	tailwindCompiler = func(localSource, html string) (string, error) {
		mu.Lock()
		calls++
		index := calls
		mu.Unlock()
		switch index {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(secondFinished)
		}
		return tailwindCSSFor(html), nil
	}
	t.Cleanup(func() { tailwindCompiler = previous })
}

// tailwindCSSFor is a fake compiler's answer for one document: distinct per
// document, so which compile wrote the stylesheet is readable.
func tailwindCSSFor(html string) string {
	return fmt.Sprintf("/* %x */\n", sha256.Sum256([]byte(html)))
}

func tailwindPage(classes string) string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Tailwind</title>
<link rel="stylesheet" href="/tailwindcss/tw.css">
</head>
<body class="` + classes + `">
<p>Tailwind.</p>
</body>
</html>
`
}

func tailwindCSSPath(root, name string) string {
	return filepath.Join(root, filepath.FromSlash(tailwindDirName), name+".css")
}

func readTailwindCSS(t *testing.T, root, name string) string {
	t.Helper()
	raw, err := os.ReadFile(tailwindCSSPath(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func waitForTailwindCSS(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err := os.ReadFile(path)
		if err == nil && string(raw) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stylesheet = %q, want %q", raw, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func tailwindSave(t *testing.T, address string, port int, name, body string) {
	t.Helper()
	headers := map[string]string{
		"Document-URL": fmt.Sprintf("http://127.0.0.1:%d/%s", port, name),
		"Content-Type": "text/html",
	}
	status, _, answer := serveSend(t, address, http.MethodPost, "/_/save", headers, []byte(body), port)
	if status != http.StatusOK {
		t.Fatalf("save of %s = %d (%s)", name, status, answer)
	}
}
