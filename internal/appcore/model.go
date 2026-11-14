package appcore

import "time"

// SnapshotVersion is the wire format's version. A shell refuses a snapshot whose v
// it does not know.
const SnapshotVersion = 1

// Snapshot is everything a shell shows: the Mac app, the trays and the dashboard
// all render it and hold no rule of their own. Times are RFC 3339 and sizes are
// bytes; every sentence comes from copy.go.
type Snapshot struct {
	V           int           `json:"v"`
	Rev         uint64        `json:"rev"`
	Epoch       string        `json:"epoch"`
	Host        HostState     `json:"host"`
	Documents   []Document    `json:"documents"`
	Folders     []Folder      `json:"folders"`
	Programs    []Program     `json:"programs"`
	Requests    []Request     `json:"requests"`
	Waiting     []WaitingItem `json:"waiting"`
	Counts      Counts        `json:"counts"`
	Sync        SyncState     `json:"sync"`
	Update      UpdateState   `json:"update"`
	Preferences Preferences   `json:"preferences"`
	Move        *Move         `json:"move"`
}

// Service states.
const (
	ServiceOff        = "off"
	ServiceStarting   = "starting"
	ServiceRunning    = "running"
	ServiceRestarting = "restarting"
	ServiceFailed     = "failed"
)

type HostState struct {
	Version      string `json:"version"`
	OS           string `json:"os"`
	Port         int    `json:"port"`
	DashboardURL string `json:"dashboardURL"`
	Service      string `json:"service"`
	Stopped      bool   `json:"stopped"`

	// Can is every action type, true when this host runs it now.
	Can map[string]bool `json:"can"`
}

type Document struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Place        string     `json:"place"`
	FolderID     *string    `json:"folderID"`
	URL          string     `json:"url"`
	OpenNow      bool       `json:"openNow"`
	OpenedAt     *time.Time `json:"openedAt"`
	FileMissing  bool       `json:"fileMissing"`
	VersionsKept int        `json:"versionsKept"`
}

// Folder origins.
const (
	OriginLocal   = "local"
	OriginAccount = "account"
	OriginTeam    = "team"
)

type Origin struct {
	Kind     string `json:"kind"`
	Username string `json:"username,omitempty"`
	Team     string `json:"team,omitempty"`
}

// Folder states, in the order Build checks them.
const (
	FolderNotOnThisComputer = "not-on-this-computer"
	FolderMissing           = "missing"
	FolderIdentityChanged   = "identity-changed"
	FolderServedByLocal     = "served-by-local"
	FolderBoundToLocalSync  = "bound-to-local-sync"
	FolderViewer            = "viewer"
	FolderOK                = "ok"
)

type Folder struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Path          string     `json:"path"`
	DisplayPath   string     `json:"displayPath"`
	Origin        Origin     `json:"origin"`
	ParentID      *string    `json:"parentID"`
	Trusted       bool       `json:"trusted"`
	Served        bool       `json:"served"`
	AllowsFraming bool       `json:"allowsFraming"`
	Synced        bool       `json:"synced"`
	Address       string     `json:"address"`
	URL           string     `json:"url"`
	State         string     `json:"state"`
	DocumentCount int        `json:"documentCount"`
	StorageBytes  int64      `json:"storageBytes"`
	Sync          FolderSync `json:"sync"`
	Can           FolderCan  `json:"can"`
	Why           FolderWhy  `json:"why"`
}

// Folder sync states.
const (
	FolderSyncOff       = "off"
	FolderSyncUpToDate  = "up-to-date"
	FolderSyncSyncing   = "syncing"
	FolderSyncConflicts = "conflicts"
	FolderSyncPaused    = "paused"
)

type FolderSync struct {
	State           string     `json:"state"`
	Files           int        `json:"files"`
	LastFinishedAt  *time.Time `json:"lastFinishedAt"`
	UploadedToday   int        `json:"uploadedToday"`
	DownloadedToday int        `json:"downloadedToday"`
	Role            string     `json:"role,omitempty"`
}

// FolderCan is what a person may do to a folder now. A shell renders a control
// from it and never decides on its own.
type FolderCan struct {
	Serve         bool `json:"serve"`
	StopServing   bool `json:"stopServing"`
	SetFraming    bool `json:"setFraming"`
	Sync          bool `json:"sync"`
	StopSyncing   bool `json:"stopSyncing"`
	StopTrusting  bool `json:"stopTrusting"`
	Merge         bool `json:"merge"`
	OpenWorkspace bool `json:"openWorkspace"`
}

// FolderWhy says why a FolderCan entry is false, for the ones a person might
// expect to be true. Absent means no explanation is shown.
type FolderWhy struct {
	Serve        *Reason `json:"serve,omitempty"`
	Sync         *Reason `json:"sync,omitempty"`
	StopTrusting *Reason `json:"stopTrusting,omitempty"`
	Merge        *Reason `json:"merge,omitempty"`
}

// Reason is a code a program can act on and the sentence a person reads.
type Reason struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

type Program struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Path              string   `json:"path"`
	FileMissing       bool     `json:"fileMissing"`
	AllowedDocuments  []string `json:"allowedDocuments"`
	AllTrustedFolders bool     `json:"allTrustedFolders"`
	RemovedDocuments  []string `json:"removedDocuments"`
}

// Request kinds, the preview's six.
const (
	RequestFound           = "found"
	RequestNoFile          = "noFile"
	RequestTeamDocument    = "teamDocument"
	RequestRiskyFile       = "riskyFile"
	RequestFolderRead      = "folderRead"
	RequestFolderReadAgain = "folderReadAgain"
)

type Request struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Revision   uint64   `json:"revision"`
	DocumentID *string  `json:"documentID"`
	Document   string   `json:"document"`
	Program    string   `json:"program"`
	Folder     string   `json:"folder"`
	Path       string   `json:"path"`
	Detail     string   `json:"detail"`
	Answers    []string `json:"answers"`
}

// Waiting kinds, the preview's eight.
const (
	WaitingConflict           = "conflict"
	WaitingProgramRequest     = "programRequest"
	WaitingSyncKeyRefused     = "syncKeyRefused"
	WaitingFolderMissing      = "folderMissing"
	WaitingServiceFailed      = "serviceFailed"
	WaitingProgramFileMissing = "programFileMissing"
	WaitingFileTooBig         = "fileTooBig"
	WaitingTeamToSetUp        = "teamToSetUp"
)

// Settings panes a waiting item belongs to.
const (
	PaneGeneral  = "general"
	PaneFolders  = "folders"
	PanePrograms = "programs"
	PaneSync     = "sync"
	PaneYourName = "yourName"
	PaneUpdates  = "updates"
	PaneAdvanced = "advanced"
)

type WaitingItem struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Pane      string  `json:"pane"`
	EarnsDot  bool    `json:"earnsDot"`
	Seen      bool    `json:"seen"`
	Title     string  `json:"title"`
	Detail    string  `json:"detail"`
	SubjectID *string `json:"subjectID"`
}

type Counts struct {
	Total  int            `json:"total"`
	ByPane map[string]int `json:"byPane"`
	Dot    bool           `json:"dot"`
}

type SyncState struct {
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

type UpdateState struct {
	Current   string     `json:"current"`
	Ready     *string    `json:"ready"`
	CheckedAt *time.Time `json:"checkedAt"`
	Automatic bool       `json:"automatic"`
	Finishing bool       `json:"finishing"`
}

// Appearances.
const (
	AppearanceSystem = "system"
	AppearanceLight  = "light"
	AppearanceDark   = "dark"
)

type Preferences struct {
	OpenAtLogin       bool   `json:"openAtLogin"`
	OpenAtLoginChosen bool   `json:"openAtLoginChosen"`
	AIEditing         bool   `json:"aiEditing"`
	Appearance        string `json:"appearance"`
	YourName          string `json:"yourName"`
	UseName           bool   `json:"useName"`
}

// Move is the offer, and later the progress, of moving from Hyperclay Local.
// Nil means there is nothing to move.
type Move struct {
	State   string       `json:"state"`
	Folders []MoveFolder `json:"folders"`
}

// Move states.
const (
	MoveOffered = "offered"
	MoveRunning = "running"
	MoveDone    = "done"
)

type MoveFolder struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Address string `json:"address"`
}
