package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/carried"
	"github.com/panphora/htmlclay/internal/tier"
)

const programsTestPage = `<!doctype html><html><head>
<script type="text/x-clay-program" name="first-line" can="read">
const words = require("./words");
async function main(input, clay) { return words.first(await clay.files.read(input.file)) }
</script>
<script type="text/x-clay-program" name="words">exports.first = (text) => text.split("\n")[0];</script>
</head><body></body></html>`

func TestAProgramRunsOnlyFromTheCheckedBytesAndTheStoredCopy(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	writeTestFile(t, doc, programsTestPage)
	folder := real(t, t.TempDir())
	writeTestFile(t, filepath.Join(folder, "notes.txt"), "first\nsecond\n")
	host := programsHost{a}
	downloadedHere(t, a, doc)

	set, err := host.Load(doc)
	if err != nil {
		t.Fatal(err)
	}
	if set.Trust.Ready || len(set.Scan.Programs) != 2 {
		t.Fatalf("a document nobody vouches for: trust %+v, %d programs", set.Trust, len(set.Scan.Programs))
	}
	if err := a.tierStore().TrustCode(a.tierOf(doc).Code, tier.HowCommand); err != nil {
		t.Fatal(err)
	}
	if set, err = host.Load(doc); err != nil || !set.Trust.Ready || set.Trust.Tier != "mine" {
		t.Fatalf("after trusting its code: %+v, %v", set.Trust, err)
	}

	// The page's one folder is shared with its AI agent: a folder chosen for
	// one is reused for the other without a second question.
	setAIToolsFolder(t, a, doc, folder, aitools.LevelRead)
	got, remembered, err := host.Folder(context.Background(), doc, "first-line", false, string(carried.LevelRead), func() { t.Error("asked again for a remembered folder") })
	if err != nil || got != folder || !remembered {
		t.Fatalf("folder = (%q, %v, %v)", got, remembered, err)
	}

	modules := map[string]string{}
	for _, p := range set.Scan.Programs {
		modules[p.Name] = p.Text
	}
	calls, _ := carried.Policy(set.Trust.Tier, carried.LevelRead)
	res, err := host.Run(context.Background(), carried.Job{Entry: "first-line", Modules: modules, Input: json.RawMessage(`{"file":"notes.txt"}`), Level: carried.LevelRead, Calls: calls, Folder: folder})
	if err != nil || string(res.Data) != `"first"` {
		t.Fatalf("run = %s, %v", res.Data, err)
	}
	for _, p := range set.Scan.Programs {
		info, err := os.Lstat(filepath.Join(a.rt.configDir, "carried-programs", carried.Hash(p.Text)+".js"))
		if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o400) || (runtime.GOOS == "windows" && info.Mode().Perm()&0o222 != 0) {
			t.Fatalf("stored copy of %s: %v, %v", p.Name, info, err)
		}
	}

	// A save after the trust check is new code: it waits for the person again.
	writeTestFile(t, doc, programsTestPage+"<!-- -->"+`<script type="text/x-clay-program" name="extra">async function main() {}</script>`)
	if set, err = host.Load(doc); err != nil || set.Trust.Ready {
		t.Fatalf("a changed program kept the old trust: %+v, %v", set.Trust, err)
	}
}

func TestAProgramsFolderQuestionNamesTheProgram(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	folder := real(t, t.TempDir())
	out := make(chan folderAnswer, 1)
	go func() {
		f, remembered, err := programsHost{a}.Folder(context.Background(), doc, "release-notes", false, string(carried.LevelRead), func() {})
		out <- folderAnswer{f, remembered, err}
	}()
	req := waitForSetupRequest(t, a, appcore.RequestAITools)
	if req.Program != "release-notes" {
		t.Fatalf("the question names %q, want the program", req.Program)
	}
	var title string
	for _, w := range a.snapshot().Waiting {
		if w.SubjectID != nil && *w.SubjectID == req.ID {
			title = w.Title
		}
	}
	if title != "app.html wants its program release-notes to read a folder" {
		t.Fatalf("waiting title = %q", title)
	}
	if err := answerSetup(t, a, "answer-program-folder", req, "choose-folder", folder); err != nil {
		t.Fatal(err)
	}
	if got := readFolder(t, out); got.err != nil || got.folder != folder {
		t.Fatalf("folder = (%q, %v)", got.folder, got.err)
	}
}

// A program inside an svg CDATA is not one the browser runs, so it is not a
// program here either: a page cannot run code its trusted bytes do not cover.
func TestAProgramTheBrowserDoesNotRunIsNotOne(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	writeTestFile(t, doc, `<!doctype html><html><head><svg><![CDATA[><script type="text/x-clay-program" name="hidden" can="change">async function main() {}</script>]]></svg></head><body></body></html>`)
	downloadedHere(t, a, doc)
	if err := a.tierStore().TrustCode(a.tierOf(doc).Code, tier.HowCommand); err != nil {
		t.Fatal(err)
	}
	set, err := programsHost{a}.Load(doc)
	if err != nil || !set.Trust.Ready {
		t.Fatalf("trust %+v, %v", set.Trust, err)
	}
	if len(set.Scan.Programs) != 0 {
		t.Fatalf("read %v: a tag the browser does not run is not a program", set.Scan.Programs)
	}
	if _, seen := set.Scan.Program("hidden"); seen {
		t.Fatal("a program hidden in an svg CDATA was read, so a page runs code its trusted bytes do not cover")
	}
}

func TestAChangeProgramsFolderQuestionAsksForChange(t *testing.T) {
	a, doc := newAIToolsFolderApp(t)
	out := make(chan folderAnswer, 1)
	go func() {
		f, remembered, err := programsHost{a}.Folder(context.Background(), doc, "release-notes", false, string(carried.LevelChange), func() {})
		out <- folderAnswer{f, remembered, err}
	}()
	req := waitForSetupRequest(t, a, appcore.RequestAITools)
	var title string
	for _, w := range a.snapshot().Waiting {
		if w.SubjectID != nil && *w.SubjectID == req.ID {
			title = w.Title
		}
	}
	if title != "app.html wants its program release-notes to read and change a folder" {
		t.Fatalf("waiting title = %q", title)
	}
	if err := answerSetup(t, a, "answer-program-folder-change", req, "deny", ""); err != nil {
		t.Fatal(err)
	}
	got := readFolder(t, out)
	if got.err == nil || got.err.Error() != programNoFolderText {
		t.Fatalf("refusal = %v, want %q", got.err, programNoFolderText)
	}
}
