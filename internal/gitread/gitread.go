// Package gitread reads a person's git repository for a page: the diff of a
// branch against its base, the branch's commits, the branch list, a file at a
// commit and the working tree status. It reads through go-git inside HTML
// Clay and never starts a process, so no program a repository names can run:
// not fsmonitor, an external diff, a diff driver, a filter, a signature
// program, a pager, a hook or a lazy fetch. It never writes to the repository.
//
// Every call opens the repository afresh, checks its input before it reads,
// and stops at a byte limit and a time limit. A repository go-git cannot read
// whole (a partial clone, a newer format, an extension it does not know) is
// refused with *Unsupported, never read in part. Only the chosen folder's own
// .git is read, never one in a folder above it.
//
// gitread never calls go-git's Fetch, Pull, Push, Clone or submodule update:
// go-git's file transport is the one place go-git starts a process
// (plumbing/transport/file/client.go).
package gitread

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
)

const (
	// MaxBytes is one source's limit, a diff or a file. It matches a run
	// file's (wire.MaxRunFileBytes).
	MaxBytes = 4 << 20
	// Timeout bounds one call when the caller's context has no earlier
	// deadline.
	Timeout = 20 * time.Second
	// MaxCommits is Log's default and largest count.
	MaxCommits = 500
	// MaxEntries bounds Branches and Status.
	MaxEntries = 5000
)

// fileDiffTimeout bounds the line diff of one file. Past it the rest of the
// file shows as removed and added whole, as git does for a diff too costly to
// refine, instead of stalling the review: go-git's own diff waits an hour, and
// a fully rewritten 30,000 line file takes it 3 seconds.
var fileDiffTimeout = time.Second

var (
	ErrNotRepository = errors.New("gitread: the folder is not a git repository")
	ErrUnknownRef    = errors.New("gitread: no such branch, tag or commit")
	ErrNoMergeBase   = errors.New("gitread: the two commits share no history")
	ErrNoFile        = errors.New("gitread: no such file at that commit")
	ErrTooLarge      = errors.New("gitread: the file is larger than the limit")
	ErrIncomplete    = errors.New("gitread: the repository is missing objects it needs, as a shallow clone does")
	ErrTimeout       = errors.New("gitread: reading the repository took too long")
)

// Unsupported is a repository gitread refuses whole: a partial clone, a newer
// format, or any extension go-git does not know.
type Unsupported struct {
	Feature string
}

func (e *Unsupported) Error() string {
	return "gitread: this repository uses " + e.Feature + ", which HTML Clay cannot read"
}

// Refused is input no call accepts: a ref that could read as an option or a
// range, a path that is absolute or climbs out with "..", a bad pattern.
type Refused struct {
	What  string
	Value string
}

func (e *Refused) Error() string {
	return "gitread: refused " + e.What + " " + strconv.Quote(e.Value)
}

// ValidRef reports whether ref names a branch, tag or commit in a shape git
// allows, plus the revision suffixes gitread resolves: never an option or a
// range. It follows git's check-ref-format rules and refuses a control byte,
// space, ":", "?", "*", "[", "\\", a leading "-" and any "..".
func ValidRef(ref string) bool {
	if ref == "" || len(ref) > 200 || !utf8.ValidString(ref) || strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") {
		return false
	}
	for _, r := range ref {
		if r < 0x20 || r == 0x7f {
			return false
		}
		switch r {
		case ' ', ':', '?', '*', '[', '\\':
			return false
		}
	}
	for i := 0; i < len(ref); i++ {
		if ref[i] != '~' && ref[i] != '^' {
			continue
		}
		digits := 0
		for j := i + 1; j < len(ref) && ref[j] >= '0' && ref[j] <= '9'; j++ {
			digits++
		}
		if digits > 4 {
			return false
		}
	}
	return true
}

// ValidPath reports whether p names a place inside the repository: relative,
// "/"-separated, with no ".." part and no Windows drive or separator.
func ValidPath(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) || strings.HasPrefix(p, "/") {
		return false
	}
	if strings.ContainsRune(p, '\\') || len(p) > 1 && p[1] == ':' {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

// checkDotGit refuses a .git entry that would make go-git read a repository
// outside dir: a link, a .git folder with a commondir (git writes one only
// inside .git/worktrees/<name>), or a .git file that is not this folder's own
// entry in the worktrees of the repository it names.
func checkDotGit(dir string) error {
	entry := filepath.Join(dir, ".git")
	fi, err := os.Lstat(entry)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotRepository
	}
	if err != nil {
		return fmt.Errorf("gitread: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return &Unsupported{Feature: "a .git entry that is a link"}
	}
	if fi.IsDir() {
		if _, err := os.Lstat(filepath.Join(entry, "commondir")); err == nil {
			return &Unsupported{Feature: "a .git folder with a commondir"}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("gitread: %w", err)
		}
		return checkAlternates(entry)
	}
	if !fi.Mode().IsRegular() {
		return &Unsupported{Feature: "a .git entry that is not a file or a folder"}
	}
	gitdir, err := linkedGitDir(entry, dir)
	if err != nil {
		return err
	}
	if filepath.Base(filepath.Dir(gitdir)) != "worktrees" {
		if underModules(gitdir) {
			return checkSubmoduleWorktree(gitdir, dir)
		}
		return &Unsupported{Feature: "a .git file outside a worktrees folder"}
	}
	backBody, err := readRepoFile(filepath.Join(gitdir, "gitdir"))
	if err != nil {
		return &Unsupported{Feature: "a .git file whose worktree entry does not name it"}
	}
	link, err := resolveFrom(gitdir, strings.TrimSpace(string(backBody)))
	if err != nil {
		return &Unsupported{Feature: "a .git file whose worktree entry does not name it"}
	}
	back, err := os.Stat(link)
	if err != nil || !os.SameFile(back, fi) {
		return &Unsupported{Feature: "a .git file whose worktree entry does not name it"}
	}
	body, err := readRepoFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return &Unsupported{Feature: "a .git file whose worktree entry has no commondir"}
	}
	common, err := resolveFrom(gitdir, strings.TrimSpace(string(body)))
	if err != nil {
		return &Unsupported{Feature: "a .git file whose commondir does not exist"}
	}
	commonInfo, err := os.Stat(common)
	if err != nil {
		return &Unsupported{Feature: "a .git file whose commondir does not exist"}
	}
	parentInfo, err := os.Stat(filepath.Dir(filepath.Dir(gitdir)))
	if err != nil || !os.SameFile(commonInfo, parentInfo) {
		return &Unsupported{Feature: "a .git file whose commondir is not the folder above worktrees"}
	}
	return checkAlternates(common)
}

// checkAlternates refuses a repository that borrows objects from another
// repository, which would let it read outside the chosen folder.
func checkAlternates(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, "objects", "info", "alternates")); err == nil {
		return &Unsupported{Feature: "a repository that borrows objects from another (alternates)"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("gitread: %w", err)
	}
	return nil
}

// underModules reports whether gitdir is at some depth under a ".git/modules"
// folder, the storage git keeps for a repository's submodules.
func underModules(gitdir string) bool {
	parts := strings.Split(filepath.ToSlash(gitdir), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == ".git" && parts[i+1] == "modules" {
			return true
		}
	}
	return false
}

// checkSubmoduleWorktree accepts a submodule's own folder: a .git file naming
// its storage under .git/modules, with a core.worktree in that storage that
// resolves back to the folder.
func checkSubmoduleWorktree(gitdir, dir string) error {
	body, err := readRepoFile(filepath.Join(gitdir, "config"))
	if err != nil {
		return &Unsupported{Feature: "a submodule folder whose storage has no config"}
	}
	cfg := config.New()
	if err := config.NewDecoder(bytes.NewReader(body)).Decode(cfg); err != nil {
		return &Unsupported{Feature: "a submodule folder whose storage has no config"}
	}
	worktree := cfg.Section("core").Options.Get("worktree")
	if worktree == "" {
		return &Unsupported{Feature: "a submodule folder whose storage names no worktree"}
	}
	resolved, err := resolveFrom(gitdir, worktree)
	if err != nil {
		return &Unsupported{Feature: "a submodule folder whose worktree does not exist"}
	}
	back, err := os.Stat(resolved)
	chosen, cerr := os.Stat(dir)
	if err != nil || cerr != nil || !os.SameFile(back, chosen) {
		return &Unsupported{Feature: "a submodule folder whose storage names another folder"}
	}
	if _, err := os.Lstat(filepath.Join(gitdir, "commondir")); err == nil {
		return &Unsupported{Feature: "a submodule folder whose storage has a commondir"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("gitread: %w", err)
	}
	return checkAlternates(gitdir)
}

// readRepoFile reads a small file the repository controls, refusing a named
// pipe or a device, which would block the read or never end it.
func readRepoFile(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	return os.ReadFile(path)
}

// linkedGitDir is the folder a .git file names, resolved.
func linkedGitDir(entry, dir string) (string, error) {
	body, err := readRepoFile(entry)
	if err != nil {
		return "", fmt.Errorf("gitread: %w", err)
	}
	const prefix = "gitdir: "
	if !strings.HasPrefix(string(body), prefix) {
		return "", &Unsupported{Feature: "a .git file with no gitdir line"}
	}
	gitdir, _, _ := strings.Cut(string(body)[len(prefix):], "\n")
	return resolveFrom(dir, strings.TrimSpace(gitdir))
}

// resolveFrom resolves a path read from a file at from: a relative one is
// relative to from, and every link in it is followed.
func resolveFrom(from, name string) (string, error) {
	if !filepath.IsAbs(name) {
		name = filepath.Join(from, name)
	}
	return filepath.EvalSymlinks(name)
}

func open(dir string) (*git.Repository, error) {
	if !filepath.IsAbs(dir) {
		return nil, &Refused{What: "folder", Value: dir}
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("gitread: %w", err)
	}
	if err := checkDotGit(dir); err != nil {
		return nil, err
	}
	fs, err := repositoryFS(dir)
	if err != nil {
		return nil, err
	}
	st := filesystem.NewStorageWithOptions(fs, cache.NewObjectLRU(16*cache.MiByte), filesystem.Options{LargeObjectThreshold: 1})
	r, err := git.Open(&boundedStorage{Storer: st, fs: fs}, readFS{osfs.New(dir)})
	switch {
	case errors.Is(err, git.ErrRepositoryNotExists):
		return nil, ErrNotRepository
	case errors.Is(err, git.ErrUnknownExtension), errors.Is(err, git.ErrUnsupportedExtensionRepositoryFormatVersion):
		return nil, &Unsupported{Feature: "extensions." + detail(err)}
	case err != nil:
		return nil, fmt.Errorf("gitread: %w", err)
	}
	cfg, err := r.Config()
	if err != nil {
		return nil, fmt.Errorf("gitread: %w", err)
	}
	if v := cfg.Raw.Section("core").Options.Get("repositoryformatversion"); v != "" && v != "0" && v != "1" {
		return nil, &Unsupported{Feature: "repository format " + v}
	}
	return r, nil
}

// repositoryFS is the folder go-git reads the repository from: dir/.git, or
// the folder a linked worktree's .git file names, with its commondir.
func repositoryFS(dir string) (billy.Filesystem, error) {
	entry := filepath.Join(dir, ".git")
	fi, err := os.Lstat(entry)
	if err != nil {
		return nil, fmt.Errorf("gitread: %w", err)
	}
	root := entry
	if !fi.IsDir() {
		root, err = linkedGitDir(entry, dir)
		if err != nil {
			return nil, err
		}
	}
	dot := readFS{osfs.New(root)}
	body, err := readRepoFile(filepath.Join(root, "commondir"))
	if errors.Is(err, os.ErrNotExist) {
		return dot, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gitread: %w", err)
	}
	common, err := resolveFrom(root, strings.TrimSpace(string(body)))
	if err != nil {
		return nil, fmt.Errorf("gitread: %w", err)
	}
	return dotgit.NewRepositoryFilesystem(dot, readFS{osfs.New(common)}), nil
}

func detail(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

// within opens dir and runs read with a deadline. Some go-git steps (the
// merge base walk, rename detection, status) take no context, so the call
// returns at the deadline while such a step finishes on its own repository
// copy and is dropped.
func within[T any](ctx context.Context, dir string, read func(context.Context, *git.Repository) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		r, err := open(dir)
		if err != nil {
			done <- result{err: err}
			return
		}
		v, err := read(ctx, r)
		done <- result{v, err}
	}()
	var zero T
	select {
	case res := <-done:
		if res.err != nil && ctx.Err() != nil {
			return zero, stopped(ctx)
		}
		if errors.Is(res.err, plumbing.ErrObjectNotFound) {
			return zero, ErrIncomplete
		}
		return res.v, res.err
	case <-ctx.Done():
		return zero, stopped(ctx)
	}
}

func stopped(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ctx.Err()
}

func commitAt(r *git.Repository, ref string) (*object.Commit, error) {
	names := []plumbing.ReferenceName{}
	if ref == "HEAD" {
		names = append(names, plumbing.HEAD)
	}
	names = append(names,
		plumbing.NewBranchReferenceName(ref),
		plumbing.ReferenceName("refs/remotes/"+ref),
		plumbing.NewTagReferenceName(ref),
	)
	for _, name := range names {
		if c, ok := commitNamed(r, name); ok {
			return c, nil
		}
	}
	h, err := r.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRef, ref)
	}
	c, err := r.CommitObject(*h)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRef, ref)
	}
	return c, nil
}

// commitNamed resolves an exact reference, peeling a symbolic ref and an
// annotated tag to its commit.
func commitNamed(r *git.Repository, name plumbing.ReferenceName) (*object.Commit, bool) {
	ref, err := r.Reference(name, true)
	if err != nil {
		return nil, false
	}
	if c, err := r.CommitObject(ref.Hash()); err == nil {
		return c, true
	}
	if t, err := r.TagObject(ref.Hash()); err == nil {
		if c, err := t.Commit(); err == nil {
			return c, true
		}
	}
	return nil, false
}

func checkRefs(refs ...string) error {
	for _, ref := range refs {
		if !ValidRef(ref) {
			return &Refused{What: "ref", Value: ref}
		}
	}
	return nil
}
