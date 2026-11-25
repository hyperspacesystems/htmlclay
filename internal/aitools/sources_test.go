package aitools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/testutil"
)

func TestGateRunsOneReadPerFolderAndTwoInAll(t *testing.T) {
	var gate Gate
	var mu sync.Mutex
	running, most := map[string]int{}, map[string]int{}
	all, mostAll := 0, 0
	var wg sync.WaitGroup
	for i := range 12 {
		folder := []string{"a", "b", "c"}[i%3]
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate.Do(context.Background(), folder, func(context.Context) error {
				mu.Lock()
				running[folder]++
				all++
				most[folder] = max(most[folder], running[folder])
				mostAll = max(mostAll, all)
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				running[folder]--
				all--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if most["a"] != 1 || most["b"] != 1 || most["c"] != 1 || mostAll != 2 {
		t.Fatalf("most at once per folder = %v, in all = %d; want 1 each and 2 in all", most, mostAll)
	}
}

func TestFileSourcesStayInsideTheFolder(t *testing.T) {
	root := realTemp(t)
	folder := filepath.Join(root, "app")
	os.MkdirAll(filepath.Join(folder, "sub"), 0o700)
	os.WriteFile(filepath.Join(root, "secret.txt"), []byte("SECRET\n"), 0o600)
	os.MkdirAll(filepath.Join(root, "app-old"), 0o700)
	os.WriteFile(filepath.Join(root, "app-old", "x.txt"), []byte("SECRET\n"), 0o600)
	os.WriteFile(filepath.Join(folder, "sub", "a.txt"), []byte("1\n2\n3\n"), 0o600)
	if err := os.Symlink(filepath.Join(root, "secret.txt"), filepath.Join(folder, "link.txt")); err != nil {
		t.Skip("no symlinks here")
	}
	os.Symlink(root, filepath.Join(folder, "up"))
	var gate Gate
	files := &runFiles{}
	for _, path := range []string{"../secret.txt", "link.txt", "up/secret.txt", "../app-old/x.txt", filepath.Join(root, "secret.txt"), "sub/../../secret.txt"} {
		_, err := Expand(context.Background(), &gate, folder, []SourceRef{{ID: "s", Kind: "file", Path: path, First: 1, Last: 9}}, files.put)
		if codeOf(err) != CodeSourceRefused {
			t.Errorf("%s: err = %v, want source_refused", path, err)
		}
	}
	if len(files.files) != 0 {
		t.Fatalf("stored %v", files.files)
	}
	got, err := Expand(context.Background(), &gate, folder, []SourceRef{{ID: "s", Kind: "file", Path: "sub/a.txt", First: 2, Last: 9}}, files.put)
	if err != nil || string(files.files["s"]) != "2\n3\n" || got[0].Lines[1] != 3 {
		t.Fatalf("inside read: %v %q %+v", err, files.files["s"], got)
	}
}

func TestAPartialCloneIsSourceUnsupported(t *testing.T) {
	repo := testutil.PlantGitRepo(t)
	folder := repo.Lazy
	var gate Gate
	_, err := Expand(context.Background(), &gate, folder, []SourceRef{{ID: "diff", Kind: "git", Base: "main", Head: "feature"}}, (&runFiles{}).put)
	if codeOf(err) != CodeSourceUnsupported {
		t.Fatalf("err = %v", err)
	}
	git, err := GitContext(context.Background(), &gate, folder, PromptDiffBytes)
	if err != nil || !strings.Contains(git, "cannot read this folder's git history") {
		t.Fatalf("prompt context = %q, %v", git, err)
	}
	if ran := repo.Ran(); ran != "" {
		t.Fatalf("a program the repository names ran: %s", ran)
	}
}

func TestSourcesNamedTwiceOrBadlyAreRefused(t *testing.T) {
	folder := realTemp(t)
	os.WriteFile(filepath.Join(folder, "a.txt"), []byte("x\n"), 0o600)
	var gate Gate
	for _, refs := range [][]SourceRef{
		{{ID: "a", Kind: "file", Path: "a.txt"}, {ID: "a", Kind: "file", Path: "a.txt"}},
		{{ID: "../x", Kind: "file", Path: "a.txt"}},
		{{ID: "a", Kind: "url", Path: "a.txt"}},
	} {
		if _, err := Expand(context.Background(), &gate, folder, refs, (&runFiles{}).put); codeOf(err) != CodeResultInvalid {
			t.Errorf("%+v: err = %v", refs, err)
		}
	}
}

func TestParseKeepsThePagesLimits(t *testing.T) {
	r, err := Parse([]byte(`{"request":"hi","instructions":[{"name":"a","text":"b"}]}`))
	if err != nil || r.Tools != LevelRead || r.Folder != FolderRemembered || string(r.Schema) != `{"type":"object"}` {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	for _, bad := range []string{
		`{"request":""}`,
		`{"request":"x","folder":"/Users/me/app"}`,
		`{"request":"x","tools":"root"}`,
		`{"request":"x","engine":"Claude Code"}`,
		`{"request":"x","schema":[]}`,
		`{"request":"` + strings.Repeat("x", 64<<10+1) + `"}`,
	} {
		if _, err := Parse([]byte(bad)); err == nil || err.Code != CodeInvalidRequest {
			t.Errorf("%.60s: err = %v", bad, err)
		}
	}
}

func TestValidateNamesWhereAnAnswerIsWrong(t *testing.T) {
	schema := Wrap(json.RawMessage(`{"type":"object","additionalProperties":false,"required":["files"],"properties":{"files":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["anchor"],"properties":{"anchor":{"type":"string"}}}}}}`), true)
	ok := `{"data":{"files":[{"anchor":"a"}]},"sources":[]}`
	if err := Validate(schema, json.RawMessage(ok)); err != nil {
		t.Fatalf("valid answer: %v", err)
	}
	for bad, where := range map[string]string{
		`{"data":{"files":[{"anchor":1}]},"sources":[]}`:            "data.files[0].anchor",
		`{"data":{"files":[{"anchor":"a","x":1}]},"sources":[]}`:    "data.files[0]",
		`{"data":{"files":[]}}`:                                     "sources",
		`{"data":{"files":[]},"sources":[{"id":"d","kind":"git"}]}`: "sources[0]",
	} {
		err := Validate(schema, json.RawMessage(bad))
		if err == nil || !strings.HasPrefix(err.Error(), where) {
			t.Errorf("%s: err = %v, want it to name %s", bad, err, where)
		}
	}
}
