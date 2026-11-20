package wire

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func getRunFile(core *Core, page, url, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Document-URL", "http://127.0.0.1:4321/"+page)
	req.Header.Set(TokenHeader, token)
	rec := httptest.NewRecorder()
	core.WireMux().ServeHTTP(rec, req)
	return rec
}

func TestRunFileRouteServesOnlyTheRunsDocument(t *testing.T) {
	core, docs, _, file := newFakeCore(t)
	other := filepath.Join(filepath.Dir(file), "other.htmlclay")
	writeHelperDocument(t, other)
	run := NewRunID()
	ref, err := core.RunFiles.Put(file, run, "diff", []byte("-one\n+two\n"))
	if err != nil {
		t.Fatal(err)
	}

	rec := getRunFile(core, "doc.htmlclay", ref.URL, "token")
	if rec.Code != http.StatusOK || rec.Body.String() != "-one\n+two\n" {
		t.Fatalf("owner: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers %v", rec.Header())
	}
	if rec := getRunFile(core, "other.htmlclay", ref.URL, "token"); rec.Code != http.StatusNotFound {
		t.Errorf("another document read the run file: %d", rec.Code)
	}
	if rec := getRunFile(core, "doc.htmlclay", RunFileURL(run, "missing"), "token"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown file id: %d", rec.Code)
	}
	docs.mu.Lock()
	docs.mayUse = false
	docs.mu.Unlock()
	if rec := getRunFile(core, "doc.htmlclay", ref.URL, "wrong"); rec.Code != http.StatusForbidden {
		t.Errorf("a caller without the token read the run file: %d", rec.Code)
	}
}

func TestRunFilesLimits(t *testing.T) {
	var r RunFiles
	run := NewRunID()
	if _, err := r.Put("/d", run, "a", make([]byte, MaxRunFileBytes+1)); !errors.Is(err, ErrRunFileTooLarge) {
		t.Errorf("an oversized file: %v", err)
	}
	if _, err := r.Put("/d", run, "a", make([]byte, MaxRunFileBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Put("/d", run, "b", make([]byte, MaxRunBytes-MaxRunFileBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Put("/d", run, "c", []byte("x")); !errors.Is(err, ErrRunTooLarge) {
		t.Errorf("a run past its limit: %v", err)
	}
	if _, err := r.Put("/d", run, "a", []byte("smaller")); err != nil {
		t.Errorf("replacing a file with a smaller one was refused: %v", err)
	}
	if _, err := r.Put("/other", run, "d", []byte("x")); err == nil {
		t.Error("another document wrote into the run")
	}
	for _, bad := range [][2]string{{"short", "a"}, {run, "../x"}, {run, ""}} {
		if _, err := r.Put("/d", bad[0], bad[1], []byte("x")); err == nil {
			t.Errorf("Put accepted run %q file %q", bad[0], bad[1])
		}
	}
}

func TestRunFilesExpireAndEvictOldestFirst(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	r := RunFiles{now: func() time.Time { return now }}
	old := NewRunID()
	if _, err := r.Put("/d", old, "a", []byte("old")); err != nil {
		t.Fatal(err)
	}
	r.Finish(old)
	now = now.Add(WireTerminalTTL)
	if _, ok := r.Get("/d", old, "a"); ok {
		t.Error("a run file outlived WireTerminalTTL")
	}

	var runs []string
	for i := 0; i*MaxRunBytes <= MaxRunFilesBytes; i++ {
		run := NewRunID()
		runs = append(runs, run)
		for _, id := range []string{"a", "b"} {
			if _, err := r.Put("/d", run, id, bytes.Repeat([]byte{'x'}, MaxRunFileBytes)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, ok := r.Get("/d", runs[0], "a"); ok {
		t.Error("the oldest run survived past the total limit")
	}
	body, ok := r.Get("/d", runs[len(runs)-1], "b")
	if !ok || len(body) != MaxRunFileBytes {
		t.Error("the newest run was evicted")
	}
	if r.total > MaxRunFilesBytes {
		t.Errorf("total %d is past the limit", r.total)
	}
}

// A run that wrote first and writes again last still pushes the oldest other
// run out, so several reviews at once never hold more than the total limit.
func TestRunFilesEvictWhenTheOldestRunWritesAgain(t *testing.T) {
	var r RunFiles
	first := NewRunID()
	if _, err := r.Put("/d", first, "a", bytes.Repeat([]byte{'x'}, MaxRunFileBytes)); err != nil {
		t.Fatal(err)
	}
	for r.total+MaxRunFileBytes <= MaxRunFilesBytes {
		if _, err := r.Put("/d", NewRunID(), "a", bytes.Repeat([]byte{'x'}, MaxRunFileBytes)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Put("/d", first, "b", bytes.Repeat([]byte{'x'}, MaxRunFileBytes)); err != nil {
		t.Fatal(err)
	}
	if r.total > MaxRunFilesBytes {
		t.Errorf("total %d is past the limit %d", r.total, MaxRunFilesBytes)
	}
	if _, ok := r.Get("/d", first, "b"); !ok {
		t.Error("the run being written was evicted")
	}
}

// Closing, moving or untrusting a document drops its run files at once.
func TestForgetDropsTheDocumentsRunFiles(t *testing.T) {
	core, _, _, file := newFakeCore(t)
	mine, other := NewRunID(), NewRunID()
	if _, err := core.RunFiles.Put(file, mine, "diff", []byte("mine")); err != nil {
		t.Fatal(err)
	}
	if _, err := core.RunFiles.Put("/elsewhere.htmlclay", other, "diff", []byte("other")); err != nil {
		t.Fatal(err)
	}
	core.Forget(file)
	if _, ok := core.RunFiles.Get(file, mine, "diff"); ok {
		t.Error("a forgotten document's run file is still readable")
	}
	if _, ok := core.RunFiles.Get("/elsewhere.htmlclay", other, "diff"); !ok {
		t.Error("Forget dropped another document's run")
	}
}

// A run that writes again while another run is over the total limit is not the
// one evicted: the run that has gone longest without a write goes first.
func TestRunFilesKeepsTheRunThatJustWrote(t *testing.T) {
	var r RunFiles
	a, b := NewRunID(), NewRunID()
	full := bytes.Repeat([]byte{'x'}, MaxRunFileBytes)
	for _, c := range []struct{ run, id string }{{a, "a"}, {b, "a"}, {a, "b"}} {
		if _, err := r.Put("/d", c.run, c.id, full); err != nil {
			t.Fatal(err)
		}
	}
	for r.total+MaxRunFileBytes <= MaxRunFilesBytes {
		if _, err := r.Put("/d", NewRunID(), "a", full); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Put("/d", NewRunID(), "a", full); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("/d", b, "a"); ok {
		t.Error("the run that had gone longest without a write survived")
	}
	for _, id := range []string{"a", "b"} {
		if _, ok := r.Get("/d", a, id); !ok {
			t.Errorf("the run that just wrote lost its file %s", id)
		}
	}
}

// A folder renamed or trashed inside a served root forgets every run below it,
// including a document the wire never bound.
func TestForgetUnderDropsTheFoldersRunFiles(t *testing.T) {
	core, _, _, _ := newFakeCore(t)
	runs := map[string]string{}
	for _, doc := range []string{"a/x.html", "a/b/y.html", "ab/z.html"} {
		run := NewRunID()
		runs[doc] = run
		if _, err := core.RunFiles.Put(doc, run, "diff", []byte(doc)); err != nil {
			t.Fatal(err)
		}
	}
	core.ForgetUnder("a")
	for _, doc := range []string{"a/x.html", "a/b/y.html"} {
		if body, ok := core.RunFiles.Get(doc, runs[doc], "diff"); ok {
			t.Errorf("%s kept a run file: %q", doc, body)
		}
	}
	if body, ok := core.RunFiles.Get("ab/z.html", runs["ab/z.html"], "diff"); !ok || string(body) != "ab/z.html" {
		t.Error("ForgetUnder dropped a document of another folder")
	}
	core.Shutdown()
	if _, ok := core.RunFiles.Get("ab/z.html", runs["ab/z.html"], "diff"); ok {
		t.Error("Shutdown kept a run file")
	}
}

// A run that is still open keeps its files however long ago it wrote them; the
// WireTerminalTTL clock starts at Finish, not at the last Put.
func TestRunFilesExpireAfterTheRunEnds(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	r := RunFiles{now: func() time.Time { return now }}
	run := NewRunID()
	if _, err := r.Put("/d", run, "diff", []byte("body")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Minute)
	if body, ok := r.Get("/d", run, "diff"); !ok || string(body) != "body" {
		t.Error("an open run expired")
	}
	r.Finish(run)
	now = now.Add(4 * time.Minute)
	if _, ok := r.Get("/d", run, "diff"); !ok {
		t.Error("a finished run expired before WireTerminalTTL")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := r.Get("/d", run, "diff"); ok {
		t.Error("a finished run outlived WireTerminalTTL")
	}
}
