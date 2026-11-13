package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// shellJSON is an action body, written rather than marshalled so a test sends
// exactly the bytes the Mac app would.
func shellJSON(id, actionType string, args any) string {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"id":%q,"type":%q,"args":%s}`, id, actionType, raw)
}

func TestShellBackendDoesThroughTheCore(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(notes, "index.html"), servedTestDocument)
	blog := filepath.Join(home, "blog")
	writeTestFile(t, filepath.Join(blog, "index.html"), servedTestDocument)

	a := buildTestApp(t, home, t.TempDir())
	a.rt.port = servedTestPort
	a.startServed()
	t.Cleanup(func() {
		if a.served != nil {
			a.served.Close()
		}
	})
	backend := shellBackend{a}

	snapshot, refusal := backend.Do(context.Background(), appcore.Action{
		ID:   "a1",
		Type: "folder.serve",
		Args: json.RawMessage(fmt.Sprintf(`{"path":%q,"name":"notes"}`, notes)),
	})
	if refusal != nil {
		t.Fatalf("Do folder.serve refused: %+v", refusal)
	}
	served := false
	for _, tf := range a.rt.cfg.ServedFolderList() {
		if tf.Path == notes && tf.Name == "notes" && tf.Serving {
			served = true
		}
	}
	if !served {
		t.Fatal("Do folder.serve did not serve the folder")
	}
	inSnapshot := false
	for _, folder := range snapshot.Folders {
		if folder.Path == notes && folder.Served && folder.Address == servedHostName("notes", servedTestPort) {
			inSnapshot = true
		}
	}
	if !inSnapshot {
		t.Fatalf("snapshot folders = %+v, want %s served as notes", snapshot.Folders, notes)
	}

	answered, answeredRefusal, ok := backend.Answer("a1")
	if !ok {
		t.Fatal("Answer does not remember the action id")
	}
	if answeredRefusal != nil || answered.Rev != snapshot.Rev {
		t.Fatalf("Answer = %+v, %+v; want the outcome Do gave (%+v)", answered, answeredRefusal, snapshot)
	}
	if answered.Epoch != snapshot.Epoch {
		t.Fatalf("Answer epoch = %q, want %q", answered.Epoch, snapshot.Epoch)
	}

	preview, refusal := backend.Check(appcore.Action{
		ID:   "c1",
		Type: "folder.serve",
		Args: json.RawMessage(fmt.Sprintf(`{"path":%q,"name":"blog"}`, blog)),
	})
	if refusal != nil {
		t.Fatalf("Check of a folder that may be served refused: %+v", refusal)
	}
	if want := servedHostName("blog", servedTestPort); preview.Address != want || preview.Name != "blog" {
		t.Fatalf("preview = %+v, want blog at %s", preview, want)
	}
	for _, tf := range a.rt.cfg.ServedFolderList() {
		if tf.Path == blog {
			t.Fatal("Check served the folder; it must only preview")
		}
	}

	_, refusal = backend.Check(appcore.Action{
		ID:   "c2",
		Type: "folder.serve",
		Args: json.RawMessage(fmt.Sprintf(`{"path":%q,"name":"home"}`, home)),
	})
	if refusal == nil || refusal.Code != appcore.RefuseIsHome {
		t.Fatalf("Check of the home folder = %+v, want an is-home refusal", refusal)
	}
}

// freeLoopbackPort answers a port free on both loopback stacks, the way the app
// asks for it: a launch finds the port held on one stack and asks the person.
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
			// No IPv6 loopback here, or the port is held on it: either way the app
			// binds what it can, so only a held port sends this back around.
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

// randomSecret is a bootstrap secret: 64 lowercase hex characters.
func randomSecret(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}

// buildShellBinary builds the app once, so a test can start it the way the Mac app
// does.
func buildShellBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "htmlclay")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

// readReady waits up to timeout for the app's one READY line on stdout.
func readReady(t *testing.T, stdout io.Reader, timeout time.Duration) (shellapi.Ready, bool) {
	t.Helper()
	found := make(chan shellapi.Ready, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			payload, ok := strings.CutPrefix(scanner.Text(), "READY ")
			if !ok {
				continue
			}
			var ready shellapi.Ready
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
		return shellapi.Ready{}, false
	}
}

// waitExit waits for the child to exit and reports its code.
func waitExit(t *testing.T, cmd *exec.Cmd, timeout time.Duration, stderr *strings.Builder) (int, bool) {
	t.Helper()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil {
			return 0, true
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), true
		}
		t.Fatalf("wait: %v (stderr: %s)", err, stderr.String())
		return 0, false
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-exited
		return 0, false
	}
}

func TestShellStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("--shell-stdin is the Mac app's way to start the host")
	}
	bin := buildShellBinary(t)

	t.Run("serves the app and exits when stdin closes", func(t *testing.T) {
		// A short base: the app's single-instance socket lives under the config
		// directory, and a unix socket path past 104 bytes cannot bind.
		base, err := os.MkdirTemp("", "htmlclay-shell-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(base) })
		configDir := filepath.Join(base, "htmlclay")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		port := freeLoopbackPort(t)
		config := fmt.Sprintf("{\"startOnLogin\": false, \"port\": %d, \"updates\": \"off\"}\n", port)
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}

		secret := randomSecret(t)
		var stderr strings.Builder
		cmd := exec.Command(bin, "--shell-stdin")
		cmd.Env = append(os.Environ(), "HTMLCLAY_CONFIG_BASE="+base)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })

		if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
			t.Fatalf("write the bootstrap line: %v\n%s", err, stderr.String())
		}
		ready, ok := readReady(t, stdout, 30*time.Second)
		if !ok {
			t.Fatalf("no READY line within 30s; stderr:\n%s", stderr.String())
		}
		if ready.V != 1 || ready.Shell == "" || ready.Port != port || ready.Epoch == "" || ready.PID != cmd.Process.Pid {
			t.Fatalf("READY = %+v, want v1 with a shell address, port %d, an epoch, and pid %d", ready, port, cmd.Process.Pid)
		}

		request, err := http.NewRequest(http.MethodGet, ready.Shell+"/shell/v1/snapshot", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+secret)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("GET snapshot: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"v":1`) {
			t.Fatalf("GET snapshot = %d %s, want 200 with v1\nstderr:\n%s", response.StatusCode, body, stderr.String())
		}

		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		folder, err := os.MkdirTemp(home, ".shell-stdin-test-")
		if err != nil {
			t.Fatalf("a folder under %s: %v", home, err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(folder) })

		action := shellJSON("serve1", "folder.serve", map[string]string{"path": folder, "name": "shell-stdin-test"})
		// The request is sent, but its answer is never read: stdin closes while it
		// is still in flight, so what this checks is that a write already under way
		// completes before the host is gone.
		wrote := make(chan struct{}, 1)
		go func() {
			request, err := http.NewRequest(http.MethodPost, ready.Shell+"/shell/v1/actions", strings.NewReader(action))
			if err != nil {
				return
			}
			request.Header.Set("Authorization", "Bearer "+secret)
			trace := &httptrace.ClientTrace{
				WroteRequest: func(httptrace.WroteRequestInfo) {
					select {
					case wrote <- struct{}{}:
					default:
					}
				},
			}
			request = request.WithContext(httptrace.WithClientTrace(context.Background(), trace))
			if response, err := http.DefaultClient.Do(request); err == nil {
				response.Body.Close()
			}
		}()
		select {
		case <-wrote:
		case <-time.After(5 * time.Second):
			t.Fatalf("the folder.serve request was never sent; stderr:\n%s", stderr.String())
		}

		if err := stdin.Close(); err != nil {
			t.Fatalf("close stdin: %v", err)
		}
		code, exited := waitExit(t, cmd, 5*time.Second, &stderr)
		if !exited {
			t.Fatalf("the app did not exit within 5s of stdin closing; stderr:\n%s", stderr.String())
		}
		if code != 0 {
			t.Fatalf("the app exited %d, want 0; stderr:\n%s", code, stderr.String())
		}

		raw, err := os.ReadFile(filepath.Join(configDir, "config.json"))
		if err != nil {
			t.Fatalf("read the config back: %v", err)
		}
		var saved struct {
			ServedFolders []struct {
				Name    string `json:"name"`
				Path    string `json:"path"`
				Serving bool   `json:"serving"`
			} `json:"servedFolders"`
		}
		if err := json.Unmarshal(raw, &saved); err != nil {
			t.Fatalf("decode the config: %v\n%s", err, raw)
		}
		served := false
		for _, entry := range saved.ServedFolders {
			if entry.Path == folder && entry.Serving {
				served = true
			}
		}
		if !served {
			t.Fatalf("config servedFolders = %+v, want %s serving (the write in flight completed)\nstderr:\n%s", saved.ServedFolders, folder, stderr.String())
		}
	})

	t.Run("refuses a bad bootstrap line", func(t *testing.T) {
		var stderr strings.Builder
		cmd := exec.Command(bin, "--shell-stdin")
		cmd.Env = append(os.Environ(), "HTMLCLAY_CONFIG_BASE="+t.TempDir())
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stdout strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })

		if _, err := fmt.Fprintln(stdin, `{"v":1,"secret":"short"}`); err != nil {
			t.Fatalf("write the bad bootstrap line: %v", err)
		}
		_ = stdin.Close()
		code, exited := waitExit(t, cmd, 30*time.Second, &stderr)
		if !exited {
			t.Fatalf("the app did not exit after a bad bootstrap line; stderr:\n%s", stderr.String())
		}
		if code != 2 {
			t.Fatalf("the app exited %d, want 2; stderr:\n%s", code, stderr.String())
		}
		if strings.Contains(stdout.String(), "READY") {
			t.Fatalf("the app printed READY after a bad bootstrap line: %q", stdout.String())
		}
	})
}
