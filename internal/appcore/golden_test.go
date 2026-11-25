package appcore

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/v1/*.snapshot.json from Build")

// personas are the fixture names the preview samples the shells with.
var personas = []string{"newPerson", "fiveDocuments", "trustedFolder", "servedFolder", "syncedWithTeams", "localUser"}

func TestGolden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the facts are macOS paths, and Build shows paths in the system's own spelling; TestDisplayPath covers Windows")
	}
	dir := filepath.Join("testdata", "v1")
	for _, name := range personas {
		t.Run(name, func(t *testing.T) {
			factsPath := filepath.Join(dir, name+".facts.json")
			raw, err := os.ReadFile(factsPath)
			if err != nil {
				t.Fatal(err)
			}
			var facts Facts
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&facts); err != nil {
				t.Fatalf("%s: %v", factsPath, err)
			}
			got, err := json.MarshalIndent(Build(facts), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			snapshotPath := filepath.Join(dir, name+".snapshot.json")
			if *update {
				if err := os.WriteFile(snapshotPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(snapshotPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				line, wantLine, gotLine := firstDifference(want, got)
				t.Fatalf("%s differs at line %d:\n want: %s\n  got: %s", snapshotPath, line, wantLine, gotLine)
			}
		})
	}
	fixtures, err := filepath.Glob(filepath.Join(dir, "*.facts.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".facts.json")
		if !isPersona(name) {
			t.Errorf("%s is not one of the fixtures TestGolden knows: %s", fixture, strings.Join(personas, ", "))
		}
	}
}

func isPersona(name string) bool {
	for _, persona := range personas {
		if persona == name {
			return true
		}
	}
	return false
}

// firstDifference answers the number of the first line where want and got differ,
// and both lines.
func firstDifference(want, got []byte) (int, string, string) {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		wantLine, gotLine := "", ""
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine != gotLine {
			return i + 1, wantLine, gotLine
		}
	}
	return 0, "", ""
}
