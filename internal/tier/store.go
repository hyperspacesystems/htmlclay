package tier

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Store is HTML Clay's own record of where documents came from and whose code
// the person trusts. Each fact is its own small file, written by a rename, so
// the app and the htmlclay trust command can both write while the app runs
// without a lock, and every read sees the latest answer:
//
//	code/<hash>     a code set the person trusts, from any document
//	mine/<pathkey>  a document the person trusts at its path
//	seen/<pathkey>  what HTML Clay saw the first time it opened a document
//	answer/<pathkey> the person's last answer for a document: "Allow for this
//	                document" for the code it was given for, or "Don't allow"
//	upgrade.json    the first launch that had this store
//	developer       present while developer mode is on
//	programs        present while ~/htmlclay/programs/ is on without it
type Store struct {
	dir string
}

// How a trust record came to be.
const (
	HowCreated   = "created"
	HowCommand   = "command"
	HowAnswer    = "answer"
	HowDeveloper = "developer"
	HowUpgrade   = "upgrade"
	HowCopy      = "copy"
	// HowNewDocument is a blank copy the person made of their own document.
	HowNewDocument = "new-document"
	// HowAIEdit is code an AI edit wrote into a document the person trusted.
	HowAIEdit = "ai-edit"
)

var kinds = []string{"code", "mine", "seen", "answer"}

var codeHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Open opens the store in dir, creating it when missing. created reports a
// store that did not exist before, the first launch of a version that has one.
func Open(dir string) (s *Store, created bool, err error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		created = true
	}
	for _, kind := range kinds {
		if err := os.MkdirAll(filepath.Join(dir, kind), 0o700); err != nil {
			return nil, false, err
		}
	}
	return &Store{dir: dir}, created, nil
}

// MineRecord is a document the person trusts at its path. Marked records
// whether the file carried a download mark when it was trusted: trusting a
// downloaded file on purpose holds, but a file trusted unmarked that later
// carries a mark was replaced by a download and no longer counts.
type MineRecord struct {
	Path   string `json:"path"`
	How    string `json:"how"`
	Marked bool   `json:"marked,omitempty"`
	// Codes are the code sets the person trusted through this path, so
	// untrusting the document takes them back too.
	Codes []string `json:"codes,omitempty"`
	At    int64    `json:"at"`
}

// SeenRecord is the first sight of a document: whether it carried a download
// mark before anything HTML Clay did could change it.
type SeenRecord struct {
	Path       string `json:"path"`
	Downloaded bool   `json:"downloaded,omitempty"`
	// Developer is whether developer mode was on at this first sight. A first
	// sight in developer mode is the person's; turning developer mode on
	// later changes nothing.
	Developer bool `json:"developer,omitempty"`
	// BrokenSignature is whether the document carried a signature that did not
	// count. A copy keeps it, so dropping the signature tag clears nothing.
	BrokenSignature bool  `json:"brokenSignature,omitempty"`
	At              int64 `json:"at"`
}

// AnswerRecord is the person's last answer for a document. An allow holds for
// the code set it was given for, so a document asks again when its code
// changes. "Don't allow" holds whatever the code, so a page cannot ask again by
// changing itself; htmlclay untrust takes it back.
type AnswerRecord struct {
	Path    string `json:"path"`
	Display string `json:"display,omitempty"`
	Code    string `json:"code,omitempty"`
	Denied  bool   `json:"denied,omitempty"`
	At      int64  `json:"at"`
}

type codeRecord struct {
	How string `json:"how"`
	At  int64  `json:"at"`
}

// Upgrade is the first launch that had this store: when it was, and the
// trusted folders of that moment. A file already in one of them then, with no
// download mark, counts as the person's.
type Upgrade struct {
	At      time.Time `json:"at"`
	Folders []string  `json:"folders"`
}

func pathKey(path string) string {
	sum := sha256.Sum256([]byte(canonicalPath(path)))
	return hex.EncodeToString(sum[:])
}

func (s *Store) file(kind, name string) string {
	return filepath.Join(s.dir, kind, name)
}

func (s *Store) write(name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = renameWithRetry(tmp.Name(), name)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

func (s *Store) read(name string, v any) bool {
	data, err := os.ReadFile(name)
	return err == nil && json.Unmarshal(data, v) == nil
}

func now() int64 { return time.Now().Unix() }

// TrustCode adds a code set to the trust list.
func (s *Store) TrustCode(hash, how string) error {
	if !codeHash.MatchString(hash) {
		return errors.New("tier: not a code set hash")
	}
	return s.write(s.file("code", hash), codeRecord{How: how, At: now()})
}

// CodeTrusted reports whether the person trusts a code set.
func (s *Store) CodeTrusted(hash string) bool {
	if !codeHash.MatchString(hash) {
		return false
	}
	_, err := os.Stat(s.file("code", hash))
	return err == nil
}

// TrustCodeViaPath adds a code set to the trust list and lists it on the
// document's path record, so untrusting the document takes the code back too.
func (s *Store) TrustCodeViaPath(hash, path, how string) error {
	if err := s.TrustCode(hash, how); err != nil {
		return err
	}
	path = canonicalPath(path)
	rec, ok := s.PathTrust(path)
	if !ok {
		return nil
	}
	for _, have := range rec.Codes {
		if have == hash {
			return nil
		}
	}
	rec.Codes = append(rec.Codes, hash)
	rec.At = now()
	return s.write(s.file("mine", pathKey(path)), rec)
}

// TrustPath records that the person trusts the document at path. The code
// sets already trusted through the path stay listed, and an earlier Don't
// allow for the path is taken back: trusting a document is the answer.
func (s *Store) TrustPath(path string, marked bool, how string) error {
	path = canonicalPath(path)
	rec, _ := s.PathTrust(path)
	rec.Path, rec.How, rec.Marked, rec.At = path, how, marked, now()
	if err := s.write(s.file("mine", pathKey(path)), rec); err != nil {
		return err
	}
	if err := os.Remove(s.file("answer", pathKey(path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// PathTrust returns the document's trust record, if it has one.
func (s *Store) PathTrust(path string) (MineRecord, bool) {
	path = canonicalPath(path)
	var r MineRecord
	ok := s.read(s.file("mine", pathKey(path)), &r) && r.Path == path
	return r, ok
}

// NoteSeen records the first sight of path. A later call changes nothing, so
// the record keeps what the file was before any save. first reports the call
// that wrote it.
func (s *Store) NoteSeen(path string, downloaded bool) (first bool, err error) {
	return s.noteSeen(SeenRecord{Path: path, Downloaded: downloaded, Developer: s.Developer()})
}

// noteSeen records the first sight with the developer mode of that moment. The
// record is published with a link, so of two writers the first one wins and the
// other sees the name already taken.
func (s *Store) noteSeen(rec SeenRecord) (first bool, err error) {
	rec.Path = canonicalPath(rec.Path)
	path := rec.Path
	if _, ok := s.Seen(path); ok {
		return false, nil
	}
	rec.At = now()
	data, err := json.Marshal(rec)
	if err != nil {
		return false, err
	}
	dir := filepath.Join(s.dir, "seen")
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Link(tmp.Name(), s.file("seen", pathKey(path))); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Seen returns the first sight of path, if HTML Clay has opened it before.
func (s *Store) Seen(path string) (SeenRecord, bool) {
	path = canonicalPath(path)
	var r SeenRecord
	ok := s.read(s.file("seen", pathKey(path)), &r) && r.Path == path
	return r, ok
}

// Allow records "Allow for this document" for path while its code set is code.
func (s *Store) Allow(path, code string) error {
	if !codeHash.MatchString(code) {
		return errors.New("tier: an allow needs the code set it was given for")
	}
	return s.write(s.file("answer", pathKey(path)), AnswerRecord{Path: canonicalPath(path), Code: code, At: now()})
}

// Deny records "Don't allow" for path.
func (s *Store) Deny(path string) error {
	return s.write(s.file("answer", pathKey(path)), AnswerRecord{Path: canonicalPath(path), Display: filepath.Clean(path), Denied: true, At: now()})
}

// Answer returns the person's last answer for path, if there is one.
func (s *Store) Answer(path string) (AnswerRecord, bool) {
	path = canonicalPath(path)
	var r AnswerRecord
	ok := s.read(s.file("answer", pathKey(path)), &r) && r.Path == path
	return r, ok
}

// DeniedDocuments lists every document with a stored "Don't allow", sorted by
// path.
func (s *Store) DeniedDocuments() []string {
	entries, err := os.ReadDir(filepath.Join(s.dir, "answer"))
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		var r AnswerRecord
		if s.read(filepath.Join(s.dir, "answer", entry.Name()), &r) && r.Denied && r.Path != "" {
			if r.Display != "" {
				out = append(out, r.Display)
			} else {
				out = append(out, r.Path)
			}
		}
	}
	slices.Sort(out)
	return out
}

// ClearDeny removes a stored "Don't allow" for path so the document can ask
// again, leaving its first-sight record untouched.
func (s *Store) ClearDeny(path string) error {
	path = canonicalPath(path)
	r, ok := s.Answer(path)
	if !ok || !r.Denied {
		return nil
	}
	if err := os.Remove(s.file("answer", pathKey(path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Forget takes back the person's trust and answer for the document at path,
// and every code set trusted through that path, so the document asks again. A
// code set another trusted path lists stays trusted. The first sight stays: it
// is a fact, not a decision. code is the document's code set when the caller
// knows it; it does not have to.
func (s *Store) Forget(path, code string) error {
	path = canonicalPath(path)
	var errs []error
	rec, _ := s.PathTrust(path)
	for _, name := range []string{s.file("mine", pathKey(path)), s.file("answer", pathKey(path))} {
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	for _, hash := range append(rec.Codes, code) {
		if !codeHash.MatchString(hash) || s.pathListsCode(hash, path) {
			continue
		}
		if err := os.Remove(s.file("code", hash)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// pathListsCode reports whether a trusted path other than except lists hash.
func (s *Store) pathListsCode(hash, except string) bool {
	names, err := os.ReadDir(filepath.Join(s.dir, "mine"))
	if err != nil {
		return false
	}
	for _, entry := range names {
		var rec MineRecord
		if !s.read(filepath.Join(s.dir, "mine", entry.Name()), &rec) || rec.Path == except {
			continue
		}
		for _, have := range rec.Codes {
			if have == hash {
				return true
			}
		}
	}
	return false
}

// SetDeveloper switches developer mode: a local file with no download mark
// counts as the person's on first sight, and programs in ~/htmlclay/programs/
// may run.
func (s *Store) SetDeveloper(on bool) error { return s.setMarker("developer", on) }

// Developer reports developer mode.
func (s *Store) Developer() bool { return s.marker("developer") }

// SetPrograms switches ~/htmlclay/programs/ on without developer mode.
func (s *Store) SetPrograms(on bool) error { return s.setMarker("programs", on) }

// Programs reports whether programs in ~/htmlclay/programs/ may run: developer
// mode switches them on too.
func (s *Store) Programs() bool { return s.marker("programs") || s.Developer() }

func (s *Store) setMarker(name string, on bool) error {
	path := filepath.Join(s.dir, name)
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return s.write(path, codeRecord{How: HowCommand, At: now()})
}

func (s *Store) marker(name string) bool {
	_, err := os.Stat(filepath.Join(s.dir, name))
	return err == nil
}

// MarkUpgrade writes the upgrade record once. A store that already has one
// keeps it.
func (s *Store) MarkUpgrade(folders []string) error {
	if _, ok := s.Upgrade(); ok {
		return nil
	}
	if folders == nil {
		folders = []string{}
	}
	return s.write(filepath.Join(s.dir, "upgrade.json"), Upgrade{At: time.Now(), Folders: folders})
}

// Upgrade returns the upgrade record, if there is one.
func (s *Store) Upgrade() (Upgrade, bool) {
	var u Upgrade
	return u, s.read(filepath.Join(s.dir, "upgrade.json"), &u)
}

// Rebind moves every record HTML Clay keeps for the document at from to to, so
// a rename inside a workspace keeps the person's answers and first sight. A
// folder carries every record under it.
func (s *Store) Rebind(from, to string) error {
	from, to = canonicalPath(from), canonicalPath(to)
	var errs []error
	for _, kind := range []string{"mine", "seen", "answer"} {
		if err := s.rebindKind(kind, from, to); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// rebindKind moves the records of one kind at or under from to the same place
// under to.
func (s *Store) rebindKind(kind, from, to string) error {
	names, err := os.ReadDir(filepath.Join(s.dir, kind))
	if err != nil {
		return nil
	}
	var errs []error
	for _, entry := range names {
		name := filepath.Join(s.dir, kind, entry.Name())
		switch kind {
		case "mine":
			var rec MineRecord
			if s.read(name, &rec) && atOrUnder(rec.Path, from) {
				rec.Path = rebindPath(rec.Path, from, to)
				if err := s.write(s.file(kind, pathKey(rec.Path)), rec); err != nil {
					errs = append(errs, err)
					continue
				}
				if s.file(kind, pathKey(rec.Path)) != name {
					errs = append(errs, removeRecord(name))
				}
			}
		case "seen":
			var rec SeenRecord
			if s.read(name, &rec) && atOrUnder(rec.Path, from) {
				rec.Path = rebindPath(rec.Path, from, to)
				if err := s.write(s.file(kind, pathKey(rec.Path)), rec); err != nil {
					errs = append(errs, err)
					continue
				}
				if s.file(kind, pathKey(rec.Path)) != name {
					errs = append(errs, removeRecord(name))
				}
			}
		case "answer":
			var rec AnswerRecord
			if s.read(name, &rec) && atOrUnder(rec.Path, from) {
				rec.Path = rebindPath(rec.Path, from, to)
				if rec.Display != "" {
					rec.Display = filepath.Clean(rec.Path)
				}
				if err := s.write(s.file(kind, pathKey(rec.Path)), rec); err != nil {
					errs = append(errs, err)
					continue
				}
				if s.file(kind, pathKey(rec.Path)) != name {
					errs = append(errs, removeRecord(name))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// NoteCopy copies the first sight of the document at from onto the copy at to,
// so a duplicate of a download stays a stranger: the copy carries the same
// download mark, and HTML Clay must know that.
func (s *Store) NoteCopy(from, to string) error {
	from, to = canonicalPath(from), canonicalPath(to)
	seen, ok := s.Seen(from)
	if !ok {
		return nil
	}
	if _, ok := s.Seen(to); ok {
		return nil
	}
	return s.write(s.file("seen", pathKey(to)), SeenRecord{Path: to, Downloaded: seen.Downloaded, Developer: seen.Developer, BrokenSignature: seen.BrokenSignature, At: now()})
}

// atOrUnder reports whether path is dir or sits inside it.
func atOrUnder(path, dir string) bool {
	path, dir = filepath.Clean(path), filepath.Clean(dir)
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// rebindPath moves path from under from to the same place under to.
func rebindPath(path, from, to string) string {
	if rest, ok := strings.CutPrefix(filepath.Clean(path), filepath.Clean(from)); ok && (rest == "" || rest[0] == filepath.Separator) {
		return filepath.Clean(to) + rest
	}
	return path
}

// removeRecord deletes a record file that has been written again elsewhere.
func removeRecord(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
