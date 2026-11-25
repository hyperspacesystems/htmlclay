package gitread

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var hunkContext = regexp.MustCompile(`(?m)^(@@ [^@]+ @@).*$`)

// The diff a page renders is git's own unified diff, except for three things
// go-git does not print: the function name after a hunk header, the
// similarity line of a rename, and the tab git puts after a name with a space
// on the ---/+++ lines. Git quotes non-ASCII names unless core.quotePath is
// off; go-git never does, which What The Func's parser needs
// (what-the-func/index.html:432).
func TestDiffMatchesGitsOwnPatch(t *testing.T) {
	p := plantRepo(t)
	patch, err := Diff(context.Background(), p.dir, DiffOptions{Base: "base", Head: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	want := p.run("-c", "core.quotePath=false", "--no-pager", "diff", "--full-index", "--no-ext-diff", "--no-textconv", "-M", "base...feature")
	if got := p.ran(); got != "" {
		t.Fatalf("the reference git ran %s", got)
	}
	var kept []string
	for _, line := range strings.SplitAfter(hunkContext.ReplaceAllString(want, "$1"), "\n") {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			line = strings.Replace(line, "\t\n", "\n", 1)
		}
		if !strings.HasPrefix(line, "similarity index ") {
			kept = append(kept, line)
		}
	}
	got, wantLines := strings.SplitAfter(string(patch.Text), "\n"), strings.SplitAfter(strings.Join(kept, ""), "\n")
	for i := 0; i < len(got) || i < len(wantLines); i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Fatalf("line %d differs from git's:\ngitread %q\ngit     %q", i+1, g, w)
		}
	}
}

func fileNames(files []File) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.Path)
	}
	return names
}

func skipped(p *Patch, reason string) []string {
	var names []string
	for _, s := range p.Skipped {
		if s.Reason == reason {
			names = append(names, s.Path)
		}
	}
	return names
}

// An agent's path names one file or folder: a glob or magic word in it widens
// nothing. The host's own patterns leave files out and list them.
func TestDiffNamesPathsLiterallyAndListsSkippedFiles(t *testing.T) {
	p := plantRepo(t)
	diff := func(o DiffOptions) *Patch {
		t.Helper()
		o.Base, o.Head = "base", "feature"
		patch, err := Diff(context.Background(), p.dir, o)
		if err != nil {
			t.Fatal(err)
		}
		return patch
	}
	for _, c := range []struct {
		paths []string
		want  string
	}{
		{[]string{"a.txt"}, "a.txt"},
		{[]string{"./src/"}, "src/app/util.js"},
		{[]string{"before.md"}, "after.md"},
		{[]string{"*.txt"}, ""},
		{[]string{":/"}, ""},
		{[]string{"src/app/util"}, ""},
	} {
		if got := strings.Join(fileNames(diff(DiffOptions{Paths: c.paths}).Files), " "); got != c.want {
			t.Errorf("Paths %q gave %q, want %q", c.paths, got, c.want)
		}
	}
	patch := diff(DiffOptions{Exclude: append([]string{"*.secret"}, DefaultExclude...)})
	if got := strings.Join(skipped(patch, SkipExcluded), " "); got != "notes.secret package-lock.json vendor/lib/x.js" {
		t.Errorf("excluded %q", got)
	}
	for _, name := range []string{"notes.secret", "package-lock.json", "vendor/lib/x.js"} {
		if strings.Contains(string(patch.Text), name) {
			t.Errorf("%s is in the diff text", name)
		}
	}
	if !strings.Contains(string(patch.Text), "src/app/util.js") {
		t.Error("a file no pattern names was left out")
	}
}

// Text never passes MaxBytes: files past the text budget are skipped as
// limit, while a file under the package limit is never too large however
// small the budget.
func TestDiffStopsAtTheByteLimit(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "a\n")
	r.write("big.txt", strings.Repeat("x", 900)+"\n")
	r.write("c.txt", "c\n")
	r.write("d.txt", "d\n")
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	r.write("a.txt", "a2\n")
	r.write("big.txt", strings.Repeat("y", 900)+"\n")
	r.write("c.txt", "c2\n")
	r.write("d.txt", "d2\n")
	r.commit("two")
	full, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature"})
	if err != nil || len(full.Files) != 4 {
		t.Fatalf("the whole diff: %v %v", fileNames(full.Files), err)
	}
	limit := 250
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature", MaxBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	if len(patch.Text) > limit {
		t.Errorf("Text is %d bytes, past the limit %d", len(patch.Text), limit)
	}
	if got := skipped(patch, SkipTooLarge); len(got) != 0 {
		t.Errorf("a 900 byte file is listed as too large at a %d byte budget: %q", limit, got)
	}
	if !patch.Truncated || len(skipped(patch, SkipLimit)) == 0 || len(patch.Files) == 0 {
		t.Errorf("want some files in and some past the limit: files %v, skipped %+v", fileNames(patch.Files), patch.Skipped)
	}
	if len(patch.Files)+len(patch.Skipped) != 4 {
		t.Errorf("a changed file is neither in the diff nor listed: files %v, skipped %+v", fileNames(patch.Files), patch.Skipped)
	}
}

// A fully rewritten generated file must not stall a review: past
// fileDiffTimeout the rest of the file shows as removed and added whole.
func TestDiffBoundsOneFilesLineDiff(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("big.txt", lines(20000, ""))
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	r.write("big.txt", lines(20000, "changed "))
	r.commit("two")
	was := fileDiffTimeout
	fileDiffTimeout = time.Millisecond
	t.Cleanup(func() { fileDiffTimeout = was })
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature"})
	if err != nil || len(patch.Files) != 1 {
		t.Fatalf("Diff = %v", err)
	}
	if f := patch.Files[0]; f.Added <= 10000 || f.Deleted <= 10000 {
		t.Errorf("the line diff ran to the end (+%d -%d, the exact diff is +10000 -10000)", f.Added, f.Deleted)
	}
}

// base...head is what head changed since it left base: a later change on
// base is not in it.
func TestDiffIsTheBranchsOwnChange(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.commit("one")
	fork := strings.TrimSpace(r.run("rev-parse", "HEAD"))
	r.run("checkout", "-q", "-b", "feature")
	r.write("a.txt", "a2\n")
	r.commit("feature")
	r.run("checkout", "-q", "main")
	r.write("b.txt", "b2\n")
	r.commit("main moves on")
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fileNames(patch.Files), " "); got != "a.txt" || patch.MergeBase != fork {
		t.Errorf("main...feature = %q from %s, want a.txt from %s", got, patch.MergeBase, fork)
	}
}

// A moved submodule and a file whose name holds a newline, which would read
// as a new line of the diff, are listed and never printed.
func TestDiffListsFilesItCannotShow(t *testing.T) {
	r := newRepo(t, t.TempDir())
	odd := "odd\n+a fake added line.txt"
	r.write("a.txt", "a\n")
	r.write(odd, "x\n")
	r.commit("one")
	r.run("update-index", "--add", "--cacheinfo", "160000,"+strings.TrimSpace(r.run("rev-parse", "HEAD"))+",sub")
	r.run("commit", "-q", "-m", "add sub")
	r.run("checkout", "-q", "-b", "feature")
	r.write("a.txt", "b\n")
	r.write(odd, "y\n")
	r.run("add", "a.txt", odd)
	r.run("update-index", "--cacheinfo", "160000,"+strings.TrimSpace(r.run("rev-parse", "HEAD"))+",sub")
	r.run("commit", "-q", "-m", "two")
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fileNames(patch.Files), " "); got != "a.txt" {
		t.Errorf("files %q, want a.txt", got)
	}
	if got := skipped(patch, SkipName); len(got) != 1 || got[0] != odd {
		t.Errorf("skipped for the name: %q", got)
	}
	if got := skipped(patch, SkipSubmodule); len(got) != 1 || got[0] != "sub" {
		t.Errorf("skipped as a submodule: %q", got)
	}
	if strings.Contains(string(patch.Text), "fake") || strings.Contains(string(patch.Text), "Subproject") {
		t.Errorf("the diff text holds a skipped file:\n%s", patch.Text)
	}
}

// A rename is left out when either of its names matches an exclude pattern,
// so a file moved out of a vendored folder contributes no lines.
func TestDiffExcludesARenameByEitherName(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("vendor/a.js", "var a = 1\nvar b = 2\n")
	r.write("src/b.js", "var b = 3\n")
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	r.run("mv", "vendor/a.js", "src/a.js")
	r.commit("two")
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature", Exclude: []string{"vendor/**"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patch.Text), "vendor/") {
		t.Errorf("the patch holds the excluded old name:\n%s", patch.Text)
	}
	if got := strings.Join(skipped(patch, SkipExcluded), " "); got != "vendor/a.js" {
		t.Errorf("excluded %q, want %q", got, "vendor/a.js")
	}

	other := newRepo(t, t.TempDir())
	other.write("src/keep.js", "var keep = 1\nvar two = 2\n")
	other.commit("one")
	other.run("checkout", "-q", "-b", "feature")
	if err := os.MkdirAll(filepath.Join(other.dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	other.run("mv", "src/keep.js", "vendor/keep.js")
	other.commit("two")
	patch, err = Diff(context.Background(), other.dir, DiffOptions{Base: "main", Head: "feature", Exclude: []string{"vendor/**"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patch.Text), "keep = 1") || strings.Contains(string(patch.Text), "vendor/") {
		t.Errorf("the patch holds the excluded rename:\n%s", patch.Text)
	}
	if got := strings.Join(skipped(patch, SkipExcluded), " "); got != "vendor/keep.js" {
		t.Errorf("excluded %q, want %q", got, "vendor/keep.js")
	}

	plain := newRepo(t, t.TempDir())
	plain.write("src/b.js", "var b = 1\n")
	plain.commit("one")
	plain.run("checkout", "-q", "-b", "feature")
	plain.write("src/b.js", "var b = 2\n")
	plain.commit("two")
	patch, err = Diff(context.Background(), plain.dir, DiffOptions{Base: "main", Head: "feature", Exclude: []string{".*"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch.Text), "+var b = 2") {
		t.Errorf("a modified file was dropped by an exclude pattern that matches no name:\n%s", patch.Text)
	}
}

// The file cap is the package limit, not the text budget: a large file with a
// small change is still in the text at a small budget.
func TestDiffUsesThePackageLimitForFiles(t *testing.T) {
	r := newRepo(t, t.TempDir())
	body := strings.Repeat("line of the app\n", 20000)
	r.write("app.html", body)
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	r.write("app.html", strings.Replace(body, "line of the app", "CHANGED", 1))
	r.commit("two")
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature", MaxBytes: 256 << 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch.Text), "+CHANGED") {
		t.Errorf("the one-line change to a %d byte file is not in the text at a 256 KiB budget", len(body))
	}
	if got := skipped(patch, SkipTooLarge); len(got) != 0 {
		t.Errorf("a file under the package limit is listed as too large: %q", got)
	}
}

// The skipped list is capped, with a count of the rest, so a branch that adds
// a build folder cannot grow the prompt without a bound.
func TestDiffCapsTheSkippedList(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "a\n")
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	for i := 0; i < 1200; i++ {
		r.write("dist/f"+strings.Repeat("0", 4)+strconv.Itoa(i)+".js", "x\n")
	}
	r.commit("two")
	patch, err := Diff(context.Background(), r.dir, DiffOptions{Base: "main", Head: "feature", MaxBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(patch.Skipped) != 500 || patch.SkippedMore != 700 {
		t.Errorf("skipped %d files and counted %d more, want 500 and 700", len(patch.Skipped), patch.SkippedMore)
	}
}
