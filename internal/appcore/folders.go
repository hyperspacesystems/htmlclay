package appcore

import (
	"fmt"

	"github.com/panphora/htmlclay/internal/pathrel"
)

// Why codes, stable for programs; the sentences are in copy.go.
const (
	WhyMissing          = "missing"
	WhyIdentityChanged  = "identity-changed"
	WhyServedByLocal    = "served-by-local"
	WhyBoundToLocalSync = "bound-to-local-sync"
	WhyNotOnComputer    = "not-on-this-computer"
	WhyNotFromAccount   = "not-from-account"
	WhyConnectFirst     = "connect-first"
	WhyServeFirst       = "serve-first"
	WhyViewer           = "viewer"
	WhyStopServingFirst = "stop-serving-first"
	WhyOverlapsServed   = "overlaps-served"
	WhyNotOwnAddress    = "not-own-address"
	WhyNotLocal         = "not-local"
)

// folderRules is the whole rule set of a folder's levels (trusted, served, synced),
// the preview's SampleReducer rules (macos/Sources/PreviewCore/SampleHost.swift):
// serving needs the folder present and owned by nobody else; syncing needs it served,
// from the account, with an account connected, and the person able to edit; a served
// folder must stop serving before it can stop being trusted; a folder inside another
// trusted folder can merge into it. Serve's deeper checks (home, nesting, names) run
// when the action is planned, against the real filesystem.
func folderRules(f Facts, ff FolderFact, state string) (FolderCan, FolderWhy) {
	var can FolderCan
	var why FolderWhy

	if !ff.Served {
		switch state {
		case FolderOK, FolderViewer:
			if o, ok := servedAround(f.Folders, ff); ok {
				why.Serve = reason(WhyOverlapsServed, fmt.Sprintf(TextWhyServeOverlaps, servedDisplayName(o)))
			} else {
				can.Serve = true
			}
		case FolderNotOnThisComputer:
			why.Serve = reason(WhyNotOnComputer, TextWhyNotOnComputer)
		case FolderMissing:
			why.Serve = reason(WhyMissing, TextWhyServeMissing)
		case FolderIdentityChanged:
			why.Serve = reason(WhyIdentityChanged, TextWhyServeIdentityChanged)
		case FolderServedByLocal:
			why.Serve = reason(WhyServedByLocal, TextWhyServeServedByLocal)
		case FolderBoundToLocalSync:
			why.Serve = reason(WhyBoundToLocalSync, TextWhyServeBoundToLocalSync)
		}
	}
	can.StopServing = ff.Served
	can.SetFraming = ff.Served && ff.OnThisComputer && !ff.Team && ff.Origin.Kind != OriginTeam

	if ff.Synced {
		can.StopSyncing = true
	} else {
		switch {
		case ff.Origin.Kind == OriginLocal:
			why.Sync = reason(WhyNotFromAccount, TextWhySyncNotFromAccount)
		case state == FolderNotOnThisComputer:
			why.Sync = reason(WhyNotOnComputer, TextWhyNotOnComputer)
		case !f.Sync.Connected:
			why.Sync = reason(WhyConnectFirst, TextWhySyncConnectFirst)
		case state == FolderMissing:
			why.Sync = reason(WhyMissing, TextWhySyncMissing)
		case ff.Viewer:
			why.Sync = reason(WhyViewer, TextWhySyncViewer)
		case !ff.Served:
			why.Sync = reason(WhyServeFirst, TextWhySyncServeFirst)
		default:
			can.Sync = true
		}
	}

	if ff.Trusted {
		can.StopTrusting = true
	}

	can.Merge, why.Merge = mergeRule(f, ff)
	can.OpenWorkspace = ff.Served && state == FolderOK
	return can, why
}

// mergeRule answers whether ff may merge into the trusted folder around it. Only
// a folder that already answers at an address of its own can give that address
// up, only a folder on this computer that belongs to no team can take it, and a
// served folder has to stop serving first.
func mergeRule(f Facts, ff FolderFact) (bool, *Reason) {
	parent, ok := folderFact(f.Folders, ff.ParentID)
	if ff.ParentID == "" || !ok {
		return false, nil
	}
	if !localScope(ff) || !localScope(parent) {
		return false, reason(WhyNotLocal, TextWhyMergeNotLocal)
	}
	if ff.Served {
		return false, reason(WhyStopServingFirst, TextWhyStopTrustingServed)
	}
	if !ff.OwnAddress {
		return false, reason(WhyNotOwnAddress, TextWhyMergeNotOwnAddress)
	}
	return true, nil
}

// localScope reports whether a folder belongs to this computer alone: not part of
// a team and not from the account.
func localScope(ff FolderFact) bool {
	return ff.Origin.Kind == OriginLocal && !ff.Team && !ff.Synced
}

// servedAround finds a served folder, other than ff itself, whose path is ff's or
// lies below or above it: the folders CheckServe refuses as overlapping.
func servedAround(folders []FolderFact, ff FolderFact) (FolderFact, bool) {
	for _, o := range folders {
		if !o.Served || o.ID == ff.ID || o.Path == "" || ff.Path == "" {
			continue
		}
		if pathrel.SameOrBelow(ff.Path, o.Path) || pathrel.SameOrBelow(o.Path, ff.Path) {
			return o, true
		}
	}
	return FolderFact{}, false
}

// servedDisplayName is the name a served folder goes by: its served name when it
// has one, else its own name.
func servedDisplayName(ff FolderFact) string {
	if ff.ServedName != "" {
		return ff.ServedName
	}
	return ff.Name
}

func reason(code, text string) *Reason { return &Reason{Code: code, Text: text} }

func hasFolder(f Facts, id string) bool {
	for _, other := range f.Folders {
		if other.ID == id {
			return true
		}
	}
	return false
}
