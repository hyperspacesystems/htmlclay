package served

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type keepReplacedAnswerHelper struct {
	status int
	body   map[string]any
	raw    string
}

func keepReplacedSendHelper(t *testing.T, method, serverURL, path, contentType, body string, headers map[string]string) keepReplacedAnswerHelper {
	t.Helper()
	request, err := http.NewRequest(method, serverURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = racePublicHost
	request.Header.Set("Origin", "http://"+racePublicHost)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", "http://"+racePublicHost+"/"+raceDocName)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	answer := keepReplacedAnswerHelper{status: response.StatusCode, raw: string(raw)}
	_ = json.Unmarshal(raw, &answer.body)
	return answer
}

func keepReplacedEtagHelper(answer keepReplacedAnswerHelper) string {
	etag, _ := answer.body["etag"].(string)
	return etag
}

func keepReplacedDuplicatesHelper(versions []raceVersion) int {
	seen := map[string]int{}
	for _, version := range versions {
		seen[version.body]++
	}
	duplicates := 0
	for _, count := range seen {
		duplicates += count - 1
	}
	return duplicates
}

// The hour a clock goes back: local names repeat, so a plain string order of
// names and the store's order by instant disagree about which version is newest.
// Saves must still keep every replaced document.
func TestKeepReplacedNewestOrdering(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	previousZone := versionZone
	versionZone = func() *time.Location { return zone }
	previousClock := versionClock
	t.Cleanup(func() {
		versionZone = previousZone
		versionClock = previousClock
	})

	dir := raceTempDir(t)
	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "doc")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	beforeFallBack := time.Date(2026, 11, 1, 5, 45, 0, 0, time.UTC)
	afterFallBack := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	current := raceBrowserBody("current")
	older := raceBrowserBody("older")
	olderName := GenerateTimestamp(beforeFallBack.In(zone)) + ".html"
	newerName := GenerateTimestamp(afterFallBack.In(zone)) + ".html"
	if err := os.WriteFile(filepath.Join(versionsDir, olderName), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionsDir, newerName), []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := raceBrowserBody("outside-edit")
	docPath := filepath.Join(dir, raceDocName)
	if err := os.WriteFile(docPath, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}

	server, _ := raceServer(t, dir, false)
	clock := afterFallBack
	versionClock = func() int64 { return clock.UnixMilli() }
	const saves = 4
	for save := 1; save <= saves; save++ {
		clock = clock.Add(time.Minute)
		answer := raceSendWrite(server.URL, fmt.Sprintf("dst-%d", save), false)
		if answer.status != http.StatusOK {
			t.Fatalf("save %d: %s", save, answer)
		}
	}
	versions := raceVersions(t, dir)
	held := false
	for _, version := range versions {
		held = held || version.body == outside
	}
	if !held {
		t.Fatalf("the outside edit on disk was replaced and no version holds its bytes (versions=%d)", len(versions))
	}
	if len(versions) != 3+saves {
		t.Fatalf("2 versions + the outside edit + %d distinct saves -> %d versions, want %d (duplicates=%d)", saves, len(versions), 3+saves, keepReplacedDuplicatesHelper(versions))
	}
	if duplicates := keepReplacedDuplicatesHelper(versions); duplicates != 0 {
		t.Fatalf("%d versions hold bytes another version already holds", duplicates)
	}
}

// A file that is not a version, left in a document's versions folder, must not
// stop saves from keeping what they replace.
func TestKeepReplacedStrayFile(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)
	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "doc")
	if err := os.WriteFile(filepath.Join(versionsDir, "desktop.ini"), []byte("[.ShellClassInfo]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := raceOutsideBody("stray-outside")
	if err := os.WriteFile(filepath.Join(dir, raceDocName), []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	const saves = 4
	for save := 1; save <= saves; save++ {
		raceSendWrite(server.URL, fmt.Sprintf("stray-%d", save), false)
	}
	versions := raceVersions(t, dir)
	held := false
	for _, version := range versions {
		held = held || version.body == outside
	}
	if !held {
		t.Fatalf("the outside edit beside desktop.ini was replaced and no version holds its bytes (files=%d)", len(versions))
	}
	if len(versions) != 3+saves {
		t.Fatalf("seed + the outside edit + %d distinct saves beside desktop.ini -> %d files in the versions folder, want %d (duplicates=%d)", saves, len(versions), 3+saves, keepReplacedDuplicatesHelper(versions))
	}
	if duplicates := keepReplacedDuplicatesHelper(versions); duplicates != 0 {
		t.Fatalf("%d versions hold bytes another version already holds", duplicates)
	}
}

// An outside write that lands while a save is in flight, driven through the
// request's own BeforeRename, which the looping variant runs before its read.
func TestKeepReplacedMidSaveCatch(t *testing.T) {
	dir := raceTempDir(t)
	server, root := raceServer(t, dir, false)
	raceSeed(t, server)
	docPath := filepath.Join(dir, raceDocName)
	outside := raceOutsideBody("mid-save-outside")
	result, err := root.commit.commit(&commitRequest{
		Kind: "browser-save", Name: raceDocName, Bytes: []byte(raceBrowserBody("save-a")), Text: true,
		BeforeRename: func() error { return os.WriteFile(docPath, []byte(outside), 0o644) },
	})
	afterCatch := raceVersions(t, dir)
	kept := false
	for _, version := range afterCatch {
		kept = kept || version.body == outside
	}
	if !kept {
		t.Fatalf("commit=%s err=%v: the outside write landed before the rename and no version holds its exact bytes (versions=%d)", result.Outcome, err, len(afterCatch))
	}
	answer := raceSendWrite(server.URL, "save-b", false)
	if answer.status != http.StatusOK {
		t.Fatalf("save after the catch: %s", answer)
	}
	versions := raceVersions(t, dir)
	stillKept := false
	for _, version := range versions {
		stillKept = stillKept || version.body == outside
	}
	if !stillKept {
		t.Fatalf("the save that followed dropped the outside write no version had before it")
	}
	if len(versions) <= len(afterCatch) {
		t.Fatalf("the save after the catch added no version (%d, was %d)", len(versions), len(afterCatch))
	}
}

// One overwrite, with no timing in it: an outside edit, a refused save, and the
// person's overwrite. The outside edit must survive in a version.
func TestKeepReplacedSingleOverwrite(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	seed := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceDocument(`<h1>seed</h1>`), nil)
	second := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceBrowserBody("tab-1"), map[string]string{"If-Match": keepReplacedEtagHelper(seed)})
	docPath := filepath.Join(dir, raceDocName)
	if err := os.WriteFile(docPath, []byte(raceOutsideBody("vscode-edit")), 0o644); err != nil {
		t.Fatal(err)
	}
	refusedSave := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceBrowserBody("tab-2"), map[string]string{"If-Match": keepReplacedEtagHelper(second)})
	overwriteSave := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceBrowserBody("tab-2"), map[string]string{"If-Match": keepReplacedEtagHelper(refusedSave)})
	versions := raceVersions(t, dir)
	if refusedSave.status != http.StatusPreconditionFailed {
		t.Fatalf("the save over an outside edit answered %d, want %d", refusedSave.status, http.StatusPreconditionFailed)
	}
	if overwriteSave.status != http.StatusOK {
		t.Fatalf("the overwrite answered %d: %s", overwriteSave.status, strings.TrimSpace(overwriteSave.raw))
	}
	if !raceHeld(versions, "vscode-edit") {
		t.Fatalf("the outside edit the person overwrote is in no version (versions=%d)", len(versions))
	}
}

// The guard's Revert of an outside edit that dropped saved data: the outside
// bytes the revert replaces must still be kept somewhere.
func TestKeepReplacedGuardRevertOfOutsideEdit(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)
	raceSendWrite(server.URL, "tab-1", false)
	docPath := filepath.Join(dir, raceDocName)
	outside := raceDocument(`<h1></h1>` + "\n" + `<section>OUTSIDE-REWRITE</section>`)
	if err := os.WriteFile(docPath, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	event := keepReplacedSendHelper(t, http.MethodGet, server.URL, "/_/data-loss?file="+raceDocName, "", "", nil)
	eventBody, _ := event.body["event"].(map[string]any)
	id, _ := eventBody["id"].(string)
	if id == "" {
		t.Fatalf("no data-loss event was raised for the outside edit: %s", strings.TrimSpace(event.raw))
	}
	revert := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/data-loss", "application/json",
		`{"file":"`+raceDocName+`","id":"`+id+`","choice":"revert"}`, nil)
	if revert.status != http.StatusOK {
		t.Fatalf("revert answered %d: %s", revert.status, strings.TrimSpace(revert.raw))
	}
	versions := raceVersions(t, dir)
	if !raceHeld(versions, "OUTSIDE-REWRITE") {
		t.Fatalf("the revert replaced the outside edit and no version holds it (versions=%d)", len(versions))
	}
}

// keepReplacedNewestVersionHelper is the version the store's own ordering calls
// newest in one versions folder.
func keepReplacedNewestVersionHelper(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	candidates := []VersionEntry{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !IsVersionName(entry.Name()) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			t.Fatalf("statting %s: %v", entry.Name(), infoErr)
		}
		candidates = append(candidates, VersionEntry{Name: entry.Name(), Full: filepath.Join(dir, entry.Name()), MtimeMs: info.ModTime().UnixMilli()})
	}
	if len(candidates) == 0 {
		t.Fatalf("no version is stored in %s", dir)
	}
	sortNewestFirst(candidates)
	return raceRead(t, candidates[0].Full)
}

// When the bytes a save replaces need a version and the version cannot be
// written, the save is refused and the file keeps its bytes.
func TestKeepReplacedBackupFailureRefuses(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only versions folder")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores a folder's read-only mode")
	}
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)
	docPath := filepath.Join(dir, raceDocName)
	outside := raceOutsideBody("backup-failed-outside")
	if err := os.WriteFile(docPath, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "doc")
	if err := os.Chmod(versionsDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(versionsDir, 0o755) })

	answer := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceBrowserBody("save-over-outside"), nil)
	if answer.status != http.StatusConflict {
		t.Fatalf("the save whose version could not be written answered %d, want %d: %s", answer.status, http.StatusConflict, strings.TrimSpace(answer.raw))
	}
	if code, _ := answer.body["code"].(string); code != "backup-failed" {
		t.Fatalf("the refused save answered code %q, want backup-failed: %s", code, strings.TrimSpace(answer.raw))
	}
	current, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading %s after the refused save: %v", docPath, err)
	}
	if string(current) != outside {
		t.Fatalf("the refused save replaced %s: %d bytes on disk, want the outside edit's %d bytes", raceDocName, len(current), len(outside))
	}
}

// A write caught mid-save lands after the save's own version, so the outside
// bytes are what the store's ordering calls newest, and the save that follows
// adds exactly one version rather than keeping those bytes again.
func TestKeepReplacedMidSaveNewestIsDisk(t *testing.T) {
	dir := raceTempDir(t)
	server, root := raceServer(t, dir, false)
	raceSeed(t, server)
	docPath := filepath.Join(dir, raceDocName)
	outside := raceOutsideBody("mid-save-newest")
	_, err := root.commit.commit(&commitRequest{
		Kind: "browser-save", Name: raceDocName, Bytes: []byte(raceBrowserBody("mid-save-newest-save")), Text: true,
		BeforeRename: func() error { return os.WriteFile(docPath, []byte(outside), 0o644) },
	})
	if err != nil {
		t.Fatalf("the save caught mid-write failed: %v", err)
	}
	afterCatch := raceVersions(t, dir)
	if !raceHeld(afterCatch, "mid-save-newest") {
		t.Fatalf("the outside write landed before the rename and no version holds its exact bytes (versions=%d)", len(afterCatch))
	}

	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "doc")
	newest := keepReplacedNewestVersionHelper(t, versionsDir)
	doc := raceRead(t, docPath)
	if newest != doc {
		t.Fatalf("the newest version holds %d bytes on disk=%t, want the file's %d bytes", len(newest), newest == doc, len(doc))
	}

	before := len(raceVersions(t, dir))
	answer := raceSendWrite(server.URL, "mid-save-newest-after", false)
	if answer.status != http.StatusOK {
		t.Fatalf("the save after the catch: %s", answer)
	}
	if after := len(raceVersions(t, dir)); after != before+1 {
		t.Fatalf("one ordinary save added %d versions, want 1 (%d -> %d)", after-before, before, after)
	}
}

// Two documents named alike share one versions folder. Saving them alternately
// must not store the same bytes twice.
func TestKeepReplacedSharedVersionsFolder(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	names := []string{"notes.html", "notes.htmlclay"}
	const rounds = 3
	saves := 0
	for round := 1; round <= rounds; round++ {
		for _, name := range names {
			saves++
			marker := fmt.Sprintf("shared-%02d", saves)
			answer := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceBrowserBody(marker),
				map[string]string{"Document-URL": "http://" + racePublicHost + "/" + name})
			if answer.status != http.StatusOK {
				t.Fatalf("save %d of %s answered %d, want 200: %s", round, name, answer.status, strings.TrimSpace(answer.raw))
			}
		}
	}

	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "notes")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", versionsDir, err)
	}
	seen := map[string]string{}
	stored := 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !IsVersionName(entry.Name()) {
			continue
		}
		stored++
		body := raceRead(t, filepath.Join(versionsDir, entry.Name()))
		if earlier, ok := seen[body]; ok {
			t.Fatalf("%d saves left %s and %s holding the same %d bytes", saves, earlier, entry.Name(), len(body))
		}
		seen[body] = entry.Name()
	}
	if stored == 0 {
		t.Fatalf("%d saves left no version in %s", saves, versionsDir)
	}
}

// A document this host cannot read is bytes it cannot keep, so the save is
// refused and the file keeps its exact bytes.
func TestKeepReplacedUnreadableRefuses(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file with no permissions")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores a file's permission bits")
	}
	dir := raceTempDir(t)
	server, root := raceServer(t, dir, false)
	raceSeed(t, server)
	docPath := filepath.Join(dir, raceDocName)
	outside := raceOutsideBody("unreadable-outside")
	if err := os.WriteFile(docPath, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(docPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(docPath, 0o644) })

	answer := keepReplacedSendHelper(t, http.MethodPost, server.URL, "/_/save", "text/html", raceBrowserBody("save-over-unreadable"), nil)
	if answer.status != http.StatusConflict {
		t.Fatalf("the save over an unreadable document answered %d, want %d: %s", answer.status, http.StatusConflict, strings.TrimSpace(answer.raw))
	}
	if code, _ := answer.body["code"].(string); code != "backup-failed" {
		t.Fatalf("the refused save answered code %q, want backup-failed: %s", code, strings.TrimSpace(answer.raw))
	}
	// keepReplaced's own contract: a read error other than "does not exist" is
	// bytes this save cannot keep, before the pre-rename read is asked anything.
	if _, kept := root.commit.keepReplaced(raceDocName, docPath); kept {
		t.Fatalf("keepReplaced kept a document it could not read")
	}
	if err := os.Chmod(docPath, 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading %s after the refused save: %v", docPath, err)
	}
	if string(current) != outside {
		t.Fatalf("the refused save replaced %s: %d bytes on disk, want the outside edit's %d bytes", raceDocName, len(current), len(outside))
	}
	if versions := raceVersions(t, dir); raceHeld(versions, "save-over-unreadable") {
		t.Fatalf("the refused save kept a version of its own bytes, though it wrote nothing (versions=%d)", len(versions))
	}
}

// A save whose bytes already are what the file holds still publishes its own
// version after a write caught mid-save: the bytes it wrote are what the store
// must call newest, and the bytes the outside write replaced stay in a version.
func TestKeepReplacedSameBytesCaughtOrder(t *testing.T) {
	dir := raceTempDir(t)
	server, root := raceServer(t, dir, false)
	incoming := raceBrowserBody("same-bytes-save")
	if answer := raceSendWrite(server.URL, "same-bytes-save", false); answer.status != http.StatusOK {
		t.Fatalf("the save that establishes the document: %s", answer)
	}
	docPath := filepath.Join(dir, raceDocName)
	current := raceRead(t, docPath)
	outside := raceOutsideBody("same-bytes-outside")
	result, err := root.commit.commit(&commitRequest{
		Kind: "browser-save", Name: raceDocName, Bytes: []byte(incoming), Text: true,
		BeforeRename: func() error { return os.WriteFile(docPath, []byte(outside), 0o644) },
	})
	if err != nil {
		t.Fatalf("the save whose bytes match the file: %v", err)
	}
	if result.Outcome != "committed" {
		t.Fatalf("the save whose bytes match the file answered %q, want committed", result.Outcome)
	}

	versionsDir := filepath.Join(dir, versionsDirName, backupSiteName(raceDocName))
	newest := keepReplacedNewestVersionHelper(t, versionsDir)
	doc := raceRead(t, docPath)
	if newest != doc {
		t.Fatalf("the newest version holds %d bytes, the file holds %d (same=%t, was %d before the save): the save's own version was not published after the outside write",
			len(newest), len(doc), newest == doc, len(current))
	}
	held := false
	for _, version := range raceVersions(t, dir) {
		held = held || version.body == outside
	}
	if !held {
		t.Fatalf("the outside write the mid-save catch replaced is in no version")
	}
}

// A filesystem without hard links (exFAT, some network and cloud drives)
// publishes versions by rename instead, and never overwrites one already there.
func TestKeepReplacedLinkFallback(t *testing.T) {
	fixed := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	previousClock := versionClock
	versionClock = func() int64 { return fixed.UnixMilli() }
	previousLink := linkFile
	linkFile = func(old, new string) error {
		return &os.LinkError{Op: "link", Old: old, New: new, Err: errors.New("Incorrect function.")}
	}
	t.Cleanup(func() {
		versionClock = previousClock
		linkFile = previousLink
	})

	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	versionsDir := filepath.Join(dir, versionsDirName, backupSiteName(raceDocName))
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	taken := GenerateTimestamp(fixed.In(versionZone())) + ".html"
	if !IsVersionName(taken) {
		t.Fatalf("%q is not a version name", taken)
	}
	if err := os.Symlink(filepath.Join(dir, "no-such-target"), filepath.Join(versionsDir, taken)); err != nil {
		t.Fatal(err)
	}

	for save := 1; save <= 2; save++ {
		answer := raceSendWrite(server.URL, fmt.Sprintf("link-fallback-%d", save), false)
		if answer.status != http.StatusOK {
			t.Fatalf("save %d on a filesystem without hard links answered %s", save, answer)
		}
	}

	if info, err := os.Lstat(filepath.Join(versionsDir, taken)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s was replaced by a version: %v %v", taken, info, err)
	}
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", versionsDir, err)
	}
	names := []string{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && IsVersionName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	if len(names) != 2 {
		t.Fatalf("two saves on a filesystem without hard links left %d versions %v, want 2 distinct ones", len(names), names)
	}
	if names[0] == names[1] {
		t.Fatalf("two saves left one version name %q: the second overwrote the first", names[0])
	}
}

// TestKeepReplacedWriteDuringKeep: keeping a caught outside write as a version
// takes a while, and an editor can save again meanwhile. The save reads again
// before its rename, so that second write is kept too.
func TestKeepReplacedWriteDuringKeep(t *testing.T) {
	dir := raceTempDir(t)
	server, root := raceServer(t, dir, false)
	raceSeed(t, server)
	docPath := filepath.Join(dir, raceDocName)
	first := raceOutsideBody("during-keep-first")
	second := raceOutsideBody("during-keep-second")

	previousClock := versionClock
	t.Cleanup(func() { versionClock = previousClock })
	armed, wrote := false, false
	versionClock = func() int64 {
		if armed && !wrote {
			wrote = true
			if err := os.WriteFile(docPath, []byte(second), 0o644); err != nil {
				t.Errorf("second outside write: %v", err)
			}
		}
		return previousClock()
	}

	result, err := root.commit.commit(&commitRequest{
		Kind: "browser-save", Name: raceDocName, Bytes: []byte(raceBrowserBody("save-a")), Text: true,
		BeforeRename: func() error {
			armed = true
			return os.WriteFile(docPath, []byte(first), 0o644)
		},
	})
	if err != nil || result.Outcome != "committed" {
		t.Fatalf("save: outcome=%s err=%v", result.Outcome, err)
	}
	if !wrote {
		t.Fatal("the second outside write never happened, so nothing was proved")
	}
	held := map[string]bool{}
	for _, version := range raceVersions(t, dir) {
		held[version.body] = true
	}
	if !held[first] {
		t.Fatal("the first outside write was replaced and no version holds it")
	}
	if !held[second] {
		t.Fatal("the outside write made while the first was being kept was replaced and no version holds it")
	}
}
