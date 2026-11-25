package served

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/specwire"
)

// TestM3AgentSave is a program's whole-document save and restore on the served
// owner: the lane a page's save runs, conditional on the version the program
// read, with the replaced bytes kept first.
func TestM3AgentSave(t *testing.T) {
	agentSaveSubtests(t, guardV2NoService)
	t.Run("service-stopped", func(t *testing.T) {
		agentSaveSubtests(t, guardV2Stopped)
	})
	t.Run("a-service-that-answers-admits-and-settles-the-save", agentSaveServiceAttached)
}

func agentSaveSubtests(t *testing.T, stand func(*testing.T) *guardV2Stand) {
	t.Run("a-save-keeps-a-version-that-holds-the-old-bytes", func(t *testing.T) { agentSaveKeeps(t, stand(t)) })
	t.Run("a-pages-next-save-with-the-old-etag-gets-its-stale-answer", func(t *testing.T) { agentSavePageStale(t, stand(t)) })
	t.Run("version-cannot-be-stored-bytes-unchanged", func(t *testing.T) { agentSaveNotKept(t, stand(t)) })
	t.Run("a-stale-if-match-writes-nothing", func(t *testing.T) { agentSaveStale(t, stand(t)) })
	t.Run("a-save-racing-a-browser-save-leaves-one-winner-and-one-stale-answer", func(t *testing.T) { agentSaveRace(t, stand(t)) })
	t.Run("restore-keeps-the-current-bytes-first", func(t *testing.T) { agentSaveRestore(t, stand(t)) })
	t.Run("open-tabs-hear-of-the-save", func(t *testing.T) { agentSaveHeard(t, stand(t)) })
	t.Run("an-unchanged-save-writes-nothing", func(t *testing.T) { agentSaveUnchanged(t, stand(t)) })
	t.Run("a-missing-document-is-not-created", func(t *testing.T) { agentSaveMissing(t, stand(t)) })
	t.Run("too-large-and-not-a-document-are-refused", func(t *testing.T) { agentSaveRefusedBodies(t, stand(t)) })
	t.Run("the-owner-is-resolved-again-under-the-lock", func(t *testing.T) { agentSaveOwner(t, stand(t)) })
	t.Run("read-stamps-what-the-save-lane-compares", func(t *testing.T) { agentSavePageState(t, stand(t)) })
	t.Run("a-removal-by-the-reader-of-that-version-raises-no-data-loss-event", func(t *testing.T) { agentSaveMeantRemoval(t, stand(t)) })
}

func agentSaveRead(t *testing.T, s *guardV2Stand) (string, string) {
	t.Helper()
	html, etag, err := s.host.Read(guardV2RootID, guardV2Name)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return string(html), etag
}

func agentSaveDo(s *guardV2Stand, html, etag, saveID string) (DocumentSaved, error) {
	return s.host.SaveDocument(guardV2RootID, guardV2Name, DocumentSave{HTML: []byte(html), IfMatch: etag, SaveID: saveID})
}

func agentSaveRestoreDo(s *guardV2Stand, version, etag string) (DocumentSaved, error) {
	return s.host.RestoreDocument(guardV2RootID, guardV2Name, version, DocumentSave{IfMatch: etag})
}

// agentSaveVersion is one kept version's bytes, read the way a program reads them.
func agentSaveVersion(t *testing.T, s *guardV2Stand, id string) string {
	t.Helper()
	data, err := s.host.Version(guardV2RootID, guardV2Name, id)
	if err != nil {
		t.Fatalf("Version %q: %v", id, err)
	}
	return string(data)
}

// agentSaveHolding is how many kept versions hold exactly html.
func agentSaveHolding(t *testing.T, s *guardV2Stand, html string) int {
	t.Helper()
	held := 0
	for _, name := range s.versionNames(t) {
		if !IsVersionName(name) {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(s.versionsDir(), name)); err == nil && string(data) == html {
			held++
		}
	}
	return held
}

// agentSavePage is a page's conditional save. It fails no test itself, so a
// goroutine may call it.
func agentSavePage(s *guardV2Stand, html string, headers map[string]string) (int, map[string]any, error) {
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/save", strings.NewReader(html))
	if err != nil {
		return 0, nil, err
	}
	request.Host = guardV2Host
	request.Header.Set("Origin", "http://"+guardV2Host)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", "http://"+guardV2Host+"/"+guardV2Name)
	request.Header.Set("Content-Type", "text/html")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, nil, err
	}
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	return response.StatusCode, body, nil
}

// agentSaveServiceAttached is a served folder whose service answers: the save is
// admitted and settled as the lane's own write, a stale save never reaches the
// service, and the stamp answered is the stamp of the bytes the service had the
// lane write.
func agentSaveServiceAttached(t *testing.T) {
	consultShortAborts(t)
	original := guardV2Doc("List", "first note", "alpha", "beta")
	rebased := guardV2Doc("Rebased by the service", "first note", "alpha", "beta", "gamma")
	rebase := false
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path != "/v1/admit" {
			return nil
		}
		admitted := body["incoming"]
		if rebase {
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
	_, etag := agentSaveRead(t, s)

	second := guardV2Doc("List by the agent", "first note", "alpha", "beta")
	saved, err := agentSaveDo(s, second, etag, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if got := s.disk(t); got != second {
		t.Fatalf("the file = %q, want the saved document", got)
	}
	if saved.Etag != specwire.Etag([]byte(second)) || saved.Replaced == "" || agentSaveVersion(t, s, saved.Replaced) != original {
		t.Fatalf("saved = %+v, want the stamp of the written bytes and a version holding the replaced ones", saved)
	}
	if admits, settles := script.count("/v1/admit"), script.count("/v1/settle"); admits != 1 || settles != 1 {
		t.Fatalf("the save made %d admits and %d settles, want 1 and 1", admits, settles)
	}

	_, err = agentSaveDo(s, guardV2Doc("Late", "first note", "alpha", "beta"), etag, "")
	if got := s.disk(t); got != second {
		t.Fatalf("a save with the stamp of an older version changed the file: %q", got)
	}
	var stale *StaleError
	if !errors.As(err, &stale) || stale.Etag != saved.Etag {
		t.Fatalf("a save with the stamp of an older version answered %v, want a StaleError carrying %q", err, saved.Etag)
	}
	if admits := script.count("/v1/admit"); admits != 1 {
		t.Fatalf("the stale save reached the service: %d admits, want the 1 from before", admits)
	}

	rebase = true
	again, err := agentSaveDo(s, guardV2Doc("List by the agent again", "first note", "alpha", "beta"), saved.Etag, "")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if got := s.disk(t); got != rebased {
		t.Fatalf("the lane did not write the bytes the service admitted: %q", got)
	}
	if again.Etag != specwire.Etag([]byte(rebased)) || agentSaveVersion(t, s, again.Replaced) != second {
		t.Fatalf("saved = %+v, want the stamp of the bytes the service had written and a version holding the replaced ones", again)
	}
	if _, next := agentSaveRead(t, s); next != again.Etag {
		t.Fatalf("the next read stamps %q, and the save answered %q", next, again.Etag)
	}
}

// agentSaveMeantRemoval is why the save is the lane's program write: a program
// that named the exact version it read meant what it removed, so the guard keeps
// quiet about one item of four.
func agentSaveMeantRemoval(t *testing.T, s *guardV2Stand) {
	s.put(t, guardV2Doc("List", "first note", "alpha", "beta", "gamma", "delta"))
	_, etag := agentSaveRead(t, s)
	trimmed := guardV2Doc("List", "first note", "alpha", "gamma", "delta")
	if _, err := agentSaveDo(s, trimmed, etag, ""); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if got := s.disk(t); got != trimmed {
		t.Fatalf("the file = %q, want the saved document", got)
	}
	if event := s.event(t); event != nil {
		t.Fatalf("the guard raised %v for an item the save's own reader removed", event)
	}
}

func agentSaveKeeps(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha", "beta")
	s.put(t, original)
	read, etag := agentSaveRead(t, s)
	if read != original {
		t.Fatalf("Read = %q, want the file", read)
	}
	frames := len(s.root.control.events.heldFrames())

	second := guardV2Doc("List by the agent", "first note", "alpha", "beta", "gamma")
	saved, err := agentSaveDo(s, second, etag, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if got := s.disk(t); got != second {
		t.Fatalf("the file = %q, want the saved document", got)
	}
	if !saved.Changed || saved.Etag != specwire.Etag([]byte(second)) {
		t.Fatalf("saved = %+v, want changed and the stamp of the written bytes %q", saved, specwire.Etag([]byte(second)))
	}
	if saved.Replaced == "" {
		t.Fatalf("the save named no version for the bytes it replaced; versions: %v", s.versionNames(t))
	}
	if got := agentSaveVersion(t, s, saved.Replaced); got != original {
		t.Fatalf("version %s holds %q, want the bytes the save replaced", saved.Replaced, got)
	}
	rows, err := s.host.Versions(guardV2RootID, guardV2Name, "", 50)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if !slices.ContainsFunc(rows, func(row VersionRow) bool { return row.ID == saved.Replaced }) {
		t.Fatalf("the versions list %v does not name %s", rows, saved.Replaced)
	}

	committed := false
	for _, frame := range s.root.control.events.heldFrames()[frames:] {
		if strings.Contains(frame, `"type":"commit"`) && strings.Contains(frame, `"kind":"data-api"`) &&
			strings.Contains(frame, `"rel":"`+guardV2Name+`"`) && strings.Contains(frame, `"outcome":"committed"`) {
			committed = true
		}
	}
	if !committed {
		t.Fatalf("the save wrote %s with no commit frame of the save lane: %v", guardV2Name, s.root.control.events.heldFrames()[frames:])
	}

	// The bytes this next save replaces are already a recent version (the first
	// save kept what it wrote), so that version is the undo and no copy is added.
	third := guardV2Doc("List by the agent, again", "first note", "alpha", "beta", "gamma")
	again, err := agentSaveDo(s, third, saved.Etag, "agent-save-2")
	if err != nil {
		t.Fatalf("the second SaveDocument: %v", err)
	}
	if again.Replaced == "" || again.Replaced == saved.Replaced {
		t.Fatalf("the second save named %q for the bytes it replaced (the first named %q)", again.Replaced, saved.Replaced)
	}
	if got := agentSaveVersion(t, s, again.Replaced); got != second {
		t.Fatalf("version %s holds %q, want the bytes the second save replaced", again.Replaced, got)
	}
	if held := agentSaveHolding(t, s, second); held != 1 {
		t.Fatalf("%d versions hold the bytes the second save replaced, want the one already kept: %v", held, s.versionNames(t))
	}
}

func agentSavePageStale(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha")
	s.put(t, original)
	_, loaded := agentSaveRead(t, s)

	fromAgent := guardV2Doc("List by the agent", "first note", "alpha", "beta")
	saved, err := agentSaveDo(s, fromAgent, loaded, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	status, body, err := agentSavePage(s, guardV2Doc("List by the page", "first note", "alpha"), map[string]string{"If-Match": loaded})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusPreconditionFailed || body["code"] != "conflict" {
		t.Fatalf("the page's save with the stamp it loaded answered %d %v, want 412 conflict", status, body)
	}
	if body["etag"] != saved.Etag {
		t.Fatalf("the stale answer's etag = %v, want the stamp the agent's save answered %q", body["etag"], saved.Etag)
	}
	if body["saveId"] != "agent-save-1" {
		t.Fatalf("the stale answer's saveId = %v, want the id of the save that moved the document", body["saveId"])
	}
	if _, named := body["changedBy"]; named {
		t.Fatalf("the stale answer says changedBy %v, and no tab made the agent's save", body["changedBy"])
	}
	if got := s.disk(t); got != fromAgent {
		t.Fatalf("the refused page save changed the file: %q", got)
	}

	status, body, err = agentSavePage(s, guardV2Doc("List by the page", "first note", "alpha", "beta"), map[string]string{"If-Match": saved.Etag})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("the page's save with the stamp the agent's save answered got %d %v, want 200", status, body)
	}
}

func agentSaveNotKept(t *testing.T, s *guardV2Stand) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only versions folder")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores a folder's read-only mode")
	}
	original := guardV2Doc("List", "first note", "alpha", "beta")
	s.put(t, original)
	_, etag := agentSaveRead(t, s)
	if err := os.MkdirAll(s.versionsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.versionsDir(), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.versionsDir(), 0o755) })

	saved, err := agentSaveDo(s, guardV2Doc("List by the agent", "first note", "alpha", "beta"), etag, "")
	if !errors.Is(err, ErrVersionNotKept) {
		t.Fatalf("a save whose replaced version cannot be stored answered %+v, %v, want ErrVersionNotKept", saved, err)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the refused save changed the file: %q", got)
	}
	if names := s.versionNames(t); len(names) != 0 {
		t.Fatalf("the refused save left %v in the versions folder", names)
	}
}

func agentSaveStale(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha")
	s.put(t, original)
	_, old := agentSaveRead(t, s)
	moved := guardV2Doc("List", "a note typed since", "alpha", "beta")
	s.pageSave(t, moved)
	versions := s.versionNames(t)

	// The stale write removes nothing, so only the stamp can refuse it.
	incoming := guardV2Doc("List by the agent", "a note typed since", "alpha", "beta", "gamma")
	for _, stamp := range []struct{ name, value string }{
		{"the stamp of an older version", old},
		{"no stamp", ""},
		{"a wildcard", "*"},
		{"a weak stamp", "W/" + specwire.Etag([]byte(moved))},
		{"a list of stamps", old + ", " + specwire.Etag([]byte(moved))},
	} {
		saved, err := agentSaveDo(s, incoming, stamp.value, "agent-save-1")
		if got := s.disk(t); got != moved {
			t.Fatalf("a save with %s changed the file: %q", stamp.name, got)
		}
		var stale *StaleError
		if !errors.As(err, &stale) {
			t.Fatalf("a save with %s answered %+v, %v, want a StaleError", stamp.name, saved, err)
		}
		if stale.Etag != specwire.Etag([]byte(moved)) {
			t.Fatalf("the stale answer for %s carries %q, want the document's stamp %q", stamp.name, stale.Etag, specwire.Etag([]byte(moved)))
		}
	}
	if got := s.versionNames(t); !slices.Equal(got, versions) {
		t.Fatalf("the stale saves changed the versions folder: %v, was %v", got, versions)
	}

	// A late repeat of the program's own save reads its own id in the answer.
	first, err := agentSaveDo(s, incoming, specwire.Etag([]byte(moved)), "agent-save-2")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	_, err = agentSaveDo(s, incoming+"<!-- later -->", specwire.Etag([]byte(moved)), "agent-save-2")
	var stale *StaleError
	if !errors.As(err, &stale) || stale.SaveID != "agent-save-2" || stale.Etag != first.Etag {
		t.Fatalf("the repeat answered %v (%+v), want a StaleError naming save agent-save-2 and stamp %q", err, stale, first.Etag)
	}
}

func agentSaveRace(t *testing.T, s *guardV2Stand) {
	agentWon, pageWon := 0, 0
	for round := 0; round < 16; round++ {
		base := guardV2Doc(fmt.Sprintf("Round %d", round), "first note", "alpha")
		s.put(t, base)
		_, etag := agentSaveRead(t, s)
		fromAgent := guardV2Doc(fmt.Sprintf("Round %d by the agent", round), "first note", "alpha")
		fromPage := guardV2Doc(fmt.Sprintf("Round %d by the page", round), "first note", "alpha")

		var (
			wait       sync.WaitGroup
			agentErr   error
			pageStatus int
			pageBody   map[string]any
			pageErr    error
		)
		start := make(chan struct{})
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			time.Sleep(time.Duration(round%4) * 150 * time.Microsecond)
			_, agentErr = agentSaveDo(s, fromAgent, etag, "")
		}()
		go func() {
			defer wait.Done()
			<-start
			pageStatus, pageBody, pageErr = agentSavePage(s, fromPage, map[string]string{"If-Match": etag})
		}()
		close(start)
		wait.Wait()
		if pageErr != nil {
			t.Fatal(pageErr)
		}

		disk := s.disk(t)
		var stale *StaleError
		switch {
		case agentErr == nil && pageStatus == http.StatusPreconditionFailed:
			agentWon++
			if disk != fromAgent {
				t.Fatalf("round %d: the agent's save won and the file is %q", round, disk)
			}
			if pageBody["etag"] != specwire.Etag([]byte(fromAgent)) {
				t.Fatalf("round %d: the page's stale answer carries %v, want the winner's stamp", round, pageBody["etag"])
			}
		case errors.As(agentErr, &stale) && pageStatus == http.StatusOK:
			pageWon++
			if disk != fromPage {
				t.Fatalf("round %d: the page's save won and the file is %q", round, disk)
			}
			if stale.Etag != specwire.Etag([]byte(fromPage)) {
				t.Fatalf("round %d: the agent's stale answer carries %q, want the winner's stamp", round, stale.Etag)
			}
		default:
			t.Fatalf("round %d: the agent's save answered %v and the page's %d %v, want one winner and one stale answer; the file is %q",
				round, agentErr, pageStatus, pageBody, disk)
		}
	}
	t.Logf("the agent's save won %d rounds and the page's %d", agentWon, pageWon)
}

func agentSaveRestore(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha", "beta")
	s.put(t, original)
	_, etag := agentSaveRead(t, s)
	second := guardV2Doc("List by the agent", "second note", "alpha")
	saved, err := agentSaveDo(s, second, etag, "")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	stale, err := agentSaveRestoreDo(s, saved.Replaced, etag)
	var refused *StaleError
	if !errors.As(err, &refused) || refused.Etag != saved.Etag {
		t.Fatalf("a restore with an old stamp answered %+v, %v, want a StaleError carrying %q", stale, err, saved.Etag)
	}
	if got := s.disk(t); got != second {
		t.Fatalf("the refused restore changed the file: %q", got)
	}

	if _, err := agentSaveRestoreDo(s, "2026-01-01-00-00-00-000+0000.html", saved.Etag); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a restore of a version that is not kept answered %v, want fs.ErrNotExist", err)
	}
	if got := s.disk(t); got != second {
		t.Fatalf("the restore of a missing version changed the file: %q", got)
	}

	restored, err := agentSaveRestoreDo(s, saved.Replaced, saved.Etag)
	if err != nil {
		t.Fatalf("RestoreDocument: %v", err)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the file after the restore = %q, want the version's bytes", got)
	}
	if !restored.Changed || restored.Etag != specwire.Etag([]byte(original)) {
		t.Fatalf("restored = %+v, want changed and the stamp of the restored bytes", restored)
	}
	if restored.Replaced == "" {
		t.Fatalf("the restore named no version for the bytes it replaced; versions: %v", s.versionNames(t))
	}
	if got := agentSaveVersion(t, s, restored.Replaced); got != second {
		t.Fatalf("version %s holds %q, want the bytes the restore replaced", restored.Replaced, got)
	}
}

// agentSaveSink is a tab's stream, as the live hub writes to one.
type agentSaveSink struct {
	mu     sync.Mutex
	frames []string
}

func (k *agentSaveSink) write(message string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.frames = append(k.frames, message)
	return nil
}

func (k *agentSaveSink) heard(parts ...string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return agentSaveFrame(k.frames, parts...)
}

func agentSaveFrame(frames []string, parts ...string) bool {
	return slices.ContainsFunc(frames, func(frame string) bool {
		for _, part := range parts {
			if !strings.Contains(frame, part) {
				return false
			}
		}
		return true
	})
}

// agentSaveHeard is the reason the save runs as the lane's write with no tab
// behind it: a tab that is editing the document is told, with the new bytes,
// where a page's own save tells only the tabs that view it.
func agentSaveHeard(t *testing.T, s *guardV2Stand) {
	s.put(t, guardV2Doc("List", "first note", "alpha"))
	_, etag := agentSaveRead(t, s)
	editing := &agentSaveSink{}
	s.root.live.subscribe(guardV2Name, editing, "live")
	t.Cleanup(func() { s.root.live.unsubscribe(guardV2Name, editing) })
	held := len(s.root.control.events.heldFrames())

	if _, err := agentSaveDo(s, guardV2Doc("Heard by the tabs", "first note", "alpha"), etag, ""); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	// A host with a service link hands the notice to the event stream, which
	// the service relays to the tabs.
	deadline := time.Now().Add(2 * time.Second)
	for {
		relayed := agentSaveFrame(s.root.control.events.heldFrames()[held:], "external-change", "Heard by the tabs")
		if relayed || editing.heard("external-change", "Heard by the tabs") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no tab editing %s was told of the save: stream %v, events %v", guardV2Name, editing.frames, s.root.control.events.heldFrames()[held:])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func agentSaveUnchanged(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha")
	s.put(t, original)
	read, etag := agentSaveRead(t, s)
	frames := len(s.root.control.events.heldFrames())

	saved, err := agentSaveDo(s, read, etag, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if saved.Changed || saved.Etag != etag || saved.Replaced != "" {
		t.Fatalf("a save of the bytes just read answered %+v, want unchanged, the same stamp and no version", saved)
	}
	if names := s.versionNames(t); len(names) != 0 {
		t.Fatalf("a save of the bytes just read kept %v", names)
	}
	if after := s.root.control.events.heldFrames()[frames:]; agentSaveFrame(after, `"type":"commit"`) {
		t.Fatalf("a save of the bytes just read made a write: %v", after)
	}
}

func agentSaveMissing(t *testing.T, s *guardV2Stand) {
	saved, err := agentSaveDo(s, guardV2Doc("New", "note"), specwire.Etag(nil), "")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a save to a document that does not exist answered %+v, %v, want fs.ErrNotExist", saved, err)
	}
	if _, err := os.Stat(s.docPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the save created %s: %v", s.docPath(), err)
	}
}

func agentSaveRefusedBodies(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha")
	s.put(t, original)
	_, etag := agentSaveRead(t, s)

	if _, err := agentSaveDo(s, "<p>a fragment</p>", etag, ""); !errors.Is(err, ErrNotDocument) {
		t.Fatalf("a save of a fragment answered %v, want ErrNotDocument", err)
	}
	large := guardV2Doc("List", strings.Repeat("x", saveMaxBytes), "alpha")
	if _, err := agentSaveDo(s, large, etag, ""); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a save over the page's limit answered %v, want ErrTooLarge", err)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("a refused save changed the file: %q", got)
	}
}

func agentSaveOwner(t *testing.T, s *guardV2Stand) {
	original := guardV2Doc("List", "first note", "alpha")
	s.put(t, original)
	_, etag := agentSaveRead(t, s)
	incoming := guardV2Doc("List by the agent", "first note", "alpha")

	asked := 0
	_, err := s.host.SaveDocument(guardV2RootID, guardV2Name, DocumentSave{
		HTML: []byte(incoming), IfMatch: etag,
		Owned: func() bool {
			asked++
			if s.root.commit.locks.gate.TryLock() {
				s.root.commit.locks.gate.Unlock()
				t.Error("the owner was asked with no document lock held")
			}
			return false
		},
	})
	if !errors.Is(err, ErrNotOwned) || asked != 1 {
		t.Fatalf("a save whose owner changed answered %v after asking %d times, want ErrNotOwned after one", err, asked)
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the save changed a document this folder no longer owns: %q", got)
	}

	// The folder stops being served while the save waits for the lock.
	resolved, err := resolveWritePath(s.root.registry, guardV2Name)
	if err != nil {
		t.Fatal(err)
	}
	unlock := s.root.commit.locks.lock(resolved)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := agentSaveDo(s, incoming, etag, "")
		done <- err
	}()
	<-started
	time.Sleep(150 * time.Millisecond)
	if !s.host.RemoveRoot(guardV2RootID) {
		t.Fatal("RemoveRoot found no root")
	}
	unlock()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotOwned) {
			t.Fatalf("a save that waited while its folder stopped being served answered %v, want ErrNotOwned", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting save never answered")
	}
	if got := s.disk(t); got != original {
		t.Fatalf("the save wrote into a folder that is no longer served: %q", got)
	}
}

// agentSavePageState is a page whose state lives in a sidecar: the lane judges
// the file and that state as one document, so the stamp Read answers has to be
// that document's or every save of what was read is stale.
func agentSavePageState(t *testing.T, s *guardV2Stand) {
	file := saveGoldenPage(saveGoldenPageRev)
	s.put(t, file)
	writeCommitFile(t, s.dir, filepath.Join(".hyperclay", "blockeditor-state", guardV2Name+".json"),
		[]byte(fmt.Sprintf(`{"v":1,"rev":%q,"tag":%s}`, saveGoldenPageRev, mustJSON(saveGoldenYJSTag))))
	logical, err := logicalBytes(s.dir, guardV2Name, []byte(file))
	if err != nil {
		t.Fatal(err)
	}
	if string(logical) == file {
		t.Fatal("the fixture's page state is not part of the document the lane compares")
	}

	read, etag := agentSaveRead(t, s)
	if read != file {
		t.Fatalf("Read = %q, want the file's own bytes", read)
	}
	if etag != specwire.Etag(logical) {
		t.Fatalf("Read stamped %q, want what the save lane compares %q (the file alone stamps %q)", etag, specwire.Etag(logical), specwire.Etag([]byte(file)))
	}

	incoming := strings.Replace(file, "<title>Doc</title>", "<title>Doc by the agent</title>", 1)
	saved, err := agentSaveDo(s, incoming, etag, "")
	if err != nil {
		t.Fatalf("a save with the stamp Read answered: %v", err)
	}
	if got := s.disk(t); got != incoming {
		t.Fatalf("the file = %q, want the saved page", got)
	}
	_, next := agentSaveRead(t, s)
	if next != saved.Etag {
		t.Fatalf("Read after the save stamped %q, and the save answered %q", next, saved.Etag)
	}
	status, body, err := agentSavePage(s, string(logical), map[string]string{"If-Match": next})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("a page's save with the stamp Read answered got %d %v, want 200", status, body)
	}
}
