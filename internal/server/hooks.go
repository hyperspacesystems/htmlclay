package server

import (
	"context"

	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/confirm"
)

// Hooks are the decisions a site's server cannot make for itself. Each one
// needs the declared trusted-folder list, the app's site registry, or a native
// dialog, and the server imports none of those.
//
// Wiring them as one struct at construction makes the whole app-to-server
// contract a single declaration that can be read from either end. As five
// separate setters mutating a live server, it could only be reconstructed by
// grepping, and two of them had to take the broker's lock because they could be
// called at any moment.
//
// A nil field disables that route, which is what a standalone server (every
// server test) wants.
type Hooks struct {
	// Confirm presents one read question and waits for the person's answer, so
	// only the presenter differs between a window and a dialog. Nil uses the real
	// native dialog.
	//
	// The Ask's answers say whether the durable choice may be offered: the app
	// decides that from its refusal rules BEFORE the question is shown, so a
	// folder that could never be trusted is never offered. The button used to be
	// drawn always and the refusal arrived afterwards as a notification, which
	// asked the user for a choice that could not be honored. ctx is cancelled
	// when every request waiting on the answer has stopped waiting.
	Confirm func(ctx context.Context, ask confirm.Ask) (confirm.Reply, error)

	// TrustedCovers reports whether absPath sits inside a DECLARED trusted
	// folder. It is the config fact, not this site's runtime roots: a file can
	// belong to a trusted folder this site holds no root for, and answering from
	// the roots is exactly what made a folder declared below an already-open
	// site grant nothing at all. It must not touch the filesystem; see
	// shouldAutoRegister for why the ordering there is load-bearing.
	TrustedCovers func(absPath string) bool

	// TrustedLive is the same question with the identity pin applied: is absPath
	// inside a trusted folder that is still the folder that was trusted? A folder
	// deleted and recreated still covers lexically but is no longer live, and the
	// two must not be confused. TrustedCovers is the gate on whether to TRY
	// auto-registering, and answers from memory; this one is the standing
	// capability, and stats. Only handleTrustRequest asks it, on the file its own
	// token was minted for, so the stat reveals nothing a caller did not know.
	TrustedLive func(absPath string) bool

	// Route registers absPath through the app and reports where it serves, so
	// this site can redirect when the registration landed on another origin.
	// Trusted-folder auto-registration is its only caller, and the app refuses
	// anything that does not anchor at a live trusted folder.
	Route func(absPath string) (url string, ok bool)

	// MayTrustFolder gates Confirm's trust-folder answer for a candidate folder.
	MayTrustFolder func(dir string) bool

	// MainFolder reports whether a candidate folder is one of the main personal
	// folders. Nil leaves the question without that detail.
	MainFolder func(dir string) bool

	// TrustFolder is the durable half of the read prompt's "Trust this folder"
	// choice. The folder there is one the PAGE steered, by choosing which
	// out-of-scope assets to request, so the app applies its stricter refusal
	// rule behind this.
	TrustFolder func(dir string) error

	// TrustRequest is the one door for "trust this file's folder". Both routes
	// into it name a FILE the server already vouches for, never a folder: the
	// banner's nonce resolves to the descriptor that was served, and the JSON
	// route's token to a registration. openedByUser is false when the file was
	// reached by a link, and the dialog says so on its first line.
	TrustRequest func(requestingFile string, openedByUser bool) (url string, ok bool)

	// AIEditFor answers whether document may use AI editing now (on) and, when
	// not, whether to ask the person (ask). Nil means this server offers no AI
	// editing at all: no discovery entry and no built-in answer, which is what a
	// standalone server (every server test) wants.
	AIEditFor func(document string) (on, ask bool)
	// AskAIEdit raises the AI editing question for document. Nil asks nothing.
	AskAIEdit func(document string)

	// AIEditEngines returns the default engine name ("" for claude) and the
	// user-defined engines. Nil means the built-ins only.
	AIEditEngines func() (string, map[string][]string)

	// AIEditModels returns the model each AI engine runs with. Nil means defaults.
	AIEditModels func() map[string]string
	// BuiltinFor answers whether document may run a built-in program now. Nil
	// means every built-in program is off.
	BuiltinFor func(document, program string) builtin.Access
	// AskBuiltin raises the built-in program question for document. Nil asks nothing.
	AskBuiltin func(document, program string)

	// Profile reports the person documents may see (spec §9 People). Nil means this
	// server offers no People at all: no extension and no block. ok false means the
	// person turned sharing off, and documents get `me: null`. Read on every
	// document-scoped discovery, so a change in the app reaches open files.
	Profile func() (id, name string, ok bool)

	// AskHelper raises the document-programs dialog for one declared helper the
	// page just tried to use and the person has not answered. It reports whether
	// the helper is now allowed, after the app has reattached the document's
	// helpers. Nil refuses a request for an unanswered helper without asking,
	// which is what a standalone server (every server test) wants.
	AskHelper func(file, name string) bool

	// HelperGranted reports whether name may run the registered program programID
	// for file at this moment. The server asks it immediately before a program's
	// process starts, because the grant a dispatcher holds was resolved earlier:
	// a folder that has since become a team folder, a scope that can no longer be
	// read, or an access the person removed must stop the process. Nil allows,
	// which is what a standalone server (every server test) wants.
	HelperGranted func(file, name, programID string) bool

	// Reveal shows a document's file in the OS file manager, selected. Nil hides
	// the action, which is what a standalone server (every server test) wants.
	Reveal func(absPath string) error

	// OpenFolder opens a folder in the OS file manager. Nil hides the versions
	// action.
	OpenFolder func(dir string) error

	// ExportDocument zips a document with its uploads into Downloads and shows it.
	// Nil hides the action.
	ExportDocument func(absPath string) error
}

// SetHooks wires the app behind this server. Call once, before Start.
func (s *Server) SetHooks(h Hooks) {
	s.hooks = h
	if h.Confirm != nil {
		s.broker.confirm = h.Confirm
	}
	s.broker.mayTrust = h.MayTrustFolder
	s.broker.mainFolder = h.MainFolder
	s.broker.trust = h.TrustFolder
}
