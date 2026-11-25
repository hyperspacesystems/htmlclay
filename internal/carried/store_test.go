package carried

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTheStoreKeepsReadOnlyCopiesNamedByTheirHash(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "carried-programs"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Put("async function main() { return 1 }")
	if err != nil || first != Hash("async function main() { return 1 }") {
		t.Fatalf("Put = %q, %v", first, err)
	}
	info, err := os.Stat(store.path(first))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o400 {
		t.Fatalf("stored copy mode %v, want 0400", info.Mode().Perm())
	}
	again, err := store.Put("async function main() { return 1 }")
	if err != nil || again != first {
		t.Fatalf("the same text stored as %q, %v", again, err)
	}
	changed, err := store.Put("async function main() { return 2 }")
	if err != nil || changed == first {
		t.Fatalf("a changed program kept the hash %q, %v", changed, err)
	}
}

func TestOnlyTheStoredCopyRuns(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	text := `async function main() { return "checked" }`
	hash, err := store.Put(text)
	if err != nil {
		t.Fatal(err)
	}
	p := store.path(hash)
	must(t, os.Chmod(p, 0o600))
	must(t, os.WriteFile(p, []byte(`async function main() { return "swapped" }`), 0o600))
	if got, err := store.Read(hash); err == nil {
		t.Fatalf("a stored copy changed after the check was read: %q", got)
	}
	kept, err := store.Keep(map[string]string{"main": text})
	if err != nil || kept["main"] != text {
		t.Fatalf("Keep = %q, %v; want the checked text stored again", kept["main"], err)
	}
	res, err := runJob(t, Job{Modules: kept})
	if err != nil || string(res.Data) != `"checked"` {
		t.Fatalf("ran %s, %v", res.Data, err)
	}
	if runtime.GOOS != "windows" {
		other := filepath.Join(t.TempDir(), "other.js")
		must(t, os.WriteFile(other, []byte(text), 0o400))
		must(t, os.Remove(p))
		must(t, os.Symlink(other, p))
		if _, err := store.Read(hash); err == nil {
			t.Fatal("a link in the store was read as a stored copy")
		}
	}
}
