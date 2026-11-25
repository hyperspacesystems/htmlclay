package update

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// waitingFile records the update CheckOnce staged, so a start after a quit can
// install it without fetching the feed again. The hash ties the record to the
// staged file: a later check that restages, or anything else that changes the
// file, makes the record stale.
const (
	waitingFile   = "waiting.json"
	waitingSchema = 1
)

type waiting struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// RecordWaiting notes that staged holds version, for Waiting at a later start.
func RecordWaiting(dir, version, staged string) error {
	sum, err := fileSHA256(staged)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(dir, waitingFile), &waiting{Schema: waitingSchema, Version: version, SHA256: sum})
}

// Waiting returns the update an earlier run downloaded and staged beside
// installPath, while the staged file is unchanged, newer than current, and not a
// version that failed before. Anything else removes the record and returns nil.
func Waiting(dir, current, installPath string) *Ready {
	path := filepath.Join(dir, waitingFile)
	var w waiting
	ok, err := readJSON(path, &w)
	if !ok && err == nil {
		return nil
	}
	drop := func() *Ready {
		os.Remove(path)
		return nil
	}
	if err != nil || w.Schema != waitingSchema || !IsStrictVersion(w.Version) || compareVersions(w.Version, current) <= 0 {
		return drop()
	}
	if st, _ := ReadState(dir); st != nil && st.Failed != nil && st.Failed.Version == w.Version {
		return drop()
	}
	staged := stagedPathFor(installPath)
	if sum, err := fileSHA256(staged); err != nil || sum != w.SHA256 {
		return drop()
	}
	return &Ready{Version: w.Version, StagedPath: staged}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
