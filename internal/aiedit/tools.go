package aiedit

import (
	"errors"
	"runtime"
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
	// DefaultEffort and Efforts are the tool's reasoning effort setting; Efforts
	// is empty for a tool that has none. ModelEfforts overrides Efforts for
	// specific models whose supported effort set differs (e.g., gemini-4-carbon
	// tops out at high while gemini-4-neon supports xhigh).
	DefaultEffort string
	Efforts       []string
	ModelEfforts  map[string][]string
	Command       []string
}

var builtinListOrder = []string{"claude", "codex", "agy"}

var toolModels = map[string][]string{
	"claude": {"claude-opus-5-5", "claude-sonnet-5-5", "claude-fable-5-2"},
	"codex":  {"gpt-5.6-sol", "gpt-6-astra"},
	"agy":    {"gemini-4-carbon", "gemini-4-neon", "gemini-4-argon", "gemini-3.8-flash"},
}

var toolEfforts = map[string][]string{
	"claude": {"low", "medium", "high", "xhigh", "max"},
	"codex":  {"low", "medium", "high", "xhigh"},
	"agy":    {"low", "medium", "high", "xhigh"},
}

var toolModelEfforts = map[string]map[string][]string{
	"agy": {
		"gemini-4-neon":    {"low", "medium", "high", "xhigh"},
		"gemini-4-carbon":  {"low", "medium", "high"},
		"gemini-4-argon":   {"low", "medium", "high"},
		"gemini-3.8-flash": {"low", "medium", "high"},
	},
}

// EffortsForModel returns the valid reasoning efforts for toolID when running
// model ("" uses the built-in default model), or nil for tools with no effort
// setting.
func EffortsForModel(toolID, model string) []string {
	if model == "" {
		if b, ok := builtinEngines()[toolID]; ok {
			model = b.model
		}
	}
	if perModel, ok := toolModelEfforts[toolID]; ok {
		if me, ok := perModel[model]; ok && len(me) > 0 {
			return slices.Clone(me)
		}
	}
	if eff, ok := toolEfforts[toolID]; ok && len(eff) > 0 {
		return slices.Clone(eff)
	}
	return nil
}

// Tools lists Claude, Codex and Gemini (agy), each marked found when its program
// is on the login PATH, then every user engine in name order. env nil means helper.LoginEnv().
func Tools(engines map[string][]string, env []string) []Tool {
	if env == nil {
		env = helper.LoginEnv()
	}
	builtins := builtinEngines()
	tools := make([]Tool, 0, len(builtinListOrder)+len(engines))
	for _, name := range builtinListOrder {
		e := builtins[name]
		program, _ := engineProgram(e)
		_, err := lookPath(program, env)
		var modelEfforts map[string][]string
		if perModel := toolModelEfforts[name]; len(perModel) > 0 {
			modelEfforts = make(map[string][]string, len(perModel))
			for m, effs := range perModel {
				modelEfforts[m] = slices.Clone(effs)
			}
		}
		tools = append(tools, Tool{
			ID:            name,
			Found:         err == nil,
			DefaultModel:  e.model,
			Models:        append([]string(nil), toolModels[name]...),
			DefaultEffort: e.effort,
			Efforts:       append([]string{}, toolEfforts[name]...),
			ModelEfforts:  modelEfforts,
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
		tools = append(tools, Tool{ID: name, Custom: true, Found: true, Command: append([]string(nil), resolved[name].command...), Models: []string{}, Efforts: []string{}})
	}
	return tools
}

// FirstFound is the tool AI editing uses when none is chosen: the first of
// Claude and Codex installed, else Claude, whose refusal then says to install it.
// A nil env reads the login PATH.
func FirstFound(env []string) string {
	for _, t := range Tools(nil, env) {
		if t.Found {
			return t.ID
		}
	}
	return "claude"
}

// ErrCommand refuses a custom command line that is empty or has an unclosed quote.
var ErrCommand = errors.New("the command is empty or has an unclosed quote")

// SplitCommand splits a command line into its arguments the way a shell would
// for plain words, 'single' and "double" quotes, and backslash escapes outside
// single quotes. On Windows a backslash separates folders in a path, so there
// it is an ordinary character. It runs nothing and expands nothing.
func SplitCommand(line string) ([]string, error) {
	return splitCommand(line, runtime.GOOS != "windows")
}

func splitCommand(line string, escapes bool) ([]string, error) {
	var args []string
	var cur strings.Builder
	inWord, quote, escaped := false, rune(0), false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case escapes && r == '\\' && quote != '\'':
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

// JoinCommand writes argv back as one line that SplitCommand reads as the same
// arguments, so a saved command can be shown and saved again unchanged.
func JoinCommand(argv []string) string {
	return joinCommand(argv, runtime.GOOS != "windows")
}

func joinCommand(argv []string, escapes bool) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		special := " \t\n\"'"
		if escapes {
			special += "\\"
		}
		switch {
		case a != "" && !strings.ContainsAny(a, special):
			out[i] = a
		case !strings.Contains(a, "'"):
			out[i] = "'" + a + "'"
		case escapes:
			out[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a) + `"`
		default:
			out[i] = `"` + a + `"`
		}
	}
	return strings.Join(out, " ")
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
