package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/wire"
)

const aiEditAskRefusal = "AI editing cannot be used until you allow this document in HTML Clay."

// aiEditStand is one host with a document and its neighbor in one folder, and
// the two things a row needs from it: send the page's ai-edit request and read
// the frame that answers it.
type aiEditStand struct {
	a        *app
	cfgBase  string
	folder   string
	document string
	neighbor string
	post     func(t *testing.T, document, id, elementHTML string) wire.Envelope
}

func (s *aiEditStand) send(t *testing.T, document, id string) wire.Envelope {
	t.Helper()
	return s.post(t, document, id, "<p>Old</p>")
}

func aiEditRequestBody(id, file, elementHTML string) string {
	return fmt.Sprintf(`{"v":1,"type":"wire/request","id":%q,"file":%q,"helper":"ai-edit","payload":{"elementHTML":%q,"tag":"p","comment":"make it [mock:end_turn]","editId":"p"}}`, id, file, elementHTML)
}

func aiEditTerminal(t *testing.T, next func() (wire.Envelope, bool), id string) wire.Envelope {
	t.Helper()
	for {
		env, ok := next()
		if !ok {
			t.Fatalf("no terminal frame for %s", id)
		}
		if env.ID == id && (env.Type == "wire/done" || env.Type == "wire/error") {
			return env
		}
	}
}

func newLooseAIEditStand(t *testing.T) *aiEditStand {
	t.Helper()
	t.Setenv("MOCK_MODEL", "1")
	home := evalTempHome(t)
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, home, cfgBase)
	a.shell = true
	t.Cleanup(a.stopRequests)
	folder := filepath.Join(home, "docs")
	document := filepath.Join(folder, "roadmap.htmlclay")
	neighbor := filepath.Join(folder, "budget.htmlclay")
	writeTestFile(t, document, "<!doctype html><head></head><body><p>Old</p></body>")
	writeTestFile(t, neighbor, "<!doctype html><head></head><body><p>Old</p></body>")
	sites := map[string]*site{}
	for _, path := range []string{document, neighbor} {
		s, _ := a.openForTest(t, path)
		sites[path] = s
	}
	t.Cleanup(a.cancelAIEdits)
	send := func(t *testing.T, path, id, elementHTML string) wire.Envelope {
		t.Helper()
		s := sites[path]
		registration, ok := s.sessions.LookupByPath(path)
		if !ok {
			t.Fatalf("%s is not registered", path)
		}
		sub := &wire.Sub{Key: path, Ch: make(chan []byte, 64), Done: make(chan struct{})}
		if _, _, err := s.srv.Core.Hub.Add(sub, 0); err != nil {
			t.Fatal(err)
		}
		defer s.srv.Core.Hub.Remove(sub)
		req, err := http.NewRequest(http.MethodPost, "http://"+s.host+"/_/wire/send", strings.NewReader(aiEditRequestBody(id, path, elementHTML)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Save-Token", registration.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("wire send = %d", resp.StatusCode)
		}
		deadline := time.After(5 * time.Second)
		return aiEditTerminal(t, func() (wire.Envelope, bool) {
			select {
			case raw := <-sub.Ch:
				env, err := wire.DecodeEnvelope(raw)
				if err != nil {
					t.Fatal(err)
				}
				return env, true
			case <-deadline:
				return wire.Envelope{}, false
			}
		}, id)
	}
	return &aiEditStand{a: a, cfgBase: cfgBase, folder: folder, document: document, neighbor: neighbor, post: send}
}

func newServedAIEditStand(t *testing.T) *aiEditStand {
	t.Helper()
	t.Setenv("MOCK_MODEL", "1")
	served := servedWireOpen(t)
	served.a.shell = true
	t.Cleanup(served.a.stopRequests)
	t.Cleanup(served.a.cancelAIEdits)
	document := served.file(t)
	neighbor := filepath.Join(filepath.Dir(document), "budget.html")
	writeTestFile(t, neighbor, servedTestDocument)
	send := func(t *testing.T, path, id, elementHTML string) wire.Envelope {
		t.Helper()
		name := filepath.Base(path)
		response, frames := served.pageSubscribe(t, name)
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("subscribe = %d", response.StatusCode)
		}
		sent := served.pageSend(t, aiEditRequestBody(id, "", elementHTML), name)
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("wire send = %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
		}
		deadline := time.After(5 * time.Second)
		return aiEditTerminal(t, func() (wire.Envelope, bool) {
			select {
			case env, ok := <-frames.ch:
				return env, ok
			case <-deadline:
				return wire.Envelope{}, false
			}
		}, id)
	}
	return &aiEditStand{a: served.a, folder: filepath.Dir(document), document: document, neighbor: neighbor, post: send}
}

func (s *aiEditStand) markTeam(t *testing.T, key string) {
	t.Helper()
	if err := s.a.rt.cfg.MarkTeamFolder(config.TeamFolder{
		Path:     s.folder,
		Identity: platform.DirIdentity(s.folder),
		Kind:     config.TeamFolderKindTeam,
		Key:      key,
	}); err != nil {
		t.Fatal(err)
	}
	if scope := s.a.scopeOf(s.document); scope != (config.Scope{Kind: config.ScopeTeam, Key: "team:" + key}) {
		t.Fatalf("fixture: scope = %+v, want team:%s", scope, key)
	}
}

func (s *aiEditStand) questions(t *testing.T) []confirm.Pending {
	t.Helper()
	var out []confirm.Pending
	for _, p := range s.a.registry().Pending() {
		if p.Ask.Program == aiEditProgramID {
			out = append(out, p)
		}
	}
	return out
}

func (s *aiEditStand) awaitQuestion(t *testing.T, document string) confirm.Pending {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range s.questions(t) {
			if p.Ask.Document == document {
				return p
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no AI editing question for %s", document)
	return confirm.Pending{}
}

// answer answers the question and waits until the flow has finished with it.
func (s *aiEditStand) answer(t *testing.T, document, choice string) {
	t.Helper()
	p := s.awaitQuestion(t, document)
	if err := s.a.registry().Answer(p.ID, p.Revision, choice, ""); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.a.helperStateMu.Lock()
		asking := s.a.aiEditAsking[document]
		s.a.helperStateMu.Unlock()
		if !asking {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the AI editing question never finished")
}

func (s *aiEditStand) requireAsks(t *testing.T, document, id string) {
	t.Helper()
	env := s.send(t, document, id)
	if env.Type != "wire/error" || env.Text != aiEditAskRefusal {
		t.Fatalf("%s: frame = %s %q, want the ask refusal", filepath.Base(document), env.Type, env.Text)
	}
}

func (s *aiEditStand) requireRuns(t *testing.T, document, id string) {
	t.Helper()
	env := s.send(t, document, id)
	if env.Type != "wire/done" {
		t.Fatalf("%s: frame = %s %q, want wire/done", filepath.Base(document), env.Type, env.Text)
	}
}

func (s *aiEditStand) rewindQuiet(t *testing.T, document string) {
	t.Helper()
	s.a.helperStateMu.Lock()
	defer s.a.helperStateMu.Unlock()
	if s.a.aiEditQuiet[document].IsZero() {
		t.Fatalf("fixture: %s has no quiet minute to rewind", document)
	}
	s.a.aiEditQuiet[document] = time.Now().Add(-time.Second)
}

func TestM3AIEditTeamDocument(t *testing.T) {
	hosts := []struct {
		name string
		open func(*testing.T) *aiEditStand
	}{
		{"loose", newLooseAIEditStand},
		{"served", newServedAIEditStand},
	}
	for _, host := range hosts {
		t.Run(host.name+": a local document runs without asking", func(t *testing.T) {
			s := host.open(t)
			s.requireRuns(t, s.document, "e1")
			if got := s.questions(t); len(got) != 0 {
				t.Fatalf("questions = %+v, want none", got)
			}
			if rows := s.a.rt.cfg.AIEditDocumentList(); len(rows) != 0 {
				t.Fatalf("rows = %+v, want none", rows)
			}
		})

		t.Run(host.name+": a team document asks, runs after allow-document, and its neighbor still asks", func(t *testing.T) {
			s := host.open(t)
			s.markTeam(t, "design")
			s.requireAsks(t, s.document, "e1")
			p := s.awaitQuestion(t, s.document)
			if p.Ask.Kind != appcore.RequestTeamDocument || p.Ask.Path != "" || p.Ask.Folder != filepath.Base(s.folder) {
				t.Fatalf("question = %+v", p.Ask)
			}
			if !slices.Equal(p.Ask.Answers, []string{"deny", "allow-document"}) {
				t.Fatalf("answers = %v, want deny and allow-document only", p.Ask.Answers)
			}
			if err := s.a.registry().Answer(p.ID, p.Revision, "choose-file", "/tmp/x"); err == nil {
				t.Fatal("choose-file was accepted")
			}
			s.requireAsks(t, s.document, "e2")
			if got := s.questions(t); len(got) != 1 {
				t.Fatalf("questions while one is open = %d, want 1", len(got))
			}
			s.answer(t, s.document, "allow-document")
			rows := s.a.rt.cfg.AIEditDocumentList()
			if len(rows) != 1 || rows[0].Document != s.document || rows[0].Scope != (config.Scope{Kind: config.ScopeTeam, Key: "team:design"}) {
				t.Fatalf("rows = %+v", rows)
			}
			s.requireRuns(t, s.document, "e3")
			s.requireAsks(t, s.neighbor, "n1")
			s.awaitQuestion(t, s.neighbor)
		})

		t.Run(host.name+": the switch off refuses everything and asks nothing", func(t *testing.T) {
			s := host.open(t)
			s.markTeam(t, "design")
			if _, _, err := s.a.rt.cfg.AllowAIEditDocument(s.document, s.a.scopeOf(s.document)); err != nil {
				t.Fatal(err)
			}
			s.a.rt.cfg.SetAIEditEnabled(false)
			for _, document := range []string{s.document, s.neighbor} {
				env := s.send(t, document, "off-"+filepath.Base(document))
				if env.Type != "wire/error" || !strings.HasPrefix(env.Text, "AI Editing is off") {
					t.Fatalf("%s: frame = %s %q, want the off refusal", filepath.Base(document), env.Type, env.Text)
				}
			}
			time.Sleep(50 * time.Millisecond)
			if got := s.questions(t); len(got) != 0 {
				t.Fatalf("questions = %+v, want none", got)
			}
		})

		t.Run(host.name+": deny stores nothing and keeps a quiet minute", func(t *testing.T) {
			s := host.open(t)
			s.markTeam(t, "design")
			s.requireAsks(t, s.document, "e1")
			s.answer(t, s.document, "deny")
			if rows := s.a.rt.cfg.AIEditDocumentList(); len(rows) != 0 {
				t.Fatalf("rows after deny = %+v, want none", rows)
			}
			s.requireAsks(t, s.document, "e2")
			time.Sleep(50 * time.Millisecond)
			if got := s.questions(t); len(got) != 0 {
				t.Fatalf("questions inside the quiet minute = %d, want 0", len(got))
			}
			s.rewindQuiet(t, s.document)
			s.requireAsks(t, s.document, "e3")
			s.awaitQuestion(t, s.document)
		})

		t.Run(host.name+": Remove Access turns it back to ask", func(t *testing.T) {
			s := host.open(t)
			s.markTeam(t, "design")
			s.requireAsks(t, s.document, "e1")
			s.answer(t, s.document, "allow-document")
			s.requireAsks(t, s.neighbor, "n1")
			s.answer(t, s.neighbor, "allow-document")
			var listed appcore.Program
			for _, program := range s.a.programFacts() {
				if program.ID == aiEditProgramID {
					listed = program
				}
			}
			if !slices.Equal(listed.AllowedDocuments, []string{s.neighbor, s.document}) {
				t.Fatalf("listed documents = %v", listed.AllowedDocuments)
			}
			args, _ := json.Marshal(map[string]string{"programID": aiEditProgramID, "documentID": s.document})
			if _, refusal := s.a.do(context.Background(), appcore.Action{Type: "program.removeAccess", Args: args}, ChannelShell); refusal != nil {
				t.Fatalf("program.removeAccess refused: %+v", refusal)
			}
			s.requireAsks(t, s.document, "e2")
			s.awaitQuestion(t, s.document)
			s.requireRuns(t, s.neighbor, "n2")
			wide, _ := json.Marshal(map[string]any{"programID": aiEditProgramID, "on": true})
			if _, refusal := s.a.do(context.Background(), appcore.Action{Type: "program.setAllTrusted", Args: wide}, ChannelShell); refusal == nil {
				t.Fatal("AI editing was widened to all trusted folders")
			}
		})
	}

	for _, host := range hosts {
		t.Run(host.name+": a team document that must ask is listed ready, and unavailable with the switch off", func(t *testing.T) {
			s := host.open(t)
			s.markTeam(t, "design")
			state := func() string {
				core, ok := s.a.wireCoreFor(s.document)
				if !ok {
					t.Fatal("no wire core for the document")
				}
				_, meta := core.HelperDiscovery(s.document)
				for _, entry := range meta {
					if entry.Name == aiEditProgramID {
						return entry.State
					}
				}
				return ""
			}
			if got := state(); got != "ready" {
				t.Fatalf("state while it must ask = %q, want ready so the page sends", got)
			}
			time.Sleep(50 * time.Millisecond)
			if got := s.questions(t); len(got) != 0 {
				t.Fatalf("reading the list asked %d questions, want 0", len(got))
			}
			s.a.rt.cfg.SetAIEditEnabled(false)
			if got := state(); got != "unavailable" {
				t.Fatalf("state with the switch off = %q, want unavailable", got)
			}
		})

		t.Run(host.name+": Remove Access stops a running edit", func(t *testing.T) {
			s := host.open(t)
			s.markTeam(t, "design")
			if _, _, err := s.a.rt.cfg.AllowAIEditDocument(s.document, s.a.scopeOf(s.document)); err != nil {
				t.Fatal(err)
			}
			terminal := make(chan wire.Envelope, 1)
			go func() {
				terminal <- s.post(t, s.document, "slow", "<p>"+strings.Repeat("x", 48*400)+"</p>")
			}()
			core, ok := s.a.wireCoreFor(s.document)
			if !ok {
				t.Fatal("no wire core for the document")
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				core.AIEdit.Mu.Lock()
				_, running := core.AIEdit.Running[s.document]
				core.AIEdit.Mu.Unlock()
				if running {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("the edit never started")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if refusal := s.a.removeProgramAccess(appcore.Effect{Type: "program.removeAccess", ProgramID: aiEditProgramID, DocumentID: s.document}); refusal != nil {
				t.Fatalf("removeProgramAccess refused: %+v", refusal)
			}
			select {
			case env := <-terminal:
				var payload struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(env.Payload, &payload)
				if env.Type != "wire/error" || payload.Code != "helper_cancelled" {
					t.Fatalf("frame = %s %s, want the cancelled refusal", env.Type, payload.Code)
				}
			case <-time.After(2 * time.Second):
				core.CancelAIEdits()
				<-terminal
				t.Fatal("the running edit was not stopped")
			}
		})
	}

	t.Run("turning the switch off in the Mac app stops a running edit", func(t *testing.T) {
		s := newLooseAIEditStand(t)
		terminal := make(chan wire.Envelope, 1)
		go func() {
			terminal <- s.post(t, s.document, "slow", "<p>"+strings.Repeat("x", 48*400)+"</p>")
		}()
		core, ok := s.a.wireCoreFor(s.document)
		if !ok {
			t.Fatal("no wire core for the document")
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			core.AIEdit.Mu.Lock()
			_, running := core.AIEdit.Running[s.document]
			core.AIEdit.Mu.Unlock()
			if running {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the edit never started")
			}
			time.Sleep(5 * time.Millisecond)
		}
		args, _ := json.Marshal(map[string]any{"key": "aiEditing", "value": false})
		if _, refusal := s.a.do(context.Background(), appcore.Action{Type: "prefs.set", Args: args}, ChannelShell); refusal != nil {
			t.Fatalf("prefs.set refused: %+v", refusal)
		}
		select {
		case env := <-terminal:
			if env.Type != "wire/error" {
				t.Fatalf("frame = %s, want the cancelled refusal", env.Type)
			}
		case <-time.After(2 * time.Second):
			core.CancelAIEdits()
			<-terminal
			t.Fatal("the running edit was not stopped")
		}
	})

	t.Run("outside the Mac app one dialog is shown at a time, and Allow stores the allow", func(t *testing.T) {
		s := newLooseAIEditStand(t)
		s.a.shell = false
		s.markTeam(t, "design")
		shown := make(chan string, 4)
		release := make(chan platform.ConfirmChoice)
		s.a.rt.confirm = func(title, message string, allowTrust bool) (platform.ConfirmChoice, error) {
			shown <- title
			return <-release, nil
		}
		s.requireAsks(t, s.document, "e1")
		select {
		case title := <-shown:
			if !strings.Contains(title, filepath.Base(s.document)) {
				t.Fatalf("dialog title = %q, want the document's name", title)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no dialog was shown")
		}
		s.requireAsks(t, s.document, "e2")
		select {
		case <-shown:
			t.Fatal("a second dialog was shown while the first was open")
		case <-time.After(100 * time.Millisecond):
		}
		release <- platform.ConfirmAllowOnce
		deadline := time.Now().Add(3 * time.Second)
		for len(s.a.rt.cfg.AIEditDocumentList()) == 0 {
			if time.Now().After(deadline) {
				t.Fatal("Allow stored nothing")
			}
			time.Sleep(5 * time.Millisecond)
		}
		s.requireRuns(t, s.document, "e3")
	})

	t.Run("an allow from another team does not apply", func(t *testing.T) {
		s := newLooseAIEditStand(t)
		s.markTeam(t, "design")
		if _, _, err := s.a.rt.cfg.AllowAIEditDocument(s.document, config.Scope{Kind: config.ScopeTeam, Key: "team:other"}); err != nil {
			t.Fatal(err)
		}
		if on, ask := s.a.aiEditFor(s.document); on || !ask {
			t.Fatalf("aiEditFor = %v, %v, want ask", on, ask)
		}
	})

	t.Run("an unreadable team record asks and stores the unknown scope", func(t *testing.T) {
		s := newLooseAIEditStand(t)
		s.markTeam(t, "design")
		record := filepath.Join(config.DirFrom(s.cfgBase), "team-folders.json")
		if _, err := os.Stat(record); err != nil {
			t.Skipf("fixture: the team record is not at %s: %v", record, err)
		}
		if err := os.WriteFile(record, []byte("{broken"), 0644); err != nil {
			t.Fatal(err)
		}
		s.requireAsks(t, s.document, "e1")
		s.answer(t, s.document, "allow-document")
		rows := s.a.rt.cfg.AIEditDocumentList()
		if len(rows) != 1 || rows[0].Scope.Kind != config.ScopeUnknown {
			t.Fatalf("rows = %+v, want one unknown-scope row", rows)
		}
		s.requireRuns(t, s.document, "e2")
	})

	t.Run("the allow survives a save and a load", func(t *testing.T) {
		s := newLooseAIEditStand(t)
		s.markTeam(t, "design")
		s.requireAsks(t, s.document, "e1")
		s.answer(t, s.document, "allow-document")
		data, err := os.ReadFile(filepath.Join(config.DirFrom(s.cfgBase), "config.json"))
		if err != nil {
			t.Skipf("fixture: config.json is not under rt.configDir: %v", err)
		}
		if !strings.Contains(string(data), `"aiEditDocuments"`) {
			t.Fatalf("config.json has no aiEditDocuments: %s", data)
		}
	})
}

func TestAIEditProgramNameMatchesTheHelper(t *testing.T) {
	if appcore.AIEditProgram != aiedit.HelperName {
		t.Fatalf("appcore.AIEditProgram = %q, helper name = %q", appcore.AIEditProgram, aiedit.HelperName)
	}
}
