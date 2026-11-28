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

// fetchIdle is how long a download may go without a byte before it is given up.
var fetchIdle = 30 * time.Second

const (
	fetchTimeout  = 15 * time.Second
	maxCrawlCSS   = 5 << 20
	maxRedirects  = 5
	maxCrawlDepth = 3
	fetchParallel = 4
)

// Fetcher is HTML Clay's own GET for a copy: files a page loaded before its worker took
// control, opaque files the worker could not read, and the files a stylesheet loads. It
// reaches the public internet only, keeps no cookies and sends no credentials.
type Fetcher struct {
	Store *Store
	// Off reports whether offline copies are switched off; nothing is stored while it
	// says so, even by a fetch already running. Nil reads as on.
	Off    func() bool
	client *http.Client
	slots  chan struct{}
}

var errOff = errors.New("mirror: offline copies are off")

// StatusError is a server's answer other than 200 or a usable 304: the network worked,
// so trying again soon would get the same answer.
type StatusError struct {
	URL  string
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("mirror: %s answered %d", e.URL, e.Code) }

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
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("more than %d redirects", maxRedirects)
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return errors.New("a redirect to an address that is not http or https")
				}
				if req.URL.User != nil || req.URL.Hostname() == "" {
					return errors.New("a redirect to an address with a login in it")
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
	meta, fresh, err := f.getAndStore(ctx, origin, key, h, via)
	<-f.slots
	if err != nil {
		return err
	}
	if fresh {
		f.Store.Touch(origin, key)
	}
	if isCSS(meta.Type) && depth < maxCrawlDepth {
		base := key
		if meta.Base != "" {
			base = meta.Base
		}
		if css, ok := f.Store.small(origin, key, maxCrawlCSS); ok {
			f.crawl(ctx, origin, base, css, h, via, depth+1, seen, true)
		}
	}
	return nil
}

func (f *Fetcher) off() bool { return f.Off != nil && f.Off() }

// idleReader cancels its request when no bytes arrive for fetchIdle.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
}

func (b *idleReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if n > 0 {
		b.timer.Reset(fetchIdle)
	}
	return n, err
}

// getAndStore performs one GET and streams a 200 answer into the store. fresh is true
// when the server answered 304 to origin's stored validators; meta is then the stored
// entry, so a stylesheet that did not change is still crawled for references that
// failed before.
func (f *Fetcher) getAndStore(ctx context.Context, origin, key string, h Hints, via string) (meta Entry, fresh bool, err error) {
	if f.off() {
		return Entry{}, false, errOff
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(fetchIdle, cancel)
	defer idle.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, key, nil)
	if err != nil {
		return Entry{}, false, err
	}
	if h.UserAgent != "" {
		req.Header.Set("User-Agent", h.UserAgent)
	}
	if h.Accept != "" {
		req.Header.Set("Accept", h.Accept)
	}
	old, had := f.Store.Lookup(origin, key)
	if had {
		if old.ETag != "" {
			req.Header.Set("If-None-Match", old.ETag)
		}
		if old.LastModified != "" {
			req.Header.Set("If-Modified-Since", old.LastModified)
		}
	}
	res, err := f.client.Do(req)
	if err != nil {
		return Entry{}, false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified && had {
		return old, true, nil
	}
	if res.StatusCode != http.StatusOK {
		return Entry{}, false, &StatusError{URL: key, Code: res.StatusCode}
	}
	if limit := f.Store.Limits().PerFile; limit > 0 && res.ContentLength > limit {
		return Entry{}, false, ErrTooBig
	}
	meta = Entry{
		Type:         res.Header.Get("Content-Type"),
		ETag:         res.Header.Get("ETag"),
		LastModified: res.Header.Get("Last-Modified"),
		Via:          via,
	}
	if !untyped(meta.Type) && !f.Store.Policy().AllowsType(meta.Type) {
		return Entry{}, false, ErrNotKept
	}
	// A redirected stylesheet's relative references resolve against where it landed.
	if final := res.Request.URL.String(); final != key {
		meta.Base = final
	}
	if f.off() {
		return Entry{}, false, errOff
	}
	if err := f.Store.PutStream(origin, key, meta, &idleReader{r: res.Body, timer: idle}); err != nil {
		return Entry{}, false, err
	}
	return meta, false, nil
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
	cssImport  = regexp.MustCompile(`(?i)@import\s*(?:url\(\s*(?:"([^"]+)"|'([^']+)'|([^"')\s]+))\s*\)|"([^"]+)"|'([^']+)')`)
	cssURL     = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]+)"|'([^']+)'|([^"')\s]+))\s*\)`)
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
	for _, re := range []*regexp.Regexp{cssImport, cssURL} {
		for _, m := range re.FindAllStringSubmatch(css, -1) {
			for _, g := range m[1:] {
				if g != "" {
					add(g)
					break
				}
			}
		}
	}
	return out
}

func isCSS(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/css")
}
