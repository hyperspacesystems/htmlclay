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
