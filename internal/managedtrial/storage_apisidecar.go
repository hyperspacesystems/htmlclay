//go:build managedtrial

package managedtrial

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/dataapi"
)

// The `/_/api` data sidecar, ported from Local's src/main/utils/api-sidecar.js.
// The extracted data is a bare JSON file (no wrapper, no hash) under a hidden
// directory beside the document:
//
//	{baseDir}/.hyperclay/api-v2/<complete-source-name>.json
//
// Hidden, not a visible `api/`, because a bare user folder named `api` is a real
// supported site path: a visible one would collide with user content and break
// syncing a person's own `api/` folder. `.hyperclay/` is already excluded by
// every sync scan.
//
// Every entry point is non-fatal: a sidecar failure must never break a save or a
// request. The file IS the data — it is served raw and regenerated on every save
// and lazily on a request miss.

// sidecarDirName is Local's artifact-paths.js API_DIR.
const sidecarDirName = ".hyperclay/api-v2"

// sidecarConsole receives the console.error line a non-fatal failure emits. Nil
// (the default) drops it, the way a library with no console attached does.
var sidecarConsole func(level, message string)

func reportSidecarFailure(message string) {
	if sidecarConsole != nil {
		sidecarConsole("error", message)
	}
}

// sidecarRelPath keeps the source extension so same-basename documents have
// separate caches.
func sidecarRelPath(name string) string {
	return upathJoin(sidecarDirName, name+".json")
}

// resolveSidecarPath is the absolute sidecar path with a containment check, so a
// crafted name can never escape baseDir even though callers already validate the
// source name.
func resolveSidecarPath(baseDir, name string) (string, error) {
	abs := upathResolve(upathJoin(baseDir, sidecarRelPath(name)))
	base := upathResolve(baseDir)
	if !strings.HasPrefix(abs, base+"/") {
		return "", errors.New("Sidecar path escapes base directory")
	}
	return abs, nil
}

// sidecarCanonicalBase is the CANONICAL base, not the lexical one — on macOS the
// served folder commonly sits under the /var -> /private/var symlink, so
// comparing a realpath'd target against a lexical base rejects every legitimate
// path in the folder.
func sidecarCanonicalBase(baseDir string) string {
	real, err := canonicalizeBase(baseDir)
	if err != nil {
		return upathResolve(baseDir)
	}
	return upathResolve(real)
}

// resolveSidecarWritePath canonicalizes the nearest existing parent (the sidecar
// itself usually does not exist yet) and rechecks containment there. That second
// half is what a lexical resolve cannot do: with `.hyperclay/api-v2/blog` linked
// to an external folder, `blog/post.html.json` resolves lexically inside the
// tree while the write lands outside it.
func resolveSidecarWritePath(baseDir, abs string) (string, error) {
	parentReal, err := realpathNearestParent(upathDir(abs))
	if err != nil {
		return "", err
	}
	target := upathJoin(parentReal, upathBase(abs))
	if !isContained(sidecarCanonicalBase(baseDir), target) {
		return "", errors.New("Sidecar path escapes base directory")
	}
	return target, nil
}

// resolveSidecarCanonical is the single resolver every sidecar operation goes
// through — stat, read, write and unlink alike. The lexical check runs first so
// a crafted name is rejected before it touches the filesystem.
func resolveSidecarCanonical(baseDir, name string) (string, error) {
	lexical, err := resolveSidecarPath(baseDir, name)
	if err != nil {
		return "", err
	}
	return resolveSidecarWritePath(baseDir, lexical)
}

// guardedUnlink unlinks target after re-verifying its directory chain. unlink
// never follows the final component, so only the directory chain matters, and a
// missing file is not an error.
func guardedUnlink(canonicalBase, target string) error {
	if err := assertRealDirChain(canonicalBase, upathDir(target)); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// sidecarRead is one file's mtime and text, taken off a single open handle.
type sidecarRead struct {
	mtimeMs float64
	text    string
}

// nodeMtimeMs is a modification time the way Node's fs.Stats.mtimeMs reports it:
// milliseconds with the sub-millisecond part kept. Local compares a sidecar with
// its document at that precision, so a document written later in the same
// millisecond is newer than its sidecar here too.
func nodeMtimeMs(t time.Time) float64 {
	return float64(t.Unix())*1e3 + float64(t.Nanosecond())/1e6
}

// guardedOpenRead re-verifies the directory chain immediately before the read,
// then takes the stat and the bytes off the SAME handle so they describe one
// inode with no reopen gap.
//
// A symlinked final component is refused outright. Local relies on O_NOFOLLOW
// for that on POSIX and on this very lstat on Windows; Go's portable open has no
// O_NOFOLLOW, so the lstat runs everywhere. The refusal — not its errno — is what
// carries over. The lstat is not atomic with the open, the same residual Local
// accepts for the directory chain: a same-user process that swaps the file for a
// symlink in between already holds the user's full filesystem authority.
func guardedOpenRead(canonicalBase, target string) (sidecarRead, error) {
	if err := assertRealDirChain(canonicalBase, upathDir(target)); err != nil {
		return sidecarRead{}, err
	}
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return sidecarRead{}, errors.New("Refusing to read '" + target + "': the sidecar is a symlink")
	}
	handle, err := os.Open(target)
	if err != nil {
		return sidecarRead{}, err
	}
	info, err := handle.Stat()
	if err != nil {
		handle.Close()
		return sidecarRead{}, err
	}
	data, err := io.ReadAll(handle)
	if closeErr := handle.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return sidecarRead{}, err
	}
	return sidecarRead{mtimeMs: nodeMtimeMs(info.ModTime()), text: string(data)}, nil
}

// writeAPISidecarData writes the bare JSON, or deletes the file when data is nil
// (the page lost its api tag). Empty [] / {} are valid data and ARE written.
// Non-fatal.
func writeAPISidecarData(baseDir, name string, data []byte) {
	if err := writeSidecarData(baseDir, name, data); err != nil {
		reportSidecarFailure("writeApiSidecarData failed (non-fatal): " + err.Error())
	}
}

func writeSidecarData(baseDir, name string, data []byte) error {
	cbase := sidecarCanonicalBase(baseDir)
	target, err := resolveSidecarCanonical(baseDir, name)
	if err != nil {
		return err
	}
	if data == nil {
		return guardedUnlink(cbase, target)
	}
	// Re-verify the directory chain immediately before publishing: atomicWriteFile
	// renames a temp into place, replacing rather than following a final symlink.
	if err := assertRealDirChain(cbase, upathDir(target)); err != nil {
		return err
	}
	return atomicWriteFile(target, data, nil, false)
}

// writeAPISidecar extracts the api rules from html, then writes or refreshes the
// bare file (or removes it). A malformed or unknown-version tag cannot yield
// valid data, so a stale file is deleted rather than served. Non-fatal.
func writeAPISidecar(baseDir, name, html string) {
	data, err := extractAPIData(html)
	if err != nil {
		reportSidecarFailure("writeApiSidecar extract failed (non-fatal): " + err.Error())
		writeAPISidecarData(baseDir, name, nil)
		return
	}
	writeAPISidecarData(baseDir, name, data)
}

// extractAPIData is data-extractor.js' extractViaTag(html, "api"): the document's
// own rules tag, the data API's extraction, and the result encoded the way
// JSON.stringify encodes it. A document with no api tag yields nil bytes, which
// is the delete signal.
func extractAPIData(html string) ([]byte, error) {
	result, err := dataapi.ReadDocument([]byte(html), dataapi.ReadOptions{})
	if err != nil {
		return nil, err
	}
	if result.Data == nil {
		return nil, nil
	}
	return dataapi.Marshal(result.Data)
}

// deleteAPISidecar removes the sidecar if present. Non-fatal.
func deleteAPISidecar(baseDir, name string) {
	cbase := sidecarCanonicalBase(baseDir)
	target, err := resolveSidecarCanonical(baseDir, name)
	if err == nil {
		err = guardedUnlink(cbase, target)
	}
	if err != nil {
		reportSidecarFailure("deleteApiSidecar failed (non-fatal): " + err.Error())
	}
}

// readFreshSidecar returns the sidecar text only when it exists AND is at least
// as new as the source file; otherwise nothing, and the caller regenerates. The
// mtime check closes the hand-edit / external-delete staleness gap the platform
// never has, because there files only change through lifecycle paths that
// rewrite the sidecar.
func readFreshSidecar(baseDir, name string, sourceMtimeMs float64) (string, bool) {
	cbase := sidecarCanonicalBase(baseDir)
	target, err := resolveSidecarCanonical(baseDir, name)
	if err != nil {
		return "", false
	}
	read, err := guardedOpenRead(cbase, target)
	if err != nil {
		return "", false
	}
	if read.mtimeMs >= sourceMtimeMs {
		return read.text, true
	}
	return "", false
}
