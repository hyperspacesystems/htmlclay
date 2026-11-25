// Package aitools runs the person's own agent CLI (Claude Code, Codex or agy)
// on a folder they chose, with read tools only, and turns its answer into data
// a page asked for plus sources the host expands itself.
//
// Nothing here confines the agent with an OS sandbox. Each agent is held to the
// folder by its own rules: Claude Code runs --restricted with Read, Grep and
// Glob; Codex runs its own sandbox with a folder-read permission profile; agy
// runs with a settings object that allows reading inside the folder and
// refuses commands and writes. A self-test per agent version checks those
// rules on this machine before the version's first real run, and an agent that
// leaks a canary is refused. The agent never runs git: the host puts the
// branch's commits and diff in the prompt, and expands the git ranges and file
// lines the agent names through internal/gitread and os.Root.
package aitools

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// HelperName is the wire helper a page asks for.
const HelperName = "ai-tools"

// Levels a page may ask for. Only read and change run; internet is refused.
const (
	LevelRead     = "read"
	LevelChange   = "change"
	LevelInternet = "internet"
)

// Folder answers a page may send. A page never sends a path.
const (
	FolderRemembered = "remembered"
	FolderChoose     = "choose"
)

const (
	maxRequestBytes      = 64 << 10
	maxInstructionsBytes = 128 << 10
	maxInstructions      = 32
	maxInstructionName   = 128
	maxSchemaBytes       = 64 << 10
)

// Error codes the page branches on (mock-contract.md, "Error codes").
const (
	CodeInvalidRequest    = "invalid_request"
	CodeLevelUnavailable  = "level_unavailable"
	CodeEngineUnsupported = "engine_unsupported"
	CodeEngineFailed      = "engine_failed"
	CodeFolderMissing     = "folder_missing"
	CodeResultInvalid     = "result_invalid"
	CodeSourceUnsupported = "source_unsupported"
	CodeSourceRefused     = "source_refused"
	CodeSourceTooLarge    = "source_too_large"
	CodeBusy              = "helper_busy"
)

// Error is a refusal written for the person, with the code the page branches on.
type Error struct {
	Code    string
	Message string
	// refused is an agent that stopped at a tool its rules refused, which the
	// self-test counts as holding.
	refused bool
}

func (e *Error) Error() string { return e.Message }

func fail(code, message string) *Error { return &Error{Code: code, Message: message} }

// Instruction is one hidden instruction the page resolved to text.
type Instruction struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// Request is the page's ai-tools payload (clayjs/src/plugins/ai.js requestPayload).
type Request struct {
	Request      string          `json:"request"`
	Instructions []Instruction   `json:"instructions"`
	Tools        string          `json:"tools"`
	Folder       string          `json:"folder"`
	Schema       json.RawMessage `json:"schema"`
	Sources      bool            `json:"sources"`
	Engine       string          `json:"engine"`
}

var engineName = regexp.MustCompile(`^[a-z0-9_-]{0,32}$`)

// Parse reads and checks a payload with the same limits ClayJS applies before
// it sends, because any page can post its own.
func Parse(payload []byte) (Request, *Error) {
	var r Request
	if err := json.Unmarshal(payload, &r); err != nil {
		return Request{}, fail(CodeInvalidRequest, "HTML Clay could not read this request.")
	}
	if strings.TrimSpace(r.Request) == "" {
		return Request{}, fail(CodeInvalidRequest, "Write a request first.")
	}
	if len(r.Request) > maxRequestBytes || !utf8.ValidString(r.Request) {
		return Request{}, fail(CodeInvalidRequest, "The request is longer than 64 KB.")
	}
	if len(r.Instructions) > maxInstructions {
		return Request{}, fail(CodeInvalidRequest, "A request can name at most 32 instructions.")
	}
	total := 0
	for _, in := range r.Instructions {
		if in.Name == "" || len(in.Name) > maxInstructionName || !utf8.ValidString(in.Name) || !utf8.ValidString(in.Text) {
			return Request{}, fail(CodeInvalidRequest, "An instruction has no usable name.")
		}
		total += len(in.Text)
	}
	if total > maxInstructionsBytes {
		return Request{}, fail(CodeInvalidRequest, "The instructions are longer than 128 KB in all.")
	}
	if r.Tools == "" {
		r.Tools = LevelRead
	}
	switch r.Tools {
	case LevelRead, LevelChange, LevelInternet:
	default:
		return Request{}, fail(CodeInvalidRequest, `tools must be "read", "change" or "internet".`)
	}
	if r.Folder == "" {
		r.Folder = FolderRemembered
	}
	if r.Folder != FolderRemembered && r.Folder != FolderChoose {
		return Request{}, fail(CodeInvalidRequest, `folder must be "remembered" or "choose".`)
	}
	if !engineName.MatchString(r.Engine) {
		return Request{}, fail(CodeInvalidRequest, "engine must be a short agent name.")
	}
	if len(r.Schema) == 0 || string(r.Schema) == "null" {
		r.Schema = json.RawMessage(`{"type":"object"}`)
	}
	if len(r.Schema) > maxSchemaBytes {
		return Request{}, fail(CodeInvalidRequest, "The answer shape is larger than 64 KB.")
	}
	var shape map[string]any
	if err := json.Unmarshal(r.Schema, &shape); err != nil {
		return Request{}, fail(CodeInvalidRequest, "The answer shape must be a JSON object.")
	}
	return r, nil
}
