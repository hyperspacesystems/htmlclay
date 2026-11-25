package wire

import (
	"net/http"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/logging"
)

type Document struct {
	Path string
	Name string
	Ref  any
}

type Documents interface {
	Caller(r *http.Request) (isBrowser, ok bool)
	Target(r *http.Request, isBrowser bool, supplied string) (Document, bool)
	At(path string) (Document, bool)
	MayUsePrograms(doc Document, isBrowser bool, presented string) bool
	OpenHistory(doc Document) error
	Lease(doc Document)
	Unlease(doc Document)
	Poke(doc Document)
	Internal(absPath string) bool
}

type Policy interface {
	CanAskProgram() bool
	AskProgram(file, name string) bool
	ProgramGranted(file, name, programID string) bool
	AIEditOffered() bool
	// AIEditFor answers whether document may use AI editing now (on), and when
	// it may not, whether the person should be asked (ask).
	AIEditFor(document string) (on, ask bool)
	// AskAIEdit raises the question for document. It answers at once; the
	// person decides later.
	AskAIEdit(document string)
	// AIEditFiles answers whether document's AI requests may attach its sibling
	// files with @file. Only a document whose code the person or a signed
	// release vouches for may.
	AIEditFiles(document string) bool
	AIEditEngines() (string, map[string][]string)
	// AIEditModels is the model each AI engine runs with, by engine name.
	AIEditModels() map[string]string
	// AIEditEfforts is the reasoning effort each AI engine runs with, by engine name.
	AIEditEfforts() map[string]string
	// BuiltinFor answers whether document may run a built-in program now, and
	// the folder the program works in.
	BuiltinFor(document, program string) builtin.Access
	// AskBuiltin raises the question for document and program. It answers at
	// once; the person decides later.
	AskBuiltin(document, program string)
	// RevealFile shows a file in the file manager (file-search's reveal).
	RevealFile(path string) error
	// DocumentTrust is what the host knows of document's trust.
	DocumentTrust(document string) Trust
	// AskDocument raises the one question document's code needs, for program.
	// It answers at once; the person decides later.
	AskDocument(document, program string)
	// AITools is the ai-tools host, or nil where AI with tools is not offered.
	AITools() AIToolsHost
	// Programs is the carried programs host, or nil where programs do not run.
	Programs() ProgramsHost
	// NoteAIEditResult hears an AI edit that finished on document, so a save
	// after Keep can trust exactly the code that edit produces.
	NoteAIEditResult(document, elementHTML, tag, resultHTML string)
	// NoteDocumentSaved hears the bytes a page saved into document.
	NoteDocumentSaved(document string, data []byte)
}

type Core struct {
	docs       Documents
	policy     Policy
	Logger     *logging.Logger
	writeError func(w http.ResponseWriter, status int, message string)

	Hub            *Hub
	HelperBindings helperBindingRegistry
	HelperMu       sync.Mutex
	Helpers        map[string]*Dispatcher
	HelperOffline  map[string][]string
	HelperAsking   map[string][]parkedHelperRequest
	helperSetting  map[string]bool
	helperAskQuiet map[string]time.Time
	AIEdit         aiEditRegistry
	AITools        aiToolsRegistry
	Programs       aiToolsRegistry
	Builtin        builtinRegistry
	RunFiles       RunFiles
}

func NewCore(docs Documents, policy Policy, logger *logging.Logger, writeError func(http.ResponseWriter, int, string)) *Core {
	return &Core{
		docs:       docs,
		policy:     policy,
		Logger:     logger,
		writeError: writeError,
		Hub:        NewHub(),
	}
}

// Shutdown cancels every AI run and closes every stream. A served root calls
// it when the root goes away; the loose Server keeps its own two calls.
func (s *Core) Shutdown() {
	s.CancelAI()
	s.Hub.Shutdown()
	s.RunFiles.Clear()
}
