package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/service"
	"github.com/panphora/htmlclay/internal/tailwind"
	"github.com/panphora/htmlclay/internal/update"
)

// commandSI is a SingleInstance whose primary answers every command with
// answer, or fails with err.
type commandSI struct {
	answer string
	err    error
	sent   []string
}

func (c *commandSI) TryLock() (bool, error)        { return false, nil }
func (c *commandSI) SendFilePath(string) error     { return errors.New("not a file") }
func (c *commandSI) OnFileReceived(func(string))   {}
func (c *commandSI) OnCommand(func(string) string) {}
func (c *commandSI) Unlock() error                 { return nil }
func (c *commandSI) SendCommand(cmd string) (string, error) {
	c.sent = append(c.sent, cmd)
	return c.answer, c.err
}

func TestUpdateNowClientExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		err    error
		code   int
		out    string
	}{
		{"accepted", "accepted", nil, updateNowExitOK, "accepted\n"},
		{"already there", "already 9.9.9", nil, updateNowExitOK, "already 9.9.9\n"},
		{"already another version", "already 9.9.8", nil, updateNowExitRefused, "already 9.9.8\n"},
		{"refused", "refused self-update is off: x", nil, updateNowExitRefused, "refused self-update is off: x\n"},
		{"failed", "failed the restart was cancelled", nil, updateNowExitFailed, "failed the restart was cancelled\n"},
		{"starting", platform.CommandStarting, nil, updateNowExitStarting, "starting\n"},
		{"old app", "", platform.ErrNoAnswer, updateNowExitTooOld, "refused the running HTML Clay predates update-now\n"},
		{"not running", "", errors.New("dial unix: connection refused"), updateNowExitNotRunning, ""},
	}
	for _, tc := range cases {
		si := &commandSI{answer: tc.answer, err: tc.err}
		var out, errOut bytes.Buffer
		code := runUpdateNow(si, []string{"--version", "9.9.9"}, &out, &errOut)
		if code != tc.code || out.String() != tc.out {
			t.Errorf("%s: exit %d, stdout %q; want %d, %q", tc.name, code, out.String(), tc.code, tc.out)
		}
		if len(si.sent) != 1 || si.sent[0] != "update-now 9.9.9" {
			t.Errorf("%s: sent %q", tc.name, si.sent)
		}
	}
}

func TestUpdateNowClientRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--version", "1.2"}, {"--version", "v1.2.3"}, {"--version", "1.2.3", "extra"}, {"1.2.3"}} {
		si := &commandSI{answer: "accepted"}
		var out, errOut bytes.Buffer
		if code := runUpdateNow(si, args, &out, &errOut); code != updateNowExitUsage || len(si.sent) != 0 {
			t.Errorf("args %q: exit %d, sent %q; want usage and nothing sent", args, code, si.sent)
		}
	}
}

func TestUpdateNowDispatchOnlyTakesItsOwnSubcommand(t *testing.T) {
	for _, argv := range [][]string{{"htmlclay"}, {"htmlclay", "/tmp/update-now"}, {"htmlclay", "wire"}} {
		if _, handled := updateNowDispatch(argv); handled {
			t.Errorf("updateNowDispatch handled %q", argv)
		}
	}
}

func TestUpdateInfoPrintsWhatThisBinaryIs(t *testing.T) {
	origVersion, origChannel := version, updateChannel
	t.Cleanup(func() { version, updateChannel = origVersion, origChannel })
	version, updateChannel = "7.8.9", "stable"

	var out, errb bytes.Buffer
	if code := runUpdateInfo(nil, &out, &errb); code != updateNowExitOK {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	var got updateInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not one JSON line: %q: %v", out.String(), err)
	}
	want := updateInfo{Version: "7.8.9", Channel: "stable", Service: service.Bundled(), Tailwind: tailwind.Bundled()}
	if got != want {
		t.Fatalf("update-info = %+v, want %+v", got, want)
	}
}

func TestUpdateInfoRefusesArguments(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runUpdateInfo([]string{"--version"}, &out, &errb); code != updateNowExitUsage {
		t.Fatalf("exit %d, want %d", code, updateNowExitUsage)
	}
	if out.Len() != 0 {
		t.Fatalf("printed %q on a usage error", out.String())
	}
}

func TestUpdateInfoDispatchTouchesNoConfigFolder(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HTMLCLAY_CONFIG_BASE", base)
	code, handled := updateNowDispatch([]string{"htmlclay", "update-info"})
	if !handled || code != updateNowExitOK {
		t.Fatalf("handled=%v code=%d", handled, code)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("update-info wrote into the config base: %v", entries)
	}
}

func TestUpdateNowRefusesWithAReason(t *testing.T) {
	orig := updateNowOS
	t.Cleanup(func() { updateNowOS = orig })
	updateNowOS = "darwin"
	a, _ := updateTestApp(t)
	cases := []struct {
		target string
		want   string
	}{
		{"1.2", `refused "1.2" is not X.Y.Z`},
		{version, "already " + version},
		{"0.0.1", "refused v0.0.1 is not newer than the running v" + version},
	}
	for _, tc := range cases {
		if got := a.updateNow(tc.target); got != tc.want {
			t.Errorf("updateNow(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}

	a.updateOff = `config sets "updates": "off"`
	if got := a.updateNow("9.9.9"); got != `refused self-update is off: config sets "updates": "off"` {
		t.Errorf("with self-update off: %q", got)
	}
	a.updateOff = ""

	updateNowOS = "windows"
	if got := a.updateNow("9.9.9"); !strings.HasPrefix(got, "refused update-now runs on macOS only") {
		t.Errorf("on Windows: %q", got)
	}
	updateNowOS = "darwin"
}

// On macOS the HTML Clay app runs Sparkle and answers update-now itself, so a
// host started on its own refuses and points at the app.
func TestUpdateNowOnMacIsTheAppsJob(t *testing.T) {
	orig := updateNowOS
	t.Cleanup(func() { updateNowOS = orig })
	updateNowOS = "darwin"
	a, _ := updateTestApp(t)
	a.updateOff = "on macOS the HTML Clay app updates itself"
	if got := a.updateNow("9.9.9"); got != "refused self-update is off: on macOS the HTML Clay app updates itself" {
		t.Fatalf("updateNow = %q", got)
	}
}

func TestUpdateNowCommandParsing(t *testing.T) {
	orig := updateNowOS
	t.Cleanup(func() { updateNowOS = orig })
	updateNowOS = "darwin"
	a, _ := updateTestApp(t)
	for _, cmd := range []string{"", "update-now", "update-now 9.9.9 extra", "install 9.9.9"} {
		if got := a.handleInstanceCommand(cmd); got != "refused unknown command" {
			t.Errorf("handleInstanceCommand(%q) = %q", cmd, got)
		}
	}
	if got := a.handleInstanceCommand("update-now " + version); got != "already "+version {
		t.Errorf("handleInstanceCommand = %q", got)
	}
}

// shellUpdateTestAppIn is an app the Mac app drives, running from a fake bundle
// whose binary stands in for the Go host, with its config loaded from cfgBase.
func shellUpdateTestAppIn(t *testing.T, cfgBase string) (*app, string) {
	t.Helper()
	a := newTestAppAs(t, t.TempDir(), cfgBase, true)
	t.Cleanup(a.shutdown)
	if err := os.MkdirAll(a.rt.configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "HTMLClay.app")
	macos := filepath.Join(bundle, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	origPath, origBegin, origStep := shellInstallPath, beginSparkle, shellUpdateStep
	t.Cleanup(func() { shellInstallPath, beginSparkle, shellUpdateStep = origPath, origBegin, origStep })
	shellInstallPath = func() (string, error) { return filepath.Join(macos, "htmlclay-host"), nil }
	return a, bundle
}

func shellUpdateTestApp(t *testing.T) (*app, string) {
	t.Helper()
	return shellUpdateTestAppIn(t, t.TempDir())
}

// writeShellUpdateState writes updates/state.json the way the update package
// does.
func writeShellUpdateState(t *testing.T, configDir string, st update.State) {
	t.Helper()
	dir := update.UpdatesDir(configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	st.Schema, st.Time = update.StateSchema, time.Now().UTC()
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// The Swift app asks the host to prepare an update: writes stop, the handoff is
// written for the tabs that are open, and only then does recovery start, which
// copies the whole bundle aside.
func TestShellUpdateOrder(t *testing.T) {
	a, bundle := shellUpdateTestApp(t)
	handoff := filepath.Join(a.rt.configDir, updateHandoffFile)
	var steps []string
	handoffAt := map[string]bool{}
	shellUpdateStep = func(stage string) {
		steps = append(steps, stage)
		_, err := os.Stat(handoff)
		handoffAt[stage] = err == nil
	}
	var got update.SparkleParams
	beginSparkle = func(dir string, p update.SparkleParams) (*update.State, error) {
		got = p
		return &update.State{Txid: "tx1", PreviousPath: filepath.Join(dir, "previous", "tx1", "HTMLClay.app")}, nil
	}

	pid := os.Getppid()
	if answer := a.prepareShellUpdate("9.9.9", pid, false, false); answer != "prepared tx1" {
		t.Fatalf("update-prepare = %q, want prepared tx1", answer)
	}
	if want := []string{"quiet", "drained", "handoff", "begin"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("stages %q, want %q", steps, want)
	}
	if handoffAt["quiet"] || handoffAt["drained"] {
		t.Fatalf("the handoff was written before the writes were drained: %v", handoffAt)
	}
	if !handoffAt["handoff"] || !handoffAt["begin"] {
		t.Fatalf("the handoff was not written before recovery began: %v", handoffAt)
	}
	if got.Executable != filepath.Join(bundle, "Contents", "MacOS", "htmlclay") {
		t.Fatalf("recovery executable %q", got.Executable)
	}
	if len(got.Helpers) != 1 || got.Helpers[0] != "Contents/MacOS/htmlclay-host" {
		t.Fatalf("recovery helpers %q", got.Helpers)
	}
	if got.Bundle != bundle || got.OldPID != pid || got.HandoffPath != handoff || got.Quit || got.To != "9.9.9" {
		t.Fatalf("recovery params %+v", got)
	}
	if _, err := os.Stat(handoff); err != nil {
		t.Fatalf("the handoff is missing after prepare: %v", err)
	}
	if !a.updating.Load() {
		t.Fatal("the app is not marked as updating after prepare")
	}
}

// A quit install relaunches nothing, so no tab comes back to the handoff.
func TestShellUpdateQuitHasNoHandoff(t *testing.T) {
	a, _ := shellUpdateTestApp(t)
	handoff := filepath.Join(a.rt.configDir, updateHandoffFile)
	var steps []string
	shellUpdateStep = func(stage string) { steps = append(steps, stage) }
	var got update.SparkleParams
	beginSparkle = func(dir string, p update.SparkleParams) (*update.State, error) {
		got = p
		return &update.State{Txid: "tx1"}, nil
	}

	if answer := a.prepareShellUpdate("9.9.9", os.Getppid(), true, false); answer != "prepared tx1" {
		t.Fatalf("update-prepare = %q, want prepared tx1", answer)
	}
	if want := []string{"quiet", "drained", "begin"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("stages %q, want %q", steps, want)
	}
	if got.HandoffPath != "" || !got.Quit {
		t.Fatalf("recovery params %+v, want no handoff and Quit", got)
	}
	if _, err := os.Stat(handoff); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a quit install wrote a handoff: %v", err)
	}
}

func TestShellUpdateRefusals(t *testing.T) {
	cases := []struct {
		name     string
		change   func(t *testing.T, a *app)
		pid      func() int
		target   string
		answer   string
		updating bool
	}{
		{name: "not-shell", change: func(t *testing.T, a *app) { a.shell = false }, pid: os.Getppid, target: "9.9.9", answer: "failed not-shell"},
		{name: "bad-pid", pid: func() int { return os.Getppid() + 1 }, target: "9.9.9", answer: "failed bad-pid"},
		{name: "bad-version", pid: os.Getppid, target: "1.2", answer: "failed bad-version"},
		{name: "not-in-bundle", change: func(t *testing.T, a *app) {
			outside := filepath.Join(t.TempDir(), "htmlclay")
			shellInstallPath = func() (string, error) { return outside, nil }
		}, pid: os.Getppid, target: "9.9.9", answer: "failed not-in-bundle"},
		{name: "busy", change: func(t *testing.T, a *app) { a.updating.Store(true) }, pid: os.Getppid, target: "9.9.9", answer: "failed busy", updating: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := shellUpdateTestApp(t)
			if tc.change != nil {
				tc.change(t, a)
			}
			base := writeServer(t, a, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
			beginSparkle = func(string, update.SparkleParams) (*update.State, error) {
				t.Error("a refused update started recovery")
				return &update.State{Txid: "tx1"}, nil
			}
			shellUpdateStep = func(stage string) { t.Errorf("a refused update passed %s", stage) }

			if answer := a.prepareShellUpdate(tc.target, tc.pid(), false, false); answer != tc.answer {
				t.Fatalf("update-prepare = %q, want %q", answer, tc.answer)
			}
			if got := a.updating.Load(); got != tc.updating {
				t.Fatalf("updating = %v after %q, want %v", got, tc.answer, tc.updating)
			}
			if code, _, body := postStatus(t, base+"/_/save/tok"); code != http.StatusOK {
				t.Fatalf("a save after %q got %d: %s", tc.answer, code, body)
			}
			if _, err := os.Stat(filepath.Join(a.rt.configDir, updateHandoffFile)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("a refused update wrote a handoff: %v", err)
			}
			if _, err := os.Stat(filepath.Join(update.UpdatesDir(a.rt.configDir), "state.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("a refused update wrote update state: %v", err)
			}
		})
	}
}

// Without a supervisor there is nothing to roll a bad install back, but the
// install still goes ahead: the app stays closed to saves and updating.
func TestShellUpdateBeginFailsStillPrepared(t *testing.T) {
	a, _ := shellUpdateTestApp(t)
	base := writeServer(t, a, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	beginSparkle = func(string, update.SparkleParams) (*update.State, error) {
		return nil, errors.New("no supervisor")
	}

	if answer := a.prepareShellUpdate("9.9.9", os.Getppid(), false, false); answer != "prepared -" {
		t.Fatalf("update-prepare = %q, want prepared -", answer)
	}
	if code, _, body := postStatus(t, base+"/_/save/tok"); code != http.StatusServiceUnavailable || !strings.Contains(body, `"restarting"`) {
		t.Fatalf("a save after prepare got %d: %s", code, body)
	}
	if !a.updating.Load() {
		t.Fatal("the app stopped updating after recovery failed to start")
	}
}

// Once prepared, the tabs' saves are refused until the Swift app either
// installs or aborts; aborting takes the handoff and the recovery state back.
func TestShellUpdateClosedThenAborted(t *testing.T) {
	a, _ := shellUpdateTestApp(t)
	updates := update.UpdatesDir(a.rt.configDir)
	base := writeServer(t, a, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	beginSparkle = func(dir string, p update.SparkleParams) (*update.State, error) {
		writeShellUpdateState(t, a.rt.configDir, update.State{Phase: update.PhaseApplying, From: version, To: p.To, Txid: "tx1", Installer: update.InstallerSparkle})
		if err := os.MkdirAll(filepath.Join(dir, "previous", "tx1"), 0o700); err != nil {
			t.Fatal(err)
		}
		return &update.State{Txid: "tx1", PreviousPath: filepath.Join(dir, "previous", "tx1", "HTMLClay.app")}, nil
	}

	if answer := a.prepareShellUpdate("9.9.9", os.Getppid(), false, false); answer != "prepared tx1" {
		t.Fatalf("update-prepare = %q, want prepared tx1", answer)
	}
	if code, hdr, body := postStatus(t, base+"/_/save/tok"); code != http.StatusServiceUnavailable || hdr.Get("Retry-After") == "" || !strings.Contains(body, `"restarting"`) {
		t.Fatalf("a save after prepare got %d (Retry-After %q): %s", code, hdr.Get("Retry-After"), body)
	}
	if answer := a.abortShellUpdate("tx1"); answer != "aborted" {
		t.Fatalf("update-abort = %q", answer)
	}
	if code, _, body := postStatus(t, base+"/_/save/tok"); code != http.StatusOK {
		t.Fatalf("a save after abort got %d: %s", code, body)
	}
	if a.updating.Load() {
		t.Fatal("the app is still marked as updating after abort")
	}
	if _, err := os.Stat(filepath.Join(a.rt.configDir, updateHandoffFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the handoff survived the abort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(updates, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the update state survived the abort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(updates, "previous")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the bundle copy survived the abort: %v", err)
	}
}

func TestShellUpdateStatus(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		a, _ := shellUpdateTestApp(t)
		if answer := a.shellUpdateStatus(); answer != "ok" {
			t.Fatalf("update-status = %q, want ok", answer)
		}
	})

	t.Run("off", func(t *testing.T) {
		cfgBase := t.TempDir()
		dir := filepath.Join(cfgBase, "htmlclay")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"updates":"off"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		a, _ := shellUpdateTestAppIn(t, cfgBase)
		if answer := a.shellUpdateStatus(); answer != "off "+updatesOffReason {
			t.Fatalf("update-status = %q, want off %s", answer, updatesOffReason)
		}
	})

	t.Run("failed", func(t *testing.T) {
		a, _ := shellUpdateTestApp(t)
		writeShellUpdateState(t, a.rt.configDir, update.State{
			Phase:  update.PhaseRolledBack,
			From:   "1.0.0",
			To:     "9.9.9",
			Txid:   "tx1",
			Failed: &update.Failure{Version: "9.9.9", Reason: "did not report serving"},
		})
		if answer := a.shellUpdateStatus(); answer != "failed 9.9.9 did not report serving" {
			t.Fatalf("update-status = %q", answer)
		}
	})
}

func TestHostCommandClients(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		err    error
		code   int
		out    string
	}{
		{"prepared", "prepared tx1", nil, updateNowExitOK, "prepared tx1\n"},
		{"refused", `refused unsaved ["notes/index.html"]`, nil, updateNowExitRefused, `refused unsaved ["notes/index.html"]` + "\n"},
		{"starting", platform.CommandStarting, nil, updateNowExitStarting, platform.CommandStarting + "\n"},
		{"old app", "", platform.ErrNoAnswer, updateNowExitTooOld, "refused the running HTML Clay predates update-prepare\n"},
	}
	for _, tc := range cases {
		si := &commandSI{answer: tc.answer, err: tc.err}
		var out, errOut bytes.Buffer
		code := runHostCommand(si, "update-prepare", []string{"9.9.9", "123", "restart"}, []string{"prepared"}, &out, &errOut)
		if code != tc.code || out.String() != tc.out {
			t.Errorf("%s: exit %d, stdout %q; want %d, %q", tc.name, code, out.String(), tc.code, tc.out)
		}
		if len(si.sent) != 1 || si.sent[0] != "update-prepare 9.9.9 123 restart" {
			t.Errorf("%s: sent %q", tc.name, si.sent)
		}
	}
}
