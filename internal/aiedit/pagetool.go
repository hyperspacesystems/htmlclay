package aiedit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/panphora/htmlclay/internal/helper"
)

// The built-in blockeditor-tool helper: one `blockeditor-tool/1` request in, one tool
// artifact out, run through the same agents AI editing uses.
//
// The request and reply rules are the hosted runner's, ported from
// server-lib/ai/page-tool-contract.js: the sizes are capped on both ends and
// neither side is trusted. The hosted runner also parses the artifact's
// JavaScript in a vm; Go has no JavaScript parser here, so a reply carrying any
// `javascript` string within the limits is accepted. The page's own runtime
// check covers the syntax before the tool mounts.
const (
	PageToolHelperName = "blockeditor-tool"
	ToolProtocol       = "blockeditor-tool/1"
)

const (
	toolArtifactBytes = 24576
	toolStateBytes    = 8192
	toolContextBytes  = 16384
	toolPromptBytes   = 8192
	toolPayloadBytes  = 65536
)

var toolArtifactFields = []string{"title", "html", "css", "javascript", "fallbackHtml", "summary"}

var (
	toolRequestID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	toolFence     = regexp.MustCompile("(?s)^" + "```" + `[a-zA-Z0-9_-]*\n(.*?)\n?` + "```" + "$")
)

// ToolRequest is one validated `blockeditor-tool/1` request. Current is nil unless the
// request revises a tool that already exists.
type ToolRequest struct {
	RequestID string
	Prompt    string
	Context   json.RawMessage
	Current   json.RawMessage
}

// ToolReply is one validated reply: the artifact and state exactly as the model
// wrote them, and the model the engine reported.
type ToolReply struct {
	Artifact json.RawMessage
	State    json.RawMessage
	Model    string
}

// ToolOptions is what a blockeditor-tool run needs. SystemPrompt is the tool-generation
// system.md and context.md, read from the embedded runtime by the host.
type ToolOptions struct {
	BaseDir      string // folder a user-configured engine runs in (the document's)
	Default      string // default engine name; "" means "claude"
	Engines      map[string][]string
	Env          []string
	Mock         bool
	Status       func(text string)
	SystemPrompt string
}

func toolFailure(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// encodedToolBytes is the size JSON.stringify would produce: the compact form of
// the JSON, in bytes.
func encodedToolBytes(raw json.RawMessage) int {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return len(raw)
	}
	return compact.Len()
}

func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var probe map[string]json.RawMessage
	return json.Unmarshal(trimmed, &probe) == nil
}

func jsonString(raw json.RawMessage) (string, bool) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false
	}
	return text, true
}

func toolArtifactProblem(raw json.RawMessage) string {
	if !isJSONObject(raw) {
		return "The tool artifact must be an object."
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "The tool artifact must be an object."
	}
	if len(fields) != len(toolArtifactFields) {
		return "The tool artifact must have exactly title, html, css, javascript, fallbackHtml and summary."
	}
	for _, field := range toolArtifactFields {
		if _, ok := jsonString(fields[field]); !ok {
			return "The tool artifact field " + field + " must be a string."
		}
	}
	if encodedToolBytes(raw) > toolArtifactBytes {
		return "The tool artifact is too large."
	}
	return ""
}

func toolStateProblem(raw json.RawMessage) string {
	if !isJSONObject(raw) {
		return "The tool state must be a JSON object."
	}
	if encodedToolBytes(raw) > toolStateBytes {
		return "The tool state is too large."
	}
	return ""
}

// ValidateToolRequest checks one wire payload as contract C, with the hosted
// runner's limits and its error codes.
func ValidateToolRequest(payload json.RawMessage) (ToolRequest, *Error) {
	if !isJSONObject(payload) {
		return ToolRequest{}, toolFailure("invalid_request", "A tool request must be an object.")
	}
	if encodedToolBytes(payload) > toolPayloadBytes {
		return ToolRequest{}, toolFailure("invalid_request", "This tool request is too large.")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return ToolRequest{}, toolFailure("invalid_request", "A tool request must be an object.")
	}

	protocol, _ := jsonString(fields["protocol"])
	if protocol != ToolProtocol {
		return ToolRequest{}, toolFailure("invalid_request", "A tool request must say protocol "+ToolProtocol+".")
	}
	requestID, _ := jsonString(fields["requestId"])
	if !toolRequestID.MatchString(requestID) {
		return ToolRequest{}, toolFailure("invalid_request", "A tool request needs a UUID requestId.")
	}
	prompt, _ := jsonString(fields["prompt"])
	if strings.TrimSpace(prompt) == "" {
		return ToolRequest{}, toolFailure("invalid_request", "A tool request needs a prompt.")
	}
	if len(prompt) > toolPromptBytes {
		return ToolRequest{}, toolFailure("invalid_request", "The prompt is too long.")
	}

	context := json.RawMessage("{}")
	if raw, ok := fields["context"]; ok && !isJSONNull(raw) {
		if !isJSONObject(raw) {
			return ToolRequest{}, toolFailure("invalid_request", "The document context must be an object.")
		}
		if encodedToolBytes(raw) > toolContextBytes {
			return ToolRequest{}, toolFailure("invalid_context", "This document context is too large to send.")
		}
		context = raw
	}

	targetRaw := fields["target"]
	hasTarget := !isJSONNull(targetRaw)
	if hasTarget {
		if !isJSONObject(targetRaw) {
			return ToolRequest{}, toolFailure("invalid_request", "A revision must name the tool it revises.")
		}
		var target map[string]json.RawMessage
		if err := json.Unmarshal(targetRaw, &target); err != nil {
			return ToolRequest{}, toolFailure("invalid_request", "A revision must name the tool it revises.")
		}
		if toolID, _ := jsonString(target["toolId"]); toolID == "" {
			return ToolRequest{}, toolFailure("invalid_request", "A revision must name the tool it revises.")
		}
		rev := target["rev"]
		var asString string
		var asNumber float64
		if json.Unmarshal(rev, &asString) != nil && json.Unmarshal(rev, &asNumber) != nil {
			return ToolRequest{}, toolFailure("invalid_request", "A revision must name the revision it started from.")
		}
	}

	currentRaw := fields["current"]
	hasCurrent := !isJSONNull(currentRaw)
	if hasTarget && !hasCurrent {
		return ToolRequest{}, toolFailure("invalid_request", "A revision must include the tool as it stands.")
	}
	if hasCurrent {
		if !isJSONObject(currentRaw) {
			return ToolRequest{}, toolFailure("invalid_request", "The current tool must be an object.")
		}
		if !hasTarget {
			return ToolRequest{}, toolFailure("invalid_request", "A current tool must belong to a revision.")
		}
		var current map[string]json.RawMessage
		if err := json.Unmarshal(currentRaw, &current); err != nil {
			return ToolRequest{}, toolFailure("invalid_request", "The current tool must be an object.")
		}
		if problem := toolArtifactProblem(current["artifact"]); problem != "" {
			return ToolRequest{}, toolFailure("invalid_request", problem)
		}
		if problem := toolStateProblem(current["state"]); problem != "" {
			return ToolRequest{}, toolFailure("invalid_request", problem)
		}
	}

	request := ToolRequest{RequestID: requestID, Prompt: prompt, Context: context}
	if hasCurrent {
		request.Current = currentRaw
	}
	return request, nil
}

// ParseToolReply reads one model reply: a JSON object with exactly an artifact
// and a state, either bare or inside a markdown fence.
func ParseToolReply(text string) (ToolReply, *Error) {
	trimmed := strings.TrimSpace(text)
	if fenced := toolFence.FindStringSubmatch(trimmed); fenced != nil {
		trimmed = strings.TrimSpace(fenced[1])
	}
	if trimmed == "" {
		return ToolReply{}, toolFailure("incomplete", "the model produced no reply")
	}
	if !isJSONObject(json.RawMessage(trimmed)) {
		return ToolReply{}, toolFailure("invalid_reply", "the model's reply is not a tool")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return ToolReply{}, toolFailure("invalid_reply", "the model's reply is not a tool")
	}
	if len(fields) != 2 {
		return ToolReply{}, toolFailure("invalid_reply", "the model's reply is not a tool")
	}
	artifact, hasArtifact := fields["artifact"]
	state, hasState := fields["state"]
	if !hasArtifact || !hasState {
		return ToolReply{}, toolFailure("invalid_reply", "the model's reply is not a tool")
	}
	if problem := toolArtifactProblem(artifact); problem != "" {
		return ToolReply{}, toolFailure("invalid_reply", problem)
	}
	if problem := toolStateProblem(state); problem != "" {
		return ToolReply{}, toolFailure("invalid_reply", problem)
	}
	return ToolReply{Artifact: artifact, State: state}, nil
}

// toolUserTurn is the user turn for this helper: the prompt, the document
// context and, on a revision, the tool as it stands.
func toolUserTurn(p ToolRequest) string {
	turn := struct {
		Prompt  string          `json:"prompt"`
		Context json.RawMessage `json:"context"`
		Current json.RawMessage `json:"current"`
	}{Prompt: p.Prompt, Context: p.Context, Current: p.Current}
	if len(turn.Context) == 0 {
		turn.Context = json.RawMessage("{}")
	}
	if len(turn.Current) == 0 {
		turn.Current = json.RawMessage("null")
	}
	encoded, err := json.Marshal(turn)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// toolMDSEnv switches off CLAUDE.md discovery for the blockeditor-tool's Claude child,
// so the house rules the host sends are the only instructions it follows.
const toolMDSEnv = "CLAUDE_CODE_DISABLE_CLAUDE_MDS=1"

// toolEnv is a copy of env with the blockeditor-tool's own variable set. ai-edit's env
// is never touched: both helpers share the caller's slice.
func toolEnv(env []string) []string {
	copied := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, "CLAUDE_CODE_DISABLE_CLAUDE_MDS") {
			continue
		}
		copied = append(copied, entry)
	}
	return append(copied, toolMDSEnv)
}

// toolClaudeArgs is the blockeditor-tool's Claude Code invocation: no tools, one turn,
// and the host's system prompt standing in for the default one.
func toolClaudeArgs(model, systemPromptPath string) []string {
	return []string{
		"-p",
		"--model", model,
		"--system-prompt-file", systemPromptPath,
		"--tools", "",
		"--strict-mcp-config",
		"--no-session-persistence",
		"--max-turns", "1",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
	}
}

func runToolClaude(ctx context.Context, e engine, bin, systemPrompt, userPrompt string, env []string) (Result, error) {
	scratch, err := os.MkdirTemp("", "blockeditor-tool-claude-")
	if err != nil {
		return Result{}, engineFailure(err)
	}
	defer os.RemoveAll(scratch)
	systemPromptPath := filepath.Join(scratch, "system.md")
	if err := os.WriteFile(systemPromptPath, []byte(systemPrompt), 0600); err != nil {
		return Result{}, engineFailure(err)
	}
	argv := append([]string{bin}, toolClaudeArgs(e.model, systemPromptPath)...)
	return runClaude(ctx, e, argv, userPrompt, toolEnv(env), func(string) {})
}

// mockToolHang is the marker a test puts in a prompt to hold a mock run open
// until its context ends, so cancellation and budget expiry are deterministic.
var mockToolHang = regexp.MustCompile(`\[mock:hang\]`)

// mockToolReply is a fixed, valid counter tool, and its script is one the page's
// runtime can mount: it defines mount(tool), which reads the count out of the
// tool's own state and returns its cleanup.
const mockToolReply = `{"artifact":{"title":"Counter","html":"<div class=\"counter\"><button type=\"button\">Add one</button> <output>0</output></div>","css":".counter { display: flex; gap: 0.5rem; align-items: center; } .counter button { font: inherit; }","javascript":"function mount(tool) { const button = tool.root.querySelector('button'); const output = tool.root.querySelector('output'); const draw = () => { output.textContent = String(tool.state.data.count ?? 0); }; const click = () => tool.state.add(['count'], 1); button.addEventListener('click', click); const off = tool.on('change', draw); draw(); return () => { button.removeEventListener('click', click); off(); }; }","fallbackHtml":"<p>Counter</p>","summary":"A counter that counts up."},"state":{"count":0}}`

func mockToolRun(ctx context.Context, p ToolRequest, label string) (Result, error) {
	if mockToolHang.MatchString(p.Prompt) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	return Result{HTML: mockToolReply, Model: "mock(" + label + ")", StopReason: "end_turn"}, nil
}

// RunTool answers one blockeditor-tool request with the configured agent. A cancelled
// ctx kills the agent and returns ctx.Err().
func RunTool(ctx context.Context, p ToolRequest, o ToolOptions) (ToolReply, error) {
	engines := resolveEngines(o.Engines)
	defaultName := strings.ToLower(o.Default)
	if defaultName == "" {
		defaultName = "claude"
	}
	chosen, ok := engines[defaultName]
	if !ok {
		return ToolReply{}, &Error{Code: "unknown_engine", Message: fmt.Sprintf("default engine %q is not configured", defaultName)}
	}
	if chosen.adapter == "unsupported" {
		return ToolReply{}, &Error{
			Code:    "engine_unsupported",
			Message: fmt.Sprintf("@%s can't make tools: it can read files without asking, and making a tool only runs agents with no tools", chosen.name),
		}
	}
	status := o.Status
	if status == nil {
		status = func(string) {}
	}

	label := chosen.model
	if label == "" {
		label = chosen.name
	}
	if o.Mock {
		status("Making your tool with " + statusLabel(chosen))
		return toolFinish(mockToolRun(ctx, p, label))
	}

	program, programErr := engineProgram(chosen)
	if programErr != nil {
		return ToolReply{}, programErr
	}
	env := o.Env
	if env == nil {
		env = helper.LoginEnv()
	}
	bin, lookErr := lookPath(program, env)
	if lookErr != nil && o.Env == nil {
		env = helper.RefreshLoginEnv()
		bin, lookErr = lookPath(program, env)
	}
	if lookErr != nil {
		other, found := engines[fallbackName(defaultName)]
		if !found {
			return ToolReply{}, unavailableError(chosen)
		}
		otherProgram, otherProgramErr := engineProgram(other)
		if otherProgramErr != nil {
			return ToolReply{}, otherProgramErr
		}
		otherBin, otherErr := lookPath(otherProgram, env)
		if otherErr != nil {
			return ToolReply{}, &Error{Code: "engine_unavailable", Message: "Neither Claude Code nor Codex is installed. Install one and sign in."}
		}
		status(fmt.Sprintf("%s isn't installed, making your tool with %s", programLabel(chosen), statusLabel(other)))
		chosen, program, bin = other, otherProgram, otherBin
	}
	status("Making your tool with " + statusLabel(chosen))

	turn := toolUserTurn(p)
	var result Result
	var err error
	switch chosen.adapter {
	case "claude":
		result, err = runToolClaude(ctx, chosen, bin, o.SystemPrompt, turn, env)
	case "codex":
		result, err = runCodex(ctx, chosen, bin, o.SystemPrompt+"\n\n"+turn, env)
	default:
		result, err = runGeneric(ctx, chosen, bin, o.SystemPrompt+"\n\n"+turn, o.BaseDir, env, func(string) {})
	}
	if err != nil {
		return ToolReply{}, err
	}
	return toolFinish(result, nil)
}

// toolFinish turns one engine result into a validated reply.
func toolFinish(result Result, err error) (ToolReply, error) {
	if err != nil {
		return ToolReply{}, err
	}
	if _, err := finish(result); err != nil {
		return ToolReply{}, err
	}
	reply, replyErr := ParseToolReply(result.HTML)
	if replyErr != nil {
		return ToolReply{}, replyErr
	}
	reply.Model = result.Model
	return reply, nil
}
