package aitools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/runner"
)

// ClaudeSelfTestModel is the cheap model the self-test and the sign-in
// renewal run with.
const ClaudeSelfTestModel = "haiku"

const (
	claudeMaxTurns = "40"
	helpTimeout    = 15 * time.Second
	renewTimeout   = 90 * time.Second
	// a token with less left than one whole request is renewed first
	tokenRenewAhead = 20 * time.Minute
)

var (
	claudeReadTools   = []string{"Read", "Grep", "Glob"}
	claudeChangeTools = []string{"Read", "Grep", "Glob", "Edit", "Write"}
)

// claudeAnswerTool is the tool --json-schema adds, through which Claude
// returns its answer. It reads and writes nothing.
const claudeAnswerTool = "StructuredOutput"

func claudeTools(level string) []string {
	if level == LevelChange {
		return claudeChangeTools
	}
	return claudeReadTools
}

// claudeArgs is the whole command line after the program. --restricted is
// what holds Claude Code to the folder and keeps a project's hooks, settings
// and CLAUDE.md from running; --setting-sources "" loads no settings file at
// all, so a project's .claude/settings.json cannot widen what Read reaches.
// The token never appears here: it travels only in the environment.
func claudeArgs(level, model string, schema json.RawMessage) []string {
	mode := "dontAsk"
	if level == LevelChange {
		mode = "acceptEdits"
	}
	argv := []string{
		"-p",
		"--restricted",
		"--tools", strings.Join(claudeTools(level), ","),
		"--strict-mcp-config",
		"--setting-sources", "",
		"--permission-mode", mode,
		"--no-session-persistence",
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", claudeMaxTurns,
		"--json-schema", string(schema),
	}
	if model != "" {
		argv = append(argv, "--model", model)
	}
	return argv
}

// claudeEnv is the environment of a run: a scratch HOME, so Claude never reads
// the person's own settings, hooks, MCP servers or CLAUDE.md, and the token
// from Claude's own sign-in.
func claudeEnv(scratch, path, token string) []string {
	tmp := filepath.Join(scratch, "tmp")
	os.MkdirAll(tmp, 0o700)
	return runner.CleanEnv(scratch, path,
		"CLAUDE_CODE_OAUTH_TOKEN="+token,
		"CLAUDE_CODE_TMPDIR="+tmp,
		"DISABLE_TELEMETRY=1",
		"DISABLE_AUTOUPDATER=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	)
}

type claudeEvent struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	Model            string          `json:"model"`
	Tools            []string        `json:"tools"`
	MCPServers       []any           `json:"mcp_servers"`
	IsError          bool            `json:"is_error"`
	APIErrorStatus   int             `json:"api_error_status"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Message          struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

// claudeHasRestricted reports whether this Claude Code lists --restricted in
// its help. One without it cannot be held to the folder and is refused.
func claudeHasRestricted(ctx context.Context, program string, loginEnv []string, scratch string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, helpTimeout)
	defer cancel()
	res := run(ctx, proc{Argv: []string{program, "--help"}, Dir: scratch, Env: runner.CleanEnv(scratch, agentPath(program, loginEnv))})
	if res.Err != nil && len(res.Stdout) == 0 {
		return false, fmt.Errorf("claude --help: %w", res.Err)
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.TrimSuffix(f[0], ",") == "--restricted" {
			return true, nil
		}
	}
	return false, nil
}

// runClaude runs one Claude Code call in c.Folder with token. It stops the
// run as soon as Claude reports tools or MCP servers HTML Clay did not ask
// for, and answers errSignIn when Anthropic refused the token.
func runClaude(ctx context.Context, program string, loginEnv []string, token string, c call) (reply, error) {
	allowed := append(slices.Clone(claudeTools(c.Level)), claudeAnswerTool)
	var result *claudeEvent
	model := c.Model
	var unexpected string
	var tools []toolCall
	res := run(ctx, proc{
		Argv:    append([]string{program}, claudeArgs(c.Level, c.Model, c.Schema)...),
		Dir:     c.Folder,
		Env:     claudeEnv(c.Scratch, agentPath(program, loginEnv), token),
		Stdin:   c.Prompt,
		Started: c.Started,
		Line: func(line []byte) bool {
			var ev claudeEvent
			if json.Unmarshal(line, &ev) != nil {
				return true
			}
			switch {
			case ev.Type == "system" && ev.Subtype == "init":
				if ev.Model != "" {
					model = ev.Model
				}
				for _, tool := range ev.Tools {
					if !slices.Contains(allowed, tool) {
						unexpected = "the tool " + tool
						return false
					}
				}
				if len(ev.MCPServers) > 0 {
					unexpected = "an MCP server"
					return false
				}
			case ev.Type == "assistant":
				for _, part := range ev.Message.Content {
					if part.Type == "tool_use" {
						tools = append(tools, toolCall{Name: part.Name, Input: part.Input})
					}
				}
			case ev.Type == "result":
				seen := ev
				result = &seen
			}
			return true
		},
	})
	out := append(append([]byte(nil), res.Stdout...), res.Stderr...)
	if err := ctx.Err(); err != nil {
		return reply{Output: out, Tools: tools}, err
	}
	if unexpected != "" {
		return reply{Output: out}, fail(CodeEngineUnsupported, fmt.Sprintf("This Claude Code started with %s, which this HTML Clay does not know, so HTML Clay stopped it before it read anything. HTML Clay needs an update to work with this Claude Code.", unexpected))
	}
	if result == nil {
		message := "Claude Code stopped without an answer."
		if t := tail(res.Stderr, 300); t != "" {
			message += " It said: " + t
		}
		return reply{Output: out}, fail(CodeEngineFailed, message)
	}
	if result.IsError {
		if result.APIErrorStatus == 401 {
			return reply{Output: out}, errSignIn
		}
		message := strings.TrimSpace(result.Result)
		if message == "" {
			message = result.Subtype
		}
		return reply{Output: out, Tools: tools}, fail(CodeEngineFailed, "Claude Code could not answer: "+message)
	}
	answer := result.StructuredOutput
	if len(answer) == 0 || string(answer) == "null" {
		var ok bool
		if answer, ok = jsonObject(result.Result); !ok {
			return reply{Output: out, Model: model, Tools: tools}, fail(CodeResultInvalid, "Claude Code's answer was not the JSON the page asked for.")
		}
	}
	return reply{Answer: answer, Model: model, Output: out, Tools: tools}, nil
}

// renewClaude runs one short Claude Code call with no folder, no tools and
// the person's real home, exactly as if they ran claude in Terminal, so Claude
// refreshes its own sign-in record. HTML Clay never refreshes the token
// itself: refresh tokens rotate, and a refresh Claude does not write back
// would sign the person out of Claude Code.
func renewClaude(ctx context.Context, program string, loginEnv []string, home, scratch string) error {
	ctx, cancel := context.WithTimeout(ctx, renewTimeout)
	defer cancel()
	res := run(ctx, proc{
		Argv: []string{program, "-p", "--strict-mcp-config", "--setting-sources", "", "--disable-slash-commands", "--output-format", "json", "--no-session-persistence", "--max-turns", "1", "--tools", "", "--model", ClaudeSelfTestModel, "Reply with the word OK."},
		Dir:  scratch,
		Env:  runner.CleanEnv(scratch, agentPath(program, loginEnv), "HOME="+home, "USERPROFILE="+home),
	})
	return res.Err
}
