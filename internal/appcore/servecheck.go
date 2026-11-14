// Package appcore holds HTML Clay's decisions as pure functions over facts the app
// gathers, so every door (tray, command line, dashboard, Mac app) asks the same rules.
package appcore

import (
	"errors"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/pathrel"
)

// Refusal is why a folder cannot be served: a code and, when another folder is the
// reason, that folder's path (and its served name, if it has one).
type Refusal struct {
	Code      string
	OtherPath string
	OtherName string
	// Field names the argument at fault, for a form to mark ("name", "path").
	Field string
	// SubjectID is the folder, program or request the refusal is about.
	SubjectID string
	// Documents names the documents a refusal is about, as "<served name>/<file>"
	// (quit-unsaved: the edits the live-room service could not save).
	Documents []string
	// Text is the sentence to show instead of the one copy.go gives Code, for a
	// refusal made from a folder rule's reason.
	Text string
}

const (
	RefuseNotAFolder       = "not-a-folder"
	RefuseCannotTrust      = "cannot-trust"
	RefuseOutsideHome      = "outside-home"
	RefuseIsHome           = "is-home"
	RefuseProtectedFolder  = "protected-folder"
	RefuseOverlapsServed   = "overlaps-served"
	RefuseNestedTrusted    = "nested-trusted"
	RefuseServedByLocal    = "served-by-local"
	RefuseBoundToLocalSync = "bound-to-local-sync"
	RefuseNameInvalid      = "name-invalid"
	RefuseNameTaken        = "name-taken"
	RefuseNameRetired      = "name-retired"
	// RefuseServedElsewhere is another HTML Clay process (another profile or build)
	// holding the folder's lock. CheckServe does not return it: the lock is taken
	// after the check. It is listed here so every door knows the code.
	RefuseServedElsewhere = "served-elsewhere"

	// Action refusals (Plan).
	RefuseUnknownAction   = "unknown-action"
	RefuseBadArguments    = "bad-arguments"
	RefuseNoSuchFolder    = "no-such-folder"
	RefuseNoSuchProgram   = "no-such-program"
	RefuseNoSuchDocument  = "no-such-document"
	RefuseFileMissing     = "file-missing"
	RefuseFileOutsideHome = "file-outside-home"
	// RefuseDestinationExists is a recovery destination where a file already is;
	// that file is never touched.
	RefuseDestinationExists = "destination-exists"
	RefuseNoSuchRequest     = "no-such-request"
	RefuseRequestChanged    = "request-changed"
	RefuseAnswerNotOffered  = "answer-not-offered"
	RefuseNotAllowedNow     = "not-allowed-now"
	RefuseUnknownPreference = "unknown-preference"
	RefuseNothingToMove     = "nothing-to-move"
	RefuseNotConnected      = "not-connected"
	RefuseNothingReady      = "nothing-ready"
	// RefuseUnsavedEdits is a stop serving the live-room service kept, because
	// the folder still has edits that are not saved.
	RefuseUnsavedEdits = "unsaved-edits"
	// RefuseQuitUnsaved is a quit the host held back, because the live-room
	// service could not save every edit. A forced quit goes ahead.
	RefuseQuitUnsaved = "quit-unsaved"

	// RefuseNeedsConfirmation is a grant asked for on the dashboard origin, which
	// only the host's confirmer may approve.
	RefuseNeedsConfirmation = "needs-confirmation"
	// RefuseFailed is an effect the app tried to run and could not; the cause goes
	// to the log, never to the sentence.
	RefuseFailed = "failed"
	// RefuseStillRunning is an action that is already running; the answer comes
	// when its effects finish.
	RefuseStillRunning = "still-running"
	// RefuseNotYet is an action whose effect the app has no code for yet.
	RefuseNotYet = "not-yet"
)

// OtherFolder is a folder that matters to the check: how the asked-for folder stands
// to it, its path, and its served name if it is served.
type OtherFolder struct {
	Relation pathrel.Relation
	Path     string
	Name     string
}

// ServeFacts is everything CheckServe needs, gathered by GatherServeFacts.
type ServeFacts struct {
	Path          string // the folder asked for, symlinks resolved
	Exists        bool
	IsDir         bool
	Home          pathrel.Relation // how Path stands to the home folder
	Protected     []OtherFolder    // the config folder, the uploads library
	Served        []OtherFolder    // folders HTML Clay already serves
	Trusted       []OtherFolder    // trusted folders that are not served
	LocalServed   []OtherFolder    // folders Hyperclay Local serves now
	HasSyncMarker bool             // Path or a folder above it holds .hyperclay/sync-root.json
	NameErr       error            // config.ServedNameAvailable(name)
}

// CheckServe answers nil when the folder may be served, else the first rule it breaks,
// in the order below.
func CheckServe(f ServeFacts) *Refusal {
	if !f.Exists || !f.IsDir {
		return &Refusal{Code: RefuseNotAFolder, Field: "path"}
	}
	if f.Home == pathrel.Same {
		return &Refusal{Code: RefuseIsHome, Field: "path"}
	}
	if f.Home != pathrel.Inside {
		return &Refusal{Code: RefuseOutsideHome, Field: "path"}
	}
	if other, ok := touching(f.Protected); ok {
		return &Refusal{Code: RefuseProtectedFolder, OtherPath: other.Path, OtherName: other.Name, Field: "path"}
	}
	if other, ok := touching(f.Served); ok {
		return &Refusal{Code: RefuseOverlapsServed, OtherPath: other.Path, OtherName: other.Name, Field: "path"}
	}
	if other, ok := touching(f.LocalServed); ok {
		return &Refusal{Code: RefuseServedByLocal, OtherPath: other.Path, OtherName: other.Name, Field: "path"}
	}
	if f.HasSyncMarker {
		return &Refusal{Code: RefuseBoundToLocalSync, Field: "path"}
	}
	for _, other := range f.Trusted {
		if other.Relation == pathrel.Contains || other.Relation == pathrel.Inside {
			return &Refusal{Code: RefuseNestedTrusted, OtherPath: other.Path, OtherName: other.Name, Field: "path"}
		}
	}
	return NameRefusal(f.NameErr)
}

// NameRefusal answers the served-name rules alone: nil when the name may be given
// out now, else the refusal. A folder named by id is already ruled on by Plan, so
// the name is the whole answer a sheet still needs.
func NameRefusal(err error) *Refusal {
	switch {
	case errors.Is(err, config.ErrServedNameInvalid):
		return &Refusal{Code: RefuseNameInvalid, Field: "name"}
	case errors.Is(err, config.ErrServedNameTaken):
		return &Refusal{Code: RefuseNameTaken, Field: "name"}
	case errors.Is(err, config.ErrServedNameRetired):
		return &Refusal{Code: RefuseNameRetired, Field: "name"}
	case err != nil:
		return &Refusal{Code: RefuseNameInvalid, Field: "name"}
	}
	return nil
}

// touching returns the first folder that is, lies inside, or contains the asked-for
// folder.
func touching(folders []OtherFolder) (OtherFolder, bool) {
	for _, other := range folders {
		if other.Relation != pathrel.Unrelated {
			return other, true
		}
	}
	return OtherFolder{}, false
}
