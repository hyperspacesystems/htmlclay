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

func TestShellCheckTrust(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(plain, "index.html"), servedTestDocument)
	hidden := filepath.Join(home, ".secret")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(home, "outer")
	inner := filepath.Join(outer, "inner")
	writeTestFile(t, filepath.Join(inner, "index.html"), servedTestDocument)

	a := buildTestApp(t, home, t.TempDir())
	backend := shellBackend{a}

	trust := func(id, path string) (shellapi.Preview, *appcore.Refusal) {
		return backend.Check(appcore.Action{
			ID:   id,
			Type: "folder.trust",
			Args: json.RawMessage(fmt.Sprintf(`{"path":%q}`, path)),
		})
	}

	preview, refusal := trust("c-plain", plain)
	if refusal != nil {
		t.Fatalf("Check of a plain folder refused: %+v", refusal)
	}
	if preview.Path != plain {
		t.Fatalf("preview path = %q, want %q", preview.Path, plain)
	}
	if preview.Name != "notes" {
		t.Fatalf("preview name = %q, want notes", preview.Name)
	}
	if len(preview.Warnings) != 0 {
		t.Fatalf("plain folder warnings = %+v, want none", preview.Warnings)
	}

	if _, refusal := trust("c-home", home); refusal == nil || refusal.Code != appcore.RefuseCannotTrust {
		t.Fatalf("Check of the home folder = %+v, want a %s refusal", refusal, appcore.RefuseCannotTrust)
	}
	if _, refusal := trust("c-hidden", hidden); refusal == nil || refusal.Code != appcore.RefuseCannotTrust {
		t.Fatalf("Check of a hidden folder = %+v, want a %s refusal", refusal, appcore.RefuseCannotTrust)
	}

	documents := filepath.Join(home, "Documents")
	if err := os.MkdirAll(documents, 0o755); err != nil {
		t.Fatal(err)
	}
	preview, refusal = trust("c-personal", documents)
	if refusal != nil {
		t.Fatalf("Check of a personal folder refused: %+v", refusal)
	}
	personal := false
	for _, warning := range preview.Warnings {
		if warning.Code == "personal-folder" && warning.Text == appcore.TextWarnPersonalFolder {
			personal = true
		}
	}
	if !personal {
		t.Fatalf("warnings = %+v, want a personal-folder warning", preview.Warnings)
	}

	if err := a.trustFolder(inner); err != nil {
		t.Fatalf("trustFolder(%s): %v", inner, err)
	}
	before := a.rt.cfg.TrustedFolderList()
	preview, refusal = trust("c-outer", outer)
	if refusal != nil {
		t.Fatalf("Check of a folder holding a trusted folder refused: %+v", refusal)
	}
	contains := false
	for _, warning := range preview.Warnings {
		if warning.Code != "contains-trusted" {
			continue
		}
		contains = len(warning.Names) == 1 && warning.Names[0] == "inner"
		if want := fmt.Sprintf(appcore.TextWarnContainsTrusted, "inner"); warning.Text != want {
			t.Fatalf("contains-trusted text = %q, want %q", warning.Text, want)
		}
	}
	if !contains {
		t.Fatalf("warnings = %+v, want a contains-trusted warning naming inner", preview.Warnings)
	}
	after := a.rt.cfg.TrustedFolderList()
	if len(after) != len(before) {
		t.Fatalf("trusted folders = %+v after Check, want %+v unchanged", after, before)
	}
	for i := range after {
		if after[i] != before[i] {
			t.Fatalf("trusted folders = %+v after Check, want %+v unchanged", after, before)
		}
	}

	_, refusal = backend.Do(context.Background(), appcore.Action{
		ID:   "t-home",
		Type: "folder.trust",
		Args: json.RawMessage(fmt.Sprintf(`{"path":%q}`, home)),
	})
	if refusal == nil || refusal.Code != appcore.RefuseCannotTrust {
		t.Fatalf("Do folder.trust of the home folder = %+v, want a %s refusal", refusal, appcore.RefuseCannotTrust)
	}
}

func TestShellCheckServeKnownFolderKeepsName(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blog := filepath.Join(home, "blog")
	writeTestFile(t, filepath.Join(blog, "index.html"), servedTestDocument)
	notes := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(notes, "index.html"), servedTestDocument)

	a := buildTestApp(t, home, t.TempDir())
	a.rt.port = servedTestPort
	a.startServed()
	t.Cleanup(func() {
		if a.served != nil {
			a.served.Close()
		}
	})
	backend := shellBackend{a}

	entry, refusal, err := a.serveFolder(blog, "blog")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}
	if err := a.stopServing(entry.ID); err != nil {
		t.Fatalf("stopServing: %v", err)
	}
	if _, refusal, err := a.serveFolder(notes, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder(notes): refusal=%+v err=%v", refusal, err)
	}

	check := func(id, name string) (shellapi.Preview, *appcore.Refusal) {
		return backend.Check(appcore.Action{
			ID:   "c-" + name,
			Type: "folder.serve",
			Args: json.RawMessage(fmt.Sprintf(`{"id":%q,"name":%q}`, id, name)),
		})
	}

	// A stopped folder served again resumes its entry and keeps its name, so the
	// address and what pages stored under it stay the same; check says so, and the
	// action does it.
	preview, refusal := check(entry.ID, "blog2")
	if refusal != nil || preview.Name != "blog" {
		t.Fatalf("Check of a stopped folder with a typed name = %+v, %+v; want its own name blog", preview, refusal)
	}
	snapshot, refusal := a.do(context.Background(), appcore.Action{
		ID:   "serve-again",
		Type: "folder.serve",
		Args: json.RawMessage(fmt.Sprintf(`{"id":%q,"name":"blog2"}`, entry.ID)),
	}, ChannelShell)
	if refusal != nil {
		t.Fatalf("serving the stopped folder again refused: %+v", refusal)
	}
	if got := snapshotFolder(t, snapshot, entry.ID); !got.Served || got.Address != preview.Address {
		t.Fatalf("served again at %q (served %v), want the address check gave, %q", got.Address, got.Served, preview.Address)
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

// readFailed waits up to timeout for the host's one FAILED line on stdout.
func readFailed(t *testing.T, stdout io.Reader, timeout time.Duration) (shellapi.Failure, bool) {
	t.Helper()
	found := make(chan shellapi.Failure, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			payload, ok := strings.CutPrefix(scanner.Text(), "FAILED ")
			if !ok {
				continue
			}
			var failure shellapi.Failure
			if err := json.Unmarshal([]byte(payload), &failure); err != nil {
				return
			}
			found <- failure
			return
		}
	}()
	select {
	case failure := <-found:
		return failure, true
	case <-time.After(timeout):
		return shellapi.Failure{}, false
	}
}

// shellBase is a short temporary base for every directory the host touches, so
// nothing it writes lands in this Mac's real home.
func shellBase(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("", "htmlclay-shell-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	return base
}

// writeShellConfig writes the config the host finds at its first launch.
func writeShellConfig(t *testing.T, base, config string) {
	t.Helper()
	dir := filepath.Join(base, "htmlclay")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

// startShellHost starts the host the way the Mac app does: --shell-stdin, a
// secret on stdin, and every directory under base. extraEnv adds to the
// environment the host starts with.
func startShellHost(t *testing.T, bin, base string, extraEnv ...string) (*exec.Cmd, io.WriteCloser, io.Reader, *strings.Builder) {
	t.Helper()
	var stderr strings.Builder
	cmd := exec.Command(bin, "--shell-stdin")
	cmd.Env = append(os.Environ(), "HOME="+base, "HTMLCLAY_CONFIG_BASE="+base)
	cmd.Env = append(cmd.Env, extraEnv...)
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
	return cmd, stdin, stdout, &stderr
}

// shellSnapshot answers the app's GET /shell/v1/snapshot, so a test can prove the
// primary host is still serving.
func shellSnapshot(t *testing.T, shell, secret string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, shell+"/shell/v1/snapshot", nil)
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
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET snapshot = %d %s, want 200", response.StatusCode, body)
	}
	return string(body)
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
		// Stdin closes while the request is in flight. The host may not have read
		// it yet, and then the client hears an error and nothing was promised; but
		// an action the host answered must be in the config when it is gone.
		wrote := make(chan struct{}, 1)
		answered := make(chan int, 1)
		go func() {
			defer close(answered)
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
				answered <- response.StatusCode
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

		status, ok := <-answered
		if !ok {
			t.Logf("stdin closed before the host read the serve request; the client heard an error, so nothing was promised")
			return
		}
		if status != http.StatusOK {
			t.Fatalf("folder.serve answered %d during shutdown, want 200", status)
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
			t.Fatalf("config servedFolders = %+v, want %s serving (the host answered the serve)\nstderr:\n%s", saved.ServedFolders, folder, stderr.String())
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

	t.Run("a bad bootstrap line answers FAILED", func(t *testing.T) {
		base := shellBase(t)
		cmd, stdin, stdout, stderr := startShellHost(t, bin, base)
		t.Cleanup(func() { _ = stdin.Close() })

		if _, err := fmt.Fprint(stdin, "not json\n"); err != nil {
			t.Fatalf("write the bad bootstrap line: %v", err)
		}
		failure, ok := readFailed(t, stdout, 30*time.Second)
		if !ok {
			t.Fatalf("no FAILED line within 30s; stderr:\n%s", stderr.String())
		}
		if failure.V != 1 || failure.Code != shellapi.FailBadBootstrap {
			t.Fatalf("FAILED = %+v, want v1 with code %q", failure, shellapi.FailBadBootstrap)
		}
		code, exited := waitExit(t, cmd, 30*time.Second, stderr)
		if !exited {
			t.Fatalf("the app did not exit after a bad bootstrap line; stderr:\n%s", stderr.String())
		}
		if code != 2 {
			t.Fatalf("the app exited %d, want 2; stderr:\n%s", code, stderr.String())
		}
	})

	t.Run("a second copy answers FAILED already-running", func(t *testing.T) {
		base := shellBase(t)
		writeShellConfig(t, base, fmt.Sprintf(`{"startOnLogin": false, "port": %d, "updates": "off"}`, freeLoopbackPort(t)))

		_, firstStdin, firstStdout, firstStderr := startShellHost(t, bin, base)
		defer firstStdin.Close()
		secret := randomSecret(t)
		if _, err := fmt.Fprintf(firstStdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
			t.Fatalf("write the bootstrap line: %v\n%s", err, firstStderr.String())
		}
		ready, ok := readReady(t, firstStdout, 30*time.Second)
		if !ok {
			t.Fatalf("the first host printed no READY line within 30s; stderr:\n%s", firstStderr.String())
		}

		second, secondStdin, secondStdout, secondStderr := startShellHost(t, bin, base)
		t.Cleanup(func() { _ = secondStdin.Close() })
		if _, err := fmt.Fprintf(secondStdin, `{"v":1,"secret":%q}`+"\n", randomSecret(t)); err != nil {
			t.Fatalf("write the second bootstrap line: %v\n%s", err, secondStderr.String())
		}
		failure, ok := readFailed(t, secondStdout, 30*time.Second)
		if !ok {
			t.Fatalf("the second copy printed no FAILED line within 30s; stderr:\n%s", secondStderr.String())
		}
		if failure.V != 1 || failure.Code != shellapi.FailAlreadyRunning {
			t.Fatalf("FAILED = %+v, want v1 with code %q", failure, shellapi.FailAlreadyRunning)
		}
		code, exited := waitExit(t, second, 30*time.Second, secondStderr)
		if !exited {
			t.Fatalf("the second copy did not exit; stderr:\n%s", secondStderr.String())
		}
		if code != 3 {
			t.Fatalf("the second copy exited %d, want 3; stderr:\n%s", code, secondStderr.String())
		}
		if body := shellSnapshot(t, ready.Shell, secret); !strings.Contains(body, `"v":1`) {
			t.Fatalf("the first host answered %q, want the snapshot of a running host", body)
		}
	})

	t.Run("a taken port answers FAILED port-taken without a dialog", func(t *testing.T) {
		held, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("hold a port: %v", err)
		}
		t.Cleanup(func() { held.Close() })
		port := held.Addr().(*net.TCPAddr).Port

		base := shellBase(t)
		writeShellConfig(t, base, fmt.Sprintf(`{"startOnLogin": false, "port": %d, "updates": "off"}`, port))

		cmd, stdin, stdout, stderr := startShellHost(t, bin, base)
		t.Cleanup(func() { _ = stdin.Close() })
		if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", randomSecret(t)); err != nil {
			t.Fatalf("write the bootstrap line: %v\n%s", err, stderr.String())
		}
		failure, ok := readFailed(t, stdout, 30*time.Second)
		if !ok {
			t.Fatalf("no FAILED line within 30s on port %d, so a dialog may be waiting; stderr:\n%s", port, stderr.String())
		}
		if failure.V != 1 || failure.Code != shellapi.FailPortTaken || failure.Port != port {
			t.Fatalf("FAILED = %+v, want v1 with code %q and port %d", failure, shellapi.FailPortTaken, port)
		}
		code, exited := waitExit(t, cmd, 30*time.Second, stderr)
		if !exited {
			t.Fatalf("the app did not exit with port %d taken; stderr:\n%s", port, stderr.String())
		}
		if code != 4 {
			t.Fatalf("the app exited %d, want 4; stderr:\n%s", code, stderr.String())
		}
	})

	t.Run("a host started by the app leaves the login item alone", func(t *testing.T) {
		base := shellBase(t)
		writeShellConfig(t, base, fmt.Sprintf(`{"startOnLogin": true, "port": %d, "updates": "off"}`, freeLoopbackPort(t)))

		_, stdin, stdout, stderr := startShellHost(t, bin, base)
		defer stdin.Close()
		secret := randomSecret(t)
		if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
			t.Fatalf("write the bootstrap line: %v\n%s", err, stderr.String())
		}
		ready, ok := readReady(t, stdout, 30*time.Second)
		if !ok {
			t.Fatalf("no READY line within 30s; stderr:\n%s", stderr.String())
		}
		if ready.V != 1 {
			t.Fatalf("READY = %+v, want v1", ready)
		}

		launchAgents := filepath.Join(base, "Library", "LaunchAgents")
		if _, err := os.Stat(launchAgents); err == nil {
			t.Fatalf("%s exists after READY; a host the Mac app started must not write the login item", launchAgents)
		}

		action := shellJSON("pref1", "prefs.set", map[string]any{"key": "openAtLogin", "value": true})
		request, err := http.NewRequest(http.MethodPost, ready.Shell+"/shell/v1/actions", strings.NewReader(action))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+secret)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST prefs.set: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		var outcome struct {
			Ok    bool `json:"ok"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &outcome); err != nil {
			t.Fatalf("decode the answer: %v\n%s", err, body)
		}
		if response.StatusCode != http.StatusConflict || outcome.Ok || outcome.Error.Code != appcore.RefuseNotYet {
			t.Fatalf("prefs.set openAtLogin = %d %s, want a %q refusal", response.StatusCode, body, appcore.RefuseNotYet)
		}
		if _, err := os.Stat(launchAgents); err == nil {
			t.Fatalf("%s exists after prefs.set; a host the Mac app started must not write the login item", launchAgents)
		}
	})
}

// TestOpenLog proves the host the Mac app started writes the URL it would open
// instead of opening a browser, and that only such a host does.
func TestOpenLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("--shell-stdin is the Mac app's way to start the host")
	}
	bin := buildShellBinary(t)

	t.Run("the host the app started logs the URL it would open", func(t *testing.T) {
		base := shellBase(t)
		writeShellConfig(t, base, fmt.Sprintf(`{"startOnLogin": false, "port": %d, "updates": "off"}`, freeLoopbackPort(t)))
		document := filepath.Join(base, "notes", "a.htmlclay")
		writeTestFile(t, document, servedTestDocument)
		logPath := filepath.Join(base, "opened.log")

		_, stdin, stdout, stderr := startShellHost(t, bin, base, "HTMLCLAY_OPEN_LOG="+logPath)
		t.Cleanup(func() { _ = stdin.Close() })
		secret := randomSecret(t)
		if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
			t.Fatalf("write the bootstrap line: %v\n%s", err, stderr.String())
		}
		ready, ok := readReady(t, stdout, 30*time.Second)
		if !ok {
			t.Fatalf("no READY line within 30s; stderr:\n%s", stderr.String())
		}

		action := shellJSON("open1", "file.open", map[string]string{"path": document})
		request, err := http.NewRequest(http.MethodPost, ready.Shell+"/shell/v1/actions", strings.NewReader(action))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+secret)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST file.open: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("file.open = %d %s, want 200\nstderr:\n%s", response.StatusCode, body, stderr.String())
		}

		deadline := time.Now().Add(5 * time.Second)
		for {
			logged, err := os.ReadFile(logPath)
			if err == nil && len(logged) > 0 {
				lines := strings.Split(strings.TrimRight(string(logged), "\n"), "\n")
				if len(lines) != 1 || !strings.Contains(lines[0], "a.htmlclay") {
					t.Fatalf("opened.log = %q, want one line naming a.htmlclay", logged)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("opened.log was not written within 5s; read: %v; stderr:\n%s", err, stderr.String())
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("the host logs the folder it reveals and the address it copies", func(t *testing.T) {
		base := shellBase(t)
		writeShellConfig(t, base, fmt.Sprintf(`{"startOnLogin": false, "port": %d, "updates": "off"}`, freeLoopbackPort(t)))
		notes := filepath.Join(base, "notes")
		writeTestFile(t, filepath.Join(notes, "index.html"), servedTestDocument)
		logPath := filepath.Join(base, "opened.log")

		_, stdin, stdout, stderr := startShellHost(t, bin, base, "HTMLCLAY_OPEN_LOG="+logPath)
		t.Cleanup(func() { _ = stdin.Close() })
		secret := randomSecret(t)
		if _, err := fmt.Fprintf(stdin, `{"v":1,"secret":%q}`+"\n", secret); err != nil {
			t.Fatalf("write the bootstrap line: %v\n%s", err, stderr.String())
		}
		ready, ok := readReady(t, stdout, 30*time.Second)
		if !ok {
			t.Fatalf("no READY line within 30s; stderr:\n%s", stderr.String())
		}

		post := func(action string) []byte {
			t.Helper()
			request, err := http.NewRequest(http.MethodPost, ready.Shell+"/shell/v1/actions", strings.NewReader(action))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+secret)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("POST %s: %v", action, err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("%s = %d %s, want 200\nstderr:\n%s", action, response.StatusCode, body, stderr.String())
			}
			return body
		}

		post(shellJSON("trust1", "folder.trust", map[string]string{"path": notes}))
		served := post(shellJSON("serve1", "folder.serve", map[string]string{"path": notes, "name": "notes"}))
		var outcome struct {
			Snapshot struct {
				Folders []struct {
					ID      string `json:"id"`
					Address string `json:"address"`
					Served  bool   `json:"served"`
				} `json:"folders"`
			} `json:"snapshot"`
		}
		if err := json.Unmarshal(served, &outcome); err != nil {
			t.Fatalf("decode the serve answer: %v\n%s", err, served)
		}
		id, address := "", ""
		for _, folder := range outcome.Snapshot.Folders {
			if folder.Served && folder.Address != "" {
				id, address = folder.ID, folder.Address
			}
		}
		if id == "" {
			t.Fatalf("serving %s gave no served folder with an address: %s", notes, served)
		}

		resolved, err := filepath.EvalSymlinks(notes)
		if err != nil {
			t.Fatal(err)
		}

		post(shellJSON("reveal1", "folder.reveal", map[string]string{"id": id}))
		post(shellJSON("copy1", "folder.copyAddress", map[string]string{"id": id}))

		deadline := time.Now().Add(5 * time.Second)
		for {
			logged, err := os.ReadFile(logPath)
			if err == nil {
				lines := strings.Split(strings.TrimRight(string(logged), "\n"), "\n")
				revealed, copied := false, false
				for _, line := range lines {
					if line == "reveal "+resolved {
						revealed = true
					}
					if strings.HasPrefix(line, "copy http") && strings.Contains(line, address) {
						copied = true
					}
				}
				if revealed && copied {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("opened.log = %q within 5s, want a %q line and a copy of %q; read: %v; stderr:\n%s",
					logged, "reveal "+resolved, address, err, stderr.String())
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("only a host the app started logs", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "opened.log")
		if open := openLogOverride(false, logPath); open != nil {
			if err := open("http://127.0.0.1:1/a.htmlclay"); err != nil {
				t.Fatalf("log the URL: %v", err)
			}
			t.Fatalf("an ordinary launch replaced launchBrowser; a launch outside --shell-stdin must open the browser")
		}
		if open := openLogOverride(true, filepath.Join("relative", "opened.log")); open != nil {
			t.Fatal("a relative HTMLCLAY_OPEN_LOG replaced launchBrowser")
		}
		open := openLogOverride(true, logPath)
		if open == nil {
			t.Fatal("a host the app started did not replace launchBrowser for an absolute HTMLCLAY_OPEN_LOG")
		}
		if err := open("http://127.0.0.1:1/a.htmlclay"); err != nil {
			t.Fatalf("log the URL: %v", err)
		}
		logged, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read the log: %v", err)
		}
		if string(logged) != "http://127.0.0.1:1/a.htmlclay\n" {
			t.Fatalf("opened.log = %q, want the URL and a newline", logged)
		}
	})
}
