//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The golden answers come from Local's own src/main/page-state-store.js,
// recorded by testdata/pagestate-golden/gen.cjs. Every scenario is rebuilt here
// with the same deterministic generation source and compared exactly: the
// return value or error of every step, and the files each step leaves behind
// (relative path, kind and exact bytes).

type pageStateGolden struct {
	GeneratedWith map[string]any      `json:"generatedWith"`
	Scenarios     []pageStateScenario `json:"scenarios"`
	Revs          []pageStateRevCase  `json:"revs"`
}

type pageStateScenario struct {
	Name    string            `json:"name"`
	Steps   []pageStateStep   `json:"steps"`
	Results []json.RawMessage `json:"results"`
	Entries [][]storageEntry  `json:"entries"`
}

type pageStateStep struct {
	Kind   string      `json:"kind"`
	Name   string      `json:"name"`
	HTML   string      `json:"html"`
	Text   string      `json:"text"`
	Path   string      `json:"path"`
	From   string      `json:"from"`
	To     string      `json:"to"`
	Stored string      `json:"stored"`
	Rev    *string     `json:"rev"`
	Exact  bool        `json:"exact"`
	State  *PageRecord `json:"state"`
	Base   *string     `json:"base"`
}

type pageStateRevCase struct {
	Name  string  `json:"name"`
	Input string  `json:"input"`
	Rev   *string `json:"rev"`
}

func TestPageStateGolden(t *testing.T) {
	golden := loadPageStateGolden(t)

	if len(golden.Scenarios) == 0 || len(golden.Revs) == 0 {
		t.Fatal("golden.json holds no scenarios")
	}
	for _, want := range golden.Scenarios {
		t.Run(want.Name, func(t *testing.T) {
			root := storageScenarioRoot(t, want.Name)
			base := filepath.Join(root, "root")
			mustStorageMkdir(t, base)

			// The generator's stub: call n answers 16 bytes holding n big-endian,
			// whose hex is n zero padded to 32 digits.
			uses := 0
			original := pageStateNewGeneration
			pageStateNewGeneration = func() string {
				uses++
				return fmt.Sprintf("%032x", uses)
			}
			t.Cleanup(func() { pageStateNewGeneration = original })

			if len(want.Steps) != len(want.Results) || len(want.Steps) != len(want.Entries) {
				t.Fatalf("golden scenario records %d steps, %d results and %d entry lists",
					len(want.Steps), len(want.Results), len(want.Entries))
			}
			for i, step := range want.Steps {
				got := runPageStateStep(t, root, base, step)
				compareStorageJSON(t, fmt.Sprintf("results[%d] %s", i, step.Kind), got, want.Results[i])
				compareStorageJSON(t, fmt.Sprintf("entries[%d]", i), recordPageStateEntries(t, base), mustStorageJSON(want.Entries[i]))
			}
		})
	}

	for _, want := range golden.Revs {
		t.Run("pageRev "+want.Name, func(t *testing.T) {
			expected := ""
			if want.Rev != nil {
				expected = *want.Rev
			}
			if got := pageRev(want.Input); got != expected {
				t.Errorf("pageRev = %q, want %q", got, expected)
			}
		})
	}
}

func loadPageStateGolden(t *testing.T) pageStateGolden {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "pagestate-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden pageStateGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func pageStateStepRev(step pageStateStep) string {
	if step.Rev == nil {
		return ""
	}
	return *step.Rev
}

func pageStateStepBase(base string, step pageStateStep) string {
	if step.Base == nil {
		return base
	}
	return *step.Base
}

func runPageStateStep(t *testing.T, root, base string, step pageStateStep) any {
	t.Helper()
	refuse := func(err error) any { return map[string]any{"error": err.Error()} }
	switch step.Kind {
	case "writeFile":
		full := filepath.Join(base, filepath.FromSlash(step.Path))
		mustStorageMkdir(t, filepath.Dir(full))
		if err := os.WriteFile(full, []byte(step.Text), 0o644); err != nil {
			t.Fatal(err)
		}
		return nil
	case "mkdir":
		mustStorageMkdir(t, filepath.Join(base, filepath.FromSlash(step.Path)))
		return nil
	case "storePageState":
		file, err := storePageState(base, step.Name, step.HTML, false)
		if err != nil {
			return refuse(err)
		}
		full := filepath.Join(base, filepath.FromSlash(step.Name))
		mustStorageMkdir(t, filepath.Dir(full))
		if err := os.WriteFile(full, []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"file": file}
	case "readPageState":
		record, err := readPageState(base, step.Name, PageStateReadOptions{Rev: pageStateStepRev(step), Exact: step.Exact})
		if err != nil {
			return refuse(err)
		}
		return record
	case "readPageRecord":
		record, err := readPageRecord(base, step.Name, pageStateStepRev(step))
		if err != nil {
			return refuse(err)
		}
		return record
	case "writePageState":
		if err := writePageState(base, step.Name, step.State, false); err != nil {
			return refuse(err)
		}
		return nil
	case "pageStatePath":
		target, err := pageStatePath(base, step.Name)
		if err != nil {
			return refuse(err)
		}
		return map[string]any{"path": storageRelPath(t, root, target)}
	case "pageStateDirPath":
		target, err := pageStateDirPath(base, step.Name)
		if err != nil {
			return refuse(err)
		}
		return map[string]any{"path": storageRelPath(t, root, target)}
	case "dropPageState":
		if err := dropPageState(base, step.Name); err != nil {
			return refuse(err)
		}
		return nil
	case "dropPageStateDir":
		if err := dropPageStateDir(base, step.Name); err != nil {
			return refuse(err)
		}
		return nil
	case "movePageState":
		if err := movePageState(base, step.From, step.To); err != nil {
			return refuse(err)
		}
		return nil
	case "movePageStateDir":
		if err := movePageStateDir(base, step.From, step.To); err != nil {
			return refuse(err)
		}
		return nil
	case "logicalDocument":
		document, err := logicalDocument(pageStateStepBase(base, step), step.Name, step.Stored)
		if err != nil {
			return refuse(err)
		}
		return map[string]any{"document": document}
	case "logicalBytes":
		out, err := logicalBytes(pageStateStepBase(base, step), step.Name, []byte(step.Text))
		if err != nil {
			return refuse(err)
		}
		return map[string]any{"bytes": string(out)}
	case "diskRev":
		result := map[string]any{"rev": nil}
		if rev := diskRev(base, step.Name); rev != "" {
			result["rev"] = rev
		}
		return result
	}
	t.Fatalf("unknown blockeditor-state step kind %q", step.Kind)
	return nil
}

// recordPageStateEntries is the generator's recorder: every path under the
// scenario root with its kind, and a file's exact text.
func recordPageStateEntries(t *testing.T, base string) []storageEntry {
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
		if info.IsDir() {
			recorded.Kind = "dir"
		} else {
			data, readErr := os.ReadFile(target)
			if readErr != nil {
				return nil
			}
			recorded.Kind = "file"
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
