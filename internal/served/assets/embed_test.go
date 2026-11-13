package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedFilesMatchLock(t *testing.T) {
	lock, err := ReadLock()
	if err != nil {
		t.Fatalf("the lock does not load: %v", err)
	}
	if len(lock.Files) != 6 {
		t.Fatalf("the lock lists %d files", len(lock.Files))
	}
	files := FS()
	if err := verify(files, lock); err != nil {
		t.Fatalf("the embedded files do not match the lock: %v", err)
	}
	for name, entry := range lock.Files {
		bytes, err := fs.ReadFile(files, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sum := sha256.Sum256(bytes)
		if digest := hex.EncodeToString(sum[:]); digest != entry.SHA256 {
			t.Fatalf("%s: sha256 is %s, the lock records %s", name, digest, entry.SHA256)
		}
		if int64(len(bytes)) != entry.Bytes {
			t.Fatalf("%s: is %d bytes, the lock records %d", name, len(bytes), entry.Bytes)
		}
	}
	walked := 0
	err = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		walked++
		if name == lockName {
			return nil
		}
		if _, ok := lock.Files[name]; !ok {
			t.Fatalf("%s is embedded but the lock does not list it", name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the embedded files: %v", err)
	}
	if walked != len(lock.Files)+1 {
		t.Fatalf("the embedded tree holds %d files, the lock lists %d plus itself", walked, len(lock.Files))
	}
}

func TestModTimeIsTheLocalCommit(t *testing.T) {
	lock, err := ReadLock()
	if err != nil {
		t.Fatalf("the lock does not load: %v", err)
	}
	if lock.Local.CommittedAt.IsZero() {
		t.Fatal("the lock records no commit time")
	}
	want := lock.Files["assets/favicon.ico"]
	files := FS()
	info, err := fs.Stat(files, "assets/favicon.ico")
	if err != nil {
		t.Fatalf("assets/favicon.ico: %v", err)
	}
	if !info.ModTime().Equal(lock.Local.CommittedAt) {
		t.Fatalf("assets/favicon.ico reports modified %s, the lock records %s", info.ModTime(), lock.Local.CommittedAt)
	}
	if info.Size() != want.Bytes {
		t.Fatalf("assets/favicon.ico is %d bytes, the lock records %d", info.Size(), want.Bytes)
	}
	file, err := files.Open("assets/favicon.ico")
	if err != nil {
		t.Fatalf("assets/favicon.ico: %v", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		t.Fatalf("assets/favicon.ico: %v", err)
	}
	if !opened.ModTime().Equal(lock.Local.CommittedAt) {
		t.Fatalf("the opened assets/favicon.ico reports modified %s, the lock records %s", opened.ModTime(), lock.Local.CommittedAt)
	}
	if opened.Size() != want.Bytes {
		t.Fatalf("the opened assets/favicon.ico is %d bytes, the lock records %d", opened.Size(), want.Bytes)
	}
}

func TestLockCheckCatchesAChangedByte(t *testing.T) {
	lock, err := ReadLock()
	if err != nil {
		t.Fatalf("the lock does not load: %v", err)
	}
	files := FS()
	copied := fstest.MapFS{}
	for name := range lock.Files {
		bytes, err := fs.ReadFile(files, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		copied[name] = &fstest.MapFile{Data: bytes}
	}
	if err := verify(copied, lock); err != nil {
		t.Fatalf("the copy does not verify before the byte is flipped: %v", err)
	}
	const planted = "src/main/assets/sync-worker.js"
	flipped := bytes.Clone(copied[planted].Data)
	flipped[0] ^= 0xff
	copied[planted] = &fstest.MapFile{Data: flipped}
	err = verify(copied, lock)
	if err == nil {
		t.Fatal("verify accepted a changed byte")
	}
	if !strings.Contains(err.Error(), planted) {
		t.Fatalf("verify reported %q, which does not name %s", err, planted)
	}
}
