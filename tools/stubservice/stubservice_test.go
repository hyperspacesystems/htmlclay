package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"crypto/rand"
)

// stubTestDocument is a document a served folder holds, so the folder is one the
// host will serve.
const stubTestDocument = `<!doctype html><html><head><title>Notes</title></head><body><p>Notes</p></body></html>`

// repoRoot is the module's root: the tests build the host and the stub from it.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// buildGo builds one package into bin.
func buildGo(t *testing.T, root, bin, pkg string) {
	t.Helper()
	build := exec.Command("go", "build", "-o", bin, pkg)
	build.Dir = root
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build %s: %v", pkg, err)
	}
}

// randomSecret is a bootstrap secret: 64 lowercase hex characters.
func randomSecret(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}

// freeLoopbackPort answers a port nothing holds on either loopback stack.
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	for attempt := 0; attempt < 20; attempt++ {
		v4, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		port := v4.Addr().(*net.TCPAddr).Port
		v6, v6Err := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", port))
		v4.Close()
		if v6Err != nil {
			if !strings.Contains(v6Err.Error(), "address already in use") {
				return port
			}
			continue
		}
		v6.Close()
		return port
	}
	t.Fatal("could not find a port free on both loopback stacks")
	return 0
}

// shellReady is the host's READY line as the Mac app reads it.
type shellReady struct {
	Shell string `json:"shell"`
	Port  int    `json:"port"`
}

// readShellReady waits up to timeout for the host's one READY line on stdout.
func readShellReady(t *testing.T, stdout io.Reader, timeout time.Duration) (shellReady, bool) {
	t.Helper()
	found := make(chan shellReady, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			payload, ok := strings.CutPrefix(scanner.Text(), "READY ")
			if !ok {
				continue
			}
			var ready shellReady
			if err := json.Unmarshal([]byte(payload), &ready); err != nil {
				return
			}
			found <- ready
			return
		}
	}()
	select {
	case ready := <-found:
		return ready, true
	case <-time.After(timeout):
		return shellReady{}, false
	}
}

// stubHost starts the host the way the Mac app does, with the stub as its
// live-room service, and answers its address, secret and hold file.
type stubHost struct {
	shell  string
	secret string
	stdin  io.WriteCloser
	exited chan struct{}
	err    error
	stderr *strings.Builder
	hold   string
	home   string
}

func startStubHost(t *testing.T, hostBin, stubBin string) *stubHost {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "hc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	configDir := filepath.Join(home, "htmlclay")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"startOnLogin": false, "port": %d, "updates": "off"}`+"\n", freeLoopbackPort(t))
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(home, "notes")
	if err := os.MkdirAll(notes, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "index.html"), []byte(stubTestDocument), 0644); err != nil {
		t.Fatal(err)
	}
	hold := filepath.Join(home, "hold")
	if err := os.WriteFile(hold, nil, 0600); err != nil {
		t.Fatal(err)
	}

	host := &stubHost{hold: hold, home: home, stderr: &strings.Builder{}, exited: make(chan struct{})}
	stderr := host.stderr
	cmd := exec.Command(hostBin, "--shell-stdin")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"HTMLCLAY_CONFIG_BASE="+home,
		"HTMLCLAY_OPEN_LOG="+filepath.Join(home, "opened.log"),
		"HTMLCLAY_SERVICE_CMD="+stubBin,
		"HTMLCLAY_STUB_HOLD="+hold,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the host: %v", err)
	}
	go func() {
		host.err = cmd.Wait()
		close(host.exited)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-host.exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-host.exited
		}
	})

	secret := randomSecret(t)
	if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
		t.Fatalf("write the bootstrap line: %v\n%s", err, stderr.String())
	}
	ready, ok := readShellReady(t, stdout, 30*time.Second)
	if !ok {
		t.Fatalf("no READY line within 30s; stderr:\n%s", stderr.String())
	}
	host.shell, host.secret, host.stdin = ready.Shell, secret, stdin
	return host
}

// postAction sends one action to the host's shell listener.
func (h *stubHost) postAction(t *testing.T, id, actionType string, args any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	action := fmt.Sprintf(`{"id":%q,"type":%q,"args":%s}`, id, actionType, raw)
	request, err := http.NewRequest(http.MethodPost, h.shell+"/shell/v1/actions", strings.NewReader(action))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+h.secret)
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("POST %s (%s): %v\nstderr:\n%s", actionType, id, err, h.stderr.String())
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}

// serviceState answers host.service from the host's snapshot.
func (h *stubHost) serviceState(t *testing.T) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, h.shell+"/shell/v1/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+h.secret)
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("GET snapshot: %v\nstderr:\n%s", err, h.stderr.String())
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET snapshot = %d %s, want 200", response.StatusCode, body)
	}
	var snapshot struct {
		Host struct {
			Service string `json:"service"`
		} `json:"host"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snapshot.Host.Service
}

// waitForServiceState waits until the snapshot's service state is want.
func (h *stubHost) waitForServiceState(t *testing.T, want string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if got := h.serviceState(t); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("host.service = %q after %v, want %q\nstderr:\n%s", h.serviceState(t), within, want, h.stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestQuitHoldsStubUnsavedEdit proves step 6.15's Quit refusal against a real
// host: the stub service reports one unsaved document while its hold file
// exists, host.quit is refused naming it and the host keeps running, and the
// same quit is accepted once the hold file is gone.
func TestQuitHoldsStubUnsavedEdit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("--shell-stdin is the Mac app's way to start the host")
	}
	root := repoRoot(t)
	binDir := t.TempDir()
	hostBin := filepath.Join(binDir, "htmlclay")
	stubBin := filepath.Join(binDir, "stubservice")
	buildGo(t, root, hostBin, "./cmd/htmlclay")
	buildGo(t, root, stubBin, "./tools/stubservice")

	host := startStubHost(t, hostBin, stubBin)

	status, body := host.postAction(t, "serve-1", "folder.serve", map[string]string{
		"path": filepath.Join(host.home, "notes"),
		"name": "notes",
	})
	if status != http.StatusOK {
		t.Fatalf("folder.serve = %d %s, want 200\nstderr:\n%s", status, body, host.stderr.String())
	}
	host.waitForServiceState(t, "running", 30*time.Second)

	status, body = host.postAction(t, "quit-1", "host.quit", struct{}{})
	if status != http.StatusConflict {
		t.Fatalf("host.quit with unsaved edits = %d %s, want 409\nstderr:\n%s", status, body, host.stderr.String())
	}
	var refused struct {
		OK    bool `json:"ok"`
		Error struct {
			Code      string   `json:"code"`
			Documents []string `json:"documents"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &refused); err != nil {
		t.Fatalf("host.quit refusal: %v (%s)", err, body)
	}
	if refused.Error.Code != "quit-unsaved" {
		t.Fatalf("host.quit refusal code = %q, want quit-unsaved (%s)", refused.Error.Code, body)
	}
	named := false
	for _, doc := range refused.Error.Documents {
		if doc == "unsaved.htmlclay" || doc == "notes/unsaved.htmlclay" {
			named = true
		}
	}
	if !named {
		t.Fatalf("host.quit refusal documents = %v, want the stub's unsaved.htmlclay (%s)", refused.Error.Documents, body)
	}
	if state := host.serviceState(t); state != "running" {
		t.Fatalf("host.service after the refused quit = %q, want running", state)
	}
	select {
	case <-host.exited:
		t.Fatalf("the host exited after the refused quit: %v\nstderr:\n%s", host.err, host.stderr.String())
	case <-time.After(time.Second):
	}

	if err := os.Remove(host.hold); err != nil {
		t.Fatal(err)
	}
	status, body = host.postAction(t, "quit-2", "host.quit", struct{}{})
	if status != http.StatusOK {
		t.Fatalf("host.quit with the hold gone = %d %s, want 200\nstderr:\n%s", status, body, host.stderr.String())
	}
	select {
	case <-host.exited:
		if host.err != nil {
			t.Fatalf("the host exited with %v after quitting\nstderr:\n%s", host.err, host.stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the host did not exit within 10s of the accepted quit\nstderr:\n%s", host.stderr.String())
	}
}

// startBareStub runs the stub on its own, as the host does, and answers its URL
// and the bootstrap secret it was given.
func startBareStub(t *testing.T, stubBin string, env ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(stubBin, "--data-dir", t.TempDir())
	cmd.Env = append(os.Environ(), env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the stub: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	secret := randomSecret(t)
	if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
		t.Fatalf("write the stub's bootstrap line: %v", err)
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		payload, ok := strings.CutPrefix(scanner.Text(), "READY ")
		if !ok {
			continue
		}
		var ready ready
		if err := json.Unmarshal([]byte(payload), &ready); err != nil {
			t.Fatalf("the stub's READY line: %v (%s)", err, payload)
		}
		return ready.URL, secret
	}
	t.Fatalf("the stub printed no READY line; stderr:\n%s", stderr.String())
	return "", ""
}

// TestStubSpeaksTheServiceProtocol proves the stub's own answers: a wrong
// bearer is refused, the flush answer holds the one unsaved document while the
// hold file exists, the roots answer decodes, another path is 404, and a bad
// bootstrap line exits 3.
func TestStubSpeaksTheServiceProtocol(t *testing.T) {
	stubBin := filepath.Join(t.TempDir(), "stubservice")
	if runtime.GOOS == "windows" {
		stubBin += ".exe"
	}
	buildGo(t, repoRoot(t), stubBin, "./tools/stubservice")
	hold := filepath.Join(t.TempDir(), "hold")
	if err := os.WriteFile(hold, nil, 0600); err != nil {
		t.Fatal(err)
	}
	url, secret := startBareStub(t, stubBin, "HTMLCLAY_STUB_HOLD="+hold)

	post := func(path, bearer string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, url+path, bytes.NewReader([]byte(`{"deadlineMs":5000}`)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+bearer)
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}

	if status, body := post("/v1/flush", strings.Repeat("0", 64)); status != http.StatusUnauthorized {
		t.Fatalf("POST /v1/flush with a wrong bearer = %d %s, want 401", status, body)
	}

	status, body := post("/v1/flush", secret)
	if status != http.StatusOK {
		t.Fatalf("POST /v1/flush = %d %s, want 200", status, body)
	}
	var flush flushAnswer
	if err := json.Unmarshal(body, &flush); err != nil {
		t.Fatalf("flush answer: %v (%s)", err, body)
	}
	if len(flush.Unsaved) != 1 || flush.Unsaved[0].Name != "unsaved.htmlclay" || flush.TimedOut {
		t.Fatalf("flush answer = %s, want one unsaved.htmlclay and timedOut false", body)
	}

	if status, body := post("/v1/roots", secret); status != http.StatusOK {
		t.Fatalf("POST /v1/roots = %d %s, want 200", status, body)
	} else {
		var roots rootsAnswer
		if err := json.Unmarshal(body, &roots); err != nil {
			t.Fatalf("roots answer: %v (%s)", err, body)
		}
		if len(roots.Added) != 0 || len(roots.Removed) != 0 || len(roots.Kept) != 0 {
			t.Fatalf("roots answer = %s, want nothing added, removed or kept", body)
		}
	}

	if status, body := post("/v1/unknown", secret); status != http.StatusNotFound {
		t.Fatalf("POST /v1/unknown = %d %s, want 404", status, body)
	}

	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	status, body = post("/v1/flush", secret)
	if status != http.StatusOK {
		t.Fatalf("POST /v1/flush with the hold gone = %d %s, want 200", status, body)
	}
	if err := json.Unmarshal(body, &flush); err != nil {
		t.Fatalf("flush answer: %v (%s)", err, body)
	}
	if flush.Unsaved == nil || len(flush.Unsaved) != 0 {
		t.Fatalf("flush answer with the hold gone = %s, want an empty unsaved list", body)
	}

	bad := exec.Command(stubBin)
	bad.Stdin = strings.NewReader("not a bootstrap line\n")
	if err := bad.Run(); err == nil {
		t.Fatal("the stub accepted a bad bootstrap line")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 {
		t.Fatalf("the stub exited %v on a bad bootstrap line, want 3", err)
	}
}
