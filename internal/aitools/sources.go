package aitools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/panphora/htmlclay/internal/gitread"
)

// maxFileScan bounds how much of one file a file source reads to find its
// lines; the lines themselves stop at gitread.MaxBytes.
const maxFileScan = 64 << 20

var sourceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// SourceRef is one source the agent named in its answer.
type SourceRef struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"`
	Base  string   `json:"base"`
	Head  string   `json:"head"`
	Paths []string `json:"paths"`
	Path  string   `json:"path"`
	First int      `json:"first"`
	Last  int      `json:"last"`
}

// Answer is the agent's whole answer, in the shape Wrap describes.
type Answer struct {
	Data    json.RawMessage `json:"data"`
	Sources []SourceRef     `json:"sources"`
}

// Source is one expanded source as wire/done carries it. The text itself is a
// run file at URL, never part of the frame.
type Source struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	URL       string   `json:"url"`
	Bytes     int      `json:"bytes"`
	SHA256    string   `json:"sha256"`
	Truncated bool     `json:"truncated"`
	Skipped   []string `json:"skipped"`
	Base      string   `json:"base,omitempty"`
	Head      string   `json:"head,omitempty"`
	Path      string   `json:"path,omitempty"`
	Lines     []int    `json:"lines,omitempty"`
}

// Put stores one run file of the run and returns its address and SHA-256. A
// refusal it returns as *Error reaches the page as is.
type Put func(id string, body []byte) (url, sha256 string, err error)

// Gate allows one gitread call per folder at a time and two in all. go-git's
// merge base, rename detection and status take no context, so a call that
// timed out keeps working in the background; the gate keeps many requests
// from piling such work up.
type Gate struct {
	once    sync.Once
	mu      sync.Mutex
	folders map[string]chan struct{}
	all     chan struct{}
}

func (g *Gate) init() {
	g.once.Do(func() {
		g.folders = map[string]chan struct{}{}
		g.all = make(chan struct{}, 2)
	})
}

// Do runs read once the folder's slot and one of the two shared slots are
// free, or answers ctx's error when ctx ends first.
func (g *Gate) Do(ctx context.Context, folder string, read func(context.Context) error) error {
	g.init()
	g.mu.Lock()
	slot := g.folders[folder]
	if slot == nil {
		slot = make(chan struct{}, 1)
		g.folders[folder] = slot
	}
	g.mu.Unlock()
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-slot }()
	select {
	case g.all <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.all }()
	return read(ctx)
}

// Expand turns the sources the agent named into run files. The diff the page
// shows always comes from here, never from the agent's own text.
func Expand(ctx context.Context, gate *Gate, folder string, refs []SourceRef, put Put) ([]Source, error) {
	if len(refs) == 0 {
		return []Source{}, nil
	}
	if len(refs) > maxSources {
		return nil, fail(CodeResultInvalid, fmt.Sprintf("The agent named more than %d sources.", maxSources))
	}
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return nil, folderMissing(folder)
	}
	seen := map[string]bool{}
	out := make([]Source, 0, len(refs))
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !sourceID.MatchString(ref.ID) || seen[ref.ID] {
			return nil, fail(CodeResultInvalid, fmt.Sprintf("The agent named a source with an unusable id %q.", ref.ID))
		}
		seen[ref.ID] = true
		var src Source
		var body []byte
		var err error
		switch ref.Kind {
		case "git":
			src, body, err = gitSource(ctx, gate, folder, ref)
		case "file":
			src, body, err = fileSource(folder, ref)
		default:
			err = fail(CodeResultInvalid, fmt.Sprintf("The agent named a source of an unknown kind %q.", ref.Kind))
		}
		if err != nil {
			return nil, err
		}
		url, sum, err := put(ref.ID, body)
		if err != nil {
			return nil, err
		}
		src.ID, src.Kind, src.URL, src.SHA256, src.Bytes = ref.ID, ref.Kind, url, sum, len(body)
		if src.Skipped == nil {
			src.Skipped = []string{}
		}
		out = append(out, src)
	}
	return out, nil
}

func gitSource(ctx context.Context, gate *Gate, folder string, ref SourceRef) (Source, []byte, error) {
	var patch *gitread.Patch
	err := gate.Do(ctx, folder, func(ctx context.Context) error {
		var err error
		patch, err = gitread.Diff(ctx, folder, gitread.DiffOptions{Base: ref.Base, Head: ref.Head, Paths: ref.Paths, Exclude: gitread.DefaultExclude})
		return err
	})
	if err != nil {
		return Source{}, nil, gitError(ctx, folder, err)
	}
	skipped := make([]string, 0, len(patch.Skipped))
	for _, s := range patch.Skipped {
		skipped = append(skipped, s.Path)
	}
	return Source{Base: patch.Base, Head: patch.Head, Truncated: patch.Truncated || patch.SkippedMore > 0, Skipped: skipped}, patch.Text, nil
}

// gitError is a gitread failure as the page sees it.
func gitError(ctx context.Context, folder string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var unsupported *gitread.Unsupported
	var refused *gitread.Refused
	switch {
	case errors.As(err, &unsupported):
		return fail(CodeSourceUnsupported, fmt.Sprintf("This repository uses %s, which HTML Clay cannot read.", unsupported.Feature))
	case errors.Is(err, gitread.ErrIncomplete):
		return fail(CodeSourceUnsupported, "This repository is missing commits HTML Clay needs, as a shallow clone is. Fetch its full history, then try again.")
	case errors.As(err, &refused):
		return fail(CodeSourceRefused, fmt.Sprintf("The agent asked for the %s %q, which HTML Clay does not read.", refused.What, refused.Value))
	case errors.Is(err, gitread.ErrUnknownRef):
		return fail(CodeSourceRefused, "The agent named a branch or commit that does not exist in this folder.")
	case errors.Is(err, gitread.ErrNoMergeBase):
		return fail(CodeSourceRefused, "The two commits the agent named share no history.")
	case errors.Is(err, gitread.ErrNotRepository):
		return fail(CodeSourceRefused, "The agent named a git range, but this folder is not a git repository.")
	case errors.Is(err, gitread.ErrTimeout):
		return fail(CodeSourceRefused, "Reading the repository took too long. Ask about a smaller part of it.")
	case errors.Is(err, fs.ErrNotExist):
		return folderMissing(folder)
	}
	return fail(CodeSourceRefused, "HTML Clay could not read that git range: "+err.Error())
}

func fileSource(folder string, ref SourceRef) (Source, []byte, error) {
	name := strings.TrimPrefix(ref.Path, "./")
	if !gitread.ValidPath(name) {
		return Source{}, nil, fail(CodeSourceRefused, fmt.Sprintf("The agent asked for %q, which is not a file inside the folder.", ref.Path))
	}
	name = path.Clean(name)
	root, err := os.OpenRoot(folder)
	if err != nil {
		return Source{}, nil, folderMissing(folder)
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Source{}, nil, fail(CodeSourceRefused, fmt.Sprintf("The agent named %q, which does not exist in the folder.", name))
		}
		return Source{}, nil, fail(CodeSourceRefused, fmt.Sprintf("The agent asked for %q, which is not a file HTML Clay may read inside the folder.", name))
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return Source{}, nil, fail(CodeSourceRefused, fmt.Sprintf("The agent asked for %q, which is not a plain file.", name))
	}
	first, last := ref.First, ref.Last
	if first < 1 {
		first = 1
	}
	text, end, truncated, err := readLines(io.LimitReader(f, maxFileScan), first, last)
	if err != nil {
		return Source{}, nil, fail(CodeSourceRefused, fmt.Sprintf("HTML Clay could not read %q.", name))
	}
	return Source{Path: name, Lines: []int{first, end}, Truncated: truncated}, text, nil
}

// readLines returns lines first to last (1-based, inclusive; last below first
// reads to the end of the file), the last line it returned, and whether it
// stopped at gitread.MaxBytes.
func readLines(r io.Reader, first, last int) ([]byte, int, bool, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var out []byte
	line, end := 1, first-1
	for {
		chunk, err := br.ReadSlice('\n')
		inRange := line >= first && (last < first || line <= last)
		if inRange && len(chunk) > 0 {
			if len(out)+len(chunk) > gitread.MaxBytes {
				return out, end, true, nil
			}
			out = append(out, chunk...)
		}
		switch {
		case err == bufio.ErrBufferFull:
			continue
		case err == io.EOF:
			if inRange && len(chunk) > 0 {
				end = line
			}
			return out, end, false, nil
		case err != nil:
			return nil, 0, false, err
		}
		if inRange {
			end = line
		}
		if last >= first && line >= last {
			return out, end, false, nil
		}
		line++
	}
}

func folderMissing(folder string) *Error {
	return fail(CodeFolderMissing, fmt.Sprintf("HTML Clay cannot find the folder %s. It may have been moved or renamed.", baseName(folder)))
}

func baseName(folder string) string {
	folder = strings.TrimRight(folder, `/\`)
	if i := strings.LastIndexAny(folder, `/\`); i >= 0 {
		return folder[i+1:]
	}
	return folder
}
