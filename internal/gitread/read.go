package gitread

import (
	"container/heap"
	"context"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage"
)

const maxSubject = 300

type Commit struct {
	Hash    string    `json:"hash"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	When    time.Time `json:"when"`
}

// Log is git log base..head: the commits head has and base does not, newest
// first, at most max (MaxCommits when max is zero or larger). Ask for one
// more than you show to learn whether there are more.
func Log(ctx context.Context, dir, base, head string, max int) ([]Commit, error) {
	if err := checkRefs(base, head); err != nil {
		return nil, err
	}
	if max <= 0 || max > MaxCommits {
		max = MaxCommits
	}
	return within(ctx, dir, func(ctx context.Context, r *git.Repository) ([]Commit, error) {
		b, err := commitAt(r, base)
		if err != nil {
			return nil, err
		}
		h, err := commitAt(r, head)
		if err != nil {
			return nil, err
		}
		const (
			interesting   = 1
			uninteresting = 2
		)
		marks := map[plumbing.Hash]int{}
		queued := commitQueue{}
		queuedInteresting := 0
		push := func(c *object.Commit, mark int) {
			cur, ok := marks[c.Hash]
			if ok && cur >= mark {
				return
			}
			if ok && cur == interesting {
				queuedInteresting--
			}
			marks[c.Hash] = mark
			if mark == interesting {
				queuedInteresting++
			}
			heap.Push(&queued, c)
		}
		push(b, uninteresting)
		push(h, interesting)

		var out []Commit
		handled := map[plumbing.Hash]int{}
		slop := -1
		seen := 0
		for queued.Len() > 0 {
			c := heap.Pop(&queued).(*object.Commit)
			mark := marks[c.Hash]
			if handled[c.Hash] == mark {
				continue
			}
			handled[c.Hash] = mark
			seen++
			if seen%100 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			if mark == interesting {
				queuedInteresting--
				out = append(out, Commit{Hash: c.Hash.String(), Subject: subject(c.Message), Author: c.Author.Name, When: c.Author.When})
				if len(out) >= max {
					return out, nil
				}
			}
			for i := 0; i < c.NumParents(); i++ {
				parent, err := c.Parent(i)
				if err != nil {
					return nil, err
				}
				push(parent, mark)
			}
			if queuedInteresting == 0 {
				if slop < 0 {
					slop = 0
				} else {
					slop++
				}
				if slop > 5 {
					break
				}
			} else {
				slop = -1
			}
		}
		return out, nil
	})
}

type commitQueue []*object.Commit

func (q commitQueue) Len() int { return len(q) }
func (q commitQueue) Less(i, j int) bool {
	a, b := q[i].Committer.When, q[j].Committer.When
	if a.Equal(b) {
		return q[i].Hash.String() < q[j].Hash.String()
	}
	return a.After(b)
}
func (q commitQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *commitQueue) Push(x any)   { *q = append(*q, x.(*object.Commit)) }
func (q *commitQueue) Pop() any {
	old := *q
	n := len(old)
	c := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return c
}

func subject(message string) string {
	s, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	s = strings.TrimSpace(s)
	if len(s) > maxSubject {
		s = strings.ToValidUTF8(s[:maxSubject], "")
	}
	return s
}

type Branch struct {
	// Name is "main", or "origin/main" for a remote-tracking branch.
	Name    string `json:"name"`
	Hash    string `json:"hash"`
	Remote  bool   `json:"remote,omitempty"`
	Current bool   `json:"current,omitempty"`
}

// Branches lists local branches, then remote-tracking ones, each by name, at
// most MaxEntries.
func Branches(ctx context.Context, dir string) ([]Branch, error) {
	return within(ctx, dir, func(ctx context.Context, r *git.Repository) ([]Branch, error) {
		current := ""
		if h, err := r.Head(); err == nil && h.Name().IsBranch() {
			current = h.Name().Short()
		}
		refs, err := r.References()
		if err != nil {
			return nil, err
		}
		out := []Branch{}
		err = refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Type() != plumbing.HashReference {
				return nil
			}
			switch n := ref.Name(); {
			case n.IsBranch():
				out = append(out, Branch{Name: n.Short(), Hash: ref.Hash().String(), Current: n.Short() == current})
			case n.IsRemote():
				out = append(out, Branch{Name: n.Short(), Hash: ref.Hash().String(), Remote: true})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Remote != out[j].Remote {
				return !out[i].Remote
			}
			return out[i].Name < out[j].Name
		})
		if len(out) > MaxEntries {
			out = out[:MaxEntries]
		}
		return out, nil
	})
}

// FileAt is a file's bytes at a commit (git show rev:name), at most MaxBytes.
// A symbolic link gives the path it points to.
func FileAt(ctx context.Context, dir, rev, name string) ([]byte, error) {
	if err := checkRefs(rev); err != nil {
		return nil, err
	}
	if !ValidPath(name) {
		return nil, &Refused{What: "path", Value: name}
	}
	name = path.Clean(name)
	return within(ctx, dir, func(ctx context.Context, r *git.Repository) ([]byte, error) {
		c, err := commitAt(r, rev)
		if err != nil {
			return nil, err
		}
		f, err := c.File(name)
		if errors.Is(err, object.ErrFileNotFound) {
			return nil, ErrNoFile
		}
		if err != nil {
			return nil, err
		}
		if f.Size > MaxBytes {
			return nil, ErrTooLarge
		}
		rd, err := f.Reader()
		if err != nil {
			return nil, err
		}
		defer rd.Close()
		b, err := io.ReadAll(io.LimitReader(rd, MaxBytes+1))
		if err != nil {
			return nil, err
		}
		if len(b) > MaxBytes {
			return nil, ErrTooLarge
		}
		return b, nil
	})
}

// StatusEntry is one changed file in the working tree, with git status
// --short's letters: " " unchanged, "M", "A", "D", "R", "C", "U", "?".
type StatusEntry struct {
	Path     string `json:"path"`
	Staging  string `json:"staging"`
	Worktree string `json:"worktree"`
}

// Status lists the working tree's changed and untracked files by path, at
// most MaxEntries. Files the repository's .gitignore files name are left out,
// and so are the ones a normal repository's .git/info/exclude names (a linked
// worktree's is not read); the person's global ignore file is not read.
func Status(ctx context.Context, dir string) ([]StatusEntry, error) {
	return within(ctx, dir, func(ctx context.Context, r *git.Repository) ([]StatusEntry, error) {
		wt, err := r.Worktree()
		if err != nil {
			return nil, err
		}
		plain, err := git.Open(&noSubmodules{Storer: r.Storer}, wt.Filesystem)
		if err != nil {
			return nil, err
		}
		if wt, err = plain.Worktree(); err != nil {
			return nil, err
		}
		st, err := wt.Status()
		if err != nil {
			return nil, err
		}
		out := make([]StatusEntry, 0, len(st))
		for name, s := range st {
			if s.Staging == git.Unmodified && s.Worktree == git.Unmodified {
				continue
			}
			out = append(out, StatusEntry{Path: name, Staging: string(rune(s.Staging)), Worktree: string(rune(s.Worktree))})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		if len(out) > MaxEntries {
			out = out[:MaxEntries]
		}
		return out, nil
	})
}

// noSubmodules gives Status a config with no submodule marked initialized, so
// go-git leaves each one at its index hash and never opens or writes a
// submodule in a linked worktree, where modules/<name> is not set up.
type noSubmodules struct {
	storage.Storer
}

func (s *noSubmodules) Config() (*config.Config, error) {
	cfg, err := s.Storer.Config()
	if err != nil {
		return nil, err
	}
	cfg.Submodules = map[string]*config.Submodule{}
	return cfg, nil
}
