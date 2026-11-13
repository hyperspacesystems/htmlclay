//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
)

// The guard's own store: one JSON record per document under
// .hyperclay/guard/<name without extension>.json, and the whole recoverable
// file beside it as <name without extension>.recover.html. Both paths are what
// Local writes, so Local's notice, its sync and the trial read the same files.

const (
	guardDirName          = ".hyperclay/guard"
	legacyVersionsDirName = "sites-versions"
	guardRecoverSuffix    = ".recover.html"
)

// guardBaseline is the last island this host wrote and the digest of its data.
type guardBaseline struct {
	Data *dataapi.Object `json:"data"`
	Hash string          `json:"hash"`
	At   int64           `json:"at"`
}

// guardRecordEvent is one detected loss awaiting a person's decision.
type guardRecordEvent struct {
	ID                    string                `json:"id"`
	RecoverableData       *dataapi.Object       `json:"recoverableData"`
	RecoverableHTMLBackup *string               `json:"recoverableHtmlBackup"`
	FirstDetectedAt       int64                 `json:"firstDetectedAt"`
	LastWriteAt           int64                 `json:"lastWriteAt"`
	LossSummary           dataguard.LossSummary `json:"lossSummary"`
	Status                string                `json:"status"`
}

// guardRecord is the whole file. Field order is Local's insertion order, because
// the bytes are shared with the desktop app and a golden compares them.
type guardRecord struct {
	Baseline      *guardBaseline    `json:"baseline"`
	UIWorkPending bool              `json:"uiWorkPending"`
	Event         *guardRecordEvent `json:"event"`
	Status        string            `json:"status"`
	LastSeen      *string           `json:"lastSeen,omitempty"`
}

func (b *guardBaseline) UnmarshalJSON(data []byte) error {
	var raw struct {
		Data json.RawMessage `json:"data"`
		Hash string          `json:"hash"`
		At   int64           `json:"at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value, err := decodeGuardData(raw.Data)
	if err != nil {
		return err
	}
	object, _ := value.(*dataapi.Object)
	b.Data, b.Hash, b.At = object, raw.Hash, raw.At
	return nil
}

func (e *guardRecordEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID                    string                `json:"id"`
		RecoverableData       json.RawMessage       `json:"recoverableData"`
		RecoverableHTMLBackup *string               `json:"recoverableHtmlBackup"`
		FirstDetectedAt       int64                 `json:"firstDetectedAt"`
		LastWriteAt           int64                 `json:"lastWriteAt"`
		LossSummary           dataguard.LossSummary `json:"lossSummary"`
		Status                string                `json:"status"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value, err := decodeGuardData(raw.RecoverableData)
	if err != nil {
		return err
	}
	object, _ := value.(*dataapi.Object)
	e.ID = raw.ID
	e.RecoverableData = object
	e.RecoverableHTMLBackup = raw.RecoverableHTMLBackup
	e.FirstDetectedAt = raw.FirstDetectedAt
	e.LastWriteAt = raw.LastWriteAt
	e.LossSummary = raw.LossSummary
	e.Status = raw.Status
	return nil
}

// decodeGuardData reads stored island bytes back into a dataapi value, keeping
// the key order the bytes carry.
func decodeGuardData(raw json.RawMessage) (dataapi.Value, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	return decodeGuardValue(decoder)
}

func decodeGuardValue(decoder *json.Decoder) (dataapi.Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		object := dataapi.NewObject()
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("guard record has a non-string key")
			}
			value, err := decodeGuardValue(decoder)
			if err != nil {
				return nil, err
			}
			object.Define(key, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		items := []dataapi.Value{}
		for decoder.More() {
			item, err := decodeGuardValue(decoder)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return items, nil
	}
	return nil, errors.New("guard record has an unexpected delimiter")
}

func emptyGuard() *guardRecord {
	return &guardRecord{Status: "none"}
}

func newGuardBaseline(island dataapi.Value, at int64) *guardBaseline {
	island = dataguard.StoredIsland(island)
	if island == nil {
		return nil
	}
	object, _ := island.(*dataapi.Object)
	return &guardBaseline{Data: object, Hash: dataguard.IslandHash(island), At: at}
}

// guardRelPath is Local's guardRelPath: the record beside the document, in the
// same subfolder, named for the document without its extension.
func guardRelPath(name string) string {
	return guardDirName + "/" + documentBase(name) + ".json"
}

func resolveGuardPath(baseDir, name string) (string, error) {
	abs := filepath.Clean(filepath.Join(baseDir, guardRelPath(name)))
	root := filepath.Clean(baseDir)
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", errors.New("Guard path escapes base directory")
	}
	return abs, nil
}

func recoverHTMLPath(baseDir, name string) (string, error) {
	abs := filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, guardDirName, documentBase(name)+guardRecoverSuffix)))
	root := filepath.ToSlash(filepath.Clean(baseDir))
	if !strings.HasPrefix(abs, root+"/") {
		return "", errors.New("Recover path escapes base directory")
	}
	return abs, nil
}

func documentBase(name string) string {
	return pageExtension.ReplaceAllString(name, "")
}

func readGuard(baseDir, name string) (*guardRecord, error) {
	abs, err := resolveGuardPath(baseDir, name)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	var record guardRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func writeGuard(baseDir, name string, guard *guardRecord) error {
	key := guardKey(baseDir, name)
	if guard.Baseline != nil && guard.LastSeen == nil {
		if stamped, ok := guardSeen.takeUnstamped(key); ok {
			guard.LastSeen = &stamped
		}
	}
	abs, err := resolveGuardPath(baseDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o777); err != nil {
		return err
	}
	encoded, err := json.Marshal(guard)
	if err != nil {
		return err
	}
	tmp := abs + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o666); err != nil {
		return err
	}
	return os.Rename(tmp, abs)
}

func guardKey(baseDir, name string) string {
	return baseDir + "::" + name
}

// guardLocks serializes every read-modify-write of one document's record, the
// way Local's per-file promise chain does.
var guardLocks = struct {
	sync.Mutex
	locks map[string]*sync.Mutex
}{locks: map[string]*sync.Mutex{}}

func withGuardLock(key string, fn func()) {
	guardLocks.Lock()
	lock, ok := guardLocks.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		guardLocks.locks[key] = lock
	}
	guardLocks.Unlock()

	lock.Lock()
	defer lock.Unlock()
	fn()
}

// guardSeen is Local's pendingSeen and unstampedSeen: the bytes a save is
// landing so a page load can tell them from an outside edit, and the stamp of a
// save that arrived before its document had a baseline.
type guardSeenBoard struct {
	sync.Mutex
	pending   map[string]string
	unstamped map[string]string
}

var guardSeen = &guardSeenBoard{pending: map[string]string{}, unstamped: map[string]string{}}

func (s *guardSeenBoard) setPending(key, hash string) {
	s.Lock()
	s.pending[key] = hash
	s.Unlock()
}

func (s *guardSeenBoard) pendingFor(key string) string {
	s.Lock()
	defer s.Unlock()
	return s.pending[key]
}

func (s *guardSeenBoard) clearPending(key, hash string) {
	s.Lock()
	if s.pending[key] == hash {
		delete(s.pending, key)
	}
	s.Unlock()
}

func (s *guardSeenBoard) setUnstamped(key, hash string) {
	s.Lock()
	s.unstamped[key] = hash
	s.Unlock()
}

func (s *guardSeenBoard) takeUnstamped(key string) (string, bool) {
	s.Lock()
	defer s.Unlock()
	hash, ok := s.unstamped[key]
	delete(s.unstamped, key)
	return hash, ok
}

func (s *guardSeenBoard) deleteUnstamped(key string) {
	s.Lock()
	delete(s.unstamped, key)
	s.Unlock()
}

func guardSHA256(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

// newestVersionPath is the last-good full file for a raw write: the newest
// entry under .hyperclay/versions/<base>, then the pre-move sites-versions.
func newestVersionPath(baseDir, name string) string {
	base := documentBase(name)
	for _, versionsDir := range []string{versionsDirName, legacyVersionsDirName} {
		if found := newestVersionIn(baseDir, versionsDir, base); found != "" {
			return found
		}
	}
	return ""
}

// newestVersionIn ranks by parsed instant and then by collision suffix, the same
// ordering the pruner uses, so "newest" means one thing everywhere.
func newestVersionIn(baseDir, versionsDir, base string) string {
	dir := filepath.Join(baseDir, versionsDir, base)
	canonical, err := canonicalizeBase(baseDir)
	if err != nil {
		return ""
	}
	chain, err := rebaseOntoCanonical(canonical, baseDir, dir)
	if err != nil {
		return ""
	}
	if err := assertRealDirChain(canonical, chain); err != nil {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	ranked := []VersionEntry{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		ranked = append(ranked, VersionEntry{Name: entry.Name(), Full: full, MtimeMs: info.ModTime().UnixMilli()})
	}
	if len(ranked) == 0 {
		return ""
	}
	sort.SliceStable(ranked, func(i, j int) bool { return CompareNewestFirst(ranked[i], ranked[j]) < 0 })
	return ranked[0].Full
}
