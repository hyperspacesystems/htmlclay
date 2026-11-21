package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/codeset"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
)

func TestADontAllowShowsOnProgramsAndAskAgainClearsIt(t *testing.T) {
	home := real(t, t.TempDir())
	a := newShellTestApp(t, home)
	t.Cleanup(a.stopRequests)

	// 1. A document HTML Clay made is Mine: DocumentTrust is ready and needs no dialog.
	mineDoc := filepath.Join(home, "docs", "mine.html")
	writeTestFile(t, mineDoc, `<!doctype html><html><head><script>console.log("mine")</script></head><body><p>Mine</p></body></html>`)
	if err := a.recordMadeDocument(mineDoc, tier.HowCreated); err != nil {
		t.Fatal(err)
	}
	if trust := a.documentTrust(mineDoc); trust.Tier != tier.Mine.String() || !trust.Ready {
		t.Fatalf("mine documentTrust = %+v, want mine and ready", trust)
	}
	a.askDocumentForWire(mineDoc, aitools.HelperName)
	if len(a.snapshot().Requests) != 0 {
		t.Fatalf("Mine document raised a dialog: %+v", a.snapshot().Requests)
	}

	// 2. A downloaded Unknown document is not ready and raises RequestUnknownDocument.
	strangerDoc := filepath.Join(home, "docs", "stranger.html")
	writeTestFile(t, strangerDoc, `<!doctype html><html><head><script>console.log("stranger")</script></head><body><p>Stranger</p></body></html>`)
	downloadedHere(t, a, strangerDoc)
	if trust := a.documentTrust(strangerDoc); trust.Tier != tier.Unknown.String() || trust.Ready {
		t.Fatalf("stranger documentTrust = %+v, want unknown and not ready", trust)
	}

	a.askDocumentForWire(strangerDoc, aitools.HelperName)
	req := waitForSetupRequest(t, a, appcore.RequestUnknownDocument)
	if err := answerSetup(t, a, "deny-stranger", req, documentAnswerDeny, ""); err != nil {
		t.Fatal(err)
	}

	// 3. Don't allow marks the document denied, surfaces in Snapshot.DeniedDocuments and /programs,
	// and Ask again clears the denial while keeping first sight.
	// The answer is recorded on the asking goroutine after the reply.
	waitFor(t, "Don't allow recorded", func() bool { return a.documentTrust(strangerDoc).Denied })
	snap := a.snapshot()
	if !slices.Contains(snap.DeniedDocuments, strangerDoc) {
		t.Fatalf("DeniedDocuments = %v, missing %s", snap.DeniedDocuments, strangerDoc)
	}

	c := signIn(t, a)
	rec := c.do("GET", "/programs", nil, nil)
	rendered := rec.Body.String()
	if !strings.Contains(rendered, "stranger.html") || !strings.Contains(rendered, "ask-again-document") {
		t.Fatal("/programs page missing Ask Again row for denied document")
	}

	// Clear Don't allow through the Mac shell action program.askAgain.
	argsRaw, _ := json.Marshal(map[string]string{
		"programID":  appcore.DocumentTrustProgram,
		"documentID": strangerDoc,
	})
	if _, refusal := a.do(context.Background(), appcore.Action{
		ID:   "clear-deny-1",
		Type: "program.askAgain",
		Args: argsRaw,
	}, ChannelShell); refusal != nil {
		t.Fatalf("program.askAgain(document-trust) refused: %+v", refusal)
	}

	if trust := a.documentTrust(strangerDoc); trust.Denied {
		t.Fatalf("documentTrust after Ask again = %+v, want not denied", trust)
	}
	if len(a.snapshot().DeniedDocuments) != 0 {
		t.Fatalf("DeniedDocuments after Ask again = %v, want empty", a.snapshot().DeniedDocuments)
	}

	// Now the document can ask again and be allowed for its current code (read-only, not Mine).
	a.askDocumentForWire(strangerDoc, aitools.HelperName)
	req2 := waitForSetupRequest(t, a, appcore.RequestUnknownDocument)
	if err := answerSetup(t, a, "allow-stranger", req2, helperAnswerAllowDocument, ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Allow recorded", func() bool {
		trust := a.documentTrust(strangerDoc)
		return trust.Tier == tier.Unknown.String() && trust.Ready
	})
}

func TestProgramsPageAskAgainActionRequiresNativeDialogAndClearsDeny(t *testing.T) {
	settledSetup(t)
	a := newProgramsApp(t)
	doc := filepath.Join(a.rt.home, "docs", "blocked.html")
	if err := os.MkdirAll(filepath.Dir(doc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte(`<!doctype html><html><head><script>1</script></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	downloadedHere(t, a, doc)
	if err := a.denyDocument(doc); err != nil {
		t.Fatal(err)
	}

	cancelDialogs := helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			return platform.ChoiceRefuse, nil
		},
	}
	if outcome, err := a.askDocumentAgainFromPrograms(doc, cancelDialogs); err != nil || outcome != "unchanged" {
		t.Fatalf("refused dialog = (%q, %v), want (unchanged, nil)", outcome, err)
	}
	if !a.tierOf(doc).Denied {
		t.Fatal("refused dialog cleared Don't allow")
	}

	affirmDialogs := helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			return platform.ChoiceAffirm, nil
		},
	}
	if outcome, err := a.askDocumentAgainFromPrograms(doc, affirmDialogs); err != nil || outcome != "ask-again" {
		t.Fatalf("affirmed dialog = (%q, %v), want (ask-again, nil)", outcome, err)
	}
	if a.tierOf(doc).Denied {
		t.Fatal("affirmed Ask Again left Don't allow set")
	}
}

func TestAIEditKeepMatchesByCodeHashAndNothingElse(t *testing.T) {
	home := real(t, t.TempDir())
	a := newShellTestApp(t, home)
	t.Cleanup(a.stopRequests)

	// A downloaded document trusted ONLY by its code hash (mimicking a signed
	// app or a downloaded copy of trusted code, not path-trusted).
	origHTML := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js"></script></head><body><div id="card" class="box">Old</div></body></html>`
	origSet, err := codeset.Of([]byte(origHTML))
	if err != nil || origSet.Parts == 0 {
		t.Fatalf("codeset.Of = %+v, %v", origSet, err)
	}
	doc := filepath.Join(home, "docs", "app.html")
	writeTestFile(t, doc, origHTML)
	downloadedHere(t, a, doc)
	if err := a.tierStore().TrustCode(origSet.Hash, tier.HowCommand); err != nil {
		t.Fatal(err)
	}
	if v := a.tierOf(doc); !v.Trusted() {
		t.Fatalf("initial verdict = %+v, want Trusted()", v)
	}

	// Record an AI edit result that adds an active attribute (changing the code-set hash).
	elemBefore := `<div class="box" id="card">Old</div>`
	elemAfter := `<div id="card" class="box" hx-get="/items">New</div>`
	a.noteAIEditResult(doc, elemBefore, "div", elemAfter)

	// 1. Saving a DIFFERENT code change (e.g. an injected script) must NOT be trusted.
	tampered := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js"></script></head><body><div id="card" class="box" hx-get="/items">New</div><script>evil()</script></body></html>`
	writeTestFile(t, doc, tampered)
	a.noteDocumentSaved(doc, []byte(tampered))
	if v := a.tierOf(doc); v.Trusted() {
		t.Fatalf("tampered save was trusted: %+v", v)
	}

	// 2. Saving the exact kept AI edit code (even with data edits elsewhere) trusts the new code!
	kept := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js"></script></head><body><div id="card" class="box" hx-get="/items">New and edited prose</div></body></html>`
	writeTestFile(t, doc, kept)
	a.noteDocumentSaved(doc, []byte(kept))
	if v := a.tierOf(doc); !v.Trusted() {
		t.Fatalf("kept AI edit was not trusted: %+v", v)
	}

	// 3. A subsequent unapproved code edit on the same path still loses trust immediately.
	laterTampered := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js"></script></head><body><div id="card" class="box" hx-get="/items">New</div><script>later()</script></body></html>`
	writeTestFile(t, doc, laterTampered)
	if v := a.tierOf(doc); v.Trusted() {
		t.Fatalf("later unapproved code edit stayed trusted: %+v", v)
	}

	// 4. A kept AI edit on a document nobody vouches for never trusts its code.
	strangerHTML := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js?plugins=menu"></script></head><body><div id="card" class="box">Old</div></body></html>`
	stranger := filepath.Join(home, "docs", "stranger.html")
	writeTestFile(t, stranger, strangerHTML)
	downloadedHere(t, a, stranger)
	a.noteAIEditResult(stranger, elemBefore, "div", elemAfter)
	keptStranger := strings.Replace(strangerHTML, `<div id="card" class="box">Old</div>`, elemAfter, 1)
	writeTestFile(t, stranger, keptStranger)
	a.noteDocumentSaved(stranger, []byte(keptStranger))
	if v := a.tierOf(stranger); v.Trusted() {
		t.Fatalf("a kept AI edit made a stranger's document trusted: %+v", v)
	}
}

func TestAIEditKeepTrustsNothingWhenNoCodeChanged(t *testing.T) {
	home := real(t, t.TempDir())
	a := newShellTestApp(t, home)
	t.Cleanup(a.stopRequests)

	storeDir := t.TempDir()
	store, _, err := tier.Open(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetDeveloper(true); err != nil {
		t.Fatal(err)
	}
	a.core.mu.Lock()
	a.core.tiers = store
	a.core.mu.Unlock()
	codeFiles := func() int {
		entries, err := os.ReadDir(filepath.Join(storeDir, "code"))
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	pending := func(doc string) bool {
		a.core.mu.Lock()
		defer a.core.mu.Unlock()
		_, ok := a.core.pendingKeep[closedPath(doc)]
		return ok
	}

	origHTML := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js"></script></head><body><div id="card" class="box">Old</div></body></html>`
	origSet, err := codeset.Of([]byte(origHTML))
	if err != nil || origSet.Parts == 0 {
		t.Fatalf("codeset.Of = %+v, %v", origSet, err)
	}
	doc := filepath.Join(home, "docs", "app.html")
	writeTestFile(t, doc, origHTML)
	downloadedHere(t, a, doc)
	if err := store.TrustCode(origSet.Hash, tier.HowCreated); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.PathTrust(doc); ok {
		t.Fatal("the document has a path record; the test needs code-only trust")
	}
	if v := a.tierOf(doc); !v.Trusted() {
		t.Fatalf("initial verdict = %+v, want Trusted()", v)
	}
	before := codeFiles()

	elemBefore := `<div id="card" class="box">Old</div>`
	elemAfter := `<div id="card" class="box">New words</div>`
	a.noteAIEditResult(doc, elemBefore, "div", elemAfter)
	if pending(doc) {
		t.Fatal("a text-only AI edit left a pending trust for code that did not change")
	}

	edited := strings.Replace(origHTML, elemBefore, elemAfter, 1)
	writeTestFile(t, doc, edited)
	a.noteDocumentSaved(doc, []byte(edited))
	if got := codeFiles(); got != before {
		t.Fatalf("the code trust folder grew from %d to %d entries for an edit that changed no code", before, got)
	}
	if _, ok := store.PathTrust(doc); ok {
		t.Fatal("a text-only AI edit save wrote a path trust record")
	}

	pathDoc := filepath.Join(home, "docs", "pin.html")
	pathHTML := `<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js?plugins=pin"></script></head><body><div id="card" class="box">Old</div></body></html>`
	writeTestFile(t, pathDoc, pathHTML)
	if err := store.TrustPath(pathDoc, false, tier.HowCreated); err != nil {
		t.Fatal(err)
	}
	if v := a.tierOf(pathDoc); !v.Trusted() {
		t.Fatalf("path-trusted verdict = %+v, want Trusted()", v)
	}
	a.noteAIEditResult(pathDoc, elemBefore, "div", `<div id="card" class="box" hx-get="/x">New</div>`)
	if pending(pathDoc) {
		t.Fatal("a document trusted by its path left a pending AI edit trust")
	}
}
