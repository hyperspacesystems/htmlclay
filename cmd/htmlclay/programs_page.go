package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/browser"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// The Programs page lists every program HTML Clay may run and what may run it.
// It is served only on http://127.0.0.1:<port>, a different site from every
// document origin (*.localhost), and only to a browser that came through a
// one-time link the tray opened. It has no JavaScript. Actions that take access
// away happen directly; anything that gives access goes through a native dialog,
// so nothing a web page sends can grant a program.
const (
	programsCookie      = "htmlclay_programs"
	programsTicketTTL   = time.Minute
	programsTicketMax   = 8
	programsSessionIdle = 30 * time.Minute
	programsSessionMax  = 12 * time.Hour
	programsSessionCap  = 4
	programsFormLimit   = 4096
)

type programsAuth struct {
	mu       sync.Mutex
	tickets  map[string]time.Time
	sessions map[string]*programsSession
}

type programsSession struct {
	csrf    string
	created time.Time
	seen    time.Time
}

func randomHex() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (p *programsAuth) mintTicket(now time.Time) (string, error) {
	ticket, err := randomHex()
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tickets == nil {
		p.tickets = make(map[string]time.Time)
	}
	for t, expires := range p.tickets {
		if !now.Before(expires) {
			delete(p.tickets, t)
		}
	}
	if len(p.tickets) >= programsTicketMax {
		return "", fmt.Errorf("too many Programs page links are waiting to be opened")
	}
	p.tickets[ticket] = now.Add(programsTicketTTL)
	return ticket, nil
}

// redeem burns ticket and, when it was live, starts a session.
func (p *programsAuth) redeem(ticket string, now time.Time) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	expires, ok := p.tickets[ticket]
	delete(p.tickets, ticket)
	if !ok || !now.Before(expires) {
		return "", false
	}
	sid, err := randomHex()
	if err != nil {
		return "", false
	}
	csrf, err := randomHex()
	if err != nil {
		return "", false
	}
	if p.sessions == nil {
		p.sessions = make(map[string]*programsSession)
	}
	p.pruneLocked(now)
	for len(p.sessions) >= programsSessionCap {
		oldest, at := "", now
		for id, s := range p.sessions {
			if s.seen.Before(at) || oldest == "" {
				oldest, at = id, s.seen
			}
		}
		delete(p.sessions, oldest)
	}
	p.sessions[sid] = &programsSession{csrf: csrf, created: now, seen: now}
	return sid, true
}

func (p *programsAuth) pruneLocked(now time.Time) {
	for id, s := range p.sessions {
		if now.Sub(s.seen) > programsSessionIdle || now.Sub(s.created) > programsSessionMax {
			delete(p.sessions, id)
		}
	}
}

// session returns the CSRF token of the request's live session.
func (p *programsAuth) session(r *http.Request, now time.Time) (string, bool) {
	cookie, err := r.Cookie(programsCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked(now)
	s, ok := p.sessions[cookie.Value]
	if !ok {
		return "", false
	}
	s.seen = now
	return s.csrf, true
}

// openProgramsPage opens the Programs page in the browser through a one-time
// link. The link is never logged: it is the credential.
func (a *app) openProgramsPage() {
	ticket, err := a.programs.mintTicket(time.Now())
	if err != nil {
		a.reportHelperProgramError("Could not open the Programs page", err)
		return
	}
	a.rt.logger.Printf("Opening the Programs page")
	open := a.rt.openProgramsURL
	if open == nil {
		open = browser.OpenURL
	}
	if err := open(fmt.Sprintf("http://127.0.0.1:%d/programs/open?t=%s", a.rt.port, ticket)); err != nil {
		a.rt.logger.Printf("Error opening the Programs page: %v", err)
	}
}

func programsHeaders(w http.ResponseWriter, nonce string) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+nonce+"'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	// same-origin, not no-referrer: under no-referrer a browser sends a form
	// POST with Origin: null, which the action check refuses.
	h.Set("Referrer-Policy", "same-origin")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
}

// servePrograms answers every /programs path on the bare 127.0.0.1 host.
func (a *app) servePrograms(w http.ResponseWriter, r *http.Request) {
	nonce, err := randomHex()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	programsHeaders(w, nonce)
	if r.Method == http.MethodOptions {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now()
	switch r.URL.Path {
	case "/programs/open":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "none" {
			a.renderProgramsSignIn(w, nonce)
			return
		}
		sid, ok := a.programs.redeem(r.URL.Query().Get("t"), now)
		if !ok {
			a.renderProgramsSignIn(w, nonce)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: programsCookie, Value: sid, Path: "/programs", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/programs", http.StatusSeeOther)
	case "/programs":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		csrf, ok := a.programs.session(r, now)
		if !ok {
			a.renderProgramsSignIn(w, nonce)
			return
		}
		a.renderPrograms(w, nonce, csrf, r.URL.Query().Get("done"))
	case "/programs/action":
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		csrf, ok := a.programs.session(r, now)
		origin := fmt.Sprintf("http://127.0.0.1:%d", a.rt.port)
		if !ok || r.Header.Get("Origin") != origin || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, programsFormLimit)
		if err := r.ParseForm(); err != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(csrf)) != 1 {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		// Relink and widen wait on a native dialog (two minutes) and the file
		// picker (five), longer than the listener's write timeout.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(8 * time.Minute))
		done := a.programsAction(r.PostForm.Get("action"), r.PostForm.Get("program"), r.PostForm.Get("document"), r.PostForm.Get("name"))
		http.Redirect(w, r, "/programs?done="+done, http.StatusSeeOther)
	default:
		http.NotFound(w, r)
	}
}

// programsDone maps the outcome keys an action redirects with to the sentence
// the page shows, so the URL never carries text a link could forge.
var programsDone = map[string]string{
	"revoked":   "Access removed. Open documents stop using it right away.",
	"relinked":  "The program now points at the new file.",
	"widened":   "Allowed for documents in your trusted folders.",
	"removed":   "Removed from the list. No file on your Mac was deleted.",
	"unchanged": "Nothing changed.",
	"failed":    "That did not work. HTML Clay's log has the details.",
}

// programsAction runs one form action and returns its outcome key. Revoking,
// narrowing and removing happen here directly; relinking and widening only ask,
// through a native dialog the page cannot answer.
func (a *app) programsAction(action, programID, document, name string) string {
	a.helperMu.Lock()
	defer a.helperMu.Unlock()
	program, found := a.rt.cfg.LookupHelperProgram(programID)
	var err error
	outcome := "unchanged"
	switch action {
	case "revoke-document":
		if document == "" || !config.ValidHelperName(name) {
			return "failed"
		}
		a.helperStateMu.Lock()
		previous, had, rerr := a.rt.cfg.RevokeHelper(document, name)
		if err = rerr; err == nil {
			if err = a.rt.cfg.Save(); err != nil {
				a.rt.cfg.ForgetHelperDecision(document, name)
				if had {
					a.rt.cfg.RestoreHelperDecisions([]config.HelperDecision{previous})
				}
			}
		}
		a.helperStateMu.Unlock()
		outcome = "revoked"
	case "revoke-trusted":
		if !found {
			return "failed"
		}
		a.helperStateMu.Lock()
		previous, _ := a.rt.cfg.SetHelperAnyDocument(programID, false)
		if err = a.rt.cfg.Save(); err != nil {
			a.rt.cfg.SetHelperAnyDocument(programID, previous)
		}
		a.helperStateMu.Unlock()
		outcome = "revoked"
	case "revoke-all":
		if !found {
			return "failed"
		}
		a.helperStateMu.Lock()
		var flags = make(map[string]bool)
		var removed []config.HelperDecision
		for _, p := range a.rt.cfg.HelperProgramList() {
			if filepath.Clean(p.Path) != filepath.Clean(program.Path) {
				continue
			}
			flags[p.ID], _ = a.rt.cfg.SetHelperAnyDocument(p.ID, false)
			removed = append(removed, a.rt.cfg.ForgetHelperProgramDecisions(p.ID)...)
		}
		// Each document that lost access remembers it, as a single Remove Access
		// does, so it stops asking instead of offering setup again.
		for _, d := range removed {
			if _, _, err = a.rt.cfg.RevokeHelper(d.Document, d.Name); err != nil {
				break
			}
		}
		if err == nil {
			err = a.rt.cfg.Save()
		}
		if err != nil {
			for _, d := range removed {
				a.rt.cfg.ForgetHelperDecision(d.Document, d.Name)
			}
			for id, previous := range flags {
				a.rt.cfg.SetHelperAnyDocument(id, previous)
			}
			a.rt.cfg.RestoreHelperDecisions(removed)
		}
		a.helperStateMu.Unlock()
		outcome = "revoked"
	case "remove":
		if !found || !helperProgramMissing(program.Path) {
			return "failed"
		}
		a.helperStateMu.Lock()
		removed := a.rt.cfg.ForgetHelperProgramDecisions(programID)
		a.rt.cfg.RemoveHelperProgram(programID)
		// The documents that used it keep a refusal, so none of them falls
		// through to another program with the same name without asking.
		for _, d := range removed {
			if _, _, err = a.rt.cfg.RevokeHelper(d.Document, d.Name); err != nil {
				break
			}
		}
		if err == nil {
			err = a.rt.cfg.Save()
		}
		if err != nil {
			for _, d := range removed {
				a.rt.cfg.ForgetHelperDecision(d.Document, d.Name)
			}
			a.rt.cfg.RestoreHelperProgram(program)
			a.rt.cfg.RestoreHelperDecisions(removed)
		}
		a.helperStateMu.Unlock()
		outcome = "removed"
	case "relink":
		if !found {
			return "failed"
		}
		outcome, err = a.relinkHelperProgram(program, a.systemHelperApprovalDialogs())
	case "widen":
		if !found || helperProgramMissing(program.Path) {
			return "failed"
		}
		outcome, err = a.widenHelperProgram(program, a.systemHelperApprovalDialogs())
	default:
		return "failed"
	}
	if err != nil {
		a.rt.logger.Printf("Programs page action %s failed: %v", action, err)
		return "failed"
	}
	if outcome != "unchanged" {
		a.rt.logger.Printf("Programs page action %s: program=%s document=%s name=%s", action, programID, document, name)
		go a.refreshHelperDispatchers()
	}
	return outcome
}

// relinkHelperProgram points a program whose file is missing at a file of the
// same name in a folder the person keeps programs in, or at one they pick, once
// they confirm. The program keeps its ID, so the documents allowed to use it
// keep working. Caller holds helperMu.
func (a *app) relinkHelperProgram(program config.HelperProgram, dialogs helperApprovalDialogs) (string, error) {
	found, _ := a.scanKnownProgramFolders(a.knownProgramFolders("", a.rt.cfg.HelperProgramList()), filepath.Base(program.Path), helperSuggestNow())
	path := ""
	if len(found) == 1 {
		path = found[0]
	}
	message := fmt.Sprintf("“%s” used %s, which is no longer there.", program.Name, a.tildePath(program.Path))
	affirm := helperSetupChooseFile
	if path != "" {
		message += fmt.Sprintf("\n\nHTML Clay found a file with the same name:\n\n    %s\n\nEvery document allowed to use “%s” will run this file instead.", a.tildePath(path), program.Name)
		affirm = "Allow File"
	} else {
		message += fmt.Sprintf("\n\nChoose the file to use instead. Every document allowed to use “%s” will run it.", program.Name)
	}
	message += "\n\nThe program runs as you and can read or change any file your account can access."
	var shown os.FileInfo
	if path != "" {
		shown, _ = os.Stat(path)
	}
	choice, err := dialogs.setup("Use a moved program for “"+program.Name+"”?", message, helperSetupCancel, helperSetupChooseOther, affirm)
	if err != nil {
		return "", err
	}
	switch {
	case choice == platform.ChoiceAffirm && path != "":
		again, _ := a.scanKnownProgramFolders(a.knownProgramFolders("", a.rt.cfg.HelperProgramList()), filepath.Base(program.Path), helperSuggestNow())
		if len(again) != 1 || again[0] != path || !sameFileAsShown(path, shown) {
			return "", fmt.Errorf("the file for %s changed while the dialog was open", program.Name)
		}
	case choice == platform.ChoiceMiddle || choice == platform.ChoiceAffirm:
		picked, ok, err := a.chooseHelperFile(dialogs, "", "Choose the program for “"+program.Name+"”", filepath.Dir(program.Path))
		if err != nil {
			return "", err
		}
		if !ok {
			return "unchanged", nil
		}
		path = picked
	default:
		return "unchanged", nil
	}
	if ok, err := dialogs.prepare(path); err != nil || !ok {
		return "unchanged", err
	}
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	previous, ok := a.rt.cfg.SetHelperProgramPath(program.ID, path)
	if !ok {
		return "", fmt.Errorf("program %s is no longer registered", program.ID)
	}
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetHelperProgramPath(program.ID, previous)
		return "", err
	}
	return "relinked", nil
}

// widenHelperProgram lets every document in a trusted folder that asks for the
// program's name run it, once the person confirms. Caller holds helperMu.
func (a *app) widenHelperProgram(program config.HelperProgram, dialogs helperApprovalDialogs) (string, error) {
	message := fmt.Sprintf("Allow %s for every document in your trusted folders that asks for “%s”?\n\n"+
		"Those documents will run it with their own input, without asking you. Documents outside your trusted folders still ask, one at a time.", a.tildePath(program.Path), program.Name)
	choice, err := dialogs.setup("Allow “"+program.Name+"” in trusted folders?", message, helperSetupCancel, "Keep Asking", helperSetupTrusted)
	if err != nil || choice != platform.ChoiceAffirm {
		return "unchanged", err
	}
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	previous, ok := a.rt.cfg.SetHelperAnyDocument(program.ID, true)
	if !ok {
		return "", fmt.Errorf("program %s is no longer registered", program.ID)
	}
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetHelperAnyDocument(program.ID, previous)
		return "", err
	}
	return "widened", nil
}

type programsGrant struct {
	Label    string
	Detail   string
	Document string
	Name     string
	Program  string
	Trusted  bool
}

type programsRegistration struct {
	ID          string
	Name        string
	AnyDocument bool
}

type programsGroup struct {
	File          string
	Path          string
	Status        string
	Missing       bool
	Registrations []programsRegistration
	Grants        []programsGrant
	Relink        string
}

type programsView struct {
	Nonce    string
	CSRF     string
	Done     string
	Missing  []programsGroup
	Groups   []programsGroup
	Open     []programsOpenRow
	SignedIn bool
}

type programsOpenRow struct {
	Document string
	Name     string
	Status   string
}

func (a *app) programsViewModel() programsView {
	programs := a.rt.cfg.HelperProgramList()
	decisions := a.rt.cfg.HelperDecisionList()
	byPath := make(map[string]*programsGroup)
	var order []string
	for _, p := range programs {
		key := filepath.Clean(p.Path)
		g, ok := byPath[key]
		if !ok {
			g = &programsGroup{File: filepath.Base(p.Path), Path: a.tildePath(p.Path), Missing: helperProgramMissing(p.Path)}
			switch {
			case g.Missing:
				g.Status = "File missing"
			case runtime.GOOS != "windows" && !helperProgramExecutable(p.Path):
				g.Status = "Not executable"
			default:
				g.Status = "Ready"
			}
			if g.Missing {
				if found, _ := a.scanKnownProgramFolders(a.knownProgramFolders("", programs), filepath.Base(p.Path), helperSuggestNow()); len(found) == 1 {
					g.Relink = a.tildePath(found[0])
				}
			}
			byPath[key] = g
			order = append(order, key)
		}
		g.Registrations = append(g.Registrations, programsRegistration{ID: p.ID, Name: p.Name, AnyDocument: p.AnyDocument})
		if p.AnyDocument {
			g.Grants = append(g.Grants, programsGrant{Label: "All documents in trusted folders", Detail: "for “" + p.Name + "”", Program: p.ID, Trusted: true})
		}
		for _, d := range decisions {
			if d.Allowed && d.Program == p.ID {
				g.Grants = append(g.Grants, programsGrant{Label: filepath.Base(d.Document), Detail: "for “" + d.Name + "”, " + a.tildePath(d.Document), Document: d.Document, Name: d.Name, Program: p.ID})
			}
		}
	}
	view := programsView{}
	for _, key := range order {
		if byPath[key].Missing {
			view.Missing = append(view.Missing, *byPath[key])
		} else {
			view.Groups = append(view.Groups, *byPath[key])
		}
	}
	for _, row := range a.helperDocumentRows() {
		document, name, _ := strings.Cut(row.Path, "\x00")
		view.Open = append(view.Open, programsOpenRow{Document: a.tildePath(document), Name: name, Status: a.helperBindingStatus(document, name)})
	}
	sort.SliceStable(view.Open, func(i, j int) bool { return view.Open[i].Document < view.Open[j].Document })
	return view
}

func (a *app) renderPrograms(w http.ResponseWriter, nonce, csrf, done string) {
	view := a.programsViewModel()
	view.Nonce, view.CSRF, view.SignedIn = nonce, csrf, true
	view.Done = programsDone[done]
	a.writeProgramsPage(w, view)
}

func (a *app) renderProgramsSignIn(w http.ResponseWriter, nonce string) {
	w.WriteHeader(http.StatusForbidden)
	a.writeProgramsPage(w, programsView{Nonce: nonce})
}

func (a *app) writeProgramsPage(w http.ResponseWriter, view programsView) {
	var buf bytes.Buffer
	if err := programsTemplate.Execute(&buf, view); err != nil {
		a.rt.logger.Printf("Could not render the Programs page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

// programsPath reports whether path is under /programs, the only paths the
// bare 127.0.0.1 host serves besides its index page.
func programsPath(path string) bool {
	return path == "/programs" || strings.HasPrefix(path, "/programs/")
}
