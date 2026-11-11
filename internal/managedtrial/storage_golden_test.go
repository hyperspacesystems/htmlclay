//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The golden answers come from Local's own write-queue.js and api-sidecar.js,
// recorded by testdata/storage-golden/gen.cjs. Every scenario is rebuilt here and
// compared exactly: the files each one leaves behind (path, kind, mode, size,
// digest and text), the return value or error code, and the non-fatal console
// failure each one reports.

type storageGolden struct {
	Constants storageGoldenConstants   `json:"constants"`
	Atomic    []storageAtomicScenario  `json:"atomic"`
	Sidecar   []storageSidecarScenario `json:"sidecar"`
	Paths     []storagePathRow         `json:"paths"`
}

type storageGoldenConstants struct {
	SidecarDir         string   `json:"sidecarDir"`
	TempMode           int      `json:"tempMode"`
	DefaultMode        int      `json:"defaultMode"`
	RenameAttempts     int      `json:"renameAttempts"`
	RenameRetryCodes   []string `json:"renameRetryCodes"`
	DirSyncUnsupported []string `json:"dirSyncUnsupported"`
}

type storageSetup struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Mode       int    `json:"mode"`
	ContentB64 string `json:"contentB64"`
	Target     string `json:"target"`
	MtimeMs    int64  `json:"mtimeMs"`
}

type storageAtomicAction struct {
	Path         string `json:"path"`
	Encoding     string `json:"encoding"`
	ContentB64   string `json:"contentB64"`
	BeforeRename string `json:"beforeRename"`
	ObserveTemp  bool   `json:"observeTemp"`
}

type storageTemp struct {
	Shape string `json:"shape"`
	Base  string `json:"base"`
	Mode  int    `json:"mode"`
}

type storageEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Mode   int    `json:"mode"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
	Text   string `json:"text"`
	Target string `json:"target"`
}

type storageConsoleLine struct {
	Level  string `json:"level"`
	Prefix string `json:"prefix"`
}

type storageAtomicScenario struct {
	Name     string               `json:"name"`
	Setup    []storageSetup       `json:"setup"`
	Action   storageAtomicAction  `json:"action"`
	Result   *string              `json:"result"`
	Temp     *storageTemp         `json:"temp"`
	TempLeft bool                 `json:"tempLeft"`
	Console  []storageConsoleLine `json:"console"`
	Entries  []storageEntry       `json:"entries"`
}

type storageSidecarScenario struct {
	Name    string               `json:"name"`
	Setup   []storageSetup       `json:"setup"`
	Actions []storageSidecarStep `json:"actions"`
	Results []json.RawMessage    `json:"results"`
	Console []storageConsoleLine `json:"console"`
	Entries []storageEntry       `json:"entries"`
}

type storageSidecarStep struct {
	Kind          string  `json:"kind"`
	Name          string  `json:"name"`
	HTML          string  `json:"html"`
	DataText      *string `json:"dataText"`
	Path          string  `json:"path"`
	MtimeMs       int64   `json:"mtimeMs"`
	SourceMtimeMs float64 `json:"sourceMtimeMs"`
}

type storagePathRow struct {
	Name      string  `json:"name"`
	RelPath   *string `json:"relPath"`
	Lexical   *string `json:"lexical"`
	Canonical *string `json:"canonical"`
	Error     string  `json:"error"`
}

func TestStorageAtomicGolden(t *testing.T) {
	golden := loadStorageGolden(t)
	checkStorageConstants(t, golden.Constants)

	if len(golden.Atomic) == 0 {
		t.Fatal("golden.json holds no atomic scenarios")
	}
	for _, want := range golden.Atomic {
		t.Run(want.Name, func(t *testing.T) {
			base := storageScenarioRoot(t, want.Name)
			applyStorageSetup(t, base, want.Setup)
			recorder := captureSidecarConsole(t)

			content := storageBytes(t, want.Action.ContentB64)
			var observed *storageTemp
			var beforeRename func() error
			switch {
			case want.Action.BeforeRename == "throw":
				beforeRename = func() error { return errors.New("blocked by test") }
			case want.Action.ObserveTemp:
				beforeRename = func() error {
					temp, err := observeAtomicTemp(base, want.Action.Path)
					observed = temp
					return err
				}
			}

			err := atomicWriteFile(filepath.Join(base, filepath.FromSlash(want.Action.Path)), content, beforeRename, false)
			gotResult := storageResult(err)
			if !reflect.DeepEqual(gotResult, want.Result) {
				if storageSameRefusal(gotResult, want.Result) {
					t.Logf("refused as %v where Local reports %v: same refusal, different syscall path",
						storageResultText(gotResult), storageResultText(want.Result))
				} else {
					t.Errorf("result = %v, want %v", storageResultText(gotResult), storageResultText(want.Result))
				}
			}

			compareStorageTemp(t, observed, want.Temp)
			entries := recordStorageEntries(t, base)
			tempLeft := false
			for _, entry := range entries {
				if strings.HasSuffix(entry.Path, ".tmp") {
					tempLeft = true
				}
			}
			if tempLeft != want.TempLeft {
				t.Errorf("tempLeft = %v, want %v", tempLeft, want.TempLeft)
			}
			compareStorageEntries(t, "entries", entries, want.Entries)
			compareStorageJSON(t, "console", recorder.lines, mustStorageJSON(want.Console))
		})
	}
}

func TestStorageSidecarGolden(t *testing.T) {
	golden := loadStorageGolden(t)
	checkStorageConstants(t, golden.Constants)

	if len(golden.Sidecar) == 0 {
		t.Fatal("golden.json holds no sidecar scenarios")
	}
	for _, want := range golden.Sidecar {
		t.Run(want.Name, func(t *testing.T) {
			scenario := storageScenarioRoot(t, want.Name)
			base := filepath.Join(scenario, "root")
			mustStorageMkdir(t, base)
			applyStorageSetup(t, scenario, want.Setup)
			recorder := captureSidecarConsole(t)

			for i, action := range want.Actions {
				got := runStorageSidecarStep(t, scenario, base, action)
				compareStorageJSON(t, "results["+action.Kind+"]", got, want.Results[i])
			}

			compareStorageJSON(t, "console", recorder.lines, mustStorageJSON(want.Console))
			compareStorageEntries(t, "entries", recordStorageEntries(t, scenario), want.Entries)
		})
	}
}

func TestStorageSidecarPaths(t *testing.T) {
	golden := loadStorageGolden(t)
	if len(golden.Paths) == 0 {
		t.Fatal("golden.json holds no path rows")
	}
	root := storageScenarioRoot(t, "paths")
	base := filepath.Join(root, "root")
	mustStorageMkdir(t, base)

	for _, want := range golden.Paths {
		t.Run(want.Name, func(t *testing.T) {
			got := map[string]any{
				"name":      want.Name,
				"relPath":   sidecarRelPath(want.Name),
				"lexical":   nil,
				"canonical": nil,
			}
			lexical, err := resolveSidecarPath(base, want.Name)
			if err == nil {
				got["lexical"] = storageRelPath(t, base, lexical)
				canonical, canonicalErr := resolveSidecarCanonical(base, want.Name)
				if canonicalErr != nil {
					err = canonicalErr
				} else {
					got["canonical"] = storageRelPath(t, base, canonical)
				}
			}
			if err != nil {
				got["error"] = err.Error()
			}
			expected := map[string]any{
				"name":      want.Name,
				"relPath":   want.RelPath,
				"lexical":   want.Lexical,
				"canonical": want.Canonical,
			}
			if want.Error != "" {
				expected["error"] = want.Error
			}
			compareStorageJSON(t, "path", got, mustStorageJSON(expected))
		})
	}
}

func loadStorageGolden(t *testing.T) storageGolden {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "storage-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden storageGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func checkStorageConstants(t *testing.T, want storageGoldenConstants) {
	t.Helper()
	cases := []struct {
		field string
		got   any
		want  any
	}{
		{"sidecarDir", sidecarDirName, want.SidecarDir},
		{"tempMode", int(atomicTempMode), want.TempMode},
		{"defaultMode", int(atomicDefaultMode), want.DefaultMode},
		{"renameAttempts", atomicRenameAttempts, want.RenameAttempts},
		{"renameRetryCodes", errnoNames(atomicRenameRetryCodes), want.RenameRetryCodes},
		{"dirSyncUnsupported", errnoNames(atomicDirSyncUnsupportedCodes), want.DirSyncUnsupported},
	}
	for _, testCase := range cases {
		if !reflect.DeepEqual(testCase.got, testCase.want) {
			t.Errorf("%s = %v, want %v", testCase.field, testCase.got, testCase.want)
		}
	}
}

// errnoNames is the portable identity of a code list: the names, sorted, so a
// recorded list compares regardless of the order the port declares them in.
func errnoNames(codes []syscall.Errno) []string {
	names := make([]string, 0, len(codes))
	for _, code := range codes {
		names = append(names, errnoName(code))
	}
	sort.Strings(names)
	return names
}

func errnoName(errno syscall.Errno) string {
	if runtime.GOOS == "windows" {
		switch errno {
		case 3: // ERROR_PATH_NOT_FOUND, which libuv names ENOENT
			return "ENOENT"
		case 5: // ERROR_ACCESS_DENIED, which libuv names EPERM
			return "EPERM"
		}
	}
	switch errno {
	case syscall.EPERM:
		return "EPERM"
	case syscall.EBUSY:
		return "EBUSY"
	case syscall.EACCES:
		return "EACCES"
	case syscall.EISDIR:
		return "EISDIR"
	case syscall.EINVAL:
		return "EINVAL"
	case syscall.ENOTSUP:
		return "ENOTSUP"
	case syscall.EBADF:
		return "EBADF"
	case syscall.ENOENT:
		return "ENOENT"
	case syscall.ENOTDIR:
		return "ENOTDIR"
	case syscall.EEXIST:
		return "EEXIST"
	case syscall.ELOOP:
		return "ELOOP"
	}
	return "ERRNO"
}

// storageResult is the scenario's answer: null on success, otherwise the error's
// portable identity — its errno code, or its message when it has no code.
func storageResult(err error) *string {
	if err == nil {
		return nil
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		name := errnoName(errno)
		return &name
	}
	message := err.Error()
	return &message
}

// storageSameRefusal reports whether two recorded error codes describe the same
// refusal. Renaming a file onto an existing directory is refused with EISDIR by
// Node's libuv and with EEXIST by Go on macOS: the write fails, the target keeps
// its contents and no temp is left either way, so only the errno differs. On
// Windows both Node and Go are refused with ERROR_ACCESS_DENIED, which libuv
// names EPERM.
func storageSameRefusal(got, want *string) bool {
	if got == nil || want == nil {
		return false
	}
	renameOntoDirectory := map[string]bool{"EISDIR": true, "EEXIST": true}
	if runtime.GOOS == "windows" {
		renameOntoDirectory["EPERM"] = true
	}
	return renameOntoDirectory[*got] && renameOntoDirectory[*want]
}

func storageResultText(result *string) any {
	if result == nil {
		return nil
	}
	return *result
}

func storageScenarioRoot(t *testing.T, name string) string {
	t.Helper()
	raw := t.TempDir()
	root, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func mustStorageMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func applyStorageSetup(t *testing.T, base string, setup []storageSetup) {
	t.Helper()
	for _, op := range setup {
		full := filepath.Join(base, filepath.FromSlash(op.Path))
		switch op.Kind {
		case "dir":
			mustStorageMkdir(t, full)
			if op.Mode != 0 {
				if err := os.Chmod(full, os.FileMode(op.Mode)); err != nil {
					t.Fatal(err)
				}
			}
		case "symlink":
			mustStorageMkdir(t, filepath.Dir(full))
			if err := os.Symlink(filepath.FromSlash(op.Target), full); err != nil {
				t.Fatal(err)
			}
		default:
			mustStorageMkdir(t, filepath.Dir(full))
			mode := os.FileMode(0o644)
			if op.Mode != 0 {
				mode = os.FileMode(op.Mode)
			}
			if err := os.WriteFile(full, storageBytes(t, op.ContentB64), mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(full, mode); err != nil {
				t.Fatal(err)
			}
			if op.MtimeMs != 0 {
				stamp := time.UnixMilli(op.MtimeMs)
				if err := os.Chtimes(full, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func storageBytes(t *testing.T, contentB64 string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		t.Fatalf("decode content: %v", err)
	}
	return data
}

// observeAtomicTemp reads the temp file the port left beside the target, the way
// the generator's beforeRename hook does: the name shape with the pid and the
// process-wide counter replaced, plus the mode it carries at publication time.
func observeAtomicTemp(base, target string) (*storageTemp, error) {
	parent := filepath.Join(base, filepath.FromSlash(filepath.Dir(target)))
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		trimmed := strings.TrimPrefix(name, ".")
		trimmed = strings.TrimSuffix(trimmed, ".tmp")
		parts := strings.Split(trimmed, ".")
		if len(parts) < 3 {
			return nil, errors.New("unexpected temp name " + name)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		return &storageTemp{
			Shape: ".<base>.<pid>.<n>.tmp",
			Base:  strings.Join(parts[:len(parts)-2], "."),
			Mode:  int(info.Mode() & os.ModePerm),
		}, nil
	}
	return nil, errors.New("no temp file observed")
}

func recordStorageEntries(t *testing.T, base string) []storageEntry {
	t.Helper()
	entries := []storageEntry{}
	err := filepath.WalkDir(base, func(target string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(base, target)
		if relErr != nil || rel == "." {
			return nil
		}
		recorded := storageEntry{Path: filepath.ToSlash(rel)}
		info, infoErr := os.Lstat(target)
		if infoErr != nil {
			return nil
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, linkErr := os.Readlink(target)
			if linkErr != nil {
				return nil
			}
			recorded.Kind = "symlink"
			recorded.Target = filepath.ToSlash(linkTarget)
		case info.IsDir():
			recorded.Kind = "dir"
		default:
			data, readErr := os.ReadFile(target)
			if readErr != nil {
				return nil
			}
			sum := sha256.Sum256(data)
			recorded.Kind = "file"
			recorded.Mode = int(info.Mode() & os.ModePerm)
			recorded.Size = int64(len(data))
			recorded.Sha256 = hex.EncodeToString(sum[:])
			recorded.Text = string(data)
		}
		entries = append(entries, recorded)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func storageRelPath(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("relative %s: %v", target, err)
	}
	return filepath.ToSlash(rel)
}

func runStorageSidecarStep(t *testing.T, scenario, base string, step storageSidecarStep) any {
	t.Helper()
	switch step.Kind {
	case "writeSidecar":
		writeAPISidecar(base, step.Name, step.HTML)
		return nil
	case "writeSidecarData":
		var data []byte
		if step.DataText != nil {
			data = []byte(*step.DataText)
		}
		writeAPISidecarData(base, step.Name, data)
		return nil
	case "deleteSidecar":
		deleteAPISidecar(base, step.Name)
		return nil
	case "readFresh":
		text, ok := readFreshSidecar(base, step.Name, step.SourceMtimeMs)
		if !ok {
			return map[string]any{"text": nil}
		}
		return map[string]any{"text": text}
	case "setTimes":
		stamp := time.UnixMilli(step.MtimeMs)
		target := filepath.Join(base, filepath.FromSlash(step.Path))
		if err := os.Chtimes(target, stamp, stamp); err != nil {
			t.Fatalf("set times on %s: %v", step.Path, err)
		}
		return nil
	case "resolve":
		canonical, err := resolveSidecarCanonical(base, step.Name)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"path": storageRelPath(t, scenario, canonical)}
	}
	t.Fatalf("unknown sidecar action kind %q", step.Kind)
	return nil
}

// storageConsoleRecorder collects the non-fatal console lines the port reports.
type storageConsoleRecorder struct {
	lines []storageConsoleLine
}

func captureSidecarConsole(t *testing.T) *storageConsoleRecorder {
	t.Helper()
	recorder := &storageConsoleRecorder{lines: []storageConsoleLine{}}
	original := sidecarConsole
	sidecarConsole = func(level, message string) {
		recorder.lines = append(recorder.lines, storageConsoleLine{
			Level:  level,
			Prefix: storageConsolePrefix(message),
		})
	}
	t.Cleanup(func() { sidecarConsole = original })
	return recorder
}

// storageConsolePrefix is the generator's consolePrefix: the failure sentence a
// non-fatal path reports, without the platform's errno text.
func storageConsolePrefix(message string) string {
	const marker = " failed (non-fatal)"
	if at := strings.Index(message, marker); at >= 0 {
		return message[:at+len(marker)]
	}
	return message
}

func mustStorageJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

// compareStorageEntries compares the files a scenario left behind. Windows has
// no Unix permission bits, so the mode is the one field left out there.
func compareStorageEntries(t *testing.T, field string, got, want []storageEntry) {
	t.Helper()
	if runtime.GOOS == "windows" {
		got = storageEntriesWithoutMode(got)
		want = storageEntriesWithoutMode(want)
	}
	compareStorageJSON(t, field, got, mustStorageJSON(want))
}

func storageEntriesWithoutMode(entries []storageEntry) []storageEntry {
	trimmed := make([]storageEntry, len(entries))
	copy(trimmed, entries)
	for i := range trimmed {
		trimmed[i].Mode = 0
	}
	return trimmed
}

// compareStorageTemp compares the observed temp file the same way.
func compareStorageTemp(t *testing.T, got, want *storageTemp) {
	t.Helper()
	if runtime.GOOS == "windows" {
		got = storageTempWithoutMode(got)
		want = storageTempWithoutMode(want)
	}
	compareStorageJSON(t, "temp", got, mustStorageJSON(want))
}

func storageTempWithoutMode(temp *storageTemp) *storageTemp {
	if temp == nil {
		return nil
	}
	trimmed := *temp
	trimmed.Mode = 0
	return &trimmed
}

func compareStorageJSON(t *testing.T, field string, got any, want json.RawMessage) {
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
