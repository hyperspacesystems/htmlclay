package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SyncVersion is the only sync.json shape this build reads or writes.
const SyncVersion = 1

// syncNotifiedCap keeps the newest notice ids, so a held conflict is not
// notified again after a restart and the list cannot grow without bound.
const syncNotifiedCap = 500

// ErrSyncVersion refuses a sync.json written by a newer build. The file is left
// exactly as it is.
var ErrSyncVersion = errors.New("sync.json was written by a newer HTML Clay")

// Sync is sync.json: what the person chose about syncing. It sits beside
// config.json, as team-folders.json does, because an older build rewrites
// config.json from a struct that does not know these fields. The sync key is
// never here: it lives in the system's secret store (internal/secret). The
// session rows keep Hyperclay Local's field names so milestone 5 can copy them.
type Sync struct {
	V            int              `json:"v"`
	DeviceID     string           `json:"deviceId"`
	Account      *SyncAccount     `json:"account"`
	Paused       bool             `json:"paused"`
	KeyRefused   *SyncKeyRefused  `json:"keyRefused"`
	Accounts     []SyncAccountRow `json:"accounts"`
	SyncSessions []SyncSession    `json:"syncSessions"`
	Notified     []string         `json:"notified"`

	path string
}

// SyncAccount is the connected account. KeySuffix is the key's last four
// characters, so a screen can tell two keys apart without holding either.
type SyncAccount struct {
	ActorID     int64  `json:"actorId"`
	Username    string `json:"username"`
	FullName    string `json:"fullName"`
	KeySuffix   string `json:"keySuffix"`
	KeyStore    string `json:"keyStore"`
	ConnectedAt string `json:"connectedAt"`
}

// SyncKeyRefused is set when the server stopped accepting the key.
type SyncKeyRefused struct {
	Since string `json:"since"`
}

// SyncAccountRow is one account or team from the last discovery answer.
// FolderID is given the first time the row is seen and kept after.
type SyncAccountRow struct {
	AccountID   int64           `json:"accountId"`
	Kind        string          `json:"kind"`
	Username    string          `json:"username"`
	DisplayName string          `json:"displayName"`
	Role        string          `json:"role"`
	FolderID    string          `json:"folderId"`
	Sync        SyncAccountSync `json:"sync"`
	WebURL      *string         `json:"webUrl"`
}

type SyncAccountSync struct {
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason"`
}

// SyncSession is one account synced into one folder. The first seven fields are
// Hyperclay Local's settings.json syncSessions row; Enabled false is "Stop
// syncing this folder", which keeps the row so the folder's marker stays known.
type SyncSession struct {
	ID            string            `json:"id"`
	RootID        string            `json:"rootId"`
	AccountID     int64             `json:"accountId"`
	Kind          string            `json:"kind"`
	Cached        SyncSessionCached `json:"cached"`
	Paused        *SyncPause        `json:"paused"`
	LegacyMetaDir *string           `json:"legacyMetaDir"`
	Enabled       bool              `json:"enabled"`
	// OffAtDisconnect marks a row that was syncing when the person disconnected,
	// so connecting an account that can reach it again turns it back on.
	OffAtDisconnect bool `json:"offAtDisconnect,omitempty"`
}

type SyncSessionCached struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type SyncPause struct {
	Reason string `json:"reason"`
	Since  string `json:"since"`
}

func syncPath(dir string) string { return filepath.Join(dir, "sync.json") }

// LoadSync reads dir's sync.json. A missing file is a new, unconnected record
// with a fresh device id, not yet written. A file that does not parse, or that a
// newer build wrote, is an error and is never rewritten here.
func LoadSync(dir string) (*Sync, error) {
	path := syncPath(dir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		id, err := newDeviceID()
		if err != nil {
			return nil, err
		}
		return &Sync{V: SyncVersion, DeviceID: id, path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	var probe struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("sync.json: %w", err)
	}
	if probe.V != SyncVersion {
		return nil, fmt.Errorf("%w (v %d)", ErrSyncVersion, probe.V)
	}
	s := &Sync{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("sync.json: %w", err)
	}
	s.path = path
	return s, nil
}

// Save writes the record through a temporary file and a rename, readable only by
// this user.
func (s *Sync) Save() error {
	if s.path == "" {
		return errors.New("sync.json: Save on a record that was not loaded")
	}
	if len(s.Notified) > syncNotifiedCap {
		s.Notified = append([]string(nil), s.Notified[len(s.Notified)-syncNotifiedCap:]...)
	}
	if s.Accounts == nil {
		s.Accounts = []SyncAccountRow{}
	}
	if s.SyncSessions == nil {
		s.SyncSessions = []SyncSession{}
	}
	if s.Notified == nil {
		s.Notified = []string{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sync-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// newDeviceID answers a random version 4 UUID, the shape Local's deviceId has.
func newDeviceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
