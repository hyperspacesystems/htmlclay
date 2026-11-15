package appcore

import (
	"fmt"
	"strings"
)

// Text people read. Every value is working text until the copywriter replaces it;
// each copy: note says where it shows and how long it may be.

const (
	TextQuitUnsavedTitle   = "Some edits aren't saved yet"                                                       // copy: quit dialog title, max 40 chars
	TextQuitUnsavedBody    = "HTML Clay couldn't save these documents:\n\n%s\n\nQuitting now loses those edits." // copy: quit dialog body, %s is the documents one per line, max 200 chars without the list
	TextQuitRetry          = "Retry"                                                                             // copy: quit dialog button that tries to save again, max 16 chars
	TextQuitAnyway         = "Quit anyway"                                                                       // copy: quit dialog button that quits and loses the edits, max 16 chars
	TextQuitCancel         = "Cancel"                                                                            // copy: quit dialog button that keeps the app open, max 16 chars
	TextQuitNotQuitting    = "HTML Clay is still open because you have unsaved edits: %s. Try quitting again in a moment."
	TextQuitNoAnswer       = "Your open documents could not be checked."                                             // copy: quit dialog list line when the service could not be asked, max 60 chars
	TextServeAFolder       = "Serve a Folder…"                                                                       // copy: tray row that opens the folder picker, max 24 chars
	TextServedFolders      = "Served Folders"                                                                        // copy: tray submenu title, max 24 chars
	TextNoServedFolders    = "No served folders yet"                                                                 // copy: tray, disabled row when the list is empty, max 32 chars
	TextOpenServed         = "Open Workspace"                                                                        // copy: tray, per folder, max 24 chars
	TextCopyAddress        = "Copy Address"                                                                          // copy: tray, per folder, max 24 chars
	TextStopServing        = "Stop Serving"                                                                          // copy: tray, per folder, and the confirm button, max 24 chars
	TextFolderMissing      = "(missing)"                                                                             // copy: tray, after a served folder's name when its folder is gone, max 16 chars
	TextPickServeFolder    = "Choose a folder to serve at its own address."                                          // copy: folder picker prompt, max 120 chars
	TextStopServingTitle   = "Stop serving this folder?"                                                             // copy: confirm dialog title, max 40 chars
	TextStopServingMessage = "%s will stop working. The files in the folder stay where they are."                    // copy: confirm dialog body, %s is the address, max 160 chars
	TextNowServed          = "%s is served at %s"                                                                    // copy: notification and CLI success line, %s name then address, max 120 chars
	TextNotServed          = "No served folder is named %s."                                                         // copy: CLI, stop-serving with an unknown name, max 80 chars
	TextStopped            = "Stopped serving %s."                                                                   // copy: CLI success line, max 80 chars
	TextCannotServe        = "HTML Clay can't serve this folder"                                                     // copy: notification title for a refusal, max 48 chars
	TextServeCLIWindows    = "The htmlclay serve command runs on Mac and Linux. On Windows, use the HTML Clay menu." // copy: CLI, on Windows, max 120 chars
	// The htmlclay sync command line (08-sync.md step 8.12). TODO-COPY: every line below.
	TextSyncCLIWindows    = "The htmlclay sync command runs on Mac and Linux. On Windows, use the HTML Clay menu." // copy: CLI, on Windows, max 120 chars
	TextSyncKeyPrompt     = "Sync key: "                                                                           // copy: CLI prompt on a terminal, max 20 chars
	TextSyncKeyNoArgument = "Type the sync key when asked, or pipe it in. It is never a command argument."         // copy: CLI, key given as an argument, max 120 chars
	TextSyncConnectedAs   = "Connected as %s"                                                                      // copy: CLI success, %s username, max 80 chars
	TextSyncDisconnected  = "Disconnected. Your synced folders stay on this computer."                             // copy: CLI success, max 120 chars
	TextSyncPausedLine    = "Sync is paused"                                                                       // copy: CLI success and status line, max 80 chars
	TextSyncResumedLine   = "Sync is on"                                                                           // copy: CLI success and status line, max 80 chars
	TextSyncNotConnected  = "Not connected to a Hyperclay account"                                                 // copy: CLI status line, max 80 chars
	// The htmlclay sync setup lines (08-sync.md step 8.25).
	TextSyncSetupDownload   = "TODO-COPY %d files (%s) will download into %s."
	TextSyncSetupExisting   = "TODO-COPY %d files go up, %d come down, and %d differ and are kept both ways. Nothing is deleted."
	TextSyncSetupAsk        = "TODO-COPY Set up sync? [y/N] "
	TextSyncSetupStarted    = "TODO-COPY Sync is set up. The first download runs in HTML Clay."
	TextSyncSetupNoAccount  = "TODO-COPY There is no account or team called %s. Run htmlclay sync status to see them."
	TextWarnPersonalFolder  = "This is one of your main folders. HTML Clay will be able to save every document in it."
	TextWarnContainsTrusted = "HTML Clay already trusts these folders inside it, so they will become part of this folder: %s."
	// TextWarnInsideTrusted is the Trust sheet's warning for a folder inside one
	// already trusted, the one trust warning a person has to acknowledge.
	TextWarnInsideTrusted = "This folder is inside %s. Its documents get a new address, and what they stored in your browser stays at the old address."
	// TextWarnDifferentFolder is the Find it sheet's warning when the folder at the
	// new path is not the one that moved.
	TextWarnDifferentFolder = "This is a different folder from the one that moved. HTML Clay would trust this folder in place of the old one."
)

// Why texts: the footnote or tooltip beside a control that is off.
const (
	TextWhyServeMissing          = "This folder can't be found where it was."                                 // copy: why Serve is off, max 90 chars
	TextWhyServeIdentityChanged  = "A different folder now sits where this one was."                          // copy: why Serve is off, max 90 chars
	TextWhyServeServedByLocal    = "Hyperclay Local is serving this folder now."                              // copy: why Serve is off, max 90 chars
	TextWhyServeOverlaps         = "This folder is inside or around %s, which HTML Clay already serves."      // copy: why Serve is off, max 90 chars
	TextWhyServeBoundToLocalSync = "This folder syncs with Hyperclay Local."                                  // copy: why Serve is off, max 90 chars
	TextWhyNotOnComputer         = "This team folder isn't on this computer yet."                             // copy: why Serve or Sync is off, max 90 chars
	TextWhySyncNotFromAccount    = "Only folders from your Hyperclay account can sync."                       // copy: why Sync is off, max 90 chars
	TextWhySyncConnectFirst      = "Connect your Hyperclay account first."                                    // copy: why Sync is off, max 90 chars
	TextWhySyncServeFirst        = "Serve this folder first."                                                 // copy: why Sync is off, max 90 chars
	TextWhySyncViewer            = "You can view this team folder but not change it."                         // copy: why Sync is off, max 90 chars
	TextWhySyncMissing           = "This folder can't be found where it was."                                 // copy: why Sync is off, max 90 chars
	TextWhyStopTrustingServed    = "Stop serving this folder first."                                          // copy: why Stop Trusting is off, max 90 chars
	TextWhyMergeNotOwnAddress    = "This folder already uses the address of the folder around it."            // copy: why Merge is off, max 90 chars
	TextWhyMergeNotLocal         = "You can only merge folders on this computer that don't belong to a team." // copy: why Merge is off, max 90 chars
)

// Waiting items: the title, and where a %s appears the subject's name.
const (
	TextWaitingProgramRequest  = "%s wants to run %s"                // copy: waiting item title, document then program, max 70 chars
	TextWaitingAIEditRequest   = "Allow AI editing for %s?"          // copy: waiting item title for AI editing, the document's name, max 70 chars
	TextWaitingFolderRead      = "%s wants to read %s"               // copy: waiting item title, document then folder, max 70 chars
	TextWaitingFolderReadAgain = "%s wants you to trust %s"          // copy: waiting item title, document then folder, max 70 chars
	TextWaitingProgramMissing  = "%s can't be found"                 // copy: waiting item title, the program's name, max 70 chars
	TextWaitingFolderMissing   = "%s can't be found"                 // copy: waiting item title, the folder's name, max 70 chars
	TextWaitingServiceFailed   = "HTML Clay's live features stopped" // copy: waiting item title when the service failed, max 70 chars
	TextWaitingConflict        = "%s changed in both places"         // copy: waiting item title, the file's path in the folder, max 70 chars
	TextWaitingSyncKeyRefused  = "Your sync key was refused"         // copy: waiting item title, max 70 chars
	TextWaitingFileTooBig      = "%s is too big to sync"             // copy: waiting item title, the file's name, max 70 chars
	TextWaitingTeamToSetUp     = "%s is ready to set up"             // copy: waiting item title, the team folder's name, max 70 chars
	TextWaitingRemovalHeld     = "TODO-COPY %s attachments were removed from %s at once"
	TextWaitingFileNotText     = "TODO-COPY %s pages in %s aren't saved as UTF-8"          // copy: waiting item title, a count then the folder's name, max 70 chars; detail: they stay on this computer until saved as UTF-8
	TextWaitingNameClash       = "TODO-COPY %s files in %s differ only in capital letters" // copy: waiting item title, a count then the folder's name, max 70 chars; detail: this computer keeps one; rename the other on hyperclay.com
	TextNoticeConflictBody     = "TODO-COPY Choose which copy to keep in HTML Clay."
	TextNoticeKeyStoppedBody   = "TODO-COPY Sync is paused. Add a new sync key in HTML Clay."
	TextNoticeAddedToTeam      = "TODO-COPY You were added to %s"
	TextNoticeAddedToTeamBody  = "TODO-COPY Set up its folder in HTML Clay."
	TextTrayPauseSyncing       = "TODO-COPY Pause Syncing"  // copy: tray item, Windows and Linux, max 30 chars
	TextTrayResumeSyncing      = "TODO-COPY Resume Syncing" // copy: tray item, Windows and Linux, max 30 chars
	TextTrayOneConflict        = "TODO-COPY 1 Conflict…"    // copy: tray item opening Needs you, max 30 chars
	TextTrayConflicts          = "TODO-COPY %d Conflicts…"  // copy: tray item opening Needs you, the count, max 30 chars
)

// RefusalText is the one-line "what happened, and what to do" for each refusal code.
// %s, where present, is the other folder (OtherPath, or OtherName when set).
var RefusalText = map[string]string{
	RefuseNotAFolder:         "That is not a folder. Pick a folder instead.",                                                // copy: refusal, max 140 chars
	RefuseOutsideHome:        "Only folders inside your home folder can be served. Pick a folder inside it.",                // copy: refusal, max 140 chars
	RefuseIsHome:             "Your whole home folder can't be served. Pick a folder inside it.",                            // copy: refusal, max 140 chars
	RefuseProtectedFolder:    "HTML Clay keeps its own files in this folder, so it can't be served. Pick another folder.",   // copy: refusal, max 140 chars
	RefuseOverlapsServed:     "This folder overlaps %s, which is already served. Stop serving that one first.",              // copy: refusal, max 140 chars
	RefuseServedByLocal:      "Hyperclay Local is serving this folder now. Stop it there first.",                            // copy: refusal, max 140 chars
	RefuseBoundToLocalSync:   "This folder syncs with Hyperclay Local. Disconnect it there first.",                          // copy: refusal, max 140 chars
	RefuseNestedTrusted:      "This folder overlaps the trusted folder %s. Stop trusting one of them first.",                // copy: refusal, max 140 chars
	RefuseNameInvalid:        "That name can't be used. Use lowercase letters, numbers and hyphens.",                        // copy: refusal, max 140 chars
	RefuseNameTaken:          "Another served folder already uses that name. Pick another.",                                 // copy: refusal, max 140 chars
	RefuseNameRetired:        "That name was used before and can't be used again. Pick another.",                            // copy: refusal, max 140 chars
	RefuseUnknownAction:      "HTML Clay doesn't know that action. Update HTML Clay and try again.",                         // copy: refusal, max 140 chars
	RefuseBadArguments:       "Something was missing from that request. Try again.",                                         // copy: refusal, max 140 chars
	RefuseNoSuchFolder:       "That folder is no longer listed. Add it again if you still want to use it.",                  // copy: refusal, max 140 chars
	RefuseNoSuchProgram:      "That program is no longer listed. Look at the list again.",                                   // copy: refusal, max 140 chars
	RefuseNoSuchDocument:     "That document is no longer listed. Open it again if you still need it.",                      // copy: refusal, max 140 chars
	RefuseFileMissing:        "That file isn't there anymore. It may have moved, or you may have deleted it.",               // copy: refusal, max 140 chars
	RefuseFileOutsideHome:    "HTML Clay only opens files inside your home folder. Move the file there and open it again.",  // copy: refusal, max 140 chars
	RefuseDestinationExists:  "A file with that name is already there. Choose another name.",                                // copy: refusal, max 140 chars
	RefuseNoSuchRequest:      "That request has ended. Try again if you still need it.",                                     // copy: refusal, max 140 chars
	RefuseRequestChanged:     "That request changed while you were answering. Look at it again.",                            // copy: refusal, max 140 chars
	RefuseAnswerNotOffered:   "That answer isn't one this request offers. Choose one of the options shown.",                 // copy: refusal, max 140 chars
	RefuseNotAllowedNow:      "That isn't possible for this folder right now. Check the folder in HTML Clay and try again.", // copy: refusal, max 140 chars
	RefuseUnknownPreference:  "HTML Clay doesn't have that setting. Check the setting name or update HTML Clay.",            // copy: refusal, max 140 chars
	RefuseNothingToMove:      "There is nothing to move from Hyperclay Local. Pick a folder to serve instead.",              // copy: refusal, max 140 chars
	RefuseNotConnected:       "Your Hyperclay account isn't connected. Connect it in HTML Clay first.",                      // copy: refusal, max 140 chars
	RefuseSyncKeyShape:       "TODO-COPY That isn't a sync key. A sync key starts with hcsk_.",
	RefuseSyncKeyRefused:     "TODO-COPY Hyperclay didn't accept that sync key. Make a new one on hyperclay.com.",
	RefuseSyncWrongAccount:   "TODO-COPY That sync key belongs to a different account.",
	RefuseSyncOffline:        "TODO-COPY HTML Clay couldn't reach Hyperclay. Check your connection and try again.",
	RefuseSyncServerUpdate:   "TODO-COPY Hyperclay needs an update before HTML Clay can sync with it.",
	RefuseServiceNeeded:      "TODO-COPY Sync starts once a folder is served. Serve a folder and try again.",
	RefuseSyncFolderNotEmpty: "TODO-COPY A team folder starts empty. Pick an empty folder or a new one.",
	RefuseSyncPaused:         "TODO-COPY Sync is paused. Turn sync back on, then choose which copy to keep.",
	RefuseConflictGone:       "TODO-COPY That file isn't in conflict any more. Nothing was changed.",
	RefuseConflictChanged:    "TODO-COPY That file changed since you looked. Look at both copies again, then choose.",
	RefuseNotText:            "TODO-COPY This page isn't saved as UTF-8, so it can't be sent. Nothing was changed.",
	RefuseSyncDownloading:    "TODO-COPY This folder's first sync is still running. Find it again when it finishes.",
	RefuseRemovalGone:        "TODO-COPY Those attachments aren't waiting for you any more. Nothing was changed.",
	RefuseConfirmNeeded:      "TODO-COPY This folder already has files. Check what will sync, then confirm.",
	RefuseNothingReady:       "No update is ready yet. Try again later.",                                                    // copy: refusal, max 140 chars
	RefuseNeedsConfirmation:  "This action needs your approval. Approve it in HTML Clay first.",                             // copy: refusal, max 140 chars
	RefuseFailed:             "HTML Clay couldn't do that just now. Try again.",                                             // copy: refusal, max 140 chars
	RefuseCannotTrust:        "HTML Clay can't trust this folder. Pick a folder inside your home folder that isn't hidden.", // copy: refusal, max 140 chars
	RefuseStillRunning:       "HTML Clay is still working on that. Wait a moment.",                                          // copy: refusal, max 140 chars
	RefuseNotYet:             "HTML Clay can't do that yet. Update HTML Clay and try again.",                                // copy: refusal, max 140 chars
	// A folder rule's reason, when an action runs into it.
	WhyIdentityChanged:    TextWhyServeIdentityChanged,
	WhyNotOnComputer:      TextWhyNotOnComputer,
	WhyNotFromAccount:     TextWhySyncNotFromAccount,
	WhyConnectFirst:       TextWhySyncConnectFirst,
	WhyServeFirst:         TextWhySyncServeFirst,
	WhyViewer:             TextWhySyncViewer,
	WhyStopServingFirst:   TextWhyStopTrustingServed,
	WhyNotOwnAddress:      TextWhyMergeNotOwnAddress,
	WhyNotLocal:           TextWhyMergeNotLocal,
	RefuseServedElsewhere: "Another copy of HTML Clay is serving this folder. Stop it there first.",      // copy: refusal, max 140 chars
	RefuseUnsavedEdits:    "Some edits in this folder haven't been saved yet. Try again in a moment.",    // copy: refusal, max 140 chars
	RefuseQuitUnsaved:     "Some edits haven't been saved yet. Try again, or quit anyway and lose them.", // copy: refusal, max 140 chars
}

// RefusalMessage fills in RefusalText for r.
func RefusalMessage(r *Refusal) string {
	if r == nil {
		return ""
	}
	if r.Text != "" {
		return r.Text
	}
	text, ok := RefusalText[r.Code]
	if !ok {
		return r.Code
	}
	if !strings.Contains(text, "%s") {
		return text
	}
	other := r.OtherName
	if other == "" {
		other = r.OtherPath
	}
	return fmt.Sprintf(text, other)
}
