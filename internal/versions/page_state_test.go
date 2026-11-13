package versions

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pageTag1 = `<script merge="blockeditor" clay="live" type="application/json">{"v":1}</script>`
	pageTag2 = `<script merge="blockeditor" clay="live" type="application/json">{"v":2}</script>`
)

func pageGenerationForTest(t *testing.T) string {
	t.Helper()
	gen, err := NewPageGeneration()
	if err != nil {
		t.Fatal(err)
	}
	return gen
}

func readPageStateJSON(t *testing.T, dir string) pageStateFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, PageStateName))
	if err != nil {
		t.Fatal(err)
	}
	var ps pageStateFile
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("sidecar is not valid JSON: %v", err)
	}
	return ps
}

func pruneHistoryRootForTest(t *testing.T, s *Store, key, absPath string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	vroot, err := s.openVersionsRoot(false)
	if err != nil {
		t.Fatal(err)
	}
	defer vroot.Close()
	ref, err := s.openHistory(vroot, key, absPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Close()
	pruneDir(ref.root)
}

// diskBytes is a document with one recognized block editor article, so a
// settle that finds no matching record for it removes the sidecar.
func diskBytes(label string) []byte {
	return []byte(`<!doctype html><html><body><article class="blockeditor" data-blockeditor="1" data-rev="` + label + `"></article></body></html>`)
}

// settlePageStateForTest stores record as the settled state for hash.
func settlePageStateForTest(t *testing.T, s *Store, key, absPath string, written []byte, record PageStateRecord) {
	t.Helper()
	if err := s.PreparePageState(key, absPath, Hash(written), record); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePageState(key, absPath, written); err != nil {
		t.Fatal(err)
	}
}

func TestPageStatePrepareThenSettle(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	hashB := Hash(diskBytes("B"))

	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashB, gen, pageTag1)); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.PageStateFor(key, absPath, hashB)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.FileHash != hashB || got.Tag != pageTag1 || got.Generation != gen {
		t.Fatalf("candidate not selectable: ok=%v record=%+v", ok, got)
	}
	if _, ok, err := s.PageStateFor(key, absPath, hashA); err != nil || ok {
		t.Fatalf("record for bytes that were never written: ok=%v err=%v", ok, err)
	}

	if err := s.SettlePageState(key, absPath, diskBytes("B")); err != nil {
		t.Fatal(err)
	}
	dir := lookupHistoryDirForTest(t, s, key, absPath)
	ps := readPageStateJSON(t, dir)
	if ps.Version != 1 {
		t.Fatalf("sidecar version = %d, want 1", ps.Version)
	}
	if ps.Matching == nil || ps.Matching.FileHash != hashB || ps.Matching.Tag != pageTag1 {
		t.Fatalf("matching slot = %+v, want hash B", ps.Matching)
	}
	if ps.Candidate != nil {
		t.Fatalf("candidate slot survived settle: %+v", ps.Candidate)
	}
	if got, ok, err := s.PageStateFor(key, absPath, hashB); err != nil || !ok || got.Tag != pageTag1 {
		t.Fatalf("settled record not selectable: ok=%v record=%+v err=%v", ok, got, err)
	}
}

func TestPageStateKeepsTheDiskRecordWhenTheWriteNeverLands(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	hashB := Hash(diskBytes("B"))
	hashC := Hash(diskBytes("C"))
	recordA := NewPageStateRecord(hashA, gen, pageTag1)

	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), recordA)

	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashB, gen, pageTag2)); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.PageStateFor(key, absPath, hashA)
	if err != nil || !ok {
		t.Fatalf("disk record lost after a write that never landed: ok=%v err=%v", ok, err)
	}
	if got != recordA {
		t.Fatalf("disk record changed: got %+v want %+v", got, recordA)
	}

	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashC, gen, pageTag1)); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.PageStateFor(key, absPath, hashA); err != nil || !ok || got != recordA {
		t.Fatalf("disk record changed by a second candidate: ok=%v record=%+v err=%v", ok, got, err)
	}
	if got, ok, err := s.PageStateFor(key, absPath, hashC); err != nil || !ok || got.FileHash != hashC {
		t.Fatalf("new candidate not selectable: ok=%v record=%+v err=%v", ok, got, err)
	}
	if _, ok, err := s.PageStateFor(key, absPath, hashB); err != nil || ok {
		t.Fatalf("dropped candidate was promoted: ok=%v err=%v", ok, err)
	}
}

func TestPageStateInterruptedAtEachBoundary(t *testing.T) {
	baseDir := t.TempDir()
	s := New(baseDir)
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	hashB := Hash(diskBytes("B"))
	hashC := Hash(diskBytes("C"))

	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), NewPageStateRecord(hashA, gen, pageTag1))
	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashB, gen, pageTag2)); err != nil {
		t.Fatal(err)
	}

	restarted := New(baseDir)
	if got, ok, err := restarted.PageStateFor(key, absPath, hashA); err != nil || !ok || got.Tag != pageTag1 {
		t.Fatalf("disk record lost across a restart: ok=%v record=%+v err=%v", ok, got, err)
	}
	if got, ok, err := restarted.PageStateFor(key, absPath, hashB); err != nil || !ok || got.Tag != pageTag2 {
		t.Fatalf("prepared candidate lost across a restart: ok=%v record=%+v err=%v", ok, got, err)
	}

	if err := restarted.SettlePageState(key, absPath, diskBytes("B")); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := restarted.PageStateFor(key, absPath, hashB); err != nil || !ok || got.Tag != pageTag2 {
		t.Fatalf("settled record lost: ok=%v record=%+v err=%v", ok, got, err)
	}
	if _, ok, err := restarted.PageStateFor(key, absPath, hashA); err != nil || ok {
		t.Fatalf("settle kept a record for bytes no longer on disk: ok=%v err=%v", ok, err)
	}

	if err := restarted.PreparePageState(key, absPath, hashB, NewPageStateRecord(hashC, gen, pageTag1)); err != nil {
		t.Fatal(err)
	}
	third := New(baseDir)
	if err := third.SettlePageState(key, absPath, diskBytes("B")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := third.PageStateFor(key, absPath, hashC); err != nil || ok {
		t.Fatalf("a write that never landed was promoted: ok=%v err=%v", ok, err)
	}
	if got, ok, err := third.PageStateFor(key, absPath, hashB); err != nil || !ok || got.Tag != pageTag2 {
		t.Fatalf("record for the bytes on disk lost: ok=%v record=%+v err=%v", ok, got, err)
	}
}

func TestPageStateSameBytesAdvancesState(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))

	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), NewPageStateRecord(hashA, gen, pageTag1))
	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashA, gen, pageTag2)); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.PageStateFor(key, absPath, hashA)
	if err != nil || !ok {
		t.Fatalf("advanced state not selectable: ok=%v err=%v", ok, err)
	}
	if got.Tag != pageTag2 {
		t.Fatalf("Tag = %q, want the advanced tag", got.Tag)
	}
	ps := readPageStateJSON(t, lookupHistoryDirForTest(t, s, key, absPath))
	if ps.Matching == nil || ps.Matching.Tag != pageTag2 {
		t.Fatalf("matching slot = %+v, want the advanced record", ps.Matching)
	}
	if ps.Candidate != nil {
		t.Fatalf("same-bytes record left a candidate slot: %+v", ps.Candidate)
	}
}

func TestPageStateRejectsInvalidRecords(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	recordA := NewPageStateRecord(hashA, gen, pageTag1)

	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), recordA)
	dir := lookupHistoryDirForTest(t, s, key, absPath)
	before, err := os.ReadFile(filepath.Join(dir, PageStateName))
	if err != nil {
		t.Fatal(err)
	}

	wrongStateHash := recordA
	wrongStateHash.StateHash = Hash([]byte("something else"))
	emptyTag := recordA
	emptyTag.Tag = ""
	emptyTag.StateHash = Hash(nil)
	oversizeTag := NewPageStateRecord(hashA, gen, strings.Repeat("x", MaxPageStateTag+1))
	shortGeneration := NewPageStateRecord(hashA, strings.Repeat("ab", 15), pageTag1)
	badFileHash := NewPageStateRecord("not-hex", gen, pageTag1)
	versionTwo := NewPageStateRecord(hashA, gen, pageTag1)
	versionTwo.Version = 2

	cases := []struct {
		name     string
		record   PageStateRecord
		diskHash string
	}{
		{"wrongStateHash", wrongStateHash, hashA},
		{"emptyTag", emptyTag, hashA},
		{"oversizeTag", oversizeTag, hashA},
		{"shortGeneration", shortGeneration, hashA},
		{"nonHexFileHash", badFileHash, hashA},
		{"versionTwo", versionTwo, hashA},
	}
	for _, tc := range cases {
		if err := s.PreparePageState(key, absPath, tc.diskHash, tc.record); !errors.Is(err, ErrInvalidPageState) {
			t.Fatalf("%s: err = %v, want ErrInvalidPageState", tc.name, err)
		}
		after, err := os.ReadFile(filepath.Join(dir, PageStateName))
		if err != nil {
			t.Fatalf("%s: sidecar unreadable after a rejected record: %v", tc.name, err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("%s: sidecar changed by a rejected record", tc.name)
		}
	}
}

func TestPageStateCorruptSidecarReadsAsNone(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	hashB := Hash(diskBytes("B"))

	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), NewPageStateRecord(hashA, gen, pageTag1))
	dir := lookupHistoryDirForTest(t, s, key, absPath)
	if err := os.WriteFile(filepath.Join(dir, PageStateName), []byte("}{ not json at all"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := s.PageStateFor(key, absPath, hashA); err != nil || ok {
		t.Fatalf("corrupt sidecar selected a record: ok=%v err=%v", ok, err)
	}

	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashB, gen, pageTag2)); err != nil {
		t.Fatal(err)
	}
	ps := readPageStateJSON(t, dir)
	if ps.Matching != nil {
		t.Fatalf("prepare carried a record out of the corrupt sidecar: %+v", ps.Matching)
	}
	if ps.Candidate == nil || ps.Candidate.FileHash != hashB || ps.Candidate.Tag != pageTag2 {
		t.Fatalf("prepare lost the candidate: %+v", ps.Candidate)
	}
	if err := s.SettlePageState(key, absPath, diskBytes("B")); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.PageStateFor(key, absPath, hashB); err != nil || !ok || got.Tag != pageTag2 {
		t.Fatalf("record not usable after a corrupt sidecar was replaced: ok=%v record=%+v err=%v", ok, got, err)
	}
}

func TestPageStateIsNotAVersion(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	hashA := Hash(diskBytes("A"))
	gen := pageGenerationForTest(t)

	created, err := s.Backup(key, absPath, []byte("version one"))
	if err != nil || !created {
		t.Fatalf("backup = %v, %v", created, err)
	}
	backups, err := s.List(key, absPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %d, want 1", len(backups))
	}

	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashA, gen, pageTag1)); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List(key, absPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(backups) || entries[0].Name != backups[0].Name {
		t.Fatalf("List = %+v, want exactly the backup entry", entries)
	}
	for _, e := range entries {
		if e.Name == PageStateName {
			t.Fatalf("sidecar listed as a version")
		}
	}

	pruneHistoryRootForTest(t, s, key, absPath)
	dir := lookupHistoryDirForTest(t, s, key, absPath)
	if _, err := os.Stat(filepath.Join(dir, PageStateName)); err != nil {
		t.Fatalf("pruning removed the sidecar: %v", err)
	}
}

func TestPageStateStaysInsideTheHistory(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	hashOutside := Hash(diskBytes("Outside"))

	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), NewPageStateRecord(hashA, gen, pageTag1))
	dir := lookupHistoryDirForTest(t, s, key, absPath)

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.json")
	outside := pageStateFile{Version: 1, Matching: &PageStateRecord{
		Version:    1,
		FileHash:   hashOutside,
		Generation: gen,
		StateHash:  Hash([]byte(pageTag2)),
		Tag:        pageTag2,
	}}
	outsideData, err := json.Marshal(outside)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath, outsideData, 0600); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(dir, PageStateName)
	if err := os.Remove(sidecarPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, sidecarPath); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := s.PageStateFor(key, absPath, hashOutside); err != nil || ok {
		t.Fatalf("sidecar symlink served an outside record: ok=%v err=%v", ok, err)
	}
	if err := s.PreparePageState(key, absPath, hashA, NewPageStateRecord(hashA, gen, pageTag2)); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(outsideData, after) {
		t.Fatalf("prepare wrote through the symlink to a file outside the store")
	}
	if got, ok, err := s.PageStateFor(key, absPath, hashA); err != nil || !ok || got.Tag != pageTag2 {
		t.Fatalf("prepare did not replace the sidecar with a real file: ok=%v record=%+v err=%v", ok, got, err)
	}
}

func TestPageStateKeysAreSeparate(t *testing.T) {
	s := New(t.TempDir())
	gen := pageGenerationForTest(t)
	pathOne := filepath.Join(t.TempDir(), "one.html")
	pathTwo := filepath.Join(t.TempDir(), "two.html")
	keyOne := Key(pathOne, nil)
	keyTwo := Key(pathTwo, nil)
	hashOne := Hash(diskBytes("One"))
	hashTwo := Hash(diskBytes("Two"))

	settlePageStateForTest(t, s, keyOne, pathOne, diskBytes("One"), NewPageStateRecord(hashOne, gen, pageTag1))
	settlePageStateForTest(t, s, keyTwo, pathTwo, diskBytes("Two"), NewPageStateRecord(hashTwo, gen, pageTag2))

	got, ok, err := s.PageStateFor(keyOne, pathOne, hashOne)
	if err != nil || !ok || got.Tag != pageTag1 {
		t.Fatalf("key one record = %+v ok=%v err=%v", got, ok, err)
	}
	got, ok, err = s.PageStateFor(keyTwo, pathTwo, hashTwo)
	if err != nil || !ok || got.Tag != pageTag2 {
		t.Fatalf("key two record = %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err := s.PageStateFor(keyOne, pathOne, hashTwo); err != nil || ok {
		t.Fatalf("key one saw the other document's record: ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.PageStateFor(keyTwo, pathTwo, hashOne); err != nil || ok {
		t.Fatalf("key two saw the other document's record: ok=%v err=%v", ok, err)
	}
	dirOne := lookupHistoryDirForTest(t, s, keyOne, pathOne)
	dirTwo := lookupHistoryDirForTest(t, s, keyTwo, pathTwo)
	if dirOne == dirTwo {
		t.Fatalf("two keys share a history folder")
	}
	if ps := readPageStateJSON(t, dirOne); ps.Matching == nil || ps.Matching.Tag != pageTag1 {
		t.Fatalf("key one sidecar = %+v", ps.Matching)
	}
	if ps := readPageStateJSON(t, dirTwo); ps.Matching == nil || ps.Matching.Tag != pageTag2 {
		t.Fatalf("key two sidecar = %+v", ps.Matching)
	}
}

// TestPageStateSurvivesAnUnrecognizedWrite is rule F-A: a write with no
// recognized block editor article (a bare pagination article, a document with
// no article) keeps the sidecar, while a write with a different recognized
// article still removes it.
func TestPageStateSurvivesAnUnrecognizedWrite(t *testing.T) {
	s := New(t.TempDir())
	absPath := filepath.Join(t.TempDir(), "doc.html")
	key := Key(absPath, nil)
	gen := pageGenerationForTest(t)
	hashA := Hash(diskBytes("A"))
	settlePageStateForTest(t, s, key, absPath, diskBytes("A"), NewPageStateRecord(hashA, gen, pageTag1))
	sidecar := filepath.Join(lookupHistoryDirForTest(t, s, key, absPath), PageStateName)

	for _, written := range []string{
		`<!doctype html><html><body><article data-page="2">Chapter two</article></body></html>`,
		`<!doctype html><html><body><p>no article</p></body></html>`,
		`<!doctype html><html><body><article data-blockeditor="1"></article></body></html>`,
		`<!doctype html><html><body><template><article data-blockeditor="1" data-rev="x"></article></template></body></html>`,
	} {
		if err := s.SettlePageState(key, absPath, []byte(written)); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sidecar); err != nil {
			t.Fatalf("an unrecognized write removed the sidecar: %v\n%s", err, written)
		}
		if got, ok, err := s.PageStateFor(key, absPath, hashA); err != nil || !ok || got.Tag != pageTag1 {
			t.Fatalf("record lost after an unrecognized write: ok=%v record=%+v err=%v\n%s", ok, got, err, written)
		}
	}

	if err := s.SettlePageState(key, absPath, diskBytes("B")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecar); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a recognized write with no matching record kept the sidecar: %v", err)
	}
}
