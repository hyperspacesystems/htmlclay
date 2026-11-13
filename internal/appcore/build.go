package appcore

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Build turns facts into the snapshot every shell renders. It is pure: no I/O,
// no clock other than f.Now.
func Build(f Facts) Snapshot {
	s := Snapshot{
		V:     SnapshotVersion,
		Rev:   f.Rev,
		Epoch: f.Epoch,
		Host: HostState{
			Version:      f.Host.Version,
			OS:           f.Host.OS,
			Port:         f.Host.Port,
			DashboardURL: fmt.Sprintf("http://htmlclay.localhost:%d/", f.Host.Port),
			Service:      f.Host.Service,
			Stopped:      f.Host.Stopped,
		},
		Documents:   make([]Document, 0, len(f.Documents)),
		Folders:     make([]Folder, 0, len(f.Folders)),
		Programs:    make([]Program, 0, len(f.Programs)),
		Requests:    make([]Request, 0, len(f.Requests)),
		Waiting:     waiting(f),
		Sync:        syncState(f.Sync),
		Update:      f.Update,
		Preferences: f.Preferences,
		Move:        f.Move,
	}
	for _, d := range f.Documents {
		doc := Document{
			ID:           d.ID,
			Name:         d.Name,
			Place:        displayPath(f.Home, filepath.Dir(d.Path)),
			FolderID:     optionalText(d.FolderID),
			URL:          d.URL,
			OpenNow:      d.OpenNow,
			OpenedAt:     d.OpenedAt,
			FileMissing:  d.FileMissing,
			VersionsKept: d.VersionsKept,
		}
		if folder, ok := folderFact(f.Folders, d.FolderID); ok && folder.Origin.Kind != OriginLocal {
			doc.Place = folder.Name
		}
		s.Documents = append(s.Documents, doc)
	}
	for _, ff := range f.Folders {
		folder := Folder{
			ID:            ff.ID,
			Name:          ff.Name,
			Path:          ff.Path,
			DisplayPath:   displayPath(f.Home, ff.Path),
			Origin:        ff.Origin,
			ParentID:      optionalText(ff.ParentID),
			Trusted:       ff.Trusted,
			Served:        ff.Served,
			Synced:        ff.Synced,
			DocumentCount: ff.DocumentCount,
			StorageBytes:  ff.StorageBytes,
			State:         folderState(ff),
			Sync:          ff.Sync,
		}
		if folder.Sync.State == "" {
			folder.Sync.State = FolderSyncOff
		}
		if ff.Served && ff.ServedName != "" {
			folder.Address = fmt.Sprintf("%s.htmlclay.localhost:%d", ff.ServedName, f.Host.Port)
			folder.URL = "http://" + folder.Address + "/"
		}
		folder.Can, folder.Why = folderRules(f, ff, folder.State)
		s.Folders = append(s.Folders, folder)
	}
	for _, p := range f.Programs {
		program := p
		if program.AllowedDocuments == nil {
			program.AllowedDocuments = []string{}
		}
		if program.RemovedDocuments == nil {
			program.RemovedDocuments = []string{}
		}
		s.Programs = append(s.Programs, program)
	}
	for _, r := range f.Requests {
		s.Requests = append(s.Requests, Request{
			ID:         r.ID,
			Kind:       r.Kind,
			Revision:   r.Revision,
			DocumentID: optionalText(r.DocumentID),
			Document:   r.Document,
			Program:    r.Program,
			Folder:     r.Folder,
			Path:       r.Path,
			Detail:     r.Detail,
			Answers:    AnswersFor(r.Kind),
		})
	}
	s.Counts = counts(s.Waiting)
	return s
}

// folderState is the first state that applies, in the order the shells read them.
func folderState(f FolderFact) string {
	switch {
	case !f.OnThisComputer:
		return FolderNotOnThisComputer
	case f.Missing:
		return FolderMissing
	case f.IdentityChanged:
		return FolderIdentityChanged
	case f.ServedByLocal:
		return FolderServedByLocal
	case f.BoundToLocalSync:
		return FolderBoundToLocalSync
	case f.Viewer:
		return FolderViewer
	default:
		return FolderOK
	}
}

func syncState(f SyncFacts) SyncState {
	return SyncState{
		Connected:      f.Connected,
		Username:       f.Username,
		KeySuffix:      f.KeySuffix,
		Paused:         f.Paused,
		SyncingFiles:   f.SyncingFiles,
		OfflineChanges: f.OfflineChanges,
		ViewOnlyTeams:  f.ViewOnlyTeams,
		LastFinishedAt: f.LastFinishedAt,
		FullName:       f.FullName,
	}
}

// optionalText is a pointer to s, or nil when s is empty.
func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// folderFact finds the folder with id, if there is one.
func folderFact(folders []FolderFact, id string) (FolderFact, bool) {
	for _, folder := range folders {
		if folder.ID == id {
			return folder, true
		}
	}
	return FolderFact{}, false
}

// displayPath writes path the way a person reads it: the home folder and anything
// under it start with ~, other paths stay as they are, and an empty path stays empty.
func displayPath(home, path string) string {
	if path == "" || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + path[len(home):]
	}
	return path
}
