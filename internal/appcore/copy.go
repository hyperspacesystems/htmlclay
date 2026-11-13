package appcore

import (
	"fmt"
	"strings"
)

// Text people read. Every value is working text until the copywriter replaces it;
// each copy: note says where it shows and how long it may be.

const (
	TextQuitUnsavedTitle    = "Some edits aren't saved yet"                                                               // copy: quit dialog title, max 40 chars
	TextQuitUnsavedBody     = "HTML Clay couldn't save these documents:\n\n%s\n\nQuitting now loses those edits."         // copy: quit dialog body, %s is the documents one per line, max 200 chars without the list
	TextQuitRetry           = "Retry"                                                                                     // copy: quit dialog button that tries to save again, max 16 chars
	TextQuitAnyway          = "Quit anyway"                                                                               // copy: quit dialog button that quits and loses the edits, max 16 chars
	TextQuitCancel          = "Cancel"                                                                                    // copy: quit dialog button that keeps the app open, max 16 chars
	TextQuitNotQuitting     = "HTML Clay is still open because these edits aren't saved: %s. Try Quit again in a moment." // TODO-COPY: notice when no quit dialog can be shown (Linux without zenity or kdialog); %s lists the documents
	TextQuitNoAnswer        = "Your open documents could not be checked."                                                 // copy: quit dialog list line when the service could not be asked, max 60 chars
	TextServeAFolder        = "Serve a Folder…"                                                                           // copy: tray row that opens the folder picker, max 24 chars
	TextServedFolders       = "Served Folders"                                                                            // copy: tray submenu title, max 24 chars
	TextNoServedFolders     = "No served folders yet"                                                                     // copy: tray, disabled row when the list is empty, max 32 chars
	TextOpenServed          = "Open Workspace"                                                                            // copy: tray, per folder, max 24 chars
	TextCopyAddress         = "Copy Address"                                                                              // copy: tray, per folder, max 24 chars
	TextStopServing         = "Stop Serving"                                                                              // copy: tray, per folder, and the confirm button, max 24 chars
	TextFolderMissing       = "(missing)"                                                                                 // copy: tray, after a served folder's name when its folder is gone, max 16 chars
	TextPickServeFolder     = "Choose a folder to serve at its own address."                                              // copy: folder picker prompt, max 120 chars
	TextStopServingTitle    = "Stop serving this folder?"                                                                 // copy: confirm dialog title, max 40 chars
	TextStopServingMessage  = "%s will stop working. The files in the folder stay where they are."                        // copy: confirm dialog body, %s is the address, max 160 chars
	TextNowServed           = "%s is served at %s"                                                                        // copy: notification and CLI success line, %s name then address, max 120 chars
	TextNotServed           = "No served folder is named %s."                                                             // copy: CLI, stop-serving with an unknown name, max 80 chars
	TextStopped             = "Stopped serving %s."                                                                       // copy: CLI success line, max 80 chars
	TextCannotServe         = "HTML Clay can't serve this folder"                                                         // copy: notification title for a refusal, max 48 chars
	TextServeCLIWindows     = "The htmlclay serve command runs on Mac and Linux. On Windows, use the HTML Clay menu."     // copy: CLI, on Windows, max 120 chars
	TextWarnPersonalFolder  = "This is one of your main folders. HTML Clay will be able to save every document in it."    // TODO-COPY: trust sheet warning before trusting a personal folder
	TextWarnContainsTrusted = "These folders inside it are already trusted and become part of it: %s."                    // TODO-COPY: trust sheet warning, %s lists the trusted folders inside
)

// Why texts: the footnote or tooltip beside a control that is off.
const (
	TextWhyServeMissing          = "This folder can't be found where it was."           // copy: why Serve is off, max 90 chars
	TextWhyServeIdentityChanged  = "A different folder now sits where this one was."    // copy: why Serve is off, max 90 chars
	TextWhyServeServedByLocal    = "Hyperclay Local is serving this folder now."        // copy: why Serve is off, max 90 chars
	TextWhyServeBoundToLocalSync = "This folder syncs with Hyperclay Local."            // copy: why Serve is off, max 90 chars
	TextWhyNotOnComputer         = "This team folder isn't on this computer yet."       // copy: why Serve or Sync is off, max 90 chars
	TextWhySyncNotFromAccount    = "Only folders from your Hyperclay account can sync." // copy: why Sync is off, max 90 chars
	TextWhySyncConnectFirst      = "Connect your Hyperclay account first."              // copy: why Sync is off, max 90 chars
	TextWhySyncServeFirst        = "Serve this folder first."                           // copy: why Sync is off, max 90 chars
	TextWhySyncViewer            = "You can view this team folder but not change it."   // copy: why Sync is off, max 90 chars
	TextWhySyncMissing           = "This folder can't be found where it was."           // copy: why Sync is off, max 90 chars
	TextWhyStopTrustingServed    = "Stop serving this folder first."                    // copy: why Stop Trusting is off, max 90 chars
)

// Waiting items: the title, and where a %s appears the subject's name.
const (
	TextWaitingProgramRequest  = "%s wants to run %s"                // copy: waiting item title, document then program, max 70 chars
	TextWaitingFolderRead      = "%s wants to read %s"               // copy: waiting item title, document then folder, max 70 chars
	TextWaitingFolderReadAgain = "%s wants you to trust %s"          // copy: waiting item title, document then folder, max 70 chars
	TextWaitingProgramMissing  = "%s can't be found"                 // copy: waiting item title, the program's name, max 70 chars
	TextWaitingFolderMissing   = "%s can't be found"                 // copy: waiting item title, the folder's name, max 70 chars
	TextWaitingServiceFailed   = "HTML Clay's live features stopped" // copy: waiting item title when the service failed, max 70 chars
	TextWaitingConflict        = "%s changed in both places"         // copy: waiting item title, the file's path in the folder, max 70 chars
	TextWaitingSyncKeyRefused  = "Your sync key was refused"         // copy: waiting item title, max 70 chars
	TextWaitingFileTooBig      = "%s is too big to sync"             // copy: waiting item title, the file's name, max 70 chars
	TextWaitingTeamToSetUp     = "%s is ready to set up"             // copy: waiting item title, the team folder's name, max 70 chars
)

// RefusalText is the one-line "what happened, and what to do" for each refusal code.
// %s, where present, is the other folder (OtherPath, or OtherName when set).
var RefusalText = map[string]string{
	RefuseNotAFolder:        "That is not a folder. Pick a folder instead.",                                                // copy: refusal, max 140 chars
	RefuseOutsideHome:       "Only folders inside your home folder can be served. Pick a folder inside it.",                // copy: refusal, max 140 chars
	RefuseIsHome:            "Your whole home folder can't be served. Pick a folder inside it.",                            // copy: refusal, max 140 chars
	RefuseProtectedFolder:   "HTML Clay keeps its own files in this folder, so it can't be served. Pick another folder.",   // copy: refusal, max 140 chars
	RefuseOverlapsServed:    "This folder overlaps %s, which is already served. Stop serving that one first.",              // copy: refusal, max 140 chars
	RefuseServedByLocal:     "Hyperclay Local is serving this folder now. Stop it there first.",                            // copy: refusal, max 140 chars
	RefuseBoundToLocalSync:  "This folder syncs with Hyperclay Local. Disconnect it there first.",                          // copy: refusal, max 140 chars
	RefuseNestedTrusted:     "This folder overlaps the trusted folder %s. Stop trusting one of them first.",                // copy: refusal, max 140 chars
	RefuseNameInvalid:       "That name can't be used. Use lowercase letters, numbers and hyphens.",                        // copy: refusal, max 140 chars
	RefuseNameTaken:         "Another served folder already uses that name. Pick another.",                                 // copy: refusal, max 140 chars
	RefuseNameRetired:       "That name was used before and can't be used again. Pick another.",                            // copy: refusal, max 140 chars
	RefuseUnknownAction:     "HTML Clay doesn't know that action. Update HTML Clay and try again.",                         // copy: refusal, max 140 chars
	RefuseBadArguments:      "Something was missing from that request. Try again.",                                         // copy: refusal, max 140 chars
	RefuseNoSuchFolder:      "That folder is no longer listed. Add it again if you still want to use it.",                  // copy: refusal, max 140 chars
	RefuseNoSuchProgram:     "That program is no longer listed. Look at the list again.",                                   // copy: refusal, max 140 chars
	RefuseNoSuchDocument:    "That document is no longer listed. Open it again if you still need it.",                      // copy: refusal, max 140 chars
	RefuseFileMissing:       "That file isn't there anymore. It may have been moved or deleted.",                           // copy: refusal, max 140 chars TODO-COPY
	RefuseFileOutsideHome:   "HTML Clay only opens files inside your home folder. Move the file there and open it again.",  // copy: refusal, max 140 chars TODO-COPY
	RefuseNoSuchRequest:     "That request has ended. Try again if you still need it.",                                     // copy: refusal, max 140 chars
	RefuseRequestChanged:    "That request changed while you were answering. Look at it again.",                            // copy: refusal, max 140 chars
	RefuseAnswerNotOffered:  "That answer isn't one this request offers. Choose one of the options shown.",                 // copy: refusal, max 140 chars
	RefuseNotAllowedNow:     "That isn't possible for this folder right now. Check the folder in HTML Clay and try again.", // copy: refusal, max 140 chars
	RefuseUnknownPreference: "HTML Clay doesn't have that setting. Check the setting name or update HTML Clay.",            // copy: refusal, max 140 chars
	RefuseNothingToMove:     "There is nothing to move from Hyperclay Local. Pick a folder to serve instead.",              // copy: refusal, max 140 chars
	RefuseNotConnected:      "Your Hyperclay account isn't connected. Connect it in HTML Clay first.",                      // copy: refusal, max 140 chars
	RefuseNothingReady:      "No update is ready yet. Try again later.",                                                    // copy: refusal, max 140 chars
	RefuseNeedsConfirmation: "This action needs your approval. Approve it in HTML Clay first.",                             // copy: refusal, max 140 chars
	RefuseFailed:            "HTML Clay couldn't do that just now. Try again.",                                             // copy: refusal, max 140 chars
	RefuseCannotTrust:       "HTML Clay can't trust this folder. Pick a folder inside your home folder that isn't hidden.", // copy: refusal, max 140 chars TODO-COPY
	RefuseStillRunning:      "HTML Clay is still doing that. Wait a moment.",                                               // copy: refusal, max 140 chars TODO-COPY
	RefuseNotYet:            "HTML Clay can't do that yet. Update HTML Clay and try again.",                                // copy: refusal, max 140 chars
	// A folder rule's reason, when an action runs into it.
	WhyIdentityChanged:    TextWhyServeIdentityChanged,
	WhyNotOnComputer:      TextWhyNotOnComputer,
	WhyNotFromAccount:     TextWhySyncNotFromAccount,
	WhyConnectFirst:       TextWhySyncConnectFirst,
	WhyServeFirst:         TextWhySyncServeFirst,
	WhyViewer:             TextWhySyncViewer,
	WhyStopServingFirst:   TextWhyStopTrustingServed,
	RefuseServedElsewhere: "Another copy of HTML Clay is serving this folder. Stop it there first.",      // copy: refusal, max 140 chars
	RefuseUnsavedEdits:    "Some edits in this folder haven't been saved yet. Try again in a moment.",    // copy: refusal, max 140 chars
	RefuseQuitUnsaved:     "Some edits haven't been saved yet. Try again, or quit anyway and lose them.", // copy: refusal, max 140 chars
}

// RefusalMessage fills in RefusalText for r.
func RefusalMessage(r *Refusal) string {
	if r == nil {
		return ""
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
