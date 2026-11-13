package appcore

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestShellFixturesMatch keeps the Swift tests' copy of the snapshots equal to the
// goldens: after `go test -update`, run scripts/export-shell-fixtures.sh.
func TestShellFixturesMatch(t *testing.T) {
	src := filepath.Join("testdata", "v1")
	dst := filepath.Join("..", "..", "macos", "Tests", "PreviewCoreTests", "Fixtures", "v1")
	list := func(dir string) []string {
		names, err := filepath.Glob(filepath.Join(dir, "*.snapshot.json"))
		if err != nil {
			t.Fatal(err)
		}
		for i, n := range names {
			names[i] = filepath.Base(n)
		}
		sort.Strings(names)
		return names
	}
	want, got := list(src), list(dst)
	if len(want) == 0 {
		t.Fatal("no golden snapshots found")
	}
	if len(want) != len(got) {
		t.Fatalf("Swift fixtures %v, goldens %v; run scripts/export-shell-fixtures.sh", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("Swift fixtures %v, goldens %v; run scripts/export-shell-fixtures.sh", got, want)
		}
		a, errA := os.ReadFile(filepath.Join(src, name))
		b, errB := os.ReadFile(filepath.Join(dst, name))
		if errA != nil || errB != nil {
			t.Fatal(errA, errB)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs from the Swift copy; run scripts/export-shell-fixtures.sh", name)
		}
	}
}
