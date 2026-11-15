package served

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/panphora/htmlclay/internal/wire"
)

// wireDocuments is a served root as the wire sees it. A page is admitted by
// the same rule as the root's save route (same-origin, the gate); a local
// process is admitted only when it carries no browser header at all.
type wireDocuments struct {
	root        *rootServer
	internalDir string
}

func (d *wireDocuments) Caller(r *http.Request) (isBrowser, ok bool) {
	site := r.Header.Get("Sec-Fetch-Site")
	origin := r.Header.Get("Origin")
	if site == "same-origin" {
		if origin == "" || d.root.policy(r).ownOrigin(r, origin) {
			return true, true
		}
		return false, false
	}
	if origin == "" && site == "" && r.Header.Get("Sec-Fetch-Mode") == "" && r.Header.Get("Sec-Fetch-Dest") == "" && r.Header.Get("Cookie") == "" {
		return false, true
	}
	return false, false
}

func (d *wireDocuments) Target(r *http.Request, isBrowser bool, supplied string) (wire.Document, bool) {
	if isBrowser {
		name := resolveResourceFromHref(wire.PageURL(r))
		abs, err := resolveWriteTarget(d.root.registry, name)
		if err != nil {
			return wire.Document{}, false
		}
		return d.At(abs)
	}
	return d.At(supplied)
}

func (d *wireDocuments) At(p string) (wire.Document, bool) {
	if !filepath.IsAbs(p) || !documentTargetPattern.MatchString(p) {
		return wire.Document{}, false
	}
	d.root.registry.Ready()
	// The registry keeps its base with forward slashes on every system, so a
	// path is put in that form before it is compared; the wire itself keys a
	// document on the path as the system writes it.
	base := d.root.registry.baseReal
	if !isContained(base, filepath.ToSlash(p)) || d.Internal(p) {
		return wire.Document{}, false
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil || !isContained(base, filepath.ToSlash(real)) || d.Internal(real) {
		return wire.Document{}, false
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return wire.Document{}, false
	}
	rel, _ := filepath.Rel(base, real)
	return wire.Document{Path: real, Name: filepath.ToSlash(rel)}, true
}

func (d *wireDocuments) MayUsePrograms(doc wire.Document, isBrowser bool, presented string) bool {
	return isBrowser
}

func (d *wireDocuments) OpenHistory(doc wire.Document) error {
	// A save keys this lock on the path with forward slashes.
	unlock := fileLocks.lock(filepath.ToSlash(doc.Path))
	defer unlock()
	CreateBackupIfExists(doc.Path, backupSiteName(doc.Name), d.root.baseDir, nil)
	return nil
}

func (d *wireDocuments) Lease(doc wire.Document)   {}
func (d *wireDocuments) Unlease(doc wire.Document) {}
func (d *wireDocuments) Poke(doc wire.Document)    {}

func (d *wireDocuments) Internal(absPath string) bool {
	if d.internalDir != "" && (absPath == d.internalDir || strings.HasPrefix(absPath, d.internalDir+string(filepath.Separator))) {
		return true
	}
	d.root.registry.Ready()
	rel, err := filepath.Rel(d.root.registry.baseReal, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(segment, ".") && segment != "." {
			return true
		}
	}
	return false
}
