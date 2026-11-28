package mirror

import (
	"bytes"
	"context"
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
	warmEvery     = 5 * time.Minute
	warmFresh     = 24 * time.Hour
	maxWarmFiles  = 50000
	maxWarmWalk   = 200000
	maxWarmParse  = 10 << 20
	warmFetchWait = 2 * time.Minute
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

	mu   sync.Mutex
	seen map[string]fileStamp
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
	count, walked := 0, 0
	for _, root := range w.Folders() {
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
	for _, path := range changed {
		if ctx.Err() != nil || (w.Off != nil && w.Off()) {
			return
		}
		w.warmFile(ctx, path, !first)
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
func (w *Warmer) warmFile(ctx context.Context, path string, changed bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxWarmParse {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, ref := range StaticReferences(data) {
		if !changed {
			if e, ok := w.Fetch.Store.Lookup(Shared, ref); ok && time.Since(e.StoredAt) < warmFresh {
				continue
			}
		}
		fctx, cancel := context.WithTimeout(ctx, warmFetchWait)
		w.Fetch.Fetch(fctx, Shared, ref, w.Hints, "warm")
		cancel()
	}
}

// StaticReferences lists the absolute http and https addresses an HTML document loads
// from its static tags: scripts, stylesheets, icons, preloads, images, picture sources
// inside <picture>, and video posters. It leaves out relative addresses (the document's own folder),
// credentialed loads (crossorigin="use-credentials") and clayjs.com, which HTML Clay
// bundles.
func StaticReferences(doc []byte) []string {
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
		for _, part := range strings.Split(v, ",") {
			if f := strings.Fields(part); len(f) > 0 {
				add(f[0])
			}
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
			if linkWarmed(attrs["rel"]) {
				add(attrs["href"])
			}
		case "img":
			add(attrs["src"])
			addSrcset(attrs["srcset"])
		case "source":
			if inPicture > 0 {
				add(attrs["src"])
				addSrcset(attrs["srcset"])
			}
		case "video":
			add(attrs["poster"])
		}
	}
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
