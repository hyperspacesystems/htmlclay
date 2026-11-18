package builtin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var coded *Error
	if !errors.As(err, &coded) {
		t.Fatalf("want a coded error, got %v", err)
	}
	return coded.Code
}

func TestSearchFindsNamesAndLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes/todo.txt", []byte("buy milk\nCall the Budget office\n"))
	writeFile(t, dir, "budget.csv", []byte("x"))
	writeFile(t, dir, ".hidden/budget.txt", []byte("budget"))
	writeFile(t, dir, "bin.dat", []byte("\x00budget"))

	result, err := Search(context.Background(), dir, SearchRequest{Query: "budget"}, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []SearchMatch{
		{Path: "budget.csv"},
		{Path: "notes/todo.txt", Line: 2, Text: "Call the Budget office"},
	}
	if !slices.Equal(result.Matches, want) {
		t.Fatalf("matches = %+v, want %+v", result.Matches, want)
	}
	if result.Files != 3 {
		t.Fatalf("Files = %d, want 3", result.Files)
	}
}

func TestSearchRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{"..", "../x", "/etc", "a/../b", ".git", `a\b`} {
		_, err := Search(context.Background(), dir, SearchRequest{Query: "budget", Path: path}, nil)
		if got := codeOf(t, err); got != "invalid_request" {
			t.Errorf("Path %q gave code %q, want invalid_request", path, got)
		}
	}
}

func TestSearchStopsAtTheLimit(t *testing.T) {
	dir := t.TempDir()
	for i := range 10 {
		writeFile(t, dir, "file"+string(rune('a'+i))+".txt", []byte("hit"))
	}
	result, err := Search(context.Background(), dir, SearchRequest{Query: "hit", Limit: 3}, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Matches) != 3 {
		t.Fatalf("matches = %d, want 3", len(result.Matches))
	}
	if !result.Truncated {
		t.Fatal("Truncated = false, want true")
	}
}

func TestSearchDoesNotFollowSymlinksOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege on Windows")
	}
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt", []byte("budget"))
	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	result, err := Search(context.Background(), dir, SearchRequest{Query: "budget"}, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, match := range result.Matches {
		if match.Path == "out" || filepath.Dir(match.Path) == "out" {
			t.Fatalf("a match went through a symlink: %+v", match)
		}
	}
}

func TestExamplesAreEmbedded(t *testing.T) {
	for _, id := range IDs {
		name, data, ok := Example(id)
		if !ok {
			t.Fatalf("Example(%q) is missing", id)
		}
		if name != ExampleNames[id] {
			t.Fatalf("Example(%q) name = %q, want %q", id, name, ExampleNames[id])
		}
		if len(data) == 0 {
			t.Fatalf("Example(%q) is empty", id)
		}
	}
	if _, _, ok := Example("ai-edit"); ok {
		t.Fatal("ai-edit is not a built-in program with an example")
	}
}

func TestRevealTargetStaysInside(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", []byte("a"))
	got, err := RevealTarget(dir, "a.txt")
	if err != nil {
		t.Fatalf("RevealTarget: %v", err)
	}
	if want := filepath.Join(dir, "a.txt"); got != want {
		t.Fatalf("RevealTarget = %q, want %q", got, want)
	}
	if _, err := RevealTarget(dir, "../x"); codeOf(t, err) != "invalid_request" {
		t.Fatalf("RevealTarget(../x) = %v, want invalid_request", err)
	}
	if _, err := RevealTarget(dir, "missing.txt"); codeOf(t, err) != "not_found" {
		t.Fatalf("RevealTarget(missing.txt) = %v, want not_found", err)
	}
}
