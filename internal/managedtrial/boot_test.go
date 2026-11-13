//go:build managedtrial && !windows

package managedtrial

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRunServesEveryRootAndStopsOnStdinEOF(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	source := writeLocalSource(t)

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinReader.Close()

	secret := strings.Repeat("0123456789abcdef", 4)
	secretReader, secretWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(secretWriter, secret); err != nil {
		t.Fatal(err)
	}
	secretWriter.Close()
	// Run owns the descriptor it is given and closes it once the secret is read,
	// so it gets its own copy. Handing it secretReader's would leave two files on
	// one descriptor number, and the second close would land on whatever reused
	// the number by then, such as Run's own listener.
	controlFD, err := syscall.Dup(int(secretReader.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	secretReader.Close()

	stdoutReader, stdoutWriter := io.Pipe()
	stderr := &syncBuffer{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	args := []string{
		"--root", rootA,
		"--root", rootB,
		"--port", "0",
		"--local-source", source,
		"--control-fd", strconv.Itoa(controlFD),
	}
	done := make(chan error, 1)
	go func() { done <- Run(ctx, args, stdinReader, stdoutWriter, stderr) }()

	ready := readReady(t, stdoutReader)

	if ready.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", ready.PID, os.Getpid())
	}
	if len(ready.Roots) != 2 {
		t.Fatalf("got %d roots, want 2", len(ready.Roots))
	}
	wantPaths := []string{realPathOrFail(t, rootA), realPathOrFail(t, rootB)}
	ports := make(map[string]string, 2)
	for i, root := range ready.Roots {
		if root.ID != fmt.Sprintf("root-%d", i) {
			t.Fatalf("root %d id = %q", i, root.ID)
		}
		if root.Path != wantPaths[i] {
			t.Fatalf("root %d path = %q, want real path %q", i, root.Path, wantPaths[i])
		}
		u, err := url.Parse(root.URL)
		if err != nil {
			t.Fatalf("root %d url %q: %v", i, root.URL, err)
		}
		if u.Hostname() != "127.0.0.1" {
			t.Fatalf("root %d binds %q, want 127.0.0.1", i, u.Hostname())
		}
		if other, ok := ports[u.Host]; ok {
			t.Fatalf("roots %s and %d share a port", other, i)
		}
		ports[u.Host] = root.ID
	}
	if ready.Control == "" {
		t.Fatal("control url is empty")
	}
	if ready.Control == ready.Roots[0].URL || ready.Control == ready.Roots[1].URL {
		t.Fatalf("control shares a listener with a root: %s", ready.Control)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, root := range ready.Roots {
		status, body, contentType := get(t, client, root.URL+"/anything", nil)
		if status != http.StatusNotFound {
			t.Fatalf("GET %s/anything = %d, want 404", root.URL, status)
		}
		if body != "File not found" {
			t.Fatalf("root body = %q, want %q", body, "File not found")
		}
		if contentType != "text/html; charset=utf-8" {
			t.Fatalf("root content type = %q", contentType)
		}
	}

	status, body, _ := get(t, client, ready.Control+"/health", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("health without a secret = %d, want 401", status)
	}
	if body != `{"error":"unauthorized"}` {
		t.Fatalf("unauthorized body = %q", body)
	}
	status, _, _ = get(t, client, ready.Control+"/health", map[string]string{
		"Authorization": "Bearer " + strings.Repeat("f", 64),
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("health with the wrong secret = %d, want 401", status)
	}
	status, body, contentType := get(t, client, ready.Control+"/health", map[string]string{
		"Authorization": "Bearer " + secret,
	})
	if status != http.StatusOK {
		t.Fatalf("health with the secret = %d, want 200", status)
	}
	if contentType != "application/json" {
		t.Fatalf("health content type = %q", contentType)
	}
	if !strings.Contains(body, `"roots":2`) {
		t.Fatalf("health body = %q, want roots 2", body)
	}
	status, _, _ = get(t, client, ready.Control+"/other", map[string]string{
		"Authorization": "Bearer " + secret,
	})
	if status != http.StatusNotFound {
		t.Fatalf("GET /other with the secret = %d, want 404", status)
	}

	stdinWriter.Close()
	if err := waitForRun(t, done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	for _, root := range ready.Roots {
		assertRefused(t, root.URL)
	}
	assertRefused(t, ready.Control)
}

func TestRunStopsOnContextCancel(t *testing.T) {
	root := t.TempDir()
	source := writeLocalSource(t)

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinWriter.Close()
	defer stdinReader.Close()

	stdoutReader, stdoutWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []string{"--root", root, "--local-source", source}, stdinReader, stdoutWriter, io.Discard)
	}()

	readReady(t, stdoutReader)

	cancel()
	if err := waitForRun(t, done); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestRefuseLiveRoot(t *testing.T) {
	live := t.TempDir()
	inside := filepath.Join(live, "app")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, root := range []string{live, inside, filepath.Join(live, "does-not-exist")} {
		err := RefuseLiveRoot(root, live)
		if err == nil {
			t.Fatalf("%q was not refused", root)
		}
		if !strings.Contains(err.Error(), "LOCAL_APPS") {
			t.Fatalf("%q error %q does not name LOCAL_APPS", root, err)
		}
	}

	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "shortcut")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	if err := RefuseLiveRoot(link, live); err == nil {
		t.Fatalf("symlink %q into the live folder was not refused", link)
	} else if !strings.Contains(err.Error(), "LOCAL_APPS") {
		t.Fatalf("symlink error %q does not name LOCAL_APPS", err)
	}

	sibling := t.TempDir()
	if err := RefuseLiveRoot(sibling, live); err != nil {
		t.Fatalf("sibling %q was refused: %v", sibling, err)
	}

	for _, parent := range []string{filepath.Dir(live), filepath.Dir(filepath.Dir(live)), string(filepath.Separator)} {
		err := RefuseLiveRoot(parent, live)
		if err == nil {
			t.Fatalf("parent %q of the live folder was not refused", parent)
		}
		if !strings.Contains(err.Error(), "contains") || !strings.Contains(err.Error(), "LOCAL_APPS") {
			t.Fatalf("parent %q error %q does not say it contains LOCAL_APPS", parent, err)
		}
	}

	defaultLive, err := DefaultLiveRoot()
	if err != nil {
		t.Fatalf("DefaultLiveRoot: %v", err)
	}
	if !filepath.IsAbs(defaultLive) || !strings.HasSuffix(defaultLive, "LOCAL_APPS") {
		t.Fatalf("DefaultLiveRoot = %q", defaultLive)
	}
	name := fmt.Sprintf("does-not-exist-%d", time.Now().UnixNano())
	if err := RefuseLiveRoot(filepath.Join(defaultLive, name), defaultLive); err == nil {
		t.Fatalf("%q under the default live root was not refused", name)
	}
	if _, err := os.Stat(filepath.Join(defaultLive, name)); !os.IsNotExist(err) {
		t.Fatalf("%q exists, the check must not create anything", name)
	}
}

// TestRefuseLiveRootOtherCase is the guard on a case-insensitive volume: another
// spelling of the live folder, of a folder inside it, or of its parent is the same
// folder, and is refused.
func TestRefuseLiveRootOtherCase(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "Live", "LOCAL_APPS")
	if err := os.MkdirAll(filepath.Join(live, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "live", "local_apps")); err != nil {
		t.Skip("this volume is case-sensitive")
	}
	for _, root := range []string{
		filepath.Join(base, "live", "local_apps"),
		filepath.Join(base, "live", "local_apps", "APP"),
		filepath.Join(base, "live", "local_apps", "new-folder"),
	} {
		if err := RefuseLiveRoot(root, live); err == nil || !strings.Contains(err.Error(), "inside") {
			t.Fatalf("%q: err = %v, want refused as inside LOCAL_APPS", root, err)
		}
	}
	if err := RefuseLiveRoot(filepath.Join(base, "LIVE"), live); err == nil || !strings.Contains(err.Error(), "contains") {
		t.Fatalf("other-case parent: err = %v, want refused as containing LOCAL_APPS", err)
	}
	if err := RefuseLiveRoot(filepath.Join(base, "other"), live); err != nil {
		t.Fatalf("a sibling folder was refused: %v", err)
	}
}

// TestConsentRefusesSymlinkIntoLiveRoot: a link inside a served root that points
// into the live folder is never consented, so no write resolves through it, while
// a link to any other folder still is.
func TestConsentRefusesSymlinkIntoLiveRoot(t *testing.T) {
	live := filepath.Join(t.TempDir(), "LOCAL_APPS")
	other := t.TempDir()
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := liveRoot
	liveRoot = func() (string, error) { return realPath(live) }
	t.Cleanup(func() { liveRoot = saved })

	root := t.TempDir()
	if err := os.Symlink(live, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(other, filepath.Join(root, "other")); err != nil {
		t.Fatal(err)
	}
	registry := newConsentRegistry(root, nil)
	registry.Ready()
	liveReal, _ := realPath(live)
	otherReal, _ := realPath(other)
	consented := registry.ConsentedRoots()
	if containsString(consented, upathResolve(liveReal)) {
		t.Fatalf("the live folder was consented: %v", consented)
	}
	if !containsString(consented, upathResolve(otherReal)) {
		t.Fatalf("the other folder was not consented: %v", consented)
	}
	if path, err := resolveWritePath(registry, "link/new.html"); err == nil {
		t.Fatalf("a write through the link resolved to %q", path)
	}
}

func TestRunRefusesBadArguments(t *testing.T) {
	root := t.TempDir()
	source := writeLocalSource(t)
	missingRoot := filepath.Join(t.TempDir(), "missing")
	shortSecret := "0123456789abcdef"

	run := func(args []string) error {
		return Run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no root", []string{"--local-source", source}, "--root is required"},
		{"missing root", []string{"--root", missingRoot, "--local-source", source}, "--root"},
		{"duplicate root", []string{"--root", root, "--root", root, "--local-source", source}, "twice"},
		{"no local source", []string{"--root", root}, "--local-source"},
		{"local source without server.js", []string{"--root", root, "--local-source", t.TempDir()}, "--local-source"},
	}
	for _, tc := range cases {
		err := run(tc.args)
		if err == nil {
			t.Fatalf("%s: Run accepted %v", tc.name, tc.args)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error %q does not contain %q", tc.name, err, tc.want)
		}
	}

	secretReader, secretWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer secretReader.Close()
	if _, err := io.WriteString(secretWriter, shortSecret); err != nil {
		t.Fatal(err)
	}
	secretWriter.Close()
	err = run([]string{"--root", root, "--local-source", source, "--control-fd", strconv.Itoa(int(secretReader.Fd()))})
	if err == nil {
		t.Fatal("Run accepted a control secret of the wrong length")
	}
	if strings.Contains(err.Error(), shortSecret) {
		t.Fatalf("error %q leaks the control secret", err)
	}
}

// TestRunRefusesLiveRoot is the mutation target: drop the RefuseLiveRoot call
// from Run and this test fails.
func TestRunRefusesLiveRoot(t *testing.T) {
	live := t.TempDir()
	previous := liveRoot
	liveRoot = func() (string, error) { return live, nil }
	t.Cleanup(func() { liveRoot = previous })

	root := filepath.Join(live, "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	source := writeLocalSource(t)

	err := Run(context.Background(), []string{"--root", root, "--local-source", source}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil {
		t.Fatalf("Run served %q, which is inside the live LOCAL_APPS folder", root)
	}
	if !strings.Contains(err.Error(), "LOCAL_APPS") {
		t.Fatalf("error %q does not name LOCAL_APPS", err)
	}
}

func writeLocalSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	main := filepath.Join(dir, "src", "main")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "server.js"), []byte("// server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The SharedWorker script the stream routes serve, read at boot the way Local
	// reads it at require time.
	assetsDir := filepath.Join(main, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sync-worker.js", "sync-data-worker.js"} {
		if err := os.WriteFile(filepath.Join(assetsDir, name), []byte("// "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runtimeDir := filepath.Join(main, "offline-runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"selected":"1.0.0","versions":["1.0.0"]}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	index := `{"schema":1,"version":"1.0.0","files":{"clay.js":{"sha256":"` + strings.Repeat("0", 64) + `","type":"application/javascript","bytes":1}},"aliases":{}}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "1.0.0", "offline-manifest.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readReady(t *testing.T, r io.Reader) readyLine {
	t.Helper()
	type readResult struct {
		line string
		err  error
	}
	lines := make(chan readResult, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		lines <- readResult{line, err}
	}()
	var res readResult
	select {
	case res = <-lines:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the READY line")
	}
	if res.err != nil {
		t.Fatalf("reading READY: %v", res.err)
	}
	if !strings.HasPrefix(res.line, "READY ") || !strings.HasSuffix(res.line, "\n") {
		t.Fatalf("stdout line = %q, want one READY line", res.line)
	}
	var ready readyLine
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(res.line, "READY "))), &ready); err != nil {
		t.Fatalf("READY %q: %v", res.line, err)
	}
	return ready
}

func waitForRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
		return nil
	}
}

func get(t *testing.T, client *http.Client, target string, headers map[string]string) (int, string, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	return resp.StatusCode, string(body), resp.Header.Get("Content-Type")
}

func assertRefused(t *testing.T, target string) {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", u.Host, time.Second)
	if err == nil {
		conn.Close()
		t.Fatalf("%s still accepts connections", target)
	}
}

func realPathOrFail(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
