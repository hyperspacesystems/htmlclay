package appcore

import (
	"fmt"
	"runtime"
	"strings"
)

// Text people read. Every value is working text until the copywriter replaces it;
// each copy: note says where it shows and how long it may be.

// The question an update restart asks on Windows and Linux when sync has not
// finished after the wait.
const (
	TextUpdateSyncBusyTitle   = "Sync is still sending your changes"
	TextUpdateSyncBusyBody    = "HTML Clay waited for sync to finish before updating to HTML Clay %s, and it is still working.\n\nRestarting now could bring back a file you just deleted or lose a change that is still uploading."
	TextUpdateSyncBusyRestart = "Restart anyway"
)

const (
	TextQuitUnsavedTitle   = "Some edits aren't saved yet"                                                       // copy: quit dialog title, max 40 chars
	TextQuitUnsavedBody    = "HTML Clay couldn't save these documents:\n\n%s\n\nQuitting now loses those edits." // copy: quit dialog body, %s is the documents one per line, max 200 chars without the list
	TextQuitRetry          = "Retry"                                                                             // copy: quit dialog button that tries to save again, max 16 chars
	TextQuitAnyway         = "Quit anyway"                                                                       // copy: quit dialog button that quits and loses the edits, max 16 chars
	TextQuitCancel         = "Cancel"                                                                            // copy: quit dialog button that keeps the app open, max 16 chars
	TextQuitNotQuitting    = "HTML Clay is still open because you have unsaved edits: %s. Try quitting again in a moment."
	TextQuitNoAnswer       = "Your open documents could not be checked."                          // copy: quit dialog list line when the service could not be asked, max 60 chars
	TextServeAFolder       = "Serve a folder…"                                                    // copy: tray row that opens the folder picker, max 24 chars
	TextServedFolders      = "Served folders"                                                     // copy: tray submenu title, max 24 chars
	TextNoServedFolders    = "No served folders yet"                                              // copy: tray, disabled row when the list is empty, max 32 chars
	TextOpenServed         = "Open workspace"                                                     // copy: tray, per folder, max 24 chars
	TextCopyAddress        = "Copy address"                                                       // copy: tray, per folder, max 24 chars
	TextStopServing        = "Stop serving"                                                       // copy: tray, per folder, and the confirm button, max 24 chars
	TextFolderMissing      = "(missing)"                                                          // copy: tray, after a served folder's name when its folder is gone, max 16 chars
	TextPickServeFolder    = "Choose a folder to serve at its own address."                       // copy: folder picker prompt, max 120 chars
	TextStopServingTitle   = "Stop serving this folder?"                                          // copy: confirm dialog title, max 40 chars
	TextStopServingMessage = "%s will stop working. The files in the folder stay where they are." // copy: confirm dialog body, %s is the address, max 160 chars
	TextNowServed          = "%s is served at %s"                                                 // copy: notification and CLI success line, %s name then address, max 120 chars
	TextNotServed          = "No served folder is named %s."                                      // copy: CLI, stop-serving with an unknown name, max 80 chars
	TextStopped            = "Stopped serving %s."                                                // copy: CLI success line, max 80 chars
	TextCannotServe        = "HTML Clay can't serve this folder"                                  // copy: notification title for a refusal, max 48 chars
	// The htmlclay sync command line (08-sync.md step 8.12).
	TextSyncKeyPrompt     = "Sync key: "                                                             // copy: CLI prompt on a terminal, max 20 chars
	TextSyncKeyNoArgument = "A sync key can't go in the command. Type it when asked, or pipe it in." // copy: CLI, key given as an argument, max 120 chars
	TextSyncConnectedAs   = "Connected as %s"                                                        // copy: CLI success, %s username, max 80 chars
	TextSyncDisconnected  = "Disconnected. Your synced folders stay on this computer."               // copy: CLI success, max 120 chars
	TextSyncPausedLine    = "Syncing is paused"                                                      // copy: CLI success and status line, max 80 chars
	TextSyncResumedLine   = "Syncing is on"                                                          // copy: CLI success and status line, max 80 chars
	TextSyncNotConnected  = "Not connected to hyperclay.com"                                         // copy: CLI status line, max 80 chars
	// The htmlclay sync setup lines (08-sync.md step 8.25).
	TextSyncSetupDownload    = "%d files (%s) will download into %s."
	TextSyncSetupExistingOne = "1 file to upload, %d to download, %d different. You choose in Needs you. Nothing is deleted."
	TextSyncSetupExisting    = "%d files to upload, %d to download, %d different. You choose in Needs you. Nothing is deleted." // copy: corrected claim (review 2), differing files wait in Needs you
	TextSyncSetupAsk         = "Set up sync? [y/N] "
	TextSyncSetupStarted     = "Sync is set up. HTML Clay is syncing this folder now."
	TextSyncSetupNoAccount   = "No account or team is named %s. Run htmlclay sync status to see yours."
	TextWarnPersonalFolder   = "This is one of your main folders. HTML Clay will be able to save every document in it."
	TextWarnContainsTrusted  = "HTML Clay already trusts these folders inside it, so they will become part of this folder: %s."
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
	TextWaitingProgramRequest    = "%s wants “%s”"                                               // copy: waiting item title, document then program, max 70 chars
	TextWaitingAIEditRequest     = "Allow AI editing for %s?"                                    // copy: waiting item title for AI editing, the document's name, max 70 chars
	TextWaitingFolderRead        = "%s wants to read %s"                                         // copy: waiting item title, document then folder, max 70 chars
	TextWaitingFolderReadAgain   = "%s wants you to trust %s"                                    // copy: waiting item title, document then folder, max 70 chars
	TextWaitingProgramMissing    = "%s can't be found"                                           // copy: waiting item title, the program's name, max 70 chars
	TextWaitingFolderMissing     = "%s can't be found"                                           // copy: waiting item title, the folder's name, max 70 chars
	TextWaitingServiceFailed     = "HTML Clay's live features stopped"                           // copy: waiting item title when the service failed, max 70 chars
	TextWaitingConflict          = "%s changed in both places"                                   // copy: waiting item title, the file's path in the folder, max 70 chars
	TextWaitingSyncKeyRefused    = "Your sync key was refused"                                   // copy: waiting item title, max 70 chars
	TextWaitingSyncKeyUnreadable = "HTML Clay can't read your sync key. Enter it again to sync." // copy: waiting item title when the saved key cannot be read, max 70 chars
	TextWaitingFileTooBig        = "%s is too big to sync"                                       // copy: waiting item title, the file's name, max 70 chars
	TextWaitingTeamToSetUp       = "%s is ready to set up"                                       // copy: waiting item title, the team folder's name, max 70 chars
	TextWaitingRemovalHeld       = "%s attachments were removed from %s at once"
	TextWaitingFileNotText       = "%s pages in %s aren't saved as UTF-8"          // copy: waiting item title, a count then the folder's name, max 70 chars; detail: they stay on this computer until saved as UTF-8
	TextWaitingNameClash         = "%s files in %s differ only in capital letters" // copy: waiting item title, a count then the folder's name, max 70 chars; detail: this computer keeps one; rename the other on hyperclay.com
	TextNoticeConflictBody       = "Sync held this file. Open Needs you in HTML Clay to choose."
	TextNoticeKeyStoppedBody     = "Syncing stopped. Your files are safe. Add a new sync key in HTML Clay."
	TextNoticeAddedToTeam        = "You were added to %s"
	TextNoticeAddedToTeamBody    = "Set up its folder in HTML Clay."
	TextNoticeAddedToTeamBodyCLI = "Run htmlclay sync setup to set up its folder." // copy: Linux system notification body, names the command, max 120 chars
	TextTrayPauseSyncing         = "Pause syncing"                                 // copy: tray item, Windows and Linux, max 30 chars
	TextTrayResumeSyncing        = "Resume syncing"                                // copy: tray item, Windows and Linux, max 30 chars
	TextTrayOneConflict          = "1 conflict…"                                   // copy: tray item opening Needs you, max 30 chars
	TextTrayConflicts            = "%d conflicts…"                                 // copy: tray item opening Needs you, the count, max 30 chars

	TextMenuVersion         = "HTML Clay %s"                                                                                                         // copy: one menu, status row, %s is the version, max 32 chars
	TextMenuChecking        = "Checking for updates…"                                                                                                // copy: one menu, status row while checking
	TextMenuRestartToUpdate = "Restart to update to %s"                                                                                              // copy: one menu, status row with an update ready, %s is the version
	TextMenuDownloadUpdate  = "Update available: %s"                                                                                                 // copy: one menu, Linux and Windows builds that cannot install the update themselves
	TextMenuSyncUpToDate    = "Up to date · @%s"                                                                                                     // copy: one menu, sync line, %s is the username
	TextMenuSyncingOne      = "Syncing 1 file… · @%s"                                                                                                // copy: one menu, sync line
	TextMenuSyncingMany     = "Syncing %d files… · @%s"                                                                                              // copy: one menu, sync line
	TextMenuSyncPaused      = "Syncing paused"                                                                                                       // copy: one menu, sync line
	TextMenuNeedsYou        = "Needs you (%d)"                                                                                                       // copy: one menu, submenu title
	TextMenuOpenDashboard   = "Open dashboard"                                                                                                       // copy: one menu
	TextMenuOpenFile        = "Open a file…"                                                                                                         // copy: one menu
	TextMenuRecent          = "Recent documents"                                                                                                     // copy: one menu, submenu title
	TextMenuNoRecent        = "No recent documents"                                                                                                  // copy: one menu, the disabled row inside an empty Recent documents
	TextMenuRecovered       = "Recovered changes…"                                                                                                   // copy: one menu, shown only when there is at least one
	TextMenuSettings        = "Settings…"                                                                                                            // copy: one menu
	TextMenuCheckForUpdates = "Check for updates…"                                                                                                   // copy: one menu
	TextMenuQuitMac         = "Quit HTML Clay"                                                                                                       // copy: one menu, macOS
	TextMenuQuitWindows     = "Exit"                                                                                                                 // copy: one menu, Windows
	TextMenuQuitLinux       = "Quit"                                                                                                                 // copy: one menu, Linux
	TextPortRestartFailed   = "HTML Clay saved the new port, though it could not restart on its own. Quit and reopen HTML Clay to use the new port." // copy: notification when the app could not restart on the new port; the new port takes effect at the next start, max 160 chars
)

// TextPortMoved is the old port's answer to a save or other non-page request, %s
// is the new address, max 160 chars. It is a variable so it still builds with
// Sprintf.
var TextPortMoved = "HTML Clay moved to %s. Copy any new edits, then reload this page to open it at the new address."

// RefusalText is the one-line "what happened, and what to do" for each refusal code.
// %s, where present, is the other folder (OtherPath, or OtherName when set).
var RefusalText = map[string]string{
	RefuseNotAFolder:         "That is not a folder. Pick a folder instead.",                                                                              // copy: refusal, max 140 chars
	RefuseOutsideHome:        "Only folders inside your home folder can be served. Pick a folder inside it.",                                              // copy: refusal, max 140 chars
	RefuseIsHome:             "Your whole home folder can't be served. Pick a folder inside it.",                                                          // copy: refusal, max 140 chars
	RefuseProtectedFolder:    "HTML Clay keeps its own files in this folder, so it can't be served. Pick another folder.",                                 // copy: refusal, max 140 chars
	RefuseOverlapsServed:     "This folder overlaps %s, which is already served. Stop serving that one first.",                                            // copy: refusal, max 140 chars
	RefuseServedByLocal:      "Hyperclay Local is serving this folder now. Stop it there first.",                                                          // copy: refusal, max 140 chars
	RefuseBoundToLocalSync:   "This folder syncs with Hyperclay Local. Disconnect it there first.",                                                        // copy: refusal, max 140 chars
	RefuseNestedTrusted:      "This folder overlaps the trusted folder %s. Stop trusting one of them first.",                                              // copy: refusal, max 140 chars
	RefuseNameInvalid:        "That name can't be used. Use lowercase letters, numbers and hyphens.",                                                      // copy: refusal, max 140 chars
	RefuseNameTaken:          "Another served folder already uses that name. Pick another.",                                                               // copy: refusal, max 140 chars
	RefuseNameRetired:        "That name was used before and can't be used again. Pick another.",                                                          // copy: refusal, max 140 chars
	RefuseUnknownAction:      "HTML Clay doesn't know that action. Update HTML Clay and try again.",                                                       // copy: refusal, max 140 chars
	RefuseBadArguments:       "Something was missing from that request. Try again.",                                                                       // copy: refusal, max 140 chars
	RefuseNoSuchFolder:       "That folder is no longer listed. Add it again if you still want to use it.",                                                // copy: refusal, max 140 chars
	RefuseNoSuchProgram:      "That program is no longer listed. Look at the list again.",                                                                 // copy: refusal, max 140 chars
	RefuseNoSuchDocument:     "That document is no longer listed. Open it again if you still need it.",                                                    // copy: refusal, max 140 chars
	RefuseFileMissing:        "That file isn't there anymore. It may have moved, or you may have deleted it.",                                             // copy: refusal, max 140 chars
	RefuseFileOutsideHome:    "HTML Clay only opens files inside your home folder. Move the file there and open it again.",                                // copy: refusal, max 140 chars
	RefuseDestinationExists:  "A file with that name is already there. Choose another name.",                                                              // copy: refusal, max 140 chars
	RefuseNoSuchRequest:      "That request has ended. Try again if you still need it.",                                                                   // copy: refusal, max 140 chars
	RefuseRequestChanged:     "That request changed while you were answering. Look at it again.",                                                          // copy: refusal, max 140 chars
	RefuseAnswerNotOffered:   "That answer isn't one this request offers. Choose one of the options shown.",                                               // copy: refusal, max 140 chars
	RefuseNotAllowedNow:      "That isn't possible for this folder right now. Check the folder in HTML Clay and try again.",                               // copy: refusal, max 140 chars
	RefuseUnknownPreference:  "HTML Clay doesn't have that setting. Check the setting name or update HTML Clay.",                                          // copy: refusal, max 140 chars
	RefuseNothingMoved:       "No folders moved. They stay in Hyperclay Local, which keeps serving and syncing them. Check each folder below to see why.", // copy: refusal, max 140 chars
	RefuseNothingToMove:      "There is nothing to move from Hyperclay Local. Pick a folder to serve instead.",                                            // copy: refusal, max 140 chars
	RefuseNotConnected:       "Your Hyperclay account isn't connected. Connect it in HTML Clay first.",                                                    // copy: refusal, max 140 chars
	RefuseLocalRunning:       "Hyperclay Local is still open. Quit it, then move your folders.",
	RefuseSyncKeyShape:       "That isn't a complete sync key. Paste the full key starting with hcsk_.",
	RefuseSyncKeyRefused:     "Hyperclay didn't accept that sync key. Get a new one on hyperclay.com.",
	RefuseSyncWrongAccount:   "That sync key is for another account. Check the username or paste another key.",
	RefuseSyncOffline:        "HTML Clay couldn't reach hyperclay.com. Check your connection and try again.",
	RefuseSyncServerUpdate:   "Hyperclay needs an update before HTML Clay can sync with it. Try again later.",
	RefuseServiceNeeded:      "HTML Clay isn't ready to sync yet. Try again in a moment.",
	RefuseSyncFolderNotEmpty: "This folder isn't empty. Choose an empty folder or make a new one.",
	// copy: set-up refusal for a folder already in HTML Clay (it keeps its own identity), max 120 chars
	RefuseSyncFolderInUse: "This folder is already in HTML Clay. Choose another folder or make a new one.",
	RefuseSyncPaused:      "This folder isn't syncing. Turn syncing back on in HTML Clay, then try again.",
	// copy: refusal when settling a conflict while the sync key needs replacing, max 120 chars
	RefuseSyncKeyStopped:    "Your choice did not save. The item stays in Needs you. Enter your sync key again in Sync, then choose again.",
	RefuseConflictGone:      "That file isn't in conflict anymore. Nothing was changed.",
	RefuseConflictChanged:   "That file changed since you looked. Review it again, then choose.",
	RefuseNotText:           "This page isn't UTF-8 text, so it can't upload. Save it as UTF-8 and try again.",
	RefuseSyncDownloading:   "This folder's first sync is still running. Try Find it again once it finishes.",
	RefuseRemovalGone:       "Those attachments aren't waiting for you anymore. Nothing was changed.",
	RefuseConfirmNeeded:     "This folder already has files. Check what will sync, then confirm.",
	RefuseNothingReady:      "No update is ready yet. Try again later.",                                                       // copy: refusal, max 140 chars
	RefuseNeedsConfirmation: "This action needs your approval. Approve it in HTML Clay first.",                                // copy: refusal, max 140 chars
	RefusePending:           "Answer the HTML Clay dialog to finish.",                                                         // copy: COPY:refusal.pending, shown on a dashboard button while the system dialog is open, max 140 chars
	RefuseDialogOpen:        "Another HTML Clay dialog is already open. Answer it first.",                                     // copy: COPY:refusal.dialogOpen, max 140 chars
	RefuseNoDialogs:         "HTML Clay cannot show dialogs on this computer yet. Install zenity or kdialog, then try again.", // copy: COPY:refusal.noDialogs, Linux only, max 140 chars
	RefuseFailed:            "HTML Clay couldn't do that just now. Try again.",                                                // copy: refusal, max 140 chars
	RefuseCannotTrust:       "HTML Clay can't trust this folder. Pick a folder inside your home folder that isn't hidden.",    // copy: refusal, max 140 chars
	RefuseStillRunning:      "Something is still running in HTML Clay. Wait a moment, then try again.",                        // copy: refusal, max 140 chars
	RefuseNotYet:            "HTML Clay can't do that yet. Update HTML Clay and try again.",                                   // copy: refusal, max 140 chars
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
	RefusePortInvalid:     "Enter a port number between 1024 and 65535.",
	RefusePortSame:        "HTML Clay is already using that port. Enter a different port number.",
	RefusePortTaken:       "Port %s is already being used by another app on this Mac. Try another number.",
	RefusePortUnsaved:     "Save your open documents before changing the port, then try again.",
	RefuseNoSuchTool:      "That AI tool was not found on this Mac. Choose an installed tool instead.",
	RefuseCommandInvalid:  "Enter a command to run, and make sure every quotation mark is closed.",
	RefuseCommandNotFound: "Could not find “%s” on this Mac. Enter a full file path or a command installed on your system.",
	RefuseNotOnThisSystem: "Change this setting in the HTML Clay app window instead.",
}

// ForSystem swaps the Mac's words in text for goos's: "this Mac" becomes "this
// PC" or "this computer", "Finder" becomes "File Explorer" or "your file manager".
func ForSystem(text, goos string) string {
	switch goos {
	case "windows":
		return strings.NewReplacer("this Mac", "this PC", "your Mac", "your PC", "in Finder", "in File Explorer").Replace(text)
	case "linux":
		return strings.NewReplacer("another app on this Mac", "another program on this computer", "this Mac", "this computer", "your Mac", "your computer", "in Finder", "in your file manager").Replace(text)
	}
	return text
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
		return ForSystem(text, runtime.GOOS)
	}
	other := r.OtherName
	if other == "" {
		other = r.OtherPath
	}
	return ForSystem(fmt.Sprintf(text, other), runtime.GOOS)
}

// The dialogs that confirm what a dashboard page asked for (Windows and Linux).
// Every %s is a fact the host holds, a folder's path or a program's name, or
// the custom command the person typed, shown back to them before it is saved.
const (
	TextGrantTitle            = "HTML Clay"
	TextGrantServe            = "Serve this folder at its own local web address?\n\n%s\n\nRegular .html files inside it will save too."
	TextGrantServeYes         = "Serve"
	TextGrantSync             = "Sync this folder with hyperclay.com?\n\n%s\n\nIts files will stay in step with your hyperclay.com account."
	TextGrantSyncYes          = "Sync"
	TextGrantMerge            = "Merge this folder into the trusted folder around it?\n\n%s\n\nIts documents move to the outer folder's address, and anything they saved in your browser stays at the old address. Your files on disk do not move."
	TextGrantMergeYes         = "Merge"
	TextGrantFraming          = "Let other websites show this folder's pages inside their own pages?\n\n%s\n\nTurn this on only for pages you mean to share with another site."
	TextGrantFramingYes       = "Allow"
	TextGrantAllTrusted       = "Let all documents in your trusted folders use %s? Team folders still ask one document at a time."
	TextGrantAllTrustedYes    = "Allow"
	TextGrantAskAgain         = "Let this document ask to use %s again?\n\n%s"
	TextGrantAskAgainYes      = "Allow again"
	TextGrantMove             = "Move your folders from Hyperclay Local into HTML Clay?"
	TextGrantMoveYes          = "Move folders"
	TextGrantRelinkWarn       = "Use this folder in place of the one that moved?\n\n%s\n\n%s"
	TextGrantRelinkYes        = "Use this folder"
	TextGrantSyncExisting     = "Sync into this folder? It already has files.\n\n%s\n\n%d to upload, %d to download, %d different. You choose in Needs you. Nothing is deleted."
	TextGrantRelinkPick       = "Choose where this folder moved."
	TextGrantTeamPick         = "Choose where this team folder goes on your Mac."
	TextGrantOpenPick         = "Choose a file to open in HTML Clay"
	TextGrantConnectTitle     = "Connect hyperclay.com"
	TextGrantConnectMessage   = "Paste your sync key from your account settings on hyperclay.com."
	TextGrantProgramPick      = "Choose the program file that documents may run."
	TextGrantAddProgram       = "Add this program as \u201c%s\u201d?\n\n%s\n\nNo document can run it until you allow one. When allowed, it runs as you and can read or change your files."
	TextGrantAddProgramRisk   = "Be careful with this file: it came from the internet, sits in a download or temporary folder, changed just now, or is not marked to run yet."
	TextGrantAddProgramYes    = "Add program"
	TextGrantBuiltinOn        = "Turn on %s? Documents still ask before they can use it."
	TextGrantBuiltinOnYes     = "Turn on"
	TextGrantExample          = "Open the %s example? HTML Clay saves it in an examples folder inside htmlclay in your home folder, trusts that folder, and lets the example run."
	TextGrantExampleYes       = "Open example"
	TextGrantCustomCommand    = "Run this command when you rewrite selected text?\n\n%s\n\nHTML Clay sends your selected text and request to it."
	TextGrantCustomCommandYes = "Save command"
)
