package aitools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Store keeps what AI with tools remembers, one small file per fact written by
// a rename, as the trust store does: the self-test result per agent version,
// and the folder each document reads.
type Store struct {
	dir string
}

// OpenStore opens or makes the store in dir.
func OpenStore(dir string) (*Store, error) {
	for _, sub := range []string{"checks", "folders"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir}, nil
}

// CheckRecord is one self-test result. Only a result that saw a canary leave
// the folder, or one that passed, is kept; a test that could not finish is
// not, so the next request tries again.
type CheckRecord struct {
	Agent   string    `json:"agent"`
	Version string    `json:"version"`
	Level   string    `json:"level"`
	Program string    `json:"program"`
	Passed  bool      `json:"passed"`
	Leaked  []string  `json:"leaked,omitempty"`
	At      time.Time `json:"at"`
}

// FolderRecord is the folder a document's agent reads, as the person chose it.
type FolderRecord struct {
	Document string    `json:"document"`
	Folder   string    `json:"folder"`
	Identity string    `json:"identity,omitempty"`
	Level    string    `json:"level,omitempty"`
	At       time.Time `json:"at"`
}

func key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:%s,", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))[:40] + ".json"
}

func (s *Store) read(sub, name string, into any) bool {
	data, err := os.ReadFile(filepath.Join(s.dir, sub, name))
	return err == nil && json.Unmarshal(data, into) == nil
}

func (s *Store) write(sub, name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.dir, sub)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Check is the kept self-test result for one agent version and level.
func (s *Store) Check(agent, version, level string) (CheckRecord, bool) {
	var r CheckRecord
	ok := s.read("checks", key(agent, version, level), &r)
	return r, ok && r.Agent == agent && r.Version == version && r.Level == level
}

func (s *Store) SetCheck(r CheckRecord) error {
	return s.write("checks", key(r.Agent, r.Version, r.Level), r)
}

// Checks lists every kept self-test result, newest first.
func (s *Store) Checks() []CheckRecord {
	entries, _ := os.ReadDir(filepath.Join(s.dir, "checks"))
	var out []CheckRecord
	for _, e := range entries {
		var r CheckRecord
		if filepath.Ext(e.Name()) == ".json" && s.read("checks", e.Name(), &r) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Folder is the folder document reads, when the person chose one.
func (s *Store) Folder(document string) (FolderRecord, bool) {
	var r FolderRecord
	ok := s.read("folders", key(document), &r)
	return r, ok && r.Document == document && r.Folder != ""
}

func (s *Store) SetFolder(r FolderRecord) error {
	if r.Document == "" || !filepath.IsAbs(r.Folder) {
		return errors.New("aitools: a folder record needs a document and an absolute folder")
	}
	return s.write("folders", key(r.Document), r)
}

// ForgetFolder drops document's folder, so its next request asks again.
func (s *Store) ForgetFolder(document string) error {
	err := os.Remove(filepath.Join(s.dir, "folders", key(document)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Folders lists every remembered folder, by document.
func (s *Store) Folders() []FolderRecord {
	entries, _ := os.ReadDir(filepath.Join(s.dir, "folders"))
	var out []FolderRecord
	for _, e := range entries {
		var r FolderRecord
		if filepath.Ext(e.Name()) == ".json" && s.read("folders", e.Name(), &r) && r.Folder != "" {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Document < out[j].Document })
	return out
}
