package aiedit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeToolClaude is a stand-in for the Claude Code CLI that answers with
// whatever stream-json result line it is handed, so one script covers success,
// a malformed reply, an oversized reply and a failing child.
const fakeToolClaude = `#!/bin/sh
[ -n "$FAKE_ARGV" ] && printf '%s\n' "$@" > "$FAKE_ARGV"
[ -n "$FAKE_STDIN" ] && cat > "$FAKE_STDIN"
[ -n "$FAKE_ENV" ] && env > "$FAKE_ENV"
prev=""
for arg in "$@"; do
  if [ "$prev" = "--system-prompt-file" ] && [ -n "$FAKE_PROMPT" ]; then cat "$arg" > "$FAKE_PROMPT"; fi
  prev="$arg"
done
printf '%s\n' '{"type":"system","subtype":"init","model":"claude-opus-5-5"}'
if [ -n "$FAKE_SLEEP" ]; then sleep "$FAKE_SLEEP"; fi
if [ -n "$FAKE_LINE" ]; then cat "$FAKE_LINE"; printf '\n'; fi
if [ -n "$FAKE_EXIT" ]; then exit "$FAKE_EXIT"; fi
exit 0
`

const fakeToolCodex = `#!/bin/sh
[ -n "$FAKE_ARGV" ] && printf '%s\n' "$@" > "$FAKE_ARGV"
[ -n "$FAKE_STDIN" ] && cat > "$FAKE_STDIN"
out=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then out="$arg"; fi
  prev="$arg"
done
cat "$FAKE_LINE" > "$out"
`

const toolRequestIDFixture = "3f1b9c62-0000-4000-8000-1234567890ab"

func writeClaudeResult(t *testing.T, dir, result, stop string) string {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"type":        "result",
		"subtype":     "success",
		"result":      result,
		"stop_reason": stop,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "line.json")
	if err := os.WriteFile(path, line, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func validToolReply(t *testing.T, javascript string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"artifact": map[string]string{
			"title":        "Interview tally",
			"html":         `<div class="tool"><button type="button">+1</button></div>`,
			"css":          ".tool{display:flex}",
			"javascript":   javascript,
			"fallbackHtml": "<p>A tally.</p>",
			"summary":      "Counts interviews.",
		},
		"state": map[string]any{"count": 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func toolRequestJSON(t *testing.T, id, prompt, extra string) []byte {
	t.Helper()
	encodedPrompt, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	request := `{"protocol":"page-tool/1","requestId":"` + id + `","prompt":` + string(encodedPrompt) +
		`,"context":{"title":"Research","outline":[],"tables":[]},"target":null,"current":null` + extra + `}`
	return []byte(request)
}

func TestPageToolClaudeGetsItsOwnArgvAndEnv(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	files := t.TempDir()
	argvFile := filepath.Join(files, "argv")
	stdinFile := filepath.Join(files, "stdin")
	envFile := filepath.Join(files, "env")
	promptFile := filepath.Join(files, "prompt")
	line := writeClaudeResult(t, files, validToolReply(t, "var count = 0;"), "end_turn")

	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "Make a tally for completed interviews", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	reply, err := RunTool(context.Background(), request, ToolOptions{
		Env:          testEnv(binDir, "FAKE_ARGV="+argvFile, "FAKE_STDIN="+stdinFile, "FAKE_ENV="+envFile, "FAKE_PROMPT="+promptFile, "FAKE_LINE="+line),
		SystemPrompt: "HOUSE RULES",
	})
	if err != nil {
		t.Fatalf("RunTool: %v", err)
	}
	if !strings.Contains(string(reply.Artifact), "Interview tally") || string(reply.State) != `{"count":0}` {
		t.Fatalf("reply = %s %s", reply.Artifact, reply.State)
	}
	if reply.Model != "claude-opus-5-5" {
		t.Fatalf("model = %q", reply.Model)
	}

	argv := argvOf(t, argvFile)
	if hasArg(argv, "--append-system-prompt") {
		t.Fatalf("argv = %q, want the page-tool's own system prompt flag", argv)
	}
	if got := argAfter(t, argv, "--max-turns"); got != "1" {
		t.Fatalf("--max-turns = %q", got)
	}
	if got := argAfter(t, argv, "--tools"); got != "" {
		t.Fatalf("--tools = %q, want an empty argument", got)
	}
	argAfter(t, argv, "--system-prompt-file")
	if got := readText(t, promptFile); got != "HOUSE RULES" {
		t.Fatalf("system prompt file = %q", got)
	}
	if !strings.Contains(readText(t, envFile), "CLAUDE_CODE_DISABLE_CLAUDE_MDS=1\n") {
		t.Fatal("the page-tool's Claude child did not get CLAUDE_CODE_DISABLE_CLAUDE_MDS=1")
	}

	var turn struct {
		Prompt  string          `json:"prompt"`
		Context map[string]any  `json:"context"`
		Current json.RawMessage `json:"current"`
	}
	if err := json.Unmarshal([]byte(readText(t, stdinFile)), &turn); err != nil {
		t.Fatalf("stdin is not the JSON turn: %v (%q)", err, readText(t, stdinFile))
	}
	if turn.Prompt != "Make a tally for completed interviews" || turn.Context["title"] != "Research" {
		t.Fatalf("turn = %+v", turn)
	}
	if strings.TrimSpace(string(turn.Current)) != "null" {
		t.Fatalf("current = %s, want null for a new tool", turn.Current)
	}
}

func TestAIEditKeepsItsEnvAndArgv(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	files := t.TempDir()
	argvFile := filepath.Join(files, "argv")
	envFile := filepath.Join(files, "env")
	line := writeClaudeResult(t, files, "<p>New</p>", "end_turn")

	_, err := Run(context.Background(), Payload{
		Tag:         "p",
		ElementHTML: `<p class="lead">old</p>`,
		Comment:     "tighten",
	}, Options{Env: testEnv(binDir, "FAKE_ARGV="+argvFile, "FAKE_ENV="+envFile, "FAKE_LINE="+line)}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	argv := argvOf(t, argvFile)
	if !hasArg(argv, "--append-system-prompt") || hasArg(argv, "--system-prompt-file") {
		t.Fatalf("ai-edit argv = %q", argv)
	}
	if strings.Contains(readText(t, envFile), "CLAUDE_CODE_DISABLE_CLAUDE_MDS") {
		t.Fatal("ai-edit's child got the page-tool's environment variable")
	}
}

func TestPageToolCodex(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "codex", fakeToolCodex)
	files := t.TempDir()
	stdinFile := filepath.Join(files, "stdin")
	line := filepath.Join(files, "reply.json")
	if err := os.WriteFile(line, []byte(validToolReply(t, "var count = 1;")), 0644); err != nil {
		t.Fatal(err)
	}

	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "@codex make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	reply, err := RunTool(context.Background(), request, ToolOptions{
		Default:      "codex",
		Env:          testEnv(binDir, "FAKE_LINE="+line, "FAKE_STDIN="+stdinFile),
		SystemPrompt: "HOUSE RULES",
	})
	if err != nil {
		t.Fatalf("RunTool: %v", err)
	}
	if !strings.Contains(string(reply.Artifact), "Interview tally") {
		t.Fatalf("artifact = %s", reply.Artifact)
	}
	if stdin := readText(t, stdinFile); !strings.HasPrefix(stdin, "HOUSE RULES\n\n") || !strings.Contains(stdin, `"prompt":"@codex make a tally"`) {
		t.Fatalf("stdin = %q", stdin)
	}
}

func TestPageToolProcessFailure(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	_, err := RunTool(context.Background(), request, ToolOptions{
		Env:          testEnv(binDir, "FAKE_EXIT=3"),
		SystemPrompt: "HOUSE RULES",
	})
	if got := errorCode(t, err); got != "engine_failed" {
		t.Fatalf("code = %q, want engine_failed", got)
	}
}

func TestPageToolMalformedReply(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	files := t.TempDir()
	line := writeClaudeResult(t, files, "I made you a tool, enjoy!", "end_turn")
	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	_, err := RunTool(context.Background(), request, ToolOptions{
		Env:          testEnv(binDir, "FAKE_LINE="+line),
		SystemPrompt: "HOUSE RULES",
	})
	if got := errorCode(t, err); got != "invalid_reply" {
		t.Fatalf("code = %q, want invalid_reply", got)
	}
}

func TestPageToolOversizedReply(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	files := t.TempDir()
	line := writeClaudeResult(t, files, validToolReply(t, "var x = \""+strings.Repeat("x", 30000)+"\";"), "end_turn")
	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	_, err := RunTool(context.Background(), request, ToolOptions{
		Env:          testEnv(binDir, "FAKE_LINE="+line),
		SystemPrompt: "HOUSE RULES",
	})
	if got := errorCode(t, err); got != "invalid_reply" {
		t.Fatalf("code = %q, want invalid_reply", got)
	}
}

func TestPageToolFencedReply(t *testing.T) {
	reply, failure := ParseToolReply("```json\n" + validToolReply(t, "var x = 1;") + "\n```")
	if failure != nil {
		t.Fatalf("ParseToolReply: %v", failure)
	}
	if !strings.Contains(string(reply.Artifact), "Interview tally") {
		t.Fatalf("artifact = %s", reply.Artifact)
	}
}

func TestPageToolCancellation(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_, err := RunTool(ctx, request, ToolOptions{
		Env:          testEnv(binDir, "FAKE_SLEEP=30"),
		SystemPrompt: "HOUSE RULES",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want a cancellation", err)
	}
}

func TestPageToolTimeout(t *testing.T) {
	requireUnix(t)
	binDir := t.TempDir()
	writeAgent(t, binDir, "claude", fakeToolClaude)
	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := RunTool(ctx, request, ToolOptions{
		Env:          testEnv(binDir, "FAKE_SLEEP=30"),
		SystemPrompt: "HOUSE RULES",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline", err)
	}
}

func TestPageToolMissingEngine(t *testing.T) {
	requireUnix(t)
	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "make a tally", ""))
	if failure != nil {
		t.Fatalf("ValidateToolRequest: %v", failure)
	}
	_, err := RunTool(context.Background(), request, ToolOptions{
		Env:          []string{"PATH=" + t.TempDir()},
		SystemPrompt: "HOUSE RULES",
	})
	if got := errorCode(t, err); got != "engine_unavailable" {
		t.Fatalf("code = %q, want engine_unavailable", got)
	}
	if message := err.Error(); !strings.Contains(message, "Install one and sign in") {
		t.Fatalf("message = %q, want setup guidance", message)
	}
}

func TestPageToolRequestValidation(t *testing.T) {
	artifact := validToolReply(t, "var x = 1;")
	var parsed struct {
		Artifact json.RawMessage `json:"artifact"`
		State    json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal([]byte(artifact), &parsed); err != nil {
		t.Fatal(err)
	}
	current := `{"artifact":` + string(parsed.Artifact) + `,"state":` + string(parsed.State) + `}`

	cases := []struct {
		name    string
		payload string
		code    string
	}{
		{"not an object", `[]`, "invalid_request"},
		{"no protocol", `{"requestId":"` + toolRequestIDFixture + `","prompt":"x"}`, "invalid_request"},
		{"wrong protocol", `{"protocol":"page-tool/2","requestId":"` + toolRequestIDFixture + `","prompt":"x"}`, "invalid_request"},
		{"bad id", `{"protocol":"page-tool/1","requestId":"nope","prompt":"x"}`, "invalid_request"},
		{"blank prompt", `{"protocol":"page-tool/1","requestId":"` + toolRequestIDFixture + `","prompt":"   "}`, "invalid_request"},
		{"long prompt", string(toolRequestJSON(t, toolRequestIDFixture, strings.Repeat("x", 9000), "")), "invalid_request"},
		{"list context", `{"protocol":"page-tool/1","requestId":"` + toolRequestIDFixture + `","prompt":"x","context":[]}`, "invalid_request"},
		{"large context", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"context":{"title":"`+strings.Repeat("x", 17000)+`"}`)), "invalid_context"},
		{"oversized payload", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"pad":"`+strings.Repeat("x", 66000)+`"`)), "invalid_request"},
		{"revision without the tool", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"target":{"toolId":"t1","rev":1}`)), "invalid_request"},
		{"tool without a revision", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"current":`+current)), "invalid_request"},
		{"revision without rev", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"target":{"toolId":"t1"},"current":`+current)), "invalid_request"},
		{"revision without a tool id", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"target":{"toolId":"","rev":1},"current":`+current)), "invalid_request"},
		{"broken current artifact", string(toolRequestJSON(t, toolRequestIDFixture, "x", `,"target":{"toolId":"t1","rev":1},"current":{"artifact":{"title":"only"},"state":{}}`)), "invalid_request"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, failure := ValidateToolRequest([]byte(test.payload))
			if failure == nil {
				t.Fatal("the request was accepted")
			}
			if failure.Code != test.code {
				t.Fatalf("code = %q, want %q (%s)", failure.Code, test.code, failure.Message)
			}
		})
	}

	request, failure := ValidateToolRequest(toolRequestJSON(t, toolRequestIDFixture, "x",
		`,"target":{"toolId":"t1","rev":1},"current":`+current))
	if failure != nil {
		t.Fatalf("a revision was refused: %v", failure)
	}
	if !strings.Contains(string(request.Current), `"artifact"`) {
		t.Fatalf("current = %s", request.Current)
	}
}

// The mock is what browser passes run with, and Go cannot run its script, so
// the reply is held to a golden copy of the artifact byte for byte. The script
// must define mount(tool): the page's runtime calls that function, and a bare
// `root` never mounts.
const mockToolGolden = `{"artifact":{"title":"Counter","html":"<div class=\"counter\"><button type=\"button\">Add one</button> <output>0</output></div>","css":".counter { display: flex; gap: 0.5rem; align-items: center; } .counter button { font: inherit; }","javascript":"function mount(tool) { const button = tool.root.querySelector('button'); const output = tool.root.querySelector('output'); const draw = () => { output.textContent = String(tool.state.data.count ?? 0); }; const click = () => tool.state.add(['count'], 1); button.addEventListener('click', click); const off = tool.on('change', draw); draw(); return () => { button.removeEventListener('click', click); off(); }; }","fallbackHtml":"<p>Counter</p>","summary":"A counter that counts up."},"state":{"count":0}}`

func TestPageToolMockIsTheGoldenArtifact(t *testing.T) {
	if mockToolReply != mockToolGolden {
		t.Fatalf("mock reply = %s\nwant %s", mockToolReply, mockToolGolden)
	}
	reply, failure := ParseToolReply(mockToolReply)
	if failure != nil {
		t.Fatalf("ParseToolReply: %v", failure)
	}
	var artifact map[string]string
	if err := json.Unmarshal(reply.Artifact, &artifact); err != nil {
		t.Fatalf("artifact = %s (%v)", reply.Artifact, err)
	}
	if !strings.Contains(artifact["javascript"], "function mount(tool)") || !strings.Contains(artifact["javascript"], "tool.root.querySelector") {
		t.Fatalf("javascript = %q", artifact["javascript"])
	}
	if string(reply.State) != `{"count":0}` {
		t.Fatalf("state = %s", reply.State)
	}
}

func TestPageToolReplyValidation(t *testing.T) {
	valid := validToolReply(t, "var x = 1;")
	cases := []struct {
		name string
		text string
		code string
	}{
		{"empty", "   ", "incomplete"},
		{"prose", "here is your tool", "invalid_reply"},
		{"a list", `[1,2]`, "invalid_reply"},
		{"no state", `{"artifact":{}}`, "invalid_reply"},
		{"an extra key", strings.TrimSuffix(valid, "}") + `,"model":"x"}`, "invalid_reply"},
		{"state is a list", strings.Replace(valid, `"state":{"count":0}`, `"state":[]`, 1), "invalid_reply"},
		{"a short artifact", `{"artifact":{"title":"x"},"state":{}}`, "invalid_reply"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, failure := ParseToolReply(test.text)
			if failure == nil {
				t.Fatal("the reply was accepted")
			}
			if failure.Code != test.code {
				t.Fatalf("code = %q, want %q (%s)", failure.Code, test.code, failure.Message)
			}
		})
	}
	if _, failure := ParseToolReply(valid); failure != nil {
		t.Fatalf("a valid reply was refused: %v", failure)
	}
}
