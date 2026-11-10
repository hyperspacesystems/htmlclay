//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type pathOutcome struct {
	Input              string `json:"input"`
	DecodeOnce         any    `json:"decodeOnce"`
	ValidateSegments   any    `json:"validateSegments"`
	ResolveRequestRead any    `json:"resolveRequestRead"`
	ResolveReadPath    any    `json:"resolveReadPath"`
	ResolveWritePath   any    `json:"resolveWritePath"`
}

type pathGolden struct {
	Inputs         []pathOutcome `json:"inputs"`
	ConsentedRoots []string      `json:"consentedRoots"`
	Log            []string      `json:"log"`
}

type pathFixture struct {
	tmp      string
	tmpReal  string
	root     string
	baseReal string
	logs     []string
	registry *consentRegistry
}

func TestPathsGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "path-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden pathGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Inputs) == 0 {
		t.Fatal("golden.json holds no inputs")
	}

	fixture := buildPathFixture(t)

	for _, want := range golden.Inputs {
		t.Run(subtestName(want.Input), func(t *testing.T) {
			got := normalizeOutcome(t, collectPathOutcome(fixture, want.Input))
			cases := []struct {
				field string
				got   any
				want  any
			}{
				{"decodeOnce", got.DecodeOnce, want.DecodeOnce},
				{"validateSegments", got.ValidateSegments, want.ValidateSegments},
				{"resolveRequestRead", got.ResolveRequestRead, want.ResolveRequestRead},
				{"resolveReadPath", got.ResolveReadPath, want.ResolveReadPath},
				{"resolveWritePath", got.ResolveWritePath, want.ResolveWritePath},
			}
			for _, testCase := range cases {
				if !reflect.DeepEqual(testCase.got, testCase.want) {
					t.Errorf("%s = %s, want %s", testCase.field, mustJSON(testCase.got), mustJSON(testCase.want))
				}
			}
		})
	}

	var gotRoots []string
	for _, root := range fixture.registry.ConsentedRoots() {
		gotRoots = append(gotRoots, fixture.rel(root))
	}
	if !reflect.DeepEqual(gotRoots, golden.ConsentedRoots) {
		t.Errorf("consentedRoots = %s, want %s", mustJSON(gotRoots), mustJSON(golden.ConsentedRoots))
	}

	var gotLog []string
	for _, line := range fixture.logs {
		gotLog = append(gotLog, fixture.sanitize(line))
	}
	if !reflect.DeepEqual(gotLog, golden.Log) {
		t.Errorf("log = %s, want %s", mustJSON(gotLog), mustJSON(golden.Log))
	}
}

func TestPathsConsentWalkSkips(t *testing.T) {
	tmp := t.TempDir()
	outside := t.TempDir()
	root := filepath.Join(tmp, "root")
	mustMkdirAll(t, root)
	mustMkdirAll(t, filepath.Join(root, "node_modules"))
	mustMkdirAll(t, filepath.Join(root, "sites-versions"))
	mustMkdirAll(t, filepath.Join(root, ".dotdir"))
	mustSymlink(t, outside, filepath.Join(root, ".dot-link"))
	mustSymlink(t, outside, filepath.Join(root, "node_modules", "out-link"))
	mustSymlink(t, outside, filepath.Join(root, "sites-versions", "out-link"))
	mustSymlink(t, outside, filepath.Join(root, ".dotdir", "out-link"))

	var logs []string
	registry := newConsentRegistry(root, func(line string) { logs = append(logs, line) })
	registry.Ready()

	if roots := registry.ConsentedRoots(); len(roots) != 0 {
		t.Errorf("consentedRoots = %v, want none", roots)
	}
	if len(logs) != 0 {
		t.Errorf("log = %v, want none", logs)
	}
}

func TestPathsConsentWalkBudget(t *testing.T) {
	tmp := t.TempDir()
	outside := t.TempDir()
	root := filepath.Join(tmp, "root")
	mustMkdirAll(t, root)
	for i := 0; i < maxConsentEntries; i++ {
		writeFixture(t, filepath.Join(root, fmt.Sprintf("file-%05d", i)), "x")
	}
	mustSymlink(t, outside, filepath.Join(root, "zzz-link"))

	var logs []string
	registry := newConsentRegistry(root, func(line string) { logs = append(logs, line) })
	registry.Ready()

	if roots := registry.ConsentedRoots(); len(roots) != 0 {
		t.Errorf("consentedRoots = %v, want none: the walk must stop at %d entries", roots, maxConsentEntries)
	}
	if len(logs) != 0 {
		t.Errorf("log = %v, want none", logs)
	}
}

func TestPathsConsentWalkDepth(t *testing.T) {
	tmp := t.TempDir()
	outside := t.TempDir()
	root := filepath.Join(tmp, "root")
	mustMkdirAll(t, root)

	deepest := root
	for i := 0; i <= maxConsentDepth; i++ {
		deepest = filepath.Join(deepest, fmt.Sprintf("d%d", i))
		mustMkdirAll(t, deepest)
		if i == maxConsentDepth-1 {
			mustSymlink(t, outside, filepath.Join(deepest, "at-limit-link"))
		}
	}
	mustSymlink(t, outside, filepath.Join(deepest, "too-deep-link"))

	var logs []string
	registry := newConsentRegistry(root, func(line string) { logs = append(logs, line) })
	registry.Ready()

	roots := registry.ConsentedRoots()
	want := []string{mustRealPath(t, outside)}
	if !reflect.DeepEqual(roots, want) {
		t.Errorf("consentedRoots = %v, want %v", roots, want)
	}
	if len(logs) != 1 {
		t.Errorf("log = %v, want one registration", logs)
	}
}

func TestPathsDecodeOnceTable(t *testing.T) {
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"welcome.html", "welcome.html", true},
		{"a%20b", "a b", true},
		{"a+b", "a+b", true},
		{"a%2Fb", "a/b", true},
		{"%2e%2e", "..", true},
		{"%25", "%", true},
		{"%2520", "%20", true},
		{"%00", "\x00", true},
		{"%C3%A9", "é", true},
		{"%F0%9F%8E%89", "🎉", true},
		{"", "", true},
		{"%", "", false},
		{"%2", "", false},
		{"%zz", "", false},
		{"%FF", "", false},
		{"%C3", "", false},
		{"a%C3", "", false},
		{"%ED%A0%80", "", false},
		{"%E0%80%80", "", false},
	}
	for _, testCase := range cases {
		got, err := decodeOnce(testCase.input)
		if testCase.ok {
			if err != nil {
				t.Errorf("decodeOnce(%q) error = %v, want %q", testCase.input, err, testCase.want)
				continue
			}
			if got != testCase.want {
				t.Errorf("decodeOnce(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("decodeOnce(%q) = %q, want an error", testCase.input, got)
		}
	}
}

func TestPathsValidateSegmentsTable(t *testing.T) {
	cases := []struct {
		relPath string
		status  int
	}{
		{"welcome.html", 0},
		{"sub/report.txt", 0},
		{"sub//report.txt", 0},
		{"Folder With Space/index.html", 0},
		{"", 400},
		{"/welcome.html", 400},
		{"\\welcome.html", 400},
		{"..", 400},
		{"../notes.txt", 400},
		{"sub/../notes.txt", 400},
		{"./notes.txt", 400},
		{"a\x00b", 400},
		{strings.Repeat("x", 256), 400},
		{".hidden", 404},
		{"sub/.hidden", 404},
		{"sites-versions/welcome.html", 404},
		{"sites-versions", 404},
		{"node_modules/pkg/index.js", 0},
	}
	for _, testCase := range cases {
		_, err := validateSegments(testCase.relPath)
		if testCase.status == 0 {
			if err != nil {
				t.Errorf("validateSegments(%q) error = %v, want success", testCase.relPath, err)
			}
			continue
		}
		var pathError *PathError
		if !errors.As(err, &pathError) {
			t.Errorf("validateSegments(%q) error = %v, want a PathError", testCase.relPath, err)
			continue
		}
		if pathError.Status != testCase.status {
			t.Errorf("validateSegments(%q) status = %d, want %d", testCase.relPath, pathError.Status, testCase.status)
		}
	}
}

func buildPathFixture(t *testing.T) *pathFixture {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	sibling := filepath.Join(tmp, "sibling")

	mustMkdirAll(t, root)
	mustMkdirAll(t, sibling)

	writeFixture(t, filepath.Join(root, "welcome.html"), "<h1>welcome</h1>\n")
	writeFixture(t, filepath.Join(root, "notes.txt"), "notes\n")
	writeFixture(t, filepath.Join(root, ".hidden"), "dotfile\n")
	writeFixture(t, filepath.Join(root, "sub", "report.txt"), "report\n")
	writeFixture(t, filepath.Join(root, "Folder With Space", "index.html"), "<p>spaced</p>\n")
	writeFixture(t, filepath.Join(root, "caf\u00e9", "menu.html"), "<p>menu</p>\n")
	writeFixture(t, filepath.Join(root, "sites-versions", "welcome.html"), "<p>old</p>\n")
	writeFixture(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "module.exports = 1\n")
	writeFixture(t, filepath.Join(root, "target", "inside.txt"), "inside\n")
	writeFixture(t, filepath.Join(sibling, "outside.txt"), "outside\n")

	mustSymlink(t, "target", filepath.Join(root, "in-link"))
	mustSymlink(t, "../sibling", filepath.Join(root, "out-link"))
	mustSymlink(t, "loop-link", filepath.Join(root, "loop-link"))
	mustSymlink(t, "does-not-exist.txt", filepath.Join(root, "dangling-link"))

	baseReal := mustRealPath(t, root)

	logs := []string{}
	registry := newConsentRegistry(root, func(line string) { logs = append(logs, line) })
	registry.Ready()

	late := filepath.Join(tmp, "late")
	mustMkdirAll(t, late)
	writeFixture(t, filepath.Join(late, "outside.txt"), "late\n")
	mustSymlink(t, "../late", filepath.Join(root, "late-link"))

	if logs == nil {
		t.Fatal("no registration log recorded")
	}
	return &pathFixture{
		tmp:      tmp,
		tmpReal:  mustRealPath(t, tmp),
		root:     root,
		baseReal: baseReal,
		logs:     logs,
		registry: registry,
	}
}

func collectPathOutcome(fixture *pathFixture, input string) pathOutcome {
	outcome := pathOutcome{Input: input}
	outcome.DecodeOnce = pathOutcomeOf(func() (any, error) {
		return decodeOnce(input)
	})
	outcome.ValidateSegments = pathOutcomeOf(func() (any, error) {
		return validateSegments(input)
	})
	outcome.ResolveRequestRead = pathOutcomeOf(func() (any, error) {
		result, err := resolveRequestRead(fixture.registry, input)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"relPath":  result.RelPath,
			"realPath": fixture.rel(result.RealPath),
		}, nil
	})
	outcome.ResolveReadPath = pathOutcomeOf(func() (any, error) {
		realPath, err := resolveReadPath(fixture.registry, input)
		if err != nil {
			return nil, err
		}
		return fixture.rel(realPath), nil
	})
	outcome.ResolveWritePath = pathOutcomeOf(func() (any, error) {
		realPath, err := resolveWritePath(fixture.registry, input)
		if err != nil {
			return nil, err
		}
		return fixture.rel(realPath), nil
	})
	return outcome
}

func pathOutcomeOf(call func() (any, error)) any {
	value, err := call()
	if err == nil {
		return value
	}
	var pathError *PathError
	if errors.As(err, &pathError) {
		return map[string]any{"status": pathError.Status, "message": pathError.Message}
	}
	return map[string]any{"thrown": "Error"}
}

func normalizeOutcome(t *testing.T, outcome pathOutcome) pathOutcome {
	t.Helper()
	raw, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	var normalized pathOutcome
	if err := json.Unmarshal(raw, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}

func (f *pathFixture) rel(target string) string {
	relative, err := filepath.Rel(f.baseReal, target)
	if err != nil {
		return target
	}
	if relative == "." {
		return ""
	}
	return toUnix(relative)
}

func (f *pathFixture) sanitize(line string) string {
	line = strings.ReplaceAll(line, f.tmpReal, "<tmp>")
	return strings.ReplaceAll(line, f.tmp, "<tmp>")
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

func subtestName(input string) string {
	if len(input) > 40 {
		return input[:40] + "..."
	}
	if input == "" {
		return "empty"
	}
	return input
}

func writeFixture(t *testing.T, target, body string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(target))
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func mustRealPath(t *testing.T, target string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	return real
}
