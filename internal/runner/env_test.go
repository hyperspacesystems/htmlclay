package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestCleanEnvDropsLoaderAndGitOverrides(t *testing.T) {
	env := CleanEnv("/scratch", "/usr/bin:/bin",
		"NODE_OPTIONS=--require /x.js", "node_options=--require /y.js", "DYLD_INSERT_LIBRARIES=/x.dylib",
		"GIT_CONFIG_GLOBAL=/x", "GIT_DIR=/x", "LD_AUDIT=/x.so", "CLAUDE_CODE_TMPDIR=/scratch")
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"NODE_OPTIONS", "node_options", "DYLD_INSERT_LIBRARIES", "GIT_CONFIG_GLOBAL=/x", "GIT_DIR", "LD_AUDIT"} {
		if strings.Contains(joined, bad) {
			t.Errorf("CleanEnv kept %s:\n%s", bad, joined)
		}
	}
	for _, good := range []string{"HOME=/scratch", "TMPDIR=/scratch", "GIT_CONFIG_GLOBAL=/dev/null", "CLAUDE_CODE_TMPDIR=/scratch"} {
		if !slices.Contains(env, good) {
			t.Errorf("CleanEnv is missing %s:\n%s", good, joined)
		}
	}
}

// A Windows child needs SystemRoot and its kin to start, and gets nothing
// else from HTML Clay's environment.
func TestWindowsEnvCopiesOnlySystemNames(t *testing.T) {
	host := []string{`SystemRoot=C:\Windows`, "PATHEXT=.COM;.EXE", "ANTHROPIC_API_KEY=secret", `USERPROFILE=C:\Users\me`, `=C:=C:\`}
	env := systemEnv("windows", host, `C:\scratch`)
	for _, want := range []string{`SystemRoot=C:\Windows`, "PATHEXT=.COM;.EXE", `USERPROFILE=C:\scratch`, `TEMP=C:\scratch`} {
		if !slices.Contains(env, want) {
			t.Errorf("missing %s in %q", want, env)
		}
	}
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "secret") || strings.Contains(joined, `C:\Users\me`) {
		t.Errorf("copied a name that is not a system name: %q", env)
	}
	if got := systemEnv("darwin", host, "/scratch"); len(got) != 0 {
		t.Errorf("a Mac child got Windows names: %q", got)
	}
}

// A repository's own config can name an fsmonitor and a signature program
// that git runs while it reads. Under CleanEnv a child's own git status and
// git log run neither.
func TestCleanEnvTurnsOffTheRepositorysGitPrograms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planted programs are shell scripts")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	repo, canaries := filepath.Join(root, "repo"), filepath.Join(root, "canaries")
	if err := os.MkdirAll(canaries, 0o755); err != nil {
		t.Fatal(err)
	}
	plant := func(name string) string {
		p := filepath.Join(root, name+".sh")
		if err := os.WriteFile(p, []byte("#!/bin/sh\necho ran > "+filepath.Join(canaries, name)+"\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	plain := []string{"HOME=" + root, "PATH=/usr/bin:/bin", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}
	run := func(env []string, args ...string) string {
		cmd := exec.Command(git, append([]string{"-C", repo}, args...)...)
		cmd.Env = env
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	if out, err := exec.Command(git, "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(plain, "add", ".")
	run(plain, "commit", "-q", "-m", "one")
	tree := strings.TrimSpace(run(plain, "rev-parse", "HEAD^{tree}"))
	signed := "tree " + tree + "\nauthor t <t@t> 1763546400 +0000\ncommitter t <t@t> 1763546400 +0000\n" +
		"gpgsig -----BEGIN PGP SIGNATURE-----\n \n iQEzBAABCAAdFiEEfake\n =abcd\n -----END PGP SIGNATURE-----\n\nsigned\n"
	cmd := exec.Command(git, "-C", repo, "hash-object", "-t", "commit", "-w", "--stdin")
	cmd.Env, cmd.Stdin = plain, strings.NewReader(signed)
	hash, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	run(plain, "update-ref", "HEAD", strings.TrimSpace(string(hash)))
	run(plain, "config", "core.fsmonitor", plant("fsmonitor"))
	run(plain, "config", "log.showSignature", "true")
	run(plain, "config", "gpg.program", plant("gpg"))

	ran := func() []string {
		entries, _ := os.ReadDir(canaries)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
			os.Remove(filepath.Join(canaries, e.Name()))
		}
		return names
	}
	run(plain, "status")
	run(plain, "log", "-1")
	if got := ran(); len(got) != 2 {
		t.Fatalf("plain git ran %v, want fsmonitor and gpg (the test cannot fail otherwise)", got)
	}
	clean := CleanEnv(t.TempDir(), "/usr/bin:/bin")
	run(clean, "status")
	run(clean, "log", "-1")
	if got := ran(); len(got) != 0 {
		t.Errorf("git under CleanEnv ran %v", got)
	}
}
