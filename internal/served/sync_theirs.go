package served

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// theirsPrefix is the route that shows the server's version of a document held
// in a sync conflict (08-sync.md 8.33, contract C7).
const theirsPrefix = "/_/sync/theirs/"

// conflictsFolder is where the engine parks the server's bytes of a conflict,
// relative to the root. Nothing outside it is ever served by this route.
const conflictsFolder = ".hyperclay/conflicts"

// serveTheirs is GET /_/sync/theirs/<rel>: the kept server copy of rel's held
// conflict, read only. The copy is a document from someone else's edit, served on
// an address where a page can save, so it runs sandboxed: no script, no form, no
// same-origin access. Anything that is not a held copy inside conflictsFolder is
// 404.
func (s *rootServer) serveTheirs(w http.ResponseWriter, r *http.Request) error {
	decoded, err := decodeOnce(r.URL.EscapedPath())
	if err != nil {
		return fileNotFoundError()
	}
	rel := strings.TrimPrefix(decoded, theirsPrefix)
	if _, err := validateSegments(rel); err != nil || s.heldCopy == nil {
		return fileNotFoundError()
	}
	copyRel, ok := s.heldCopy(s.id, rel)
	if !ok {
		return fileNotFoundError()
	}
	copyRel = path.Clean(filepath.ToSlash(copyRel))
	name, inside := strings.CutPrefix(copyRel, conflictsFolder+"/")
	// A conflict in a subfolder keeps its server copy in the same subfolder of
	// .hyperclay/conflicts/; containment below is checked on the resolved path.
	if !inside || name == "" {
		return fileNotFoundError()
	}
	if _, err := validateSegments(name); err != nil {
		return fileNotFoundError()
	}
	folderReal, err := filepath.EvalSymlinks(filepath.Join(s.baseDir, filepath.FromSlash(conflictsFolder)))
	if err != nil {
		return fileNotFoundError()
	}
	fileReal, err := filepath.EvalSymlinks(filepath.Join(s.baseDir, filepath.FromSlash(copyRel)))
	if err != nil || !isContained(filepath.ToSlash(folderReal), filepath.ToSlash(fileReal)) {
		return fileNotFoundError()
	}
	info, err := os.Stat(fileReal)
	if err != nil || !info.Mode().IsRegular() {
		return fileNotFoundError()
	}
	policy := "sandbox"
	if frame := w.Header().Get("Content-Security-Policy"); frame != "" {
		policy = frame + "; sandbox"
	}
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	sendFile(w, r, fileReal, info, mimeTypeFor(fileReal))
	return nil
}
