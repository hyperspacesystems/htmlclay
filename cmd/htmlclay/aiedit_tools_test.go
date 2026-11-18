package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
)

func TestCustomCommandMustExist(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "aiEdit.setCustom", Command: "definitely-not-installed-7f3 {prompt}"})
	if refusal == nil || refusal.Code != appcore.RefuseCommandNotFound {
		t.Fatalf("refusal = %+v, want command-not-found", refusal)
	}
	if refusal.OtherName != "definitely-not-installed-7f3" {
		t.Fatalf("OtherName = %q, want the program's name", refusal.OtherName)
	}
	refusal = a.runEffect(context.Background(), appcore.Effect{Type: "aiEdit.setCustom", Command: `"open`})
	if refusal == nil || refusal.Code != appcore.RefuseCommandInvalid {
		t.Fatalf("refusal = %+v, want command-invalid", refusal)
	}
}

func TestCustomCommandBecomesTheTool(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	script := filepath.Join(t.TempDir(), "my-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "aiEdit.setCustom", Command: script + " {prompt}"}); refusal != nil {
		t.Fatalf("refusal = %+v, want none", refusal)
	}
	if got := a.aiEditTool(); got != "custom" {
		t.Fatalf("aiEditTool() = %q, want custom", got)
	}
	found := false
	for _, tool := range a.aiEditTools() {
		if tool.ID != "custom" {
			continue
		}
		found = true
		if !tool.Custom {
			t.Error("the custom tool must read as custom")
		}
		if want := script + " {prompt}"; tool.Command != want {
			t.Errorf("command = %q, want %q", tool.Command, want)
		}
	}
	if !found {
		t.Fatal("the tool picker does not list the custom command")
	}
}

func TestAIEditToolPreferenceRefusesUnknown(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	value := []byte(`"nope"`)
	refusal := a.setPreference(appcore.Effect{Type: "prefs.set", Key: "aiEditTool", Value: value})
	if refusal == nil || refusal.Code != appcore.RefuseNoSuchTool {
		t.Fatalf("refusal = %+v, want no-such-tool", refusal)
	}
}

func TestAIEditEffortIsCheckedSavedAndListed(t *testing.T) {
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, t.TempDir(), cfgBase)
	set := func(tool, effort string) *appcore.Refusal {
		args, _ := json.Marshal(map[string]string{"tool": tool, "effort": effort})
		_, refusal := a.do(context.Background(), appcore.Action{Type: "aiEdit.setEffort", Args: args}, ChannelShell)
		return refusal
	}
	if refusal := set("codex", "ultra"); refusal == nil || refusal.Code != appcore.RefuseBadArguments {
		t.Fatalf("an effort codex does not list = %+v, want bad-arguments", refusal)
	}
	if refusal := set("claude", "high"); refusal == nil || refusal.Code != appcore.RefuseNotAllowedNow {
		t.Fatalf("an effort for a tool with no effort setting = %+v, want not-allowed-now", refusal)
	}
	if refusal := set("codex", "xhigh"); refusal != nil {
		t.Fatalf("aiEdit.setEffort = %+v, want none", refusal)
	}
	fresh, _, err := config.LoadFrom(cfgBase, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := fresh.AIEditEfforts()["codex"]; got != "xhigh" {
		t.Fatalf("saved codex effort = %q, want xhigh", got)
	}
	for _, tool := range a.aiEditTools() {
		if tool.ID == "codex" && (tool.Effort != "xhigh" || tool.DefaultEffort != "high" || tool.DefaultModel != "gpt-5.6-sol") {
			t.Fatalf("listed codex = %+v, want effort xhigh, default effort high, default model gpt-5.6-sol", tool)
		}
	}
}

func TestAIEditModelIsSaved(t *testing.T) {
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, t.TempDir(), cfgBase)
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "aiEdit.setModel", Tool: "claude", Model: "claude-fable-5-2"}); refusal != nil {
		t.Fatalf("refusal = %+v, want none", refusal)
	}
	fresh, _, err := config.LoadFrom(cfgBase, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := fresh.AIEditModels()["claude"]; got != "claude-fable-5-2" {
		t.Fatalf("AIEditModels()[claude] = %q, want claude-fable-5-2", got)
	}
}
