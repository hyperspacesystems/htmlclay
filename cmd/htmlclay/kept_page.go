package main

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/browser"
	"github.com/panphora/htmlclay/internal/versions"
)

// The Recovered changes page lists every edit the data guard saved aside when a
// change made outside the page removed data, and lets the person at the computer
// delete one or all recovered changes of a document. It is served exactly like
// the Programs page: only on http://127.0.0.1:<port>, only to a browser that came
// through a one-time link, with a CSRF token on every action, and with no
// JavaScript.
const keptCookie = "htmlclay_kept"

type keptRow struct {
	Folder string
	Name   string
	// Confirm is built with url.Values: a version name carries its UTC offset, and
	// a "+" left bare in a query reads back as a space.
	Confirm string
	When    string
	Size    string
}

type keptDocument struct {
	Folder  string
	File    string
	Path    string
	Status  string
	Missing bool
	Count   int
	Total   string
	Rows    []keptRow
}

type keptView struct {
	Nonce       string
	CSRF        string
	RevealLabel string
	SignedIn    bool
	Done        string
	Count       int
	Total       string
	Documents   []keptDocument

	Confirm bool
	All     bool
	File    string
	Path    string
	Missing bool
	Status  string
	Folder  string
	Name    string
	Names   []string
	When    string
	Size    string
}

func keptSize(n int64) string {
	kb := (n + 512) / 1024
	if kb < 1024 {
		return strconv.FormatInt(kb, 10) + " KB"
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// keptZone is the time zone the Recovered changes page shows times in. Tests set it
// instead of time.Local, which the app's other goroutines read through
// time.Now.
var keptZone = time.Local

func keptWhen(t time.Time) string {
	return t.In(keptZone).Format("Jan 2, 2006, 3:04 PM")
}

func keptRevealLabel() string {
	if runtime.GOOS == "darwin" {
		return "Show in Finder"
	}
	return "Show in Folder"
}

// openKeptPage opens the Recovered changes page in the browser through a
// one-time link. The link is never logged: it is the credential.
func (a *app) openKeptPage() {
	ticket, err := a.kept.mintTicket(time.Now())
	if err != nil {
		a.rt.logger.Printf("Could not open the Recovered changes page: %v", err)
		return
	}
	a.rt.logger.Printf("Opening the Recovered changes page")
	open := a.rt.openKeptURL
	if open == nil {
		open = browser.OpenURL
	}
	if err := open(fmt.Sprintf("http://127.0.0.1:%d/recovered/open?t=%s", a.rt.port, ticket)); err != nil {
		a.rt.logger.Printf("Error opening the Recovered changes page: %v", err)
	}
}

// serveKept answers every /recovered path on the bare 127.0.0.1 host. The old
// /kept paths send a page that was open across the update on to the new ones.
func (a *app) serveKept(w http.ResponseWriter, r *http.Request) {
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
	if r.URL.Path == "/kept" || strings.HasPrefix(r.URL.Path, "/kept/") {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/recovered"+strings.TrimPrefix(r.URL.Path, "/kept")+queryOf(r.URL.RawQuery), http.StatusFound)
		return
	}
	switch r.URL.Path {
	case "/recovered/open":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "none" {
			a.renderKeptSignIn(w, nonce)
			return
		}
		sid, ok := a.kept.redeem(r.URL.Query().Get("t"), now)
		if !ok {
			a.renderKeptSignIn(w, nonce)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: keptCookie, Value: sid, Path: "/recovered", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/recovered", http.StatusSeeOther)
	case "/recovered":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		csrf, ok := a.kept.session(r, keptCookie, now)
		if !ok {
			a.renderKeptSignIn(w, nonce)
			return
		}
		a.renderKept(w, nonce, csrf, r.URL.Query().Get("done"))
	case "/recovered/confirm":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		csrf, ok := a.kept.session(r, keptCookie, now)
		if !ok {
			a.renderKeptSignIn(w, nonce)
			return
		}
		folder := r.URL.Query().Get("folder")
		name := r.URL.Query().Get("name")
		all := r.URL.Query().Get("all") == "1"
		if !a.renderKeptConfirm(w, nonce, csrf, folder, name, all) {
			http.Redirect(w, r, "/recovered?done=gone", http.StatusSeeOther)
		}
	case "/recovered/action":
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		origin := fmt.Sprintf("http://127.0.0.1:%d", a.rt.port)
		if r.Header.Get("Origin") != origin || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		csrf, ok := a.kept.session(r, keptCookie, now)
		if !ok {
			a.renderKeptSignIn(w, nonce)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, programsFormLimit)
		if err := r.ParseForm(); err != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(csrf)) != 1 {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		done := a.keptAction(r.PostForm.Get("action"), r.PostForm.Get("folder"), r.PostForm.Get("name"), r.PostForm["name"])
		http.Redirect(w, r, "/recovered?done="+done, http.StatusSeeOther)
	default:
		http.NotFound(w, r)
	}
}

// keptDone maps the outcome keys an action redirects with to the sentence the
// page shows. An empty sentence shows nothing.
var keptDone = map[string]string{
	"deleted":     "Deleted that recovered change.",
	"deleted-all": "Deleted all recovered changes for that document.",
	"revealed":    "",
	"gone":        "That recovered change is already gone. The list below is current.",
	"failed":      "That did not work. Open Settings and check the app log for details.",
}

// keptAction runs one form action and returns its outcome key.
func (a *app) keptAction(action, folder, name string, names []string) string {
	var err error
	outcome := "failed"
	switch action {
	case "delete":
		err = a.rt.versions.DeleteKept(folder, name)
		outcome = "deleted"
	case "delete-all":
		if len(names) == 0 {
			return "gone"
		}
		_, err = a.rt.versions.DeleteKeptNames(folder, names)
		outcome = "deleted-all"
	case "reveal":
		var path string
		path, err = a.rt.versions.KeptPath(folder, name)
		if err == nil {
			reveal := a.rt.revealKept
			if reveal == nil {
				reveal = browser.Reveal
			}
			err = reveal(path)
		}
		outcome = "revealed"
	default:
		return "failed"
	}
	if err != nil {
		if errors.Is(err, versions.ErrNotKept) || errors.Is(err, versions.ErrNoHistory) {
			return "gone"
		}
		a.rt.logger.Printf("Recovered changes action %q failed: %v", action, err)
		return "failed"
	}
	a.refreshRecovered()
	return outcome
}

func (a *app) keptDocuments() ([]keptDocument, int, int64) {
	histories, err := a.rt.versions.ListKept()
	if err != nil {
		a.rt.logger.Printf("Could not list recovered changes: %v", err)
		return nil, 0, 0
	}
	var docs []keptDocument
	totalCount := 0
	var totalSize int64
	for _, h := range histories {
		doc := keptDocument{Folder: h.Folder, File: h.Name}
		if h.AbsPath != "" {
			doc.File = filepath.Base(h.AbsPath)
			doc.Path = a.tildePath(h.AbsPath)
			if _, err := os.Stat(h.AbsPath); err != nil {
				doc.Missing = true
				doc.Status = "Original file not found here"
			}
		} else {
			doc.Path = h.Name
		}
		var size int64
		for _, e := range h.Entries {
			size += e.Size
			doc.Rows = append(doc.Rows, keptRow{
				Folder:  h.Folder,
				Name:    e.Name,
				Confirm: "/recovered/confirm?" + url.Values{"folder": {h.Folder}, "name": {e.Name}}.Encode(),
				When:    keptWhen(e.Time),
				Size:    keptSize(e.Size),
			})
		}
		doc.Count = len(h.Entries)
		doc.Total = keptSize(size)
		totalCount += doc.Count
		totalSize += size
		docs = append(docs, doc)
	}
	return docs, totalCount, totalSize
}

func (a *app) renderKept(w http.ResponseWriter, nonce, csrf, done string) {
	docs, count, size := a.keptDocuments()
	view := keptView{
		Nonce:       nonce,
		CSRF:        csrf,
		RevealLabel: keptRevealLabel(),
		SignedIn:    true,
		Done:        keptDone[done],
		Count:       count,
		Total:       keptSize(size),
		Documents:   docs,
	}
	a.writeKeptPage(w, view)
}

func (a *app) renderKeptConfirm(w http.ResponseWriter, nonce, csrf, folder, name string, all bool) bool {
	docs, _, _ := a.keptDocuments()
	for _, doc := range docs {
		if doc.Folder != folder {
			continue
		}
		view := keptView{
			Nonce:    nonce,
			CSRF:     csrf,
			SignedIn: true,
			Confirm:  true,
			All:      all,
			File:     doc.File,
			Path:     doc.Path,
			Missing:  doc.Missing,
			Status:   doc.Status,
			Folder:   doc.Folder,
			Count:    doc.Count,
			Total:    doc.Total,
			Name:     name,
		}
		if all {
			for _, row := range doc.Rows {
				view.Names = append(view.Names, row.Name)
			}
		}
		if !all {
			found := false
			for _, row := range doc.Rows {
				if row.Name == name {
					view.When, view.Size, found = row.When, row.Size, true
					break
				}
			}
			if !found {
				return false
			}
		}
		a.writeKeptPage(w, view)
		return true
	}
	return false
}

func (a *app) renderKeptSignIn(w http.ResponseWriter, nonce string) {
	w.WriteHeader(http.StatusForbidden)
	a.writeKeptPage(w, keptView{Nonce: nonce})
}

func (a *app) writeKeptPage(w http.ResponseWriter, view keptView) {
	var buf bytes.Buffer
	if err := keptTemplate.Execute(&buf, view); err != nil {
		a.rt.logger.Printf("Could not render the Recovered changes page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

// keptPath reports whether path is under /recovered, one of the paths the bare
// 127.0.0.1 host serves besides its index page, or under the page's old /kept
// prefix, which serveKept sends on to /recovered.
func keptPath(path string) bool {
	for _, prefix := range []string{"/recovered", "/kept"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// queryOf is ?query when there is one, for a redirect that keeps it.
func queryOf(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	return "?" + rawQuery
}
