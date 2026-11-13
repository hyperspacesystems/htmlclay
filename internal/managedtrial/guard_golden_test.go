//go:build managedtrial

package managedtrial

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/panphora/htmlclay/internal/dataguard"
)

type guardGoldenFile struct {
	Stubs     map[string]string     `json:"stubs"`
	Scenarios []guardGoldenScenario `json:"scenarios"`
}

type guardGoldenScenario struct {
	Name  string                `json:"name"`
	Seed  []guardGoldenFilePair `json:"seed"`
	Steps []guardGoldenStep     `json:"steps"`
	Calls []json.RawMessage     `json:"calls"`
	Tree  []guardGoldenFilePair `json:"tree"`
}

type guardGoldenFilePair struct {
	Path string `json:"path"`
	B64  string `json:"b64"`
}

// guardGoldenStep is one guard call. Only the fields its fn needs are present.
type guardGoldenStep struct {
	Fn   string              `json:"fn"`
	Args guardGoldenStepArgs `json:"args"`
}

type guardGoldenStepArgs struct {
	Name        string  `json:"name"`
	NewHTML     string  `json:"newHtml"`
	PrevContent *string `json:"prevContent"`
	Prov        string  `json:"prov"`
	CurrentHTML string  `json:"currentHtml"`
	Choice      string  `json:"choice"`
	ID          string  `json:"id"`
	Bytes       string  `json:"bytes"`
	HTML        string  `json:"html"`
	Hash        string  `json:"hash"`
}

// The generator's stubs: Date.now is BASE + a counter, crypto.randomUUID is a
// counter in the last group.
const guardGoldenNowBase = int64(1793512800000)

// TestGuardGolden replays every scenario against Local's own answers: the return
// of each call, the live notifications, and the bytes of the guard and version
// records.
func TestGuardGolden(t *testing.T) {
	golden := loadGuardGolden(t)
	if golden.Stubs["dateNow"] != "counter" || golden.Stubs["randomUUID"] != "counter" {
		t.Fatalf("the golden was not generated with counter stubs: %v", golden.Stubs)
	}
	for _, scenario := range golden.Scenarios {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			runGuardGoldenScenario(t, scenario)
		})
	}
}

func loadGuardGolden(t *testing.T) guardGoldenFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "guard-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden guardGoldenFile
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Scenarios) == 0 {
		t.Fatal("guard golden has no scenarios")
	}
	return golden
}

// guardClock is Date.now and crypto.randomUUID over the generator's counters.
type guardClock struct {
	now  int64
	uuid int64
}

func (c *guardClock) nextNow() int64 {
	value := guardGoldenNowBase + c.now
	c.now++
	return value
}

func (c *guardClock) nextUUID() string {
	value := fmt.Sprintf("00000000-0000-4000-8000-%012d", c.uuid)
	c.uuid++
	return value
}

func freezeGuardClock(t *testing.T) *guardClock {
	t.Helper()
	clock := &guardClock{}
	previousNow := guardNow
	previousID := guardNewID
	guardNow = clock.nextNow
	guardNewID = clock.nextUUID
	t.Cleanup(func() {
		guardNow = previousNow
		guardNewID = previousID
	})
	return clock
}

// guardRecorder is the recording live channel and the call log, in the shape the
// golden generator records Local's own calls.
type guardRecorder struct {
	root  string
	calls []map[string]any
}

func (r *guardRecorder) record(fn string, args ...any) {
	normalized := make([]any, len(args))
	for index, arg := range args {
		normalized[index] = normalizeGuardValue(arg, r.root)
	}
	r.calls = append(r.calls, map[string]any{"fn": fn, "args": normalized, "returns": nil})
}

func (r *guardRecorder) Notify(name string, message guardNotification) {
	r.record("live.notify", name, guardJSONPlain(message))
}

func (r *guardRecorder) writeBack(html string) error {
	island, _ := extractIsland(html)
	data := any(nil)
	if island != nil {
		data = guardJSONPlain(islandField(island, guardRulesName))
	}
	r.calls = append(r.calls, map[string]any{
		"fn":      "writeBack",
		"args":    []any{normalizeGuardValue(map[string]any{"data": data}, r.root)},
		"returns": nil,
	})
	return nil
}

// guardJSONPlain turns a Go answer into the plain JSON the golden holds, the
// shape the panel would receive.
// normalizeGuardValue is the generator's normalization: a path becomes its
// relative form, a long string its sha256 and its JS (UTF-16) length, an object
// its sorted keys.
func normalizeGuardValue(value any, root string) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if root != "" && typed == root {
			return map[string]any{"path": "."}
		}
		if root != "" && strings.HasPrefix(typed, root+"/") {
			return map[string]any{"path": strings.TrimPrefix(typed, root+"/")}
		}
		if length := len(utf16.Encode([]rune(typed))); length > 40 {
			return map[string]any{"sha256": commitSha256([]byte(typed)), "length": length}
		}
		return typed
	case bool, int, int64, float64:
		return typed
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = normalizeGuardValue(item, root)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeGuardValue(item, root)
		}
		return out
	}
	return value
}

func guardJSONPlain(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil
	}
	return decoded
}

func runGuardGoldenScenario(t *testing.T, scenario guardGoldenScenario) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range scenario.Seed {
		content, err := base64.StdEncoding.DecodeString(seed.B64)
		if err != nil {
			t.Fatal(err)
		}
		writeCommitFile(t, root, seed.Path, content)
	}
	freezeGuardClock(t)
	recorder := &guardRecorder{root: root}

	for index, step := range scenario.Steps {
		runGuardGoldenStep(t, recorder, root, step)
		if index >= len(scenario.Calls) {
			t.Fatalf("step %d (%s) has no recorded call", index, step.Fn)
		}
	}
	if len(recorder.calls) != len(scenario.Calls) {
		t.Fatalf("recorded %d calls, the golden has %d", len(recorder.calls), len(scenario.Calls))
	}
	got := marshalCommitJSON(t, recorder.calls)
	want := marshalCommitJSON(t, scenario.Calls)
	if !jsonEqual(t, got, want) {
		t.Errorf("calls:\n got %s\nwant %s", got, want)
	}

	gotTree := guardGoldenTree(t, root)
	if !reflect.DeepEqual(gotTree, scenario.Tree) {
		t.Errorf("guard and version tree:\n got %v\nwant %v", gotTree, scenario.Tree)
	}
}

func runGuardGoldenStep(t *testing.T, recorder *guardRecorder, root string, step guardGoldenStep) {
	t.Helper()
	name := step.Args.Name
	switch step.Fn {
	case "runDataLossGuard":
		args := map[string]any{"name": name, "newHtml": step.Args.NewHTML, "prevContent": nil, "prov": step.Args.Prov}
		if step.Args.PrevContent != nil {
			args["prevContent"] = *step.Args.PrevContent
		}
		event := runDataLossGuard(runGuardArgs{
			BaseDir:     root,
			Name:        name,
			NewHTML:     step.Args.NewHTML,
			PrevContent: step.Args.PrevContent,
			Prov:        dataguard.Provenance(step.Args.Prov),
			Live:        recorder,
		})
		recordGuardReturn(recorder, step.Fn, args, event)

	case "getGuardEvent":
		args := map[string]any{"name": name, "currentHtml": step.Args.CurrentHTML}
		current := step.Args.CurrentHTML
		event := getGuardEvent(root, name, &current, recorder)
		recordGuardReturn(recorder, step.Fn, args, event)

	case "resolveGuard":
		args := map[string]any{"name": name, "id": step.Args.ID, "choice": step.Args.Choice, "currentHtml": step.Args.CurrentHTML}
		result := resolveGuard(resolveGuardArgs{
			BaseDir:     root,
			Name:        name,
			ID:          step.Args.ID,
			Choice:      step.Args.Choice,
			CurrentHTML: step.Args.CurrentHTML,
			WriteBack:   recorder.writeBack,
			Live:        recorder,
		})
		recordGuardReturn(recorder, step.Fn, args, result)

	case "markPending":
		args := map[string]any{"name": name, "bytes": step.Args.Bytes}
		markPending(root, name, []byte(step.Args.Bytes))
		recordGuardReturn(recorder, step.Fn, args, nil)

	case "noteSeen":
		args := map[string]any{"name": name, "bytes": step.Args.Bytes}
		noteSeen(root, name, []byte(step.Args.Bytes))
		recordGuardReturn(recorder, step.Fn, args, nil)

	case "catchUpOutsideEdit":
		args := map[string]any{"name": name, "bytes": step.Args.Bytes, "html": step.Args.HTML}
		catchUpOutsideEdit(root, name, []byte(step.Args.Bytes), step.Args.HTML, recorder)
		recordGuardReturn(recorder, step.Fn, args, nil)

	case "applyRemoteResolution":
		args := map[string]any{"name": name, "hash": step.Args.Hash}
		cleared := applyRemoteResolution(root, name, step.Args.Hash, noopGuardLive{})
		recordGuardReturn(recorder, step.Fn, args, cleared)

	default:
		t.Fatalf("unknown step %s", step.Fn)
	}
}

func recordGuardReturn(recorder *guardRecorder, fn string, args map[string]any, returns any) {
	recorder.calls = append(recorder.calls, map[string]any{
		"fn":      fn,
		"args":    []any{normalizeGuardValue(args, recorder.root)},
		"returns": normalizeGuardValue(guardJSONPlain(returns), recorder.root),
	})
}

// guardGoldenTree is the final .hyperclay/guard and .hyperclay/versions tree. A
// guard record stores the ABSOLUTE recover path, so the root is replaced by
// `<root>`: every other byte is what the host wrote.
func guardGoldenTree(t *testing.T, root string) []guardGoldenFilePair {
	t.Helper()
	files := []guardGoldenFilePair{}
	var walk func(dir, prefix string)
	walk = func(dir, prefix string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			rel := entry.Name()
			if prefix != "" {
				rel = prefix + "/" + entry.Name()
			}
			full := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				walk(full, rel)
				continue
			}
			content, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(content), root, "<root>")
			text = strings.ReplaceAll(text, filepath.ToSlash(root), "<root>")
			files = append(files, guardGoldenFilePair{Path: rel, B64: base64.StdEncoding.EncodeToString([]byte(text))})
		}
	}
	for _, top := range []string{".hyperclay/guard", ".hyperclay/versions"} {
		full := filepath.Join(root, filepath.FromSlash(top))
		if _, err := os.Stat(full); err != nil {
			continue
		}
		walk(full, top)
	}
	return files
}
