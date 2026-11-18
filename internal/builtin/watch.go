package builtin

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// WatchRequest is folder-watch's payload.
type WatchRequest struct {
	Path string `json:"path"`
}

// Change is one file or folder added, changed or removed since the last look.
type Change struct {
	Path   string `json:"path"`
	Change string `json:"change"`
}

const (
	// WatchInterval is how often a watch looks again.
	WatchInterval   = 2 * time.Second
	maxWatchEntries = 10000
	maxWatches      = 8
	maxChangesSent  = 500
	parkedFor       = time.Minute
)

type stamp struct {
	size int64
	mod  int64
	dir  bool
}

type look map[string]stamp

var (
	watchSlots = make(chan struct{}, maxWatches)
	parkedMu   sync.Mutex
	parked     = map[string]parkedLook{}
	errTooMany = errors.New("too many entries")
)

type parkedLook struct {
	look look
	at   time.Time
}

// Watch looks at root/req.Path every interval and hands report each batch of
// changes, at most 500 at a time, until ctx ends; then it returns nil. key names
// the watch (the document and its path): a watch that ends leaves its last look
// for a minute, and the next Watch with the same key starts from it, so a page
// that asks again at once misses nothing in between.
func Watch(ctx context.Context, root, key string, req WatchRequest, interval time.Duration, report func([]Change)) error {
	start, perr := relPath(req.Path)
	if perr != nil {
		return perr
	}
	select {
	case watchSlots <- struct{}{}:
	default:
		return &Error{Code: "busy", Message: "HTML Clay is already watching 8 folders."}
	}
	defer func() { <-watchSlots }()
	r, err := os.OpenRoot(root)
	if err != nil {
		return failed(err)
	}
	defer r.Close()
	fsys := r.FS()
	top := filepath.ToSlash(start)
	last, ok := takeParked(key)
	if !ok {
		if last, err = lookAt(fsys, top); err != nil {
			return err
		}
	}
	defer func() { park(key, last) }()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		next, err := lookAt(fsys, top)
		if err != nil {
			return err
		}
		changes := compare(last, next)
		last = next
		for len(changes) > 0 {
			n := min(len(changes), maxChangesSent)
			report(changes[:n])
			changes = changes[n:]
		}
	}
}

func lookAt(fsys fs.FS, top string) (look, error) {
	seen := look{}
	err := fs.WalkDir(fsys, top, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == top {
				return err
			}
			return nil
		}
		if p == top {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if len(seen) >= maxWatchEntries {
			return errTooMany
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		seen[p] = stamp{size: info.Size(), mod: info.ModTime().UnixNano(), dir: d.IsDir()}
		return nil
	})
	switch {
	case errors.Is(err, errTooMany):
		return nil, &Error{Code: "too_large", Message: "This folder has more than 10,000 items. Watch a smaller folder inside it."}
	case errors.Is(err, fs.ErrNotExist):
		return nil, &Error{Code: "not_found", Message: "That folder does not exist."}
	case err != nil:
		return nil, failed(err)
	}
	return seen, nil
}

func compare(before, after look) []Change {
	var changes []Change
	for p, s := range after {
		old, ok := before[p]
		switch {
		case !ok:
			changes = append(changes, Change{Path: p, Change: "added"})
		case !s.dir && (old.size != s.size || old.mod != s.mod):
			changes = append(changes, Change{Path: p, Change: "changed"})
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			changes = append(changes, Change{Path: p, Change: "removed"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

func park(key string, l look) {
	parkedMu.Lock()
	defer parkedMu.Unlock()
	now := time.Now()
	for k, p := range parked {
		if now.Sub(p.at) > parkedFor {
			delete(parked, k)
		}
	}
	parked[key] = parkedLook{look: l, at: now}
}

func takeParked(key string) (look, bool) {
	parkedMu.Lock()
	defer parkedMu.Unlock()
	p, ok := parked[key]
	delete(parked, key)
	if !ok || time.Since(p.at) > parkedFor {
		return nil, false
	}
	return p.look, true
}
