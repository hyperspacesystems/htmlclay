package aiedit

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestFableRunsTheCurrentModel(t *testing.T) {
	if got, want := builtinEngines()["fable"].model, "claude-fable-5-2"; got != want {
		t.Fatalf("fable model = %q, want %q", got, want)
	}
}

func TestToolsMarksWhatIsInstalled(t *testing.T) {
	dir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + dir}
	tools := Tools(map[string][]string{"mine": {"echo", "{prompt}"}}, env)
	ids := make([]string, 0, len(tools))
	for _, tool := range tools {
		ids = append(ids, tool.ID)
	}
	if want := []string{"claude", "codex", "mine"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	found := map[string]bool{}
	for _, tool := range tools {
		found[tool.ID] = tool.Found
	}
	if !found["claude"] {
		t.Error("claude is in the temp PATH and must read as found")
	}
	if found["codex"] {
		t.Error("codex is not in the temp PATH and must read as missing")
	}
	if !found["mine"] {
		t.Error("a user engine must read as found")
	}
	if got := tools[0].DefaultModel; got != "claude-opus-5-5" {
		t.Fatalf("claude DefaultModel = %q", got)
	}
	if tools[2].Custom != true {
		t.Error("a user engine must read as custom")
	}
	if got, want := tools[2].Command, []string{"echo", "{prompt}"}; !slices.Equal(got, want) {
		t.Fatalf("mine command = %v, want %v", got, want)
	}
}

func TestSplitCommand(t *testing.T) {
	got, err := splitCommand(`my-agent --flag "two words" 'it''s' a\ b`, true)
	if err != nil {
		t.Fatalf("SplitCommand: %v", err)
	}
	if want := []string{"my-agent", "--flag", "two words", "its", "a b"}; !slices.Equal(got, want) {
		t.Fatalf("SplitCommand = %v, want %v", got, want)
	}
	if _, err := SplitCommand(`"open`); err != ErrCommand {
		t.Fatalf("unclosed quote = %v, want ErrCommand", err)
	}
	if _, err := SplitCommand("   "); err != ErrCommand {
		t.Fatalf("empty command = %v, want ErrCommand", err)
	}
}

func TestSplitCommandKeepsWindowsPaths(t *testing.T) {
	got, err := splitCommand(`"C:\Program Files\Agent\agent.exe" --dir C:\Users\maya\notes`, false)
	if err != nil {
		t.Fatalf("splitCommand: %v", err)
	}
	if want := []string{`C:\Program Files\Agent\agent.exe`, "--dir", `C:\Users\maya\notes`}; !slices.Equal(got, want) {
		t.Fatalf("splitCommand = %v, want %v", got, want)
	}
}

func TestCodexArgsCarryTheModel(t *testing.T) {
	argv := codexArgs("/d", "/o", "gpt-x")
	if !hasArg(argv, "-m") {
		t.Fatalf("argv = %v, want a -m flag", argv)
	}
	if got := argAfter(t, argv, "-m"); got != "gpt-x" {
		t.Fatalf("-m = %q, want gpt-x", got)
	}
	if slices.Index(argv, "-m") > slices.Index(argv, "-C") {
		t.Fatalf("argv = %v, want -m before -C", argv)
	}
	if hasArg(codexArgs("/d", "/o", ""), "-m") {
		t.Fatal("an empty model must add no -m flag")
	}
}

func TestRunUsesTheChosenModel(t *testing.T) {
	result, err := Run(context.Background(), Payload{
		ElementHTML: "<p>x</p>",
		Tag:         "p",
		Comment:     "tidy",
	}, Options{Mock: true, Default: "claude", Models: map[string]string{"claude": "claude-fable-5-2"}}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := result.Model, "mock(claude-fable-5-2)"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}

	result, err = Run(context.Background(), Payload{
		ElementHTML: "<p>x</p>",
		Tag:         "p",
		Comment:     "tidy",
	}, Options{Mock: true, Default: "claude"}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := result.Model, "mock(claude-opus-5-5)"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
}

func TestJoinCommandRoundTrips(t *testing.T) {
	cases := [][]string{
		{"/Users/me/Application Support/bin/agent", "--system", "be brief", "{prompt}"},
		{"agent", "it's", `say "hi"`, `a\b`, ""},
	}
	for _, escapes := range []bool{true, false} {
		for _, argv := range cases {
			if !escapes && argv[0] == "agent" {
				continue
			}
			got, err := splitCommand(joinCommand(argv, escapes), escapes)
			if err != nil || !slices.Equal(got, argv) {
				t.Fatalf("escapes=%v: %q round-trips to %q (%v)", escapes, argv, got, err)
			}
		}
	}
	win := []string{`C:\Program Files\Agent\agent.exe`, "{prompt}"}
	if got, err := splitCommand(joinCommand(win, false), false); err != nil || !slices.Equal(got, win) {
		t.Fatalf("a Windows path round-trips to %q (%v)", got, err)
	}
}

func TestFirstFoundPicksCodexWhenClaudeIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake program is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FirstFound([]string{"PATH=" + dir}); got != "codex" {
		t.Fatalf("FirstFound with only codex installed = %q, want codex", got)
	}
	if got := FirstFound([]string{"PATH=" + t.TempDir()}); got != "claude" {
		t.Fatalf("FirstFound with nothing installed = %q, want claude", got)
	}
}
