package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
)

// TestM3AgentSave is a program's whole-document save and restore on the loose
// owner: the function behind /_/save, conditional on the version the program
// read, with the replaced bytes kept first.
func TestM3AgentSave(t *testing.T) {
	t.Run("a-save-keeps-a-version-that-holds-the-old-bytes", agentSaveKeeps)
	t.Run("a-pages-next-save-with-the-old-etag-gets-its-stale-answer", agentSavePageStale)
	t.Run("version-cannot-be-stored-bytes-unchanged", agentSaveNotKept)
	t.Run("a-stale-if-match-writes-nothing", agentSaveStale)
	t.Run("a-save-racing-a-browser-save-leaves-one-winner-and-one-stale-answer", agentSaveRace)
	t.Run("restore-keeps-the-current-bytes-first", agentSaveRestore)
	t.Run("open-tabs-hear-of-the-save", agentSaveHeard)
	t.Run("an-unchanged-save-writes-nothing", agentSaveUnchanged)
	t.Run("a-missing-document-is-not-created", agentSaveMissing)
	t.Run("too-large-and-not-a-document-are-refused", agentSaveRefusedBodies)
	t.Run("the-owner-is-resolved-again-under-the-lock", agentSaveOwner)
	t.Run("read-stamps-what-the-save-lane-compares", agentSaveReadStamp)
	t.Run("a-page-save-is-not-refused-when-its-version-cannot-be-stored", agentSavePageKeepsItsRule)
	t.Run("a-removal-by-the-reader-of-that-version-raises-no-data-loss-event", agentSaveMeantRemoval)
}

func agentSaveDoc(title string, items ...string) string {
	list := ""
	for _, item := range items {
		list += "<li>" + item + "</li>"
	}
	return `<!DOCTYPE html>
<html lang="en"><head><title>Agent</title><script type="application/json" data-rules-name="api" data-rules-version="1">{title:"h1",items:"li[]"}</script></head>
<body>
<h1>` + title + `</h1>
<ul>` + list + `</ul>
</body></html>
`
}

// agentSaveStand is one open loose document holding html, written from outside
// this host so no version of it is kept yet.
func agentSaveStand(t *testing.T, html string) (*Server, *session.File) {
	t.Helper()
	srv, f, _ := setupHandlerTest(t)
	if err := os.WriteFile(f.AbsPath, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	return srv, f
}

func agentSaveRead(t *testing.T, srv *Server, f *session.File) (string, string) {
	t.Helper()
	html, etag, err := srv.ReadDocument(f.AbsPath)
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	return string(html), etag
}

func agentSaveDo(srv *Server, f *session.File, html, etag, saveID string) (DocumentSaved, error) {
	return srv.SaveDocument(f.AbsPath, DocumentSave{HTML: []byte(html), IfMatch: etag, SaveID: saveID})
}

func agentSaveVersion(t *testing.T, srv *Server, f *session.File, id string) string {
	t.Helper()
	data, err := srv.ReadDocumentVersion(f.AbsPath, id)
	if err != nil {
		t.Fatalf("ReadDocumentVersion %q: %v", id, err)
	}
	return string(data)
}

// agentSaveHolding is how many kept versions hold exactly html.
func agentSaveHolding(t *testing.T, srv *Server, f *session.File, html string) int {
	t.Helper()
	held := 0
	for _, name := range listVersionNames(t, srv, f) {
		if agentSaveVersion(t, srv, f, name) == html {
			held++
		}
	}
	return held
}

// agentSavePage is a page's save through the full stack. It fails no test
// itself, so a goroutine may call it.
func agentSavePage(srv *Server, f *session.File, html string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/_/save/"+f.Token, strings.NewReader(html))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	sameOriginHeaders(req)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	return w
}

func agentSaveKeeps(t *testing.T) {
	original := agentSaveDoc("List", "alpha", "beta")
	srv, f := agentSaveStand(t, original)
	read, etag := agentSaveRead(t, srv, f)
	if read != original {
		t.Fatalf("ReadDocument = %q, want the file", read)
	}

	second := agentSaveDoc("List by the agent", "alpha", "beta", "gamma")
	saved, err := agentSaveDo(srv, f, second, etag, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if got := docString(t, f.AbsPath); got != second {
		t.Fatalf("the file = %q, want the saved document", got)
	}
	if !saved.Changed || saved.Etag != specwire.Etag([]byte(second)) {
		t.Fatalf("saved = %+v, want changed and the stamp of the written bytes %q", saved, specwire.Etag([]byte(second)))
	}
	if saved.Replaced == "" {
		t.Fatalf("the save named no version for the bytes it replaced; versions: %v", listVersionNames(t, srv, f))
	}
	if got := agentSaveVersion(t, srv, f, saved.Replaced); got != original {
		t.Fatalf("version %s holds %q, want the bytes the save replaced", saved.Replaced, got)
	}
	if names := listVersionNames(t, srv, f); !slices.Contains(names, saved.Replaced) {
		t.Fatalf("the versions list %v does not name %s", names, saved.Replaced)
	}

	// The bytes this next save replaces are already the newest version (the
	// first save kept what it wrote), so that version is the undo and no copy
	// is added.
	third := agentSaveDoc("List by the agent, again", "alpha", "beta", "gamma")
	again, err := agentSaveDo(srv, f, third, saved.Etag, "agent-save-2")
	if err != nil {
		t.Fatalf("the second SaveDocument: %v", err)
	}
	if again.Replaced == "" || again.Replaced == saved.Replaced {
		t.Fatalf("the second save named %q for the bytes it replaced (the first named %q)", again.Replaced, saved.Replaced)
	}
	if got := agentSaveVersion(t, srv, f, again.Replaced); got != second {
		t.Fatalf("version %s holds %q, want the bytes the second save replaced", again.Replaced, got)
	}
	if held := agentSaveHolding(t, srv, f, second); held != 1 {
		t.Fatalf("%d versions hold the bytes the second save replaced, want the one already kept: %v", held, listVersionNames(t, srv, f))
	}
}

func agentSavePageStale(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	_, loaded := agentSaveRead(t, srv, f)

	fromAgent := agentSaveDoc("List by the agent", "alpha", "beta")
	saved, err := agentSaveDo(srv, f, fromAgent, loaded, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	w := agentSavePage(srv, f, agentSaveDoc("List by the page", "alpha"), map[string]string{"If-Match": loaded})
	body := decode(t, w)
	if w.Code != http.StatusPreconditionFailed || body["code"] != "conflict" {
		t.Fatalf("the page's save with the stamp it loaded answered %d %v, want 412 conflict", w.Code, body)
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
	if got := docString(t, f.AbsPath); got != fromAgent {
		t.Fatalf("the refused page save changed the file: %q", got)
	}

	w = agentSavePage(srv, f, agentSaveDoc("List by the page", "alpha", "beta"), map[string]string{"If-Match": saved.Etag})
	if w.Code != http.StatusOK {
		t.Fatalf("the page's save with the stamp the agent's save answered got %d %s, want 200", w.Code, w.Body.String())
	}
}

// agentSaveHistoryReadOnly makes the version store unable to store anything, the
// way a full or read-only disk does, and reports whether this system enforces it.
func agentSaveHistoryReadOnly(t *testing.T, srv *Server) bool {
	t.Helper()
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		return false
	}
	base := srv.versions.BaseDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	var dirs []string
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, dir := range dirs {
			_ = os.Chmod(dir, 0o755)
		}
	})
	return true
}

// agentSaveMeantRemoval is the guard's half of a program's save: a program that
// named the exact version it read meant what it removed, so the guard keeps quiet
// about an item a person typed, which it would report of a write that showed no
// such proof.
func agentSaveMeantRemoval(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha", "beta", "gamma"))
	typed := agentSaveDoc("List", "alpha", "beta", "gamma", "delta")
	if w := agentSavePage(srv, f, typed, map[string]string{"Save-Trigger": "user"}); w.Code != http.StatusOK {
		t.Fatalf("the page's save answered %d %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()

	_, etag := agentSaveRead(t, srv, f)
	trimmed := agentSaveDoc("List", "alpha", "beta", "gamma")
	if _, err := agentSaveDo(srv, f, trimmed, etag, ""); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if got := docString(t, f.AbsPath); got != trimmed {
		t.Fatalf("the file = %q, want the saved document", got)
	}
	srv.guardQueue.Settle()
	state, err := srv.guardStore.Read(guardKey(t, f), f.AbsPath)
	if err != nil {
		t.Fatalf("guard state: %v", err)
	}
	if state.Event != nil {
		t.Fatalf("the guard raised %+v for an item the save's own reader removed", state.Event)
	}
}

func agentSaveNotKept(t *testing.T) {
	original := agentSaveDoc("List", "alpha", "beta")
	refused := func(t *testing.T, srv *Server, f *session.File, etag string) {
		t.Helper()
		saved, err := agentSaveDo(srv, f, agentSaveDoc("List by the agent", "alpha", "beta"), etag, "")
		if !errors.Is(err, ErrVersionNotKept) {
			t.Fatalf("a save whose replaced version cannot be stored answered %+v, %v, want ErrVersionNotKept", saved, err)
		}
		if got := docString(t, f.AbsPath); got != original {
			t.Fatalf("the refused save changed the file: %q", got)
		}
		if names := listVersionNames(t, srv, f); len(names) != 0 {
			t.Fatalf("the refused save left %v in the history", names)
		}
	}

	srv, f := agentSaveStand(t, original)
	_, etag := agentSaveRead(t, srv, f)
	srv.backupHook = func() error { return errors.New("history is unavailable") }
	refused(t, srv, f, etag)

	srv, f = agentSaveStand(t, original)
	_, etag = agentSaveRead(t, srv, f)
	if agentSaveHistoryReadOnly(t, srv) {
		refused(t, srv, f, etag)
	}
}

// agentSavePageKeepsItsRule is the other half of the strict option: the same
// failure never refuses a page's save.
func agentSavePageKeepsItsRule(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha", "beta"))
	_, etag := agentSaveRead(t, srv, f)
	if !agentSaveHistoryReadOnly(t, srv) {
		t.Skip("this system writes into a read-only history folder")
	}

	fromPage := agentSaveDoc("List by the page", "alpha", "beta")
	w := agentSavePage(srv, f, fromPage, map[string]string{"If-Match": etag})
	if w.Code != http.StatusOK {
		t.Fatalf("a page's save answered %d %s while history is unavailable, want 200", w.Code, w.Body.String())
	}
	if got := docString(t, f.AbsPath); got != fromPage {
		t.Fatalf("the page's save did not land: %q", got)
	}
	if names := listVersionNames(t, srv, f); len(names) != 0 {
		t.Fatalf("history stored %v, so this save never met the failure it is here for", names)
	}
}

func agentSaveStale(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	_, old := agentSaveRead(t, srv, f)
	moved := agentSaveDoc("List typed since", "alpha", "beta")
	if w := agentSavePage(srv, f, moved, nil); w.Code != http.StatusOK {
		t.Fatalf("the page's save answered %d %s", w.Code, w.Body.String())
	}
	versions := listVersionNames(t, srv, f)

	// The stale write removes nothing, so only the stamp can refuse it.
	incoming := agentSaveDoc("List by the agent", "alpha", "beta", "gamma")
	for _, stamp := range []struct{ name, value string }{
		{"the stamp of an older version", old},
		{"no stamp", ""},
		{"a wildcard", "*"},
		{"a weak stamp", "W/" + specwire.Etag([]byte(moved))},
		{"a list of stamps", old + ", " + specwire.Etag([]byte(moved))},
	} {
		saved, err := agentSaveDo(srv, f, incoming, stamp.value, "agent-save-1")
		if got := docString(t, f.AbsPath); got != moved {
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
	if got := listVersionNames(t, srv, f); !slices.Equal(got, versions) {
		t.Fatalf("the stale saves changed the history: %v, was %v", got, versions)
	}

	// A late repeat of the program's own save reads its own id in the answer.
	first, err := agentSaveDo(srv, f, incoming, specwire.Etag([]byte(moved)), "agent-save-2")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	_, err = agentSaveDo(srv, f, incoming+"<!-- later -->", specwire.Etag([]byte(moved)), "agent-save-2")
	var stale *StaleError
	if !errors.As(err, &stale) || stale.SaveID != "agent-save-2" || stale.Etag != first.Etag {
		t.Fatalf("the repeat answered %v (%+v), want a StaleError naming save agent-save-2 and stamp %q", err, stale, first.Etag)
	}
}

func agentSaveRace(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	agentWon, pageWon := 0, 0
	for round := 0; round < 16; round++ {
		_, etag := agentSaveRead(t, srv, f)
		fromAgent := agentSaveDoc(fmt.Sprintf("Round %d by the agent", round), "alpha")
		fromPage := agentSaveDoc(fmt.Sprintf("Round %d by the page", round), "alpha")

		var (
			wait     sync.WaitGroup
			agentErr error
			page     *httptest.ResponseRecorder
		)
		start := make(chan struct{})
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			time.Sleep(time.Duration(round%4) * 50 * time.Microsecond)
			_, agentErr = agentSaveDo(srv, f, fromAgent, etag, "")
		}()
		go func() {
			defer wait.Done()
			<-start
			page = agentSavePage(srv, f, fromPage, map[string]string{"If-Match": etag})
		}()
		close(start)
		wait.Wait()

		disk := docString(t, f.AbsPath)
		var stale *StaleError
		switch {
		case agentErr == nil && page.Code == http.StatusPreconditionFailed:
			agentWon++
			if disk != fromAgent {
				t.Fatalf("round %d: the agent's save won and the file is %q", round, disk)
			}
			if got := decode(t, page)["etag"]; got != specwire.Etag([]byte(fromAgent)) {
				t.Fatalf("round %d: the page's stale answer carries %v, want the winner's stamp", round, got)
			}
		case errors.As(agentErr, &stale) && page.Code == http.StatusOK:
			pageWon++
			if disk != fromPage {
				t.Fatalf("round %d: the page's save won and the file is %q", round, disk)
			}
			if stale.Etag != specwire.Etag([]byte(fromPage)) {
				t.Fatalf("round %d: the agent's stale answer carries %q, want the winner's stamp", round, stale.Etag)
			}
		default:
			t.Fatalf("round %d: the agent's save answered %v and the page's %d %s, want one winner and one stale answer; the file is %q",
				round, agentErr, page.Code, page.Body.String(), disk)
		}
	}
	t.Logf("the agent's save won %d rounds and the page's %d", agentWon, pageWon)
}

func agentSaveRestore(t *testing.T) {
	original := agentSaveDoc("List", "alpha", "beta")
	srv, f := agentSaveStand(t, original)
	_, etag := agentSaveRead(t, srv, f)
	second := agentSaveDoc("List by the agent", "alpha")
	saved, err := agentSaveDo(srv, f, second, etag, "")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	stale, err := srv.RestoreDocument(f.AbsPath, saved.Replaced, DocumentSave{IfMatch: etag})
	var refused *StaleError
	if !errors.As(err, &refused) || refused.Etag != saved.Etag {
		t.Fatalf("a restore with an old stamp answered %+v, %v, want a StaleError carrying %q", stale, err, saved.Etag)
	}
	if got := docString(t, f.AbsPath); got != second {
		t.Fatalf("the refused restore changed the file: %q", got)
	}

	if _, err := srv.RestoreDocument(f.AbsPath, "20260101-000000.000000000-0000000000000000.html", DocumentSave{IfMatch: saved.Etag}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a restore of a version that is not kept answered %v, want fs.ErrNotExist", err)
	}
	if got := docString(t, f.AbsPath); got != second {
		t.Fatalf("the restore of a missing version changed the file: %q", got)
	}

	restored, err := srv.RestoreDocument(f.AbsPath, saved.Replaced, DocumentSave{IfMatch: saved.Etag})
	if err != nil {
		t.Fatalf("RestoreDocument: %v", err)
	}
	if got := docString(t, f.AbsPath); got != original {
		t.Fatalf("the file after the restore = %q, want the version's bytes", got)
	}
	if !restored.Changed || restored.Etag != specwire.Etag([]byte(original)) {
		t.Fatalf("restored = %+v, want changed and the stamp of the restored bytes", restored)
	}
	if restored.Replaced == "" {
		t.Fatalf("the restore named no version for the bytes it replaced; versions: %v", listVersionNames(t, srv, f))
	}
	if got := agentSaveVersion(t, srv, f, restored.Replaced); got != second {
		t.Fatalf("version %s holds %q, want the bytes the restore replaced", restored.Replaced, got)
	}
}

// agentSaveHeard is why the save is the lane's write with no tab behind it: a
// tab that is editing the document is told, with the new bytes, where a page's
// own save tells only the tabs that view it.
func agentSaveHeard(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	_, etag := agentSaveRead(t, srv, f)
	live := newSubscriber(f.AbsPath, laneLive)
	saved := newSubscriber(f.AbsPath, laneSaved)
	srv.hub.add(live)
	srv.hub.add(saved)

	written := agentSaveDoc("Heard by the tabs", "alpha")
	answer, err := agentSaveDo(srv, f, written, etag, "")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	f.Lock()
	key := f.HistoryKey()
	f.Unlock()
	want := forBrowser([]byte(written), key, srv.documentURLFor(f))
	notice := waitFrame(t, live, time.Second)
	data, _ := notice["data"].(map[string]interface{})
	if notice["type"] != "notification" || data["kind"] != "external-change" {
		t.Fatalf("the live lane got %v, want the notice of an outside change", notice)
	}
	if data["html"] != want || data["etag"] != answer.Etag {
		t.Fatalf("the notice carries html %q and etag %v, want the saved bytes and %q", data["html"], data["etag"], answer.Etag)
	}
	if content := waitFrame(t, saved, time.Second); content["html"] != want {
		t.Fatalf("the saved lane got %v, want the saved bytes", content)
	}
}

func agentSaveUnchanged(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	read, etag := agentSaveRead(t, srv, f)
	live := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(live)

	saved, err := agentSaveDo(srv, f, read, etag, "agent-save-1")
	if err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if saved.Changed || saved.Etag != etag || saved.Replaced != "" {
		t.Fatalf("a save of the bytes just read answered %+v, want unchanged, the same stamp and no version", saved)
	}
	if names := listVersionNames(t, srv, f); len(names) != 0 {
		t.Fatalf("a save of the bytes just read kept %v", names)
	}
	expectNoFrame(t, live, 100*time.Millisecond)
}

func agentSaveMissing(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	if err := os.Remove(f.AbsPath); err != nil {
		t.Fatal(err)
	}
	saved, err := agentSaveDo(srv, f, agentSaveDoc("New", "alpha"), specwire.Etag(nil), "")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a save to a document that does not exist answered %+v, %v, want fs.ErrNotExist", saved, err)
	}
	if _, err := os.Stat(f.AbsPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the save created %s: %v", f.AbsPath, err)
	}
	if _, err := srv.SaveDocument(f.AbsPath+".other", DocumentSave{HTML: []byte(agentSaveDoc("New")), IfMatch: specwire.Etag(nil)}); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("a save to a file this host does not hold open answered %v, want ErrNotOwned", err)
	}
}

func agentSaveRefusedBodies(t *testing.T) {
	original := agentSaveDoc("List", "alpha")
	srv, f := agentSaveStand(t, original)
	_, etag := agentSaveRead(t, srv, f)

	if _, err := agentSaveDo(srv, f, "<p>a fragment</p>", etag, ""); !errors.Is(err, ErrNotDocument) {
		t.Fatalf("a save of a fragment answered %v, want ErrNotDocument", err)
	}
	if _, err := agentSaveDo(srv, f, "", etag, ""); !errors.Is(err, ErrNotDocument) {
		t.Fatalf("a save of nothing answered %v, want ErrNotDocument", err)
	}
	large := agentSaveDoc(strings.Repeat("x", maxSaveSize), "alpha")
	if _, err := agentSaveDo(srv, f, large, etag, ""); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a save over the page's limit answered %v, want ErrTooLarge", err)
	}
	if got := docString(t, f.AbsPath); got != original {
		t.Fatalf("a refused save changed the file: %q", got)
	}
}

func agentSaveOwner(t *testing.T) {
	original := agentSaveDoc("List", "alpha")
	srv, f := agentSaveStand(t, original)
	_, etag := agentSaveRead(t, srv, f)
	incoming := agentSaveDoc("List by the agent", "alpha")

	asked := 0
	_, err := srv.SaveDocument(f.AbsPath, DocumentSave{
		HTML: []byte(incoming), IfMatch: etag,
		Owned: func() bool {
			asked++
			if f.TryLock() {
				f.Unlock()
				t.Error("the owner was asked with the file's lock not held")
			}
			return false
		},
	})
	if !errors.Is(err, ErrNotOwned) || asked != 1 {
		t.Fatalf("a save whose owner changed answered %v after asking %d times, want ErrNotOwned after one", err, asked)
	}
	if got := docString(t, f.AbsPath); got != original {
		t.Fatalf("the save changed a document this host no longer owns: %q", got)
	}

	// The file is closed while the save waits for the lock.
	f.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := agentSaveDo(srv, f, incoming, etag, "")
		done <- err
	}()
	<-started
	time.Sleep(150 * time.Millisecond)
	if !srv.sessions.Unregister(f.AbsPath) {
		t.Fatal("Unregister found no file")
	}
	f.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotOwned) {
			t.Fatalf("a save that waited while its file was closed answered %v, want ErrNotOwned", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting save never answered")
	}
	if got := docString(t, f.AbsPath); got != original {
		t.Fatalf("the save wrote a file that was closed while it waited: %q", got)
	}
}

func agentSaveReadStamp(t *testing.T) {
	srv, f := agentSaveStand(t, agentSaveDoc("List", "alpha"))
	_, etag := agentSaveRead(t, srv, f)
	if w := agentSavePage(srv, f, agentSaveDoc("List by the page", "alpha"), map[string]string{"If-Match": etag}); w.Code != http.StatusOK {
		t.Fatalf("a page's save with the stamp ReadDocument answered got %d %s, want 200", w.Code, w.Body.String())
	}
	_, next := agentSaveRead(t, srv, f)
	if _, err := agentSaveDo(srv, f, agentSaveDoc("List by the agent", "alpha"), next, ""); err != nil {
		t.Fatalf("a save with the stamp ReadDocument answered: %v", err)
	}
}
