package mirror

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	maxWarmRetries = 3
	warmEvery      = 5 * time.Minute
	warmFresh      = 24 * time.Hour
	maxWarmFiles   = 50000
	maxWarmWalk    = 200000
	maxWarmParse   = 10 << 20
	warmFetchWait  = 2 * time.Minute
)

// Warmer copies the static files that HTML files in the person's folders load, before
// any of them is opened. It polls the folders it is given, and for each HTML file that
// is new or changed since the last pass it fetches the absolute http and https
// addresses on its static tags into the shared index, following each stylesheet's
// references. Runtime loads (fetch, dynamic import) stay for the first open to fill.
type Warmer struct {
	Fetch   *Fetcher
	Folders func() []string
	Off     func() bool
	Hints   Hints
	// Images also warms what img, picture sources and video posters load. Off by
	// default: a saved newsletter in a trusted folder would fire its tracking pixel
	// without anyone opening it. Images a person's open page loads are copied anyway.
	Images bool

	mu   sync.Mutex
	seen map[string]fileStamp
	// retries counts passes in a row a file failed; it stops being retried after
	// maxWarmRetries until it changes.
	retries map[string]int
}

type fileStamp struct {
	size  int64
	mtime int64
}

// Run polls until ctx ends: once at once, then every five minutes.
func (w *Warmer) Run(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		w.Pass(ctx)
		t.Reset(warmEvery)
	}
}

// Reset forgets which files were warmed, so the next pass copies every file's references
// again. Clear and switching back on call it: the copies those files had are gone.
func (w *Warmer) Reset() {
	w.mu.Lock()
	w.seen = nil
	w.retries = nil
	w.mu.Unlock()
}

// Pass scans every folder once and warms the HTML files that are new or changed.
func (w *Warmer) Pass(ctx context.Context) {
	if w.Off != nil && w.Off() {
		return
	}
	w.mu.Lock()
	if w.seen == nil {
		w.seen = map[string]fileStamp{}
	}
	w.mu.Unlock()
	now := map[string]fileStamp{}
	var changed []string
	for _, root := range w.Folders() {
		// Each folder gets its own budget, so one huge trusted folder cannot keep the
		// folders after it from being warmed.
		count, walked := 0, 0
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || ctx.Err() != nil {
				if d != nil && d.IsDir() && path != root {
					return fs.SkipDir
				}
				return nil
			}
			if walked++; walked > maxWarmWalk {
				return fs.SkipAll
			}
			name := d.Name()
			if path != root && skippedWhileWarming(name) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() || !warmable(name) {
				return nil
			}
			if count++; count > maxWarmFiles {
				return fs.SkipAll
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			stamp := fileStamp{info.Size(), info.ModTime().UnixNano()}
			if _, dup := now[path]; dup {
				return nil
			}
			now[path] = stamp
			w.mu.Lock()
			old, had := w.seen[path]
			w.mu.Unlock()
			if !had || old != stamp {
				changed = append(changed, path)
			}
			return nil
		})
	}
	w.mu.Lock()
	first := len(w.seen) == 0
	w.seen = now
	w.mu.Unlock()
	// A file whose references could not all be fetched (no network yet at login, a
	// server error), or that this pass never reached, is forgotten so the next pass
	// tries it again.
	for i, path := range changed {
		if ctx.Err() != nil || (w.Off != nil && w.Off()) {
			w.forget(changed[i:])
			return
		}
		if w.warmFile(ctx, path, !first) {
			w.mu.Lock()
			delete(w.retries, path)
			w.mu.Unlock()
			continue
		}
		w.mu.Lock()
		if w.retries == nil {
			w.retries = map[string]int{}
		}
		w.retries[path]++
		again := w.retries[path] <= maxWarmRetries
		w.mu.Unlock()
		if again {
			w.forget([]string{path})
		}
	}
}

func (w *Warmer) forget(paths []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range paths {
		delete(w.seen, p)
	}
}

// skippedWhileWarming names folders and files the warmer never walks into: hidden ones,
// package and build folders, and the system's own application data, which a trusted
// home folder would otherwise walk every pass.
func skippedWhileWarming(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "sites-versions", "tailwindcss", "Library", "AppData":
		return true
	}
	return false
}

func warmable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".htmlclay":
		return true
	}
	return false
}

// warmFile fetches the static references of the HTML file at path. A file that changed
// since the last pass revalidates every reference; a file seen for the first time this
// run fetches only references the shared index lacks or got more than a day ago, so a
// restart does not refetch everything.
func (w *Warmer) warmFile(ctx context.Context, path string, changed bool) bool {
	// Only regular files: a FIFO or a link to a device named .html would block or never
	// end.
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxWarmParse {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	data, err := io.ReadAll(io.LimitReader(f, maxWarmParse+1))
	f.Close()
	if err != nil || int64(len(data)) > maxWarmParse {
		return true
	}
	ok := true
	for _, ref := range staticReferences(data, w.Images) {
		if !changed {
			if e, ok := w.Fetch.Store.Lookup(Shared, ref); ok && time.Since(e.StoredAt) < warmFresh {
				continue
			}
		}
		fctx, cancel := context.WithTimeout(ctx, warmFetchWait)
		var status *StatusError
		if err := w.Fetch.Fetch(fctx, Shared, ref, w.Hints, "warm"); err != nil && !errors.As(err, &status) && !errors.Is(err, ErrTooBig) && !errors.Is(err, errOff) {
			ok = false
		}
		cancel()
	}
	return ok
}

// StaticReferences lists the absolute http and https addresses an HTML document loads
// from its static tags: scripts, stylesheets, icons, preloads, images, picture sources
// inside <picture>, and video posters. It leaves out relative addresses (the document's own folder),
// credentialed loads (crossorigin="use-credentials") and clayjs.com, which HTML Clay
// bundles.
func StaticReferences(doc []byte) []string { return staticReferences(doc, true) }

// staticReferences lists what StaticReferences does, leaving out images (img, picture
// sources, video posters, image preloads) unless images is true. Video and audio preloads
// are always left out: the worker never answers media from copies.
func staticReferences(doc []byte, images bool) []string {
	z := html.NewTokenizer(bytes.NewReader(doc))
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(raw, "//") {
			raw = "https:" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return
		}
		host := strings.ToLower(u.Hostname())
		if host == "clayjs.com" || strings.HasSuffix(host, ".clayjs.com") {
			return
		}
		key, err := NormalizeURL(u.String())
		if err != nil || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, key)
	}
	addSrcset := func(v string) {
		for _, u := range srcsetURLs(v) {
			add(u)
		}
	}
	inPicture := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out
		}
		if tt == html.EndTagToken {
			if name, _ := z.TagName(); string(name) == "picture" && inPicture > 0 {
				inPicture--
			}
			continue
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		if string(name) == "picture" && tt == html.StartTagToken {
			inPicture++
		}
		if !hasAttr {
			continue
		}
		attrs := map[string]string{}
		for {
			k, v, more := z.TagAttr()
			attrs[string(k)] = string(v)
			if !more {
				break
			}
		}
		if strings.EqualFold(attrs["crossorigin"], "use-credentials") {
			continue
		}
		switch string(name) {
		case "script":
			add(attrs["src"])
		case "link":
			if linkWarmed(attrs["rel"]) && preloadWarmed(attrs["rel"], attrs["as"], images) {
				add(attrs["href"])
			}
		case "img":
			if images {
				add(attrs["src"])
				addSrcset(attrs["srcset"])
			}
		case "source":
			if images && inPicture > 0 {
				add(attrs["src"])
				addSrcset(attrs["srcset"])
			}
		case "video":
			if images {
				add(attrs["poster"])
			}
		}
	}
}

// preloadWarmed says whether a preload's destination is warmed: never media, and images
// only when images are.
func preloadWarmed(rel, as string, images bool) bool {
	if !strings.Contains(" "+strings.ToLower(rel)+" ", " preload ") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "video", "audio", "track":
		return false
	case "image":
		return images
	}
	return true
}

func linkWarmed(rel string) bool {
	for _, r := range strings.Fields(strings.ToLower(rel)) {
		switch r {
		case "stylesheet", "icon", "apple-touch-icon", "preload", "modulepreload":
			return true
		}
	}
	return false
}

// srcsetURLs lists a srcset's candidate URLs as the HTML standard reads them: a URL runs
// to the next whitespace, so commas inside it (image CDNs put them in paths) stay part of
// it, and a trailing comma ends a candidate with no descriptor.
func srcsetURLs(v string) []string {
	var out []string
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }
	i := 0
	for i < len(v) {
		for i < len(v) && (isSpace(v[i]) || v[i] == ',') {
			i++
		}
		start := i
		for i < len(v) && !isSpace(v[i]) {
			i++
		}
		u := v[start:i]
		trailing := strings.HasSuffix(u, ",")
		u = strings.TrimRight(u, ",")
		if u != "" {
			out = append(out, u)
		}
		if trailing {
			continue
		}
		depth := 0
		for i < len(v) {
			c := v[i]
			if c == '(' {
				depth++
			} else if c == ')' && depth > 0 {
				depth--
			} else if c == ',' && depth == 0 {
				i++
				break
			}
			i++
		}
	}
	return out
}
