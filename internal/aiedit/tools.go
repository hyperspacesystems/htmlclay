package aiedit

import (
	"errors"
	"slices"
	"strings"
	"unicode"

	"github.com/panphora/htmlclay/internal/helper"
)

// CustomEngine is the engine name "Custom command…" stores its command under.
const CustomEngine = "custom"

// Tool is one AI tool a person can pick: a built-in agent found on this
// computer, or a command they set up themselves.
type Tool struct {
	ID           string
	Custom       bool
	Found        bool
	DefaultModel string
	Models       []string
	Command      []string
}

var toolModels = map[string][]string{
	"claude": {"claude-opus-5-5", "claude-fable-5-2"},
	"codex":  {},
}

// Tools lists Claude and Codex, each marked found when its program is on the
// login PATH, then every user engine in name order. env nil means helper.LoginEnv().
func Tools(engines map[string][]string, env []string) []Tool {
	if env == nil {
		env = helper.LoginEnv()
	}
	builtins := builtinEngines()
	tools := make([]Tool, 0, 2+len(engines))
	for _, name := range []string{"claude", "codex"} {
		e := builtins[name]
		program, _ := engineProgram(e)
		_, err := lookPath(program, env)
		tools = append(tools, Tool{
			ID:           name,
			Found:        err == nil,
			DefaultModel: e.model,
			Models:       append([]string(nil), toolModels[name]...),
		})
	}
	resolved := resolveEngines(engines)
	names := make([]string, 0, len(resolved))
	for name, e := range resolved {
		if e.adapter == "generic" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		tools = append(tools, Tool{ID: name, Custom: true, Found: true, Command: append([]string(nil), resolved[name].command...), Models: []string{}})
	}
	return tools
}

// ErrCommand refuses a custom command line that is empty or has an unclosed quote.
var ErrCommand = errors.New("the command is empty or has an unclosed quote")

// SplitCommand splits a command line into its arguments the way a shell would
// for plain words, 'single' and "double" quotes, and backslash escapes outside
// single quotes. It runs nothing and expands nothing.
func SplitCommand(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inWord, quote, escaped := false, rune(0), false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inWord = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case unicode.IsSpace(r):
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, ErrCommand
	}
	if inWord {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, ErrCommand
	}
	return args, nil
}

// Find reports whether program can be run: a path to a file, or a name on the
// login PATH (asked again once, as Run does, before giving up).
func Find(program string) error {
	if _, err := lookPath(program, helper.LoginEnv()); err == nil {
		return nil
	}
	_, err := lookPath(program, helper.RefreshLoginEnv())
	return err
}
