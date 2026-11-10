//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type versionGolden struct {
	Zone       string                           `json:"zone"`
	SecondZone string                           `json:"secondZone"`
	Timestamps map[string][]versionTimestampRow `json:"timestamps"`
	Constants  versionConstants                 `json:"constants"`
	Scenarios  []versionScenario                `json:"scenarios"`
}

type versionTimestampRow struct {
	Instant int64  `json:"instant"`
	Stamp   string `json:"stamp"`
}

type versionConstants struct {
	VersionsDir      string `json:"versionsDir"`
	RecoveredDir     string `json:"recoveredDir"`
	MaxAgeMs         int64  `json:"maxAgeMs"`
	KeepNewest       int    `json:"keepNewest"`
	VersionName      string `json:"versionName"`
	VersionNameFlags string `json:"versionNameFlags"`
}

type versionSetup struct {
	Op         string `json:"op"`
	Path       string `json:"path"`
	ContentB64 string `json:"contentB64"`
	Target     string `json:"target"`
	MtimeMs    int64  `json:"mtimeMs"`
}

type versionAction struct {
	Kind        string   `json:"kind"`
	Clock       *int64   `json:"clock"`
	Site        string   `json:"site"`
	ContentB64  string   `json:"contentB64"`
	FilePath    string   `json:"filePath"`
	Upload      string   `json:"upload"`
	Rel         string   `json:"rel"`
	HTML        string   `json:"html"`
	VersionsDir string   `json:"versionsDir"`
	Dir         string   `json:"dir"`
	Now         int64    `json:"now"`
	Filenames   []string `json:"filenames"`
}

type versionResult struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

type versionParseRow struct {
	Filename  string  `json:"filename"`
	MtimeMs   int64   `json:"mtimeMs"`
	IsVersion bool    `json:"isVersion"`
	Parsed    *int64  `json:"parsed"`
	SortKey   int64   `json:"sortKey"`
	Suffix    int64   `json:"suffix"`
	Stamp     *string `json:"stamp"`
}

type versionGoldenFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}

type versionCall struct {
	Event   string         `json:"event,omitempty"`
	Level   string         `json:"level,omitempty"`
	Tag     string         `json:"tag,omitempty"`
	Message string         `json:"message,omitempty"`
	Fields  []VersionField `json:"fields,omitempty"`
}

type versionScenario struct {
	Name    string              `json:"name"`
	Setup   []versionSetup      `json:"setup"`
	Actions []versionAction     `json:"actions"`
	Results []versionResult     `json:"results"`
	Console []versionCall       `json:"console"`
	Emit    []versionCall       `json:"emit"`
	Log     []versionCall       `json:"log"`
	Files   []versionGoldenFile `json:"files"`
}

type versionRecorder struct {
	emit    []versionCall
	log     []versionCall
	console []versionCall
}

func TestVersionsGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "versions-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden versionGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Scenarios) == 0 {
		t.Fatal("golden.json holds no scenarios")
	}
	checkVersionConstants(t, golden.Constants)

	locations := map[string]*time.Location{}
	for _, zone := range []string{golden.Zone, golden.SecondZone} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatalf("LoadLocation(%q): %v", zone, err)
		}
		locations[zone] = loc
	}

	originalLocal := time.Local
	originalClock := versionClock
	defer func() {
		time.Local = originalLocal
		versionClock = originalClock
	}()
	time.Local = locations[golden.Zone]

	checkVersionTimestamps(t, golden, locations)

	for _, want := range golden.Scenarios {
		t.Run(want.Name, func(t *testing.T) {
			rawTmp := t.TempDir()
			tmp, err := filepath.EvalSymlinks(rawTmp)
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(tmp, want.Name)
			mustMkdirAll(t, base)
			applyVersionSetup(t, base, want.Setup)

			lastPruneMu.Lock()
			lastPruneAt = map[string]int64{}
			lastPruneMu.Unlock()

			recorder := &versionRecorder{emit: []versionCall{}, log: []versionCall{}, console: []versionCall{}}
			observer := &VersionObserver{
				Emit: func(event string, fields []VersionField) {
					recorder.emit = append(recorder.emit, versionCall{Event: event, Fields: fields})
				},
				Log: func(level, tag, message string, fields []VersionField) {
					recorder.log = append(recorder.log, versionCall{Level: level, Tag: tag, Message: message, Fields: fields})
				},
				Console: func(level, message string) {
					recorder.console = append(recorder.console, versionCall{Level: level, Message: message})
				},
			}

			for i, action := range want.Actions {
				if action.Clock != nil {
					clock := *action.Clock
					versionClock = func() int64 { return clock }
				}
				got := runVersionAction(t, base, action, observer)
				compareVersionJSON(t, action.Kind, got, want.Results[i].Value)
			}

			compareVersionJSON(t, "console", normalizeVersionCalls(recorder.console, base), mustJSONRaw(want.Console))
			compareVersionJSON(t, "emit", normalizeVersionCalls(recorder.emit, base), mustJSONRaw(want.Emit))
			compareVersionJSON(t, "log", normalizeVersionCalls(recorder.log, base), mustJSONRaw(want.Log))
			compareVersionJSON(t, "files", listVersionFiles(t, base), mustJSONRaw(want.Files))
		})
	}
}

func checkVersionConstants(t *testing.T, want versionConstants) {
	t.Helper()
	cases := []struct {
		field string
		got   any
		want  any
	}{
		{"versionsDir", versionsDirName, want.VersionsDir},
		{"recoveredDir", recoveredDirName, want.RecoveredDir},
		{"maxAgeMs", maxAgeMS, want.MaxAgeMs},
		{"keepNewest", keepNewest, want.KeepNewest},
		{"versionName", versionNameRe.String(), want.VersionName},
		{"versionNameFlags", "", want.VersionNameFlags},
	}
	for _, testCase := range cases {
		if !reflect.DeepEqual(testCase.got, testCase.want) {
			t.Errorf("%s = %v, want %v", testCase.field, testCase.got, testCase.want)
		}
	}
}

func checkVersionTimestamps(t *testing.T, golden versionGolden, locations map[string]*time.Location) {
	t.Helper()
	for _, zone := range []string{golden.Zone, golden.SecondZone} {
		rows := golden.Timestamps[zone]
		if len(rows) == 0 {
			t.Fatalf("golden.json holds no timestamps for %s", zone)
		}
		for _, row := range rows {
			now := time.UnixMilli(row.Instant).In(locations[zone])
			if got := GenerateTimestamp(now); got != row.Stamp {
				t.Errorf("%s GenerateTimestamp(%d) = %s, want %s", zone, row.Instant, got, row.Stamp)
			}
		}
	}
}

func applyVersionSetup(t *testing.T, base string, ops []versionSetup) {
	t.Helper()
	for _, op := range ops {
		target := filepath.Join(base, filepath.FromSlash(op.Path))
		switch op.Op {
		case "mkdir":
			mustMkdirAll(t, target)
		case "write":
			mustMkdirAll(t, filepath.Dir(target))
			data, err := base64.StdEncoding.DecodeString(op.ContentB64)
			if err != nil {
				t.Fatalf("setup write %s: %v", op.Path, err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("setup write %s: %v", op.Path, err)
			}
		case "symlink":
			mustMkdirAll(t, filepath.Dir(target))
			mustSymlink(t, filepath.FromSlash(op.Target), target)
		case "mtime":
			when := time.UnixMilli(op.MtimeMs)
			if err := os.Chtimes(target, when, when); err != nil {
				t.Fatalf("setup mtime %s: %v", op.Path, err)
			}
		default:
			t.Fatalf("unknown setup op %q", op.Op)
		}
	}
}

func runVersionAction(t *testing.T, base string, action versionAction, observer *VersionObserver) any {
	t.Helper()
	switch action.Kind {
	case "createBackup":
		published := CreateBackup(base, action.Site, decodeVersionContent(t, action.ContentB64), observer)
		return normalizeVersionPointer(base, published)
	case "createBackupIfExists":
		published := CreateBackupIfExists(filepath.Join(base, filepath.FromSlash(action.FilePath)), action.Site, base, observer)
		return normalizeVersionPointer(base, published)
	case "createBinaryBackup":
		published := CreateBinaryBackup(base, action.Upload, decodeVersionContent(t, action.ContentB64), observer)
		return normalizeVersionPointer(base, published)
	case "createBinaryBackupIfExists":
		published := CreateBinaryBackupIfExists(filepath.Join(base, filepath.FromSlash(action.FilePath)), action.Upload, base, observer)
		return normalizeVersionPointer(base, published)
	case "publishRecoveredCopy":
		published, err := PublishRecoveredCopy(base, action.Rel, action.HTML)
		if err != nil {
			return map[string]any{"error": normalizeVersionText(err.Error(), base), "filename": nil, "full": nil}
		}
		return map[string]any{"error": nil, "filename": published.Filename, "full": normalizeVersionText(published.Full, base)}
	case "assertVersionsChain":
		err := AssertVersionsChain(base, filepath.Join(base, filepath.FromSlash(action.VersionsDir)))
		if err != nil {
			return map[string]any{"error": normalizeVersionText(err.Error(), base)}
		}
		return map[string]any{"error": nil}
	case "pruneSiteVersions":
		result := PruneSiteVersions(base, filepath.Join(base, filepath.FromSlash(action.Dir)), time.UnixMilli(action.Now), observer)
		return map[string]any{"kept": result.Kept, "deleted": result.Deleted}
	case "pruneAllVersions":
		result := PruneAllVersions(base, time.UnixMilli(action.Now), observer)
		return map[string]any{"sites": result.Sites, "deleted": result.Deleted}
	case "parse":
		return map[string]any{"rows": parseVersionRows(t, base, action.Dir, action.Filenames)}
	}
	t.Fatalf("unknown action kind %q", action.Kind)
	return nil
}

func parseVersionRows(t *testing.T, base, dir string, filenames []string) []versionParseRow {
	t.Helper()
	rows := make([]versionParseRow, 0, len(filenames))
	for _, filename := range filenames {
		info, err := os.Stat(filepath.Join(base, filepath.FromSlash(dir), filename))
		if err != nil {
			t.Fatalf("parse stat %s: %v", filename, err)
		}
		entry := VersionEntry{Name: filename, MtimeMs: info.ModTime().UnixMilli()}
		row := versionParseRow{
			Filename:  filename,
			MtimeMs:   entry.MtimeMs,
			IsVersion: IsVersionName(filename),
			SortKey:   VersionSortKey(entry),
			Suffix:    CollisionSuffix(filename),
		}
		if parsed, ok := ParseVersionTimestamp(filename); ok {
			row.Parsed = &parsed
		}
		if stamp, ok := VersionStamp(filename); ok {
			row.Stamp = &stamp
		}
		rows = append(rows, row)
	}
	return rows
}

func listVersionFiles(t *testing.T, root string) []versionGoldenFile {
	t.Helper()
	files := []versionGoldenFile{}
	err := filepath.WalkDir(root, func(target string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(target)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, target)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		files = append(files, versionGoldenFile{
			Path:   filepath.ToSlash(rel),
			Size:   int64(len(data)),
			Sha256: hex.EncodeToString(sum[:]),
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func normalizeVersionPointer(base string, published *PublishedVersion) any {
	if published == nil {
		return nil
	}
	return normalizeVersionText(published.Full, base)
}

func normalizeVersionText(value, base string) string {
	return strings.ReplaceAll(value, base, "<root>")
}

func normalizeVersionCalls(calls []versionCall, base string) []versionCall {
	out := make([]versionCall, 0, len(calls))
	for _, call := range calls {
		call.Message = normalizeVersionText(call.Message, base)
		if call.Fields != nil {
			fields := make([]VersionField, len(call.Fields))
			for i, field := range call.Fields {
				fields[i] = VersionField{Key: field.Key, Value: normalizeVersionText(field.Value, base)}
			}
			call.Fields = fields
		}
		out = append(out, call)
	}
	return out
}

func decodeVersionContent(t *testing.T, contentB64 string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		t.Fatalf("decode content: %v", err)
	}
	return data
}

func mustJSONRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func compareVersionJSON(t *testing.T, field string, got any, want json.RawMessage) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%s = %s, want %s", field, gotJSON, want)
	}
}
