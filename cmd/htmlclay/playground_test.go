package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
)

func TestSeedDefaultPlaygroundOnFreshInstallOnly(t *testing.T) {
	t.Run("fresh install seeds ~/htmlclay/playground once and does not re-seed after removal", func(t *testing.T) {
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		a := newServedTestApp(t, home, cfgBase)
		a.loaded.FreshInstall = true

		a.seedDefaultPlayground()

		dir := filepath.Join(home, "htmlclay", "playground")
		for _, name := range []string{"welcome.html", "journal.html", "kanban.html", "freelance-rate-calculator.html"} {
			p := filepath.Join(dir, name)
			data, err := os.ReadFile(p)
			if err != nil || len(data) == 0 {
				t.Fatalf("expected seeded starter %s, err=%v len=%d", p, err, len(data))
			}
			if v := a.tierOf(p); v.Tier != tier.Mine {
				t.Fatalf("seeded starter %s tier = %s (%s), want mine", name, v.Tier, v.Reason)
			}
		}
		welcomeBytes, _ := os.ReadFile(filepath.Join(dir, "welcome.html"))
		if !strings.Contains(string(welcomeBytes), "Build software in a single HTML file") {
			t.Fatalf("welcome.html missing Option A headline")
		}
		if !a.rt.cfg.PlaygroundSeededDone() {
			t.Fatal("expected PlaygroundSeededDone true after seeding")
		}
		servedList := a.rt.cfg.ServedFolderList()
		if len(servedList) != 1 || servedList[0].Name != "playground" || !servedList[0].Serving {
			t.Fatalf("served folders = %+v, want single serving playground", servedList)
		}

		// User untrusts and deletes playground.
		if _, refused := a.do(context.Background(), coreAction(t, "untrust-pg", "folder.stopTrusting", map[string]any{"id": servedList[0].ID}), ChannelTray); refused != nil {
			t.Fatalf("stopTrusting playground refused: %+v", refused)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}

		// Next launch loads saved config (FreshInstall == false, PlaygroundSeeded == true) and never recreates it.
		a2 := newServedTestApp(t, home, cfgBase)
		cfg2, res2, err := config.LoadFrom(cfgBase, platform.DirIdentity)
		if err != nil {
			t.Fatal(err)
		}
		a2.rt.cfg = cfg2
		a2.loaded = res2
		a2.seedDefaultPlayground()
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("deleted playground was recreated on next launch (stat err=%v)", err)
		}
		if len(a2.rt.cfg.ServedFolderList()) != 0 {
			t.Fatalf("playground re-appeared in served folders: %+v", a2.rt.cfg.ServedFolderList())
		}
	})

	t.Run("existing 1.x config does not seed playground and keeps startOnLogin false", func(t *testing.T) {
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		cfgDir := config.DirFrom(cfgBase)
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"startOnLogin":false}`), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, res, err := config.LoadFrom(cfgBase, platform.DirIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if res.FreshInstall || cfg.StartOnLoginEnabled() {
			t.Fatalf("existing config = fresh:%v startOnLogin:%v, want false/false", res.FreshInstall, cfg.StartOnLoginEnabled())
		}
		a := newServedTestApp(t, home, cfgBase)
		a.rt.cfg = cfg
		a.loaded = res
		a.seedDefaultPlayground()

		if _, err := os.Stat(filepath.Join(home, "htmlclay", "playground")); !os.IsNotExist(err) {
			t.Fatalf("existing 1.x config should not seed playground (stat err=%v)", err)
		}
		if len(a.rt.cfg.ServedFolderList()) != 0 {
			t.Fatalf("existing 1.x config should have 0 served folders, got %+v", a.rt.cfg.ServedFolderList())
		}
	})

	t.Run("HTMLCLAY_NO_DEFAULT_PLAYGROUND=1 suppresses seeding", func(t *testing.T) {
		t.Setenv("HTMLCLAY_NO_DEFAULT_PLAYGROUND", "1")
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		a := newServedTestApp(t, home, cfgBase)
		a.loaded.FreshInstall = true

		a.seedDefaultPlayground()
		if _, err := os.Stat(filepath.Join(home, "htmlclay", "playground")); !os.IsNotExist(err) {
			t.Fatalf("HTMLCLAY_NO_DEFAULT_PLAYGROUND=1 should suppress seeding (stat err=%v)", err)
		}
	})
}

func TestHandleInstanceCommandOpenDashboard(t *testing.T) {
	home := evalTempHome(t)
	a := newServedTestApp(t, home, t.TempDir())
	opened := make(chan string, 1)
	a.rt.launchBrowser = func(target string) error {
		opened <- target
		return nil
	}
	if got := a.handleInstanceCommand("open-dashboard extra"); got != "refused unknown command" {
		t.Fatalf("open-dashboard with extra arg = %q", got)
	}
	if got := a.handleInstanceCommand("open-dashboard"); got != "ok" {
		t.Fatalf("handleInstanceCommand(open-dashboard) = %q, want ok", got)
	}
	url := <-opened
	if !strings.Contains(url, "/enter?ticket=") {
		t.Fatalf("opened %q, want dashboard entry URL", url)
	}
}
