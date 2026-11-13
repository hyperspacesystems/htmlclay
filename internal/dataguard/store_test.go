package dataguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/versions"
)

// guardStoreForTest puts a real versions store under a temp directory and wires
// the guard store to its Folder, the way the server does.
func guardStoreForTest(t *testing.T) (*Store, *versions.Store, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v := versions.New(filepath.Join(cfg, "versions"))
	doc := filepath.Join(dir, "board.htmlclay")
	if err := os.WriteFile(doc, []byte("<html>board</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewStore(v.Folder), v, doc
}

func orderedObject(pairs ...[2]any) *dataapi.Object {
	obj := dataapi.NewObject()
	for _, p := range pairs {
		obj.Define(p[0].(string), p[1])
	}
	return obj
}

// The baseline holds dataapi.Marshal output, so a stored baseline must come back
// key for key in the order the document had, not sorted by encoding/json.
func TestStoreRoundTripsKeyOrder(t *testing.T) {
	s, _, doc := guardStoreForTest(t)
	key := versions.Key(doc, nil)

	data, err := dataapi.Marshal(orderedObject([2]any{"zeta", "z"}, [2]any{"alpha", 1.0}, [2]any{"mid", "m"}))
	if err != nil {
		t.Fatal(err)
	}
	st := &State{Version: 1, Baseline: &Baseline{Data: data, Hash: "abc", At: 17}, Status: "idle", LastSeenHash: "abc"}
	if err := s.Write(key, doc, st); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := s.Read(key, doc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Version != 1 || got.Baseline == nil || got.Status != "idle" || got.LastSeenHash != "abc" {
		t.Fatalf("state did not survive: %+v", got)
	}
	if got.Baseline.Hash != "abc" || got.Baseline.At != 17 {
		t.Fatalf("baseline did not survive: %+v", got.Baseline)
	}
	if !bytes.Equal(got.Baseline.Data, data) {
		t.Fatalf("baseline bytes changed: got %s, want %s", got.Baseline.Data, data)
	}
	obj, err := decodeObject(json.NewDecoder(bytes.NewReader(got.Baseline.Data)))
	if err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	want := []string{"zeta", "alpha", "mid"}
	if keys := obj.Keys(); len(keys) != len(want) {
		t.Fatalf("keys %v, want %v", keys, want)
	} else {
		for i := range want {
			if keys[i] != want[i] {
				t.Fatalf("keys %v, want %v", keys, want)
			}
		}
	}
}

// A write must land under the one name the reader looks at, and a second write
// must replace it whole: no temp file is left behind and no partial state is
// visible.
func TestStoreWritesAtomically(t *testing.T) {
	s, v, doc := guardStoreForTest(t)
	key := versions.Key(doc, nil)

	if err := s.Write(key, doc, &State{Version: 1, Status: "first"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	folder, err := v.Folder(key, doc)
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	if names := guardNames(t, folder); len(names) != 1 || names[0] != StateName {
		t.Fatalf("folder after write holds %v, want only %q", names, StateName)
	}

	if err := s.Write(key, doc, &State{Version: 1, Status: "second"}); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if names := guardNames(t, folder); len(names) != 1 || names[0] != StateName {
		t.Fatalf("folder after rewrite holds %v, want only %q", names, StateName)
	}
	raw, err := os.ReadFile(filepath.Join(folder, StateName))
	if err != nil {
		t.Fatal(err)
	}
	var parsed State
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("stored state is not valid JSON: %v", err)
	}
	if parsed.Status != "second" {
		t.Fatalf("stored status %q, want %q", parsed.Status, "second")
	}
}

// Guard bookkeeping must never break a save, so a sidecar that does not parse
// reads as an empty state instead of an error.
func TestStoreCorruptStateReadsAsEmpty(t *testing.T) {
	s, v, doc := guardStoreForTest(t)
	key := versions.Key(doc, nil)

	folder, err := v.Folder(key, doc)
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, StateName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := s.Read(key, doc)
	if err != nil {
		t.Fatalf("read corrupt: %v", err)
	}
	if st.Version != 1 || st.Baseline != nil || st.Event != nil || st.Status != "" {
		t.Fatalf("corrupt state read as %+v, want empty version 1", st)
	}

	fresh := filepath.Join(filepath.Dir(doc), "never-guarded.html")
	st, err = s.Read(versions.Key(fresh, nil), fresh)
	if err != nil {
		t.Fatalf("read missing: %v", err)
	}
	if st.Version != 1 || st.Baseline != nil {
		t.Fatalf("missing state read as %+v, want empty version 1", st)
	}
}

// The whole design rests on this: guard state lives in the app's config folder,
// never beside the document the person is editing.
func TestStoreKeepsStateOutOfDocumentFolder(t *testing.T) {
	s, v, doc := guardStoreForTest(t)
	key := versions.Key(doc, nil)
	before := dirNames(t, filepath.Dir(doc))

	data, err := dataapi.Marshal(orderedObject([2]any{"a", 1.0}))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(key, doc, &State{Version: 1, Baseline: &Baseline{Data: data, Hash: "h", At: 1}}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := s.WriteRecover(key, doc, []byte("<html>recover</html>")); err != nil {
		t.Fatalf("write recover: %v", err)
	}

	if after := dirNames(t, filepath.Dir(doc)); len(after) != len(before) {
		t.Fatalf("document folder changed from %v to %v", before, after)
	}
	for _, name := range []string{StateName, RecoverName} {
		if _, err := os.Lstat(filepath.Join(filepath.Dir(doc), name)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s was written beside the document: %v", name, err)
		}
	}

	folder, err := v.Folder(key, doc)
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	if !v.Contains(folder) {
		t.Fatalf("guard folder %q is outside the versions directory %q", folder, v.BaseDir())
	}
	names := guardNames(t, folder)
	if len(names) != 2 || names[0] != StateName || names[1] != RecoverName {
		t.Fatalf("guard folder holds %v, want %q and %q", names, RecoverName, StateName)
	}
}

func TestStoreRecoverRoundTrips(t *testing.T) {
	s, v, doc := guardStoreForTest(t)
	key := versions.Key(doc, nil)

	html := []byte("<html><body>kept</body></html>")
	if got, err := s.ReadRecover(key, doc); err != nil || got != nil {
		t.Fatalf("missing recover read as %q, %v; want nil, nil", got, err)
	}
	if err := s.WriteRecover(key, doc, html); err != nil {
		t.Fatalf("write recover: %v", err)
	}
	got, err := s.ReadRecover(key, doc)
	if err != nil {
		t.Fatalf("read recover: %v", err)
	}
	if !bytes.Equal(got, html) {
		t.Fatalf("recover copy read as %q, want %q", got, html)
	}
	folder, err := v.Folder(key, doc)
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, RecoverName)); err != nil {
		t.Fatalf("recover copy is not in the guard folder: %v", err)
	}
}

// The guard's two names sit in the same folder as version files, so List and
// pruning must never offer them as versions or delete them.
func TestVersionListIgnoresGuardFiles(t *testing.T) {
	s, v, doc := guardStoreForTest(t)
	key := versions.Key(doc, nil)

	if _, err := v.Backup(key, doc, []byte("<html>one</html>")); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := v.Backup(key, doc, []byte("<html>two</html>")); err != nil {
		t.Fatalf("backup: %v", err)
	}
	data, err := dataapi.Marshal(orderedObject([2]any{"a", 1.0}))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(key, doc, &State{Version: 1, Baseline: &Baseline{Data: data, Hash: "h", At: 1}, Status: "idle", UIWorkPending: true}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := s.WriteRecover(key, doc, []byte("<html>recover</html>")); err != nil {
		t.Fatalf("write recover: %v", err)
	}

	folder, err := v.Folder(key, doc)
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("2000-01-01-00-00-%02d-000+0000.html", i)
		if err := os.WriteFile(filepath.Join(folder, name), []byte("<html>old</html>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := v.List(key, doc)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Name == StateName || e.Name == RecoverName {
			t.Fatalf("List returned guard file %q", e.Name)
		}
		if _, _, err := versions.ParseEntryName(e.Name); err != nil {
			t.Fatalf("List returned non-version %q", e.Name)
		}
	}

	v.MaybePrune(key, doc)

	entries, err = v.List(key, doc)
	if err != nil {
		t.Fatalf("list after prune: %v", err)
	}
	if len(entries) != versions.MinKeep {
		t.Fatalf("prune kept %d versions, want %d", len(entries), versions.MinKeep)
	}
	for _, name := range []string{StateName, RecoverName} {
		if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
			t.Fatalf("prune removed %s: %v", name, err)
		}
	}
}

// guardNames returns the folder entries that are not version files or the
// history's own metadata, so a test can see exactly what the guard left behind.
func guardNames(t *testing.T, dir string) []string {
	t.Helper()
	names := make([]string, 0, 2)
	for _, name := range dirNames(t, dir) {
		if name == "meta.json" {
			continue
		}
		if _, _, err := versions.ParseEntryName(name); err == nil {
			continue
		}
		names = append(names, name)
	}
	return names
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %q: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
