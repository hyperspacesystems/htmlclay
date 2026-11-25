package gitread

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every read runs against the planted repository with fake git, ssh and gpg
// programs first on PATH. Nothing the repository names runs, nothing on PATH
// runs, and no file in the repository changes, while plain git on the same
// repository runs the planted programs.
func TestReadingRunsNoRepositoryProgram(t *testing.T) {
	p := plantRepo(t)
	fake := t.TempDir()
	for _, name := range []string{"git", "git-upload-pack", "git-receive-pack", "ssh", "gpg", "gpg2"} {
		script(t, fake, name, "echo path-"+name+" >> '"+p.log+"'\nexit 1\n")
	}
	before := snapshot(t, p.dir, p.lazy)

	t.Run("reads", func(t *testing.T) {
		t.Setenv("PATH", fake)
		ctx := context.Background()
		patch, err := Diff(ctx, p.dir, DiffOptions{Base: "base", Head: "feature"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(patch.Text), `+	greeting := "hello there"`) || !strings.Contains(string(patch.Text), "+secret v2") {
			t.Errorf("the diff is missing the committed changes:\n%s", patch.Text)
		}
		if strings.Contains(string(patch.Text), "uncommitted") {
			t.Errorf("the diff holds the working tree's changes:\n%s", patch.Text)
		}
		commits, err := Log(ctx, p.dir, "base", "feature", 0)
		if err != nil || len(commits) != 2 || commits[0].Subject != "signed follow-up" {
			t.Errorf("Log = %+v, %v", commits, err)
		}
		branches, err := Branches(ctx, p.dir)
		if err != nil || len(branches) != 3 {
			t.Errorf("Branches = %+v, %v", branches, err)
		}
		body, err := FileAt(ctx, p.dir, "base", "a.txt")
		if err != nil || string(body) != "alpha\nbeta\ngamma\n" {
			t.Errorf("FileAt = %q, %v", body, err)
		}
		status, err := Status(ctx, p.dir)
		if err != nil || !hasStatus(status, "a.txt", "M") || !hasStatus(status, "notes.secret", "M") {
			t.Errorf("Status = %+v, %v", status, err)
		}
		var u *Unsupported
		if _, err := Diff(ctx, p.lazy, DiffOptions{Base: "base", Head: "feature"}); !errors.As(err, &u) || !strings.Contains(u.Feature, "partialclone") {
			t.Errorf("the partial clone: %v, want *Unsupported naming partialclone", err)
		}
	})

	if got := p.ran(); got != "" {
		t.Errorf("reading ran %s", got)
	}
	after := snapshot(t, p.dir, p.lazy)
	for path, was := range before {
		if after[path] != was {
			t.Errorf("reading changed %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("reading created %s", path)
		}
	}

	p.run("--no-pager", "diff", "base...feature")
	p.run("--no-pager", "log", "-1")
	filtered := filepath.Join(p.dir, "a.txt")
	if body, err := os.ReadFile(filtered); err == nil && len(body) > 0 {
		body[0] ^= 0x20
		if err := os.WriteFile(filtered, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p.run("status")
	got := p.ran()
	for _, want := range []string{"external", "gpg", "fsmonitor", "clean"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain git ran %q, want %s too (the test cannot fail otherwise)", got, want)
		}
	}
}

func hasStatus(entries []StatusEntry, path, worktree string) bool {
	for _, e := range entries {
		if e.Path == path && e.Worktree == worktree {
			return true
		}
	}
	return false
}

func TestUnsupportedRepositoriesAreRefused(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		version string
		key     string
		feature string
	}{
		{"worktreeConfig in format 1", "1", "extensions.worktreeConfig", "worktreeconfig"},
		{"worktreeConfig in format 0", "0", "extensions.worktreeConfig", "worktreeconfig"},
		{"refStorage", "1", "extensions.refStorage", "refstorage"},
		{"a newer format", "2", "", "repository format 2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRepo(t, t.TempDir())
			r.write("a.txt", "a\n")
			r.commit("one")
			r.run("config", "core.repositoryformatversion", c.version)
			if c.key != "" {
				r.run("config", c.key, "true")
			}
			var u *Unsupported
			if _, err := Branches(ctx, r.dir); !errors.As(err, &u) || !strings.Contains(u.Feature, c.feature) {
				t.Errorf("Branches = %v, want *Unsupported naming %s", err, c.feature)
			}
		})
	}

	r := newRepo(t, filepath.Join(t.TempDir(), "repo"))
	r.write("sub/a.txt", "a\n")
	r.commit("one")
	if _, err := Branches(ctx, r.dir); err != nil {
		t.Fatalf("the repository itself: %v", err)
	}
	if _, err := Branches(ctx, filepath.Join(r.dir, "sub")); !errors.Is(err, ErrNotRepository) {
		t.Errorf("a folder inside a repository read the repository above it: %v", err)
	}
	if _, err := Branches(ctx, t.TempDir()); !errors.Is(err, ErrNotRepository) {
		t.Errorf("a plain folder: %v", err)
	}
	if _, err := Branches(ctx, filepath.Join(t.TempDir(), "moved")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing folder: %v, want fs.ErrNotExist", err)
	}
	var refused *Refused
	if _, err := Branches(ctx, "repo"); !errors.As(err, &refused) {
		t.Errorf("a relative folder: %v", err)
	}
}

func TestRefusesOptionShapedInput(t *testing.T) {
	for _, ref := range []string{"main", "feature/login", "HEAD~2", "v1.2.3", "origin/main", "main^", "@{u}", "a1b2c3d",
		"v1.2.3+build.4", "main~3", "HEAD^2", "fix/#482-login", "feature/correção", "release/1.0,hotfix", "it's-done", "renovate/@types/node", "a;b", "$(x)"} {
		if !ValidRef(ref) {
			t.Errorf("ValidRef(%q) = false, want true", ref)
		}
	}
	for _, ref := range []string{"", "-p", "--output=/tmp/x", "-x", "a..b", "a b", "a:b", "a?b", "a*b", "a[b", `a\b`, "HEAD:secret",
		"cycle~2000000000", "main~12345", "HEAD^00001", "a\x01b", strings.Repeat("a", 201)} {
		if ValidRef(ref) {
			t.Errorf("ValidRef(%q) = true, want false", ref)
		}
	}
	ctx := context.Background()
	dir := t.TempDir()
	var refused *Refused
	for _, o := range []DiffOptions{
		{Base: "--output=/tmp/x", Head: "main"},
		{Base: "main", Head: "a..b"},
		{Base: "main", Head: "feature", Paths: []string{"/etc/passwd"}},
		{Base: "main", Head: "feature", Paths: []string{"src/../../outside.txt"}},
		{Base: "main", Head: "feature", Paths: []string{""}},
		{Base: "main", Head: "feature", Exclude: []string{"["}},
		{Base: "main", Head: "feature", Exclude: []string{"**/a/**/b/**"}},
	} {
		if _, err := Diff(ctx, dir, o); !errors.As(err, &refused) {
			t.Errorf("Diff(%+v) = %v, want *Refused", o, err)
		}
	}
	for _, name := range []string{"/etc/passwd", "../x", "a/../../x", `C:\Windows\System32`, `src\..\..\secret.txt`, `\etc\passwd`, "C:secret.txt"} {
		if _, err := FileAt(ctx, dir, "main", name); !errors.As(err, &refused) {
			t.Errorf("FileAt(%q) = %v, want *Refused", name, err)
		}
	}
	if !ValidPath("src/app/main.go") {
		t.Error("ValidPath(src/app/main.go) = false")
	}
}

// A hostile .git entry or .git/commondir must not make a read reach a
// repository outside the chosen folder.
func TestDotGitCannotPointOutside(t *testing.T) {
	root := t.TempDir()
	victim := newRepo(t, filepath.Join(root, "victim"))
	victim.write("secret.env", "API_KEY=topsecret123\n")
	victim.commit("add the secret")
	ctx := context.Background()

	pointed := filepath.Join(root, "pointed")
	if err := os.MkdirAll(pointed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pointed, ".git"), []byte("gitdir: "+filepath.Join(victim.dir, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var u *Unsupported
	if body, err := FileAt(ctx, pointed, "main", "secret.env"); !errors.As(err, &u) || len(body) != 0 {
		t.Errorf("a .git file naming another repository: FileAt = %q, %v; want *Unsupported", body, err)
	}

	linked := newRepo(t, filepath.Join(root, "linked"))
	linked.write("a.txt", "a\n")
	linked.commit("one")
	if err := os.WriteFile(filepath.Join(linked.dir, ".git", "commondir"), []byte(filepath.Join("..", "..", "victim", ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if body, err := FileAt(ctx, linked.dir, "main", "secret.env"); !errors.As(err, &u) || len(body) != 0 {
		t.Errorf("a .git/commondir naming another repository: FileAt = %q, %v; want *Unsupported", body, err)
	}

	victim.run("worktree", "add", "-q", "-b", "side", filepath.Join(root, "victim-side"))
	borrowed := filepath.Join(root, "borrowed")
	if err := os.MkdirAll(borrowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(borrowed, ".git"), []byte("gitdir: "+filepath.Join(victim.dir, ".git", "worktrees", "victim-side")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if body, err := FileAt(ctx, borrowed, "main", "secret.env"); !errors.As(err, &u) || len(body) != 0 {
		t.Errorf("a .git file naming another repository's real worktree entry: FileAt = %q, %v; want *Unsupported", body, err)
	}
}

// Status of a repository whose submodule is not set up lists the rest of the
// working tree and writes nothing into the repository.
func TestStatusWritesNothing(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "a\n")
	r.write(".gitmodules", "[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.com/sub.git\n")
	r.commit("one")
	r.run("config", "submodule.sub.url", "https://example.com/sub.git")
	before := snapshot(t, r.dir)
	if _, err := Status(context.Background(), r.dir); err != nil {
		t.Errorf("Status with a submodule that is not set up = %v, want nil", err)
	}
	after := snapshot(t, r.dir)
	for path, was := range before {
		if after[path] != was {
			t.Errorf("Status changed %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("Status created %s", path)
		}
	}
}

// David works in linked worktrees, whose .git is a file naming a folder in
// the main repository.
func TestReadsALinkedWorktree(t *testing.T) {
	root := t.TempDir()
	r := newRepo(t, filepath.Join(root, "main"))
	r.write("a.txt", "one\n")
	r.commit("one")
	linked := filepath.Join(root, "linked")
	r.run("worktree", "add", "-q", "-b", "topic", linked)
	w := &repo{t: t, git: r.git, dir: linked, env: r.env, tick: r.tick}
	w.write("a.txt", "two\n")
	w.commit("two")
	w.write("a.txt", "three\n")
	ctx := context.Background()
	patch, err := Diff(ctx, linked, DiffOptions{Base: "main", Head: "HEAD"})
	if err != nil || !strings.Contains(string(patch.Text), "+two") {
		t.Fatalf("Diff in a linked worktree = %v\n%s", err, patchText(patch))
	}
	status, err := Status(ctx, linked)
	if err != nil || !hasStatus(status, "a.txt", "M") {
		t.Errorf("Status in a linked worktree = %+v, %v", status, err)
	}
}

func patchText(p *Patch) string {
	if p == nil {
		return ""
	}
	return string(p.Text)
}

// A call returns at its deadline even while go-git is still diffing one
// large file, a step that takes no context.
func TestCallsStopAtTheirDeadline(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("big.txt", lines(20000, ""))
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	r.write("big.txt", lines(20000, "changed "))
	r.commit("two")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Diff(ctx, r.dir, DiffOptions{Base: "main", Head: "feature"}); !errors.Is(err, ErrTimeout) {
		t.Errorf("Diff past its deadline = %v, want ErrTimeout", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := Log(ctx, r.dir, "main", "feature", 0); !errors.Is(err, context.Canceled) {
		t.Errorf("Log after a cancel = %v, want context.Canceled", err)
	}
}

func addSubmodule(t *testing.T, super *repo, sub *repo, name string) {
	t.Helper()
	super.run("-c", "protocol.file.allow=always", "submodule", "add", sub.dir, name)
	super.commit("add " + name)
}

// A submodule's own folder is a repository: its .git file names the
// superproject's .git/modules storage, whose core.worktree points back.
func TestReadsASubmodulesOwnFolder(t *testing.T) {
	root := t.TempDir()
	sub := newRepo(t, filepath.Join(root, "lib"))
	sub.write("lib.txt", "one\n")
	sub.commit("one")
	sub.run("branch", "topic")
	super := newRepo(t, filepath.Join(root, "super"))
	super.write("a.txt", "a\n")
	super.commit("one")
	addSubmodule(t, super, sub, "lib")
	inside := filepath.Join(super.dir, "lib")
	ctx := context.Background()

	branches, err := Branches(ctx, inside)
	if err != nil {
		t.Fatalf("Branches in the submodule = %v", err)
	}
	var names []string
	for _, b := range branches {
		if !b.Remote {
			names = append(names, b.Name)
		}
	}
	if strings.Join(names, " ") != "main" {
		t.Errorf("local branches in the submodule = %q", names)
	}
	if _, err := Log(ctx, inside, "main", "main", 0); err != nil {
		t.Errorf("Log in the submodule = %v", err)
	}

	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, ".git"), []byte("gitdir: "+filepath.Join(super.dir, ".git", "modules", "lib")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var u *Unsupported
	if _, err := Branches(ctx, other); !errors.As(err, &u) {
		t.Errorf("a .git file naming another folder's submodule storage = %v, want *Unsupported", err)
	}

	// A crafted submodule storage carrying a commondir or alternates escape must be refused.
	victim := newRepo(t, filepath.Join(root, "private-victim"))
	victim.write("secret.txt", "top-secret\n")
	victim.commit("secret")

	for _, mode := range []string{"commondir", "alternates"} {
		evil := filepath.Join(root, "evil-"+mode)
		modDir := filepath.Join(evil, "sub", ".git", "modules", "m")
		if err := os.MkdirAll(filepath.Join(modDir, "objects", "info"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(evil, ".git"), []byte("gitdir: sub/.git/modules/m\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(modDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(modDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n\tworktree = ../../../..\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "commondir" {
			if err := os.WriteFile(filepath.Join(modDir, "commondir"), []byte(filepath.Join(victim.dir, ".git")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(filepath.Join(modDir, "objects", "info", "alternates"), []byte(filepath.Join(victim.dir, ".git", "objects")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var esc *Unsupported
		if _, err := Branches(ctx, evil); !errors.As(err, &esc) {
			t.Errorf("crafted submodule with %s = %v, want *Unsupported", mode, err)
		}
	}
}

// A linked worktree of a repository with an initialized submodule still
// reports the working tree's changes, matching git status.
func TestStatusInALinkedWorktreeWithASubmodule(t *testing.T) {
	root := t.TempDir()
	sub := newRepo(t, filepath.Join(root, "lib"))
	sub.write("lib.txt", "one\n")
	sub.commit("one")
	super := newRepo(t, filepath.Join(root, "super"))
	super.write("a.txt", "a\n")
	super.commit("one")
	addSubmodule(t, super, sub, "lib")
	linked := filepath.Join(root, "linked")
	super.run("worktree", "add", "-q", "-b", "topic", linked)
	w := &repo{t: t, git: super.git, dir: linked, env: super.env, tick: super.tick}
	w.write("a.txt", "changed\n")
	status, err := Status(context.Background(), linked)
	if err != nil {
		t.Fatalf("Status in a linked worktree with a submodule = %v", err)
	}
	if !hasStatus(status, "a.txt", "M") {
		t.Errorf("Status = %+v, want a.txt modified", status)
	}
}
