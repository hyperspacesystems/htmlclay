package pathrel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		path, dir string
		want      bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b/c", "/a/b", true},
		{"/a/bc", "/a/b", false},
		{"/a", "/a/b", false},
	}
	for _, c := range cases {
		c.path, c.dir = filepath.FromSlash(c.path), filepath.FromSlash(c.dir)
		if got := Within(c.path, c.dir); got != c.want {
			t.Errorf("Within(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
	if !Within("/a/b"+sep+"c", "/a/b") {
		t.Errorf("Within missed a child joined with %q", sep)
	}
}

func TestSameOrBelow(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "dir")
	child := filepath.Join(dir, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if !SameOrBelow(dir, dir) {
		t.Fatal("a folder is not below itself")
	}
	if !SameOrBelow(child, dir) {
		t.Fatal("a child is not below its parent")
	}
	if SameOrBelow(base, dir) {
		t.Fatal("a parent is below its child")
	}
	if SameOrBelow(filepath.Join(base, "missing"), dir) {
		t.Fatal("a missing sibling is below its sibling")
	}

	other := filepath.Join(base, "Other")
	if err := os.MkdirAll(filepath.Join(other, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "other")); err != nil {
		t.Skip("volume is case sensitive")
	}
	if !SameOrBelow(filepath.Join(base, "other", "child"), other) {
		t.Fatalf("a child spelled in another letter case is not below %q", other)
	}
}

func TestRealPath(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolved, err := RealPath(link)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("RealPath(%q) = %q, want %q", link, resolved, want)
	}
	missing, err := RealPath(filepath.Join(link, "not", "yet"))
	if err != nil {
		t.Fatal(err)
	}
	if missing != filepath.Join(want, "not", "yet") {
		t.Fatalf("RealPath of a missing child = %q, want %q", missing, filepath.Join(want, "not", "yet"))
	}
}

func TestRelate(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "dir")
	child := filepath.Join(dir, "child")
	sibling := filepath.Join(base, "sibling")
	for _, path := range []string{child, sibling} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		a, b string
		want Relation
	}{
		{dir, dir, Same},
		{child, dir, Inside},
		{dir, child, Contains},
		{sibling, dir, Unrelated},
		{child, sibling, Unrelated},
	}
	for _, c := range cases {
		if got := Relate(c.a, c.b); got != c.want {
			t.Errorf("Relate(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
