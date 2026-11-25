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
	// Running is what a restart would stop (restart-stops-work): "work" (an AI
	// edit or a document's program) and "sync".
	Running []string
	// Text is the sentence to show instead of the one copy.go gives Code, for a
	// refusal made from a folder rule's reason.
	Text string
	// Counts is what syncing into a folder that has files would do, for
	// confirm-needed: the person sees it and sends the action again confirmed.
	Counts *SyncCounts
}

// SyncCounts is a first sync of an existing folder: files going up, files coming
// down, and files on both sides that differ, which are kept both ways.
type SyncCounts struct {
	ToUpload   int `json:"toUpload"`
	ToDownload int `json:"toDownload"`
	Differ     int `json:"differ"`
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
	// RefuseNothingMoved is a move where every folder stayed with Hyperclay Local.
	RefuseNothingMoved = "nothing-moved"
	RefuseLocalRunning = "local-running"
	RefuseNotConnected = "not-connected"
	// The sync key refusals Connect answers (08-sync.md step 8.10).
	RefuseSyncKeyShape     = "sync-key-shape"
	RefuseSyncKeyRefused   = "sync-key-refused"
	RefuseSyncWrongAccount = "sync-wrong-account"
	RefuseSyncOffline      = "sync-offline"
	RefuseSyncServerUpdate = "sync-server-update"
	// RefuseSyncFolderNotEmpty is a team's first sync into a folder that has files.
	RefuseSyncFolderNotEmpty = "sync-folder-not-empty"
	// RefuseSyncFolderInUse is a set-up into a folder HTML Clay already has
	// under its own id.
	RefuseSyncFolderInUse = "sync-folder-in-use"
	// RefuseConfirmNeeded is syncing into a folder that has files, before the
	// person has seen what it will do (its Counts).
	RefuseConfirmNeeded = "confirm-needed"
	// RefuseConfirmSyncStop is stopping trust on a synced folder without the
	// person confirming that syncing and local serving will stop.
	RefuseConfirmSyncStop = "confirm-sync-stop"
	// RefuseServiceNeeded is a sync action with no live-room service running:
	// sync runs inside it, and it runs while a folder is served.
	RefuseServiceNeeded = "service-needed"
	// RefuseNotText is a conflict choice that would send a page not saved as UTF-8.
	RefuseNotText = "not-text"
	// RefuseSyncDownloading is Find it on a folder whose first sync download runs.
	RefuseSyncDownloading = "sync-downloading"
	// RefuseSyncPaused is a conflict choice while sync is paused: the choice
	// uploads or downloads, which pause promises not to do.
	RefuseSyncPaused = "sync-paused"
	// RefuseSyncKeyStopped is a conflict or removal choice while the sync key is
	// refused or unreadable: no session is running to take it.
	RefuseSyncKeyStopped = "sync-key-stopped"
	// RefuseConflictGone is a choice for a conflict that is no longer held.
	RefuseConflictGone = "conflict-gone"
	// RefuseConflictChanged is a choice for a conflict whose file or server
	// copy changed since the person looked at it; nothing was changed.
	RefuseConflictChanged = "conflict-changed"
	// RefuseRemovalGone is a choice for a held removal that is no longer held.
	RefuseRemovalGone  = "removal-gone"
	RefuseNothingReady = "nothing-ready"
	// RefuseUnsavedEdits is a stop serving the live-room service kept, because
	// the folder still has edits that are not saved.
	RefuseUnsavedEdits = "unsaved-edits"
	// RefuseQuitUnsaved is a quit the host held back, because the live-room
	// service could not save every edit. A forced quit goes ahead.
	RefuseQuitUnsaved = "quit-unsaved"

	// RefuseNeedsConfirmation is a grant asked for on the dashboard origin, which
	// only the host's confirmer may approve.
	RefuseNeedsConfirmation = "needs-confirmation"
	// RefusePending is a grant asked for on the dashboard that a system dialog
	// is now asking the person about. Nothing has changed yet. When the person
	// says yes the change arrives on the snapshot stream.
	RefusePending = "pending"
	// RefuseDialogOpen is a grant asked for while another grant's dialog is open.
	RefuseDialogOpen = "dialog-open"
	// RefuseNoDialogs is a grant on a system with no program to show its dialog.
	RefuseNoDialogs = "no-dialogs"
	// RefuseFailed is an effect the app tried to run and could not; the cause goes
	// to the log, never to the sentence.
	RefuseFailed = "failed"
	// RefuseStillRunning is an action that is already running; the answer comes
	// when its effects finish.
	RefuseStillRunning = "still-running"
	// RefuseRestartStopsWork is a restart that would stop running work; the
	// person may send the action again confirmed.
	RefuseRestartStopsWork = "restart-stops-work"
	// RefuseFinishInSettings is Make default on Windows opening Windows
	// Settings: another app still holds .htmlclay, and only the person can
	// choose HTML Clay there.
	RefuseFinishInSettings = "finish-in-settings"
	// RefuseNotYet is an action whose effect the app has no code for yet.
	RefuseNotYet = "not-yet"

	// RefusePortInvalid is a port outside 1024 to 65535.
	RefusePortInvalid = "port-invalid"
	// RefusePortSame is moving to the port the app already uses.
	RefusePortSame = "port-same"
	// RefusePortTaken is a port another program holds.
	RefusePortTaken = "port-taken"
	// RefusePortUnsaved is a port change held back by unsaved edits (Documents).
	RefusePortUnsaved = "port-unsaved"
	// RefuseNoSuchTool is an AI tool the picker does not list.
	RefuseNoSuchTool = "no-such-tool"
	// RefuseCommandInvalid is a custom command with an unclosed quote or nothing in it.
	RefuseCommandInvalid = "command-invalid"
	// RefuseCommandNotFound is a custom command whose program is not on this computer.
	RefuseCommandNotFound = "command-not-found"
	// RefuseNotOnThisSystem is an action this system does in another place: the
	// Mac app runs file types and Sparkle itself.
	RefuseNotOnThisSystem = "not-on-this-system"
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
	Path        string // the folder asked for, symlinks resolved
	Exists      bool
	IsDir       bool
	Home        pathrel.Relation // how Path stands to the home folder
	Protected   []OtherFolder    // the config folder, the uploads library
	Served      []OtherFolder    // folders HTML Clay already serves
	Trusted     []OtherFolder    // trusted folders that are not served
	LocalServed []OtherFolder    // folders Hyperclay Local serves now
	// SyncMarkers are the .hyperclay/sync-root.json files in Path, above it up to
	// home, and up to three levels below it. OwnSyncRoots are the rootIds of the
	// folders this app syncs.
	SyncMarkers  []SyncMarker
	OwnSyncRoots []string
	NameErr      error // config.ServedNameAvailable(name)
}

// SyncMarker is one sync marker: the folder that holds it and the rootId it
// names, "" when that cannot be read.
type SyncMarker struct {
	Folder string
	RootID string
}

// boundToLocalSync reports a marker that is not this app's own: a folder
// Hyperclay Local syncs. A marker whose rootId cannot be read is not this app's.
func boundToLocalSync(f ServeFacts) bool {
	for _, marker := range f.SyncMarkers {
		own := false
		for _, id := range f.OwnSyncRoots {
			if marker.RootID != "" && marker.RootID == id {
				own = true
				break
			}
		}
		if !own {
			return true
		}
	}
	return false
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
	if boundToLocalSync(f) {
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
