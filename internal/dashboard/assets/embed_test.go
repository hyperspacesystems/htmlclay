package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sort"
	"testing"
)

func digest(t *testing.T, name string) (string, int64) {
	t.Helper()
	b, err := fs.ReadFile(Bundle(), name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), int64(len(b))
}

// TestBundleMatchesLock: every embedded file is in the lock with its digest, and
// every locked file is embedded, so a hand edit or a partial copy fails.
func TestBundleMatchesLock(t *testing.T) {
	lock, err := LoadLock()
	if err != nil {
		t.Fatal(err)
	}
	if lock.Package.Name != "@panphora/hyperclay-workspace" || len(lock.Package.Commit) != 40 {
		t.Fatalf("lock package = %+v", lock.Package)
	}
	var embedded []string
	fs.WalkDir(Bundle(), ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			embedded = append(embedded, path)
		}
		return err
	})
	sort.Strings(embedded)
	if len(embedded) != len(lock.Files) {
		t.Fatalf("embedded %d files, lock lists %d", len(embedded), len(lock.Files))
	}
	for _, name := range embedded {
		want, ok := lock.Files[name]
		if !ok {
			t.Fatalf("%s is embedded but not locked", name)
		}
		if got, _ := digest(t, name); got != want {
			t.Fatalf("%s digest %s, lock %s", name, got, want)
		}
	}
}

// TestManifestNamesEmbeddedFiles: the entry script, its stylesheets and every
// asset the manifest lists are embedded, assets with the manifest's size and digest.
func TestManifestNamesEmbeddedFiles(t *testing.T) {
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entry.CSS) == 0 || len(m.Assets) == 0 {
		t.Fatalf("manifest entry %+v, %d assets; want stylesheets and assets", m.Entry, len(m.Assets))
	}
	for _, name := range append([]string{m.Entry.JavaScript}, m.Entry.CSS...) {
		digest(t, name)
	}
	for _, a := range m.Assets {
		sum, size := digest(t, a.Path)
		if sum != a.SHA256 || size != a.Bytes {
			t.Fatalf("%s: %s %d bytes, manifest %s %d", a.Path, sum, size, a.SHA256, a.Bytes)
		}
	}
}
