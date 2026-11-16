package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/update"
)

func confirmTestApp(t *testing.T) *app {
	t.Helper()
	home, cfgBase := t.TempDir(), t.TempDir()
	t.Setenv("HTMLCLAY_CONFIG_BASE", cfgBase)
	a := newTestAppWithConfigDir(t, home, cfgBase)
	t.Cleanup(a.shutdown)
	a.rt.configDir = config.DirFrom(cfgBase)
	if a.rt.configDir != config.DirFrom(cfgBase) {
		t.Fatalf("configDir %q is not the folder markUpdateHealth writes to (%q)", a.rt.configDir, config.DirFrom(cfgBase))
	}
	return a
}

// writeConfirmState is writeUpdateState with To set to this binary's version, so
// MarkHealth accepts it. Tests never assign the global version: an app's publish
// timer reads it from another goroutine, and the race detector flags the write.
func writeConfirmState(t *testing.T, configDir, phase string) {
	t.Helper()
	dir := update.UpdatesDir(configDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(update.State{Schema: update.StateSchema, Phase: phase, From: "1.0.0", To: version, Txid: "t1", Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateConfirmReportsServingForTheApp(t *testing.T) {
	a := confirmTestApp(t)
	writeConfirmState(t, a.rt.configDir, update.PhaseApplying)
	health := filepath.Join(update.UpdatesDir(a.rt.configDir), "health.json")

	for _, cmd := range []string{"update-confirm now", "update-confirmed"} {
		if got := a.handleInstanceCommand(cmd); got != "refused unknown command" {
			t.Fatalf("handleInstanceCommand(%q) = %q", cmd, got)
		}
	}
	if _, err := os.Stat(health); !os.IsNotExist(err) {
		t.Fatalf("health.json exists before the app confirmed: %v", err)
	}

	if got := a.handleInstanceCommand("update-confirm"); got != "confirmed" {
		t.Fatalf("update-confirm = %q", got)
	}
	data, err := os.ReadFile(health)
	if err != nil {
		t.Fatal(err)
	}
	var h update.Health
	if err := json.Unmarshal(data, &h); err != nil {
		t.Fatal(err)
	}
	if h.Phase != update.HealthServing || h.Txid != "t1" || h.Version != version || h.PID != os.Getpid() {
		t.Fatalf("health = %+v", h)
	}
}

func TestUpdateConfirmSettlesTheHeldSave(t *testing.T) {
	a := confirmTestApp(t)
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.rt.configDir, "config.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeConfirmState(t, a.rt.configDir, update.PhaseApplying)
	a.holdSavesWhileUnjudged()
	a.rt.cfg.SetStartOnLogin(!a.rt.cfg.StartOnLoginEnabled())
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
		t.Fatal("a held save wrote config.json")
	}

	writeConfirmState(t, a.rt.configDir, update.PhaseDone)
	if got := a.handleInstanceCommand("update-confirm"); got != "confirmed" {
		t.Fatalf("update-confirm = %q", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("update-confirm never settled the held config save")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestUpdateConfirmClientExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		err    error
		code   int
		out    string
	}{
		{"confirmed", "confirmed", nil, updateNowExitOK, "confirmed\n"},
		{"starting", platform.CommandStarting, nil, updateNowExitStarting, platform.CommandStarting + "\n"},
		{"refused", "refused unknown command", nil, updateNowExitRefused, "refused unknown command\n"},
		{"too old", "", platform.ErrNoAnswer, updateNowExitTooOld, "refused the running HTML Clay predates update-confirm\n"},
		{"not running", "", errors.New("dial: no such file"), updateNowExitNotRunning, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			si := &commandSI{answer: c.answer, err: c.err}
			var out, errb bytes.Buffer
			if code := runUpdateConfirm(si, nil, &out, &errb); code != c.code {
				t.Fatalf("exit %d, want %d (stderr %q)", code, c.code, errb.String())
			}
			if out.String() != c.out {
				t.Fatalf("stdout %q, want %q", out.String(), c.out)
			}
			if len(si.sent) != 1 || si.sent[0] != "update-confirm" {
				t.Fatalf("sent %q", si.sent)
			}
		})
	}
	var out, errb bytes.Buffer
	if code := runUpdateConfirm(&commandSI{}, []string{"x"}, &out, &errb); code != updateNowExitUsage {
		t.Fatalf("an argument gave exit %d", code)
	}
}
