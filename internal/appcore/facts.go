package appcore

import "time"

// Facts is everything the rules read, gathered by the app (cmd/htmlclay) from
// config, live sites, the served host, the service and the clock. It holds raw
// state only: no sentence, no count, no can or why. Build turns it into a
// Snapshot, and Plan decides actions against it. A fixture is a Facts in JSON.
type Facts struct {
	Now   time.Time `json:"now"`
	Rev   uint64    `json:"rev"`
	Epoch string    `json:"epoch"`
	Home  string    `json:"home"`

	Host        HostFacts      `json:"host"`
	Documents   []DocumentFact `json:"documents"`
	Folders     []FolderFact   `json:"folders"`
	Programs    []Program      `json:"programs"`
	Requests    []RequestFact  `json:"requests"`
	Events      []EventFact    `json:"events"`
	Seen        []string       `json:"seen"`
	Sync        SyncFacts      `json:"sync"`
	Update      UpdateState    `json:"update"`
	Preferences Preferences    `json:"preferences"`
	Move        *Move          `json:"move"`
}

type HostFacts struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Port    int    `json:"port"`
	Service string `json:"service"`
	Stopped bool   `json:"stopped"`

	// Implemented is every action type this host runs; Build marks the rest false in host.can.
	Implemented []string `json:"implemented"`
}

type DocumentFact struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	FolderID     string     `json:"folderID"`
	URL          string     `json:"url"`
	OpenNow      bool       `json:"openNow"`
	OpenedAt     *time.Time `json:"openedAt"`
	FileMissing  bool       `json:"fileMissing"`
	VersionsKept int        `json:"versionsKept"`
}

// FolderFact is one folder the app knows: trusted, served, synced from the
// account, or a team folder that is not on this computer.
type FolderFact struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Origin   Origin `json:"origin"`
	ParentID string `json:"parentID"`

	Trusted    bool   `json:"trusted"`
	Served     bool   `json:"served"`
	ServedName string `json:"servedName"`
	Synced     bool   `json:"synced"`

	OnThisComputer   bool `json:"onThisComputer"`
	Missing          bool `json:"missing"`
	IdentityChanged  bool `json:"identityChanged"`
	ServedByLocal    bool `json:"servedByLocal"`
	BoundToLocalSync bool `json:"boundToLocalSync"`
	Viewer           bool `json:"viewer"`

	DocumentCount int        `json:"documentCount"`
	StorageBytes  int64      `json:"storageBytes"`
	Sync          FolderSync `json:"sync"`
}

type RequestFact struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Revision   uint64 `json:"revision"`
	DocumentID string `json:"documentID"`
	Document   string `json:"document"`
	Program    string `json:"program"`
	Folder     string `json:"folder"`
	Path       string `json:"path"`
	Detail     string `json:"detail"`
}

// EventFact is something that happened and waits for the person, other than what
// Build can see in the state itself (requests, missing folders, missing program
// files, a failed service): a sync conflict, a refused sync key, a file too big
// to sync, a team to set up. Name and Detail fill the waiting item's sentence.
type EventFact struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	SubjectID string `json:"subjectID"`
	Name      string `json:"name"`
	Detail    string `json:"detail"`
}

type SyncFacts struct {
	Connected      bool       `json:"connected"`
	Username       string     `json:"username"`
	KeySuffix      string     `json:"keySuffix"`
	Paused         bool       `json:"paused"`
	SyncingFiles   int        `json:"syncingFiles"`
	OfflineChanges int        `json:"offlineChanges"`
	ViewOnlyTeams  int        `json:"viewOnlyTeams"`
	LastFinishedAt *time.Time `json:"lastFinishedAt"`
	FullName       string     `json:"fullName"`
}
