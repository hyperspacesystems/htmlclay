package mirror_test

import (
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
	store, err := mirror.Open(dir, mirror.Limits{Total: 25})
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
	if bytes, _ := store.Usage(); bytes != 20 {
		t.Fatalf("usage = %d bytes, want 20", bytes)
	}
	files, err := filepath.Glob(filepath.Join(dir, "blobs", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("%d blob files on disk after the eviction, want 2: %v", len(files), files)
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
