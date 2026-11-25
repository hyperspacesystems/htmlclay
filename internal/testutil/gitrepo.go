package testutil

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// GitRepo builds a fixture repository with the real git binary for tests. Each
// command advances the commit timestamp by one second so commit order matches
// commit date order.
type GitRepo struct {
	T    *testing.T
	Git  string
	Dir  string
	Env  []string
	Tick int
}

// NewGitRepo initializes a clean git repository on branch main in dir.
func NewGitRepo(t *testing.T, dir string) *GitRepo {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixtures use shell scripts")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &GitRepo{T: t, Git: git, Dir: dir, Env: []string{
		"HOME=" + filepath.Dir(dir), "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	}}
	r.Run("init", "-q", "-b", "main")
	return r
}

// Run executes git in r.Dir with deterministic author and committer dates.
func (r *GitRepo) Run(args ...string) string {
	r.T.Helper()
	r.Tick++
	date := fmt.Sprintf("2026-11-19T10:%02d:%02dZ", r.Tick/60, r.Tick%60)
	cmd := exec.Command(r.Git, append([]string{"-C", r.Dir}, args...)...)
	cmd.Env = append(append([]string{}, r.Env...), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Write writes body to a slash-separated path inside r.Dir.
func (r *GitRepo) Write(name, body string) {
	r.T.Helper()
	p := filepath.Join(r.Dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.T.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		r.T.Fatal(err)
	}
}

// Commit stages all changes and commits them with message.
func (r *GitRepo) Commit(message string) {
	r.T.Helper()
	r.Run("add", "-A")
	r.Run("commit", "-q", "-m", message)
}

// WriteScript writes an executable /bin/sh script in dir and returns its path.
func WriteScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// PlantedRepo is a git repository whose config, attributes and hooks name
// executable scripts for every hook and helper git can run while reading, plus
// a partial-clone copy at Lazy.
type PlantedRepo struct {
	*GitRepo
	Root string
	Lazy string
	Log  string
}

// PlantGitRepo builds a PlantedRepo in a fresh TempDir.
func PlantGitRepo(t *testing.T) *PlantedRepo {
	t.Helper()
	root := t.TempDir()
	r := NewGitRepo(t, filepath.Join(root, "repo"))
	p := &PlantedRepo{GitRepo: r, Root: root, Lazy: filepath.Join(root, "lazy"), Log: filepath.Join(root, "canary.log")}
	log := "'" + p.Log + "'"
	canary := WriteScript(t, root, "canary.sh", "echo \"$1\" >> "+log+"\ncase \"$1\" in\n  textconv) cat \"$2\" ;;\n  clean|smudge) cat ;;\nesac\nexit 0\n")
	gpg := WriteScript(t, root, "canary-gpg.sh", "echo gpg >> "+log+"\nexit 1\n")
	lazyFetch := WriteScript(t, root, "canary-lazy.sh", "echo lazyfetch >> "+log+"\nexit 1\n")

	r.Write("server.go", "package main\n\nimport \"fmt\"\n\nfunc handle(name string) string {\n\tgreeting := \"hello\"\n\treturn fmt.Sprintf(\"%s %s\", greeting, name)\n}\n\nfunc main() {\n\tfmt.Println(handle(\"world\"))\n}\n")
	r.Write("a.txt", "alpha\nbeta\ngamma\n")
	r.Write("notes.secret", "secret v1\n")
	r.Write("old.txt", "to be deleted\n")
	r.Write("before.md", "rename me\nline 2\nline 3\nline 4\n")
	r.Write("blob.bin", "\x00\x01\x02binary")
	r.Write("tail.txt", "no newline at end")
	r.Write("crlf.txt", "windows\r\nline\r\n")
	r.Write("café.txt", "unicode name\n")
	r.Write("my file.txt", "a name with a space\n")
	r.Write("tool.sh", "run me\n")
	r.Write("src/app/util.js", "export const one = 1\n")
	r.Write("package-lock.json", "{\"v\": 1}\n")
	r.Write("vendor/lib/x.js", "var x = 1\n")
	r.Write(".gitattributes", "*.secret diff=evil filter=evil\n")
	r.Commit("base commit")
	r.Run("branch", "base")

	r.Run("checkout", "-q", "-b", "feature")
	r.Write("server.go", "package main\n\nimport \"fmt\"\n\nfunc handle(name string) string {\n\tgreeting := \"hello there\"\n\tif name == \"\" {\n\t\tname = \"stranger\"\n\t}\n\treturn fmt.Sprintf(\"%s %s\", greeting, name)\n}\n\nfunc main() {\n\tfmt.Println(handle(\"world\"))\n}\n")
	r.Write("a.txt", "alpha\nBETA\ngamma\ndelta\n")
	r.Write("notes.secret", "secret v2\n")
	r.Run("rm", "-q", "old.txt")
	r.Run("mv", "before.md", "after.md")
	r.Write("added.txt", "new file\n")
	r.Write("blob.bin", "\x00\x01\x03binary!")
	r.Write("tail.txt", "no newline at end, changed")
	r.Write("crlf.txt", "windows\r\nline two\r\n")
	r.Write("café.txt", "unicode name, changed\n")
	r.Write("my file.txt", "a name with a space, changed\n")
	r.Write("src/app/util.js", "export const one = 1\nexport const two = 2\n")
	r.Write("package-lock.json", "{\"v\": 2}\n")
	r.Write("vendor/lib/x.js", "var x = 2\n")
	if err := os.Chmod(filepath.Join(r.Dir, "tool.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.Commit("feature: greet strangers")
	tree := strings.TrimSpace(r.Run("rev-parse", "HEAD^{tree}"))
	parent := strings.TrimSpace(r.Run("rev-parse", "HEAD"))
	signed := "tree " + tree + "\nparent " + parent + "\nauthor t <t@example.com> 1795086000 +0000\ncommitter t <t@example.com> 1795086000 +0000\n" +
		"gpgsig -----BEGIN PGP SIGNATURE-----\n \n iQEzBAABCAAdFiEEfakefakefakefake\n =abcd\n -----END PGP SIGNATURE-----\n\nsigned follow-up\n"
	cmd := exec.Command(r.Git, "-C", r.Dir, "hash-object", "-t", "commit", "-w", "--stdin")
	cmd.Env, cmd.Stdin = r.Env, strings.NewReader(signed)
	hash, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	r.Run("update-ref", "refs/heads/feature", strings.TrimSpace(string(hash)))
	r.Write("a.txt", "alpha\nBETA\ngamma\ndelta\nuncommitted\n")
	r.Write("notes.secret", "secret v3 uncommitted\n")

	for _, h := range []string{"pre-commit", "post-checkout", "post-index-change", "reference-transaction", "pre-auto-gc", "post-rewrite", "post-merge", "pre-push", "fsmonitor-watchman"} {
		WriteScript(t, filepath.Join(r.Dir, ".git", "hooks"), h, "echo hook-"+h+" >> "+log+"\n")
	}
	if err := os.WriteFile(filepath.Join(r.Dir, ".git", "info", "attributes"), []byte("*.txt diff=evil filter=evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"core.fsmonitor", canary + " fsmonitor"},
		{"diff.external", canary + " external"},
		{"diff.evil.command", canary + " diffcommand"},
		{"diff.evil.textconv", canary + " textconv"},
		{"filter.evil.clean", canary + " clean"},
		{"filter.evil.smudge", canary + " smudge"},
		{"log.showSignature", "true"},
		{"gpg.program", gpg},
		{"core.pager", canary + " pager"},
		{"pager.log", canary + " pagerlog"},
		{"core.alternateRefsCommand", canary + " altrefs"},
		{"core.sshCommand", canary + " ssh"},
	} {
		r.Run("config", kv[0], kv[1])
	}

	if out, err := exec.Command("/bin/cp", "-R", r.Dir, p.Lazy).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	lazy := &GitRepo{T: t, Git: r.Git, Dir: p.Lazy, Env: r.Env}
	blob := strings.TrimSpace(lazy.Run("rev-parse", "feature:server.go"))
	if err := os.Remove(filepath.Join(p.Lazy, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"core.repositoryformatversion", "1"},
		{"extensions.partialClone", "origin"},
		{"remote.origin.url", "ext::" + lazyFetch},
		{"remote.origin.promisor", "true"},
		{"protocol.ext.allow", "always"},
	} {
		lazy.Run("config", kv[0], kv[1])
	}
	if err := os.WriteFile(p.Log, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Ran returns what the planted scripts logged since the previous call and
// clears the log.
func (p *PlantedRepo) Ran() string {
	p.T.Helper()
	b, err := os.ReadFile(p.Log)
	if err != nil {
		p.T.Fatal(err)
	}
	if err := os.WriteFile(p.Log, nil, 0o644); err != nil {
		p.T.Fatal(err)
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// SnapshotTree records every file and directory under roots with its mode,
// content SHA-256 and mtime.
func SnapshotTree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if d.IsDir() {
				m[p] = "dir"
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			m[p] = fmt.Sprintf("%v %x %d", info.Mode(), sha256.Sum256(b), info.ModTime().UnixNano())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return m
}
