package appcore

import (
	"fmt"
	"strings"
)

// Text people read. Every value is working text until the copywriter replaces it;
// each TODO-COPY says where it shows and how long it may be.

const (
	TextServeAFolder       = "Serve a Folder…"                                                           // TODO-COPY: tray row that opens the folder picker, max 24 chars
	TextServedFolders      = "Served Folders"                                                            // TODO-COPY: tray submenu title, max 24 chars
	TextNoServedFolders    = "No served folders yet"                                                     // TODO-COPY: tray, disabled row when the list is empty, max 32 chars
	TextOpenServed         = "Open Workspace"                                                            // TODO-COPY: tray, per folder, max 24 chars
	TextCopyAddress        = "Copy Address"                                                              // TODO-COPY: tray, per folder, max 24 chars
	TextStopServing        = "Stop Serving"                                                              // TODO-COPY: tray, per folder, and the confirm button, max 24 chars
	TextFolderMissing      = "(missing)"                                                                 // TODO-COPY: tray, after a served folder's name when its folder is gone, max 16 chars
	TextPickServeFolder    = "Choose a folder to serve at its own address."                              // TODO-COPY: folder picker prompt, max 120 chars
	TextStopServingTitle   = "Stop serving this folder?"                                                 // TODO-COPY: confirm dialog title, max 40 chars
	TextStopServingMessage = "%s will stop working. The files in the folder stay where they are."        // TODO-COPY: confirm dialog body, %s is the address, max 160 chars
	TextNowServed          = "%s is served at %s"                                                        // TODO-COPY: notification and CLI success line, %s name then address, max 120 chars
	TextNotServed          = "No served folder is named %s."                                             // TODO-COPY: CLI, stop-serving with an unknown name, max 80 chars
	TextStopped            = "Stopped serving %s."                                                       // TODO-COPY: CLI success line, max 80 chars
	TextCannotServe        = "HTML Clay can't serve this folder"                                         // TODO-COPY: notification title for a refusal, max 48 chars
	TextServeCLIWindows    = "htmlclay serve runs on macOS and Linux; on Windows use the HTML Clay menu" // TODO-COPY: CLI, on Windows, max 120 chars
)

// Why texts: the footnote or tooltip beside a control that is off.
const (
	TextWhyServeMissing          = "This folder can't be found where it was."           // TODO-COPY: why Serve is off, max 90 chars
	TextWhyServeIdentityChanged  = "A different folder now sits where this one was."    // TODO-COPY: why Serve is off, max 90 chars
	TextWhyServeServedByLocal    = "Hyperclay Local is serving this folder now."        // TODO-COPY: why Serve is off, max 90 chars
	TextWhyServeBoundToLocalSync = "This folder syncs with Hyperclay Local."            // TODO-COPY: why Serve is off, max 90 chars
	TextWhyNotOnComputer         = "This team folder isn't on this computer yet."       // TODO-COPY: why Serve or Sync is off, max 90 chars
	TextWhySyncNotFromAccount    = "Only folders from your Hyperclay account can sync." // TODO-COPY: why Sync is off, max 90 chars
	TextWhySyncConnectFirst      = "Connect your Hyperclay account first."              // TODO-COPY: why Sync is off, max 90 chars
	TextWhySyncServeFirst        = "Serve this folder first."                           // TODO-COPY: why Sync is off, max 90 chars
	TextWhySyncViewer            = "You can view this team folder but not change it."   // TODO-COPY: why Sync is off, max 90 chars
	TextWhySyncMissing           = "This folder can't be found where it was."           // TODO-COPY: why Sync is off, max 90 chars
	TextWhyStopTrustingServed    = "Stop serving this folder first."                    // TODO-COPY: why Stop Trusting is off, max 90 chars
)

// Waiting items: the title, and where a %s appears the subject's name.
const (
	TextWaitingProgramRequest  = "%s wants to run %s"                // TODO-COPY: waiting item title, document then program, max 70 chars
	TextWaitingFolderRead      = "%s wants to read %s"               // TODO-COPY: waiting item title, document then folder, max 70 chars
	TextWaitingFolderReadAgain = "%s wants you to trust %s"          // TODO-COPY: waiting item title, document then folder, max 70 chars
	TextWaitingProgramMissing  = "%s can't be found"                 // TODO-COPY: waiting item title, the program's name, max 70 chars
	TextWaitingFolderMissing   = "%s can't be found"                 // TODO-COPY: waiting item title, the folder's name, max 70 chars
	TextWaitingServiceFailed   = "HTML Clay's live features stopped" // TODO-COPY: waiting item title when the service failed, max 70 chars
	TextWaitingConflict        = "%s changed in both places"         // TODO-COPY: waiting item title, the file's path in the folder, max 70 chars
	TextWaitingSyncKeyRefused  = "Your sync key was refused"         // TODO-COPY: waiting item title, max 70 chars
	TextWaitingFileTooBig      = "%s is too big to sync"             // TODO-COPY: waiting item title, the file's name, max 70 chars
	TextWaitingTeamToSetUp     = "%s is ready to set up"             // TODO-COPY: waiting item title, the team folder's name, max 70 chars
)

// RefusalText is the one-line "what happened, and what to do" for each refusal code.
// %s, where present, is the other folder (OtherPath, or OtherName when set).
var RefusalText = map[string]string{
	RefuseNotAFolder:        "That is not a folder. Pick a folder instead.",                                   // TODO-COPY: refusal, max 140 chars
	RefuseOutsideHome:       "Only folders inside your home folder can be served.",                            // TODO-COPY: refusal, max 140 chars
	RefuseIsHome:            "Your whole home folder can't be served. Pick a folder inside it.",               // TODO-COPY: refusal, max 140 chars
	RefuseProtectedFolder:   "HTML Clay keeps its own files in this folder, so it can't be served.",           // TODO-COPY: refusal, max 140 chars
	RefuseOverlapsServed:    "This folder overlaps %s, which is already served. Stop serving that one first.", // TODO-COPY: refusal, max 140 chars
	RefuseServedByLocal:     "Hyperclay Local is serving this folder now. Stop it there first.",               // TODO-COPY: refusal, max 140 chars
	RefuseBoundToLocalSync:  "This folder syncs with Hyperclay Local. Disconnect it there first.",             // TODO-COPY: refusal, max 140 chars
	RefuseNestedTrusted:     "This folder overlaps the trusted folder %s. Stop trusting one of them first.",   // TODO-COPY: refusal, max 140 chars
	RefuseNameInvalid:       "That name can't be used. Use lowercase letters, numbers and hyphens.",           // TODO-COPY: refusal, max 140 chars
	RefuseNameTaken:         "Another served folder already uses that name. Pick another.",                    // TODO-COPY: refusal, max 140 chars
	RefuseNameRetired:       "That name was used before and can't be used again. Pick another.",               // TODO-COPY: refusal, max 140 chars
	RefuseUnknownAction:     "HTML Clay doesn't know that action. Update HTML Clay and try again.",            // TODO-COPY: refusal, max 140 chars
	RefuseBadArguments:      "Something was missing from that request. Try again.",                            // TODO-COPY: refusal, max 140 chars
	RefuseNoSuchFolder:      "That folder is no longer listed.",                                               // TODO-COPY: refusal, max 140 chars
	RefuseNoSuchProgram:     "That program is no longer listed.",                                              // TODO-COPY: refusal, max 140 chars
	RefuseNoSuchDocument:    "That document is no longer listed.",                                             // TODO-COPY: refusal, max 140 chars
	RefuseNoSuchRequest:     "That request has ended.",                                                        // TODO-COPY: refusal, max 140 chars
	RefuseRequestChanged:    "That request changed while you were answering. Look at it again.",               // TODO-COPY: refusal, max 140 chars
	RefuseAnswerNotOffered:  "That answer isn't one this request offers.",                                     // TODO-COPY: refusal, max 140 chars
	RefuseNotAllowedNow:     "That isn't possible for this folder right now.",                                 // TODO-COPY: refusal, max 140 chars
	RefuseUnknownPreference: "HTML Clay doesn't have that setting.",                                           // TODO-COPY: refusal, max 140 chars
	RefuseNothingToMove:     "There is nothing to move from Hyperclay Local.",                                 // TODO-COPY: refusal, max 140 chars
	RefuseNotConnected:      "Connect your Hyperclay account first.",                                          // TODO-COPY: refusal, max 140 chars
	RefuseNothingReady:      "No update is ready yet.",                                                        // TODO-COPY: refusal, max 140 chars
	RefuseNeedsConfirmation: "Approve this in HTML Clay first.",                                               // TODO-COPY: refusal, max 140 chars
	RefuseFailed:            "HTML Clay couldn\u0027t do that just now. Try again.",                           // TODO-COPY: refusal, max 140 chars
	RefuseNotYet:            "HTML Clay can\u0027t do that yet. Update HTML Clay and try again.",              // TODO-COPY: refusal, max 140 chars
	// A folder rule's reason, when an action runs into it.
	WhyIdentityChanged:    TextWhyServeIdentityChanged,
	WhyNotOnComputer:      TextWhyNotOnComputer,
	WhyNotFromAccount:     TextWhySyncNotFromAccount,
	WhyConnectFirst:       TextWhySyncConnectFirst,
	WhyServeFirst:         TextWhySyncServeFirst,
	WhyViewer:             TextWhySyncViewer,
	WhyStopServingFirst:   TextWhyStopTrustingServed,
	RefuseServedElsewhere: "Another copy of HTML Clay is serving this folder. Stop it there first.",        // TODO-COPY: refusal, max 140 chars
	RefuseUnsavedEdits:    "Some edits in this folder haven\u0027t been saved yet. Try again in a moment.", // TODO-COPY: refusal, max 140 chars
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
