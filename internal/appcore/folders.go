package appcore

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
			can.Serve = true
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
		if ff.Served {
			why.StopTrusting = reason(WhyStopServingFirst, TextWhyStopTrustingServed)
		} else {
			can.StopTrusting = true
		}
	}

	can.Merge = ff.ParentID != "" && hasFolder(f, ff.ParentID) && !ff.Served && !ff.Synced
	can.OpenWorkspace = ff.Served && state == FolderOK
	return can, why
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
