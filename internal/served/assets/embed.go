// Package assets is the few Hyperclay Local static files the served-folder host
// answers with (favicons, the directory listing stylesheet, the two SharedWorker
// scripts), vendored into the binary from one pinned Local release by
// scripts/vendor-local-assets.mjs and checked against files/served-assets.lock.json.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"
	"time"
)

//go:embed files
var embedded embed.FS

const lockName = "served-assets.lock.json"

// Lock pins the vendored files to one Local release and records their bytes.
type Lock struct {
	Local struct {
		Ref         string    `json:"ref"`
		Commit      string    `json:"commit"`
		CommittedAt time.Time `json:"committedAt"`
	} `json:"local"`
	Files map[string]LockedFile `json:"files"`
}

// LockedFile is one vendored file's digest and length.
type LockedFile struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

var (
	lockOnce sync.Once
	lock     Lock
	lockErr  error
)

// ReadLock parses files/served-assets.lock.json.
func ReadLock() (Lock, error) {
	lockOnce.Do(func() {
		bytes, err := embedded.ReadFile("files/" + lockName)
		if err != nil {
			lockErr = err
			return
		}
		if err := json.Unmarshal(bytes, &lock); err != nil {
			lockErr = fmt.Errorf("files/%s: %w", lockName, err)
			return
		}
		if len(lock.Files) == 0 {
			lockErr = fmt.Errorf("files/%s lists no files", lockName)
		}
	})
	if lockErr != nil {
		return Lock{}, lockErr
	}
	return lock, nil
}

// FS returns the files in Local's own layout (assets/favicon.ico,
// src/main/assets/sync-worker.js, ...). Every file reports the Local commit's time
// as its modification time, so Last-Modified and the weak ETag the host builds
// from it are stable across builds, as a checkout's would be.
func FS() fs.FS {
	lock, err := ReadLock()
	if err != nil {
		panic(fmt.Sprintf("assets: %v", err))
	}
	files, err := fs.Sub(embedded, "files")
	if err != nil {
		panic(fmt.Sprintf("assets: %v", err))
	}
	return stampedFS{fsys: files, modTime: lock.Local.CommittedAt}
}

// verify re-hashes every file the lock lists and names the first one that differs.
func verify(fsys fs.FS, lock Lock) error {
	for name, file := range lock.Files {
		bytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		sum := sha256.Sum256(bytes)
		if digest := hex.EncodeToString(sum[:]); digest != file.SHA256 {
			return fmt.Errorf("%s: sha256 is %s, the lock records %s", name, digest, file.SHA256)
		}
		if int64(len(bytes)) != file.Bytes {
			return fmt.Errorf("%s: is %d bytes, the lock records %d", name, len(bytes), file.Bytes)
		}
	}
	return nil
}

// stampedFS is the embedded tree with every regular file's modification time
// replaced by the Local commit's, leaving directories and bytes untouched.
type stampedFS struct {
	fsys    fs.FS
	modTime time.Time
}

func (s stampedFS) Open(name string) (fs.File, error) {
	file, err := s.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.IsDir() {
		return file, nil
	}
	return stampedFile{File: file, info: stampedInfo{FileInfo: info, modTime: s.modTime}}, nil
}

func (s stampedFS) Stat(name string) (fs.FileInfo, error) {
	info, err := fs.Stat(s.fsys, name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return info, nil
	}
	return stampedInfo{FileInfo: info, modTime: s.modTime}, nil
}

func (s stampedFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(s.fsys, name)
}

type stampedFile struct {
	fs.File
	info fs.FileInfo
}

func (f stampedFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

type stampedInfo struct {
	fs.FileInfo
	modTime time.Time
}

func (i stampedInfo) ModTime() time.Time {
	return i.modTime
}
