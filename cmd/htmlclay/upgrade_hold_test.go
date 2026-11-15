package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/update"
)

func writeUpdateState(t *testing.T, configDir, phase string) {
	t.Helper()
	dir := update.UpdatesDir(configDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(update.State{Schema: update.StateSchema, Phase: phase, From: "1.0.0", To: "2.0.0", Txid: "t1", Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeSavesWaitForTheVerdict(t *testing.T) {
	for _, verdict := range []string{update.PhaseDone, update.PhaseRolledBack} {
		t.Run(verdict, func(t *testing.T) {
			home, cfgBase := t.TempDir(), t.TempDir()
			a := newTestAppWithConfigDir(t, home, cfgBase)
			t.Cleanup(a.shutdown)
			if err := a.rt.cfg.Save(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(config.DirFrom(cfgBase), "config.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			writeUpdateState(t, a.rt.configDir, update.PhaseApplying)
			a.holdSavesWhileUnjudged()
			a.rt.cfg.SetStartOnLogin(!a.rt.cfg.StartOnLoginEnabled())
			if err := a.rt.cfg.Save(); err != nil {
				t.Fatal(err)
			}
			if after, _ := os.ReadFile(path); string(after) != string(before) {
				t.Fatal("a save during the trial wrote config.json")
			}

			writeUpdateState(t, a.rt.configDir, verdict)
			a.settleUpgradeSave()
			after, _ := os.ReadFile(path)
			if changed := string(after) != string(before); changed != (verdict == update.PhaseDone) {
				t.Fatalf("verdict %s: config.json changed = %v", verdict, changed)
			}
		})
	}
}

// TestStartupHoldsSavesWhileUnjudged runs main's own startup steps in main's
// order, so the hold sees the config folder before finishUpgrade saves.
func TestStartupHoldsSavesWhileUnjudged(t *testing.T) {
	// A short base keeps the single-instance socket path under the Unix limit.
	base, err := os.MkdirTemp("/tmp", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	t.Setenv("HTMLCLAY_CONFIG_BASE", base)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	writeUpdateState(t, dir, update.PhaseApplying)

	a := &app{rt: &appRuntime{}}
	a.initConfig()
	t.Cleanup(func() { a.rt.si.Unlock() })
	a.initLogger()
	t.Cleanup(func() { a.rt.logger.Close() })
	a.holdSavesWhileUnjudged()

	a.rt.cfg.SetStartOnLogin(!a.rt.cfg.StartOnLoginEnabled())
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("a save during the trial wrote config.json (stat err %v)", err)
	}
}

// TestUpdateHandoffOutlivesTheTrial: during an update's trial the new version
// restores the handoff but leaves the file for the previous version's
// supervisor, which reads it on a rollback. A kept update removes it; a rolled
// back one leaves it; a launch outside a trial removes it on read as before.
func TestUpdateHandoffOutlivesTheTrial(t *testing.T) {
	for _, tc := range []struct {
		name, before, verdict string
		afterRestore, afterSettle bool
	}{
		{"kept", update.PhaseApplying, update.PhaseDone, true, false},
		{"rolled back", update.PhaseApplying, update.PhaseRolledBack, true, true},
		{"no trial", "", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAppWithConfigDir(t, t.TempDir(), t.TempDir())
			t.Cleanup(a.shutdown)
			path := filepath.Join(a.rt.configDir, updateHandoffFile)
			if err := os.MkdirAll(a.rt.configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"schema":0}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.before != "" {
				writeUpdateState(t, a.rt.configDir, tc.before)
			}
			_, _ = a.restoreUpdateHandoff(path)
			if _, err := os.Stat(path); (err == nil) != tc.afterRestore {
				t.Fatalf("after restore: file present = %v, want %v", err == nil, tc.afterRestore)
			}
			if tc.verdict == "" {
				return
			}
			writeUpdateState(t, a.rt.configDir, tc.verdict)
			a.settleUpgradeSave()
			if _, err := os.Stat(path); (err == nil) != tc.afterSettle {
				t.Fatalf("after the %s verdict: file present = %v, want %v", tc.verdict, err == nil, tc.afterSettle)
			}
		})
	}
}
