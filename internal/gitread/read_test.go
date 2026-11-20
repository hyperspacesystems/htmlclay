package gitread

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func subjects(commits []Commit) string {
	var s []string
	for _, c := range commits {
		s = append(s, c.Subject)
	}
	return strings.Join(s, " ")
}

// A branch that merged main later still lists only its own commits and the
// merge, never main's, as git log main..feature does.
func TestLogListsOnlyTheHeadsOwnCommits(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "1\n")
	r.commit("c1")
	r.run("checkout", "-q", "-b", "feature")
	r.write("f.txt", "1\n")
	r.commit("f1")
	r.run("checkout", "-q", "main")
	r.write("a.txt", "2\n")
	r.commit("c2")
	r.write("a.txt", "3\n")
	r.commit("c3")
	r.run("checkout", "-q", "feature")
	r.run("merge", "-q", "--no-edit", "-m", "merge main", "main")
	r.write("f.txt", "2\n")
	r.commit("f2")
	ctx := context.Background()
	commits, err := Log(ctx, r.dir, "main", "feature", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := subjects(commits); got != "f2 merge main f1" {
		t.Errorf("Log main..feature = %q, want %q", got, "f2 merge main f1")
	}
	if commits, err := Log(ctx, r.dir, "main", "feature", 2); err != nil || subjects(commits) != "f2 merge main" {
		t.Errorf("Log with max 2 = %q, %v", subjects(commits), err)
	}
	if commits, err := Log(ctx, r.dir, "feature", "main", 0); err != nil || len(commits) != 0 {
		t.Errorf("Log feature..main = %q, %v; main has nothing feature lacks", subjects(commits), err)
	}
}

func TestBranchesAndFileAt(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "one\n")
	r.write("dir/b.txt", "b\n")
	r.commit("one")
	r.run("tag", "-a", "v1", "-m", "first")
	r.run("checkout", "-q", "-b", "feature")
	r.write("a.txt", "two\n")
	r.commit("two")
	r.run("update-ref", "refs/remotes/origin/main", "main")
	r.run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	ctx := context.Background()

	branches, err := Branches(ctx, r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range branches {
		s := b.Name
		if b.Current {
			s += "*"
		}
		if b.Remote {
			s += "@"
		}
		got = append(got, s)
	}
	if strings.Join(got, " ") != "feature* main origin/main@" {
		t.Errorf("Branches = %q, want local first, the current one marked, then origin/main", got)
	}

	for _, c := range []struct{ rev, name, want string }{
		{"main", "a.txt", "one\n"},
		{"v1", "a.txt", "one\n"},
		{"feature", "a.txt", "two\n"},
		{"HEAD~1", "./dir/b.txt", "b\n"},
	} {
		if body, err := FileAt(ctx, r.dir, c.rev, c.name); err != nil || string(body) != c.want {
			t.Errorf("FileAt(%s, %s) = %q, %v; want %q", c.rev, c.name, body, err, c.want)
		}
	}
	if _, err := FileAt(ctx, r.dir, "main", "missing.txt"); !errors.Is(err, ErrNoFile) {
		t.Errorf("a missing file: %v", err)
	}
	if _, err := FileAt(ctx, r.dir, "main", "dir"); !errors.Is(err, ErrNoFile) {
		t.Errorf("a folder: %v", err)
	}
	if _, err := FileAt(ctx, r.dir, "nope", "a.txt"); !errors.Is(err, ErrUnknownRef) {
		t.Errorf("an unknown ref: %v", err)
	}
}

// looseObject writes a loose object of kind and body and returns its hash.
func looseObject(t *testing.T, dir, kind string, body []byte) string {
	t.Helper()
	raw := append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...)
	sum := sha1.Sum(raw)
	hash := hex.EncodeToString(sum[:])
	writeObject(t, dir, hash, raw)
	return hash
}

// writeObject writes raw, zlib-compressed, as the loose object hash.
func writeObject(t *testing.T, dir, hash string, raw []byte) {
	t.Helper()
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ".git", "objects", hash[:2], hash[2:])
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, z.Bytes(), 0o444); err != nil {
		t.Fatal(err)
	}
}

// A blob past MaxBytes and a blob whose header lies must never be inflated
// whole into memory, and a diff lists such files without reading them.
func TestLargeObjectsStayBounded(t *testing.T) {
	ctx := context.Background()
	const big = 32 << 20
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "a\n")
	r.commit("one")
	raw := []byte(fmt.Sprintf("blob %d\x00", big))
	raw = append(raw, bytes.Repeat([]byte("Z"), big)...)
	sum := sha1.Sum(raw)
	bigHash := hex.EncodeToString(sum[:])
	writeObject(t, r.dir, bigHash, raw)
	r.run("update-index", "--add", "--cacheinfo", "100644,"+bigHash+",big.txt")
	r.run("commit", "-q", "-m", "add big")

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	body, err := FileAt(ctx, r.dir, "main", "big.txt")
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("FileAt of a 32 MiB loose blob = %d bytes, %v; want ErrTooLarge", len(body), err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20+MaxBytes {
		t.Errorf("FileAt of a 32 MiB loose blob allocated %d MiB", grew>>20)
	}

	lieHash := strings.Repeat("1", 40)
	lie := append([]byte("blob 10\x00"), bytes.Repeat([]byte("L"), 12<<20)...)
	writeObject(t, r.dir, lieHash, lie)
	blob, err := hex.DecodeString(lieHash)
	if err != nil {
		t.Fatal(err)
	}
	treeHash := looseObject(t, r.dir, "tree", append([]byte("100644 lie.txt\x00"), blob...))
	parent := strings.TrimSpace(r.run("rev-parse", "HEAD"))
	commit := fmt.Sprintf("tree %s\nparent %s\nauthor t <t@t> 1700000000 +0000\ncommitter t <t@t> 1700000000 +0000\n\nlie\n", treeHash, parent)
	commitHash := looseObject(t, r.dir, "commit", []byte(commit))
	if err := os.WriteFile(filepath.Join(r.dir, ".git", "refs", "heads", "lie"), []byte(commitHash+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&before)
	body, err = FileAt(ctx, r.dir, "lie", "lie.txt")
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("FileAt of a blob whose header claims 10 bytes = %d bytes, %v; want ErrTooLarge", len(body), err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20+MaxBytes {
		t.Errorf("FileAt of a lying 12 MiB blob allocated %d MiB", grew>>20)
	}

	negHash := strings.Repeat("2", 40)
	neg := append([]byte("blob -1\x00"), bytes.Repeat([]byte("N"), 12<<20)...)
	writeObject(t, r.dir, negHash, neg)
	negBlob, err := hex.DecodeString(negHash)
	if err != nil {
		t.Fatal(err)
	}
	negTree := looseObject(t, r.dir, "tree", append([]byte("100644 neg.txt\x00"), negBlob...))
	negCommit := fmt.Sprintf("tree %s\nparent %s\nauthor t <t@t> 1700000000 +0000\ncommitter t <t@t> 1700000000 +0000\n\nneg\n", negTree, parent)
	negCommitHash := looseObject(t, r.dir, "commit", []byte(negCommit))
	if err := os.WriteFile(filepath.Join(r.dir, ".git", "refs", "heads", "neg"), []byte(negCommitHash+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&before)
	body, err = FileAt(ctx, r.dir, "neg", "neg.txt")
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("FileAt of a blob whose header claims -1 bytes = %d bytes, %v; want ErrTooLarge", len(body), err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20+MaxBytes {
		t.Errorf("FileAt of a lying negative-header blob allocated %d MiB", grew>>20)
	}

	lyingCommit := append([]byte("commit 10\x00"), bytes.Repeat([]byte("M"), 48<<20)...)
	lyingCommitHash := looseObject(t, r.dir, "commit", lyingCommit)
	if err := os.WriteFile(filepath.Join(r.dir, ".git", "refs", "heads", "bigcommit"), []byte(lyingCommitHash+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err = Log(ctx, r.dir, "main", "bigcommit", 0)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Error("Log of a commit with a 48 MiB message = nil, want an error")
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20+MaxBytes {
		t.Errorf("Log of a lying 48 MiB commit allocated %d MiB", grew>>20)
	}

	alternate := newRepo(t, t.TempDir())
	alternate.write("a.txt", "a\n")
	alternate.commit("one")
	if err := os.MkdirAll(filepath.Join(alternate.dir, ".git", "objects", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alternate.dir, ".git", "objects", "info", "alternates"), []byte(filepath.Join(r.dir, ".git", "objects")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var u *Unsupported
	if _, err := Branches(ctx, alternate.dir); !errors.As(err, &u) || !strings.Contains(u.Feature, "alternates") {
		t.Errorf("a repository with alternates = %v, want *Unsupported naming alternates", err)
	}

	d := newRepo(t, t.TempDir())
	one := bytes.Repeat([]byte("line of data one\n"), 600_000)
	two := bytes.Repeat([]byte("line of data two\n"), 600_000)
	d.write("old1.txt", string(one))
	d.write("old2.txt", string(two))
	d.commit("one")
	d.run("checkout", "-q", "-b", "feature")
	d.run("rm", "-q", "old1.txt", "old2.txt")
	d.write("new1.txt", string(one)+"extra\n")
	d.write("new2.txt", string(two)+"extra\n")
	d.commit("two")
	d.run("gc", "-q")

	runtime.GC()
	runtime.ReadMemStats(&before)
	patch, err := Diff(ctx, d.dir, DiffOptions{Base: "main", Head: "feature"})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(skipped(patch, SkipTooLarge), " ")
	for _, name := range []string{"new1.txt", "new2.txt"} {
		if !strings.Contains(got, name) {
			t.Errorf("%s is not listed as too large: %q", name, got)
		}
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20+MaxBytes {
		t.Errorf("the diff of two 10 MiB files allocated %d MiB", grew>>20)
	}
}

// A branch or tag whose name holds "@" is found by its exact reference, where
// a revision parser would split on the "@".
func TestBranchesAndTagsWithAtSigns(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "one\n")
	r.commit("one")
	r.run("tag", "@scope/pkg@1.2.0")
	r.run("checkout", "-q", "-b", "renovate/@types/node")
	r.write("a.txt", "two\n")
	r.commit("two")
	ctx := context.Background()

	patch, err := Diff(ctx, r.dir, DiffOptions{Base: "main", Head: "renovate/@types/node"})
	if err != nil || !strings.Contains(string(patch.Text), "+two") {
		t.Errorf("Diff on a branch with @ = %v\n%s", err, patchText(patch))
	}
	if commits, err := Log(ctx, r.dir, "main", "renovate/@types/node", 0); err != nil || subjects(commits) != "two" {
		t.Errorf("Log on a branch with @ = %q, %v", subjects(commits), err)
	}
	if body, err := FileAt(ctx, r.dir, "renovate/@types/node", "a.txt"); err != nil || string(body) != "two\n" {
		t.Errorf("FileAt on a branch with @ = %q, %v", body, err)
	}
	if body, err := FileAt(ctx, r.dir, "@scope/pkg@1.2.0", "a.txt"); err != nil || string(body) != "one\n" {
		t.Errorf("FileAt on a tag with @ = %q, %v", body, err)
	}
}

// Log walks only the commits it needs: a one commit branch on 20,000 commits
// of history returns at once and stays small.
func TestLogWalksOnlyWhatItNeeds(t *testing.T) {
	r := newRepo(t, t.TempDir())
	var stream strings.Builder
	for i := 0; i < 20000; i++ {
		msg := fmt.Sprintf("main %d", i)
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter t <t@t> %d +0000\ndata %d\n%s\n", 1700000000+i, len(msg), msg)
		fmt.Fprintf(&stream, "M 100644 inline f.txt\ndata %d\n%d\n", len(fmt.Sprint(i)), i)
	}
	msg := "branch commit"
	fmt.Fprintf(&stream, "commit refs/heads/feature\ncommitter t <t@t> %d +0000\ndata %d\n%s\nfrom refs/heads/main\nM 100644 inline f.txt\ndata 3\nnew\n", 1800000000, len(msg), msg)
	cmd := exec.Command(r.git, "-C", r.dir, "fast-import", "--quiet")
	cmd.Env = r.env
	cmd.Stdin = strings.NewReader(stream.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fast-import: %v\n%s", err, out)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	commits, err := Log(context.Background(), r.dir, "main", "feature", 10)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if got := subjects(commits); got != "branch commit" {
		t.Errorf("Log main..feature = %q, want %q", got, "branch commit")
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 16<<20 {
		t.Errorf("Log allocated %d MiB for a one commit branch", grew>>20)
	}
}
