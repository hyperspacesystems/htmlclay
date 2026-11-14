package served

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/specwire"
)

const (
	guardV2Host   = "guard-v2.htmlclay.localhost:1212"
	guardV2RootID = "guard-v2-root"
	guardV2Name   = "doc.html"
)

// guardV2Doc is a page with a title, a note and a list, read through its own
// rules tag.
func guardV2Doc(title, note string, items ...string) string {
	list := ""
	for _, item := range items {
		list += `<li><span class="name">` + item + `</span></li>`
	}
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Guard</title>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1","note":"p.note","items":["ul.items li",{"name":".name"}]}</script>
</head>
<body>
<h1>` + title + `</h1>
<p class="note">` + note + `</p>
<ul class="items">` + list + `</ul>
</body>
</html>
`
}

// guardV2Stand is one served folder, as the app adds one.
type guardV2Stand struct {
	host   *Host
	server *httptest.Server
	root   *rootServer
	dir    string
}

func guardV2Serve(t *testing.T, host *Host) *guardV2Stand {
	t.Helper()
	dir := raceTempDir(t)
	handler, err := host.AddRoot(RootSpec{ID: guardV2RootID, Path: dir, PublicHost: guardV2Host})
	if err != nil {
		t.Fatalf("AddRoot %s: %v", dir, err)
	}
	root, ok := handler.(*rootServer)
	if !ok {
		t.Fatalf("AddRoot returned %T, want *rootServer", handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &guardV2Stand{host: host, server: server, root: root, dir: dir}
}

// guardV2NoService is a host built with no service at all.
func guardV2NoService(t *testing.T) *guardV2Stand {
	t.Helper()
	return guardV2Serve(t, hostTestHost(t))
}

// guardV2Stopped is a host built with a service link whose service has
// stopped: every write runs Go's native lanes.
func guardV2Stopped(t *testing.T) *guardV2Stand {
	t.Helper()
	host := switchTestHost(t)
	stand := guardV2Serve(t, host)
	host.DetachService()
	if !host.ServiceDetached() {
		t.Fatal("ServiceDetached = false after DetachService")
	}
	return stand
}

func (s *guardV2Stand) docPath() string {
	return filepath.Join(s.dir, guardV2Name)
}

func (s *guardV2Stand) versionsDir() string {
	return filepath.Join(s.dir, ".hyperclay", "versions", "doc")
}

func (s *guardV2Stand) disk(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(s.docPath())
	if err != nil {
		t.Fatalf("reading %s: %v", s.docPath(), err)
	}
	return string(data)
}

func (s *guardV2Stand) put(t *testing.T, html string) {
	t.Helper()
	if err := os.WriteFile(s.docPath(), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
}

type guardV2Answer struct {
	status int
	raw    string
	body   map[string]any
}

func (a guardV2Answer) String() string {
	return fmt.Sprintf("%d %s", a.status, strings.TrimSpace(a.raw))
}

// send makes one request. A page carries what a browser sends from this
// folder's own origin; a program carries no browser header at all.
func (s *guardV2Stand) send(t *testing.T, page bool, method, path, contentType, body string, headers map[string]string) guardV2Answer {
	t.Helper()
	request, err := http.NewRequest(method, s.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = guardV2Host
	if page {
		request.Header.Set("Origin", "http://"+guardV2Host)
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("Document-URL", "http://"+guardV2Host+"/"+guardV2Name)
	}
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
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	answer := guardV2Answer{status: response.StatusCode, raw: string(raw)}
	_ = json.Unmarshal(raw, &answer.body)
	return answer
}

func (s *guardV2Stand) programWrite(t *testing.T, data string, headers map[string]string) guardV2Answer {
	t.Helper()
	return s.send(t, false, http.MethodPost, "/_/api/"+guardV2Name, "application/json", data, headers)
}

func (s *guardV2Stand) pageWrite(t *testing.T, data string) guardV2Answer {
	t.Helper()
	return s.send(t, true, http.MethodPost, "/_/api/"+guardV2Name, "application/json", data, nil)
}

// pageSave is a save a person made in the page.
func (s *guardV2Stand) pageSave(t *testing.T, html string) {
	t.Helper()
	answer := s.send(t, true, http.MethodPost, "/_/save", "text/html", html, map[string]string{"Save-Trigger": "user"})
	if answer.status != http.StatusOK {
		t.Fatalf("the page's save answered %s", answer)
	}
}

// event is the pending data-loss event a page load reads, or nil.
func (s *guardV2Stand) event(t *testing.T) map[string]any {
	t.Helper()
	answer := s.send(t, true, http.MethodGet, "/_/data-loss", "", "", nil)
	if answer.status != http.StatusOK {
		t.Fatalf("GET /_/data-loss answered %s", answer)
	}
	event, _ := answer.body["event"].(map[string]any)
	return event
}

func (s *guardV2Stand) resolve(t *testing.T, id, choice string) guardV2Answer {
	t.Helper()
	body, err := json.Marshal(map[string]string{"id": id, "choice": choice})
	if err != nil {
		t.Fatal(err)
	}
	return s.send(t, true, http.MethodPost, "/_/data-loss", "application/json", string(body), nil)
}

func (s *guardV2Stand) versionNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(s.versionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// versionHoldingText is the kept version whose bytes contain marker, or "".
func (s *guardV2Stand) versionHoldingText(t *testing.T, marker string) string {
	t.Helper()
	for _, name := range s.versionNames(t) {
		if !IsVersionName(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.versionsDir(), name))
		if err == nil && strings.Contains(string(data), marker) {
			return name
		}
	}
	return ""
}

// typedLoss leaves a pending event on the stand: a person types a note, a
// program that read the file before then writes its stale copy over it with a
// new title, and the page's next load raises the loss of the note. It answers
// the event.
func (s *guardV2Stand) typedLoss(t *testing.T) map[string]any {
	t.Helper()
	s.put(t, guardV2Doc("List", "first note", "alpha", "beta"))
	s.pageSave(t, guardV2Doc("List", "the note I typed", "alpha", "beta"))
	s.put(t, guardV2Doc("Renamed by a program", "first note", "alpha", "beta"))
	event := s.event(t)
	if event == nil {
		t.Fatalf("no event after an outside write replaced a note typed a moment ago; guard record: %s", raceRead(t, filepath.Join(s.dir, ".hyperclay", "guard", "doc.json")))
	}
	return event
}

func TestM3ServedGuardV2(t *testing.T) {
	guardV2Subtests(t, guardV2NoService)
	t.Run("service-stopped", func(t *testing.T) {
		guardV2Subtests(t, guardV2Stopped)
	})
	t.Run("service-attached", func(t *testing.T) {
		t.Run("if-match-missing-is-judged-on-the-final-bytes", guardV2ServiceAttached)
	})
	t.Run("trial-root-answers-as-local", guardV2TrialRoot)
}

func guardV2Subtests(t *testing.T, stand func(*testing.T) *guardV2Stand) {
	t.Run("meta-announces-v2-beside-v1", func(t *testing.T) { guardV2Meta(t, stand(t)) })
	t.Run("if-match-missing-exact-and-stale", func(t *testing.T) { guardV2IfMatch(t, stand(t)) })
	t.Run("a-page-is-not-refused", func(t *testing.T) { guardV2Page(t, stand(t)) })
	t.Run("version-cannot-be-kept-bytes-unchanged", func(t *testing.T) { guardV2NotKept(t, stand(t)) })
	t.Run("later-edit-then-put-back-keeps-the-later-edit", func(t *testing.T) { guardV2PutBack(t, stand(t)) })
	t.Run("put-back-goes-through-the-commit-lane", func(t *testing.T) { guardV2PutBackLane(t, stand(t)) })
	t.Run("pinned-version-survives-both-prunes", func(t *testing.T) { guardV2Pins(t, stand(t)) })
}

func guardV2Meta(t *testing.T, s *guardV2Stand) {
	answer := s.send(t, true, http.MethodGet, "/_/meta", "", "", nil)
	if answer.status != http.StatusOK {
		t.Fatalf("GET /_/meta answered %s", answer)
	}
	extensions := []string{}
	list, _ := answer.body["extensions"].([]any)
	for _, name := range list {
		text, _ := name.(string)
		extensions = append(extensions, text)
	}
	v1 := slices.Index(extensions, "data-guard-v1")
	if v1 < 0 || v1+1 >= len(extensions) || extensions[v1+1] != "data-guard-v2" {
		t.Fatalf("extensions = %v, want data-guard-v2 right after data-guard-v1", extensions)
	}
	if !slices.IsSorted(extensions) {
		t.Fatalf("extensions = %v, want sorted", extensions)
	}
}

func guardV2IfMatch(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha", "beta", "gamma")
	s.put(t, original)
	drop := `{"note":"","items":[{"name":"alpha"}]}`

	missing := s.programWrite(t, drop, nil)
	if missing.status != http.StatusPreconditionRequired {
		t.Fatalf("a program's write that removes a note and two items with no If-Match answered %s, want 428", missing)
	}
	if missing.body["code"] != "read-required" || missing.body["error"] != "Read first" ||
		missing.body["message"] != "This write would remove data. Read the document first and send its etag as If-Match." {
		t.Fatalf("the 428 body = %s, want the read-required contract", missing.raw)
	}
	etag, _ := missing.body["etag"].(string)
	if etag != specwire.Etag([]byte(original)) {
		t.Fatalf("the 428 etag = %q, want the document's %q", etag, specwire.Etag([]byte(original)))
	}
	would, _ := missing.body["wouldRemove"].(map[string]any)
	paths, _ := would["paths"].([]any)
	if would["items"] != float64(2) || would["values"] != float64(1) || len(paths) == 0 {
		t.Fatalf("wouldRemove = %v, want two items, one value and its path", would)
	}
	if !strings.HasPrefix(missing.raw, `{"error":"Read first","message":`) || !strings.Contains(missing.raw, `,"code":"read-required","etag":"`+etag+`","wouldRemove":{"items":2,"values":1,"paths":[`) {
		t.Fatalf("the 428 body = %s, want the contract's key order", missing.raw)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the refused write changed the file: %q", got)
	}
	if names := s.versionNames(t); len(names) != 0 {
		t.Fatalf("the refused write kept versions %v, want nothing placed", names)
	}

	stale := s.programWrite(t, drop, map[string]string{"If-Match": `"0000000000000000"`})
	if stale.status != http.StatusPreconditionFailed {
		t.Fatalf("a stale If-Match answered %s, want 412", stale)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the stale write changed the file: %q", got)
	}

	weak := s.programWrite(t, drop, map[string]string{"If-Match": "*"})
	if weak.status != http.StatusPreconditionRequired {
		t.Fatalf("If-Match: * names no version, and answered %s, want 428", weak)
	}

	exact := s.programWrite(t, drop, map[string]string{"If-Match": etag})
	if exact.status != http.StatusOK {
		t.Fatalf("the write that named the exact version it read answered %s, want 200", exact)
	}
	if got := s.disk(t); strings.Contains(got, "beta") || !strings.Contains(got, "alpha") {
		t.Fatalf("the exact write did not land: %q", got)
	}
	if s.versionHoldingText(t, "gamma") == "" {
		t.Fatalf("the version the exact write replaced is not kept: %v", s.versionNames(t))
	}

	keeps := s.programWrite(t, `{"title":"Renamed"}`, nil)
	if keeps.status != http.StatusOK {
		t.Fatalf("a program's write that removes nothing answered %s, want 200 with no If-Match", keeps)
	}
}

func guardV2Page(t *testing.T, s *guardV2Stand) {
	s.put(t, guardV2Doc("List", "first note", "alpha", "beta", "gamma"))
	answer := s.pageWrite(t, `{"items":[{"name":"alpha"},{"name":"beta"}]}`)
	if answer.status != http.StatusOK {
		t.Fatalf("a page's write that removes an item with no If-Match answered %s, want 200", answer)
	}
	if got := s.disk(t); strings.Contains(got, "gamma") {
		t.Fatalf("the page's write did not land: %q", got)
	}

	older := s.send(t, false, http.MethodPost, "/_/api/"+guardV2Name, "application/json", `{"items":[{"name":"alpha"}]}`, map[string]string{"Origin": "http://" + guardV2Host})
	if older.status != http.StatusOK {
		t.Fatalf("a page on a browser that sends Origin and no Sec-Fetch-Site answered %s, want 200", older)
	}
	if got := s.disk(t); strings.Contains(got, "beta") {
		t.Fatalf("the older browser's write did not land: %q", got)
	}
}

func guardV2NotKept(t *testing.T, s *guardV2Stand) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only versions folder")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores a folder's read-only mode")
	}
	original := guardV2Doc("List", "first note", "alpha", "beta", "gamma")
	s.put(t, original)
	if err := os.MkdirAll(s.versionsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.versionsDir(), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.versionsDir(), 0o755) })

	for _, write := range []struct {
		who    string
		answer guardV2Answer
	}{
		{"a program that named the exact version and removes two items", s.programWrite(t, `{"items":[{"name":"alpha"}]}`, map[string]string{"If-Match": specwire.Etag([]byte(original))})},
		{"a page that renames the title", s.pageWrite(t, `{"title":"Renamed"}`)},
	} {
		if write.answer.status != http.StatusConflict || write.answer.body["code"] != "backup-failed" {
			t.Fatalf("%s wrote while the replaced version could not be kept: %s, want 409 backup-failed", write.who, write.answer)
		}
		if write.answer.body["error"] != "Could not keep the current version" ||
			write.answer.body["message"] != "The current version could not be stored in history, so nothing was written. Try again." {
			t.Fatalf("%s got %s, want the contract's wording", write.who, write.answer)
		}
		if got := s.disk(t); got != original {
			t.Fatalf("%s changed the file though its replaced version could not be kept: %q", write.who, got)
		}
	}
	if names := s.versionNames(t); len(names) != 0 {
		t.Fatalf("the refused writes left %v in the versions folder", names)
	}
}

func guardV2PutBack(t *testing.T, s *guardV2Stand) {
	event := s.typedLoss(t)
	if event["reason"] != "recent" {
		t.Fatalf("reason = %v, want recent: %v", event["reason"], event)
	}
	rows, _ := event["typed"].([]any)
	if len(rows) != 1 {
		t.Fatalf("typed = %v, want the one note", event["typed"])
	}
	row, _ := rows[0].(map[string]any)
	if row["yours"] != "the note I typed" || row["path"] == nil || row["at"] == nil {
		t.Fatalf("typed row = %v, want path, yours and at", row)
	}
	if event["canPutBack"] != true {
		t.Fatalf("canPutBack = %v, want true: %v", event["canPutBack"], event)
	}
	for _, key := range []string{"id", "firstDetectedAt", "lastWriteAt", "lossSummary", "fieldCount", "preview", "droppedAdditions", "restorable", "canRevert", "status"} {
		if _, ok := event[key]; !ok {
			t.Fatalf("the v2 event lacks the v1 field %q: %v", key, event)
		}
	}

	s.pageSave(t, guardV2Doc("Renamed by a program", "first note", "alpha, edited later", "beta"))
	pending := s.event(t)
	if pending == nil || pending["id"] != event["id"] {
		t.Fatalf("the later edit cleared the event: %v", pending)
	}

	id, _ := event["id"].(string)
	answer := s.resolve(t, id, "put-back")
	if answer.status != http.StatusOK || answer.body["ok"] != true || answer.body["choice"] != "put-back" || answer.body["status"] != "put-back" {
		t.Fatalf("put-back answered %s", answer)
	}
	got := s.disk(t)
	for _, want := range []string{"the note I typed", "Renamed by a program", "alpha, edited later"} {
		if !strings.Contains(got, want) {
			t.Fatalf("after put-back the file lacks %q: %q", want, got)
		}
	}
	if strings.Contains(got, "first note") {
		t.Fatalf("after put-back the stale note is still there: %q", got)
	}
	if left := s.event(t); left != nil {
		t.Fatalf("the event is still pending after put-back: %v", left)
	}

	unknown := s.resolve(t, id, "put-it-back")
	if unknown.status != http.StatusBadRequest {
		t.Fatalf("an unknown choice answered %s, want 400", unknown)
	}
}

func guardV2PutBackLane(t *testing.T, s *guardV2Stand) {
	event := s.typedLoss(t)
	id, _ := event["id"].(string)
	before := len(s.root.control.events.heldFrames())
	answer := s.resolve(t, id, "put-back")
	if answer.status != http.StatusOK {
		t.Fatalf("put-back answered %s", answer)
	}
	notified := false
	for _, frame := range s.root.control.events.heldFrames()[before:] {
		if strings.Contains(frame, `"type":"commit"`) && strings.Contains(frame, `"kind":"guard-restore"`) &&
			strings.Contains(frame, `"rel":"`+guardV2Name+`"`) && strings.Contains(frame, `"outcome":"committed"`) {
			notified = true
		}
	}
	if !notified {
		t.Fatalf("put-back wrote %s with no commit frame of the guard-restore lane: %v", guardV2Name, s.root.control.events.heldFrames()[before:])
	}
	written := s.disk(t)
	kept := false
	for _, name := range s.versionNames(t) {
		data, err := os.ReadFile(filepath.Join(s.versionsDir(), name))
		if err == nil && string(data) == written {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("the bytes put-back wrote are not a kept version: %v", s.versionNames(t))
	}
}

func guardV2Pins(t *testing.T, s *guardV2Stand) {
	s.pageSave(t, guardV2Doc("List", "first note", "alpha", "beta", "gamma"))
	s.put(t, guardV2Doc("List", "first note", "alpha"))
	if event := s.event(t); event != nil {
		t.Fatalf("an outside edit that took two items nobody typed raised %v, want it quiet and pinned", event)
	}
	pinned := s.versionHoldingText(t, "gamma")
	if pinned == "" {
		t.Fatalf("the version the outside edit replaced is not kept: %v", s.versionNames(t))
	}
	marker := filepath.Join(s.versionsDir(), pinned+pinSuffix)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("no pin beside %s: %v (folder holds %v)", pinned, err, s.versionNames(t))
	}

	listed := s.send(t, true, http.MethodGet, "/_/versions/"+guardV2Name, "", "", nil)
	if listed.status != http.StatusOK {
		t.Fatalf("GET /_/versions answered %s", listed)
	}
	marked := 0
	rows, _ := listed.body["versions"].([]any)
	for _, item := range rows {
		row, _ := item.(map[string]any)
		if row["name"] == pinned {
			if row["pinned"] != true || row["label"] != "Before an outside change" || row["detail"] != "Kept because the next change removed data." {
				t.Fatalf("the pinned row = %v, want pinned and its two labels", row)
			}
			marked++
		} else if _, has := row["pinned"]; has {
			t.Fatalf("the row %v is marked pinned and is not", row)
		}
	}
	if marked != 1 {
		t.Fatalf("the versions list = %s, want %s marked once", listed.raw, pinned)
	}
	history, err := s.host.Versions(guardV2RootID, guardV2Name, "", -1)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range history {
		if row.Pinned != (row.ID == pinned) {
			t.Fatalf("Versions row %+v: Pinned = %v, want %v", row, row.Pinned, row.ID == pinned)
		}
	}

	later := time.Now().Add(400 * 24 * time.Hour)
	newer := func(day, count int) {
		t.Helper()
		for i := 0; i < count; i++ {
			stamp := GenerateTimestamp(time.Now().Add(time.Duration(day)*24*time.Hour + time.Duration(i)*time.Second))
			if err := os.WriteFile(filepath.Join(s.versionsDir(), stamp+".html"), []byte(fmt.Sprintf("newer %d %d", day, i)), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	newer(1, keepNewest+5)

	previousClock := versionClock
	versionClock = func() int64 { return later.UnixMilli() }
	t.Cleanup(func() { versionClock = previousClock })
	countBefore := len(s.versionNames(t))
	maybePrune(s.dir, filepath.ToSlash(s.versionsDir()), nil)
	if countAfter := len(s.versionNames(t)); countAfter >= countBefore {
		t.Fatalf("the ordinary prune removed nothing (%d files before, %d after), so it proved nothing", countBefore, countAfter)
	}
	if _, err := os.Stat(filepath.Join(s.versionsDir(), pinned)); err != nil {
		t.Fatalf("the ordinary prune took the pinned version %s: %v", pinned, err)
	}

	newer(2, 10)
	swept := PruneAllVersions(s.dir, later, nil)
	if swept.Deleted == 0 {
		t.Fatalf("the whole-history prune removed nothing, so it proved nothing: %+v", swept)
	}
	data, err := os.ReadFile(filepath.Join(s.versionsDir(), pinned))
	if err != nil {
		t.Fatalf("the whole-history prune took the pinned version %s: %v", pinned, err)
	}
	if !strings.Contains(string(data), "gamma") {
		t.Fatalf("the pinned version no longer holds the removed data: %q", data)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("pruning took the pin itself: %v", err)
	}
}

// guardV2ServiceAttached is a served folder whose service answers: the lane
// writes the bytes the service admitted, so those are the bytes the refusal is
// judged on, and a refused write ends its consultation with nothing settled.
func guardV2ServiceAttached(t *testing.T) {
	consultShortAborts(t)
	original := guardV2Doc("List", "first note", "alpha", "beta", "gamma")
	rebased := guardV2Doc("Rebased by the service", "first note", "alpha", "beta", "gamma")
	keepAll := false
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path != "/v1/admit" {
			return nil
		}
		admitted := body["incoming"]
		if keepAll {
			admitted = map[string]any{"encoding": "utf8", "data": rebased}
		}
		return map[string]any{"decision": "proceed", "logical": admitted, "file": admitted}
	})
	peer := consultSavePeer(t, script)
	host := switchTestHost(t)
	s := guardV2Serve(t, host)
	if err := host.SetServicePeer(peer.URL, peer.Secret, peer.Epoch); err != nil {
		t.Fatalf("SetServicePeer: %v", err)
	}
	s.put(t, original)
	drop := `{"items":[{"name":"alpha"}]}`

	refused := s.programWrite(t, drop, nil)
	if refused.status != http.StatusPreconditionRequired || refused.body["code"] != "read-required" {
		t.Fatalf("a program's write the service admitted as it came answered %s, want 428 read-required", refused)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the refused write changed the file: %q", got)
	}
	if admits, settles := script.count("/v1/admit"), script.count("/v1/settle"); admits != 1 || settles != 0 {
		t.Fatalf("the refused write made %d admits and %d settles, want 1 and 0", admits, settles)
	}
	if names := s.versionNames(t); len(names) != 0 {
		t.Fatalf("the refused write kept versions %v, want nothing placed", names)
	}

	keepAll = true
	admitted := s.programWrite(t, drop, nil)
	if admitted.status != http.StatusOK {
		t.Fatalf("a write whose admitted bytes remove nothing answered %s, want 200", admitted)
	}
	if got := s.disk(t); !strings.Contains(got, "Rebased by the service") || !strings.Contains(got, "gamma") {
		t.Fatalf("the lane did not write the service's bytes: %q", got)
	}
}

// guardV2TrialRoot is a root served by port, as the trial binary serves one. It
// answers as Hyperclay Local does, which the recorded goldens pin: no 428, and
// no data-guard-v2 in its list.
func guardV2TrialRoot(t *testing.T) {
	host := hostTestHost(t)
	dir := raceTempDir(t)
	handler, err := host.AddRoot(RootSpec{ID: "guard-v2-trial", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	if err := os.WriteFile(filepath.Join(dir, guardV2Name), []byte(guardV2Doc("List", "first note", "alpha", "beta", "gamma")), 0o644); err != nil {
		t.Fatal(err)
	}

	response, err := http.Post(server.URL+"/_/api/"+guardV2Name, "application/json", strings.NewReader(`{"items":[{"name":"alpha"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("a root served by port answered %d %s, want Local's 200", response.StatusCode, raw)
	}

	response, err = http.Get(server.URL + "/_/meta")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(raw), "data-guard-v2") || !strings.Contains(string(raw), "data-guard-v1") {
		t.Fatalf("a root served by port announces %s, want data-guard-v1 without v2", raw)
	}
}
