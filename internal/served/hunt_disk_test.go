package served

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// H6: disk trouble. Every lane here breaks the disk under a save and then judges
// the folder the way a person would: the document is whole, the versions it
// replaced are whole, the save answered an error instead of a false success, the
// process survived, and once the trouble is gone the next save works.
//
// A check that fails is a finding about the host, reported rather than worked
// around.

const (
	// diskSecondRootID and diskSecondPublicHost name the second folder the
	// volume-gone lane reads after its own volume disappears.
	diskSecondRootID     = "hunt-disk-second-root"
	diskSecondPublicHost = "hunt-disk-second.htmlclay.localhost:1212"
	// diskFillCeiling stops a fill on a file system that is plainly not small,
	// so a mistaken HUNT_TMPFS cannot eat a person's real disk.
	diskFillCeiling = 256 * 1024 * 1024
)

// diskLargeDocument is a document of about 64 KB, which is the save the full
// lane makes into a file system with kilobytes free.
func diskLargeDocument(marker string) string {
	pad := strings.Repeat("<p>disk-padding</p>\n", 3072)
	return strings.Replace(raceBrowserBody(marker), "</body>", pad+"</body>", 1)
}

// diskGet fetches one path from a served root as the browser would.
func diskGet(serverURL, publicHost, path string) (int, string) {
	request, err := http.NewRequest(http.MethodGet, serverURL+path, nil)
	if err != nil {
		return 0, "new request: " + err.Error()
	}
	request.Host = publicHost
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, "transport error: " + err.Error()
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, "read error: " + err.Error()
	}
	return response.StatusCode, string(body)
}

// diskDocumentURL is the Document-URL a save sends for one served name.
func diskDocumentURL(name string) string {
	return "http://" + racePublicHost + "/" + name
}

// diskVersionBodies keys every version file under its full path.
func diskVersionBodies(t *testing.T, dir string) map[string]string {
	t.Helper()
	bodies := map[string]string{}
	for _, version := range raceVersions(t, dir) {
		bodies[version.name] = version.body
	}
	return bodies
}

// diskFill fills path until a write fails, or until limit bytes have landed when
// limit is above zero. It returns the bytes written and the failure, or nil when
// the limit or the ceiling stopped it.
func diskFill(t *testing.T, path string, limit int64) (int64, error) {
	t.Helper()
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("filling %s: %v", path, err)
	}
	chunk := make([]byte, 4096)
	written := int64(0)
	var fillErr error
	for {
		count, writeErr := handle.Write(chunk)
		written += int64(count)
		if writeErr != nil {
			fillErr = writeErr
			break
		}
		if limit > 0 && written >= limit {
			break
		}
		if limit == 0 && written >= diskFillCeiling {
			break
		}
	}
	if err := handle.Close(); err != nil && fillErr == nil {
		fillErr = err
	}
	return written, fillErr
}

// diskDocumentsIntact checks that the document holds exactly the old bytes and
// that every version file that existed before still holds exactly its own.
func diskDocumentsIntact(t *testing.T, label, docPath, oldDoc string, before, after map[string]string) {
	t.Helper()
	current, err := os.ReadFile(docPath)
	if err != nil {
		t.Errorf("%s: reading %s after the save: %v", label, docPath, err)
	} else if string(current) != oldDoc {
		t.Errorf("%s: %s is not its old bytes after the save: %d bytes now, %d before", label, filepath.Base(docPath), len(current), len(oldDoc))
	}
	for name, body := range before {
		now, ok := after[name]
		if !ok {
			t.Errorf("%s: version %s is gone after the save", label, name)
			continue
		}
		if now != body {
			t.Errorf("%s: version %s changed after the save: %d bytes now, %d before", label, name, len(now), len(body))
		}
	}
	for name, body := range after {
		if _, existed := before[name]; existed {
			continue
		}
		if !strings.Contains(body, "</html>") {
			t.Errorf("%s: version %s is not whole after the save: %d bytes", label, name, len(body))
		}
	}
}

// huntDiskFull fills a small file system and saves a document larger than the
// space left.
func huntDiskFull(t *testing.T) {
	mount := strings.TrimSpace(os.Getenv("HUNT_TMPFS"))
	if mount == "" {
		t.Skip("HUNT_TMPFS is not set: no small, nearly empty file system to fill, so the full-disk lane cannot run")
	}
	info, err := os.Stat(mount)
	if err != nil || !info.IsDir() {
		t.Skipf("HUNT_TMPFS %q is not a directory (%v), so the full-disk lane cannot run", mount, err)
	}
	limit := int64(0)
	if raw := strings.TrimSpace(os.Getenv("HUNT_FILL_LIMIT_BYTES")); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed <= 0 {
			t.Fatalf("HUNT_FILL_LIMIT_BYTES %q is not a positive byte count: %v", raw, parseErr)
		}
		limit = parsed
	}

	dir, err := os.MkdirTemp(mount, "hunt-disk-full-")
	if err != nil {
		t.Fatalf("creating a served folder inside %s: %v", mount, err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	docPath := filepath.Join(dir, raceDocName)

	fillPath := filepath.Join(mount, "hunt-disk-fill.bin")
	t.Cleanup(func() { os.Remove(fillPath) })

	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)
	if answer := raceSendWrite(server.URL, "disk-first", false); answer.err != nil || answer.status != http.StatusOK {
		t.Fatalf("saving %s the first time answered %s", raceDocName, answer)
	}
	if answer := raceSendWrite(server.URL, "disk-old", false); answer.err != nil || answer.status != http.StatusOK {
		t.Fatalf("saving %s the second time answered %s", raceDocName, answer)
	}
	oldDoc := raceRead(t, docPath)
	before := diskVersionBodies(t, dir)
	if len(before) == 0 {
		t.Fatalf("no versions exist before the fill, so a damaged version could not be judged")
	}

	written, fillErr := diskFill(t, fillPath, limit)
	noSpace := fillErr != nil && errors.Is(fillErr, syscall.ENOSPC)

	bigDoc := diskLargeDocument("disk-new")
	answer := raceSend(server.URL, "/_/save", "text/html", bigDoc)
	after := diskVersionBodies(t, dir)
	extras := raceExtras(t, dir)
	getStatus, getBody := diskGet(server.URL, racePublicHost, "/"+raceDocName)

	mode := "fill"
	if limit > 0 {
		mode = "fill-limit"
	}
	t.Logf("HUNT disk subtest=full-disk mode=%s filled=%d bytes fill-err=%v status=%s doc=%d-bytes versions-before=%d versions-after=%d extras=%v get=%d",
		mode, written, fillErr, answer, len(oldDoc), len(before), len(after), extras, getStatus)

	if !noSpace {
		if limit > 0 {
			t.Skipf("HUNT_FILL_LIMIT_BYTES stopped the fill at %d bytes without ENOSPC (%v), so the full-disk checks cannot be judged; the harness ran and the save answered %s", written, fillErr, answer)
		}
		t.Fatalf("HUNT_TMPFS %s accepted %d bytes without ENOSPC: it is not a small, nearly empty file system", mount, written)
	}

	if answer.err == nil && answer.status == http.StatusOK {
		t.Errorf("full-disk: the save answered 200 with the file system full: a false success")
	}
	diskDocumentsIntact(t, "full-disk", docPath, oldDoc, before, after)
	if len(extras) > 0 {
		t.Errorf("full-disk: the failed save left %v beside %s", extras, raceDocName)
	}
	if getStatus != http.StatusOK {
		t.Errorf("full-disk: the host answers %d for %s after the failed save (%s)", getStatus, raceDocName, getBody)
	} else if !strings.Contains(getBody, "disk-old") {
		t.Errorf("full-disk: the host serves %s without the bytes that are on disk (%d bytes)", raceDocName, len(getBody))
	}

	if err := os.Remove(fillPath); err != nil {
		t.Fatalf("removing the filler %s: %v", fillPath, err)
	}
	recovered := raceSend(server.URL, "/_/save", "text/html", bigDoc)
	landed, readErr := os.ReadFile(docPath)
	switch {
	case recovered.err != nil || recovered.status != http.StatusOK:
		t.Errorf("full-disk: the save after the filler was removed answered %s, want 200", recovered)
	case readErr != nil:
		t.Errorf("full-disk: reading %s after the recovery save: %v", docPath, readErr)
	case !strings.Contains(string(landed), "disk-new"):
		t.Errorf("full-disk: the recovery save answered 200 but %s does not hold its bytes (%d bytes)", raceDocName, len(landed))
	case len(landed) < 60*1024:
		t.Errorf("full-disk: the recovery save answered 200 but %s holds %d bytes, too few for the roughly 64 KB document it was sent", raceDocName, len(landed))
	}
	t.Logf("HUNT disk note=full-disk recovered-status=%s landed=%d-bytes versions=%d", recovered, len(landed), len(after))
}

// huntDiskReadOnlyDocument saves over a document the person made read-only.
func huntDiskReadOnlyDocument(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)
	if answer := raceSendWrite(server.URL, "disk-keep", false); answer.err != nil || answer.status != http.StatusOK {
		t.Fatalf("saving %s before the mode change answered %s", raceDocName, answer)
	}

	docPath := filepath.Join(dir, raceDocName)
	oldDoc := raceRead(t, docPath)
	if err := os.Chmod(docPath, 0o444); err != nil {
		t.Fatalf("chmod 0444 %s: %v", docPath, err)
	}
	t.Cleanup(func() { os.Chmod(docPath, 0o644) })

	before := diskVersionBodies(t, dir)
	answer := raceSendWrite(server.URL, "disk-new", false)
	current, readErr := os.ReadFile(docPath)
	if readErr != nil {
		t.Fatalf("reading %s after the save: %v", docPath, readErr)
	}
	after := diskVersionBodies(t, dir)
	replaced := string(current) != oldDoc

	t.Logf("HUNT disk subtest=read-only-document status=%s replaced=%v doc=%d-bytes old=%d-bytes versions-before=%d versions-after=%d",
		answer, replaced, len(current), len(oldDoc), len(before), len(after))

	switch {
	case replaced && !strings.Contains(string(current), "disk-new"):
		t.Errorf("read-only-document: %s was replaced by %d bytes that hold neither the old nor the new document", raceDocName, len(current))
	case !replaced && !strings.Contains(string(current), "disk-keep"):
		t.Errorf("read-only-document: %s is %d bytes holding neither the old nor the new document", raceDocName, len(current))
	}
	switch {
	case replaced && answer.err == nil && answer.status != http.StatusOK:
		t.Errorf("read-only-document: the save answered %s yet %s holds its bytes", answer, raceDocName)
	case !replaced && answer.err == nil && answer.status == http.StatusOK:
		t.Errorf("read-only-document: the save answered 200 but %s still holds its old bytes", raceDocName)
	}
	if replaced {
		held := false
		for _, body := range after {
			if strings.Contains(body, "disk-keep") {
				held = true
				break
			}
		}
		if !held {
			t.Errorf("read-only-document: %s was replaced but no version holds the bytes it replaced", raceDocName)
		}
	}
	for name, body := range before {
		if now, ok := after[name]; !ok {
			t.Errorf("read-only-document: version %s is gone after the save", name)
		} else if now != body {
			t.Errorf("read-only-document: version %s changed after the save: %d bytes now, %d before", name, len(now), len(body))
		}
	}
}

// huntDiskReadOnlyFolder serves a folder the person made read-only.
func huntDiskReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores a folder's read-only mode")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: root ignores directory modes, so a read-only folder cannot be made to refuse a write")
	}
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)
	if answer := raceSendWrite(server.URL, "disk-keep", false); answer.err != nil || answer.status != http.StatusOK {
		t.Fatalf("saving %s before the mode change answered %s", raceDocName, answer)
	}

	docPath := filepath.Join(dir, raceDocName)
	oldDoc := raceRead(t, docPath)
	before := diskVersionBodies(t, dir)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod 0555 %s: %v", dir, err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	answer := raceSendWrite(server.URL, "disk-new", false)
	after := diskVersionBodies(t, dir)
	getStatus, _ := diskGet(server.URL, racePublicHost, "/"+raceDocName)

	t.Logf("HUNT disk subtest=read-only-folder status=%s folder-mode=0555 get=%d versions-before=%d versions-after=%d",
		answer, getStatus, len(before), len(after))

	if answer.err == nil && answer.status == http.StatusOK {
		t.Errorf("read-only-folder: the save answered 200 into a folder the person made read-only: a false success")
	}
	diskDocumentsIntact(t, "read-only-folder", docPath, oldDoc, before, after)

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("restoring the mode of %s: %v", dir, err)
	}
	recovered := raceSendWrite(server.URL, "disk-new", false)
	current, readErr := os.ReadFile(docPath)
	switch {
	case recovered.err != nil || recovered.status != http.StatusOK:
		t.Errorf("read-only-folder: the save after the mode was restored answered %s, want 200", recovered)
	case readErr != nil:
		t.Errorf("read-only-folder: reading %s after the recovery save: %v", docPath, readErr)
	case !strings.Contains(string(current), "disk-new"):
		t.Errorf("read-only-folder: the recovery save answered 200 but %s does not hold its bytes", raceDocName)
	}
	t.Logf("HUNT disk note=read-only-folder recovered-status=%s doc=%d-bytes", recovered, len(current))
}

// diskSave is namesSave with an If-Match stamp, for the conditional save a tab
// sends after its last acknowledged write.
func diskSave(serverURL, documentURL, marker, ifMatch string) namesAnswer {
	request, err := http.NewRequest(http.MethodPost, serverURL+"/_/save", strings.NewReader(raceBrowserBody(marker)))
	if err != nil {
		return namesAnswer{err: err}
	}
	request.Host = racePublicHost
	request.Header.Set("Origin", "http://"+racePublicHost)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", documentURL)
	request.Header.Set("Content-Type", "text/html")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return namesAnswer{err: err}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return namesAnswer{err: readErr}
	}
	return namesAnswer{status: response.StatusCode, body: string(body)}
}

// diskAnswerCode is the `code` field of one answer.
func diskAnswerCode(answer namesAnswer) string {
	var body map[string]any
	if json.Unmarshal([]byte(answer.body), &body) != nil {
		return ""
	}
	code, _ := body["code"].(string)
	return code
}

// huntDiskFolderDeleted deletes the served folder while the host runs and then
// saves a document inside it, once unconditionally and once with the stamp of
// the last acknowledged save. Both must be refused as moved-or-deleted, and the
// folder must stay gone.
func huntDiskFolderDeleted(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	name := "sub/" + raceDocName
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Join(dir, "sub"), err)
	}
	kept := namesSave(server.URL, diskDocumentURL(name), "disk-keep")
	if kept.err != nil || kept.status != http.StatusOK {
		t.Fatalf("saving %s before the delete answered %s", name, kept)
	}
	var ack map[string]any
	if json.Unmarshal([]byte(kept.body), &ack) != nil {
		t.Fatalf("the save before the delete answered %s, want JSON", kept)
	}
	etag, _ := ack["etag"].(string)
	if etag == "" {
		t.Fatalf("the save before the delete answered no etag, so no conditional save can be made: %s", kept)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("deleting the served folder %s: %v", dir, err)
	}
	plain := namesSave(server.URL, diskDocumentURL(name), "disk-new")
	conditional := diskSave(server.URL, diskDocumentURL(name), "disk-newer", etag)

	rootInfo, rootErr := os.Stat(dir)
	docInfo, docErr := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
	rootBack := rootErr == nil && rootInfo.IsDir()
	docBack := docErr == nil && docInfo.Mode().IsRegular()

	t.Logf("HUNT disk subtest=folder-deleted status=%s code=%s conditional=%s conditional-code=%s folder-recreated=%v doc-recreated=%v",
		plain, diskAnswerCode(plain), conditional, diskAnswerCode(conditional), rootBack, docBack)

	for _, answer := range []struct {
		label  string
		answer namesAnswer
	}{{"without If-Match", plain}, {"with If-Match", conditional}} {
		if answer.answer.err != nil {
			t.Errorf("folder-deleted: the save %s never reached the host: %v", answer.label, answer.answer.err)
			continue
		}
		if answer.answer.status != http.StatusConflict {
			t.Errorf("folder-deleted: the save %s answered %s, want %d", answer.label, answer.answer, http.StatusConflict)
			continue
		}
		if code := diskAnswerCode(answer.answer); code != "moved-or-deleted" {
			t.Errorf("folder-deleted: the save %s answered code %q, want moved-or-deleted: %s", answer.label, code, answer.answer)
		}
	}
	if rootErr == nil || !errors.Is(rootErr, os.ErrNotExist) {
		t.Errorf("folder-deleted: the deleted served folder %s exists again after the refused saves (%v)", filepath.Base(dir), rootErr)
	}
}

// diskTwoRoots serves two folders from one host, each on its own listener.
func diskTwoRoots(t *testing.T, first, second string) (*httptest.Server, *httptest.Server) {
	t.Helper()
	host := hostTestHost(t)
	firstHandler, err := host.AddRoot(RootSpec{ID: raceRootID, Path: first, PublicHost: racePublicHost})
	if err != nil {
		t.Fatalf("AddRoot %s: %v", first, err)
	}
	secondHandler, err := host.AddRoot(RootSpec{ID: diskSecondRootID, Path: second, PublicHost: diskSecondPublicHost})
	if err != nil {
		t.Fatalf("AddRoot %s: %v", second, err)
	}
	firstServer := httptest.NewServer(firstHandler)
	t.Cleanup(firstServer.Close)
	secondServer := httptest.NewServer(secondHandler)
	t.Cleanup(secondServer.Close)
	return firstServer, secondServer
}

// huntDiskVolumeGone unmounts the volume a served folder lives on.
func huntDiskVolumeGone(t *testing.T) {
	mount := strings.TrimSpace(os.Getenv("HUNT_LOOPMOUNT"))
	unmount := strings.TrimSpace(os.Getenv("HUNT_UNMOUNT_CMD"))
	if mount == "" || unmount == "" {
		t.Skip("HUNT_LOOPMOUNT and HUNT_UNMOUNT_CMD must both name a mounted file system and a shell command that unmounts it, so the volume-gone lane cannot run")
	}
	if info, err := os.Stat(mount); err != nil || !info.IsDir() {
		t.Skipf("HUNT_LOOPMOUNT %q is not a mounted directory (%v), so the volume-gone lane cannot run", mount, err)
	}

	dir, err := os.MkdirTemp(mount, "hunt-disk-volume-")
	if err != nil {
		t.Fatalf("creating a served folder inside %s: %v", mount, err)
	}
	other := raceTempDir(t)
	if err := os.WriteFile(filepath.Join(other, raceDocName), []byte(raceBrowserBody("disk-other")), 0o644); err != nil {
		t.Fatalf("seeding the second folder: %v", err)
	}

	server, otherServer := diskTwoRoots(t, dir, other)
	if answer := raceSendWrite(server.URL, "disk-before", false); answer.err != nil || answer.status != http.StatusOK {
		t.Fatalf("saving into the mounted folder answered %s", answer)
	}

	command := exec.Command("/bin/sh", "-c", unmount)
	output, unmountErr := command.CombinedOutput()

	answer := raceSendWrite(server.URL, "disk-after", false)
	getStatus, getBody := diskGet(otherServer.URL, diskSecondPublicHost, "/"+raceDocName)

	t.Logf("HUNT disk subtest=volume-gone unmount-err=%v unmount-output=%q status=%s other-root=%d",
		unmountErr, strings.TrimSpace(string(output)), answer, getStatus)

	if answer.err == nil && answer.status == http.StatusOK {
		t.Errorf("volume-gone: the save answered 200 after the volume was unmounted: a false success")
	}
	if getStatus != http.StatusOK {
		t.Errorf("volume-gone: the host answers %d for another root after the unmount (%s)", getStatus, getBody)
	} else if !strings.Contains(getBody, "disk-other") {
		t.Errorf("volume-gone: the host serves another root without its bytes (%d bytes)", len(getBody))
	}
}

func TestHuntDiskTrouble(t *testing.T) {
	t.Run("full-disk", huntDiskFull)
	t.Run("read-only-document", huntDiskReadOnlyDocument)
	t.Run("read-only-folder", huntDiskReadOnlyFolder)
	t.Run("folder-deleted", huntDiskFolderDeleted)
	t.Run("volume-gone", huntDiskVolumeGone)
}
