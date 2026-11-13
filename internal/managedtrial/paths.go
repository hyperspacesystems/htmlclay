//go:build managedtrial

package managedtrial

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"
)

// Bounds for the open-time consent walk, so a huge or deeply nested folder can
// never turn "open a folder" into an unbounded filesystem crawl.
const (
	maxConsentEntries = 5000
	maxConsentDepth   = 8
)

// reservedRootSegments are internal directories that are never addressable
// over HTTP.
var reservedRootSegments = map[string]bool{"sites-versions": true}

// PathError is the answer a request gets for a path the resolver refuses; the
// status and message reach the response body.
type PathError struct {
	Status  int
	Message string
}

func (e *PathError) Error() string {
	return e.Message
}

func invalidPathError() error {
	return &PathError{Status: 400, Message: "Invalid path"}
}

func malformedURLError() error {
	return &PathError{Status: 400, Message: "Malformed URL encoding"}
}

func accessDeniedError() error {
	return &PathError{Status: 403, Message: "Access denied"}
}

func fileNotFoundError() error {
	return &PathError{Status: 404, Message: "File not found"}
}

// toUnix mirrors upath's conversion: backslashes become separators and a run of
// slashes collapses to one, except a leading run, which is left alone.
func toUnix(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	var b strings.Builder
	b.Grow(len(p))
	for i := 0; i < len(p); i++ {
		if p[i] == '/' && i > 0 && p[i-1] == '/' {
			continue
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

// upathJoin is upath.join: forward slashes only, so containment is a plain
// prefix test.
func upathJoin(elements ...string) string {
	converted := make([]string, len(elements))
	for i, element := range elements {
		converted[i] = toUnix(element)
	}
	joined := filepath.Join(converted...)
	if joined == "" {
		return "."
	}
	return toUnix(joined)
}

// upathResolve is upath.resolve: the absolute path with forward slashes. On
// Windows it keeps the drive letter, as Node's path.win32.resolve does.
func upathResolve(p string) string {
	if p == "" {
		p = "."
	}
	native := filepath.FromSlash(p)
	if abs, err := filepath.Abs(native); err == nil {
		return toUnix(abs)
	}
	return toUnix(filepath.Clean(native))
}

func upathDir(p string) string {
	return toUnix(filepath.Dir(toUnix(p)))
}

func upathBase(p string) string {
	return toUnix(filepath.Base(toUnix(p)))
}

// upathIsAbsolute is upath.isAbsolute: a drive path or a rooted path.
func upathIsAbsolute(p string) bool {
	return filepath.IsAbs(filepath.FromSlash(p)) || strings.HasPrefix(toUnix(p), "/")
}

// isContained answers whether target is baseReal itself or sits inside it.
func isContained(baseReal, target string) bool {
	if target == baseReal {
		return true
	}
	prefix := baseReal
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(target, prefix)
}

// decodeOnce decodes exactly once, with decodeURIComponent's semantics: a
// contiguous run of percent escapes must be valid UTF-8, and a malformed
// sequence is a client error.
func decodeOnce(urlPath string) (string, error) {
	if !strings.Contains(urlPath, "%") {
		return urlPath, nil
	}
	var out strings.Builder
	out.Grow(len(urlPath))
	run := make([]byte, 0, 4)
	flush := func() bool {
		if len(run) == 0 {
			return true
		}
		valid := utf8.Valid(run)
		if valid {
			out.Write(run)
		}
		run = run[:0]
		return valid
	}
	for i := 0; i < len(urlPath); {
		if urlPath[i] != '%' {
			if !flush() {
				return "", malformedURLError()
			}
			out.WriteByte(urlPath[i])
			i++
			continue
		}
		if i+2 >= len(urlPath) {
			return "", malformedURLError()
		}
		high, ok := unhex(urlPath[i+1])
		if !ok {
			return "", malformedURLError()
		}
		low, ok := unhex(urlPath[i+2])
		if !ok {
			return "", malformedURLError()
		}
		run = append(run, high<<4|low)
		i += 3
	}
	if !flush() {
		return "", malformedURLError()
	}
	return out.String(), nil
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// validateSegments rejects traversal and NUL outright and hides dotfiles and
// reserved internals behind a 404 so their existence never leaks.
func validateSegments(relPath string) ([]string, error) {
	if relPath == "" {
		return nil, invalidPathError()
	}
	if strings.ContainsRune(relPath, 0) {
		return nil, invalidPathError()
	}
	if strings.Contains(relPath, `\`) {
		return nil, invalidPathError()
	}
	if upathIsAbsolute(relPath) {
		return nil, invalidPathError()
	}

	segments := make([]string, 0, 4)
	for _, segment := range strings.Split(relPath, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		return nil, invalidPathError()
	}
	for _, segment := range segments {
		if segment == "." || segment == ".." {
			return nil, invalidPathError()
		}
		if strings.HasPrefix(segment, ".") {
			return nil, fileNotFoundError()
		}
		if jsStringLength(segment) > 255 {
			return nil, invalidPathError()
		}
	}
	if reservedRootSegments[segments[0]] {
		return nil, fileNotFoundError()
	}
	return segments, nil
}

// jsStringLength counts UTF-16 code units, the unit JavaScript's String.length
// uses.
func jsStringLength(s string) int {
	length := 0
	for _, r := range s {
		if r > 0xFFFF {
			length += 2
		} else {
			length++
		}
	}
	return length
}

// realpathNearestParent walks up until realpath succeeds, then re-appends the
// not-yet-existing tail, so a file that is about to be created still resolves
// through the symlinks it will live under.
func realpathNearestParent(dir string) (string, error) {
	var missing []string
	current := dir
	for {
		real, err := filepath.EvalSymlinks(current)
		if err == nil {
			real = upathResolve(real)
			if len(missing) == 0 {
				return real, nil
			}
			elements := make([]string, 0, len(missing)+1)
			elements = append(elements, real)
			for i := len(missing) - 1; i >= 0; i-- {
				elements = append(elements, missing[i])
			}
			return upathJoin(elements...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", accessDeniedError()
		}
		parent := upathDir(current)
		if parent == current {
			return "", accessDeniedError()
		}
		missing = append(missing, upathBase(current))
		current = parent
	}
}

// consentRegistry is built once per served folder by a bounded walk at open
// time; every out-of-tree symlink target found is registered and logged.
type consentRegistry struct {
	baseDir  string
	log      func(string)
	baseReal string
	roots    []string
	scanned  bool
}

func newConsentRegistry(baseDir string, log func(string)) *consentRegistry {
	if log == nil {
		log = func(string) {}
	}
	return &consentRegistry{
		baseDir:  baseDir,
		log:      log,
		baseReal: upathResolve(baseDir),
	}
}

// Ready canonicalizes the base and finishes the walk once; the scan never
// reports failure, so callers can treat it as already settled.
func (r *consentRegistry) Ready() {
	if !r.scanned {
		r.scanned = true
		r.init()
	}
}

// Rescan re-runs the open-time registration, because "registered at open time"
// has to mean each open.
func (r *consentRegistry) Rescan() {
	r.scanned = true
	r.init()
}

func (r *consentRegistry) BaseReal() string {
	return r.baseReal
}

func (r *consentRegistry) IsConsented(realPath string) bool {
	for _, root := range r.roots {
		if realPath == root || strings.HasPrefix(realPath, root+"/") {
			return true
		}
	}
	return false
}

func (r *consentRegistry) ConsentedRoots() []string {
	roots := make([]string, len(r.roots))
	copy(roots, r.roots)
	return roots
}

func (r *consentRegistry) init() {
	if real, err := filepath.EvalSymlinks(r.baseDir); err == nil {
		r.baseReal = upathResolve(real)
	} else if real, err := realpathNearestParent(upathResolve(r.baseDir)); err == nil {
		r.baseReal = real
	}
	found := []string{}
	count := 0
	r.walk(r.baseReal, 0, &count, &found)
	r.roots = found
}

func (r *consentRegistry) walk(dir string, depth int, count *int, found *[]string) {
	if depth > maxConsentDepth || *count >= maxConsentEntries {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if *count >= maxConsentEntries {
			return
		}
		*count++

		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || reservedRootSegments[name] {
			continue
		}

		full := upathJoin(dir, name)
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(full)
			if err != nil {
				continue
			}
			target = upathResolve(target)
			if reachesLiveRoot(target) {
				r.log("[Server] Refused symlink into LOCAL_APPS: " + full + " -> " + target)
				continue
			}
			if !isContained(r.baseReal, target) && !containsString(*found, target) {
				*found = append(*found, target)
				r.log("[Server] Registered out-of-tree symlink: " + full + " -> " + target)
			}
			continue
		}
		if entry.IsDir() {
			r.walk(full, depth+1, count, found)
		}
	}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func isNotDirError(err error) bool {
	return errors.Is(err, syscall.ENOTDIR)
}

// isMissingPathError is a failure Node reports as ENOENT or ENOTDIR. On
// Windows, libuv also names ERROR_INVALID_NAME (a `?` or `*` in a name) ENOENT.
func isMissingPathError(err error) bool {
	if errors.Is(err, os.ErrNotExist) || isNotDirError(err) {
		return true
	}
	var errno syscall.Errno
	return runtime.GOOS == "windows" && errors.As(err, &errno) && errno == 123
}

// resolveReadPath is the canonical read resolution.
func resolveReadPath(registry *consentRegistry, relPath string) (string, error) {
	registry.Ready()
	baseReal := registry.baseReal
	joined := upathResolve(upathJoin(baseReal, relPath))
	if !isContained(baseReal, joined) {
		return "", accessDeniedError()
	}

	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if isMissingPathError(err) {
			return "", fileNotFoundError()
		}
		return "", accessDeniedError()
	}
	real = upathResolve(real)

	if !isContained(baseReal, real) {
		registry.Ready()
		if !registry.IsConsented(real) {
			return "", accessDeniedError()
		}
	}
	return real, nil
}

// resolveWritePath is the canonical create/write resolution.
func resolveWritePath(registry *consentRegistry, relPath string) (string, error) {
	registry.Ready()
	baseReal := registry.baseReal
	joined := upathResolve(upathJoin(baseReal, relPath))
	if !isContained(baseReal, joined) {
		return "", accessDeniedError()
	}

	real, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if !isMissingPathError(err) {
			return "", accessDeniedError()
		}
		parentReal, parentErr := realpathNearestParent(upathDir(joined))
		if parentErr != nil {
			return "", parentErr
		}
		real = upathJoin(parentReal, upathBase(joined))
	} else {
		real = upathResolve(real)
	}

	if !isContained(baseReal, real) {
		registry.Ready()
		if !registry.IsConsented(real) {
			return "", accessDeniedError()
		}
	}
	return real, nil
}

type resolvedRequest struct {
	RelPath  string
	RealPath string
}

// resolveRequestRead is decode + validate + canonical read, the shape the
// static routes want.
func resolveRequestRead(registry *consentRegistry, urlPath string) (resolvedRequest, error) {
	decoded, err := decodeOnce(urlPath)
	if err != nil {
		return resolvedRequest{}, err
	}
	relPath := strings.TrimLeft(decoded, "/")
	if _, err := validateSegments(relPath); err != nil {
		return resolvedRequest{}, err
	}
	realPath, err := resolveReadPath(registry, relPath)
	if err != nil {
		return resolvedRequest{}, err
	}
	return resolvedRequest{RelPath: relPath, RealPath: realPath}, nil
}
