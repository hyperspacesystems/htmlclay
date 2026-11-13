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
	TextOpenServed         = "Open in Browser"                                                           // TODO-COPY: tray, per folder; becomes Open Workspace in SP5, max 24 chars
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

// RefusalText is the one-line "what happened, and what to do" for each refusal code.
// %s, where present, is the other folder (OtherPath, or OtherName when set).
var RefusalText = map[string]string{
	RefuseNotAFolder:       "That is not a folder. Pick a folder instead.",                                   // TODO-COPY: refusal, max 140 chars
	RefuseOutsideHome:      "Only folders inside your home folder can be served.",                            // TODO-COPY: refusal, max 140 chars
	RefuseIsHome:           "Your whole home folder can't be served. Pick a folder inside it.",               // TODO-COPY: refusal, max 140 chars
	RefuseProtectedFolder:  "HTML Clay keeps its own files in this folder, so it can't be served.",           // TODO-COPY: refusal, max 140 chars
	RefuseOverlapsServed:   "This folder overlaps %s, which is already served. Stop serving that one first.", // TODO-COPY: refusal, max 140 chars
	RefuseServedByLocal:    "Hyperclay Local is serving this folder now. Stop it there first.",               // TODO-COPY: refusal, max 140 chars
	RefuseBoundToLocalSync: "This folder syncs with Hyperclay Local. Disconnect it there first.",             // TODO-COPY: refusal, max 140 chars
	RefuseNestedTrusted:    "This folder overlaps the trusted folder %s. Stop trusting one of them first.",   // TODO-COPY: refusal, max 140 chars
	RefuseNameInvalid:      "That name can't be used. Use lowercase letters, numbers and hyphens.",           // TODO-COPY: refusal, max 140 chars
	RefuseNameTaken:        "Another served folder already uses that name. Pick another.",                    // TODO-COPY: refusal, max 140 chars
	RefuseNameRetired:      "That name was used before and can't be used again. Pick another.",               // TODO-COPY: refusal, max 140 chars
	RefuseServedElsewhere:  "Another copy of HTML Clay is serving this folder. Stop it there first.",         // TODO-COPY: refusal, max 140 chars
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
