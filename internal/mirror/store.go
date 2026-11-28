// Package mirror keeps offline copies of the cross-origin files a page loads, so a
// document opened once online keeps working offline. The copies live in the user's
// cache folder, never in the document: content-addressed blobs shared by every origin,
// plus one small index per origin, so one folder's page can never plant bytes another
// folder runs. Copies HTML Clay fetched on its own, with no page supplying the bytes,
// go in one shared index every origin may read.
package mirror

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
	// entryCost and blockSize charge each copy for what it really takes: its index
	// entry, and its blob rounded up to a file system block. Without them, thousands
	// of tiny copies under cache-busting addresses grow the index and the folder while
	// the usage line stays near zero.
	entryCost = 512
	blockSize = 4096
)

// Limits bound the store. A zero field means no limit of that kind.
type Limits struct {
	Total   int64
	PerFile int64
	MaxAge  time.Duration
}

// DefaultLimits are the limits a new install starts with.
var DefaultLimits = Limits{Total: 500 << 20, PerFile: 25 << 20, MaxAge: 180 * 24 * time.Hour}

// LimitsFor maps a space choice to its limits: the per-file cap and the unused-copy
// sweep grow with the space, and unlimited removes all three. An unknown choice is the
// default.
func LimitsFor(space string) Limits {
	switch space {
	case "2gb":
		return Limits{Total: 2 << 30, PerFile: 100 << 20, MaxAge: 180 * 24 * time.Hour}
	case "10gb":
		return Limits{Total: 10 << 30, PerFile: 500 << 20, MaxAge: 365 * 24 * time.Hour}
	case "unlimited":
		return Limits{}
	}
	return DefaultLimits
}

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
	// Base is where a redirected fetch landed, for resolving a stylesheet's references.
	Base string `json:"base,omitempty"`
}

type index struct {
	Version int               `json:"version"`
	Origin  string            `json:"origin"`
	Entries map[string]*Entry `json:"entries"`
}

// Store is the on-disk copy store. It is safe for concurrent use.
type Store struct {
	dir string

	// diskMu orders whole index writes against Clear, so a flush that read an index
	// before Clear cannot write it back after.
	diskMu   sync.Mutex
	mu       sync.Mutex
	limits   Limits
	indexes  map[string]*index
	refs     map[string]int
	total    int64
	entries  int
	dirty    map[string]bool
	flushing *time.Timer
	// gen counts Clears, so a store that started before one never lands after it.
	gen       uint64
	lastSweep time.Time
	now       func() time.Time
}

var (
	ErrBadOrigin = errors.New("mirror: not an origin name")
	ErrBadURL    = errors.New("mirror: not an absolute http or https address")
	ErrTooBig    = errors.New("mirror: file over the per-file limit")
)

var (
	originPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,250}[a-z0-9])?$`)
	blobPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Open loads the store under dir, creating it 0700 when missing.
func Open(dir string, limits Limits) (*Store, error) {
	s := &Store{dir: dir, limits: limits, indexes: map[string]*index{}, refs: map[string]int{}, dirty: map[string]bool{}, now: time.Now}
	for _, sub := range []string{"blobs", "origins"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, err
		}
	}
	names, err := filepath.Glob(filepath.Join(dir, "origins", "o-*.json"))
	if err != nil {
		return nil, err
	}
	names = append(names, filepath.Join(dir, "shared.json"))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		var ix index
		if json.Unmarshal(data, &ix) != nil || ix.Version != indexVersion || ix.Entries == nil || !validOrigin(ix.Origin) {
			continue
		}
		loaded := &index{Version: indexVersion, Origin: ix.Origin, Entries: map[string]*Entry{}}
		s.indexes[ix.Origin] = loaded
		for key, e := range ix.Entries {
			if e == nil || !blobPattern.MatchString(e.Blob) || e.Size < 0 {
				continue
			}
			s.addLocked(ix.Origin, key, e)
		}
	}
	s.enforceLocked()
	s.deleteOrphansLocked(true)
	return s, nil
}

// NormalizeURL returns the key a URL is stored under: absolute http or https, fragment
// removed. It refuses anything else.
func NormalizeURL(raw string) (string, error) {
	if len(raw) > maxURLLength {
		return "", ErrBadURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", ErrBadURL
	}
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}

func validOrigin(origin string) bool {
	return origin == Shared || originPattern.MatchString(origin)
}

func charge(size int64) int64 {
	return (size + blockSize - 1) / blockSize * blockSize
}

// addLocked puts e under key, replacing any entry there, and keeps the reference
// counts and the running total.
func (s *Store) addLocked(origin, key string, e *Entry) {
	ix := s.indexes[origin]
	if ix == nil {
		ix = &index{Version: indexVersion, Origin: origin, Entries: map[string]*Entry{}}
		s.indexes[origin] = ix
	}
	// Count the new reference before dropping the old entry: the same bytes stored
	// again share one blob, and dropping first would delete the file both name.
	s.refs[e.Blob]++
	if s.refs[e.Blob] == 1 {
		s.total += charge(e.Size)
	}
	if _, ok := ix.Entries[key]; ok {
		s.removeLocked(origin, key)
	}
	ix.Entries[key] = e
	s.entries++
	s.total += entryCost + int64(len(key))
}

// removeLocked drops key from origin's index and deletes its blob once no index names
// it.
func (s *Store) removeLocked(origin, key string) {
	ix := s.indexes[origin]
	if ix == nil {
		return
	}
	e := ix.Entries[key]
	if e == nil {
		return
	}
	delete(ix.Entries, key)
	s.markLocked(origin)
	s.entries--
	s.total -= entryCost + int64(len(key))
	s.refs[e.Blob]--
	if s.refs[e.Blob] <= 0 {
		delete(s.refs, e.Blob)
		s.total -= charge(e.Size)
		os.Remove(s.blobPath(e.Blob))
	}
}

// SetLimits changes the limits and applies them at once.
func (s *Store) SetLimits(l Limits) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = l
	if l.PerFile > 0 {
		for origin, ix := range s.indexes {
			for key, e := range ix.Entries {
				if e.Size > l.PerFile {
					s.removeLocked(origin, key)
				}
			}
		}
	}
	s.lastSweep = time.Time{}
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
	return s.PutStream(origin, rawURL, meta, bytes.NewReader(body))
}

// PutStream stores what r yields under rawURL for origin. The bytes go to a temporary
// file while they are hashed, so a large copy never sits in memory.
func (s *Store) PutStream(origin, rawURL string, meta Entry, r io.Reader) error {
	if !validOrigin(origin) {
		return ErrBadOrigin
	}
	key, err := NormalizeURL(rawURL)
	if err != nil {
		return err
	}
	s.mu.Lock()
	perFile, gen := s.limits.PerFile, s.gen
	s.mu.Unlock()
	blobs := filepath.Join(s.dir, "blobs")
	if err := os.MkdirAll(blobs, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(blobs, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	src := r
	if perFile > 0 {
		src = io.LimitReader(r, perFile+1)
	}
	size, err := io.Copy(io.MultiWriter(tmp, hash), src)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if perFile > 0 && size > perFile {
		return ErrTooBig
	}
	blob := hex.EncodeToString(hash.Sum(nil))
	if meta.Type == "" {
		meta.Type = "application/octet-stream"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return nil
	}
	path := s.blobPath(blob)
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.Chmod(tmpName, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmpName, path); err != nil {
			return err
		}
	}
	now := s.now()
	meta.Blob, meta.Size, meta.StoredAt, meta.UsedAt = blob, size, now, now
	s.addLocked(origin, key, &meta)
	s.markLocked(origin)
	s.enforceLocked()
	return nil
}

func (s *Store) blobPath(blob string) string {
	return filepath.Join(s.dir, "blobs", blob[:2], blob)
}

// indexPath names an origin's index file. The o- prefix keeps a served folder named
// con, aux or nul from naming a Windows device.
func (s *Store) indexPath(origin string) string {
	if origin == Shared {
		return filepath.Join(s.dir, "shared.json")
	}
	return filepath.Join(s.dir, "origins", "o-"+origin+".json")
}

// lookupLocked finds key for origin: its own index first, then the shared one. An entry
// whose blob is gone (a cache cleaner emptied the folder) is dropped, so it is fetched
// again instead of counting as held.
func (s *Store) lookupLocked(origin, key string, sharedToo bool) (*Entry, string) {
	owners := []string{origin}
	if sharedToo && origin != Shared {
		owners = append(owners, Shared)
	}
	for _, o := range owners {
		ix := s.indexes[o]
		if ix == nil {
			continue
		}
		e := ix.Entries[key]
		if e == nil {
			continue
		}
		if _, err := os.Stat(s.blobPath(e.Blob)); err != nil {
			s.removeLocked(o, key)
			continue
		}
		return e, o
	}
	return nil, ""
}

// Reader opens the copy of rawURL origin may read, marking it used. The caller closes
// the file.
func (s *Store) Reader(origin, rawURL string) (Entry, *os.File, bool) {
	key, err := NormalizeURL(rawURL)
	if err != nil || !validOrigin(origin) {
		return Entry{}, nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, owner := s.lookupLocked(origin, key, true)
	if e == nil {
		return Entry{}, nil, false
	}
	f, err := os.Open(s.blobPath(e.Blob))
	if err != nil {
		s.removeLocked(owner, key)
		return Entry{}, nil, false
	}
	e.UsedAt = s.now()
	s.markLocked(owner)
	return *e, f, true
}

// Get returns the copy of rawURL origin may read, marking it used.
func (s *Store) Get(origin, rawURL string) (Entry, []byte, bool) {
	e, f, ok := s.Reader(origin, rawURL)
	if !ok {
		return Entry{}, nil, false
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return Entry{}, nil, false
	}
	return e, body, true
}

// small reads the copy of rawURL origin may read when it is at most limit bytes, without
// reading a larger one into memory.
func (s *Store) small(origin, rawURL string, limit int64) ([]byte, bool) {
	e, f, ok := s.Reader(origin, rawURL)
	if !ok {
		return nil, false
	}
	defer f.Close()
	if e.Size > limit {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(f, limit))
	return body, err == nil
}

// Lookup returns origin's own entry for rawURL without reading its bytes or marking it
// used. The shared index is not consulted.
func (s *Store) Lookup(origin, rawURL string) (Entry, bool) {
	key, err := NormalizeURL(rawURL)
	if err != nil || !validOrigin(origin) {
		return Entry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, _ := s.lookupLocked(origin, key, false); e != nil {
		return *e, true
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
		if e, _ := s.lookupLocked(origin, key, true); e != nil {
			n++
		}
	}
	return n
}

// Usage reports the space the copies take, each blob rounded up to a file system block
// and each entry charged for its index line, and the number of entries.
func (s *Store) Usage() (int64, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total, s.entries
}

// Clear deletes every copy and index.
func (s *Store) Clear() error {
	s.diskMu.Lock()
	defer s.diskMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flushing != nil {
		s.flushing.Stop()
		s.flushing = nil
	}
	s.gen++
	s.indexes = map[string]*index{}
	s.refs = map[string]int{}
	s.total, s.entries = 0, 0
	s.dirty = map[string]bool{}
	// A file another program or a running hand-over holds open can refuse removal on
	// Windows. Nothing names it any more, and the next start deletes leftovers, so only
	// failing to recreate the folders counts as failing to clear.
	var first error
	for _, sub := range []string{"blobs", "origins"} {
		os.RemoveAll(filepath.Join(s.dir, sub))
		if err := os.MkdirAll(filepath.Join(s.dir, sub), 0o700); err != nil && first == nil {
			first = err
		}
	}
	os.Remove(filepath.Join(s.dir, "shared.json"))
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
	s.diskMu.Lock()
	defer s.diskMu.Unlock()
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
	usedAt      time.Time
}

// enforceLocked drops entries unused past MaxAge (at most hourly), then the least
// recently used entries across every origin until the total fits.
func (s *Store) enforceLocked() {
	now := s.now()
	if s.limits.MaxAge > 0 && now.Sub(s.lastSweep) >= sweepInterval {
		s.lastSweep = now
		for origin, ix := range s.indexes {
			for key, e := range ix.Entries {
				if now.Sub(e.UsedAt) > s.limits.MaxAge {
					s.removeLocked(origin, key)
				}
			}
		}
	}
	if s.limits.Total <= 0 || s.total <= s.limits.Total {
		return
	}
	all := make([]ranked, 0, s.entries)
	for origin, ix := range s.indexes {
		for key, e := range ix.Entries {
			all = append(all, ranked{origin, key, e.UsedAt})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].usedAt.Before(all[j].usedAt) })
	for _, r := range all {
		if s.total <= s.limits.Total {
			break
		}
		s.removeLocked(r.origin, r.key)
	}
}

// deleteOrphansLocked deletes blobs no index names, and with temps, temporary files a
// crash left behind. It runs once, at Open: from then on the reference counts delete
// each blob as its last entry goes.
func (s *Store) deleteOrphansLocked(temps bool) {
	root := filepath.Join(s.dir, "blobs")
	dirs, _ := os.ReadDir(root)
	for _, d := range dirs {
		if !d.IsDir() {
			if temps && strings.HasPrefix(d.Name(), ".tmp") {
				os.Remove(filepath.Join(root, d.Name()))
			}
			continue
		}
		files, _ := os.ReadDir(filepath.Join(root, d.Name()))
		for _, f := range files {
			if s.refs[f.Name()] == 0 && (temps || !strings.HasPrefix(f.Name(), ".tmp")) {
				os.Remove(filepath.Join(root, d.Name(), f.Name()))
			}
		}
	}
	for _, name := range []string{"origins"} {
		files, _ := os.ReadDir(filepath.Join(s.dir, name))
		for _, f := range files {
			if temps && strings.HasPrefix(f.Name(), ".tmp") {
				os.Remove(filepath.Join(s.dir, name, f.Name()))
			}
		}
	}
}

// writeAtomic writes data to path through a temporary file and a rename, creating the
// folder first: a cache cleaner may have emptied it while HTML Clay runs.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
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
