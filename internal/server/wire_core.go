package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/logging"
)

type wireDocument struct {
	Path string
	Name string
	Ref  any
}

type wireDocuments interface {
	caller(r *http.Request) (isBrowser, ok bool)
	target(r *http.Request, isBrowser bool, supplied string) (wireDocument, bool)
	at(path string) (wireDocument, bool)
	mayUsePrograms(doc wireDocument, isBrowser bool, presented string) bool
	openHistory(doc wireDocument) error
	lease(doc wireDocument)
	unlease(doc wireDocument)
	poke(doc wireDocument)
	internal(absPath string) bool
}

type wirePolicy interface {
	canAskProgram() bool
	askProgram(file, name string) bool
	programGranted(file, name, programID string) bool
	aiEditOffered() bool
	aiEditEnabled() bool
	aiEditEngines() (string, map[string][]string)
}

type wireCore struct {
	docs       wireDocuments
	policy     wirePolicy
	logger     *logging.Logger
	writeError func(w http.ResponseWriter, status int, message string)

	wire           *wireHub
	helperBindings helperBindingRegistry
	helperMu       sync.Mutex
	helpers        map[string]*helperDispatcher
	helperOffline  map[string][]string
	helperAsking   map[string][]parkedHelperRequest
	helperSetting  map[string]bool
	helperAskQuiet map[string]time.Time
	aiEdit         aiEditRegistry
}

func newWireCore(docs wireDocuments, policy wirePolicy, logger *logging.Logger, writeError func(http.ResponseWriter, int, string)) *wireCore {
	return &wireCore{
		docs:       docs,
		policy:     policy,
		logger:     logger,
		writeError: writeError,
		wire:       newWireHub(),
	}
}
