//go:build managedtrial

package managedtrial

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/panphora/htmlclay/internal/clayruntime"
)

// rootServer answers one root: Local's static catch-all, its document lane, its
// folder listings and its error body.
type rootServer struct {
	baseDir     string
	registry    *consentRegistry
	localSource string
	runtimeDir  string
	selected    string
	releases    map[string]clayruntime.Release
	// commit is the one writer of this root's documents: every route that writes
	// goes through the same host, so two of them cannot interleave.
	commit *commitHost
	// control is this root as the control listener and the watcher see it.
	control *controlRoot
}

// htmlRoutePattern is Local's SPA-aware route: the document names an .html or
// .htmlclay file and everything after it is a client-side route.
var htmlRoutePattern = regexp.MustCompile(`^(.*?\.html(?:clay)?)(\/.*)?$`)

// svgPathPattern matches a file send must hand over as a download: an SVG can
// carry <script>, and served inline from this origin it runs with the page's own
// authority.
var svgPathPattern = regexp.MustCompile(`(?i)\.svgz?$`)

func newRootServer(root rootSpec, cfg config) (*rootServer, error) {
	runtime, err := loadRuntimeReleases(cfg.localSource)
	if err != nil {
		return nil, err
	}
	registry := newConsentRegistry(root.path, nil)
	registry.Ready()
	baseDir := registry.BaseReal()
	host := newCommitHost(baseDir, root.id, registry, newGuardHooks(nil), runtime.Releases)
	return &rootServer{
		baseDir:     baseDir,
		registry:    registry,
		localSource: cfg.localSource,
		runtimeDir:  runtime.Dir,
		selected:    runtime.Selected,
		releases:    runtime.Releases,
		commit:      host,
		control:     newControlRoot(root.id, baseDir, registry, host, cfg.events),
	}, nil
}

func (s *rootServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath, marked, ok := gate(w, r, localPort(r))
	if !ok {
		return
	}
	// Express matches its routes against the still-encoded path, so a `%2F`
	// inside a segment does not split it here either.
	rawPath := r.URL.EscapedPath()
	favicon, isFavicon := faviconFiles[strings.TrimSuffix(rawPath, "/")]
	_, isTemplate := templateFilename(rawPath)
	getOrHead := r.Method == http.MethodGet || r.Method == http.MethodHead
	versionsTail, hasVersionsTail := routeTail(urlPath, "/versions/")
	versionTail, hasVersionTail := routeTail(urlPath, "/version/")
	restoreTail, hasRestoreTail := routeTail(urlPath, "/restore/")
	dataLossTail, hasDataLossTail := routeTail(urlPath, "/data-loss/")
	// Express matches its routes non-strictly, so `/upload` answers `/upload/` too.
	systemPath := strings.TrimSuffix(urlPath, "/")
	var err error
	switch {
	case marked && getOrHead && urlPath == "/data-loss":
		err = s.serveDataLossGet(w, r)
	case marked && r.Method == http.MethodPost && urlPath == "/data-loss":
		err = s.serveDataLossPost(w, r, "")
	case marked && r.Method == http.MethodPost && hasDataLossTail:
		err = s.serveDataLossPost(w, r, dataLossTail)
	case marked && getOrHead && hasVersionsTail:
		err = s.serveVersions(w, r, versionsTail)
	case marked && getOrHead && hasVersionTail:
		err = s.serveVersion(w, r, versionTail)
	case marked && r.Method == http.MethodPost && hasRestoreTail:
		err = s.serveRestore(w, r, restoreTail)
	case marked && r.Method == http.MethodPost && systemPath == "/upload":
		err = s.serveUpload(w, r)
	case marked && r.Method == http.MethodPost && documentActionSegments[systemPath]:
		err = s.serveDocumentAction(w, r)
	case marked && getOrHead && strings.HasPrefix(rawPath, "/_/uploads/") && len(rawPath) > len("/_/uploads/"):
		err = s.serveUploadFile(w, r)
	case marked && getOrHead && urlPath == "/meta":
		err = s.serveMeta(w, r)
	case marked && getOrHead && urlPath == "/page-state":
		err = s.servePageState(w, r)
	case r.Method == http.MethodPost && urlPath == "/save" && bareAllowed(urlPath, marked):
		err = s.serveSave(w, r)
	case getOrHead && isFavicon:
		err = s.serveFavicon(w, r, favicon)
	case getOrHead && isTemplate:
		err = s.serveTemplate(w, r, rawPath)
	case marked && getOrHead && strings.HasPrefix(rawPath, "/_/clayjs/"):
		err = s.serveRuntimeAsset(w, r)
	default:
		err = s.serve(w, r, marked)
	}
	if err != nil {
		serveError(w, r, err)
	}
}

// serve is Local's static catch-all: the folder listing at `/` and below, the
// SPA-aware document route, and every other file through send.
func (s *rootServer) serve(w http.ResponseWriter, r *http.Request, marked bool) error {
	// A request that arrived under the reserved `/_/` marker and reached the
	// catch-all was not consumed by any system route, so its stripped path must
	// not be served as a document.
	if marked {
		sendText(w, r, http.StatusNotFound, "File not found")
		return nil
	}

	urlPath, err := decodeOnce(r.URL.EscapedPath())
	if err != nil {
		return err
	}
	if urlPath == "/" {
		return s.serveDirListing(w, r, s.baseDir, s.baseDir)
	}
	if urlPath == "" || urlPath[0] != '/' {
		return invalidPathError()
	}

	requestedPath := urlPath[1:]
	match := htmlRoutePattern.FindStringSubmatch(requestedPath)
	target := requestedPath
	if match != nil {
		target = match[1]
	}
	if _, err := validateSegments(target); err != nil {
		return err
	}
	realPath, err := resolveReadPath(s.registry, target)
	if err != nil {
		return err
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		// An .html segment names a document, never a folder.
		if match != nil {
			return fileNotFoundError()
		}
		return s.serveDirListing(w, r, realPath, s.baseDir)
	}

	// A page type in an uploads folder is a download on every branch, so the
	// decision is made before the page branch is reached rather than beside the
	// header below. Uploads store directly inside `assets-<stem>/`, so only the
	// immediate parent counts, compared without case.
	uploaded := refusedUploadPath(realPath) && inUploadsFolder(realPath)
	if match != nil && !uploaded {
		return s.serveHtml(w, r, realPath, match[1])
	}

	// nosniff on everything: the browser takes the type send sets from the
	// extension and never second-guesses it from the bytes.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if svgPathPattern.MatchString(realPath) || uploaded {
		w.Header().Set("Content-Disposition", "attachment")
	}
	// send types an extensionless file by its bare basename, as if `html` or
	// `svg` were an extension: a file with no extension is bytes, never a page.
	contentType := "application/octet-stream"
	if filepath.Ext(realPath) != "" {
		contentType = mimeTypeFor(realPath)
	}
	sendFile(w, r, realPath, info, contentType)
	return nil
}

// serveDirListing renders one folder's listing.
func (s *rootServer) serveDirListing(w http.ResponseWriter, r *http.Request, dirPath, baseDir string) error {
	entries, err := readListingEntries(dirPath)
	if err != nil {
		sendText(w, r, http.StatusInternalServerError, "Error reading directory")
		return nil
	}
	displayPath := ""
	if rel, err := filepath.Rel(baseDir, dirPath); err == nil && rel != "." {
		displayPath = filepath.ToSlash(rel)
	}
	sendText(w, r, http.StatusOK, string(renderDirListing(displayPath, entries)))
	return nil
}

// readListingEntries reads a folder the way fs.readdir answers: libuv sorts the
// entries by name with strcmp, so the listing is in name order too.
func readListingEntries(dirPath string) ([]listingEntry, error) {
	raw, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	entries := make([]listingEntry, 0, len(raw))
	for _, entry := range raw {
		kind := entry.Type()
		isDir := entry.IsDir()
		if !isDir && !kind.IsRegular() {
			continue
		}
		entries = append(entries, listingEntry{Name: entry.Name(), IsDir: isDir})
	}
	return entries, nil
}

// serveError is Local's error handler: the status from the error, ENOENT and
// ENOTDIR as 404, and one body per status class.
func serveError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	var pathErr *PathError
	switch {
	case errors.As(err, &pathErr) && pathErr.Status != 0:
		status = pathErr.Status
	case errors.Is(err, fs.ErrNotExist), isNotDirError(err):
		status = http.StatusNotFound
	}
	body := "Internal server error"
	switch status {
	case http.StatusNotFound:
		body = "File not found"
	case http.StatusForbidden:
		body = "Access denied"
	case http.StatusBadRequest:
		body = "Bad request"
	}
	sendText(w, r, status, body)
}

// refusedUploadPath is Local's refusedUpload: the types a document folder may
// not hold, because a document stored beside a document and served from the
// same origin is stored XSS. SVG is deliberately accepted and served inert.
func refusedUploadPath(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" || ext == ".svg" || ext == ".svgz" {
		return false
	}
	if uploadRefusedExtensions[ext] {
		return true
	}
	return documentTypePattern.MatchString(mimeLookup(ext[1:]))
}

var uploadRefusedExtensions = map[string]bool{
	".html": true, ".htm": true, ".shtml": true, ".xhtml": true, ".xht": true, ".htmlclay": true,
	".xml": true, ".xsl": true, ".xslt": true, ".mathml": true, ".mml": true, ".rss": true,
	".atom": true, ".rdf": true,
	".js": true, ".mjs": true, ".cjs": true, ".ecma": true,
}

var documentTypePattern = regexp.MustCompile(`^(?:text/html|application/xhtml\+xml|text/xml|application/xml|[a-z0-9.-]+/[a-z0-9.+-]*\+xml|text/mathml|text/javascript|application/javascript|application/x-javascript|application/ecmascript|text/ecmascript)$`)

// inUploadsFolder is Local's inUploadsFolder: a file directly inside a
// document's uploads folder, where every upload is stored.
func inUploadsFolder(realPath string) bool {
	return strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(realPath))), "assets-")
}

// mimeExtensionFor is the extension `send` looks up: everything after the last
// `.`, `/` or `\` of the whole path, lowercased.
func mimeExtensionFor(path string) string {
	if at := strings.LastIndexAny(path, `./\`); at >= 0 {
		return strings.ToLower(path[at+1:])
	}
	return strings.ToLower(path)
}

func mimeLookup(path string) string {
	if value, ok := mimeTypes[mimeExtensionFor(path)]; ok {
		return value
	}
	return mimeDefaultType
}

// mimeCharset is send's charset rule: text types and JavaScript or JSON are
// UTF-8, and any other type is served without a charset.
func mimeCharset(mediaType string) string {
	if strings.HasPrefix(mediaType, "text/") ||
		strings.HasPrefix(mediaType, "application/javascript") ||
		strings.HasPrefix(mediaType, "application/json") {
		return "UTF-8"
	}
	return ""
}

// mimeTypeFor is what send sets for a file it serves: the type its extension
// names, with the charset send appends to that type.
func mimeTypeFor(path string) string {
	mediaType := mimeLookup(path)
	if charset := mimeCharset(mediaType); charset != "" {
		return mediaType + "; charset=" + charset
	}
	return mediaType
}
