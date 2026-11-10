//go:build managedtrial

package managedtrial

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The version store, ported from Local's src/main/utils/backup.js and
// src/main/utils/prune-versions.js. A person's version history and the version
// list in the UI read this exact layout, so the port keeps Local's names,
// ordering rules, symlink refusals and log lines verbatim.

const (
	// versionsDirName and recoveredDirName are Local's artifact-paths.js
	// VERSIONS_DIR and RECOVERED_DIR.
	versionsDirName  = ".hyperclay/versions"
	recoveredDirName = ".hyperclay/recovered"
)

// Retention for .hyperclay/versions/. Delete anything older than 60 days,
// always keep the newest 20 per site, and retain the UNION of those two sets.
const (
	maxAgeMS   = int64(60 * 24 * 60 * 60 * 1000)
	keepNewest = 20
)

// The bare name plus suffixes -001 through -999, so at most 1000 versions can
// share one instant before the instant is rolled forward.
const maxCollisionAttempts = 1000

// Opportunistic pruning: at most once an hour per site directory, never on the
// caller's critical path, and never able to fail a save.
const pruneIntervalMS = int64(60 * 60 * 1000)

// versionNameRe is Local's VERSION_NAME: `YYYY-MM-DD-HH-MM-SS-mmm`, an optional
// zone (a signed four-digit UTC offset, or the older bare `Z`), and an optional
// zero-padded collision suffix. The offset group is fixed width and positional,
// so `431-0400` splits into millis `431` and offset `-0400`. The suffix MUST be
// matched: an unrecognised name is one the pruner refuses to touch, so without
// it every collision-suffixed version would accumulate forever.
var versionNameRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{3})(Z|[+-]\d{4})?(?:-(\d{3}))?\.[A-Za-z0-9]+$`)

var recoveredDocRe = regexp.MustCompile(`(?i)\.(html|htmlclay)$`)

// versionClock is the Date/Date.now source backup.js reaches for when its
// caller does not pass an instant (publishVersion's `new Date()`, maybePrune's
// `Date.now()`). It is a variable so a test can freeze it exactly as the golden
// generator stubs the Date constructor.
var versionClock = func() int64 { return time.Now().UnixMilli() }

func clockTime() time.Time {
	return time.UnixMilli(versionClock()).In(time.Local)
}

// VersionField is one key/value pair of an emitted event payload or a logger
// field object, in the order Local's literals declare them.
type VersionField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// VersionObserver receives everything backup.js does besides the filesystem:
// the `emit` events, the `logger` calls and the console lines. A nil observer
// (or a nil member) is the JS `emit`/`logger` being absent; console lines are
// then dropped.
type VersionObserver struct {
	Emit    func(event string, fields []VersionField)
	Log     func(level, tag, message string, fields []VersionField)
	Console func(level, message string)
}

func observerConsole(obs *VersionObserver, level, message string) {
	if obs != nil && obs.Console != nil {
		obs.Console(level, message)
	}
}

func observerLog(obs *VersionObserver, level, tag, message string, fields ...VersionField) {
	if obs != nil && obs.Log != nil {
		obs.Log(level, tag, message, fields)
	}
}

func observerEmit(obs *VersionObserver, event string, fields ...VersionField) {
	if obs != nil && obs.Emit != nil {
		obs.Emit(event, fields)
	}
}

// VersionEntry is one candidate version file: its name, full path and the mtime
// that ranks it when its name is not a trustworthy instant.
type VersionEntry struct {
	Name    string
	Full    string
	MtimeMs int64
}

// PublishedVersion is the name and path publishVersion landed on.
type PublishedVersion struct {
	Filename string
	Full     string
}

// PruneResult is what pruneSiteVersions touched.
type PruneResult struct {
	Kept    int
	Deleted []string
}

// PruneAllResult is what pruneAllVersions swept.
type PruneAllResult struct {
	Sites   int
	Deleted int
}

// GenerateTimestamp renders LOCAL wall time plus the signed UTC offset in force
// at that moment, e.g. 2026-11-01-01-30-00-431-0400.
//
// Local time is what the user reads in their file browser. The offset is what
// makes it orderable: local wall time repeats for one hour on every DST
// fall-back, and the pruner DELETES, so a name that cannot be resolved to one
// instant can cost the user the version they wanted back.
//
// Note the sign. JavaScript's getTimezoneOffset() returns POSITIVE minutes for
// zones BEHIND UTC, so it negates to render as -0400; Go's Zone() offset has
// the sign already, so it is used as-is. Getting that backwards inverts every
// ordering.
func GenerateTimestamp(now time.Time) string {
	_, offsetSeconds := now.Zone()
	offsetMinutes := offsetSeconds / 60
	sign := "+"
	if offsetMinutes < 0 {
		sign = "-"
		offsetMinutes = -offsetMinutes
	}
	return fmt.Sprintf("%04d-%02d-%02d-%02d-%02d-%02d-%03d%s%02d%02d",
		now.Year(), int(now.Month()), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
		now.Nanosecond()/int(time.Millisecond),
		sign, offsetMinutes/60, offsetMinutes%60)
}

// IsVersionName reports whether the name is one this store wrote.
func IsVersionName(name string) bool {
	return versionNameRe.MatchString(name)
}

// ParseVersionTimestamp is the epoch ms for a version filename. ok is false
// when the name is not a version name or is legacy local wall time with no
// zone (ambiguous, so the caller must use mtime instead).
func ParseVersionTimestamp(filename string) (int64, bool) {
	match := versionNameRe.FindStringSubmatch(filename)
	if match == nil {
		return 0, false
	}
	zone := match[8]
	if zone == "" {
		return 0, false
	}

	wallClock := time.Date(atoi(match[1]), time.Month(atoi(match[2])), atoi(match[3]),
		atoi(match[4]), atoi(match[5]), atoi(match[6]), atoi(match[7])*int(time.Millisecond), time.UTC).UnixMilli()
	if zone == "Z" {
		return wallClock, true
	}

	offsetMinutes := int64(atoi(zone[1:3])*60 + atoi(zone[3:5]))
	if zone[0] == '-' {
		offsetMinutes = -offsetMinutes
	}
	return wallClock - offsetMinutes*60*1000, true
}

// VersionSortKey prefers the unambiguous parsed instant and falls back to
// mtime, so a legacy DST-ambiguous name can never outrank a real one.
func VersionSortKey(entry VersionEntry) int64 {
	if parsed, ok := ParseVersionTimestamp(entry.Name); ok {
		return parsed
	}
	return entry.MtimeMs
}

// CollisionSuffix is the suffix as a number, 0 when absent. Two versions
// written in the same millisecond share an instant, so the suffix is the only
// thing that orders them, and it records the order they were written in.
func CollisionSuffix(filename string) int64 {
	match := versionNameRe.FindStringSubmatch(filename)
	if match != nil && match[9] != "" {
		return int64(atoi(match[9]))
	}
	return 0
}

// VersionStamp is the timestamp portion of a version name, without the
// collision suffix or the extension: the exact string GenerateTimestamp
// produced. ok is false for a non-version name.
func VersionStamp(filename string) (string, bool) {
	m := versionNameRe.FindStringSubmatch(filename)
	if m == nil {
		return "", false
	}
	return m[1] + "-" + m[2] + "-" + m[3] + "-" + m[4] + "-" + m[5] + "-" + m[6] + "-" + m[7] + m[8], true
}

// CompareNewestFirst orders by instant, then by collision suffix. It is the ONE
// ordering function shared with the pruner and the data-loss guard, so the
// delete path and the recovery path can never disagree about which version is
// newest.
func CompareNewestFirst(a, b VersionEntry) int {
	if diff := VersionSortKey(b) - VersionSortKey(a); diff != 0 {
		if diff < 0 {
			return -1
		}
		return 1
	}
	diff := CollisionSuffix(b.Name) - CollisionSuffix(a.Name)
	switch {
	case diff < 0:
		return -1
	case diff > 0:
		return 1
	}
	return 0
}

func sortNewestFirst(entries []VersionEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return CompareNewestFirst(entries[i], entries[j]) < 0
	})
}

// versionPlan is planVersionName's answer: the stamp to render, the first
// collision suffix to try, and the committed instant being reused when the wall
// clock has rolled backwards (nil in the ordinary forward-clock case).
type versionPlan struct {
	stamp        string
	suffix       int64
	reuseInstant *int64
}

// planVersionName chooses a name that sorts strictly AFTER every committed
// version. Normally the fresh clock instant already does; but if the wall clock
// has rolled backwards to at-or-before the newest committed instant, reuse that
// instant's timestamp string and take the next collision suffix so the new file
// still ranks first.
func planVersionName(dir string, candidateDate time.Time) versionPlan {
	plan := versionPlan{stamp: GenerateTimestamp(candidateDate)}

	names, err := os.ReadDir(dir)
	if err != nil {
		return plan
	}

	committed := make([]VersionEntry, 0, len(names))
	for _, entry := range names {
		name := entry.Name()
		if !IsVersionName(name) {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		committed = append(committed, VersionEntry{Name: name, Full: full, MtimeMs: info.ModTime().UnixMilli()})
	}

	if len(committed) == 0 {
		return plan
	}

	sortNewestFirst(committed)
	newest := committed[0]
	newestInstant := VersionSortKey(newest)
	if candidateDate.UnixMilli() <= newestInstant {
		plan.stamp, _ = VersionStamp(newest.Name)
		plan.suffix = CollisionSuffix(newest.Name) + 1
		instant := newestInstant
		plan.reuseInstant = &instant
	}
	return plan
}

// PublishVersion publishes one version atomically and monotonically. The full
// content is written to a dot-prefixed temp in the same directory, fsynced,
// chmod'd and closed FIRST; the dot prefix can never match versionNameRe, so
// the pruner, the data-loss guard and the listing all ignore whatever a crash
// leaves behind. The final name is then produced by link (atomic, no-replace),
// or by rename on a filesystem without hard links. On any failure the temp is
// removed, so the final name never points at partial bytes.
//
// Locking is the caller's concern: Local wraps this in
// withFileLock(dir), which serializes the readdir of planVersionName with the
// link of publishVersion.
func PublishVersion(dir, ext string, content []byte) (PublishedVersion, error) {
	tempPath := filepath.Join(dir, ".hyperclay-ver-"+randomHex()+".tmp")
	published, err := publishVersion(dir, ext, content, tempPath)
	os.Remove(tempPath) // already gone on the rename path
	fsyncDir(dir)
	return published, err
}

func publishVersion(dir, ext string, content []byte, tempPath string) (PublishedVersion, error) {
	handle, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return PublishedVersion{}, err
	}
	if err := writeAndSync(handle, content); err != nil {
		return PublishedVersion{}, err
	}

	plan := planVersionName(dir, clockTime())
	stamp := plan.stamp
	suffix := plan.suffix

	for {
		if suffix >= maxCollisionAttempts {
			// Every suffix for this instant is taken. When reusing a committed
			// instant (clock rollback), advance it by 1ms and reset the suffix
			// so the name still sorts strictly after everything. In the ordinary
			// forward-clock case this is a genuine 1000-in-one-instant wall.
			if plan.reuseInstant == nil {
				return PublishedVersion{}, fmt.Errorf("No free backup name for %s%s after %d attempts", stamp, ext, maxCollisionAttempts)
			}
			*plan.reuseInstant += 1
			stamp = GenerateTimestamp(time.UnixMilli(*plan.reuseInstant).In(time.Local))
			suffix = 0
		}

		filename := stamp + ext
		if suffix != 0 {
			filename = fmt.Sprintf("%s-%03d%s", stamp, suffix, ext)
		}
		full := filepath.Join(dir, filename)

		err := os.Link(tempPath, full)
		if err == nil {
			return PublishedVersion{Filename: filename, Full: full}, nil
		}
		if errors.Is(err, fs.ErrExist) {
			suffix += 1
			continue
		}
		if linkUnsupported(err) {
			// No hard links on this filesystem: publish by rename. The temp is
			// already fully written and fsynced, so no partial file can appear;
			// the only property lost vs link is no-replace exclusivity, which
			// the caller's lock already provides.
			if err := os.Rename(tempPath, full); err != nil {
				return PublishedVersion{}, err
			}
			return PublishedVersion{Filename: filename, Full: full}, nil
		}
		return PublishedVersion{}, err
	}
}

func writeAndSync(handle *os.File, content []byte) error {
	defer handle.Close()
	if _, err := handle.Write(content); err != nil {
		return err
	}
	if err := handle.Sync(); err != nil {
		return err
	}
	return handle.Chmod(0o644)
}

// fsyncDir fsyncs a directory so a freshly linked/renamed entry survives a
// crash. Best-effort: unsupported on some platforms, and never allowed to fail
// a save.
func fsyncDir(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	handle.Sync()
	handle.Close()
}

// PublishRecoveredCopy writes a recovered copy under .hyperclay/recovered/,
// refusing a path that escapes or names anything but a document.
func PublishRecoveredCopy(baseDir, rel, html string) (PublishedVersion, error) {
	if !recoveredDocRe.MatchString(rel) || filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return PublishedVersion{}, errors.New("Invalid recovered document path")
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return PublishedVersion{}, errors.New("Invalid recovered document path")
		}
	}

	dir := filepath.Join(baseDir, recoveredDirName, recoveredDocRe.ReplaceAllString(rel, ""))
	canonicalBase, err := canonicalizeBase(baseDir)
	if err != nil {
		return PublishedVersion{}, err
	}
	canonicalDir, err := rebaseOntoCanonical(canonicalBase, baseDir, dir)
	if err != nil {
		return PublishedVersion{}, err
	}
	if err := assertRealDirChain(canonicalBase, canonicalDir); err != nil {
		return PublishedVersion{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PublishedVersion{}, err
	}
	if err := assertRealDirChain(canonicalBase, canonicalDir); err != nil {
		return PublishedVersion{}, err
	}
	return PublishVersion(dir, ".html", []byte(html))
}

// AssertVersionsChain requires the directory chain from the served folder down
// to the versions directory to be symlink-free before a backup writes, or a
// planted directory symlink could redirect the write out of tree (a recursive
// mkdir over an existing symlinked dir succeeds silently).
func AssertVersionsChain(baseDir, versionsDir string) error {
	canonicalBase, err := canonicalizeBase(baseDir)
	if err != nil {
		return err
	}
	rebase, err := rebaseOntoCanonical(canonicalBase, baseDir, versionsDir)
	if err != nil {
		return err
	}
	return assertRealDirChain(canonicalBase, rebase)
}

var (
	lastPruneMu sync.Mutex
	lastPruneAt = map[string]int64{}
)

func maybePrune(baseDir, siteVersionsDir string, obs *VersionObserver) {
	now := versionClock()

	lastPruneMu.Lock()
	previous := lastPruneAt[siteVersionsDir]
	if now-previous < pruneIntervalMS {
		lastPruneMu.Unlock()
		return
	}
	lastPruneAt[siteVersionsDir] = now
	lastPruneMu.Unlock()

	result := PruneSiteVersions(baseDir, siteVersionsDir, time.UnixMilli(now).In(time.Local), obs)
	if len(result.Deleted) != 0 {
		observerConsole(obs, "log", fmt.Sprintf("[BACKUP] Pruned %d old version(s) from %s", len(result.Deleted), siteVersionsDir))
	}
}

// CreateBackup backs one document up under .hyperclay/versions/<siteName>/.
// A backup failure never fails the save, so it answers nil (Local's null) and
// logs, exactly as Local does.
func CreateBackup(baseDir, siteName string, content []byte, obs *VersionObserver) *PublishedVersion {
	published, err := createBackup(baseDir, siteName, content, obs)
	if err != nil {
		observerConsole(obs, "error", fmt.Sprintf("[BACKUP] Failed to create backup for %s: %s", siteName, err.Error()))
		observerLog(obs, "error", "BACKUP", "Backup creation failed",
			VersionField{"site", siteName},
			VersionField{"error", err.Error()})
		return nil
	}
	return &published
}

func createBackup(baseDir, siteName string, content []byte, obs *VersionObserver) (PublishedVersion, error) {
	versionsDir := filepath.Join(baseDir, versionsDirName)
	siteVersionsDir := filepath.Join(versionsDir, siteName)

	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		return PublishedVersion{}, err
	}
	if err := os.MkdirAll(siteVersionsDir, 0o755); err != nil {
		return PublishedVersion{}, err
	}

	if err := AssertVersionsChain(baseDir, siteVersionsDir); err != nil {
		return PublishedVersion{}, err
	}

	published, err := PublishVersion(siteVersionsDir, ".html", content)
	if err != nil {
		return PublishedVersion{}, err
	}
	observerConsole(obs, "log", fmt.Sprintf("[BACKUP] Created: %s/%s/%s", versionsDirName, siteName, published.Filename))

	maybePrune(baseDir, siteVersionsDir, obs)

	observerLog(obs, "info", "BACKUP", "Backup created",
		VersionField{"site", siteName},
		VersionField{"backupFile", published.Filename})
	observerEmit(obs, "backup-created",
		VersionField{"original", siteName},
		VersionField{"backup", published.Full})

	return published, nil
}

// CreateBackupIfExists backs the file up only when it exists, and answers nil
// (Local's null) when it does not.
func CreateBackupIfExists(filePath, siteName, baseDir string, obs *VersionObserver) *PublishedVersion {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	return CreateBackup(baseDir, siteName, content, obs)
}

// CreateBinaryBackup backs an upload up under
// .hyperclay/versions/uploads/<path>/<basename>/. Same atomic, monotonic
// publication as the document path: a burst of upload syncs collides on the
// instant just as easily as a burst of saves.
func CreateBinaryBackup(baseDir, uploadPath string, content []byte, obs *VersionObserver) *PublishedVersion {
	published, err := createBinaryBackup(baseDir, uploadPath, content, obs)
	if err != nil {
		observerConsole(obs, "error", fmt.Sprintf("[BACKUP] Failed to create backup for %s: %s", uploadPath, err.Error()))
		observerLog(obs, "error", "BACKUP", "Binary backup creation failed",
			VersionField{"upload", uploadPath},
			VersionField{"error", err.Error()})
		return nil
	}
	return &published
}

func createBinaryBackup(baseDir, uploadPath string, content []byte, obs *VersionObserver) (PublishedVersion, error) {
	versionsDir := filepath.Join(baseDir, versionsDirName)

	pathParts := strings.Split(uploadPath, "/")
	filename := pathParts[len(pathParts)-1]
	pathParts = pathParts[:len(pathParts)-1]
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	backupSubdir := base
	if len(pathParts) > 0 {
		backupSubdir = filepath.Join(append(pathParts, base)...)
	}
	uploadVersionsDir := filepath.Join(versionsDir, backupSubdir)

	if err := os.MkdirAll(uploadVersionsDir, 0o755); err != nil {
		return PublishedVersion{}, err
	}
	if err := AssertVersionsChain(baseDir, uploadVersionsDir); err != nil {
		return PublishedVersion{}, err
	}

	published, err := PublishVersion(uploadVersionsDir, ext, content)
	if err != nil {
		return PublishedVersion{}, err
	}
	observerConsole(obs, "log", fmt.Sprintf("[BACKUP] Created: %s/%s/%s", versionsDirName, backupSubdir, published.Filename))

	maybePrune(baseDir, uploadVersionsDir, obs)

	observerLog(obs, "info", "BACKUP", "Binary backup created",
		VersionField{"upload", uploadPath},
		VersionField{"backupFile", published.Filename})
	observerEmit(obs, "backup-created",
		VersionField{"original", uploadPath},
		VersionField{"backup", published.Full},
		VersionField{"type", "upload"})

	return published, nil
}

// CreateBinaryBackupIfExists backs an upload up only when the file exists.
func CreateBinaryBackupIfExists(filePath, uploadPath, baseDir string, obs *VersionObserver) *PublishedVersion {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	return CreateBinaryBackup(baseDir, uploadPath, content, obs)
}

// PruneSiteVersions prunes one site's versions directory: keep the newest 20
// however old they are, keep everything inside the 60-day window, and retain
// the union. The chain from the served folder down to the site directory is
// verified symlink-free on entry AND again immediately before every unlink, so
// a directory symlink planted under .hyperclay can never redirect a delete out
// of tree. A symlinked prefix refuses the prune (no-op with a log line).
func PruneSiteVersions(baseDir, siteVersionsDir string, now time.Time, obs *VersionObserver) PruneResult {
	empty := PruneResult{Deleted: []string{}}

	recoveredRoot, err := filepath.Abs(filepath.Join(baseDir, recoveredDirName))
	if err != nil {
		return empty
	}
	candidate, err := filepath.Abs(siteVersionsDir)
	if err != nil {
		return empty
	}
	if candidate == recoveredRoot || strings.HasPrefix(candidate, recoveredRoot+string(filepath.Separator)) {
		observerConsole(obs, "warn", "[BACKUP] Refusing to prune recovered copies: "+siteVersionsDir)
		return empty
	}

	canonicalBase, err := canonicalizeBase(baseDir)
	if err != nil {
		return pruneRefused(obs, siteVersionsDir, err)
	}
	chainDir, err := rebaseOntoCanonical(canonicalBase, baseDir, siteVersionsDir)
	if err != nil {
		return pruneRefused(obs, siteVersionsDir, err)
	}
	if err := assertRealDirChain(canonicalBase, chainDir); err != nil {
		return pruneRefused(obs, siteVersionsDir, err)
	}

	names, err := os.ReadDir(siteVersionsDir)
	if err != nil {
		return empty
	}

	entries := make([]VersionEntry, 0, len(names))
	for _, name := range names {
		if !IsVersionName(name.Name()) {
			continue
		}
		full := filepath.Join(siteVersionsDir, name.Name())
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		entries = append(entries, VersionEntry{Name: name.Name(), Full: full, MtimeMs: info.ModTime().UnixMilli()})
	}
	if len(entries) == 0 {
		return empty
	}

	sortNewestFirst(entries)

	keep := make(map[string]bool, keepNewest)
	for i := 0; i < len(entries) && i < keepNewest; i++ {
		keep[entries[i].Name] = true
	}
	nowMs := now.UnixMilli()
	for _, entry := range entries {
		if nowMs-VersionSortKey(entry) <= maxAgeMS {
			keep[entry.Name] = true
		}
	}

	deleted := []string{}
	for _, entry := range entries {
		if keep[entry.Name] {
			continue
		}
		// Re-check the directory chain immediately before the destructive op: a
		// few lstats per rare deletion is cheap, and unlink never follows the
		// final component, so only the directory chain matters here.
		if err := assertRealDirChain(canonicalBase, chainDir); err != nil {
			continue
		}
		if err := os.Remove(entry.Full); err != nil {
			continue
		}
		deleted = append(deleted, entry.Name)
	}

	return PruneResult{Kept: len(keep), Deleted: deleted}
}

func pruneRefused(obs *VersionObserver, dir string, err error) PruneResult {
	observerConsole(obs, "warn", fmt.Sprintf("[BACKUP] Refusing to prune %s (non-fatal): %s", dir, err.Error()))
	return PruneResult{Deleted: []string{}}
}

// PruneAllVersions walks .hyperclay/versions/ and prunes every site directory
// beneath it. A symlinked .hyperclay/versions refuses the whole sweep (the
// accepted break); a merely-absent one is a silent no-op.
func PruneAllVersions(baseDir string, now time.Time, obs *VersionObserver) PruneAllResult {
	results := PruneAllResult{}

	canonicalBase, err := canonicalizeBase(baseDir)
	if err != nil {
		return pruneAllRefused(obs, baseDir, err)
	}
	if err := assertRealDirChain(canonicalBase, filepath.Join(canonicalBase, versionsDirName)); err != nil {
		return pruneAllRefused(obs, baseDir, err)
	}

	root := filepath.Join(baseDir, versionsDirName)

	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		hasVersion := false
		for _, entry := range entries {
			if entry.Type().IsRegular() && IsVersionName(entry.Name()) {
				hasVersion = true
				break
			}
		}
		// A site directory holds version files; intermediate directories mirror
		// the site's own folder nesting, so recurse and prune wherever files
		// appear.
		if hasVersion {
			result := PruneSiteVersions(baseDir, dir, now, obs)
			results.Sites += 1
			results.Deleted += len(result.Deleted)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				walk(filepath.Join(dir, entry.Name()))
			}
		}
	}
	walk(root)

	return results
}

func pruneAllRefused(obs *VersionObserver, baseDir string, err error) PruneAllResult {
	observerConsole(obs, "warn", fmt.Sprintf("[BACKUP] Refusing to prune %s under %s (non-fatal): %s", versionsDirName, baseDir, err.Error()))
	return PruneAllResult{}
}

func atoi(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}

func randomHex() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf[:])
}

// linkUnsupported reports the errors fs.link answers on a filesystem without
// hard links (exFAT USB sticks, some network shares), where the store falls
// back to a rename so backups keep working rather than failing closed.
func linkUnsupported(err error) bool {
	var linkErr *os.LinkError
	if !errors.As(err, &linkErr) {
		return false
	}
	return errors.Is(linkErr.Err, syscall.EPERM) ||
		errors.Is(linkErr.Err, syscall.ENOTSUP) ||
		errors.Is(linkErr.Err, syscall.ENOSYS)
}
