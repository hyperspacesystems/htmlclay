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
	if refusal := set("claude", "max"); refusal != nil {
		t.Fatalf("claude effort max = %+v, want none", refusal)
	}
	if refusal := set("agy", "xhigh"); refusal == nil || refusal.Code != appcore.RefuseBadArguments {
		t.Fatalf("agy default model (gemini-4-carbon) with xhigh = %+v, want bad-arguments", refusal)
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
	if got := fresh.AIEditEfforts()["claude"]; got != "max" {
		t.Fatalf("saved claude effort = %q, want max", got)
	}
	for _, tool := range a.aiEditTools() {
		if tool.ID == "codex" && (tool.Effort != "xhigh" || tool.DefaultEffort != "high" || tool.DefaultModel != "gpt-5.6-sol") {
			t.Fatalf("listed codex = %+v, want effort xhigh, default effort high, default model gpt-5.6-sol", tool)
		}
		if tool.ID == "agy" && len(tool.ModelEfforts["gemini-4-neon"]) != 4 {
			t.Fatalf("listed agy ModelEfforts = %+v, want gemini-4-neon with 4 efforts", tool.ModelEfforts)
		}
	}
}

func TestAIEditSetModelClampsUnsupportedSavedEffort(t *testing.T) {
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, t.TempDir(), cfgBase)
	do := func(typ string, args any) *appcore.Refusal {
		raw, _ := json.Marshal(args)
		_, refusal := a.do(context.Background(), appcore.Action{Type: typ, Args: raw}, ChannelShell)
		return refusal
	}
	if r := do("aiEdit.setModel", map[string]string{"tool": "agy", "model": "gemini-4-neon"}); r != nil {
		t.Fatalf("setModel neon: %+v", r)
	}
	if r := do("aiEdit.setEffort", map[string]string{"tool": "agy", "effort": "xhigh"}); r != nil {
		t.Fatalf("setEffort xhigh on neon: %+v", r)
	}
	if r := do("aiEdit.setModel", map[string]string{"tool": "agy", "model": "gemini-4-carbon"}); r != nil {
		t.Fatalf("setModel carbon: %+v", r)
	}
	fresh, _, err := config.LoadFrom(cfgBase, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := fresh.AIEditEfforts()["agy"]; got != "" {
		t.Fatalf("stale xhigh effort survived switch to gemini-4-carbon: got %q, want empty", got)
	}
	for _, tool := range a.aiEditTools() {
		if tool.ID == "agy" && tool.Effort != "" {
			t.Fatalf("snapshot agy tool still has stale effort %q after switching to gemini-4-carbon", tool.Effort)
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
