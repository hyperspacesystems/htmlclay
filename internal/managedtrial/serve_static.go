//go:build managedtrial

package managedtrial

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/panphora/htmlclay/internal/clayruntime"
)

// faviconFiles are the three icons Local answers a browser's implicit favicon
// request with, each one a file in the checkout's own assets folder.
var faviconFiles = map[string]string{
	"/favicon.ico": "favicon.ico",
	"/favicon.svg": "favicon.svg",
	"/favicon.png": "favicon.png",
}

// serveFavicon sends one of those icons the way res.sendFile does: the type its
// extension names, the file's own stamp, and the same ranges negotiation.
func (s *rootServer) serveFavicon(w http.ResponseWriter, r *http.Request, name string) error {
	path := filepath.Join(s.localSource, "assets", name)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	sendFile(w, r, path, info, mimeTypeFor(name))
	return nil
}

// templateFilename is the `:filename` Express reads out of a path: the route
// matches one still-encoded segment, and only that segment is decoded.
func templateFilename(rawPath string) (string, bool) {
	trimmed := strings.TrimSuffix(rawPath, "/")
	if !strings.HasPrefix(trimmed, "/__templates/") {
		return "", false
	}
	segment := trimmed[len("/__templates/"):]
	if segment == "" || strings.Contains(segment, "/") {
		return "", false
	}
	return segment, true
}

// serveTemplate serves one of the checkout's own template files: a `.css` name
// only, read from the basename so no path reaches outside the folder, and
// `Not found` for anything else.
func (s *rootServer) serveTemplate(w http.ResponseWriter, r *http.Request, rawPath string) error {
	segment, ok := templateFilename(rawPath)
	if !ok {
		return fileNotFoundError()
	}
	filename, err := decodeOnce(segment)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(filename, ".css") {
		sendText(w, r, http.StatusNotFound, "Not found")
		return nil
	}
	name := filepath.Base(filename)
	path := filepath.Join(s.localSource, "src", "main", "templates", name)
	css, err := os.ReadFile(path)
	if err != nil {
		sendText(w, r, http.StatusNotFound, "Not found")
		return nil
	}
	w.Header().Set("Content-Type", withCharset("text/css", "utf-8"))
	sendText(w, r, http.StatusOK, string(css))
	return nil
}

// runtimeAsset is Local's runtimeAssets.assetFor: the release index's entry for
// one public path, and the file it is served from.
func (s *rootServer) runtimeAsset(version, publicPath string) (clayruntime.File, string, bool) {
	release, ok := s.releases[version]
	if !ok {
		return clayruntime.File{}, "", false
	}
	file, ok := release.Files[publicPath]
	if !ok {
		return clayruntime.File{}, "", false
	}
	return file, filepath.Join(s.runtimeDir, version, filepath.FromSlash(publicPath)), true
}

// serveRuntimeAsset is Local's `/_/clayjs/<version>/<path>`: the bundled
// release's own bytes, so a document served to a plain browser gets its runtime
// from this host rather than from clayjs.com. Public bytes, read-only, and only
// under the marker: an unmarked `/clayjs/...` falls through to the catch-all.
func (s *rootServer) serveRuntimeAsset(w http.ResponseWriter, r *http.Request) error {
	documentURL := "http://" + r.Host + "/"
	parsed := clayruntime.ParseRuntimeURL(r.RequestURI, clayruntime.Options{DocumentURL: documentURL})
	var file clayruntime.File
	var path string
	found := false
	if parsed != nil && parsed.Kind == "local" {
		file, path, found = s.runtimeAsset(parsed.Version, parsed.Path)
	}
	if !found {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		sendText(w, r, http.StatusNotFound, "Not found")
		return nil
	}

	etag := `"` + file.SHA256 + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && strings.Contains(ifNoneMatch, etag) {
		// Local's 304 keeps the type, and net/http strips Content-Type from
		// every 304 it writes. A name it does not canonicalize survives that
		// strip, and a header name is case-insensitive on the wire.
		h["content-type"] = []string{file.Type}
		poweredBy(w)
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	// The index's own type, with nothing appended to it.
	h.Set("Content-Type", file.Type)
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	poweredBy(w)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}
	_, _ = w.Write(body)
	return nil
}
