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
