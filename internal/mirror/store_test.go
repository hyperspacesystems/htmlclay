package mirror_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/mirror"
)

func TestCopyComesBackWithItsTypeAfterARestart(t *testing.T) {
	dir := t.TempDir()
	store, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("site1.localhost", "https://cdn.example/a.js#frag", "text/javascript", []byte("x=1"), "page"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	reopened, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	entry, body, ok := reopened.Get("site1.localhost", "https://cdn.example/a.js")
	if !ok {
		t.Fatal("the reopened store has no copy of the file")
	}
	if string(body) != "x=1" {
		t.Fatalf("body = %q, want x=1", body)
	}
	if entry.Type != "text/javascript" {
		t.Fatalf("type = %q, want text/javascript", entry.Type)
	}
}

func TestAnOriginCannotSeeAnotherOriginsCopy(t *testing.T) {
	store, err := mirror.Open(t.TempDir(), mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	const one = "https://cdn.example/app.js"
	if err := store.Put("a.localhost", one, "text/javascript", []byte("a()"), "page"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, _, ok := store.Get("b.localhost", one); ok {
		t.Fatal("b.localhost read a.localhost's copy")
	}
	if n := store.Held("b.localhost", []string{one}); n != 0 {
		t.Fatalf("b.localhost holds %d of a.localhost's copies, want 0", n)
	}
	if _, _, ok := store.Get("a.localhost", one); !ok {
		t.Fatal("a.localhost cannot read its own copy")
	}

	const shared = "https://cdn.example/shared.css"
	if err := store.Put(mirror.Shared, shared, "text/css", []byte("body{}"), "host"); err != nil {
		t.Fatalf("Put into the shared index: %v", err)
	}
	if _, _, ok := store.Get("b.localhost", shared); !ok {
		t.Fatal("b.localhost cannot read the shared index")
	}
}

func TestOverTheLimitLeastRecentlyUsedGoFirstAndOrphansAreDeleted(t *testing.T) {
	dir := t.TempDir()
	// Each copy is charged its index line (512 bytes plus the address) and its blob
	// rounded up to a 4 KiB block, so two copies fit and a third does not.
	const per = int64(512 + len("https://cdn.example/u1") + 4096)
	store, err := mirror.Open(dir, mirror.Limits{Total: 2*per + 100})
	if err != nil {
		t.Fatal(err)
	}
	put := func(rawURL, body string) {
		t.Helper()
		if err := store.Put("a.localhost", rawURL, "text/plain", []byte(body), "page"); err != nil {
			t.Fatalf("Put %s: %v", rawURL, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	const (
		u1 = "https://cdn.example/u1"
		u2 = "https://cdn.example/u2"
		u3 = "https://cdn.example/u3"
	)
	put(u1, strings.Repeat("1", 10))
	put(u2, strings.Repeat("2", 10))
	if _, _, ok := store.Get("a.localhost", u1); !ok {
		t.Fatal("u1 is missing before the copy that pushes the store over its limit")
	}
	time.Sleep(2 * time.Millisecond)
	put(u3, strings.Repeat("3", 10))

	if _, _, ok := store.Get("a.localhost", u2); ok {
		t.Fatal("u2, the least recently used copy, was kept")
	}
	if _, _, ok := store.Get("a.localhost", u1); !ok {
		t.Fatal("u1, read more recently than u2, was evicted")
	}
	if _, _, ok := store.Get("a.localhost", u3); !ok {
		t.Fatal("u3, just stored, is missing")
	}
	if bytes, _ := store.Usage(); bytes != 2*per {
		t.Fatalf("usage = %d bytes, want %d", bytes, 2*per)
	}
	files, err := filepath.Glob(filepath.Join(dir, "blobs", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("%d blob files on disk after the eviction, want 2: %v", len(files), files)
	}
}

func TestLimitsForEachSpaceChoice(t *testing.T) {
	if got := mirror.LimitsFor("500mb"); got != mirror.DefaultLimits {
		t.Errorf("LimitsFor(500mb) = %+v, want the default limits %+v", got, mirror.DefaultLimits)
	}
	if got := mirror.LimitsFor("unlimited"); got != (mirror.Limits{}) {
		t.Errorf("LimitsFor(unlimited) = %+v, want no limits", got)
	}
	if got := mirror.LimitsFor("2gb"); got.Total != 2<<30 {
		t.Errorf("LimitsFor(2gb).Total = %d, want %d", got.Total, int64(2<<30))
	}
	if got := mirror.LimitsFor("bogus"); got != mirror.DefaultLimits {
		t.Errorf("LimitsFor(bogus) = %+v, want the default limits %+v", got, mirror.DefaultLimits)
	}
}

func TestSetLimitsDropsFilesOverTheNewPerFileCap(t *testing.T) {
	store, err := mirror.Open(t.TempDir(), mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	const (
		small = "https://cdn.example/small.js"
		large = "https://cdn.example/large.js"
	)
	if err := store.Put("a.localhost", small, "text/javascript", []byte("0123456789"), "page"); err != nil {
		t.Fatalf("Put small: %v", err)
	}
	if err := store.Put("a.localhost", large, "text/javascript", bytes.Repeat([]byte("x"), 100), "page"); err != nil {
		t.Fatalf("Put large: %v", err)
	}
	store.SetLimits(mirror.Limits{PerFile: 50})
	if _, _, ok := store.Get("a.localhost", large); ok {
		t.Fatalf("a 100-byte copy is still held under a 50-byte per-file cap")
	}
	if _, _, ok := store.Get("a.localhost", small); !ok {
		t.Fatalf("a 10-byte copy was dropped under a 50-byte per-file cap")
	}
}

func TestClearLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	store, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	const (
		one    = "https://cdn.example/one.js"
		two    = "https://cdn.example/two.css"
		shared = "https://cdn.example/shared.woff2"
	)
	for _, c := range []struct{ origin, rawURL, kind string }{
		{"a.localhost", one, "text/javascript"},
		{"b.localhost", two, "text/css"},
		{mirror.Shared, shared, "font/woff2"},
	} {
		if err := store.Put(c.origin, c.rawURL, c.kind, []byte("bytes"), "page"); err != nil {
			t.Fatalf("Put %s: %v", c.rawURL, err)
		}
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if bytes, entries := store.Usage(); bytes != 0 || entries != 0 {
		t.Fatalf("usage after Clear = %d bytes, %d entries, want 0 and 0", bytes, entries)
	}

	reopened, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ origin, rawURL string }{
		{"a.localhost", one},
		{"b.localhost", two},
		{"a.localhost", shared},
	} {
		if _, _, ok := reopened.Get(c.origin, c.rawURL); ok {
			t.Fatalf("after Clear, %s still reads a copy of %s", c.origin, c.rawURL)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "blobs", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%d blob files left after Clear, want none: %v", len(files), files)
	}
}

func TestPutRefusesBadOriginsAndURLs(t *testing.T) {
	dir := t.TempDir()
	store, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ origin, rawURL string }{
		{"../evil", "https://cdn.example/a.js"},
		{"A.LOCALHOST", "https://cdn.example/a.js"},
		{"a.localhost", "file:///etc/hosts"},
		{"a.localhost", "/relative.js"},
		{"a.localhost", "https://alice:hunter2@cdn.example/a.js"},
	} {
		if err := store.Put(c.origin, c.rawURL, "text/javascript", []byte("x"), "page"); err == nil {
			t.Errorf("Put(%q, %q) was accepted", c.origin, c.rawURL)
		}
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	names, err := os.ReadDir(filepath.Join(dir, "origins"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("a refused Put left %d index file(s) under origins/, want none", len(names))
	}

	tiny, err := mirror.Open(t.TempDir(), mirror.Limits{PerFile: 4})
	if err != nil {
		t.Fatal(err)
	}
	err = tiny.Put("a.localhost", "https://cdn.example/big.js", "text/javascript", []byte("12345"), "page")
	if !errors.Is(err, mirror.ErrTooBig) {
		t.Fatalf("a 5-byte body over a 4-byte per-file limit = %v, want ErrTooBig", err)
	}
}

// A page polling a live feed hands over new bytes at one address again and again. Each
// replaced copy's blob must go at once, or the folder grows past the limit while the
// usage line stays flat.
func TestReplacingACopyDeletesTheOldBlob(t *testing.T) {
	dir := t.TempDir()
	store, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := store.Put("a.localhost", "https://feed.example/now.json", "application/json", []byte(strings.Repeat("x", i+1)), "page"); err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "blobs", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("%d blob files after five hand-overs at one address, want 1: %v", len(files), files)
	}
	if _, n := store.Usage(); n != 1 {
		t.Fatalf("%d entries, want 1", n)
	}
}

// A cache cleaner can empty the folder while HTML Clay runs. A copy whose blob is gone
// must stop counting as held, so the next open fetches it again, and the index must
// still be written.
func TestACopyWhoseFileVanishedIsNoLongerHeld(t *testing.T) {
	dir := t.TempDir()
	store, err := mirror.Open(dir, mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	const u = "https://cdn.example/a.js"
	if err := store.Put("a.localhost", u, "text/javascript", []byte("x=1"), "page"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if n := store.Held("a.localhost", []string{u}); n != 0 {
		t.Fatalf("held = %d after the folder was emptied, want 0", n)
	}
	if err := store.Put("a.localhost", "https://cdn.example/b.js", "text/javascript", []byte("y=2"), "page"); err != nil {
		t.Fatalf("Put after the folder was emptied: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush after the folder was emptied: %v", err)
	}
}

// The worker hands over unchanged bytes on every later open, and a CDN without an
// ETag answers 200 with the same bytes. Storing them again must keep the copy, or
// every unchanged file is missing offline after a second online open.
func TestStoringTheSameBytesAgainKeepsTheCopy(t *testing.T) {
	store, err := mirror.Open(t.TempDir(), mirror.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	const u = "https://cdn.example/lib.js"
	for i := 0; i < 2; i++ {
		if err := store.Put("a.localhost", u, "text/javascript", []byte("window.lib=1"), "page"); err != nil {
			t.Fatal(err)
		}
	}
	if _, body, ok := store.Get("a.localhost", u); !ok || string(body) != "window.lib=1" {
		t.Fatalf("after storing the same bytes twice the copy is %q (held %v), want the bytes", body, ok)
	}
}
