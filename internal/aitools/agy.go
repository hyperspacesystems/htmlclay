package aitools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/panphora/htmlclay/internal/runner"
)

// AgySelfTestModel is the cheap model agy's self-test runs with.
const AgySelfTestModel = "gemini-3.8-flash-medium"

// agyMaxPrompt bounds the prompt agy takes as one argument: Linux refuses one
// argument over 128 KiB, Windows a whole command line over 32,767 characters,
// and macOS takes 1 MiB with the environment.
var agyMaxPrompt = map[string]int{"linux": 120 << 10, "windows": 24 << 10}[runtime.GOOS]

func init() {
	if agyMaxPrompt == 0 {
		agyMaxPrompt = 768 << 10
	}
}

// agyDenied are the tool rules agy refuses. command(*) matters most: with any
// command allowed, agy's terminal reached every canary outside the folder
// (agy spike). search_web cannot be refused by any rule tried; it is listed so
// a later agy that honors it does. The URL, browser and MCP tools agy 1.5.13
// lists are refused by name too.
var agyDenied = []string{
	"command(*)", "write_file(*)", "edit_file(*)", "search_web(*)",
	"read_url_content(*)", "open_browser_url(*)", "browser(*)", "execute_browser_javascript(*)", "call_mcp_tool(*)",
	"invoke_subagent(*)", "define_subagent(*)", "manage_subagents(*)", "swarm(*)", "schedule(*)",
	"send_message(*)", "run_workflow(*)", "manage_task(*)",
}

// agyUnsafePath is a folder path agy's rule syntax could read two ways.
var agyUnsafePath = regexp.MustCompile(`[()*?\[\]{},"\x00-\x1f]`)

// AgySettings is the session-only settings object: read_file allowed inside
// the folder and nowhere else, nothing outside the workspace.
func AgySettings(folder string) string {
	settings := map[string]any{
		"allowNonWorkspaceAccess": false,
		"trustedWorkspaces":       []string{},
		"permissions": map[string]any{
			"allow": []string{"read_file(" + strings.TrimRight(folder, `/\`) + "/**)"},
			"deny":  agyDenied,
		},
	}
	b, _ := json.Marshal(settings)
	return string(b)
}

func agyArgs(folder, prompt, model string, schema json.RawMessage) []string {
	argv := []string{"--settings", AgySettings(folder), "--output-format", "json", "--json-schema", string(schema), "--disable-slash-commands"}
	if model != "" {
		argv = append(argv, "--model", model)
	}
	return append(argv, "-p="+prompt)
}

type agyResult struct {
	Status           string          `json:"status"`
	Response         string          `json:"response"`
	Error            string          `json:"error"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

var agyRefusedTool = regexp.MustCompile(`required the "([a-z_]+)" permission`)

// runAgy runs agy in the folder. agy keeps its sign-in under the person's own
// home, so HOME is the real one; the session settings replace its permissions.
func runAgy(ctx context.Context, program string, loginEnv []string, home string, c call) (reply, error) {
	if agyUnsafePath.MatchString(c.Folder) {
		return reply{}, fail(CodeEngineUnsupported, "Gemini (agy) cannot be held to a folder whose path has brackets, commas, quotes or wildcards. Choose another folder or another agent.")
	}
	if len(c.Prompt) > agyMaxPrompt {
		return reply{}, fail(CodeEngineUnsupported, "This request is too long for Gemini (agy). Ask about a smaller part of the folder, or choose another agent.")
	}
	argv := append([]string{program}, agyArgs(c.Folder, c.Prompt, c.Model, c.Schema)...)
	total := 0
	for _, a := range argv {
		total += len(a) + 3
	}
	if runtime.GOOS == "windows" && total > 32000 {
		return reply{}, fail(CodeEngineUnsupported, "This request is too long for Gemini (agy). Ask about a smaller part of the folder, or choose another agent.")
	}
	res := run(ctx, proc{
		Argv:    argv,
		Dir:     c.Folder,
		Env:     runner.CleanEnv(c.Scratch, agentPath(program, loginEnv), "HOME="+home, "USERPROFILE="+home),
		Started: c.Started,
	})
	out := append(append([]byte(nil), res.Stdout...), res.Stderr...)
	if err := ctx.Err(); err != nil {
		return reply{Output: out}, err
	}
	if m := agyRefusedTool.FindSubmatch(res.Stderr); m != nil {
		what := "use a tool HTML Clay does not allow"
		switch string(m[1]) {
		case "read_file":
			what = "read a file outside " + baseName(c.Folder)
		case "command":
			what = "run a command"
		case "write_file", "edit_file":
			what = "change a file"
		}
		refusal := fail(CodeEngineFailed, fmt.Sprintf("The agent tried to %s, so it was stopped before it could. Nothing outside the folder was read. Try again; asking about a smaller part of the folder can help.", what))
		refusal.refused = true
		return reply{Output: out}, refusal
	}
	var r agyResult
	if json.Unmarshal(res.Stdout, &r) != nil {
		message := "Gemini (agy) stopped without an answer."
		if t := tail(res.Stderr, 300); t != "" {
			message += " It said: " + t
		}
		return reply{Output: out}, fail(CodeEngineFailed, message)
	}
	if r.Status != "SUCCESS" {
		message := strings.TrimSpace(r.Error)
		if message == "" {
			message = strings.ToLower(r.Status)
		}
		return reply{Output: out}, fail(CodeEngineFailed, "Gemini (agy) could not answer: "+message)
	}
	answer := r.StructuredOutput
	if len(answer) == 0 || string(answer) == "null" {
		var ok bool
		if answer, ok = jsonObject(r.Response); !ok {
			return reply{Output: out, Model: c.Model}, fail(CodeResultInvalid, "Gemini (agy)'s answer was not the JSON the page asked for.")
		}
	}
	return reply{Answer: answer, Model: c.Model, Output: out}, nil
}
