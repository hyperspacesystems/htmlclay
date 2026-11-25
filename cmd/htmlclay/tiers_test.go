package main

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/testutil"
	"github.com/panphora/htmlclay/internal/tier"
)

const tierTestPage = `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js"></script></head><body><p>Old</p></body></html>`

// madeHere records documents as the person's own, as HTML Clay does for a
// document it creates.
func madeHere(t *testing.T, a *app, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := a.tierStore().TrustPath(real(t, path), false, tier.HowCreated); err != nil {
			t.Fatal(err)
		}
	}
}

// downloadedHere records that HTML Clay first saw a document carrying a
// download mark, which is what the first sight of a download writes on every
// system that marks one.
func downloadedHere(t *testing.T, a *app, path string) {
	t.Helper()
	if _, err := a.tierStore().NoteSeen(real(t, path), true); err != nil {
		t.Fatal(err)
	}
}

type tierStand struct {
	app    *app
	folder string
}

func newTierStand(t *testing.T) tierStand {
	t.Helper()
	a := newShellTestApp(t, t.TempDir())
	t.Cleanup(a.stopRequests)
	folder := real(t, t.TempDir())
	a.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))
	return tierStand{app: a, folder: folder}
}

func (s tierStand) write(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(s.folder, name)
	writeTestFile(t, path, tierTestPage)
	return path
}

func (s tierStand) warnings(document string) []confirm.Pending {
	var out []confirm.Pending
	for _, p := range s.app.registry().Pending() {
		if p.Ask.Kind == appcore.RequestUnknownDocument && p.Ask.Document == document {
			out = append(out, p)
		}
	}
	return out
}

func (s tierStand) awaitWarning(t *testing.T, document string) confirm.Pending {
	t.Helper()
	var found []confirm.Pending
	waitFor(t, "the unknown document warning for "+filepath.Base(document), func() bool {
		found = s.warnings(document)
		return len(found) > 0
	})
	return found[0]
}

// askForUse runs the setup bar's ask in the background: it waits for the person's
// answers, so the test answers them and then reads what it granted.
func (s tierStand) askForUse(document, name string) <-chan bool {
	granted := make(chan bool, 1)
	go func() { granted <- s.app.askHelperForUse(document, name) }()
	return granted
}

func received(t *testing.T, granted <-chan bool) bool {
	t.Helper()
	select {
	case ok := <-granted:
		return ok
	case <-time.After(5 * time.Second):
		t.Fatal("the setup ask did not return after its question was answered")
		return false
	}
}

func (s tierStand) answer(t *testing.T, document, choice string) {
	t.Helper()
	p := s.awaitWarning(t, document)
	if err := s.app.registry().Answer(p.ID, p.Revision, choice, ""); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	waitFor(t, "the warning flow to finish", func() bool {
		s.app.helperStateMu.Lock()
		defer s.app.helperStateMu.Unlock()
		return !s.app.documentAsking[document]
	})
}

func TestUnknownDocumentWarningGatesProgramsAndAI(t *testing.T) {
	s := newTierStand(t)
	doc := s.write(t, "invoice.html")
	downloadedHere(t, s.app, doc)

	s.app.rt.cfg.SetBuiltinAllTrusted(builtin.FileSearch, true)
	if got := s.app.builtinFor(doc, builtin.FileSearch).State; got != builtin.Ask {
		t.Fatalf("built-in program for a download with All documents in my trusted folders on = %q, want ask", got)
	}
	if on, ask := s.app.aiEditFor(doc); on || !ask {
		t.Fatalf("aiEditFor a download = %v, %v, want off and ask", on, ask)
	}
	if s.app.aiEditFiles(doc) {
		t.Fatal("a download may attach its sibling files")
	}

	s.app.askBuiltin(doc, builtin.FileSearch)
	s.app.askAIEdit(doc)
	s.app.askBuiltin(doc, builtin.FileConvert)
	p := s.awaitWarning(t, doc)
	if got := s.warnings(doc); len(got) != 1 {
		t.Fatalf("warnings for one document = %d, want 1", len(got))
	}
	if p.Ask.Folder != filepath.Base(s.folder) || p.Ask.Path != s.folder || p.Ask.Detail != tier.ReasonDownloaded {
		t.Fatalf("warning = %+v, want the folder it can reach and why", p.Ask)
	}
	if !slices.Equal(p.Ask.Answers, []string{"deny-document", "allow-document", "trust-document"}) {
		t.Fatalf("answers = %v", p.Ask.Answers)
	}
	for _, other := range []string{"allow-all-trusted", "choose-file"} {
		if err := s.app.registry().Answer(p.ID, p.Revision, other, ""); err == nil {
			t.Fatalf("%s was accepted", other)
		}
	}

	s.answer(t, doc, helperAnswerAllowDocument)
	if got := s.app.builtinFor(doc, builtin.FileSearch).State; got != builtin.Ready {
		t.Fatalf("built-in program after Allow = %q, want ready", got)
	}
	if on, _ := s.app.aiEditFor(doc); !on {
		t.Fatal("AI editing is still off after Allow")
	}
	if s.app.aiEditFiles(doc) {
		t.Fatal("Allow for this document let it attach sibling files; only trust does")
	}
}

func TestTrustItLikeAFileIMade(t *testing.T) {
	s := newTierStand(t)
	doc := s.write(t, "kanban.html")
	downloadedHere(t, s.app, doc)
	s.app.askAIEdit(doc)
	s.answer(t, doc, documentAnswerTrust)

	if v := s.app.tierOf(doc); v.Tier != tier.Mine {
		t.Fatalf("after trust the document is %s", v.Tier)
	}
	if !s.app.aiEditFiles(doc) {
		t.Fatal("a trusted document may not attach its sibling files")
	}
	elsewhere := filepath.Join(real(t, t.TempDir()), "kanban copy.html")
	writeTestFile(t, elsewhere, tierTestPage)
	downloadedHere(t, s.app, elsewhere)
	if v := s.app.tierOf(elsewhere); v.Tier != tier.Mine || v.Reason != tier.ReasonTrustedCode {
		t.Fatalf("a second download of the same code is %s %s, want mine by its code", v.Tier, v.Reason)
	}
}

func TestEditedCodeLosesItsTrustAtOnce(t *testing.T) {
	s := newTierStand(t)
	doc := s.write(t, "app.html")
	downloadedHere(t, s.app, doc)
	if err := s.app.tierStore().TrustCode(s.app.tierOf(doc).Code, tier.HowCommand); err != nil {
		t.Fatal(err)
	}
	if v := s.app.tierOf(doc); v.Tier != tier.Mine || v.Reason != tier.ReasonTrustedCode {
		t.Fatalf("a download whose code is trusted is %s %s", v.Tier, v.Reason)
	}
	edited := strings.Replace(tierTestPage, "clay.js", "evil.js", 1)
	if len(edited) != len(tierTestPage) || edited == tierTestPage {
		t.Fatal("the edit must change the code and keep the size")
	}
	writeTestFile(t, doc, edited)
	if v := s.app.tierOf(doc); v.Tier != tier.Unknown {
		t.Fatalf("after its code changed the download is %s %s, want unknown", v.Tier, v.Reason)
	}
}

func TestDontAllowStaysQuiet(t *testing.T) {
	s := newTierStand(t)
	doc := s.write(t, "game.html")
	downloadedHere(t, s.app, doc)
	s.app.askBuiltin(doc, builtin.FileSearch)
	s.answer(t, doc, documentAnswerDeny)
	if got := s.app.builtinFor(doc, builtin.FileSearch).State; got != builtin.Ask {
		t.Fatalf("state after Don't allow = %q, want ask", got)
	}
	if v := s.app.tierOf(doc); !v.Denied {
		t.Fatal("Don't allow was not stored, so it would ask again after a restart")
	}
	s.app.helperStateMu.Lock()
	s.app.documentQuiet = nil
	s.app.helperStateMu.Unlock()
	writeTestFile(t, doc, strings.Replace(tierTestPage, "clay.js", "new.js", 1))
	s.app.askAIEdit(doc)
	time.Sleep(50 * time.Millisecond)
	if got := s.warnings(doc); len(got) != 0 {
		t.Fatalf("Don't allow asked again %d times after the page changed its code", len(got))
	}

	closed := s.write(t, "closed.html")
	downloadedHere(t, s.app, closed)
	s.app.askAIEdit(closed)
	p := s.awaitWarning(t, closed)
	s.app.registry().Close(p.ID)
	waitFor(t, "the closed warning to finish", func() bool {
		s.app.helperStateMu.Lock()
		defer s.app.helperStateMu.Unlock()
		return !s.app.documentAsking[closed]
	})
	if v := s.app.tierOf(closed); v.Denied {
		t.Fatal("a warning closed without an answer was stored as Don't allow")
	}
}

func TestRegisteredProgramsWaitForTheWarning(t *testing.T) {
	s := newTierStand(t)
	doc := filepath.Join(s.folder, "tool.html")
	writeTestFile(t, doc, helperPage)
	downloadedHere(t, s.app, doc)
	program, err := s.app.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0o755))
	if err != nil {
		t.Fatal(err)
	}
	s.app.rt.cfg.SetHelperAnyDocument(program.ID, true)
	if allowed, _ := s.app.resolvedHelpers(doc, []string{"search"}); len(allowed) != 0 {
		t.Fatalf("a download resolved a granted program before the warning: %+v", allowed)
	}
	if s.app.helperGranted(doc, "search", program.ID) {
		t.Fatal("a download may start a granted program before the warning")
	}
	granted := s.askForUse(doc, "search")
	s.awaitWarning(t, real(t, doc))
	select {
	case <-granted:
		t.Fatal("asking for the program finished before the warning was answered")
	default:
	}
	s.answer(t, real(t, doc), helperAnswerAllowDocument)
	if !received(t, granted) {
		t.Fatal("Allow for this document did not carry on to the program allowed for trusted documents")
	}
	if !s.app.helperGranted(doc, "search", program.ID) {
		t.Fatal("the granted program still refuses after Allow for this document")
	}
}

func TestTheirOwnDocumentsAskNothing(t *testing.T) {
	s := newTierStand(t)
	doc := s.write(t, "notes.html")
	madeHere(t, s.app, doc)
	if got := s.app.builtinFor(doc, builtin.FileSearch).State; got != builtin.Ready {
		t.Fatalf("built-in program for the person's own document = %q, want ready", got)
	}
	if on, ask := s.app.aiEditFor(doc); !on || ask {
		t.Fatalf("aiEditFor the person's own document = %v, %v", on, ask)
	}
	if !s.app.aiEditFiles(doc) {
		t.Fatal("the person's own document may not attach its sibling files")
	}
	s.app.tierStore().SetDeveloper(true)
	fresh := s.write(t, "fresh.html")
	if v := s.app.tierOf(fresh); v.Tier != tier.Mine {
		t.Fatalf("in developer mode an unmarked new file is %s %s", v.Tier, v.Reason)
	}
	s.app.tierStore().SetDeveloper(false)
	if v := s.app.tierOf(fresh); v.Tier != tier.Mine {
		t.Fatalf("a file first seen in developer mode is %s once it is off", v.Tier)
	}
	unseen := s.write(t, "unseen.html")
	if v := s.app.tierOf(unseen); v.Tier != tier.Unknown || v.Reason != tier.ReasonUnrecorded {
		t.Fatalf("a new unmarked file outside developer mode is %s %s, want unknown", v.Tier, v.Reason)
	}
}

func TestUnknownLooseDocumentHasItsOwnOrigin(t *testing.T) {
	a := newTestApp(t, evalTempHome(t))
	folder := filepath.Join(a.rt.home, "Downloads")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(folder, "mine.html")
	stranger := filepath.Join(folder, "stranger.html")
	later := filepath.Join(folder, "later.html")
	for _, path := range []string{mine, stranger, later} {
		writeTestFile(t, path, tierTestPage)
	}
	madeHere(t, a, mine, later)
	downloadedHere(t, a, stranger)

	mineSite, _ := a.openForTest(t, mine)
	strangerSite, _ := a.openForTest(t, stranger)
	laterSite, _ := a.openForTest(t, later)
	if strangerSite == mineSite || strangerSite.host == mineSite.host {
		t.Fatal("a download shares its folder's origin with the person's own document")
	}
	if strangerSite.key != real(t, stranger) {
		t.Fatalf("the download's site is keyed by %q, want its own path", strangerSite.key)
	}
	if laterSite != mineSite {
		t.Fatal("a later document of the person's own joined the download's origin")
	}
	if again, _ := a.openForTest(t, stranger); again != strangerSite {
		t.Fatal("opening the download again moved it to another origin")
	}
	if _, _, _, ok := strangerSite.sessions.AssetRootOpened(real(t, mine)); ok {
		t.Fatal("the download's page can read its neighbours")
	}
	// Once the person trusts the page, it keeps the origin it already had, so
	// its storage stays, and it gains its folder's reads as any opened file has.
	if err := a.tierStore().TrustPath(real(t, stranger), true, tier.HowAnswer); err != nil {
		t.Fatal(err)
	}
	if again, _ := a.openForTest(t, stranger); again != strangerSite {
		t.Fatal("trusting the download moved its page to another origin")
	}
	if _, _, opened, ok := strangerSite.sessions.AssetRootOpened(real(t, mine)); !ok || !opened {
		t.Fatal("a trusted page still cannot read its own folder")
	}
	if _, _, opened, ok := mineSite.sessions.AssetRootOpened(real(t, stranger)); !ok || !opened {
		t.Fatal("the person's own loose document lost its folder reads")
	}

	alone := filepath.Join(a.rt.home, "Desktop")
	app := filepath.Join(alone, "app.html")
	writeTestFile(t, app, tierTestPage)
	downloadedHere(t, a, app)
	appSite, _ := a.openForTest(t, app)
	a.mu.Lock()
	a.adoptLocked(appSite.anchor)
	adopted := appSite.trusted
	a.mu.Unlock()
	if adopted {
		t.Fatal("trusting the folder gave the download's own origin the whole folder")
	}
}

func TestStrangersInATrustedFolderHaveTheirOwnOrigin(t *testing.T) {
	a := newTestApp(t, evalTempHome(t))
	a.tierStore().SetDeveloper(false)
	folder := filepath.Join(a.rt.home, "project")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	a.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))
	mine := filepath.Join(folder, "index.html")
	plain := filepath.Join(folder, "notes.html")
	download := filepath.Join(folder, "invoice.html")
	broken := filepath.Join(folder, "signed.html")
	for _, path := range []string{mine, plain, download} {
		writeTestFile(t, path, tierTestPage)
	}
	writeTestFile(t, broken, strings.Replace(tierTestPage, "<head>", `<head><meta name="clay-signature" content="v1 key=k1 app=notes version=1.0.0 sig=AAAA">`, 1))
	madeHere(t, a, mine)
	downloadedHere(t, a, download)

	mineSite, _ := a.openForTest(t, mine)
	if plainSite, _ := a.openForTest(t, plain); plainSite != mineSite {
		t.Fatal("an unrecorded document in a trusted folder left the folder's origin")
	}
	for _, stranger := range []string{download, broken} {
		target, ok := a.serveTrusted(real(t, stranger))
		if !ok {
			t.Fatalf("%s: a link inside the folder could not route it", filepath.Base(stranger))
		}
		a.mu.Lock()
		own, _, found := a.lookupLocked(real(t, stranger))
		a.mu.Unlock()
		switch {
		case !found:
			t.Fatalf("%s is not registered", filepath.Base(stranger))
		case own == mineSite || own.host == mineSite.host:
			t.Fatalf("%s shares the trusted folder's origin", filepath.Base(stranger))
		case own.trusted:
			t.Fatalf("%s's own origin holds the trusted folder", filepath.Base(stranger))
		case !strings.Contains(target, own.host):
			t.Fatalf("%s: a link inside the folder leads to %s, not its own origin %s", filepath.Base(stranger), target, own.host)
		}
		if _, _, _, ok := own.sessions.AssetRootOpened(real(t, mine)); ok {
			t.Fatalf("%s's page can read the project", filepath.Base(stranger))
		}
	}
	framed := func(name string) string {
		rec := serveHostForTest(t, a, mineSite.host, func(r *http.Request) {
			r.URL.Path = "/project/" + name
			r.Header.Set("Sec-Fetch-Dest", "iframe")
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("the project's origin answered %d for %s", rec.Code, name)
		}
		return rec.Header().Get("Content-Security-Policy")
	}
	if got := framed("invoice.html"); got != "sandbox" {
		t.Fatalf("a download framed by the project's origin runs there: Content-Security-Policy %q", got)
	}
	feed := filepath.Join(folder, "feed.xml")
	writeTestFile(t, feed, `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><script>steal()</script></html>`)
	downloadedHere(t, a, feed)
	if got := framed("feed.xml"); got != "sandbox" {
		t.Fatalf("a downloaded XML page framed by the project's origin runs there: Content-Security-Policy %q", got)
	}
	if got := framed("notes.html"); got != "" {
		t.Fatalf("an unrecorded document framed by the project's origin is sandboxed: %q", got)
	}
	// A page stored compressed is still a page: the extension that matters is
	// the one under the .gz.
	compressed := filepath.Join(folder, "evil.html.gz")
	writeTestFile(t, compressed, tierTestPage)
	downloadedHere(t, a, compressed)
	if got := framed("evil.html.gz"); got != "sandbox" {
		t.Fatalf("a downloaded .html.gz framed by the project's origin runs there: Content-Security-Policy %q", got)
	}

	loose := filepath.Join(a.rt.home, "loose")
	if err := os.MkdirAll(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(loose, "own.htmlclay")
	visitor := filepath.Join(loose, "visitor.htmlclay")
	writeTestFile(t, own, tierTestPage)
	writeTestFile(t, visitor, tierTestPage)
	madeHere(t, a, own)
	downloadedHere(t, a, visitor)
	ownSite, _ := a.openForTest(t, own)
	rec := serveHostForTest(t, a, ownSite.host, func(r *http.Request) {
		r.URL.Path = "/loose/visitor.htmlclay"
		r.Header.Set("Sec-Fetch-Dest", "document")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		r.Header.Set("Sec-Fetch-User", "?1")
	})
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("a click from the person's page to a download beside it runs the download on their origin: %d, Content-Security-Policy %q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
}

// TestAStrangerPageThatReadsOutsideGetsTheWarning is Opus 5: a page nobody
// vouches for never widens what it can read. Its out-of-scope read is refused
// and the warning is raised about the page, not a folder read question.
func TestAStrangerPageThatReadsOutsideGetsTheWarning(t *testing.T) {
	home := evalTempHome(t)
	folder := filepath.Join(home, "Downloads")
	page := filepath.Join(folder, "page.html")
	sibling := filepath.Join(home, "Documents", "secret.txt")
	writeTestFile(t, page, tierTestPage)
	writeTestFile(t, sibling, "secret")

	a := newShellTestApp(t, home)
	t.Cleanup(a.stopRequests)
	downloadedHere(t, a, page)
	s, _ := a.openForTest(t, page)

	done := make(chan int, 1)
	go func() {
		req, err := http.NewRequest(http.MethodGet, s.url("Documents/secret.txt"), nil)
		if err != nil {
			done <- 0
			return
		}
		req.Header.Set("Referer", s.url(filepath.Join("Downloads", "page.html")))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- 0
			return
		}
		defer resp.Body.Close()
		done <- resp.StatusCode
	}()
	stand := tierStand{app: a}
	p := stand.awaitWarning(t, real(t, page))
	if !slices.Equal(p.Ask.Answers, []string{"deny-document", "allow-document", "trust-document"}) {
		t.Fatalf("the read raised %v, want the Unknown document warning", p.Ask.Answers)
	}
	if code := testutil.Receive(t, 10*time.Second, "the stranger's read", done); code != http.StatusForbidden {
		t.Fatalf("a stranger's out-of-scope read = %d, want 403", code)
	}
	for _, r := range a.snapshot().Requests {
		if r.Kind == appcore.RequestFolderRead || r.Kind == appcore.RequestFolderReadAgain {
			t.Fatalf("a stranger's page raised the folder read question %q", r.Kind)
		}
	}
}

// TestUpgradeRunsAfterACLICommandMadeTheStore is Opus 2: htmlclay trust from a
// terminal can create the trust store before the app first runs, and the launch
// that opens that store must still run the upgrade.
func TestUpgradeRunsAfterACLICommandMadeTheStore(t *testing.T) {
	home := evalTempHome(t)
	a := newTestApp(t, home)
	folder := real(t, t.TempDir())
	a.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))
	a.rt.configDir = filepath.Join(t.TempDir(), "htmlclay")
	if err := os.MkdirAll(a.rt.configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Join(a.rt.configDir, "trust")
	var out, errs strings.Builder
	if code := runTrust(storeDir, home, []string{"programs", "enable"}, &out, &errs); code != trustExitOK {
		t.Fatalf("htmlclay programs enable = %d %s", code, out.String()+errs.String())
	}
	opened, err := recents.Open(a.rt.configDir)
	if err != nil {
		t.Fatal(err)
	}
	store := a.openTiers(opened)
	if store == nil {
		t.Fatal("no trust store")
	}
	u, ok := store.Upgrade()
	if !ok {
		t.Fatal("a store the command made skipped the upgrade")
	}
	if !slices.Equal(u.Folders, []string{folder}) {
		t.Fatalf("upgrade folders = %v, want %v", u.Folders, []string{folder})
	}
}

// TestTheDocumentDialogStoresOnlyTheDenyButton is Opus 3: closing the question,
// Escape or a timeout leaves the document undecided, and only the Don't allow
// button is remembered.
func TestTheDocumentDialogStoresOnlyTheDenyButton(t *testing.T) {
	home := evalTempHome(t)
	a := newTestApp(t, home)
	t.Cleanup(a.stopRequests)
	doc := filepath.Join(home, "proj", "page.html")
	writeTestFile(t, doc, tierTestPage)
	downloadedHere(t, a, doc)
	path := real(t, doc)

	choice := platform.ConfirmDismissed
	seen := platform.ConfirmLabels{}
	a.rt.confirmDocument = func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error) {
		seen = labels
		return choice, nil
	}
	ask := func() {
		a.askDocument(path, "ai-edit")
		waitFor(t, "the document question to close", func() bool {
			a.helperStateMu.Lock()
			defer a.helperStateMu.Unlock()
			return !a.documentAsking[path]
		})
	}
	ask()
	if _, answered := a.tierStore().Answer(path); answered {
		t.Fatal("a closed or timed-out question stored an answer")
	}
	if v := a.tierOf(path); v.Denied {
		t.Fatal("a closed or timed-out question was stored as Don't allow")
	}
	if !slices.Contains([]string{helperAnswerAllowDocument, documentDialogTrustLabel}, seen.Allow) || seen.Deny != documentDialogDenyLabel {
		t.Fatalf("the question offered %+v, want Allow for this document, Trust it like a file I made and Don't allow", seen)
	}

	a.helperStateMu.Lock()
	a.documentQuiet = nil
	a.helperStateMu.Unlock()
	choice = platform.ConfirmDeny
	ask()
	if v := a.tierOf(path); !v.Denied {
		t.Fatal("the Don't allow button stored no answer")
	}
}
