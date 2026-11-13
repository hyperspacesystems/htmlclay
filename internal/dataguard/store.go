package dataguard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/versions"
)

// StateName is the guard's sidecar inside a document's history folder. It never
// parses as a version entry, so List, Read and pruning never see it.
const StateName = "data-guard.json"

// RecoverName is the recover copy beside the state sidecar: the bytes of the
// version the guard offers back, so a resolve does not depend on a version that
// pruning may already have aged out.
const RecoverName = "data-guard.recover.html"

const (
	maxStateBytes   = 64 << 20
	maxRecoverBytes = 64 << 20

	guardTempPrefix = ".htmlclay-guard-"
)

// Baseline is the last island data HTML Clay itself wrote, and the hash of the
// file bytes that carried it. An outside write is anything whose bytes hash to
// something else.
type Baseline struct {
	Data json.RawMessage `json:"data"`
	Hash string          `json:"hash"`
	At   int64           `json:"at"`
}

// Event is one detected data-loss event awaiting a person's decision.
type Event struct {
	ID              string          `json:"id"`
	RecoverableData json.RawMessage `json:"recoverableData"`
	HasRecoverCopy  bool            `json:"hasRecoverCopy"`
	FirstDetectedAt int64           `json:"firstDetectedAt"`
	LastWriteAt     int64           `json:"lastWriteAt"`
	LossSummary     LossSummary     `json:"lossSummary"`
	Status          string          `json:"status"`
}

// State is everything the guard remembers about one document.
type State struct {
	Version       int       `json:"version"`
	Baseline      *Baseline `json:"baseline,omitempty"`
	UIWorkPending bool      `json:"uiWorkPending"`
	Event         *Event    `json:"event,omitempty"`
	Status        string    `json:"status,omitempty"`
	LastSeenHash  string    `json:"lastSeenHash,omitempty"`
}

// Store keeps guard state in the document's history folder under the app's
// config directory. The folder lookup is injected so the versions store keeps
// ownership of where a document's history lives, and the guard never has to
// know its layout.
type Store struct {
	root func(key, absPath string) (string, error)
}

// NewStore returns a guard store that resolves folders with root, which is
// versions.Store.Folder.
func NewStore(root func(key, absPath string) (string, error)) *Store {
	return &Store{root: root}
}

// Read returns the stored state for a document. A missing file is an empty
// state, and a file that does not parse reads as empty too: the guard must
// never break a save over its own bookkeeping.
func (s *Store) Read(key, absPath string) (*State, error) {
	root, err := s.open(key, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, versions.ErrNoHistory) {
			return &State{Version: 1}, nil
		}
		return nil, err
	}
	defer root.Close()

	data, err := readGuardFile(root, StateName, maxStateBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &State{Version: 1}, nil
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil || st.Version != 1 {
		log.Printf("dataguard: ignoring unreadable state for %s", absPath)
		return &State{Version: 1}, nil
	}
	return &st, nil
}

// Write stores the state for a document, atomically, so a crash mid-write leaves
// the previous state rather than a truncated one.
func (s *Store) Write(key, absPath string, st *State) error {
	next := State{Version: 1}
	if st != nil {
		next = *st
		if next.Version == 0 {
			next.Version = 1
		}
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return s.publish(key, absPath, StateName, data)
}

// WriteRecover stores the recover copy the guard offers back to a document.
func (s *Store) WriteRecover(key, absPath string, html []byte) error {
	return s.publish(key, absPath, RecoverName, html)
}

// ReadRecover returns the stored recover copy, or nil when there is none.
func (s *Store) ReadRecover(key, absPath string) ([]byte, error) {
	root, err := s.open(key, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, versions.ErrNoHistory) {
			return nil, nil
		}
		return nil, err
	}
	defer root.Close()

	data, err := readGuardFile(root, RecoverName, maxRecoverBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func (s *Store) open(key, absPath string) (*os.Root, error) {
	dir, err := s.root(key, absPath)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

// publish writes data to a hidden same-directory temp beneath the history root
// and renames it over name, so the visible name never holds a partial write.
func (s *Store) publish(key, absPath, name string, data []byte) error {
	root, err := s.open(key, absPath)
	if err != nil {
		return err
	}
	defer root.Close()

	tmpName, err := guardTempName()
	if err != nil {
		return err
	}
	f, err := root.OpenFile(tmpName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			root.Remove(tmpName)
		}
	}()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := root.Rename(tmpName, name); err != nil {
		return err
	}
	committed = true
	syncRootDir(root)
	return nil
}

// syncRootDir fsyncs the folder a root refers to, so the rename that names the
// new bytes is durable too. A filesystem that cannot sync a directory is not an
// error here: the bytes are already on disk.
func syncRootDir(root *os.Root) {
	d, err := root.Open(".")
	if err != nil {
		return
	}
	d.Sync()
	d.Close()
}

func guardTempName() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return guardTempPrefix + hex.EncodeToString(b[:]), nil
}

func readGuardFile(root *os.Root, name string, max int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	if info.Size() > max {
		return nil, fmt.Errorf("%s is larger than %d bytes", name, max)
	}
	return io.ReadAll(io.LimitReader(f, max))
}

func decodeValue(dec *json.Decoder) (dataapi.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return decodeObjectBody(dec)
		case '[':
			var items []dataapi.Value
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				items = append(items, v)
			}
			return items, expectDelim(dec, ']')
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)
	}
	return tok, nil
}

func decodeObject(dec *json.Decoder) (*dataapi.Object, error) {
	if err := expectDelim(dec, '{'); err != nil {
		return nil, err
	}
	return decodeObjectBody(dec)
}

func decodeObjectBody(dec *json.Decoder) (*dataapi.Object, error) {
	obj := dataapi.NewObject()
	for dec.More() {
		key, err := keyToken(dec)
		if err != nil {
			return nil, err
		}
		v, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		obj.Define(key, v)
	}
	return obj, expectDelim(dec, '}')
}

func keyToken(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected object key, got %v", tok)
	}
	return key, nil
}

func expectDelim(dec *json.Decoder, want rune) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok || rune(d) != want {
		return fmt.Errorf("expected %q, got %v", want, tok)
	}
	return nil
}
