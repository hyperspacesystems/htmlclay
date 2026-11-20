package gitread

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

// repo builds a fixture repository with the real git. Each call gets the
// next second as its date, so commit order is commit date order.
type repo struct {
	t    *testing.T
	git  string
	dir  string
	env  []string
	tick int
}

func newRepo(t *testing.T, dir string) *repo {
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
	r := &repo{t: t, git: git, dir: dir, env: []string{
		"HOME=" + filepath.Dir(dir), "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	}}
	r.run("init", "-q", "-b", "main")
	return r
}

func (r *repo) run(args ...string) string {
	r.t.Helper()
	r.tick++
	date := fmt.Sprintf("2026-11-19T10:%02d:%02dZ", r.tick/60, r.tick%60)
	cmd := exec.Command(r.git, append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(append([]string{}, r.env...), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (r *repo) write(name, body string) {
	r.t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(message string) {
	r.t.Helper()
	r.run("add", "-A")
	r.run("commit", "-q", "-m", message)
}

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// planted is the round 3 planted repository (security-model-round3.md): its
// own settings name a program for everything git can run while it reads, and
// each program appends its name to log when it runs.
type planted struct {
	*repo
	root string
	lazy string
	log  string
}

func plantRepo(t *testing.T) *planted {
	t.Helper()
	root := t.TempDir()
	r := newRepo(t, filepath.Join(root, "repo"))
	p := &planted{repo: r, root: root, lazy: filepath.Join(root, "lazy"), log: filepath.Join(root, "canary.log")}
	log := "'" + p.log + "'"
	canary := script(t, root, "canary.sh", "echo \"$1\" >> "+log+"\ncase \"$1\" in\n  textconv) cat \"$2\" ;;\n  clean|smudge) cat ;;\nesac\nexit 0\n")
	gpg := script(t, root, "canary-gpg.sh", "echo gpg >> "+log+"\nexit 1\n")
	lazyFetch := script(t, root, "canary-lazy.sh", "echo lazyfetch >> "+log+"\nexit 1\n")

	r.write("server.go", "package main\n\nimport \"fmt\"\n\nfunc handle(name string) string {\n\tgreeting := \"hello\"\n\treturn fmt.Sprintf(\"%s %s\", greeting, name)\n}\n\nfunc main() {\n\tfmt.Println(handle(\"world\"))\n}\n")
	r.write("a.txt", "alpha\nbeta\ngamma\n")
	r.write("notes.secret", "secret v1\n")
	r.write("old.txt", "to be deleted\n")
	r.write("before.md", "rename me\nline 2\nline 3\nline 4\n")
	r.write("blob.bin", "\x00\x01\x02binary")
	r.write("tail.txt", "no newline at end")
	r.write("crlf.txt", "windows\r\nline\r\n")
	r.write("café.txt", "unicode name\n")
	r.write("my file.txt", "a name with a space\n")
	r.write("tool.sh", "run me\n")
	r.write("src/app/util.js", "export const one = 1\n")
	r.write("package-lock.json", "{\"v\": 1}\n")
	r.write("vendor/lib/x.js", "var x = 1\n")
	r.write(".gitattributes", "*.secret diff=evil filter=evil\n")
	r.commit("base commit")
	r.run("branch", "base")
	r.run("checkout", "-q", "-b", "feature")
	r.write("server.go", "package main\n\nimport \"fmt\"\n\nfunc handle(name string) string {\n\tgreeting := \"hello there\"\n\tif name == \"\" {\n\t\tname = \"stranger\"\n\t}\n\treturn fmt.Sprintf(\"%s %s\", greeting, name)\n}\n\nfunc main() {\n\tfmt.Println(handle(\"world\"))\n}\n")
	r.write("a.txt", "alpha\nBETA\ngamma\ndelta\n")
	r.write("notes.secret", "secret v2\n")
	r.run("rm", "-q", "old.txt")
	r.run("mv", "before.md", "after.md")
	r.write("added.txt", "new file\n")
	r.write("blob.bin", "\x00\x01\x03binary!")
	r.write("tail.txt", "no newline at end, changed")
	r.write("crlf.txt", "windows\r\nline two\r\n")
	r.write("café.txt", "unicode name, changed\n")
	r.write("my file.txt", "a name with a space, changed\n")
	r.write("src/app/util.js", "export const one = 1\nexport const two = 2\n")
	r.write("package-lock.json", "{\"v\": 2}\n")
	r.write("vendor/lib/x.js", "var x = 2\n")
	if err := os.Chmod(filepath.Join(r.dir, "tool.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.commit("feature: greet strangers")
	tree := strings.TrimSpace(r.run("rev-parse", "HEAD^{tree}"))
	parent := strings.TrimSpace(r.run("rev-parse", "HEAD"))
	signed := "tree " + tree + "\nparent " + parent + "\nauthor t <t@example.com> 1795086000 +0000\ncommitter t <t@example.com> 1795086000 +0000\n" +
		"gpgsig -----BEGIN PGP SIGNATURE-----\n \n iQEzBAABCAAdFiEEfakefakefakefake\n =abcd\n -----END PGP SIGNATURE-----\n\nsigned follow-up\n"
	cmd := exec.Command(r.git, "-C", r.dir, "hash-object", "-t", "commit", "-w", "--stdin")
	cmd.Env, cmd.Stdin = r.env, strings.NewReader(signed)
	hash, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	r.run("update-ref", "refs/heads/feature", strings.TrimSpace(string(hash)))
	r.write("a.txt", "alpha\nBETA\ngamma\ndelta\nuncommitted\n")
	r.write("notes.secret", "secret v3 uncommitted\n")

	for _, h := range []string{"pre-commit", "post-checkout", "post-index-change", "reference-transaction", "pre-auto-gc", "post-rewrite", "post-merge", "pre-push", "fsmonitor-watchman"} {
		script(t, filepath.Join(r.dir, ".git", "hooks"), h, "echo hook-"+h+" >> "+log+"\n")
	}
	if err := os.WriteFile(filepath.Join(r.dir, ".git", "info", "attributes"), []byte("*.txt diff=evil filter=evil\n"), 0o644); err != nil {
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
		r.run("config", kv[0], kv[1])
	}

	if out, err := exec.Command("/bin/cp", "-R", r.dir, p.lazy).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	lazy := &repo{t: t, git: r.git, dir: p.lazy, env: r.env}
	blob := strings.TrimSpace(lazy.run("rev-parse", "feature:server.go"))
	if err := os.Remove(filepath.Join(p.lazy, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"core.repositoryformatversion", "1"},
		{"extensions.partialClone", "origin"},
		{"remote.origin.url", "ext::" + lazyFetch},
		{"remote.origin.promisor", "true"},
		{"protocol.ext.allow", "always"},
	} {
		lazy.run("config", kv[0], kv[1])
	}
	if err := os.WriteFile(p.log, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ran returns what the planted programs wrote since the last call, and
// empties the log.
func (p *planted) ran() string {
	p.t.Helper()
	b, err := os.ReadFile(p.log)
	if err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(p.log, nil, 0o644); err != nil {
		p.t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// snapshot is every file under roots with its mode, content hash and time.
func snapshot(t *testing.T, roots ...string) map[string]string {
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

// lines is n numbered lines; every second one starts with prefix.
func lines(n int, prefix string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			b.WriteString(prefix)
		}
		fmt.Fprintf(&b, "line %d %s\n", i, strings.Repeat("x", i%7))
	}
	return b.String()
}
