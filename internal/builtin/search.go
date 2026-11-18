package builtin

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SearchRequest is file-search's payload. A request with Reveal set searches
// nothing: it shows that file in the file manager (the wire does that part).
type SearchRequest struct {
	Query  string `json:"query"`
	In     string `json:"in"`
	Path   string `json:"path"`
	Limit  int    `json:"limit"`
	Reveal string `json:"reveal"`
}

// SearchMatch is one hit: a file whose name matches (Line 0, Text ""), or one
// line of a file whose text matches.
type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// SearchResult is file-search's answer. Files counts the files looked at.
type SearchResult struct {
	Matches   []SearchMatch `json:"matches"`
	Files     int           `json:"files"`
	Truncated bool          `json:"truncated"`
}

const (
	maxQuery          = 200
	defaultLimit      = 100
	maxLimit          = 500
	maxSearchFiles    = 20000
	maxSearchFileSize = 2 << 20
	maxLineText       = 240
	searchBudget      = 20 * time.Second
	binarySniff       = 8000
)

// Search finds query, ignoring case, in the names and the text of the files
// under root/req.Path. Hidden names, symlinks, files over 2 MiB and files that
// look binary are skipped. progress, when set, is told how many files were read.
func Search(ctx context.Context, root string, req SearchRequest, progress func(files int)) (SearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" || len(query) > maxQuery {
		return SearchResult{}, invalid("query")
	}
	names, contents := true, true
	switch req.In {
	case "", "both":
	case "names":
		contents = false
	case "contents":
		names = false
	default:
		return SearchResult{}, invalid("in")
	}
	limit := req.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return SearchResult{}, invalid("limit")
	}
	start, perr := relPath(req.Path)
	if perr != nil {
		return SearchResult{}, perr
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return SearchResult{}, failed(err)
	}
	defer r.Close()
	fsys := r.FS()
	top := filepath.ToSlash(start)
	needle := strings.ToLower(query)
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, searchBudget)
	defer cancel()

	result := SearchResult{Matches: []SearchMatch{}}
	full := func() bool {
		if len(result.Matches) >= limit {
			result.Truncated = true
			return true
		}
		return false
	}
	walkErr := fs.WalkDir(fsys, top, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == top {
				return err
			}
			return nil
		}
		if ctx.Err() != nil {
			result.Truncated = true
			return fs.SkipAll
		}
		if p != top && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if result.Files >= maxSearchFiles {
			result.Truncated = true
			return fs.SkipAll
		}
		result.Files++
		if progress != nil {
			progress(result.Files)
		}
		if names && strings.Contains(strings.ToLower(d.Name()), needle) {
			result.Matches = append(result.Matches, SearchMatch{Path: p})
			if full() {
				return fs.SkipAll
			}
		}
		if !contents {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSearchFileSize {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil || bytes.IndexByte(data[:min(len(data), binarySniff)], 0) >= 0 {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(strings.ToLower(line), needle) {
				continue
			}
			result.Matches = append(result.Matches, SearchMatch{Path: p, Line: i + 1, Text: clip(line)})
			if full() {
				return fs.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return SearchResult{}, &Error{Code: "not_found", Message: "That folder does not exist."}
		}
		return SearchResult{}, failed(walkErr)
	}
	if parent.Err() != nil {
		return SearchResult{}, parent.Err()
	}
	return result, nil
}

// clip trims a line and cuts it to maxLineText characters.
func clip(line string) string {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) > maxLineText {
		return string(runes[:maxLineText]) + "…"
	}
	return string(runes)
}
