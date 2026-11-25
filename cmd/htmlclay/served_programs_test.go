package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/testutil"
)

const servedProgramsDocumentName = "doc.html"

// servedProgramsStand is one served folder whose document declares search, with
// one registered program that proves it ran by writing a marker.
type servedProgramsStand struct {
	*servedWireStand
	program config.HelperProgram
	marker  string
}

// servedProgramsDocument is the served document with the declaration the loose
// helper fixtures write, so the same setup flow applies.
func servedProgramsDocument() string {
	return strings.Replace(servedTestDocument, "</head>", `<meta name="htmlclay-helper" content="search"></head>`, 1)
}

func servedProgramsOpen(t *testing.T) *servedProgramsStand {
	t.Helper()
	return servedProgramsOpenAs(t, false)
}

// servedProgramsOpenAs sets shell before serving starts publishing snapshots,
// as the app does at start.
func servedProgramsOpenAs(t *testing.T, shell bool) *servedProgramsStand {
	t.Helper()
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, servedProgramsDocumentName), servedProgramsDocument())

	a := newServedTestApp(t, home, t.TempDir())
	a.shell = shell
	entry, refusal, err := a.serveFolder(dir, "notes")
	if err != nil {
		t.Fatalf("serveFolder: %v", err)
	}
	if refusal != nil {
		t.Fatalf("serveFolder refused: %+v", refusal)
	}
	server := httptest.NewServer(http.HandlerFunc(a.serveHost))
	t.Cleanup(server.Close)
	stand := &servedWireStand{
		a:      a,
		home:   home,
		entry:  entry,
		dir:    dir,
		host:   servedHostName("notes", a.rt.port),
		server: server,
	}

	programPath, marker := attachmentProgram(t, "recorder")
	program, err := a.rt.cfg.AddHelperProgram("search", programPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return &servedProgramsStand{servedWireStand: stand, program: program, marker: marker}
}

// loadPage asks for the served document the way a browser asks for a page, which
// is what runs BeforePage.
func (s *servedProgramsStand) loadPage(t *testing.T) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.server.URL+"/"+servedProgramsDocumentName, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.host
	response := s.do(t, request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", servedProgramsDocumentName, response.StatusCode)
	}
	return servedWireReadBody(t, response)
}

// pageRequest sends the program request a served document's own script sends.
func (s *servedProgramsStand) pageRequest(t *testing.T, id string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"v":1,"type":"wire/request","id":%q,"helper":"search","document":"none","payload":{"query":"clay"}}`, id)
	return s.pageSend(t, body, servedProgramsDocumentName)
}

// setup posts the setup bar's call for one program name.
func (s *servedProgramsStand) setup(t *testing.T, name string) (*http.Response, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/helper-setup", strings.NewReader(fmt.Sprintf(`{"name":%q}`, name)))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.host
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", s.documentURL(servedProgramsDocumentName))
	response := s.do(t, request)
	return response, servedWireReadBody(t, response)
}

// setupAsync posts the setup bar's call in the background, because it waits for
// the person's answer.
func (s *servedProgramsStand) setupAsync() <-chan string {
	answer := make(chan string, 1)
	go func() {
		request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/helper-setup", strings.NewReader(`{"name":"search"}`))
		if err != nil {
			answer <- "error: " + err.Error()
			return
		}
		request.Host = s.host
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("Document-URL", s.documentURL(servedProgramsDocumentName))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			answer <- "error: " + err.Error()
			return
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		answer <- fmt.Sprintf("%d %s", response.StatusCode, body)
	}()
	return answer
}

// sendLoose posts one request as the page of a loose document.
func (s *servedProgramsStand) sendLoose(t *testing.T, site *site, token, id, document string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"v":1,"type":"wire/request","id":%q,"file":%q,"helper":"search","document":"none","payload":{}}`, id, document)
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/wire/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = site.host
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Save-Token", token)
	return s.do(t, request)
}

func (s *servedProgramsStand) document(t *testing.T) string {
	t.Helper()
	return s.file(t)
}

// allowDocument records the decision the set-up dialog records for one document.
func (s *servedProgramsStand) allowDocument(t *testing.T) {
	t.Helper()
	document := s.document(t)
	if _, _, err := s.a.rt.cfg.DecideHelper(config.HelperDecision{
		Document: document,
		Name:     "search",
		Program:  s.program.ID,
		Allowed:  true,
		Scope:    s.a.scopeOf(document),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

// allowForAnyDocument is the broad grant the Programs page writes when a program
// is allowed for every document in a trusted folder.
func (s *servedProgramsStand) allowForAnyDocument(t *testing.T) {
	t.Helper()
	if _, ok := s.a.rt.cfg.SetHelperAnyDocument(s.program.ID, true); !ok {
		t.Fatal("fixture could not grant the program for any document")
	}
	if err := s.a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

// markTeam records the served folder as a team folder under key.
func (s *servedProgramsStand) markTeam(t *testing.T, key, folderID string) {
	t.Helper()
	if err := s.a.rt.cfg.MarkTeamFolder(config.TeamFolder{
		FolderID: folderID,
		Path:     s.dir,
		Identity: platform.DirIdentity(s.dir),
		Kind:     config.TeamFolderKindTeam,
		Key:      key,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

// shellApp answers setup questions through the shell registry, the way the loose
// team tests ask and answer them.
func (s *servedProgramsStand) shellApp(t *testing.T) {
	t.Helper()
	if !s.a.shell {
		t.Fatal("fixture: open the stand with servedProgramsOpenAs(t, true)")
	}
	t.Cleanup(func() {
		for _, p := range s.a.registry().Pending() {
			s.a.registry().Close(p.ID)
		}
	})
}

func (s *servedProgramsStand) coreHasBound(t *testing.T, document string) bool {
	t.Helper()
	if s.a.served == nil {
		return false
	}
	core, ok := s.a.served.Wire(s.entry.ID)
	if !ok {
		return false
	}
	return slices.Contains(core.BoundFiles(), document)
}

// TestM3ServedPrograms is step 7.28c: a served document runs its programs by the
// person's decisions, a page load binds them before the page's first request, and
// Remove Access stops a run.
func TestM3ServedPrograms(t *testing.T) {
	t.Run("a served document with its own decision runs its program", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		stand.allowDocument(t)
		stand.loadPage(t)

		response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		sent := stand.pageRequest(t, "r1")
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		page.waitFor(t, "wire/done", "r1")
		if _, err := os.Stat(stand.marker); err != nil {
			t.Fatalf("the program did not run: %v", err)
		}
	})

	t.Run("a page load binds before the page's first request", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		stand.allowDocument(t)
		stand.loadPage(t)

		document := stand.document(t)
		if !stand.coreHasBound(t, document) {
			t.Fatalf("BoundFiles after the page load does not contain %s", document)
		}
	})

	t.Run("with no decision the request parks and the setup call asks", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		stand.loadPage(t)
		stand.a.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
			return platform.ChoiceAffirm, nil
		}
		stand.a.rt.prepareHelperProgram = func(string) (bool, error) { return true, nil }

		response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		sent := stand.pageRequest(t, "r1")
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		page.waitFor(t, "wire/ack", "r1")
		if _, err := os.Stat(stand.marker); err == nil {
			t.Fatal("the program ran before the person set it up")
		}

		setupResponse, setupBody := stand.setup(t, "search")
		if setupResponse.StatusCode != http.StatusOK || !strings.Contains(setupBody, `"state":"ready"`) {
			t.Fatalf("POST /_/helper-setup = %d %s, want ready", setupResponse.StatusCode, setupBody)
		}
		page.waitFor(t, "wire/done", "r1")
		if _, err := os.Stat(stand.marker); err != nil {
			t.Fatalf("the program did not run after setup: %v", err)
		}
	})

	t.Run("a team folder never runs a program allowed for all trusted folders", func(t *testing.T) {
		stand := servedProgramsOpenAs(t, true)
		stand.shellApp(t)
		stand.markTeam(t, "acme", stand.entry.ID)
		stand.allowForAnyDocument(t)
		stand.loadPage(t)

		response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		sent := stand.pageRequest(t, "r1")
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		page.waitFor(t, "wire/ack", "r1")

		setupDone := stand.setupAsync()
		request := waitForSetupRequest(t, stand.a, appcore.RequestTeamDocument)
		if slices.Contains(request.Answers, helperAnswerAllowAllTrusted) {
			t.Fatalf("a team document offered allow-all-trusted: %v", request.Answers)
		}
		if err := answerSetup(t, stand.a, "a17", request, helperAnswerDeny, ""); err != nil {
			t.Fatal(err)
		}
		select {
		case answer := <-setupDone:
			if !strings.HasPrefix(answer, "200 ") {
				t.Fatalf("POST /_/helper-setup = %s, want 200", answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the setup flow did not finish")
		}
		if _, err := os.Stat(stand.marker); err == nil {
			t.Fatal("the broad grant started the program in a team folder")
		}
		page.waitFor(t, "wire/error", "r1")
	})

	t.Run("after the folder stops being served the team rule still holds", func(t *testing.T) {
		stand := servedProgramsOpenAs(t, true)
		stand.shellApp(t)
		stand.markTeam(t, "acme", stand.entry.ID)
		stand.allowForAnyDocument(t)
		stand.loadPage(t)

		response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		stand.pageRequest(t, "r1")
		page.waitFor(t, "wire/ack", "r1")
		setupDone := stand.setupAsync()
		request := waitForSetupRequest(t, stand.a, appcore.RequestTeamDocument)
		if err := answerSetup(t, stand.a, "a17", request, helperAnswerDeny, ""); err != nil {
			t.Fatal(err)
		}
		select {
		case answer := <-setupDone:
			if !strings.HasPrefix(answer, "200 ") {
				t.Fatalf("POST /_/helper-setup = %s, want 200", answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the setup flow did not finish")
		}

		if err := stand.a.stopServing(stand.entry.ID); err != nil {
			t.Fatalf("stopServing: %v", err)
		}
		if err := stand.a.trustFolder(stand.dir); err != nil {
			t.Fatalf("trustFolder: %v", err)
		}
		document := stand.document(t)
		if scope := stand.a.scopeOf(document); scope.Kind != config.ScopeTeam {
			t.Fatalf("scope after stop serving = %+v, want the team folder", scope)
		}
		site, _ := stand.a.openForTest(t, document)
		stand.a.applyHelperPlan(site.srv.Core, document, stand.a.helpersForNavigation(document))
		registration, ok := site.sessions.LookupByPath(document)
		if !ok {
			t.Fatal("the loose document is not registered")
		}
		sent := stand.sendLoose(t, site, registration.Token, "r2", document)
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("the loose request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		time.Sleep(300 * time.Millisecond)
		if _, err := os.Stat(stand.marker); err == nil {
			t.Fatal("the broad grant started the program after the folder stopped being served")
		}
	})

	t.Run("Remove Access stops a running served program", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		waiter, started := attachmentProgram(t, attachmentWaiter)
		if _, ok := stand.a.rt.cfg.SetHelperProgramPath(stand.program.ID, waiter); !ok {
			t.Fatal("fixture could not relink the program")
		}
		if err := stand.a.rt.cfg.Save(); err != nil {
			t.Fatal(err)
		}
		stand.allowDocument(t)
		stand.loadPage(t)

		response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		sent := stand.pageRequest(t, "r1")
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		page.waitFor(t, "wire/ack", "r1")
		// Remove Access is for a running program. On Windows a stop sent while
		// the program is still loading does not reach it, and the host's kill
		// comes seconds later.
		testutil.Eventually(t, 10*time.Second, "the program to start", func() bool {
			_, err := os.Stat(started)
			return err == nil
		})
		// A port change or an update restart asks first while it runs.
		if n := stand.a.runningWork(); n != 1 {
			t.Fatalf("runningWork with a served folder's program running = %d, want 1", n)
		}

		if refusal := m3ProgramDo(t, stand.a, ChannelShell, "program.removeAccess", map[string]any{
			"programID": stand.program.ID, "documentID": stand.document(t),
		}); refusal != nil {
			t.Fatalf("program.removeAccess = %+v", refusal)
		}
		terminal := page.waitFor(t, "wire/error", "r1")
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Code != "helper_cancelled" {
			t.Fatalf("the running program ended with %q, want helper_cancelled", payload.Code)
		}
	})

	t.Run("a local process cannot use a bound channel", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		stand.loadPage(t)
		if !stand.coreHasBound(t, stand.document(t)) {
			t.Fatal("the page load did not bind the document")
		}
		subscribed, _ := stand.processSubscribe(t, stand.document(t))
		if subscribed.StatusCode != http.StatusForbidden {
			t.Fatalf("a local process subscribed to a bound channel with %d, want 403", subscribed.StatusCode)
		}
		sent := stand.processSend(t, fmt.Sprintf(`{"v":1,"type":"wire/request","id":"p1","file":%q,"helper":"search","document":"none","payload":{}}`, stand.document(t)))
		if sent.StatusCode != http.StatusForbidden {
			t.Fatalf("a local process sent on a bound channel with %d, want 403", sent.StatusCode)
		}
	})

	t.Run("a renamed document's old channel is closed", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		document := stand.document(t)
		stand.loadPage(t)
		if !stand.coreHasBound(t, document) {
			t.Fatal("the page load did not bind the document")
		}
		response, stream := stand.pageSubscribe(t, servedProgramsDocumentName)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}

		renamed := filepath.Join(stand.dir, "renamed.html")
		if err := os.Rename(document, renamed); err != nil {
			t.Fatal(err)
		}
		stand.a.noteServedStructure(stand.entry.ID, servedProgramsDocumentName, "renamed.html", false)
		stream.waitClosed(t)
	})

	t.Run("a saved served document does not contain the setup bar", func(t *testing.T) {
		stand := servedProgramsOpen(t)
		page := stand.loadPage(t)
		if !strings.Contains(page, "data-htmlclay-setup") {
			t.Fatal("the page load did not carry the setup bar")
		}
		request, err := http.NewRequest(http.MethodPost, stand.server.URL+"/_/save", strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = stand.host
		request.Header.Set("Content-Type", "text/html")
		request.Header.Set("Document-URL", stand.documentURL(servedProgramsDocumentName))
		response := stand.do(t, request)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("POST /_/save = %d, want 200 (%s)", response.StatusCode, servedWireReadBody(t, response))
		}
		onDisk, err := os.ReadFile(stand.document(t))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(onDisk), "data-htmlclay-setup") {
			t.Fatal("the saved document kept the setup bar")
		}
	})
}

// TestDontAllowRefusesAWaitingServedProgramCall is Carbon 8: a served page's
// program call parked while the warning is open ends with a refusal the moment
// the person says Don't allow, rather than waiting out its budget.
func TestDontAllowRefusesAWaitingServedProgramCall(t *testing.T) {
	stand := servedProgramsOpenAs(t, true)
	stand.shellApp(t)
	downloadedHere(t, stand.a, stand.document(t))
	stand.loadPage(t)

	response, page := stand.pageSubscribe(t, servedProgramsDocumentName)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
	}
	sent := stand.pageRequest(t, "r1")
	if sent.StatusCode != http.StatusOK {
		t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
	}
	page.waitFor(t, "wire/ack", "r1")

	document := stand.document(t)
	granted := make(chan bool, 1)
	go func() { granted <- stand.a.askHelperForUse(document, "search") }()
	request := waitForSetupRequest(t, stand.a, appcore.RequestUnknownDocument)
	if request.Attention == 0 {
		t.Fatal("the warning Set Up raised did not bring the request window to the front")
	}
	if request.Program != "search" {
		t.Fatalf("the warning names program %q, want %q", request.Program, "search")
	}
	if err := answerSetup(t, stand.a, "a18", request, documentAnswerDeny, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-granted:
		if ok {
			t.Fatal("the program was granted after Don't allow")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Set Up did not return after Don't allow")
	}

	deadline := time.After(time.Second)
	for {
		select {
		case env, ok := <-page.ch:
			if !ok {
				t.Fatal("the page stream ended without a refusal")
			}
			if env.ID == "r1" && env.Type == "wire/error" {
				return
			}
		case <-deadline:
			t.Fatal("the parked program call was not refused within a second of Don't allow")
		}
	}
}

// TestSetUpTrustsTheDocumentThenFinishesItsProgram: one Set Up on a document
// nobody vouches for asks about the document first and, once the person trusts
// it, carries on without a second press: a program allowed for trusted
// documents is granted at once, and otherwise the program's own question follows.
func TestSetUpTrustsTheDocumentThenFinishesItsProgram(t *testing.T) {
	t.Run("a program allowed for trusted documents needs no second question", func(t *testing.T) {
		stand := servedProgramsOpenAs(t, true)
		stand.shellApp(t)
		stand.allowForAnyDocument(t)
		downloadedHere(t, stand.a, stand.document(t))
		stand.loadPage(t)
		document := stand.document(t)
		granted := make(chan bool, 1)
		go func() { granted <- stand.a.askHelperForUse(document, "search") }()
		request := waitForSetupRequest(t, stand.a, appcore.RequestUnknownDocument)
		if err := answerSetup(t, stand.a, "a30", request, documentAnswerTrust, ""); err != nil {
			t.Fatal(err)
		}
		select {
		case ok := <-granted:
			if !ok {
				t.Fatal("trusting the document did not carry on to the program allowed for trusted documents")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Set Up did not return after the document was trusted")
		}
	})

	t.Run("otherwise the program question follows, raised", func(t *testing.T) {
		stand := servedProgramsOpenAs(t, true)
		stand.shellApp(t)
		downloadedHere(t, stand.a, stand.document(t))
		stand.loadPage(t)
		document := stand.document(t)
		granted := make(chan bool, 1)
		go func() { granted <- stand.a.askHelperForUse(document, "search") }()
		request := waitForSetupRequest(t, stand.a, appcore.RequestUnknownDocument)
		if err := answerSetup(t, stand.a, "a31", request, documentAnswerTrust, ""); err != nil {
			t.Fatal(err)
		}
		var program appcore.Request
		waitFor(t, "the program question after the document was trusted", func() bool {
			for _, r := range stand.a.snapshot().Requests {
				if r.Kind != appcore.RequestUnknownDocument && r.Program == "search" {
					program = r
					return true
				}
			}
			return false
		})
		if program.Attention == 0 {
			t.Fatal("the program question did not bring the request window to the front")
		}
	})
}

// TestSecondSetUpPressRaisesTheOpenQuestion: while Set Up waits on the person,
// pressing it again brings the waiting question back to the front.
func TestSecondSetUpPressRaisesTheOpenQuestion(t *testing.T) {
	stand := servedProgramsOpenAs(t, true)
	stand.shellApp(t)
	downloadedHere(t, stand.a, stand.document(t))
	stand.loadPage(t)
	first := stand.setupAsync()
	request := waitForSetupRequest(t, stand.a, appcore.RequestUnknownDocument)
	response, body := stand.setup(t, "search")
	if response.StatusCode != http.StatusOK || !strings.Contains(body, `"busy"`) {
		t.Fatalf("second press = %d %s, want 200 busy", response.StatusCode, body)
	}
	waitFor(t, "the second press to raise the waiting question", func() bool {
		for _, r := range stand.a.snapshot().Requests {
			if r.ID == request.ID {
				return r.Attention > request.Attention
			}
		}
		return false
	})
	if err := answerSetup(t, stand.a, "a32", request, documentAnswerDeny, ""); err != nil {
		t.Fatal(err)
	}
	<-first
}
