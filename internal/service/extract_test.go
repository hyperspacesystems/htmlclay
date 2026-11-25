package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testBlob builds a gzip tarball of the given entries; a value of nil is a
// directory, "->x" a symlink to x.
func testBlob(t *testing.T, entries map[string]*string) ([]byte, string, int64) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range entries {
		h := &tar.Header{Name: name, Mode: 0o644}
		switch {
		case body == nil:
			h.Typeflag = tar.TypeDir
			h.Mode = 0o755
		case strings.HasPrefix(*body, "->"):
			h.Typeflag = tar.TypeSymlink
			h.Linkname = strings.TrimPrefix(*body, "->")
		default:
			h.Typeflag = tar.TypeReg
			h.Size = int64(len(*body))
			if strings.HasSuffix(name, "node") || strings.HasSuffix(name, ".exe") {
				h.Mode = 0o755
			}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(*body))
		}
	}
	tw.Close()
	zw.Close()
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), int64(buf.Len())
}

func str(s string) *string { return &s }

func validEntries() map[string]*string {
	return map[string]*string{
		"./":                nil,
		"./" + nodeName():   str("binary"),
		"./app/":            nil,
		"./app/main.cjs":    str("main"),
		"./app/vendor/x.js": str("x"),
	}
}

func TestInstallExtractsOnceAndReuses(t *testing.T) {
	root := t.TempDir()
	blob, sum, size := testBlob(t, validEntries())
	cmd, err := install(root, blob, sum, size, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	dir := filepath.Join(root, sum[:16])
	if cmd[0] != filepath.Join(dir, nodeName()) || cmd[1] != filepath.Join(dir, "app", "main.cjs") {
		t.Fatalf("command = %v", cmd)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "app", "vendor", "x.js")); err != nil || string(b) != "x" {
		t.Fatalf("x.js = %q, %v", b, err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(cmd[0]); info == nil || info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("node is not executable: %v", info)
		}
	}
	marker := filepath.Join(dir, completeMarker)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(marker, past, past)
	if _, err := install(root, blob, sum, size, false); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if info, _ := os.Stat(marker); info == nil || !info.ModTime().Equal(past) {
		t.Fatal("the second install extracted again instead of reusing the complete copy")
	}
}

func TestInstallRedoesAnIncompleteCopy(t *testing.T) {
	root := t.TempDir()
	blob, sum, size := testBlob(t, validEntries())
	dir := filepath.Join(root, sum[:16])
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, nodeName()), []byte("half"), 0o755)
	if _, err := install(root, blob, sum, size, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, nodeName())); string(b) != "binary" {
		t.Fatalf("node = %q, want the bundled bytes", b)
	}
}

func TestInstallPrunesOtherVersions(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "0123456789abcdef")
	os.MkdirAll(old, 0o755)
	fresh := filepath.Join(root, ".extract-aaaaaaaaaaaa")
	os.MkdirAll(fresh, 0o755)
	blob, sum, size := testBlob(t, validEntries())
	if _, err := install(root, blob, sum, size, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("an older extracted version was kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a fresh extraction of another process was removed")
	}
}

// While an update is being judged the host asks for the other versions to stay:
// a rollback starts the previous binary, which must find its own copy. Once the
// verdict is in, PruneOthers' prune removes them.
func TestInstallKeepsOtherVersionsWhenAsked(t *testing.T) {
	root := t.TempDir()
	previous := filepath.Join(root, "0123456789abcdef")
	os.MkdirAll(filepath.Join(previous, "app"), 0o755)
	stale := filepath.Join(root, ".extract-bbbbbbbbbbbb")
	os.MkdirAll(stale, 0o755)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(stale, past, past)
	blob, sum, size := testBlob(t, validEntries())

	// Twice: the first call extracts, the second takes the reuse path, and
	// both must leave the previous version alone.
	for i := 0; i < 2; i++ {
		if _, err := install(root, blob, sum, size, true); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
		if _, err := os.Stat(previous); err != nil {
			t.Fatalf("install %d removed the previous version while asked to keep it", i)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("an interrupted extraction from an hour ago was kept")
	}

	prune(root, filepath.Join(root, sum[:16]), false)
	if _, err := os.Stat(previous); !os.IsNotExist(err) {
		t.Fatal("the previous version was kept after the verdict")
	}
	if !complete(filepath.Join(root, sum[:16]), sum) {
		t.Fatal("the prune removed this build's own copy")
	}
}

func TestInstallRefuses(t *testing.T) {
	cases := map[string]map[string]*string{
		"parent path": {"../escape": str("x"), "./" + nodeName(): str("b"), "./app/main.cjs": str("m")},
		"absolute":    {"/abs": str("x"), "./" + nodeName(): str("b"), "./app/main.cjs": str("m")},
		"symlink":     {"./link": str("->/etc"), "./" + nodeName(): str("b"), "./app/main.cjs": str("m")},
		"no node":     {"./app/main.cjs": str("m")},
		"no main":     {"./" + nodeName(): str("b")},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			blob, sum, size := testBlob(t, entries)
			if _, err := install(root, blob, sum, size, false); err == nil {
				t.Fatal("install succeeded, want a refusal")
			}
			if _, err := os.Stat(filepath.Join(root, sum[:16])); !os.IsNotExist(err) {
				t.Fatal("a refused blob left an extracted copy")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); err == nil {
				t.Fatal("an entry was written outside the root")
			}
		})
	}
	t.Run("checksum", func(t *testing.T) {
		blob, sum, size := testBlob(t, validEntries())
		tampered := append([]byte{}, blob...)
		tampered[len(tampered)-1] ^= 1
		if _, err := install(t.TempDir(), tampered, sum, size, false); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("install of a tampered blob = %v, want a checksum refusal", err)
		}
	})
}
