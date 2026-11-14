package wire

import (
	"net/http"
	"sync"
	"time"

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
	AIEditEnabled() bool
	AIEditEngines() (string, map[string][]string)
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

// Shutdown cancels every AI edit and closes every stream. A served root calls
// it when the root goes away; the loose Server keeps its own two calls.
func (s *Core) Shutdown() {
	s.CancelAIEdits()
	s.Hub.Shutdown()
}
