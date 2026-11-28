package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/egress"
)

const (
	fetchTimeout  = 15 * time.Second
	maxRedirects  = 5
	maxCrawlDepth = 3
	fetchParallel = 4
)

// Fetcher is HTML Clay's own GET for a copy: files a page loaded before its worker took
// control, opaque files the worker could not read, and the files a stylesheet loads. It
// reaches the public internet only, keeps no cookies and sends no credentials.
type Fetcher struct {
	Store  *Store
	client *http.Client
	slots  chan struct{}
}

// NewFetcher returns a fetcher that stores into store. A nil dial is
// egress.DialPublic; tests pass one that also reaches their stand-in server.
func NewFetcher(store *Store, dial func(ctx context.Context, network, addr string) (net.Conn, error)) *Fetcher {
	if dial == nil {
		dial = egress.DialPublic
	}
	return &Fetcher{
		Store: store,
		slots: make(chan struct{}, fetchParallel),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:                  nil,
				DialContext:            dial,
				ForceAttemptHTTP2:      true,
				TLSHandshakeTimeout:    10 * time.Second,
				ResponseHeaderTimeout:  fetchTimeout,
				MaxResponseHeaderBytes: 64 << 10,
			},
			Timeout: fetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("more than %d redirects", maxRedirects)
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return errors.New("a redirect to an address that is not http or https")
				}
				return nil
			},
		},
	}
}

// Hints are the request headers a page's own request carried that change what a
// server sends back (Google Fonts answers per browser).
type Hints struct {
	UserAgent string
	Accept    string
}

// Fetch gets rawURL and stores it for origin, revalidating origin's existing copy with
// its validators. A stylesheet's references are followed too.
func (f *Fetcher) Fetch(ctx context.Context, origin, rawURL string, h Hints, via string) error {
	return f.fetch(ctx, origin, rawURL, h, via, 0, map[string]bool{})
}

func (f *Fetcher) fetch(ctx context.Context, origin, rawURL string, h Hints, via string, depth int, seen map[string]bool) error {
	key, err := NormalizeURL(rawURL)
	if err != nil {
		return err
	}
	if seen[key] {
		return nil
	}
	seen[key] = true
	select {
	case f.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	body, meta, fresh, err := f.get(ctx, origin, key, h)
	<-f.slots
	if err != nil {
		return err
	}
	if fresh {
		f.Store.Touch(origin, key)
		return nil
	}
	meta.Via = via
	if err := f.Store.PutEntry(origin, key, meta, body); err != nil {
		return err
	}
	if isCSS(meta.Type) && depth < maxCrawlDepth {
		f.crawl(ctx, origin, key, body, h, via, depth+1, seen, true)
	}
	return nil
}

// get performs one GET. fresh is true when the server answered 304 to origin's stored
// validators.
func (f *Fetcher) get(ctx context.Context, origin, key string, h Hints) (body []byte, meta Entry, fresh bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, Entry{}, false, err
	}
	if h.UserAgent != "" {
		req.Header.Set("User-Agent", h.UserAgent)
	}
	if h.Accept != "" {
		req.Header.Set("Accept", h.Accept)
	}
	if old, ok := f.Store.Lookup(origin, key); ok {
		if old.ETag != "" {
			req.Header.Set("If-None-Match", old.ETag)
		}
		if old.LastModified != "" {
			req.Header.Set("If-Modified-Since", old.LastModified)
		}
	}
	res, err := f.client.Do(req)
	if err != nil {
		return nil, Entry{}, false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return nil, Entry{}, true, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, Entry{}, false, fmt.Errorf("mirror: %s answered %d", key, res.StatusCode)
	}
	limit := f.Store.Limits().PerFile
	reader := io.Reader(res.Body)
	if limit > 0 {
		if res.ContentLength > limit {
			return nil, Entry{}, false, ErrTooBig
		}
		reader = io.LimitReader(res.Body, limit+1)
	}
	body, err = io.ReadAll(reader)
	if err != nil {
		return nil, Entry{}, false, err
	}
	if limit > 0 && int64(len(body)) > limit {
		return nil, Entry{}, false, ErrTooBig
	}
	return body, Entry{
		Type:         res.Header.Get("Content-Type"),
		ETag:         res.Header.Get("ETag"),
		LastModified: res.Header.Get("Last-Modified"),
	}, false, nil
}

// Crawl fetches the files a stylesheet references, resolved against the stylesheet's
// own URL. With refresh, files already held are revalidated; without, they are skipped.
func (f *Fetcher) Crawl(ctx context.Context, origin, cssURL string, css []byte, h Hints, via string, refresh bool) {
	key, err := NormalizeURL(cssURL)
	if err != nil {
		return
	}
	f.crawl(ctx, origin, key, css, h, via, 1, map[string]bool{key: true}, refresh)
}

func (f *Fetcher) crawl(ctx context.Context, origin, base string, css []byte, h Hints, via string, depth int, seen map[string]bool, refresh bool) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return
	}
	for _, ref := range CSSReferences(string(css)) {
		u, err := baseURL.Parse(ref)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		abs := u.String()
		if !refresh {
			if _, ok := f.Store.Lookup(origin, abs); ok {
				continue
			}
		}
		f.fetch(ctx, origin, abs, h, via, depth, seen)
	}
}

var (
	cssImport  = regexp.MustCompile(`@import\s+(?:url\(\s*)?["']?([^"')\s;]+)`)
	cssURL     = regexp.MustCompile(`url\(\s*["']?([^"')]+?)["']?\s*\)`)
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// CSSReferences lists the @import and url() references in a stylesheet, in order,
// without duplicates and without data: URLs.
func CSSReferences(css string) []string {
	css = cssComment.ReplaceAllString(css, "")
	var out []string
	seen := map[string]bool{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] || strings.HasPrefix(strings.ToLower(ref), "data:") || strings.HasPrefix(ref, "#") {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}
	for _, m := range cssImport.FindAllStringSubmatch(css, -1) {
		add(m[1])
	}
	for _, m := range cssURL.FindAllStringSubmatch(css, -1) {
		add(m[1])
	}
	return out
}

func isCSS(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/css")
}
