package aiedit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeClaude is a stand-in for the Claude Code CLI: it records its argv and
// stdin, then answers with the stream-json lines the real one prints.
const fakeClaude = `#!/bin/sh
[ -n "$FAKE_ARGV" ] && printf '%s\n' "$@" > "$FAKE_ARGV"
[ -n "$FAKE_STDIN" ] && cat > "$FAKE_STDIN"
printf '%s\n' '{"type":"system","subtype":"init","model":"claude-opus-5-5"}'
printf '%s\n' '{"type":"stream_event","event":{"delta":{"type":"text_delta","text":"Hello "}}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"<p>New</p>","stop_reason":"end_turn"}'
`

const fakeCodex = `#!/bin/sh
[ -n "$FAKE_ARGV" ] && printf '%s\n' "$@" > "$FAKE_ARGV"
[ -n "$FAKE_STDIN" ] && cat > "$FAKE_STDIN"
out=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then out="$arg"; fi
  prev="$arg"
done
printf '%s' '<p>From codex</p>' > "$out"
`

func requireUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
}

func writeAgent(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

// testEnv is this process's environment with PATH led by the fake agents,
// plus whatever extra variables the fake needs.
func testEnv(binDir string, extra ...string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra)+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PATH=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return append(env, extra...)
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func argvOf(t *testing.T, path string) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(readText(t, path), "\n"), "\n")
}

func hasArg(argv []string, want string) bool {
	for _, arg := range argv {
		if arg == want {
			return true
		}
	}
	return false
}

func argAfter(t *testing.T, argv []string, flag string) string {
	t.Helper()
	for i, arg := range argv {
		if arg == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	t.Fatalf("%s missing from argv %q", flag, argv)
	return ""
}

func errorCode(t *testing.T, err error) string {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) {
		t.Fatalf("want a coded error, got %v", err)
	}
	return failure.Code
}

func TestClaudeIsTheDefaultEngine(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeClaude)
	argvFile := filepath.Join(t.TempDir(), "argv")
	stdinFile := filepath.Join(t.TempDir(), "stdin")

	var streamed strings.Builder
	result, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: `<p class="lead">old</p>`,
		Comment:     "tighten",
	}, Options{Env: testEnv(binDir, "FAKE_ARGV="+argvFile, "FAKE_STDIN="+stdinFile)}, func(text string) {
		streamed.WriteString(text)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.HTML != "<p>New</p>" {
		t.Fatalf("html = %q", result.HTML)
	}
	if result.Model != "claude-opus-5-5" {
		t.Fatalf("model = %q", result.Model)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stopReason = %q", result.StopReason)
	}
	if !strings.Contains(streamed.String(), "Hello ") {
		t.Fatalf("progress = %q", streamed.String())
	}
	argv := argvOf(t, argvFile)
	if !hasArg(argv, "--strict-mcp-config") {
		t.Fatalf("argv = %q", argv)
	}
	if got := argAfter(t, argv, "--model"); got != "claude-opus-5-5" {
		t.Fatalf("--model = %q", got)
	}
	if got := argAfter(t, argv, "--tools"); got != "" {
		t.Fatalf("--tools = %q, want an empty argument", got)
	}
	if stdin := readText(t, stdinFile); !strings.Contains(stdin, "Request: tighten") {
		t.Fatalf("stdin = %q", stdin)
	}
}

func TestFableEngine(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeClaude)
	argvFile := filepath.Join(t.TempDir(), "argv")
	stdinFile := filepath.Join(t.TempDir(), "stdin")

	_, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: `<p class="lead">old</p>`,
		Comment:     "@fable tighten",
	}, Options{Env: testEnv(binDir, "FAKE_ARGV="+argvFile, "FAKE_STDIN="+stdinFile)}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := argAfter(t, argvOf(t, argvFile), "--model"); got != "claude-fable-5-2" {
		t.Fatalf("--model = %q", got)
	}
	stdin := readText(t, stdinFile)
	if !strings.Contains(stdin, "Request: tighten") {
		t.Fatalf("stdin = %q", stdin)
	}
	if strings.Contains(stdin, "@fable") {
		t.Fatalf("stdin still names the engine: %q", stdin)
	}
}

func TestCodexEngine(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "codex", fakeCodex)
	argvFile := filepath.Join(t.TempDir(), "argv")
	stdinFile := filepath.Join(t.TempDir(), "stdin")

	result, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: `<p class="lead">old</p>`,
		Comment:     "@codex tighten",
	}, Options{Env: testEnv(binDir, "FAKE_ARGV="+argvFile, "FAKE_STDIN="+stdinFile)}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.HTML != "<p>From codex</p>" {
		t.Fatalf("html = %q", result.HTML)
	}
	argv := argvOf(t, argvFile)
	if !hasArg(argv, "--disable") || !hasArg(argv, "shell_tool") {
		t.Fatalf("argv = %q", argv)
	}
	if argv[len(argv)-1] != "-" {
		t.Fatalf("last argument = %q", argv[len(argv)-1])
	}
	if stdin := readText(t, stdinFile); !strings.HasPrefix(stdin, systemPrompt) {
		t.Fatalf("stdin does not start with the system prompt: %q", stdin)
	}
}

func TestClaudeArgsEmitsValidEffort(t *testing.T) {
	argv := claudeArgs("claude-opus-5-5", "max")
	if got := argAfter(t, argv, "--effort"); got != "max" {
		t.Fatalf("--effort = %q, want max", got)
	}
	if hasArg(claudeArgs("claude-opus-5-5", "bogus"), "--effort") {
		t.Fatal("invalid effort must not emit --effort")
	}
	promptIdx := -1
	toolsIdx := -1
	mcpIdx := -1
	for i, arg := range argv {
		switch arg {
		case "--append-system-prompt":
			promptIdx = i
		case "--tools":
			toolsIdx = i
		case "--strict-mcp-config":
			mcpIdx = i
		}
	}
	if promptIdx < 0 || toolsIdx < 0 || mcpIdx < 0 || toolsIdx > promptIdx || mcpIdx > promptIdx {
		t.Fatalf("lockdown flags must precede --append-system-prompt: %q", argv)
	}
}

func TestLookPathWindowsRefusesBatchShims(t *testing.T) {
	dir := t.TempDir()
	cmdShim := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(cmdShim, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + dir, "PATHEXT=.COM;.EXE;.BAT;.CMD"}
	if _, err := lookPathOS("windows", "claude", env); !errors.Is(err, errScriptOnly) {
		t.Fatalf("lookPathOS(windows, claude) with only .cmd = %v, want errScriptOnly", err)
	}
	if _, err := lookPathOS("windows", cmdShim, env); !errors.Is(err, errScriptOnly) {
		t.Fatalf("lookPathOS(windows, explicit .cmd) = %v, want errScriptOnly", err)
	}
	exeBin := filepath.Join(dir, "claude.exe")
	if err := os.WriteFile(exeBin, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := lookPathOS("windows", "claude", env)
	if err != nil {
		t.Fatalf("lookPathOS(windows, claude) with .exe = %v", err)
	}
	if !strings.EqualFold(got, exeBin) {
		t.Fatalf("got %q, want %q", got, exeBin)
	}
}

func TestResolveAgyModelClampsEffort(t *testing.T) {
	cases := []struct {
		model, effort, want string
	}{
		{"gemini-4-carbon", "xhigh", "gemini-4-carbon-high"},
		{"gemini-4-argon", "xhigh", "gemini-4-argon-high"},
		{"gemini-3.8-flash", "xhigh", "gemini-3.8-flash-high"},
		{"gemini-4-neon", "xhigh", "gemini-4-neon-xhigh"},
		{"gemini-4-carbon", "", "gemini-4-carbon-high"},
		{"gemini-4-carbon", "low", "gemini-4-carbon-low"},
		{"gemini-4-carbon-medium", "xhigh", "gemini-4-carbon-medium"},
	}
	for _, tc := range cases {
		if got := resolveAgyModel(tc.model, tc.effort); got != tc.want {
			t.Errorf("resolveAgyModel(%q, %q) = %q, want %q", tc.model, tc.effort, got, tc.want)
		}
	}
}

func TestAgyEngineLockdownStreamingAndCleanup(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	argvFile := filepath.Join(t.TempDir(), "argv")
	cwdFile := filepath.Join(t.TempDir(), "cwd")
	settingsCopy := filepath.Join(t.TempDir(), "settings.json")
	settingsPathFile := filepath.Join(t.TempDir(), "settings.path")
	fakeAgy := `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_ARGV"
pwd -P > "$FAKE_CWD"
prev=""
for arg in "$@"; do
  if [ "$prev" = "--settings" ]; then
    printf '%s' "$arg" > "$FAKE_SETTINGS_PATH"
    cp "$arg" "$FAKE_SETTINGS_COPY"
  fi
  prev="$arg"
done
printf '%s\n' '{"event":"init","init":{"model":"gemini-4-carbon-high"}}'
printf '%s\n' '{"event":"step_update","step_update":{"text_delta":"<p>Hel"}}'
printf '%s\n' '{"event":"step_update","step_update":{"text_delta":"lo</p>"}}'
printf '%s\n' '{"event":"result","result":{"status":"SUCCESS","response":"\u0060\u0060\u0060html\n<p>Hello</p>\n\u0060\u0060\u0060"}}'
`
	writeAgent(t, binDir, "agy", fakeAgy)

	var streamed strings.Builder
	result, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: "<p>old</p>",
		Comment:     "@agy tighten",
	}, Options{
		Env: testEnv(binDir,
			"FAKE_ARGV="+argvFile,
			"FAKE_CWD="+cwdFile,
			"FAKE_SETTINGS_COPY="+settingsCopy,
			"FAKE_SETTINGS_PATH="+settingsPathFile,
		),
		Models:  map[string]string{"agy": "gemini-4-carbon"},
		Efforts: map[string]string{"agy": "xhigh"},
	}, func(text string) {
		streamed.WriteString(text)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.HTML != "<p>Hello</p>" {
		t.Fatalf("html = %q, want <p>Hello</p>", result.HTML)
	}
	if streamed.String() != "<p>Hello</p>" {
		t.Fatalf("streamed = %q, want <p>Hello</p>", streamed.String())
	}
	argv := argvOf(t, argvFile)
	if got := argAfter(t, argv, "--model"); got != "gemini-4-carbon-high" {
		t.Fatalf("--model = %q, want gemini-4-carbon-high (clamped from xhigh)", got)
	}
	if !hasArg(argv, "--sandbox") || !hasArg(argv, "--disable-slash-commands") {
		t.Fatalf("argv = %q, want --sandbox and --disable-slash-commands", argv)
	}
	// Verify prompt combines systemPrompt + userPrompt in -p=...
	argvRaw := readText(t, argvFile)
	if !strings.Contains(argvRaw, "-p="+systemPrompt) || !strings.Contains(argvRaw, "Request: tighten") {
		t.Fatalf("argvRaw = %q, want -p= containing systemPrompt and Request: tighten", argvRaw)
	}
	// Verify lockdown.json is outside cmd.Dir (child work dir) and cleaned up after exit.
	settingsPath := readText(t, settingsPathFile)
	cwd := strings.TrimSpace(readText(t, cwdFile))
	if filepath.Dir(settingsPath) == cwd {
		t.Fatalf("lockdown.json (%s) must sit outside empty child workDir (%s)", settingsPath, cwd)
	}
	if _, err := os.Stat(settingsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lockdown.json (%s) was not removed after exit", settingsPath)
	}
	settingsText := readText(t, settingsCopy)
	for _, want := range []string{`"allowNonWorkspaceAccess":false`, `"enableTerminalSandbox":true`, `"read_file(*)"`, `"run_command(*)"`, `"command(*)"`} {
		if !strings.Contains(settingsText, want) {
			t.Fatalf("lockdown.json missing %s: %s", want, settingsText)
		}
	}
}

func TestAgyErrorAndOversizedPrompt(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "agy", "#!/bin/sh\nprintf '%s\\n' '{\"event\":\"result\",\"result\":{\"status\":\"ERROR\",\"error\":\"quota exceeded\"}}'\n")
	_, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: "<p>old</p>",
		Comment:     "@agy tighten",
	}, Options{Env: testEnv(binDir)}, nil)
	if code := errorCode(t, err); code != "engine_failed" || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("err = %v, want engine_failed mentioning quota exceeded", err)
	}

	_, err = Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: "<p>" + strings.Repeat("a", agyMaxPromptBytes()+100) + "</p>",
		Comment:     "@agy tighten",
	}, Options{Env: testEnv(binDir)}, nil)
	if code := errorCode(t, err); code != "engine_unsupported" {
		t.Fatalf("oversized prompt code = %q, want engine_unsupported", code)
	}
}

func TestUnknownEngine(t *testing.T) {
	_, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: "<p>old</p>",
		Comment:     "@nope x",
	}, Options{Env: []string{"PATH=" + t.TempDir()}}, nil)
	if code := errorCode(t, err); code != "unknown_engine" {
		t.Fatalf("code = %q", code)
	}
	if !strings.Contains(err.Error(), "@claude @fable @codex @agy") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestContextReferences(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeClaude)
	stdinFile := filepath.Join(t.TempDir(), "stdin")
	argvFile := filepath.Join(t.TempDir(), "argv")

	root := t.TempDir()
	baseDir := filepath.Join(root, "doc")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	runAs := func(t *testing.T, files bool, refs ...string) (Result, error) {
		t.Helper()
		if err := os.Remove(stdinFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return Run(context.Background(), Payload{
			Tag:         "p",
			ElementHTML: "<p>old</p>",
			Comment:     "tighten",
			ContextRefs: refs,
		}, Options{
			BaseDir:      baseDir,
			Env:          testEnv(binDir, "FAKE_ARGV="+argvFile, "FAKE_STDIN="+stdinFile),
			ContextFiles: files,
		}, nil)
	}
	run := func(t *testing.T, ref string) (Result, error) {
		t.Helper()
		return runAs(t, true, ref)
	}

	t.Run("escape", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, "outside.md"), []byte("out"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := run(t, "../outside.md")
		if code := errorCode(t, err); code != "invalid_context" {
			t.Fatalf("code = %q", code)
		}
		if !strings.Contains(err.Error(), "escapes") {
			t.Fatalf("message = %q", err.Error())
		}
	})

	t.Run("symlink", func(t *testing.T) {
		target := filepath.Join(root, "secret.md")
		if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(baseDir, "link.md")); err != nil {
			t.Fatal(err)
		}
		_, err := run(t, "link.md")
		if code := errorCode(t, err); code != "invalid_context" {
			t.Fatalf("code = %q", code)
		}
	})

	t.Run("hidden", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(baseDir, ".env"), []byte("SECRET=1"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(baseDir, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(baseDir, ".git", "config"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(baseDir, ".env"), filepath.Join(baseDir, "env-link.md")); err != nil {
			t.Fatal(err)
		}
		for _, ref := range []string{".env", ".git/config", "env-link.md"} {
			_, err := run(t, ref)
			if code := errorCode(t, err); code != "invalid_context" {
				t.Fatalf("%s: code = %q", ref, code)
			}
			if !strings.Contains(err.Error(), "hidden or internal") {
				t.Fatalf("%s: message = %q", ref, err.Error())
			}
		}
	})

	t.Run("too large", func(t *testing.T) {
		big := strings.Repeat("x", 300<<10)
		if err := os.WriteFile(filepath.Join(baseDir, "big.txt"), []byte(big), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := run(t, "big.txt")
		if code := errorCode(t, err); code != "invalid_context" {
			t.Fatalf("code = %q", code)
		}
		if !strings.Contains(err.Error(), "256 KB") {
			t.Fatalf("message = %q", err.Error())
		}
	})

	t.Run("read into the prompt", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(baseDir, "notes.md"), []byte("hello notes"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := run(t, "notes.md"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		stdin := readText(t, stdinFile)
		if !strings.Contains(stdin, "Context file @notes.md:") {
			t.Fatalf("stdin = %q", stdin)
		}
		if !strings.Contains(stdin, "hello notes") {
			t.Fatalf("stdin = %q", stdin)
		}
	})

	t.Run("a document that may not attach files", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(baseDir, "notes.md"), []byte("hello notes"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := runAs(t, false, "page", "notes.md")
		if code := errorCode(t, err); code != "invalid_context" {
			t.Fatalf("code = %q", code)
		}
		if !strings.Contains(err.Error(), "@notes.md") || !strings.Contains(err.Error(), "trust") {
			t.Fatalf("message = %q", err.Error())
		}
		if _, err := os.Stat(stdinFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("the engine started for a request that attached a file it may not")
		}
		if _, err := runAs(t, false, "page"); err != nil {
			t.Fatalf("the page alone must still work: %v", err)
		}
	})
}

func TestGenericEngines(t *testing.T) {
	requireUnix(t)
	element := "<p>old</p>"
	payload := Payload{Tag: "p", ElementHTML: element, Comment: "@echo hi"}

	echoed, err := Run(context.Background(), payload, Options{
		Engines: map[string][]string{"echo": {"sh", "-c", "cat"}},
		Env:     testEnv(t.TempDir()),
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(echoed.HTML, "The element to edit:") || !strings.Contains(echoed.HTML, "Request: hi") {
		t.Fatalf("html = %q", echoed.HTML)
	}

	payload.Comment = "@arg hi"
	substituted, err := Run(context.Background(), payload, Options{
		Engines: map[string][]string{"arg": {"printf", "%s", "{prompt}"}},
		Env:     testEnv(t.TempDir()),
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(substituted.HTML, "Request: hi") {
		t.Fatalf("html = %q", substituted.HTML)
	}
	if strings.Contains(substituted.HTML, "{prompt}") {
		t.Fatalf("the placeholder reached the agent: %q", substituted.HTML)
	}
}

func TestGenericEngineRunsInTheFolderOnlyWithSiblingFiles(t *testing.T) {
	requireUnix(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, files := range []bool{false, true} {
		got, err := Run(context.Background(), Payload{Tag: "p", ElementHTML: "<p>old</p>", Comment: "@where hi"}, Options{
			File:         filepath.Join(dir, "page.html"),
			BaseDir:      dir,
			Engines:      map[string][]string{"where": {"sh", "-c", "pwd -P"}},
			Env:          testEnv(t.TempDir()),
			ContextFiles: files,
		}, nil)
		if err != nil {
			t.Fatalf("files=%v: Run: %v", files, err)
		}
		if inFolder := strings.TrimSpace(got.HTML) == dir; inFolder != files {
			t.Fatalf("files=%v: the engine ran in %q, the document's folder is %q", files, strings.TrimSpace(got.HTML), dir)
		}
	}
}

func TestStopReasons(t *testing.T) {
	requireUnix(t)
	cases := map[string]string{
		"refusal":    "declined",
		"max_tokens": "incomplete",
	}
	for stop, want := range cases {
		t.Run(stop, func(t *testing.T) {
			binDir := t.TempDir()
			writeAgent(t, binDir, "claude", "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"<p>x</p>\",\"stop_reason\":\""+stop+"\"}'\n")
			_, err := Run(context.Background(), Payload{
				Tag:         "p",
				ElementHTML: "<p>old</p>",
				Comment:     "tighten",
			}, Options{Env: testEnv(binDir)}, nil)
			if code := errorCode(t, err); code != want {
				t.Fatalf("code = %q, want %q", code, want)
			}
		})
	}
}

func TestCancelKillsTheAgent(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", "#!/bin/sh\nsleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	_, err := Run(ctx, Payload{
		Tag:         "p",
		ElementHTML: "<p>old</p>",
		Comment:     "tighten",
	}, Options{Env: testEnv(binDir)}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Run took %s", elapsed)
	}
}

func TestMock(t *testing.T) {
	pieces := 0
	var streamed strings.Builder
	result, err := Run(context.Background(), Payload{
		Tag:         "div",
		ElementHTML: `<div class="hero" data-id="1"><p>Hello there</p></div>`,
		Comment:     "make it [mock:end_turn]",
	}, Options{Mock: true}, func(text string) {
		pieces++
		streamed.WriteString(text)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result.HTML, "mock edit: make it") {
		t.Fatalf("html = %q", result.HTML)
	}
	if result.Model != "mock(claude-opus-5-5)" {
		t.Fatalf("model = %q", result.Model)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stopReason = %q", result.StopReason)
	}
	if pieces < 2 {
		t.Fatalf("progress called %d times", pieces)
	}
	if streamed.String() != result.HTML {
		t.Fatalf("progress streamed %q", streamed.String())
	}
}

func TestMalformedRequest(t *testing.T) {
	cases := map[string]Payload{
		"comment": {Tag: "p", ElementHTML: "<p>old</p>"},
		"element": {Tag: "p", Comment: "tighten"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Run(context.Background(), payload, Options{Mock: true}, nil)
			if code := errorCode(t, err); code != "invalid_request" {
				t.Fatalf("code = %q", code)
			}
		})
	}
}

func TestIsDocument(t *testing.T) {
	for _, path := range []string{"a.html", "B.HTMLCLAY", "/tmp/x/y.Html"} {
		if !IsDocument(path) {
			t.Fatalf("%s is a document", path)
		}
	}
	for _, path := range []string{"a.htm", "a.txt", "html", ""} {
		if IsDocument(path) {
			t.Fatalf("%s is not a document", path)
		}
	}
}

func TestCodexReplyOverCapIsRefused(t *testing.T) {
	requireUnix(t)
	dir := t.TempDir()
	writeAgent(t, dir, "codex", `#!/bin/sh
out=""
prev=""
for arg in "$@"; do
 if [ "$prev" = "-o" ]; then out="$arg"; fi
 prev="$arg"
done
/usr/bin/head -c 5242880 /dev/zero | /usr/bin/tr '\000' x > "$out"
`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Run(ctx, Payload{ElementHTML: "<p>x</p>", Comment: "@codex rewrite"}, Options{File: filepath.Join(dir, "app.html"), BaseDir: dir, Env: testEnv(dir)}, nil)
	if err == nil || !strings.Contains(err.Error(), "reply too large") {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeLineOverCapIsRefusedWhileReading(t *testing.T) {
	requireUnix(t)
	dir := t.TempDir()
	writeAgent(t, dir, "claude", `#!/bin/sh
/usr/bin/head -c 5242880 /dev/zero | /usr/bin/tr '\000' x
/bin/sleep 5
`)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	_, err := Run(ctx, Payload{ElementHTML: "<p>x</p>", Comment: "rewrite"}, Options{File: filepath.Join(dir, "app.html"), BaseDir: dir, Env: testEnv(dir)}, nil)
	if err == nil || !strings.Contains(err.Error(), "reply too large") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("refused only after %s: the cap waited for the deadline", elapsed)
	}
}

// statusLog records the lifecycle status of one run.
type statusLog struct {
	texts []string
}

func (s *statusLog) record(text string) { s.texts = append(s.texts, text) }

func (s *statusLog) check(t *testing.T, want ...string) {
	t.Helper()
	if len(s.texts) != len(want) {
		t.Fatalf("status = %q, want %q", s.texts, want)
	}
	for i, text := range want {
		if s.texts[i] != text {
			t.Fatalf("status[%d] = %q, want %q", i, s.texts[i], text)
		}
	}
}

// isolatedEnv is a PATH that holds only the fake agents, so an agent installed
// on this machine never decides what a test finds.
func isolatedEnv(binDir string, extra ...string) []string {
	return append([]string{"PATH=" + binDir}, extra...)
}

func plainPayload(comment string) Payload {
	return Payload{Tag: "p", ElementHTML: "<p>old</p>", Comment: comment}
}

func TestDefaultClaudeFallsBackToCodex(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "codex", fakeCodex)

	var status statusLog
	result, err := Run(context.Background(), plainPayload("tighten"), Options{
		Env:    isolatedEnv(binDir),
		Status: status.record,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.HTML != "<p>From codex</p>" {
		t.Fatalf("html = %q", result.HTML)
	}
	status.check(t, "Claude Code isn't installed, editing with Codex")
}

func TestDefaultFableFallsBackToCodex(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "codex", fakeCodex)

	var status statusLog
	result, err := Run(context.Background(), plainPayload("tighten"), Options{
		Default: "fable",
		Env:     isolatedEnv(binDir),
		Status:  status.record,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.HTML != "<p>From codex</p>" {
		t.Fatalf("html = %q", result.HTML)
	}
	status.check(t, "Claude Code isn't installed, editing with Codex")
}

func TestExplicitMissingClaudeDoesNotFallBack(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	writeAgent(t, binDir, "codex", "#!/bin/sh\n: > \"$FAKE_MARKER\"\n")

	var status statusLog
	_, err := Run(context.Background(), plainPayload("@claude tighten"), Options{
		Env:    isolatedEnv(binDir, "FAKE_MARKER="+marker),
		Status: status.record,
	}, nil)
	if code := errorCode(t, err); code != "engine_unavailable" {
		t.Fatalf("code = %q", code)
	}
	if !strings.HasPrefix(err.Error(), "@claude isn't available: `claude` was not found") {
		t.Fatalf("message = %q", err.Error())
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("the codex agent was started for a request that named claude")
	}
	status.check(t)
}

func TestBothAgentsMissing(t *testing.T) {
	var status statusLog
	_, err := Run(context.Background(), plainPayload("tighten"), Options{
		Env:    isolatedEnv(t.TempDir()),
		Status: status.record,
	}, nil)
	if code := errorCode(t, err); code != "engine_unavailable" {
		t.Fatalf("code = %q", code)
	}
	want := "None of Claude Code, Codex, or Gemini (agy) is installed. Install one and sign in."
	if err.Error() != want {
		t.Fatalf("message = %q, want %q", err.Error(), want)
	}
	status.check(t)
}

func TestClaudePresentDoesNotFallBack(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeClaude)

	var status statusLog
	result, err := Run(context.Background(), plainPayload("tighten"), Options{
		Env:    isolatedEnv(binDir),
		Status: status.record,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Model != "claude-opus-5-5" || result.HTML != "<p>New</p>" {
		t.Fatalf("result = %+v", result)
	}
	status.check(t, "Editing with Claude Code")
}

func TestConfiguredCodexDefaultFallsBackToClaude(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeClaude)

	var status statusLog
	result, err := Run(context.Background(), plainPayload("tighten"), Options{
		Default: "codex",
		Env:     isolatedEnv(binDir),
		Status:  status.record,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Model != "claude-opus-5-5" || result.HTML != "<p>New</p>" {
		t.Fatalf("result = %+v", result)
	}
	status.check(t, "Codex isn't installed, editing with Claude Code")
}

func TestConfiguredDefaultDoesNotFallBack(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeClaude)

	var status statusLog
	_, err := Run(context.Background(), plainPayload("tighten"), Options{
		Default: "mine",
		Engines: map[string][]string{"mine": {filepath.Join(binDir, "mine-agent")}},
		Env:     isolatedEnv(binDir),
		Status:  status.record,
	}, nil)
	if code := errorCode(t, err); code != "engine_unavailable" {
		t.Fatalf("code = %q", code)
	}
	if !strings.Contains(err.Error(), "@mine isn't available: its command was not found") {
		t.Fatalf("message = %q", err.Error())
	}
	status.check(t)
}
