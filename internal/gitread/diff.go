package gitread

import (
	"bytes"
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/diff"
	dmp "github.com/sergi/go-diff/diffmatchpatch"
)

// renameLimit caps rename detection at this many added and this many deleted
// files, as git's old diff.renameLimit did; past it only exact renames count.
const (
	renameLimit = 400
	maxSkipped  = 500
)

type DiffOptions struct {
	Base, Head string
	// Paths come from the agent or a carried program and match literally: a
	// file, or a folder and everything under it. None means every file.
	Paths []string
	// Exclude are the host's own globs, such as DefaultExclude. "*", "?" and
	// classes match inside one part of a path, "**" matches any number of
	// parts, and a pattern with no "/" matches a file's name in any folder.
	Exclude []string
	// MaxBytes bounds Text; zero, or more than MaxBytes, means MaxBytes.
	MaxBytes int
}

// DefaultExclude are files a review leaves out and lists by name: lock files,
// minified code and vendored code.
var DefaultExclude = []string{
	"package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb",
	"Cargo.lock", "go.sum", "Gemfile.lock", "composer.lock", "poetry.lock", "Pipfile.lock", "uv.lock",
	"*.min.js", "*.min.css", "*.map", "**/vendor/**", "**/node_modules/**",
}

// Why a changed file is in Patch.Skipped instead of Patch.Text.
const (
	SkipExcluded  = "excluded"
	SkipTooLarge  = "too large"
	SkipLimit     = "limit"
	SkipSubmodule = "submodule"
	SkipName      = "name"
)

type File struct {
	Path string `json:"path"`
	// From is the old path of a renamed file.
	From    string `json:"from,omitempty"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Binary  bool   `json:"binary,omitempty"`
}

type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Patch struct {
	Base      string `json:"base"`
	Head      string `json:"head"`
	MergeBase string `json:"mergeBase"`
	// Text is the unified diff of Files, in their order.
	Text    []byte    `json:"-"`
	Files   []File    `json:"files"`
	Skipped []Skipped `json:"skipped,omitempty"`
	// SkippedMore counts the files past Skipped's cap.
	SkippedMore int `json:"skippedMore,omitempty"`
	// Truncated is true when a file was left out because Text had no room.
	Truncated bool `json:"truncated,omitempty"`
}

var errFileTooLarge = errors.New("file too large")

// Diff is git diff base...head: the changes head made since it left base, from
// their merge base. Files Exclude matches, submodules, files with a control
// character in their name, files larger than the limit and files past the
// limit are listed in Skipped instead.
func Diff(ctx context.Context, dir string, o DiffOptions) (*Patch, error) {
	if err := checkRefs(o.Base, o.Head); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(o.Paths))
	for _, p := range o.Paths {
		if !ValidPath(p) {
			return nil, &Refused{What: "path", Value: p}
		}
		paths = append(paths, path.Clean(p))
	}
	for _, g := range o.Exclude {
		if !validPattern(g) {
			return nil, &Refused{What: "pattern", Value: g}
		}
	}
	limit := o.MaxBytes
	if limit <= 0 || limit > MaxBytes {
		limit = MaxBytes
	}
	return within(ctx, dir, func(ctx context.Context, r *git.Repository) (*Patch, error) {
		base, err := commitAt(r, o.Base)
		if err != nil {
			return nil, err
		}
		head, err := commitAt(r, o.Head)
		if err != nil {
			return nil, err
		}
		bases, err := base.MergeBase(head)
		if err != nil {
			return nil, err
		}
		if len(bases) == 0 {
			return nil, ErrNoMergeBase
		}
		from, err := bases[0].Tree()
		if err != nil {
			return nil, err
		}
		to, err := head.Tree()
		if err != nil {
			return nil, err
		}
		changes, err := object.DiffTreeWithOptions(ctx, from, to, &object.DiffTreeOptions{DetectRenames: false})
		if err != nil {
			return nil, err
		}
		p := &Patch{Base: base.Hash.String(), Head: head.Hash.String(), MergeBase: bases[0].Hash.String(), Files: []File{}}
		var kept object.Changes
		for _, c := range changes {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			name, old := c.To.Name, c.From.Name
			if name == "" {
				name, old = old, ""
			}
			if old == name {
				old = ""
			}
			reason := ""
			switch {
			case hasControl(name) || hasControl(old):
				reason = SkipName
			case c.From.TreeEntry.Mode == filemode.Submodule || c.To.TreeEntry.Mode == filemode.Submodule:
				reason = SkipSubmodule
			case tooLarge(r, c):
				reason = SkipTooLarge
			}
			if reason == "" {
				kept = append(kept, c)
				continue
			}
			if underAny(name, paths) || old != "" && underAny(old, paths) {
				p.skip(Skipped{Path: name, Reason: reason})
			}
		}
		changes, err = object.DetectRenames(kept, &object.DiffTreeOptions{RenameScore: 60, RenameLimit: renameLimit})
		if err != nil {
			return nil, err
		}
		var text bytes.Buffer
		for _, c := range changes {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			name, old := c.To.Name, c.From.Name
			if name == "" {
				name, old = old, ""
			}
			if old == name {
				old = ""
			}
			if !underAny(name, paths) && (old == "" || !underAny(old, paths)) {
				continue
			}
			reason := ""
			skipped := name
			switch {
			case excluded(name, o.Exclude):
				reason = SkipExcluded
			case old != "" && excluded(old, o.Exclude):
				reason, skipped = SkipExcluded, old
			case text.Len() >= limit:
				reason = SkipLimit
			}
			if reason == "" {
				fp, f, err := patchFile(ctx, c)
				switch {
				case errors.Is(err, errFileTooLarge):
					reason = SkipTooLarge
				case err != nil:
					return nil, err
				default:
					var buf bytes.Buffer
					if err := fdiff.NewUnifiedEncoder(&buf, fdiff.DefaultContextLines).Encode(onePatch{fp}); err != nil {
						return nil, err
					}
					if text.Len()+buf.Len() > limit {
						reason = SkipLimit
					} else {
						text.Write(buf.Bytes())
						f.Path, f.From = name, old
						p.Files = append(p.Files, f)
					}
				}
			}
			if reason != "" {
				p.skip(Skipped{Path: skipped, Reason: reason})
				p.Truncated = p.Truncated || reason == SkipLimit
			}
		}
		p.Text = text.Bytes()
		return p, nil
	})
}

func patchFile(ctx context.Context, c *object.Change) (*textPatch, File, error) {
	from, to, err := c.Files()
	if errors.Is(err, ErrTooLarge) {
		return nil, File{}, errFileTooLarge
	}
	if err != nil {
		return nil, File{}, err
	}
	for _, f := range []*object.File{from, to} {
		if f != nil && f.Size > int64(MaxBytes) {
			return nil, File{}, errFileTooLarge
		}
	}
	fp := &textPatch{from: c.From, to: c.To}
	var f File
	switch {
	case from == nil:
		f.Status = "added"
	case to == nil:
		f.Status = "deleted"
	case c.From.Name != c.To.Name:
		f.Status = "renamed"
	default:
		f.Status = "modified"
	}
	fromText, fromBinary, err := content(from)
	if err != nil {
		return nil, File{}, err
	}
	toText, toBinary, err := content(to)
	if err != nil {
		return nil, File{}, err
	}
	if fromBinary || toBinary {
		fp.binary, f.Binary = true, true
		return fp, f, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, File{}, err
	}
	timeout := fileDiffTimeout
	if dl, ok := ctx.Deadline(); ok {
		rem := time.Until(dl)
		if rem <= 0 {
			return nil, File{}, ctx.Err()
		}
		if rem < timeout {
			timeout = rem
		}
	}
	for _, d := range diff.DoWithTimeout(fromText, toText, timeout) {
		op := fdiff.Equal
		switch d.Type {
		case dmp.DiffDelete:
			op = fdiff.Delete
			f.Deleted += lineCount(d.Text)
		case dmp.DiffInsert:
			op = fdiff.Add
			f.Added += lineCount(d.Text)
		}
		fp.chunks = append(fp.chunks, chunk{d.Text, op})
	}
	return fp, f, nil
}

// skip records a skipped file, counting the ones past the cap.
func (p *Patch) skip(s Skipped) {
	if len(p.Skipped) < maxSkipped {
		p.Skipped = append(p.Skipped, s)
		return
	}
	p.SkippedMore++
}

// tooLarge reports whether either side of c passes MaxBytes, by the size the
// object header states, so a large blob is never inflated to learn it.
func tooLarge(r *git.Repository, c *object.Change) bool {
	for _, e := range []object.ChangeEntry{c.From, c.To} {
		if e.TreeEntry.Hash.IsZero() {
			continue
		}
		if size, err := r.Storer.EncodedObjectSize(e.TreeEntry.Hash); err == nil && (size > int64(MaxBytes) || size < 0) {
			return true
		}
	}
	return false
}

// content is the file's text, read to MaxBytes+1 at most whatever its header
// claims.
func content(f *object.File) (string, bool, error) {
	if f == nil {
		return "", false, nil
	}
	binary, err := f.IsBinary()
	if err != nil || binary {
		return "", binary, err
	}
	rd, err := f.Reader()
	if err != nil {
		return "", false, err
	}
	defer rd.Close()
	b, err := readBounded(rd, MaxBytes)
	if errors.Is(err, ErrTooLarge) {
		return "", false, errFileTooLarge
	}
	if err != nil {
		return "", false, err
	}
	return string(b), false, nil
}

func lineCount(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// textPatch is go-git's own file patch, built here so the line diff runs
// with fileDiffTimeout, and printed by go-git's unified encoder.
type textPatch struct {
	from, to object.ChangeEntry
	binary   bool
	chunks   []fdiff.Chunk
}

func (p *textPatch) IsBinary() bool        { return p.binary }
func (p *textPatch) Chunks() []fdiff.Chunk { return p.chunks }

func (p *textPatch) Files() (from, to fdiff.File) {
	if p.from.TreeEntry.Mode.IsFile() {
		from = entry{p.from}
	}
	if p.to.TreeEntry.Mode.IsFile() {
		to = entry{p.to}
	}
	return from, to
}

type entry struct{ e object.ChangeEntry }

func (f entry) Hash() plumbing.Hash     { return f.e.TreeEntry.Hash }
func (f entry) Mode() filemode.FileMode { return f.e.TreeEntry.Mode }
func (f entry) Path() string            { return f.e.Name }

type chunk struct {
	content string
	op      fdiff.Operation
}

func (c chunk) Content() string       { return c.content }
func (c chunk) Type() fdiff.Operation { return c.op }

type onePatch struct{ fp fdiff.FilePatch }

func (p onePatch) FilePatches() []fdiff.FilePatch { return []fdiff.FilePatch{p.fp} }
func (p onePatch) Message() string                { return "" }

func underAny(name string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		if p == "." || name == p || strings.HasPrefix(name, p+"/") {
			return true
		}
	}
	return false
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

func validPattern(g string) bool {
	if g == "" || strings.ContainsRune(g, 0) || strings.Count(g, "**") > 2 {
		return false
	}
	_, err := path.Match(g, "")
	return err == nil
}

func excluded(name string, patterns []string) bool {
	for _, g := range patterns {
		if matchGlob(g, name) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, name string) bool {
	if !strings.Contains(pattern, "/") {
		ok, _ := path.Match(pattern, path.Base(name))
		return ok
	}
	return matchParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchParts(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchParts(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], name[0]); !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}
