package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/agentlane"
	"github.com/panphora/htmlclay/internal/appcore"
)

const (
	writersStore     = "notes"
	writersPageText  = "the page's edit"
	writersFirstText = "the original"
)

// writersDocument is a served document with one paragraph and one clay.live
// store, which is what gives the document a live room.
func writersDocument(paragraph, text string) string {
	return "<!doctype html>\n<html><head><title>Notes</title></head><body><p>" + paragraph + "</p>\n" +
		`<script type="application/json" merge="` + writersStore + `" clay="live">{"text":"` + text + `"}</script>` +
		"\n</body></html>\n"
}

// writersRoom is what the live room service answers a tab that reads its room.
type writersRoom struct {
	Epoch  string `json:"epoch"`
	Seq    int    `json:"seq"`
	Conn   string `json:"conn"`
	Stores map[string]struct {
		Value struct {
			Text string `json:"text"`
		} `json:"value"`
		Landed int `json:"landed"`
		Last   int `json:"last"`
	} `json:"stores"`
}

func (r writersRoom) text() string { return r.Stores[writersStore].Value.Text }

// unsaved reports whether the room holds a change its file does not.
func (r writersRoom) unsaved() bool {
	store := r.Stores[writersStore]
	return store.Last != store.Landed
}

// writersFrameAnswer is the service's answer to one frame a tab posts.
type writersFrameAnswer struct {
	Seq   int    `json:"seq"`
	Epoch string `json:"epoch"`
	Stale bool   `json:"stale"`
	Code  string `json:"code"`
}

// writersTab is one tab of a served document as the live room service sees it,
// with no browser: the stream a tab holds open and the frames it posts.
type writersTab struct {
	server   *httptest.Server
	host     string
	document string
	conn     string
	epoch    string
	events   chan string
	ended    chan struct{}
}

// writersNotice is one notification the service sends a tab on its live stream.
type writersNotice struct {
	Type string `json:"type"`
	Data struct {
		Kind   string `json:"kind"`
		Sender string `json:"sender"`
		Etag   string `json:"etag"`
	} `json:"data"`
}

const writersTabID = "tab-1"

func (tab *writersTab) do(t *testing.T, method, target, body string, headers map[string]string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, tab.server.URL+target, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = tab.host
	request.Header.Set("Document-URL", tab.document)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	return response.StatusCode, string(raw)
}

// openWritersTab joins the document's room the way a tab does: it opens the live
// stream, waits for the service to say the tab is connected, and reads the room.
func openWritersTab(t *testing.T, server *httptest.Server, host, name string) *writersTab {
	t.Helper()
	tab := &writersTab{
		server: server, host: host, document: "http://" + host + "/" + name,
		events: make(chan string, 64), ended: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	query := url.Values{"document-url": {tab.document}, "client-id": {writersTabID}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/_/sync?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = host
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("the live stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("the live stream answered %d, want 200", response.StatusCode)
	}
	connected := make(chan struct{})
	go func() {
		defer close(tab.ended)
		defer response.Body.Close()
		told := false
		lines := bufio.NewScanner(response.Body)
		lines.Buffer(make([]byte, 0, 64<<10), 4<<20)
		for lines.Scan() {
			line := lines.Text()
			if !told && line == ": connected" {
				told = true
				close(connected)
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				select {
				case tab.events <- data:
				default:
				}
			}
		}
	}()
	select {
	case <-connected:
	case <-tab.ended:
		t.Fatal("the live stream ended before the service connected the tab")
	case <-time.After(10 * time.Second):
		t.Fatal("the service did not connect the tab to its live stream")
	}
	room := tab.room(t)
	if room.Conn == "" || room.Epoch == "" {
		t.Fatalf("the room answered a tab on its stream %+v, want a connection and an epoch", room)
	}
	tab.conn, tab.epoch = room.Conn, room.Epoch
	return tab
}

func (tab *writersTab) room(t *testing.T) writersRoom {
	t.Helper()
	status, body := tab.do(t, http.MethodGet, "/_/sync/data?client-id="+writersTabID, "", nil)
	var room writersRoom
	if status != http.StatusOK || json.Unmarshal([]byte(body), &room) != nil {
		t.Fatalf("the room's state = %d %s", status, body)
	}
	return room
}

// hearsNotice waits for the next notification on the tab's stream.
func (tab *writersTab) hearsNotice(t *testing.T) writersNotice {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case data := <-tab.events:
			var notice writersNotice
			if json.Unmarshal([]byte(data), &notice) == nil && notice.Type == "notification" {
				return notice
			}
		case <-tab.ended:
			t.Fatal("the tab's stream ended before it carried a notification")
		case <-deadline:
			t.Fatal("the tab's stream carried no notification")
		}
	}
}

// sendEdit posts the page's one edit as frame 1 of this tab, on the connection
// and for the room epoch named.
func (tab *writersTab) sendEdit(t *testing.T, conn, epoch string) (int, writersFrameAnswer) {
	t.Helper()
	frame, err := json.Marshal(map[string]any{
		"conn": conn, "name": writersStore, "n": 1, "epoch": epoch,
		"ops": []map[string]any{{"op": "set", "path": []string{"text"}, "value": writersPageText}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, body := tab.do(t, http.MethodPost, "/_/sync/data?client-id="+writersTabID, string(frame),
		map[string]string{"Origin": "http://" + tab.host, "Content-Type": "application/json"})
	var answer writersFrameAnswer
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("the frame answered %d %q, which is not JSON: %v", status, body, err)
	}
	return status, answer
}

// writersSaveAnswer is what /_/save answers a page.
type writersSaveAnswer struct {
	Code   string `json:"code"`
	Etag   string `json:"etag"`
	SaveID string `json:"saveId"`
}

// writersSaveRequest is where and how the document's own page saves it, naming
// the stamp the page last saw, as saveWithStamp sends it.
func writersSaveRequest(t *testing.T, a *app, path, ifMatch string) (host, target string, headers map[string]string) {
	t.Helper()
	host, target, extra := saveTarget(t, a, path)
	headers = map[string]string{
		"Sec-Fetch-Site": "same-origin",
		"Origin":         "http://" + host,
		"Content-Type":   "text/html",
		"If-Match":       ifMatch,
	}
	for name, value := range extra {
		headers[name] = value
	}
	return host, target, headers
}

// writersPageSave is one save by the document's own page, with its answer.
func writersPageSave(t *testing.T, a *app, path, body, ifMatch string) (int, writersSaveAnswer) {
	t.Helper()
	host, target, headers := writersSaveRequest(t, a, path, ifMatch)
	rec := servedRequest(t, a, http.MethodPost, host, target, body, headers)
	var answer writersSaveAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the page's save answered %d %q, which is not JSON: %v", rec.Code, rec.Body.String(), err)
	}
	return rec.Code, answer
}

// writersHistory is what a folder keeps for one document besides the file: the
// bytes of every version kept for it, and of every recovered copy of edits a
// room had not saved.
type writersHistory struct {
	kept      []string
	recovered []string
}

// writersKept answers the history of the document at path. It is the lane's list
// checked against the folder's own history, so bytes kept where the lane does not
// list them fail the test.
func writersKept(t *testing.T, a *app, dir, path string) writersHistory {
	t.Helper()
	listed := map[string]string{}
	var history writersHistory
	for _, version := range agentLaneVersionsOf(t, a, path).Versions {
		html := agentLaneVersioned(t, a, path, version.ID)
		listed[version.ID] = html
		if version.Kind == "recovered" {
			history.recovered = append(history.recovered, html)
		} else {
			history.kept = append(history.kept, html)
		}
	}
	onDisk := 0
	recoveredDir := filepath.Join(dir, ".hyperclay", "recovered") + string(filepath.Separator)
	err := filepath.WalkDir(filepath.Join(dir, ".hyperclay"), func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(file, ".html") {
			return err
		}
		onDisk++
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		// The lane names a recovered copy by its file name after "recovered-".
		id := filepath.Base(file)
		if strings.HasPrefix(file, recoveredDir) {
			id = "recovered-" + id
		}
		if held, ok := listed[id]; !ok || held != string(data) {
			t.Errorf("%s keeps %q, which the lane does not list", file, data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the kept versions: %v", err)
	}
	if onDisk != len(listed) {
		t.Fatalf("the lane lists %d versions and the folder keeps %d", len(listed), onDisk)
	}
	slices.Sort(history.kept)
	slices.Sort(history.recovered)
	return history
}

func wantWritersKept(t *testing.T, when string, kept []string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if !slices.Equal(kept, want) {
		t.Fatalf("%s the kept versions are %q, want %q", when, kept, want)
	}
}

// writersRecoveredStamp is what a recovered copy adds to the store it wrote a
// room's value into: the room, the room's change the value is as of, and a hash
// of the value.
var writersRecoveredStamp = regexp.MustCompile(` live-at="([^":]*):(\d+):[0-9a-f]+"`)

// wantWritersRecovered fails unless recovered is one copy that holds want, with
// its store stamped as the first change of the room named.
func wantWritersRecovered(t *testing.T, when string, recovered []string, epoch, want string) {
	t.Helper()
	if len(recovered) != 1 {
		t.Fatalf("%s the recovered copies are %q, want one", when, recovered)
	}
	stamp := writersRecoveredStamp.FindStringSubmatch(recovered[0])
	if stamp == nil || stamp[1] != epoch || stamp[2] != "1" {
		t.Fatalf("%s the recovered copy is %q, want its store stamped as change 1 of room %s", when, recovered[0], epoch)
	}
	if got := writersRecoveredStamp.ReplaceAllString(recovered[0], ""); got != want {
		t.Fatalf("%s the recovered copy holds %q, want %q", when, got, want)
	}
}

// restartWritersService kills the live room service and waits for the host to
// start it again, which is the one restart no unsaved-edits question guards.
func restartWritersService(t *testing.T, a *app) {
	t.Helper()
	before := a.service.process()
	if before == nil {
		t.Fatal("the live room service has no process to restart")
	}
	before.Kill()
	deadline := time.Now().Add(20 * time.Second)
	for {
		after := a.service.process()
		if after != nil && after != before && a.service.state() == appcore.ServiceRunning {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the live room service did not come back: state %q", a.service.state())
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// writersSteps is the host's other work while a request waits for the person.
// Each step runs off the test's goroutine, so a host that stopped answering fails
// that step by name instead of hanging the run, and the steps after it still say
// whether they were stopped too. A step asserts nothing itself.
type writersSteps struct {
	running sync.WaitGroup
}

// within reports whether step returned in time. What a step that did not return
// would have written must not be read.
func (s *writersSteps) within(t *testing.T, what string, step func()) bool {
	t.Helper()
	done := make(chan struct{})
	s.running.Add(1)
	go func() {
		defer s.running.Done()
		defer close(done)
		step()
	}()
	select {
	case <-done:
		return true
	case <-time.After(5 * time.Second):
		t.Errorf("%s did not answer while the request waited for the person", what)
		return false
	}
}

// drain gives the steps a stopped host held back time to finish once the host
// answers again, so none of them writes into a folder the test is removing.
func (s *writersSteps) drain() {
	done := make(chan struct{})
	go func() {
		s.running.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// TestM3WritersMeet is gate 7.48: one trace of the milestone's writers meeting on
// the real host. A page's edit in a live room meets an agent's conditional save
// and a restart of the live room service; a rename meets a version restore; and a
// request waiting for the person meets the lane, the snapshot and a page's save.
func TestM3WritersMeet(t *testing.T) {
	t.Run("a live room, an agent and a restart", func(t *testing.T) {
		serviceRepoTarget(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		path := filepath.Join(dir, "index.html")
		original := writersDocument("first", writersFirstText)
		agents := writersDocument("the agent's save", writersFirstText)
		pages := writersDocument("first", writersPageText)
		both := writersDocument("the agent's save", writersPageText)
		writeTestFile(t, path, original)

		a := newServiceTestApp(t, home, t.TempDir())
		a.rt.agentToken, a.rt.agentTokenFile = testAgentToken, testAgentTokenFile
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		server := httptest.NewServer(http.HandlerFunc(a.serveHost))
		t.Cleanup(server.Close)
		host := servedHostName("notes", servedTestPort)

		tab := openWritersTab(t, server, host, "index.html")
		if status, answer := tab.sendEdit(t, tab.conn, tab.epoch); status != http.StatusOK || answer.Seq != 1 || answer.Epoch != tab.epoch {
			t.Fatalf("the page's edit = %d %+v, want it taken as the room's first change", status, answer)
		}
		if room := tab.room(t); room.text() != writersPageText || !room.unsaved() {
			t.Fatalf("the room holds %+v, want the page's edit, unsaved", room)
		}
		if got := agentLaneFileBytes(t, path); got != original {
			t.Fatalf("the file holds %q while the edit is only in the room, want the original", got)
		}
		if _, err := os.Stat(filepath.Join(dir, ".hyperclay", "recovered")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("the folder holds a recovered copy before any save replaced the document: %v", err)
		}

		// From the agent's save on, the folder keeps the versions named and one
		// recovered copy: the page's edit in the document the page loaded.
		wantHistory := func(when string, kept ...string) {
			t.Helper()
			history := writersKept(t, a, dir, path)
			wantWritersKept(t, when, history.kept, kept...)
			wantWritersRecovered(t, when, history.recovered, tab.epoch, pages)
		}

		read, _ := agentLaneGet(t, a, path)
		if read.HTML != original {
			t.Fatalf("the agent read %q, want the original", read.HTML)
		}
		saved := wantAgentLaneWrite(t, agentLaneSave(t, a, path, agents, "agent-1", read.ETag), "agent-1")
		if got := agentLaneVersioned(t, a, path, *saved.UndoVersionID); got != original {
			t.Fatalf("the agent's undo version holds %q, want the bytes it replaced %q", got, original)
		}
		if got := agentLaneFileBytes(t, path); got != agents {
			t.Fatalf("the file holds %q, want the agent's save", got)
		}
		if notice := tab.hearsNotice(t); notice.Data.Kind != "external-change" || notice.Data.Sender != "data-api" || notice.Data.Etag != saved.ETag {
			t.Fatalf("the page heard %+v, want the agent's save and its stamp %s", notice, saved.ETag)
		}
		if room := tab.room(t); room.text() != writersPageText || !room.unsaved() || room.Epoch != tab.epoch {
			t.Fatalf("after the agent's save the room holds %+v, want the page's edit still unsaved in the same room", room)
		}
		wantHistory("after the agent's save", original, agents)

		restartWritersService(t, a)

		select {
		case <-tab.ended:
		case <-time.After(10 * time.Second):
			t.Fatal("the restart did not end the tab's live stream, so nothing told the page its room is gone")
		}
		if got := agentLaneFileBytes(t, path); got != agents {
			t.Fatalf("after the restart the file holds %q, want the agent's save", got)
		}
		wantHistory("after the restart", original, agents)

		// The page's edit is now in no file and no version: the recovered copy is
		// where the person can get it back. What is left to check is that every way
		// the page can offer it again is answered with a refusal the page can act on.
		if status, answer := tab.sendEdit(t, tab.conn, tab.epoch); status != http.StatusServiceUnavailable || answer.Code != "not-connected" {
			t.Fatalf("the edit sent again on the lost connection = %d %+v, want 503 not-connected", status, answer)
		}
		back := openWritersTab(t, server, host, "index.html")
		room := back.room(t)
		if back.epoch == tab.epoch || room.text() != writersFirstText || room.unsaved() || room.Seq != 0 {
			t.Fatalf("the room after the restart = %+v, want a new room built from the file, with nothing unsaved", room)
		}
		if status, answer := back.sendEdit(t, back.conn, tab.epoch); status != http.StatusOK || !answer.Stale || answer.Epoch != back.epoch {
			t.Fatalf("the old room's frame sent to the new room = %d %+v, want it refused as stale with the new epoch", status, answer)
		}
		if room := back.room(t); room.text() != writersFirstText || room.unsaved() {
			t.Fatalf("the new room took the old room's frame: %+v", room)
		}
		status, refused := writersPageSave(t, a, path, pages, read.ETag)
		if status != http.StatusPreconditionFailed || refused.Code != "conflict" || refused.Etag != saved.ETag || refused.SaveID != "agent-1" {
			t.Fatalf("the page's save over the stamp it loaded = %d %+v, want 412 conflict naming the agent's save and its stamp %s", status, refused, saved.ETag)
		}
		if got := agentLaneFileBytes(t, path); got != agents {
			t.Fatalf("a refused save changed the file to %q", got)
		}
		wantHistory("after the refusals", original, agents)

		// The refusal names the stamp the stream named, so the page's edit lands on
		// the agent's save and not instead of it.
		status, landed := writersPageSave(t, a, path, both, refused.Etag)
		if status != http.StatusOK || landed.Etag == "" || landed.Etag == saved.ETag {
			t.Fatalf("the page's save over the agent's stamp = %d %+v, want 200 and a new stamp", status, landed)
		}

		// The agent still holds the stamp of its own save, which the page has now
		// replaced.
		late := wantAgentLaneRefusal(t, agentLaneSave(t, a, path, writersDocument("the agent's late save", writersFirstText), "agent-2", saved.ETag),
			http.StatusPreconditionFailed, "conflict")
		if late.Error.ETag != landed.Etag {
			t.Fatalf("the late save's refusal names stamp %q, want the document's %q", late.Error.ETag, landed.Etag)
		}

		if got := agentLaneFileBytes(t, path); got != both {
			t.Fatalf("the file ends as %q, want the agent's save with the page's edit %q", got, both)
		}
		wantHistory("at the end", original, agents, both)
		if room := back.room(t); room.text() != writersPageText || room.unsaved() || room.Epoch != back.epoch {
			t.Fatalf("the room ends as %+v, want the page's edit, read from the file", room)
		}
	})

	t.Run("a rename, then a version restore", func(t *testing.T) {
		serviceRepoTarget(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		oldPath := filepath.Join(dir, "page.html")
		newPath := filepath.Join(dir, "renamed.html")
		first := servedTestDocument
		second := strings.Replace(first, "<p>first</p>", "<p>second</p>", 1)
		writeTestFile(t, oldPath, first)

		a := newServiceTestApp(t, home, t.TempDir())
		a.rt.agentToken, a.rt.agentTokenFile = testAgentToken, testAgentTokenFile
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)

		read, _ := agentLaneGet(t, a, oldPath)
		saved := wantAgentLaneWrite(t, agentLaneSave(t, a, oldPath, second, "before-rename", read.ETag), "before-rename")
		keptBefore := *saved.UndoVersionID
		if got := agentLaneVersioned(t, a, oldPath, keptBefore); got != first {
			t.Fatalf("the version kept before the rename holds %q, want %q", got, first)
		}

		dashboard := openDashboard(t, a)
		if status, body := dashboard.command(dashboard.rootIDOf(), map[string]any{"command": "rename", "path": "page.html", "name": "renamed.html"}); status != http.StatusOK || body != `{"path":"renamed.html"}` {
			t.Fatalf("rename = %d %s", status, body)
		}
		documents := func() []string {
			t.Helper()
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				if !entry.IsDir() {
					names = append(names, entry.Name())
				}
			}
			return names
		}
		if got := documents(); !slices.Equal(got, []string{"renamed.html"}) {
			t.Fatalf("after the rename the folder holds %v, want only renamed.html", got)
		}

		renamed, _ := agentLaneGet(t, a, newPath)
		if renamed.HTML != second || renamed.ETag != saved.ETag {
			t.Fatalf("the renamed document = %q at %s, want the bytes and stamp it had before the rename", renamed.HTML, renamed.ETag)
		}
		if got := agentLaneVersioned(t, a, newPath, keptBefore); got != first {
			t.Fatalf("under its new name the version kept before the rename holds %q, want %q", got, first)
		}

		// An agent that still addresses the old name is refused, and the refusal
		// creates nothing.
		wantAgentLaneRefusal(t, agentLaneRestore(t, a, oldPath, keptBefore, saved.ETag), http.StatusNotFound, "not-found")
		if got := documents(); !slices.Equal(got, []string{"renamed.html"}) {
			t.Fatalf("a restore addressed to the old name left %v, want only renamed.html", got)
		}

		answer := agentLaneRestore(t, a, newPath, keptBefore, renamed.ETag)
		if got := documents(); !slices.Equal(got, []string{"renamed.html"}) {
			t.Fatalf("after the restore the folder holds %v, want only renamed.html", got)
		}
		restored := wantAgentLaneWrite(t, answer, "r-"+keptBefore)
		if got := agentLaneFileBytes(t, newPath); got != first {
			t.Fatalf("the renamed document holds %q after the restore, want the restored version %q", got, first)
		}
		if got := agentLaneVersioned(t, a, newPath, *restored.UndoVersionID); got != second {
			t.Fatalf("the restore's undo version holds %q, want the bytes it replaced %q", got, second)
		}
	})

	t.Run("a grant waiting while the host stays usable", func(t *testing.T) {
		stand := servedProgramsOpenAs(t, true)
		var steps writersSteps
		t.Cleanup(steps.drain)
		stand.shellApp(t)
		a := stand.a
		a.rt.agentToken, a.rt.agentTokenFile = testAgentToken, testAgentTokenFile
		a.rt.prepareHelperProgram = func(string) (bool, error) { return true, nil }
		asking := stand.document(t)
		other := filepath.Join(stand.dir, "other.html")
		writeTestFile(t, other, servedTestDocument)
		stand.loadPage(t)
		_, loaded := readDocumentAccess(t, a, asking)
		pageHost, pageTarget, pageHeaders := writersSaveRequest(t, a, asking, loaded)

		response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		if sent := stand.pageRequest(t, "r1"); sent.StatusCode != http.StatusOK {
			t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		page.waitFor(t, "wire/ack", "r1")
		setupDone := stand.setupAsync()

		// The registry is asked directly, and never the snapshot, so a host the
		// waiting request has stopped fails the steps below that it stopped.
		var waiting string
		waitFor(t, "the program question to wait for the person", func() bool {
			for _, pending := range a.registry().Pending() {
				if pending.Ask.Kind == appcore.RequestFound {
					waiting = pending.ID
					return true
				}
			}
			return false
		})

		otherID := agentlane.DocumentID(other)
		agents := strings.Replace(servedTestDocument, "<p>first</p>", "<p>saved while the request waited</p>", 1)
		saveBody := agentLaneSaveBody(t, agents, "meanwhile")
		var read, saved *httptest.ResponseRecorder
		if steps.within(t, "the agent lane's read of another document", func() {
			read = agentLaneRequest(t, a, http.MethodGet, "/documents/"+otherID, nil)
		}) {
			var document agentLaneDocument
			if read.Code != http.StatusOK || json.Unmarshal(read.Body.Bytes(), &document) != nil || document.HTML != servedTestDocument {
				t.Fatalf("the agent lane's read = %d %s", read.Code, read.Body.String())
			}
			if steps.within(t, "the agent lane's save of another document", func() {
				saved = agentLaneWrite(t, a, http.MethodPut, "/documents/"+otherID, saveBody, "If-Match", document.ETag)
			}) {
				wantAgentLaneWrite(t, saved, "meanwhile")
				if got := agentLaneFileBytes(t, other); got != agents {
					t.Fatalf("the other document holds %q, want the agent's save", got)
				}
			}
		}

		var snapshot appcore.Snapshot
		var request appcore.Request
		if steps.within(t, "the snapshot", func() { snapshot = a.snapshot() }) {
			at := slices.IndexFunc(snapshot.Requests, func(r appcore.Request) bool { return r.ID == waiting })
			if at < 0 {
				t.Fatalf("the snapshot shows requests %+v, want the one still waiting, %s", snapshot.Requests, waiting)
			}
			request = snapshot.Requests[at]
		}

		pages := strings.Replace(servedProgramsDocument(), "<p>first</p>", "<p>the page saved while its request waited</p>", 1)
		var pageSaved *httptest.ResponseRecorder
		if steps.within(t, "the page's save", func() {
			pageSaved = servedRequest(t, a, http.MethodPost, pageHost, pageTarget, pages, pageHeaders)
		}) {
			if pageSaved.Code != http.StatusOK {
				t.Fatalf("the page's save = %d %s, want 200", pageSaved.Code, pageSaved.Body.String())
			}
			if got := agentLaneFileBytes(t, asking); got != pages {
				t.Fatalf("the asking document holds %q, want the page's save", got)
			}
		}
		if t.Failed() {
			t.FailNow()
		}

		if got := a.registry().Pending(); len(got) != 1 || got[0].ID != waiting {
			t.Fatalf("after the host's other work the pending requests are %+v, want the same one still waiting", got)
		}
		select {
		case answer := <-setupDone:
			t.Fatalf("the setup call returned %s before the person answered", answer)
		default:
		}
		if _, err := os.Stat(stand.marker); err == nil {
			t.Fatal("the program ran before the person answered")
		}

		if err := answerSetup(t, a, "w1", request, helperAnswerAllowDocument, ""); err != nil {
			t.Fatal(err)
		}
		select {
		case answer := <-setupDone:
			if !strings.HasPrefix(answer, "200 ") {
				t.Fatalf("POST /_/helper-setup = %s, want 200", answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the setup flow did not finish after the answer")
		}
		page.waitFor(t, "wire/done", "r1")
		if _, err := os.Stat(stand.marker); err != nil {
			t.Fatalf("the waiting run did not complete after the answer: %v", err)
		}
	})
}
