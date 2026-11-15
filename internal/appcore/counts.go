package appcore

import (
	"fmt"
	"path/filepath"
)

// waitingPane and waitingEarnsDot are the preview's waiting kinds
// (macos/Sources/PreviewCore/Model.swift WaitingItem.Kind).
var waitingPane = map[string]string{
	WaitingConflict:           PaneFolders,
	WaitingFolderMissing:      PaneFolders,
	WaitingFileTooBig:         PaneFolders,
	WaitingTeamToSetUp:        PaneFolders,
	WaitingFileNotText:        PaneFolders,
	WaitingNameClash:          PaneFolders,
	WaitingRemovalHeld:        PaneFolders,
	WaitingProgramRequest:     PanePrograms,
	WaitingProgramFileMissing: PanePrograms,
	WaitingSyncKeyRefused:     PaneSync,
	WaitingServiceFailed:      PaneAdvanced,
}

var waitingEarnsDot = map[string]bool{
	WaitingConflict:       true,
	WaitingProgramRequest: true,
	WaitingSyncKeyRefused: true,
	WaitingFolderMissing:  true,
	WaitingServiceFailed:  true,
	WaitingRemovalHeld:    true,
}

// waitingCounts says which kinds the red counts include: only what a person must
// act on and only they can fix (SCREEN-AUDIT.md section 2). A file too big to sync
// and a team folder to set up are listed but not counted.
var waitingCounts = map[string]bool{
	WaitingConflict:           true,
	WaitingProgramRequest:     true,
	WaitingProgramFileMissing: true,
	WaitingSyncKeyRefused:     true,
	WaitingFolderMissing:      true,
	WaitingServiceFailed:      true,
	WaitingRemovalHeld:        true,
}

// countedPanes are the panes that can hold a waiting item; each appears in
// Counts.ByPane even at zero.
var countedPanes = []string{PaneFolders, PanePrograms, PaneSync, PaneAdvanced}

// waiting lists what waits for the person: events (conflicts, a refused key, a file
// too big, a team to set up) in order, then program requests, missing program files,
// missing folders, and a failed service. A refused key is one item however many
// folders reported it.
func waiting(f Facts) []WaitingItem {
	seen := make(map[string]bool, len(f.Seen))
	for _, id := range f.Seen {
		seen[id] = true
	}
	items := []WaitingItem{}
	add := func(id, kind, title, detail, subject string) {
		item := WaitingItem{
			ID: "w-" + id, Kind: kind, Pane: waitingPane[kind], EarnsDot: waitingEarnsDot[kind],
			Title: title, Detail: detail, SubjectID: optionalText(subject),
		}
		item.Seen = seen[item.ID]
		items = append(items, item)
	}

	keyRefused := false
	for _, e := range f.Events {
		switch e.Kind {
		case WaitingConflict:
			add(e.ID, e.Kind, fmt.Sprintf(TextWaitingConflict, e.Name), e.Detail, e.SubjectID)
		case WaitingSyncKeyRefused:
			if !keyRefused {
				keyRefused = true
				add(e.ID, e.Kind, TextWaitingSyncKeyRefused, e.Detail, e.SubjectID)
			}
		case WaitingFileTooBig:
			add(e.ID, e.Kind, fmt.Sprintf(TextWaitingFileTooBig, e.Name), e.Detail, e.SubjectID)
		case WaitingFileNotText:
			add(e.ID, e.Kind, fmt.Sprintf(TextWaitingFileNotText, e.Detail, e.Name), e.Detail, e.SubjectID)
		case WaitingNameClash:
			add(e.ID, e.Kind, fmt.Sprintf(TextWaitingNameClash, e.Detail, e.Name), e.Detail, e.SubjectID)
		case WaitingTeamToSetUp:
			add(e.ID, e.Kind, fmt.Sprintf(TextWaitingTeamToSetUp, e.Name), e.Detail, e.SubjectID)
		case WaitingRemovalHeld:
			add(e.ID, e.Kind, fmt.Sprintf(TextWaitingRemovalHeld, e.Detail, e.Name), e.Detail, e.SubjectID)
		}
	}
	for _, r := range f.Requests {
		document := documentName(r.Document)
		title := fmt.Sprintf(TextWaitingProgramRequest, document, r.Program)
		if r.Program == AIEditProgram {
			title = fmt.Sprintf(TextWaitingAIEditRequest, document)
		}
		switch r.Kind {
		case RequestFolderRead:
			title = fmt.Sprintf(TextWaitingFolderRead, document, r.Folder)
		case RequestFolderReadAgain:
			title = fmt.Sprintf(TextWaitingFolderReadAgain, document, r.Folder)
		}
		add(r.ID, WaitingProgramRequest, title, r.Folder, r.ID)
	}
	for _, p := range f.Programs {
		if p.FileMissing {
			add(p.ID, WaitingProgramFileMissing, fmt.Sprintf(TextWaitingProgramMissing, p.Name), displayPath(f.Home, p.Path), p.ID)
		}
	}
	for _, ff := range f.Folders {
		if folderState(ff) == FolderMissing && ff.Trusted {
			add(ff.ID, WaitingFolderMissing, fmt.Sprintf(TextWaitingFolderMissing, ff.Name), displayPath(f.Home, ff.Path), ff.ID)
		}
	}
	if f.Host.Service == ServiceFailed {
		add("service", WaitingServiceFailed, TextWaitingServiceFailed, "", "")
	}
	return items
}

// documentName is the name a waiting row shows for a document, which the host
// names by its path.
func documentName(document string) string {
	if document == "" {
		return ""
	}
	return filepath.Base(document)
}

// counts is the total, the count per pane (a subset of the total) and the dot: an
// item that earns the dot and the person has not seen yet.
func counts(items []WaitingItem) Counts {
	c := Counts{ByPane: make(map[string]int, len(countedPanes))}
	for _, pane := range countedPanes {
		c.ByPane[pane] = 0
	}
	for _, item := range items {
		if waitingCounts[item.Kind] {
			c.Total++
			c.ByPane[item.Pane]++
		}
		if item.EarnsDot && !item.Seen {
			c.Dot = true
		}
	}
	return c
}
