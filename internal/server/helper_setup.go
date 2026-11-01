package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/config"
)

// helperSetupWait is how long a setup request may hold its response open: the
// dialog gives up after two minutes and the file picker after five.
const helperSetupWait = 8 * time.Minute

// handleHelperSetup is the page asking HTML Clay to connect one program it
// declares. The page names only the program; HTML Clay finds the file, shows
// the native dialog, and answers with a coarse state. A page can call this
// without a click, so what keeps it in check is the dialog itself (no default
// button, the real file first), one dialog per name at a time, a minute of
// quiet after a dialog that granted nothing, and Deny lasting until the person
// opens the document again.
func (s *Server) handleHelperSetup(w http.ResponseWriter, r *http.Request) {
	f, ok := s.actionDocument(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || !config.ValidHelperName(payload.Name) {
		s.writeError(w, http.StatusBadRequest, "invalid helper name")
		return
	}
	s.helperMu.Lock()
	d := s.helpers[f.AbsPath]
	declared := d != nil && d.declared[payload.Name]
	allowed := declared && d.allowed[payload.Name].ID != ""
	quiet := time.Now().Before(s.helperAskQuiet[f.AbsPath])
	s.helperMu.Unlock()
	switch {
	case !declared:
		writeHelperSetupState(w, "undeclared")
		return
	case allowed:
		writeHelperSetupState(w, "ready")
		return
	case quiet:
		writeHelperSetupState(w, "retry")
		return
	case s.hooks.AskHelper == nil:
		writeHelperSetupState(w, "unavailable")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(helperSetupWait))
	granted, ran := s.setUpHelper(f.AbsPath, payload.Name)
	switch {
	case !ran:
		writeHelperSetupState(w, "busy")
	case granted:
		writeHelperSetupState(w, "ready")
	default:
		writeHelperSetupState(w, "denied")
	}
}

func writeHelperSetupState(w http.ResponseWriter, state string) {
	noStoreJSON(w)
	fmt.Fprintf(w, `{"state":%q}`, state)
}

// setUpHelper runs the setup dialog for one file and name unless one is already
// running, then starts or refuses the requests that were waiting for it. ran is
// false when another setup for the same name was already in progress.
func (s *Server) setUpHelper(file, name string) (granted, ran bool) {
	key := helperAskKey(file, name)
	s.helperMu.Lock()
	if s.helperSetting[key] {
		s.helperMu.Unlock()
		return false, false
	}
	if s.helperSetting == nil {
		s.helperSetting = make(map[string]bool)
	}
	s.helperSetting[key] = true
	s.helperMu.Unlock()
	defer func() {
		s.helperMu.Lock()
		delete(s.helperSetting, key)
		s.helperMu.Unlock()
	}()
	return s.finishHelperAsk(file, name), true
}

// setupBarNames lists, in declaration order, the programs file declares that
// are neither allowed nor refused, which are the ones the setup bar offers.
func (s *Server) setupBarNames(file string) []string {
	s.helperMu.Lock()
	defer s.helperMu.Unlock()
	d := s.helpers[file]
	if d == nil {
		return nil
	}
	var names []string
	for _, name := range d.names {
		if d.allowed[name].ID == "" && !d.denied[name] {
			names = append(names, name)
		}
	}
	return names
}

// setupBarHTML is the bar HTML Clay puts at the top of a document whose programs
// are not set up. It names only the programs the document itself declares,
// never a file or folder, because the page can read everything in it. It sits
// inside the banner markers, so a save strips it, and carries no-save so ClayJS
// never treats it as part of the document. Names are ValidHelperName strings,
// so they need no escaping.
//
// The page's own action triggers setup too: ClayJS announces each request on
// clay:wire-state, and when that happens during a click the bar starts setup
// for that program, so pressing the page's Search button is enough.
func setupBarHTML(names []string) []byte {
	var b strings.Builder
	b.WriteString(`<div clay="no-save no-snapshot no-watch" data-htmlclay-setup style="position:fixed;top:0;left:0;right:0;z-index:2147483647;font:13px/1.4 -apple-system,system-ui,sans-serif">`)
	for _, name := range names {
		b.WriteString(`<div data-name="` + name + `" style="display:flex;align-items:center;gap:12px;padding:8px 16px;background:#f6ead0;color:#3b2f17;border-bottom:1px solid #e2cf9f">`)
		b.WriteString(`<span style="flex:1">This document wants to run a program called “` + name + `”.</span>`)
		b.WriteString(`<button type="button" data-setup style="padding:5px 12px;border:0;border-radius:6px;background:#2f6fde;color:#f7f9fc;font:inherit;cursor:pointer">Set Up “` + name + `”…</button>`)
		b.WriteString(`<button type="button" data-dismiss aria-label="Dismiss" style="padding:5px 9px;border:0;border-radius:6px;background:transparent;color:inherit;font:inherit;cursor:pointer">×</button>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<script>(function(){` +
		`var bar=document.currentScript.parentNode,busy={};` +
		`function setUp(row){var name=row.getAttribute('data-name');if(busy[name])return;busy[name]=1;var label=row.firstChild;label.textContent='Waiting for you in HTML Clay…';` +
		`fetch('/_/helper-setup/'+encodeURIComponent(document.documentElement.getAttribute('savetoken')||''),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name})})` +
		`.then(function(r){return r.json()}).then(function(d){busy[name]=0;var s=d&&d.state;` +
		`if(s==='ready'){row.remove();if(!bar.querySelector('[data-name]'))bar.remove();return}` +
		`label.textContent=s==='retry'?'Try again in a minute.':s==='busy'?'HTML Clay is already asking about “'+name+'”.':'“'+name+'” is not set up.'})` +
		`.catch(function(){busy[name]=0;label.textContent='Could not reach HTML Clay.'})}` +
		`bar.addEventListener('click',function(e){var b=e.target.closest('button');if(!b)return;var row=b.closest('[data-name]');if(b.hasAttribute('data-dismiss')){row.remove();if(!bar.querySelector('[data-name]'))bar.remove()}else setUp(row)});` +
		`document.addEventListener('clay:wire-state',function(e){var d=e.detail||{};if(!d.helper||!(navigator.userActivation&&navigator.userActivation.isActive))return;var row=bar.querySelector('[data-name="'+d.helper+'"]');if(row)setUp(row)});` +
		`})();</script>`)
	b.WriteString(`</div>`)
	return []byte(b.String())
}
