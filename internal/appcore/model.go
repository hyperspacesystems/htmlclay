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
	// RecoveredChanges is how many copies HTML Clay saved aside when a page could
	// not save normally (the Recovered changes page). 0 hides every menu row for it.
	RecoveredChanges int       `json:"recoveredChanges"`
	FileTypes        FileTypes `json:"fileTypes"`
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
	// Download is a first download's progress while it runs, from the engine's
	// own counts (08-sync.md decision 20); null otherwise.
	Download *SyncDownload `json:"download"`
	// SetUpFailed is why the last first download on this computer failed, for a
	// folder not set up here: the engine's reason ("disk-full", "offline", ...).
	SetUpFailed string `json:"setUpFailed,omitempty"`
}

type SyncDownload struct {
	Done       int   `json:"done"`
	Total      int   `json:"total"`
	BytesDone  int64 `json:"bytesDone"`
	BytesTotal int64 `json:"bytesTotal"`
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
	// Builtin is a program HTML Clay runs itself: it has no file and no path.
	// On is its switch; ai-edit's On is the AI Editing switch.
	Builtin bool `json:"builtin"`
	On      bool `json:"on"`
}

// Request kinds, the preview's six.
const (
	RequestFound           = "found"
	RequestNoFile          = "noFile"
	RequestTeamDocument    = "teamDocument"
	RequestRiskyFile       = "riskyFile"
	RequestFolderRead      = "folderRead"
	RequestFolderReadAgain = "folderReadAgain"
	RequestBuiltinProgram  = "builtinProgram"
)

const AIEditProgram = "ai-edit" // the program name the built-in AI editing asks under

// Request is one question waiting for the person. Attention counts how many
// times somebody asked for it to be shown: the Mac app raises its window when
// the count goes up, and nothing else about the request changes.
type Request struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Revision   uint64   `json:"revision"`
	Attention  uint64   `json:"attention"`
	DocumentID *string  `json:"documentID"`
	Document   string   `json:"document"`
	Program    string   `json:"program"`
	Folder     string   `json:"folder"`
	Path       string   `json:"path"`
	Detail     string   `json:"detail"`
	Answers    []string `json:"answers"`
}

// Waiting kinds: the preview's eight, and a removal sync holds (08-sync.md 8.34).
const (
	WaitingConflict           = "conflict"
	WaitingProgramRequest     = "programRequest"
	WaitingSyncKeyRefused     = "syncKeyRefused"
	WaitingFolderMissing      = "folderMissing"
	WaitingServiceFailed      = "serviceFailed"
	WaitingProgramFileMissing = "programFileMissing"
	WaitingFileTooBig         = "fileTooBig"
	WaitingRemovalHeld        = "removalHeld"
	WaitingTeamToSetUp        = "teamToSetUp"
	WaitingFileNotText        = "fileNotText"
	WaitingNameClash          = "nameClash"
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
	KeyStore       string     `json:"keyStore"`
	// EverOn is true when sync has been on at some point on this computer:
	// sync.json has an account, an account row or a session row. Show sync log
	// appears with it.
	EverOn bool `json:"everOn"`
}

type UpdateState struct {
	Current   string     `json:"current"`
	Ready     *string    `json:"ready"`
	CheckedAt *time.Time `json:"checkedAt"`
	Automatic bool       `json:"automatic"`
	Finishing bool       `json:"finishing"`
	// Checking is true while a check runs (Windows and Linux; the Mac app reads Sparkle).
	Checking bool `json:"checking"`
	// DownloadURL is set when a newer version exists that this build cannot
	// install itself: the update row opens it.
	DownloadURL string `json:"downloadURL"`
	// Downloading is the newer version being downloaded now, or empty.
	Downloading string `json:"downloading"`
	// LastCheck is how the last finished check ended; nil before the first.
	LastCheck *UpdateCheck `json:"lastCheck"`
}

// UpdateCheck is one finished update check: when, and what it found.
type UpdateCheck struct {
	At     time.Time `json:"at"`
	Result string    `json:"result"`
}

// Update check results.
const (
	UpdateLatest = "latest"
	UpdateFound  = "found"
	UpdateFailed = "failed"
)

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
	// AIEditTool is the engine AI editing uses when a request names none: "claude",
	// "codex", or a custom command's name.
	AIEditTool  string       `json:"aiEditTool"`
	AIEditTools []AIEditTool `json:"aiEditTools"`
}

// Move is the offer, and later the progress, of moving from Hyperclay Local.
// Nil means there is nothing to move.
type Move struct {
	State   string       `json:"state"`
	Folders []MoveFolder `json:"folders"`
	// LocalRunning is true while Hyperclay Local is open: nothing moves until it quits.
	LocalRunning bool `json:"localRunning"`
	// NeedsKey is true when a folder offered syncs and this app has no working sync key.
	NeedsKey bool `json:"needsKey"`
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
	// Synced is true when Hyperclay Local syncs the folder.
	Synced bool `json:"synced"`
	// Skip is why the last move left the folder with Hyperclay Local, or empty.
	Skip string `json:"skip,omitempty"`
}

// SyncKeyUnreadable is the detail of a syncKeyRefused item raised because the
// saved key cannot be read, not because the server refused it.
const SyncKeyUnreadable = "unreadable"

// AIEditTool is one choice in the AI editing tool picker.
type AIEditTool struct {
	ID           string   `json:"id"`
	Custom       bool     `json:"custom"`
	Found        bool     `json:"found"`
	Model        string   `json:"model"`
	DefaultModel string   `json:"defaultModel"`
	Models       []string `json:"models"`
	// Effort is the reasoning effort chosen for the tool, "" for HTML Clay's
	// default; Efforts is empty for a tool with no effort setting.
	Effort        string   `json:"effort"`
	DefaultEffort string   `json:"defaultEffort"`
	Efforts       []string `json:"efforts"`
	Command       string   `json:"command"`
}

// FileTypes is what HTML Clay knows about the system's file types. Managed is
// false where the host does not own them (the Mac app reads and sets its own).
type FileTypes struct {
	Managed bool `json:"managed"`
	// Default is "htmlclay" when HTML Clay opens .htmlclay files, "other" when
	// another app does, and "unknown" when the system cannot say.
	Default string `json:"default"`
	// HTMLOpenWith is HTML Clay being offered in Open With for .html files.
	HTMLOpenWith bool `json:"htmlOpenWith"`
	// Stopped is true after Stop opening .htmlclay files.
	Stopped bool `json:"stopped"`
}

// FileTypes.Default values.
const (
	DefaultAppHTMLClay = "htmlclay"
	DefaultAppOther    = "other"
	DefaultAppUnknown  = "unknown"
)
