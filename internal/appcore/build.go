package appcore

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// ServedParent is the name every served folder sits under: <name>.htmlclay.localhost.
const ServedParent = "htmlclay.localhost"

// DashboardHost is the dashboard's host name: the served folders' parent itself,
// so its frames of served documents are same-site. No document site answers
// there, because config.AllocateSiteSlug numbers them from htmlclay1.
const DashboardHost = ServedParent

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
			DashboardURL: fmt.Sprintf("http://%s:%d/", DashboardHost, f.Host.Port),
			Service:      f.Host.Service,
			Stopped:      f.Host.Stopped,
			Can:          hostCan(f.Host.Implemented),
		},
		Documents:   make([]Document, 0, len(f.Documents)),
		Folders:     make([]Folder, 0, len(f.Folders)),
		Programs:    make([]Program, 0, len(f.Programs)),
		ReadAccess:  make([]ReadAccess, 0, len(f.ReadAccess)),
		Requests:    make([]Request, 0, len(f.Requests)),
		Waiting:     waiting(f),
		Sync:        syncState(f.Sync),
		Update:      f.Update,
		Preferences: f.Preferences,
		Move:        f.Move,

		RecoveredChanges: f.RecoveredChanges,
		FileTypes:        f.FileTypes,
		Offline:          f.Offline,
	}
	if len(f.DeniedDocuments) > 0 {
		s.DeniedDocuments = slices.Clone(f.DeniedDocuments)
		slices.Sort(s.DeniedDocuments)
	}
	tools := make([]AIEditTool, 0, len(f.Preferences.AIEditTools))
	for _, t := range f.Preferences.AIEditTools {
		if t.Efforts == nil {
			t.Efforts = []string{}
		}
		if len(t.ModelEfforts) > 0 {
			me := make(map[string][]string, len(t.ModelEfforts))
			for k, v := range t.ModelEfforts {
				me[k] = slices.Clone(v)
			}
			t.ModelEfforts = me
		}
		tools = append(tools, t)
	}
	s.Preferences.AIEditTools = tools
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
			AllowsFraming: ff.AllowsFraming,
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
			folder.Address = fmt.Sprintf("%s.%s:%d", ff.ServedName, ServedParent, f.Host.Port)
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
	for _, r := range f.ReadAccess {
		access := r
		access.Place = displayPath(f.Home, access.Folder)
		if access.Pages == nil {
			access.Pages = []string{}
		}
		s.ReadAccess = append(s.ReadAccess, access)
	}
	for _, r := range f.Requests {
		answers := AnswersFor(r.Kind)
		if len(r.Answers) > 0 {
			answers = slices.Clone(r.Answers)
		}
		s.Requests = append(s.Requests, Request{
			ID:         r.ID,
			Kind:       r.Kind,
			Revision:   r.Revision,
			Attention:  r.Attention,
			DocumentID: optionalText(r.DocumentID),
			Document:   r.Document,
			Program:    r.Program,
			Folder:     r.Folder,
			Path:       r.Path,
			Detail:     r.Detail,
			Answers:    answers,
		})
	}
	s.Counts = counts(s.Waiting)
	return s
}

// hostCan answers every action type the core knows: true when the host runs it.
func hostCan(implemented []string) map[string]bool {
	can := make(map[string]bool, len(ActionClasses))
	for typ := range ActionClasses {
		can[typ] = false
	}
	for _, typ := range implemented {
		if _, known := ActionClasses[typ]; known {
			can[typ] = true
		}
	}
	return can
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
		KeyStore:       f.KeyStore,
		EverOn:         f.EverOn,
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
