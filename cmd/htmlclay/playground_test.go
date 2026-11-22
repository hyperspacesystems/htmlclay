package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

func TestSeedDefaultPlaygroundOnFreshInstallOnly(t *testing.T) {
	t.Run("fresh install seeds an empty ~/htmlclay/workspace once and does not re-seed after removal", func(t *testing.T) {
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		a := newServedTestApp(t, home, cfgBase)
		a.loaded.FreshInstall = true

		a.seedDefaultPlayground()

		dir := filepath.Join(home, "htmlclay", "workspace")
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("expected seeded workspace folder %s, err=%v", dir, err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") {
				t.Fatalf("expected the workspace to start empty, found %s", entry.Name())
			}
		}
		if !a.rt.cfg.PlaygroundSeededDone() {
			t.Fatal("expected PlaygroundSeededDone true after seeding")
		}
		servedList := a.rt.cfg.ServedFolderList()
		if len(servedList) != 1 || servedList[0].Name != "workspace" || !servedList[0].Serving {
			t.Fatalf("served folders = %+v, want single serving workspace", servedList)
		}

		// User untrusts and deletes the workspace.
		if _, refused := a.do(context.Background(), coreAction(t, "untrust-pg", "folder.stopTrusting", map[string]any{"id": servedList[0].ID}), ChannelTray); refused != nil {
			t.Fatalf("stopTrusting workspace refused: %+v", refused)
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
			t.Fatalf("deleted workspace was recreated on next launch (stat err=%v)", err)
		}
		if len(a2.rt.cfg.ServedFolderList()) != 0 {
			t.Fatalf("workspace re-appeared in served folders: %+v", a2.rt.cfg.ServedFolderList())
		}
	})

	t.Run("existing 1.x config does not seed workspace and keeps startOnLogin false", func(t *testing.T) {
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

		if _, err := os.Stat(filepath.Join(home, "htmlclay", "workspace")); !os.IsNotExist(err) {
			t.Fatalf("existing 1.x config should not seed workspace (stat err=%v)", err)
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
		if _, err := os.Stat(filepath.Join(home, "htmlclay", "workspace")); !os.IsNotExist(err) {
			t.Fatalf("HTMLCLAY_NO_DEFAULT_PLAYGROUND=1 should suppress seeding (stat err=%v)", err)
		}
	})

	t.Run("HTMLCLAY_CANDIDATE_FRESH=1 seeds the empty workspace once with an existing config", func(t *testing.T) {
		t.Setenv("HTMLCLAY_CANDIDATE_FRESH", "1")
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		cfgDir := config.DirFrom(cfgBase)
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"startOnLogin":true}`), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, res, err := config.LoadFrom(cfgBase, platform.DirIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if res.FreshInstall {
			t.Fatal("expected a pre-written config.json to be an existing install")
		}
		a := newServedTestApp(t, home, cfgBase)
		a.rt.cfg = cfg
		a.loaded = res

		a.seedDefaultPlayground()

		dir := filepath.Join(home, "htmlclay", "workspace")
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("expected seeded workspace folder %s, err=%v", dir, err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") {
				t.Fatalf("expected the workspace to start empty, found %s", entry.Name())
			}
		}
		if !a.rt.cfg.PlaygroundSeededDone() {
			t.Fatal("expected PlaygroundSeededDone true after seeding")
		}
		servedList := a.rt.cfg.ServedFolderList()
		if len(servedList) != 1 || servedList[0].Name != "workspace" || !servedList[0].Serving {
			t.Fatalf("served folders = %+v, want single serving workspace", servedList)
		}

		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		a.seedDefaultPlayground()
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("workspace was re-seeded after PlaygroundSeeded was set (stat err=%v)", err)
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
