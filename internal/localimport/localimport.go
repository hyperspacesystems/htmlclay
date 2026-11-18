// Package localimport reads what Hyperclay Local kept about a person's folders
// and hands a folder over to this app. It never writes inside Local's own data
// folder. Inside a person's folder it writes one file, the sync marker.
package localimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var (
	// ErrNotFound means Hyperclay Local has no settings on this computer.
	ErrNotFound = errors.New("Hyperclay Local is not set up here")
	// ErrOldSettings means the settings were written before Local's version 2
	// settings: opening Local once rewrites them.
	ErrOldSettings = errors.New("Hyperclay Local's settings are from an older version")
)

// Settings is the part of Local's settings.json the move reads.
type Settings struct {
	SettingsVersion int        `json:"settingsVersion"`
	DeviceID        string     `json:"deviceId"`
	Actor           *Actor     `json:"actor"`
	Roots           []Root     `json:"roots"`
	Sessions        []Session  `json:"syncSessions"`
	Programs        []Program  `json:"helperPrograms"`
	Decisions       []Decision `json:"helperDecisions"`
}

type Actor struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Root struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Path          string          `json:"path"`
	Port          int             `json:"port"`
	FormerAccount json.RawMessage `json:"formerAccount"`
}

// WasTeam reports whether the folder holds, or once held, a team's pages.
func (r Root) WasTeam() bool {
	return r.Kind == "team" || (len(r.FormerAccount) > 0 && string(r.FormerAccount) != "null")
}

// Program is a program the person registered in Local. AddedAt is in milliseconds.
type Program struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	AnyDocument bool   `json:"anyDocument"`
	AddedAt     int64  `json:"addedAt"`
}

// Decision is one document's answer in Local about one program name. DecidedAt
// is in milliseconds.
type Decision struct {
	Document  string `json:"document"`
	Name      string `json:"name"`
	Program   string `json:"program"`
	Allowed   bool   `json:"allowed"`
	DecidedAt int64  `json:"decidedAt"`
}

// AllowsIn answers Local's allows for documents inside one of folders. A refusal
// stays behind: HTML Clay never stores one, so that document asks again.
func (s *Settings) AllowsIn(folders []string) []Decision {
	var out []Decision
	for _, d := range s.Decisions {
		if !d.Allowed || d.Program == "" {
			continue
		}
		for _, folder := range folders {
			if strings.HasPrefix(d.Document, folder+string(filepath.Separator)) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

type Session struct {
	ID            string  `json:"id"`
	RootID        string  `json:"rootId"`
	AccountID     int64   `json:"accountId"`
	Kind          string  `json:"kind"`
	LegacyMetaDir *string `json:"legacyMetaDir"`
}

// ReadSettings reads <userData>/settings.json.
func ReadSettings(userData string) (*Settings, error) {
	raw, err := os.ReadFile(filepath.Join(userData, "settings.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("settings.json: %w", err)
	}
	if s.SettingsVersion != 2 {
		return nil, ErrOldSettings
	}
	return &s, nil
}

// SessionFor answers the sync session of a root, or nil when it only serves.
func (s *Settings) SessionFor(rootID string) *Session {
	for i := range s.Sessions {
		if s.Sessions[i].RootID == rootID {
			return &s.Sessions[i]
		}
	}
	return nil
}

// servedRoots reads <userData>/served-roots.json, which Local writes while it
// serves and removes on quit.
func servedRoots(userData string) (pid int, ports []int) {
	raw, err := os.ReadFile(filepath.Join(userData, "served-roots.json"))
	if err != nil {
		return 0, nil
	}
	var file struct {
		PID   int `json:"pid"`
		Roots []struct {
			Port int `json:"port"`
		} `json:"roots"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return 0, nil
	}
	for _, root := range file.Roots {
		ports = append(ports, root.Port)
	}
	return file.PID, ports
}

// singletonPID reads the pid from <userData>/SingletonLock, the link Electron
// keeps on macOS and Linux while an app runs. It points at "<host>-<pid>".
func singletonPID(userData string) int {
	target, err := os.Readlink(filepath.Join(userData, "SingletonLock"))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(target[strings.LastIndex(target, "-")+1:])
	return pid
}

// RunningPID answers Local's pid when alive says that process exists. It reads
// served-roots.json and Electron's SingletonLock: both outlive a crash, so the
// caller's alive check decides.
func RunningPID(userData string, alive func(pid int) bool) (int, bool) {
	served, _ := servedRoots(userData)
	for _, pid := range []int{served, singletonPID(userData)} {
		if pid > 0 && alive(pid) {
			return pid, true
		}
	}
	return 0, false
}

// ServedPorts answers the ports Local wrote down as listening.
func ServedPorts(userData string) []int {
	_, ports := servedRoots(userData)
	return ports
}

// Legacy reports whether the session's bookkeeping is still in Local's first layout.
func (s Session) Legacy() bool { return s.LegacyMetaDir != nil && *s.LegacyMetaDir != "" }

// metaRel is where a session's bookkeeping sits under a sync-meta folder, in
// Local's data and in this app's service data alike.
func (s Session) metaRel() string {
	if s.Legacy() {
		return *s.LegacyMetaDir
	}
	return filepath.Join("v2", s.ID)
}

// SourceMeta is the session's bookkeeping folder in Local's data.
func SourceMeta(userData string, s Session) string {
	return filepath.Join(userData, "sync-meta", s.metaRel())
}

// TargetMeta is where this app's service keeps that session's bookkeeping.
func TargetMeta(serviceData string, s Session) string {
	return filepath.Join(serviceData, "sync-meta", s.metaRel())
}

// UnfinishedBind reports whether Local stopped in the middle of the folder's
// first download. Such a folder stays with Local until it finishes.
func UnfinishedBind(userData string, s Session) bool {
	_, err := os.Stat(filepath.Join(SourceMeta(userData, s), "bind-in-progress.json"))
	return err == nil
}

// treeSum hashes every regular file under dir, by relative name and content.
// identity.json is left out: it is the one file the copy rewrites.
func treeSum(dir string) (string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, p)
			if rel != "identity.json" {
				names = append(names, rel)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	h := sha256.New()
	for _, rel := range names {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(raw))
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SameBookkeeping reports whether the copy still holds what Local has now.
func SameBookkeeping(source, copied string) (bool, error) {
	a, err := treeSum(source)
	if err != nil {
		return false, err
	}
	b, err := treeSum(copied)
	if err != nil {
		return false, err
	}
	return a == b, nil
}

// rewriteRootID sets "rootId" in a JSON object file and keeps every other field.
func rewriteRootID(file, rootID string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	id, _ := json.Marshal(rootID)
	fields["rootId"] = id
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	return writeFileSynced(file, out)
}

// writeFileSynced writes through a temp file in the same folder, syncs it and renames it.
func writeFileSynced(file string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
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
	return os.Rename(tmp.Name(), file)
}

// CopyBookkeeping copies a session's bookkeeping from Local's data into this
// app's service data and names rootID as the folder's id in identity.json. A
// legacy folder has no identity.json: the engine writes one when it converts it.
// The copy is built beside the target and renamed into place, so a crash leaves
// either nothing or a whole copy. Any earlier copy is replaced.
func CopyBookkeeping(userData, serviceData string, s Session, rootID string) error {
	source, target := SourceMeta(userData, s), TargetMeta(serviceData, s)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return fmt.Errorf("no bookkeeping for session %s in Hyperclay Local", s.ID)
	}
	staging := target + ".import"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if err := os.CopyFS(staging, os.DirFS(source)); err != nil {
		return err
	}
	if same, err := SameBookkeeping(source, staging); err != nil || !same {
		os.RemoveAll(staging)
		return fmt.Errorf("the copy of session %s does not match its source: %v", s.ID, err)
	}
	identity := filepath.Join(staging, "identity.json")
	if _, err := os.Stat(identity); err == nil {
		if err := rewriteRootID(identity, rootID); err != nil {
			os.RemoveAll(staging)
			return err
		}
	} else if !s.Legacy() {
		os.RemoveAll(staging)
		return fmt.Errorf("session %s has no identity.json", s.ID)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(staging, target)
}

func markerFile(folder string) string {
	return filepath.Join(folder, ".hyperclay", "sync-root.json")
}

// MarkerRootID answers the id the folder's sync marker names, and whether it has one.
func MarkerRootID(folder string) (string, bool) {
	raw, err := os.ReadFile(markerFile(folder))
	if err != nil {
		return "", false
	}
	var marker struct {
		RootID string `json:"rootId"`
	}
	if json.Unmarshal(raw, &marker) != nil {
		return "", false
	}
	return marker.RootID, true
}

// RewriteMarker names rootID in the folder's sync marker and keeps createdAt.
// This one write is the hand-over: after it Hyperclay Local refuses the folder.
func RewriteMarker(folder, rootID string) error {
	return rewriteRootID(markerFile(folder), rootID)
}

// SetMarkerAside renames a marker no session uses, so the folder can be served.
func SetMarkerAside(folder string) error {
	return os.Rename(markerFile(folder), markerFile(folder)+".from-hyperclay-local")
}

// Receipt records which of Local's folders moved. Hyperclay Local's last
// release reads it, and so does a second run of the move.
type Receipt struct {
	V      int           `json:"v"`
	Source string        `json:"source"`
	Roots  []ReceiptRoot `json:"roots"`
}

// Receipt states.
const (
	StateStarted = "started"
	StateDone    = "done"
	StateSkipped = "skipped"
)

type ReceiptRoot struct {
	LocalRootID string `json:"localRootId"`
	Path        string `json:"path"`
	FolderID    string `json:"folderId"`
	SessionID   string `json:"sessionId"`
	LocalPort   int    `json:"localPort"`
	State       string `json:"state"`
	Skip        string `json:"skip,omitempty"`
}

// LoadReceipt reads the receipt, or answers an empty one when there is none.
func LoadReceipt(file, source string) (*Receipt, error) {
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return &Receipt{V: 1, Source: source}, nil
	}
	if err != nil {
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	return &r, nil
}

// Root answers the entry of one of Local's roots, or nil.
func (r *Receipt) Root(localRootID string) *ReceiptRoot {
	for i := range r.Roots {
		if r.Roots[i].LocalRootID == localRootID {
			return &r.Roots[i]
		}
	}
	return nil
}

// Put adds or replaces a root's entry.
func (r *Receipt) Put(entry ReceiptRoot) {
	if existing := r.Root(entry.LocalRootID); existing != nil {
		*existing = entry
		return
	}
	r.Roots = append(r.Roots, entry)
}

// Save writes the receipt, synced, before the caller goes on.
func (r *Receipt) Save(file string) error {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeFileSynced(file, out)
}
