package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/versions"
)

// historyTree is every path under roots with the bytes it holds, so a read can
// be proved to leave the document and the store exactly as it found them.
func historyTree(t *testing.T, roots ...string) []string {
	t.Helper()
	var lines []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				lines = append(lines, path+"/")
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(data)
			lines = append(lines, fmt.Sprintf("%s %d %s", path, len(data), hex.EncodeToString(sum[:])))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	sort.Strings(lines)
	return lines
}

func sameHistoryTree(t *testing.T, before, after []string) {
	t.Helper()
	if strings.Join(before, "\n") == strings.Join(after, "\n") {
		return
	}
	t.Fatalf("the read changed the tree:\nbefore:\n%s\nafter:\n%s",
		strings.Join(before, "\n"), strings.Join(after, "\n"))
}

type historyFixture struct {
	app    *app
	home   string
	served string
	rootID string
	loose  string
}

// newHistoryFixture builds an app with one served folder holding index.html and
// one loose document. The folder's watcher and snapshot sweep are stopped, so a
// tree comparison proves what the read itself wrote and nothing else.
func newHistoryFixture(t *testing.T) *historyFixture {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	servedDir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(servedDir, "index.html"), servedTestDocument)
	loosePath := filepath.Join(home, "docs", "page.html")
	writeTestFile(t, loosePath, "<!doctype html>\n<html><body><p>loose</p></body></html>\n")

	a := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := a.serveFolder(servedDir, "notes")
	if err != nil {
		t.Fatalf("serveFolder: %v", err)
	}
	if refusal != nil {
		t.Fatalf("serveFolder refused: %+v", refusal)
	}
	a.served.Close()
	return &historyFixture{app: a, home: home, served: servedDir, rootID: entry.ID, loose: loosePath}
}

// keepLooseVersion stores one version of the loose document, the way a save
// does, without touching the document itself.
func keepLooseVersion(t *testing.T, a *app, path, body string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := a.rt.versions.Backup(versions.Key(path, data), path, []byte(body)); err != nil {
		t.Fatalf("backup %s: %v", path, err)
	}
}

// looseHistoryDir is the version store folder bound to one document.
func looseHistoryDir(t *testing.T, a *app, absPath string) string {
	t.Helper()
	base := a.rt.versions.BaseDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("read the version store: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
		if err != nil {
			continue
		}
		var meta struct {
			AbsPath string `json:"absPath"`
		}
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		if meta.AbsPath == absPath {
			return dir
		}
	}
	t.Fatalf("no history folder for %s under %s", absPath, base)
	return ""
}

// looseVersionFiles is the store's own version filenames of one document.
func looseVersionFiles(t *testing.T, a *app, absPath string) []string {
	t.Helper()
	entries, err := os.ReadDir(looseHistoryDir(t, a, absPath))
	if err != nil {
		t.Fatalf("read the history folder: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if _, _, err := versions.ParseEntryName(entry.Name()); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// servedHistoryDir is the served store folder of one document under a root.
func servedHistoryDir(servedDir, rel, store string) string {
	base := strings.TrimSuffix(rel, filepath.Ext(rel))
	return filepath.Join(servedDir, ".hyperclay", store, base)
}

func writeServedVersion(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// checkHistoryItem is the one item shape both stores must answer.
func checkHistoryItem(t *testing.T, item historyItem, wantSize int64, wantKind string, wantRestorable bool) {
	t.Helper()
	if item.ID == "" {
		t.Fatalf("row has no id: %+v", item)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", item.CreatedAt); err != nil {
		t.Fatalf("createdAt %q is not UTC to the millisecond: %v", item.CreatedAt, err)
	}
	if item.Size != wantSize {
		t.Fatalf("size = %d, want %d (%+v)", item.Size, wantSize, item)
	}
	if item.Kind != wantKind {
		t.Fatalf("kind = %q, want %q", item.Kind, wantKind)
	}
	if item.Restorable != wantRestorable {
		t.Fatalf("restorable = %v, want %v", item.Restorable, wantRestorable)
	}
}

func readLoose(t *testing.T, fx *historyFixture, path, before string) []historyItem {
	t.Helper()
	items, err := fx.app.readHistory(historyDocument{Path: path}, before)
	if err != nil {
		t.Fatalf("read the loose history of %s: %v", path, err)
	}
	return items
}

func readServed(t *testing.T, fx *historyFixture, rel, before string) []historyItem {
	t.Helper()
	items, err := fx.app.readHistory(historyDocument{RootID: fx.rootID, Path: rel}, before)
	if err != nil {
		t.Fatalf("read the served history of %s: %v", rel, err)
	}
	return items
}

func TestM3HistoryReads(t *testing.T) {
	t.Run("one shape over both stores", func(t *testing.T) {
		fx := newHistoryFixture(t)
		older := "<!doctype html>\n<html><body><p>one</p></body></html>\n"
		newer := "<!doctype html>\n<html><body><p>two two</p></body></html>\n"
		keepLooseVersion(t, fx.app, fx.loose, older)
		keepLooseVersion(t, fx.app, fx.loose, newer)
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-07-890Z.html", older)
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-08-890Z.html", newer)

		loose := readLoose(t, fx, fx.loose, "")
		if len(loose) != 2 {
			t.Fatalf("loose history has %d rows, want 2: %+v", len(loose), loose)
		}
		checkHistoryItem(t, loose[0], int64(len(newer)), "saved", true)
		checkHistoryItem(t, loose[1], int64(len(older)), "saved", true)
		if loose[0].Pinned {
			t.Fatalf("an ordinary loose version reads as pinned: %+v", loose[0])
		}

		served := readServed(t, fx, "index.html", "")
		if len(served) != 2 {
			t.Fatalf("served history has %d rows, want 2: %+v", len(served), served)
		}
		checkHistoryItem(t, served[0], int64(len(newer)), "saved", true)
		checkHistoryItem(t, served[1], int64(len(older)), "saved", true)
		if served[0].ID == served[1].ID {
			t.Fatalf("two rows share one id: %+v", served)
		}
		if !strings.HasSuffix(served[0].ID, "2026-03-04-05-06-08-890Z.html") {
			t.Fatalf("newest row is %q, want the 08 second version", served[0].ID)
		}
	})

	t.Run("reading never writes", func(t *testing.T) {
		fx := newHistoryFixture(t)
		keepLooseVersion(t, fx.app, fx.loose, "<!doctype html>\n<html><body><p>kept</p></body></html>\n")
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-07-890Z.html", "<!doctype html>\n<html><body><p>kept</p></body></html>\n")

		// A document with no history at all must not gain a store folder.
		untouched := filepath.Join(fx.home, "docs", "fresh.html")
		writeTestFile(t, untouched, "<!doctype html>\n<html><body><p>fresh</p></body></html>\n")

		before := historyTree(t, fx.served, fx.app.rt.versions.BaseDir(), filepath.Join(fx.home, "docs"))
		readLoose(t, fx, fx.loose, "")
		readServed(t, fx, "index.html", "")
		if items := readLoose(t, fx, untouched, ""); len(items) != 0 {
			t.Fatalf("a document with no history answered %d rows", len(items))
		}
		if items := readServed(t, fx, "fresh.html", ""); len(items) != 0 {
			t.Fatalf("a served document with no history answered %d rows", len(items))
		}
		sameHistoryTree(t, before, historyTree(t, fx.served, fx.app.rt.versions.BaseDir(), filepath.Join(fx.home, "docs")))
	})

	t.Run("a missing file's history can still be read", func(t *testing.T) {
		fx := newHistoryFixture(t)
		id := "b1d1c0de-0000-4000-8000-000000000017"
		clay := filepath.Join(fx.home, "docs", "note.htmlclay")
		writeTestFile(t, clay, fmt.Sprintf("<!doctype html>\n<html htmlclayid=\"%s\"><body><p>note</p></body></html>\n", id))
		kept := "<!doctype html>\n<html><body><p>older note</p></body></html>\n"
		keepLooseVersion(t, fx.app, clay, kept)
		writeServedVersion(t, servedHistoryDir(fx.served, "index.html", "versions"), "2026-03-04-05-06-07-890Z.html", kept)

		if err := os.Remove(clay); err != nil {
			t.Fatal(err)
		}
		servedPath := filepath.Join(fx.served, "index.html")
		if err := os.Remove(servedPath); err != nil {
			t.Fatal(err)
		}

		storeBefore := historyTree(t, fx.served, fx.app.rt.versions.BaseDir(), filepath.Join(fx.home, "docs"))
		loose := readLoose(t, fx, clay, "")
		if len(loose) != 1 {
			t.Fatalf("a missing loose document answered %d rows, want 1: %+v", len(loose), loose)
		}
		checkHistoryItem(t, loose[0], int64(len(kept)), "saved", false)

		served := readServed(t, fx, "index.html", "")
		if len(served) != 1 {
			t.Fatalf("a missing served document answered %d rows, want 1: %+v", len(served), served)
		}
		checkHistoryItem(t, served[0], int64(len(kept)), "saved", false)

		sameHistoryTree(t, storeBefore, historyTree(t, fx.served, fx.app.rt.versions.BaseDir(), filepath.Join(fx.home, "docs")))
		for _, path := range []string{clay, servedPath} {
			if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("reading the history of %s created it: %v", path, err)
			}
		}
	})

	t.Run("equal times, two pages", func(t *testing.T) {
		fx := newHistoryFixture(t)
		const rows = 60

		// One instant for every row in both stores: only the id orders them.
		looseStamp := "2026-03-04-05-06-07-890Z"
		keepLooseVersion(t, fx.app, fx.loose, "<!doctype html>\n<html><body><p>first</p></body></html>\n")
		looseDir := looseHistoryDir(t, fx.app, fx.loose)
		for _, name := range looseVersionFiles(t, fx.app, fx.loose) {
			if err := os.Remove(filepath.Join(looseDir, name)); err != nil {
				t.Fatal(err)
			}
		}
		looseBodies := map[string]string{}
		for i := 0; i < rows; i++ {
			name := looseStamp + ".html"
			if i > 0 {
				name = fmt.Sprintf("%s-%02d.html", looseStamp, i)
			}
			body := fmt.Sprintf("<!doctype html>\n<html><body><p>%02d</p></body></html>\n", i)
			looseBodies[name] = body
			if err := os.WriteFile(filepath.Join(looseDir, name), []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
		}

		servedStamp := "2026-03-04-05-06-07-890Z"
		servedDir := servedHistoryDir(fx.served, "index.html", "versions")
		servedBodies := map[string]string{}
		for i := 1; i <= rows; i++ {
			name := fmt.Sprintf("%s-%03d.html", servedStamp, i)
			body := fmt.Sprintf("<!doctype html>\n<html><body><p>%02d</p></body></html>\n", i)
			servedBodies[name] = body
			writeServedVersion(t, servedDir, name, body)
		}

		for _, store := range []struct {
			name   string
			read   func(before string) []historyItem
			bodies map[string]string
		}{
			{"loose", func(before string) []historyItem { return readLoose(t, fx, fx.loose, before) }, looseBodies},
			{"served", func(before string) []historyItem { return readServed(t, fx, "index.html", before) }, servedBodies},
		} {
			first := store.read("")
			if len(first) != historyPageSize {
				t.Fatalf("%s: first page has %d rows, want %d", store.name, len(first), historyPageSize)
			}
			rest := store.read(first[len(first)-1].ID)
			if len(rest) != rows-historyPageSize {
				t.Fatalf("%s: second page has %d rows, want %d (cursor %s)",
					store.name, len(rest), rows-historyPageSize, first[len(first)-1].ID)
			}
			seen := map[string]bool{}
			for _, item := range append(append([]historyItem{}, first...), rest...) {
				if seen[item.ID] {
					t.Fatalf("%s: %s repeated across pages", store.name, item.ID)
				}
				seen[item.ID] = true
				if item.CreatedAt != first[0].CreatedAt {
					t.Fatalf("%s: rows of one instant report %q and %q", store.name, first[0].CreatedAt, item.CreatedAt)
				}
				if item.Size != int64(len(store.bodies[item.ID])) {
					t.Fatalf("%s: %s reports size %d, want %d", store.name, item.ID, item.Size, len(store.bodies[item.ID]))
				}
			}
			if len(seen) != rows {
				t.Fatalf("%s: two pages showed %d of %d versions", store.name, len(seen), rows)
			}
			if first[len(first)-1].ID <= rest[0].ID {
				t.Fatalf("%s: page boundary is out of order: %q then %q", store.name, first[len(first)-1].ID, rest[0].ID)
			}
		}
	})
}
