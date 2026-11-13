package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// goFamilies names, for every vector family the package ships, either the Go
// test that runs it or "pending" when only the JS reference answers it.
var goFamilies = map[string]string{
	"copy-content.json":            "internal/dataapi TestStripCopyContentFixture",
	"data-api.json":                "internal/dataapi TestConformanceExtraction",
	"data-loss.json":               "internal/dataguard TestVectors",
	"operations.json":              "metadata",
	"blockeditor-admission.json":   "internal/pageadmission TestPageAdmissionVectors",
	"blockeditor-lineage.json":     "internal/pageadmission TestPageLineageVectors",
	"pending-checkpoint.json":      "pending",
	"pending-blockeditor-api.json": "pending",
	"pending-rooms.json":           "pending",
	"root-attrs.json":              "internal/htmlutil TestHostVectors",
	"runtime-html.json":            "internal/clayruntime TestHTMLVectorsMatchClayJS",
	"runtime-parse.json":           "internal/clayruntime TestRuntimeParseVectors",
	"runtime-releases.json":        "internal/clayruntime TestURLVectorsMatchClayJS",
	"runtime-urls.json":            "internal/clayruntime TestURLVectorsMatchClayJS",
	"selector-parity.json":         "internal/dataapi TestSelectorParity",
	"sheet-guard.json":             "internal/dataguard TestSheetGuardVectors",
	"sheet-codec.json":             "internal/gridcodec TestScanVectors",
	"spec-wire.json":               "internal/specwire TestHostVectors",
}

type hostVectorIndex struct {
	Families []struct {
		File      string `json:"file"`
		Kind      string `json:"kind"`
		GoPending bool   `json:"goPending"`
	} `json:"families"`
}

func hostVectorRepoRoot(t testing.TB) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the testutil sources")
	}
	return filepath.Join(filepath.Dir(here), "..", "..")
}

func hostVectorTestExists(t testing.TB, decision string) bool {
	t.Helper()
	dir, name, ok := strings.Cut(decision, " ")
	if !ok || dir == "" || name == "" {
		t.Fatalf("family map entry %q is not \"<dir> <TestName>\"", decision)
	}
	entries, err := os.ReadDir(filepath.Join(hostVectorRepoRoot(t), filepath.FromSlash(dir)))
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	needle := "func " + name + "("
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(hostVectorRepoRoot(t), filepath.FromSlash(dir), entry.Name()))
		if err != nil {
			t.Fatalf("read %s/%s: %v", dir, entry.Name(), err)
		}
		if strings.Contains(string(raw), needle) {
			return true
		}
	}
	return false
}

func TestEveryVectorFamilyHasAGoDecision(t *testing.T) {
	var index hostVectorIndex
	LoadVectors(t, "index.json", &index)
	if len(index.Families) == 0 {
		t.Fatal("index.json names no families")
	}
	known := map[string]bool{}
	for _, family := range index.Families {
		known[family.File] = true
		decision, ok := goFamilies[family.File]
		if !ok {
			t.Errorf("index.json names %s, which the Go family map does not know: name the Go test that runs it, or map it to \"pending\"", family.File)
			continue
		}
		if !family.GoPending {
			continue
		}
		if decision != "pending" {
			t.Errorf("index.json marks %s goPending but the Go family map says %q", family.File, decision)
			continue
		}
		var file struct {
			Cases []struct {
				GoPending bool `json:"goPending"`
			} `json:"cases"`
		}
		LoadVectors(t, family.File, &file)
		if len(file.Cases) == 0 {
			t.Errorf("%s is goPending but carries no cases", family.File)
			continue
		}
		for i, c := range file.Cases {
			if !c.GoPending {
				t.Errorf("%s case %d is not goPending, but the family is", family.File, i)
			}
		}
		t.Logf("%s: %d cases not yet run in Go (JS-only by D1)", family.File, len(file.Cases))
	}
	for name := range goFamilies {
		if !known[name] {
			t.Errorf("the Go family map names %s, which index.json lacks", name)
		}
	}
	for _, family := range index.Families {
		decision := goFamilies[family.File]
		if family.GoPending || decision == "pending" || decision == "metadata" {
			continue
		}
		if !hostVectorTestExists(t, decision) {
			t.Errorf("%s is mapped to %q, but that test function does not exist", family.File, decision)
		}
	}
}
