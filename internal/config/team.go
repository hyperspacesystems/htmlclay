package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/pathrel"
)

// teamFolderCap bounds the team record. A team folder is a standing grant over a
// whole tree and nothing ever removes an entry, so the list has to have a ceiling.
const teamFolderCap = 512

// The kinds an entry can have. A team folder is one a team shares, so what its
// folders hold is the team's and a program decision inside it may reach the
// team. An account folder is the person's own and never takes a broad grant.
const (
	TeamFolderKindTeam    = "team"
	TeamFolderKindAccount = "account"
)

// The three answers ScopeOf gives. Unknown is not "local": it means the record of
// which folders are team folders cannot be read, so nothing may be granted broad.
const (
	ScopeLocal   = "local"
	ScopeTeam    = "team"
	ScopeUnknown = "unknown"
)

// ErrTeamFolderCap is returned when the team record is full.
var ErrTeamFolderCap = errors.New("team folder record is full")

// TeamFolder is one folder recorded as a team folder. Path is where it was when it
// was recorded and Identity is the folder's fingerprint then, so the record
// survives the folder being renamed, moved, or disconnected. The record is its own
// file, team-folders.json, because an older build rewrites config.json from a
// struct that does not know these fields and would drop them.
type TeamFolder struct {
	FolderID string `json:"folderID,omitempty"`
	Path     string `json:"path"`
	Identity string `json:"identity,omitempty"`
	// Kind is TeamFolderKindTeam or TeamFolderKindAccount.
	Kind string `json:"kind"`
	// Key identifies the team, and is what a Scope key is built from.
	Key string `json:"key,omitempty"`
	// Since is when the folder was first recorded, as an RFC 3339 time.
	Since string `json:"since,omitempty"`
}

// Scope is what a program decision was decided about: one folder on this machine
// (local), one team (team, keyed team:<key>), or a machine whose team record cannot
// be read (unknown). A decision is never widened by a later one.
type Scope struct {
	Kind string `json:"kind,omitempty"`
	Key  string `json:"key,omitempty"`
}

type teamFolderFile struct {
	Folders []TeamFolder `json:"folders"`
}

type teamRecordState int

const (
	// teamRecordOK is a record that parsed: it is the truth, even when empty.
	teamRecordOK teamRecordState = iota
	// teamRecordMissing is a record that is not there. Whether that reads as no
	// team folders or as unknown depends on config.json's marker.
	teamRecordMissing
	// teamRecordBroken is a record that is there and did not parse. It is never
	// overwritten or repaired here: the bytes stay exactly as they are so the
	// person can put them back.
	teamRecordBroken
)

// teamFoldersPath is the team record's file, beside config.json.
func (c *Config) teamFoldersPath() string {
	return filepath.Join(DirFrom(c.baseDir), "team-folders.json")
}

// readTeamFolders reads the record without creating, repairing, or rewriting it.
func (c *Config) readTeamFolders() ([]TeamFolder, teamRecordState) {
	data, err := os.ReadFile(c.teamFoldersPath())
	if os.IsNotExist(err) {
		return nil, teamRecordMissing
	}
	if err != nil {
		return nil, teamRecordBroken
	}
	var file teamFolderFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, teamRecordBroken
	}
	return file.Folders, teamRecordOK
}

// teamRecordMarked reports whether config.json says the team record has had an
// entry. The caller does not hold c.mu.
func (c *Config) teamRecordMarked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.TeamRecord
}

// MarkTeamFolder records entry as a team folder, replacing the entry for the same
// path if there is one. The first entry written also sets teamRecord in
// config.json, so a record that later goes missing reads as unknown rather than as
// a machine that never had a team folder. Nothing removes an entry: a folder that
// is disconnected, removed, or renamed stays in the record.
func (c *Config) MarkTeamFolder(entry TeamFolder) error {
	if entry.Path == "" {
		return errors.New("team folder entry has no path")
	}
	if entry.Since == "" {
		entry.Since = time.Now().UTC().Format(time.RFC3339)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	folders, state := c.readTeamFolders()
	if state == teamRecordBroken {
		// The entry cannot be added without discarding what the person's record
		// already says, which is the one thing this record exists to keep.
		return fmt.Errorf("team folder record %s does not parse", c.teamFoldersPath())
	}

	replaced := false
	for i := range folders {
		if folders[i].Path != entry.Path {
			continue
		}
		// Since is when the folder was first recorded, so a later mark of the same
		// path keeps it.
		if folders[i].Since != "" {
			entry.Since = folders[i].Since
		}
		folders[i] = entry
		replaced = true
		break
	}
	if !replaced {
		if len(folders) >= teamFolderCap {
			return ErrTeamFolderCap
		}
		folders = append(folders, entry)
	}

	if err := writeTeamFolders(c.teamFoldersPath(), folders); err != nil {
		return err
	}
	if !c.TeamRecord {
		c.TeamRecord = true
		return c.saveLocked()
	}
	return nil
}

// TeamFolderList returns a copy of the team record. A missing or unreadable record
// reads as no entries; ScopeOf is what tells those apart.
func (c *Config) TeamFolderList() []TeamFolder {
	folders, _ := c.readTeamFolders()
	return append([]TeamFolder(nil), folders...)
}

// ScopeOf says what path belongs to, for the scope a program decision is stored
// with. identityOf answers the fingerprint of a folder, the same one the identity
// pin holds, so a folder renamed on disk is still found through its ancestors'
// identities. The record is read fresh every time and never cached: a folder being
// marked team must not have to reach through whoever is holding the config.
func (c *Config) ScopeOf(path string, identityOf func(string) string) Scope {
	folders, state := c.readTeamFolders()
	switch state {
	case teamRecordBroken:
		return Scope{Kind: ScopeUnknown, Key: ScopeUnknown}
	case teamRecordMissing:
		if c.teamRecordMarked() {
			return Scope{Kind: ScopeUnknown, Key: ScopeUnknown}
		}
		return Scope{Kind: ScopeLocal}
	}

	for _, f := range folders {
		if f.Kind != TeamFolderKindTeam {
			continue
		}
		if pathrel.SameOrBelow(path, f.Path) {
			return teamScope(f)
		}
	}

	// The folder may have been renamed, moved, or reached through another spelling
	// since it was recorded, so walk up from the document and match the identity of
	// each ancestor. A team folder above the document is a team folder whatever its
	// path has become.
	if identityOf != nil {
		for current := path; ; {
			if id := identityOf(current); id != "" {
				for _, f := range folders {
					if f.Kind == TeamFolderKindTeam && f.Identity != "" && f.Identity == id {
						return teamScope(f)
					}
				}
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	return Scope{Kind: ScopeLocal}
}

func teamScope(f TeamFolder) Scope {
	return Scope{Kind: ScopeTeam, Key: "team:" + f.Key}
}

// Scope is what a program decision about this entry's folder is stored as, the same
// answer ScopeOf gives for a path inside it.
func (f TeamFolder) Scope() Scope { return teamScope(f) }

// writeTeamFolders replaces the record with folders through a temporary file in the
// same folder and a rename, the way config.json's own save does.
func writeTeamFolders(path string, folders []TeamFolder) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if folders == nil {
		folders = []TeamFolder{}
	}
	data, err := json.MarshalIndent(teamFolderFile{Folders: folders}, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".team-folders-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
