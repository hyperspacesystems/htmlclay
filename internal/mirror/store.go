// Package mirror keeps offline copies of the cross-origin files a page loads, so a
// document opened once online keeps working offline. The copies live in the user's
// cache folder, never in the document: content-addressed blobs shared by every origin,
// plus one small index per origin, so one folder's page can never plant bytes another
// folder runs. Copies HTML Clay fetched on its own, with no page supplying the bytes,
// go in one shared index every origin may read.
package mirror

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Shared names the index of copies HTML Clay fetched itself. No hostname can spell it,
// so no page request ever reaches it.
const Shared = "*"

const (
	indexVersion  = 1
	flushDelay    = 2 * time.Second
	maxURLLength  = 8 << 10
	sweepInterval = time.Hour
)

// Limits bound the store. A zero field means no limit of that kind.
type Limits struct {
	Total   int64
	PerFile int64
	MaxAge  time.Duration
}

// DefaultLimits are the limits a new install starts with.
var DefaultLimits = Limits{Total: 500 << 20, PerFile: 25 << 20, MaxAge: 180 * 24 * time.Hour}

// Entry is one stored copy in an origin's index.
type Entry struct {
	Blob         string    `json:"blob"`
	Type         string    `json:"type"`
	Size         int64     `json:"size"`
	StoredAt     time.Time `json:"storedAt"`
	UsedAt       time.Time `json:"usedAt"`
	Via          string    `json:"via"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
}

type index struct {
	Version int               `json:"version"`
	Origin  string            `json:"origin"`
	Entries map[string]*Entry `json:"entries"`
}

// Store is the on-disk copy store. It is safe for concurrent use.
type Store struct {
	dir string

	mu        sync.Mutex
	limits    Limits
	indexes   map[string]*index
	dirty     map[string]bool
	flushing  *time.Timer
	lastSweep time.Time
	now       func() time.Time
}

var (
	ErrBadOrigin = errors.New("mirror: not an origin name")
	ErrBadURL    = errors.New("mirror: not an absolute http or https address")
	ErrTooBig    = errors.New("mirror: file over the per-file limit")
)

var originPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,250}[a-z0-9])?$`)

// Open loads the store under dir, creating it 0700 when missing.
func Open(dir string, limits Limits) (*Store, error) {
	s := &Store{dir: dir, limits: limits, indexes: map[string]*index{}, dirty: map[string]bool{}, now: time.Now}
	for _, sub := range []string{"blobs", "origins"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, err
		}
	}
	names, err := filepath.Glob(filepath.Join(dir, "origins", "*.json"))
	if err != nil {
		return nil, err
	}
	names = append(names, filepath.Join(dir, "shared.json"))
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		var ix index
		if json.Unmarshal(data, &ix) != nil || ix.Version != indexVersion || ix.Entries == nil {
			continue
		}
		if ix.Origin != Shared && !originPattern.MatchString(ix.Origin) {
			continue
		}
		s.indexes[ix.Origin] = &ix
	}
	s.mu.Lock()
	s.enforceLocked()
	s.deleteOrphansLocked()
	s.mu.Unlock()
	return s, nil
}

// NormalizeURL returns the key a URL is stored under: absolute http or https, fragment
// removed. It refuses anything else.
func NormalizeURL(raw string) (string, error) {
	if len(raw) > maxURLLength {
		return "", ErrBadURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", ErrBadURL
	}
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}

func validOrigin(origin string) bool {
	return origin == Shared || originPattern.MatchString(origin)
}

// SetLimits changes the limits and applies them at once.
func (s *Store) SetLimits(l Limits) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = l
	s.enforceLocked()
}

// Limits reports the current limits.
func (s *Store) Limits() Limits {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limits
}

// Put stores body as the copy of rawURL for origin.
func (s *Store) Put(origin, rawURL, contentType string, body []byte, via string) error {
	return s.PutEntry(origin, rawURL, Entry{Type: contentType, Via: via}, body)
}

// PutEntry stores body under rawURL for origin, keeping meta's type, via and
// validators.
func (s *Store) PutEntry(origin, rawURL string, meta Entry, body []byte) error {
	if !validOrigin(origin) {
		return ErrBadOrigin
	}
	key, err := NormalizeURL(rawURL)
	if err != nil {
		return err
	}
	s.mu.Lock()
	perFile := s.limits.PerFile
	s.mu.Unlock()
	if perFile > 0 && int64(len(body)) > perFile {
		return ErrTooBig
	}
	sum := sha256.Sum256(body)
	blob := hex.EncodeToString(sum[:])
	if meta.Type == "" {
		meta.Type = "application/octet-stream"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeBlob(blob, body); err != nil {
		return err
	}
	now := s.now()
	meta.Blob, meta.Size, meta.StoredAt, meta.UsedAt = blob, int64(len(body)), now, now
	ix := s.indexes[origin]
	if ix == nil {
		ix = &index{Version: indexVersion, Origin: origin, Entries: map[string]*Entry{}}
		s.indexes[origin] = ix
	}
	ix.Entries[key] = &meta
	s.markLocked(origin)
	s.enforceLocked()
	return nil
}

// writeBlob runs under s.mu, so an orphan sweep never deletes a blob between its write
// and the index entry that names it.
func (s *Store) writeBlob(blob string, body []byte) error {
	path := s.blobPath(blob)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeAtomic(path, body)
}

func (s *Store) blobPath(blob string) string {
	return filepath.Join(s.dir, "blobs", blob[:2], blob)
}

func (s *Store) indexPath(origin string) string {
	if origin == Shared {
		return filepath.Join(s.dir, "shared.json")
	}
	return filepath.Join(s.dir, "origins", origin+".json")
}

// lookupLocked finds rawURL for origin: its own index first, then the shared one.
func (s *Store) lookupLocked(origin, key string) (*Entry, string) {
	for _, o := range []string{origin, Shared} {
		if ix := s.indexes[o]; ix != nil {
			if e := ix.Entries[key]; e != nil {
				return e, o
			}
		}
	}
	return nil, ""
}

// Get returns the copy of rawURL origin may read, marking it used.
func (s *Store) Get(origin, rawURL string) (Entry, []byte, bool) {
	key, err := NormalizeURL(rawURL)
	if err != nil || !validOrigin(origin) {
		return Entry{}, nil, false
	}
	s.mu.Lock()
	e, owner := s.lookupLocked(origin, key)
	if e == nil {
		s.mu.Unlock()
		return Entry{}, nil, false
	}
	e.UsedAt = s.now()
	s.markLocked(owner)
	entry := *e
	s.mu.Unlock()
	body, err := os.ReadFile(s.blobPath(entry.Blob))
	if err != nil {
		return Entry{}, nil, false
	}
	return entry, body, true
}

// Lookup returns origin's own entry for rawURL without reading its bytes or marking it
// used. The shared index is not consulted.
func (s *Store) Lookup(origin, rawURL string) (Entry, bool) {
	key, err := NormalizeURL(rawURL)
	if err != nil {
		return Entry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ix := s.indexes[origin]; ix != nil {
		if e := ix.Entries[key]; e != nil {
			return *e, true
		}
	}
	return Entry{}, false
}

// Touch marks origin's own copy of rawURL as just confirmed fresh by its server.
func (s *Store) Touch(origin, rawURL string) {
	key, err := NormalizeURL(rawURL)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ix := s.indexes[origin]; ix != nil {
		if e := ix.Entries[key]; e != nil {
			now := s.now()
			e.StoredAt, e.UsedAt = now, now
			s.markLocked(origin)
		}
	}
}

// Held counts the URLs in urls that origin may read a copy of.
func (s *Store) Held(origin string, urls []string) int {
	if !validOrigin(origin) {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, raw := range urls {
		key, err := NormalizeURL(raw)
		if err != nil {
			continue
		}
		if e, _ := s.lookupLocked(origin, key); e != nil {
			n++
		}
	}
	return n
}

// Usage reports the bytes of the blobs the indexes name, and the number of entries.
func (s *Store) Usage() (int64, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usageLocked()
}

func (s *Store) usageLocked() (int64, int) {
	seen := map[string]bool{}
	var bytes int64
	entries := 0
	for _, ix := range s.indexes {
		for _, e := range ix.Entries {
			entries++
			if !seen[e.Blob] {
				seen[e.Blob] = true
				bytes += e.Size
			}
		}
	}
	return bytes, entries
}

// Clear deletes every copy and index.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flushing != nil {
		s.flushing.Stop()
		s.flushing = nil
	}
	s.indexes = map[string]*index{}
	s.dirty = map[string]bool{}
	var first error
	for _, sub := range []string{"blobs", "origins"} {
		if err := os.RemoveAll(filepath.Join(s.dir, sub)); err != nil && first == nil {
			first = err
		}
		if err := os.MkdirAll(filepath.Join(s.dir, sub), 0o700); err != nil && first == nil {
			first = err
		}
	}
	if err := os.Remove(filepath.Join(s.dir, "shared.json")); err != nil && !os.IsNotExist(err) && first == nil {
		first = err
	}
	return first
}

func (s *Store) markLocked(origin string) {
	s.dirty[origin] = true
	if s.flushing == nil {
		s.flushing = time.AfterFunc(flushDelay, func() { s.Flush() })
	}
}

// Flush writes every changed index now.
func (s *Store) Flush() error {
	s.mu.Lock()
	if s.flushing != nil {
		s.flushing.Stop()
		s.flushing = nil
	}
	pending := map[string][]byte{}
	for origin := range s.dirty {
		ix := s.indexes[origin]
		if ix == nil || len(ix.Entries) == 0 {
			pending[origin] = nil
			continue
		}
		data, err := json.Marshal(ix)
		if err != nil {
			continue
		}
		pending[origin] = data
	}
	s.dirty = map[string]bool{}
	s.mu.Unlock()
	var first error
	for origin, data := range pending {
		path := s.indexPath(origin)
		var err error
		if data == nil {
			err = os.Remove(path)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = writeAtomic(path, data)
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}

type ranked struct {
	origin, key string
	e           *Entry
}

// enforceLocked drops entries unused past MaxAge, then the least recently used entries
// across every origin until the total fits, then deletes blobs no index names.
func (s *Store) enforceLocked() {
	now := s.now()
	changed := false
	if s.limits.MaxAge > 0 && now.Sub(s.lastSweep) >= sweepInterval {
		s.lastSweep = now
		for origin, ix := range s.indexes {
			for key, e := range ix.Entries {
				if now.Sub(e.UsedAt) > s.limits.MaxAge {
					delete(ix.Entries, key)
					s.markLocked(origin)
					changed = true
				}
			}
		}
	}
	if s.limits.PerFile > 0 {
		for origin, ix := range s.indexes {
			for key, e := range ix.Entries {
				if e.Size > s.limits.PerFile {
					delete(ix.Entries, key)
					s.markLocked(origin)
					changed = true
				}
			}
		}
	}
	if total, _ := s.usageLocked(); s.limits.Total > 0 && total > s.limits.Total {
		var all []ranked
		for origin, ix := range s.indexes {
			for key, e := range ix.Entries {
				all = append(all, ranked{origin, key, e})
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].e.UsedAt.Before(all[j].e.UsedAt) })
		refs := map[string]int{}
		for _, r := range all {
			refs[r.e.Blob]++
		}
		for _, r := range all {
			if total <= s.limits.Total {
				break
			}
			delete(s.indexes[r.origin].Entries, r.key)
			s.markLocked(r.origin)
			changed = true
			refs[r.e.Blob]--
			if refs[r.e.Blob] == 0 {
				total -= r.e.Size
			}
		}
	}
	if changed {
		s.deleteOrphansLocked()
	}
}

func (s *Store) deleteOrphansLocked() {
	live := map[string]bool{}
	for _, ix := range s.indexes {
		for _, e := range ix.Entries {
			live[e.Blob] = true
		}
	}
	dirs, _ := os.ReadDir(filepath.Join(s.dir, "blobs"))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(s.dir, "blobs", d.Name()))
		for _, f := range files {
			if !live[f.Name()] && !strings.HasPrefix(f.Name(), ".tmp") {
				os.Remove(filepath.Join(s.dir, "blobs", d.Name(), f.Name()))
			}
		}
	}
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
