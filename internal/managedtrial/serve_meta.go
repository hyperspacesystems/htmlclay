//go:build managedtrial

package managedtrial

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/panphora/htmlclay/internal/specwire"
)

const (
	saveMaxBytes   = 20 * 1024 * 1024
	uploadMaxBytes = 25 * 1024 * 1024
	uploadAccess   = "local"
)

// trialMetaExtensions is Local's recorded extension list minus AD1, the
// difference the trial is allowed to announce: no `format`, no
// `scoped-stylesheet`, no `presence`, and no name starting with `live-`. It is
// written out rather than computed so later steps implement what is announced
// here, and the golden test pins the result.
var trialMetaExtensions = []string{
	"conditional",
	"data-guard-v1",
	"data-read",
	"data-write",
	"page",
	"page-awareness-v1",
	"page-format-v1",
	"page-recovery-v1",
	"people",
	"receipts",
	"record-awareness-v1",
	"sync",
	"sync-worker",
	"upload",
	"wire",
}

// documentTargetPattern is Local's validateAndResolvePath extension check: a
// document block is only ever answered for a page file.
var documentTargetPattern = regexp.MustCompile(`\.(html|htmlclay)$`)

// htmlResourcePattern is Local's resolveResourceFromHref prefix match: everything
// up to the page extension is the document, and the rest is a client route.
var htmlResourcePattern = regexp.MustCompile(`^(.*?\.html(?:clay)?)`)

// documentURLHeader is the request's target document, `Document-URL` first and
// the pre-spec `Page-URL` when it is absent.
func documentURLHeader(r *http.Request) string {
	if value := r.Header.Get("Document-URL"); value != "" {
		return value
	}
	return r.Header.Get("Page-URL")
}

// sameOriginDiscovery is Local's rule for naming a person: only a page on this
// root's own origin gets one.
func sameOriginDiscovery(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}

// resolveResourceFromHref is Local's Document-URL to logical name: the URL's
// pathname, decoded once, with the document part of an SPA route kept.
func resolveResourceFromHref(href string) string {
	pathname := href
	if parsed, err := url.Parse(href); err == nil && parsed.IsAbs() {
		pathname = parsed.EscapedPath()
		if pathname == "" {
			pathname = "/"
		}
		pathname = normalizeURLPath(pathname)
	}
	if decoded, err := url.PathUnescape(pathname); err == nil {
		pathname = decoded
	}
	if pathname == "/" {
		return "index.html"
	}
	pathname = strings.TrimPrefix(pathname, "/")
	if match := htmlResourcePattern.FindStringSubmatch(pathname); match != nil {
		return path.Clean(match[1])
	}
	return path.Clean(pathname)
}

// normalizeURLPath removes dot segments the way a WHATWG URL parser does,
// keeping empty segments so a doubled slash survives.
func normalizeURLPath(value string) string {
	leading := strings.HasPrefix(value, "/")
	segments := strings.Split(value, "/")
	stack := make([]string, 0, len(segments))
	for _, segment := range segments {
		switch segment {
		case ".":
			continue
		case "..":
			for len(stack) > 0 {
				last := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if last != "" {
					break
				}
			}
		default:
			stack = append(stack, segment)
		}
	}
	out := strings.Join(stack, "/")
	if leading && !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	if leading && out == "" {
		out = "/"
	}
	return out
}

// resolveWriteTarget is Local's name check plus canonical write resolution. A
// name that is not a page file is refused, so /_/meta and /_/page-state answer
// for pages only.
func resolveWriteTarget(registry *consentRegistry, name string) (string, error) {
	if !documentTargetPattern.MatchString(name) {
		return "", invalidPathError()
	}
	if _, err := validateSegments(name); err != nil {
		return "", err
	}
	return resolveWritePath(registry, name)
}

// serveMeta is Local's GET /_/meta: the host's capabilities, and the facts and
// stamp of the document the request names, withheld by omission on every
// failure.
func (s *rootServer) serveMeta(w http.ResponseWriter, r *http.Request) error {
	body := orderedObject{
		{Key: "spec", Value: 1},
		{Key: "extensions", Value: trialMetaExtensions},
	}
	if href := documentURLHeader(r); href != "" {
		document := s.metaDocument(href)
		if document != nil {
			if sameOriginDiscovery(r) {
				*document = append(*document, kv{Key: "people", Value: orderedObject{{Key: "me", Value: nil}}})
				w.Header().Set("Cache-Control", "private, no-store")
			}
			body = append(body, kv{Key: "document", Value: *document})
		}
	}
	encoded, err := jsonStringify(body)
	if err != nil {
		return err
	}
	sendJSON(w, r, http.StatusOK, encoded)
	return nil
}

// metaDocument builds the document block for one Document-URL, or nothing when
// the document is not there or cannot be read: every failure is the same
// omission, never a different answer.
func (s *rootServer) metaDocument(href string) *orderedObject {
	name := resolveResourceFromHref(href)
	filePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		return nil
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	stored, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	logical, err := logicalBytes(s.baseDir, name, stored)
	if err != nil {
		return nil
	}
	document := orderedObject{
		{Key: "writable", Value: true},
		{Key: "maxBytes", Value: saveMaxBytes},
		{Key: "versions", Value: true},
		{Key: "upload", Value: orderedObject{
			{Key: "allowed", Value: true},
			{Key: "maxBytes", Value: uploadMaxBytes},
			{Key: "access", Value: uploadAccess},
		}},
		{Key: "etag", Value: specwire.Etag(logical)},
		{Key: "pageState", Value: orderedObject{{Key: "href", Value: "/_/page-state"}}},
	}
	if pageExtension.MatchString(name) {
		document = append(document, kv{Key: "dataGuard", Value: orderedObject{{Key: "href", Value: "/_/data-loss"}}})
	}
	return &document
}
