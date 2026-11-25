package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/testutil"
)

// fetchWithContext is a read request the test can stop waiting for, which is
// what a page does when it navigates away or closes.
func fetchWithContext(ctx context.Context, target string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

// waitForReadRequest waits for the read question of one kind to reach the
// snapshot, the way the request window sees it.
func waitForReadRequest(t *testing.T, a *app, kind string) appcore.Request {
	t.Helper()
	var found appcore.Request
	waitFor(t, "the "+kind+" question to reach the snapshot", func() bool {
		for _, r := range a.snapshot().Requests {
			if r.Kind == kind {
				found = r
				return true
			}
		}
		return false
	})
	return found
}

// TestM3ReadConfirmation proves a page's read question goes through one flow
// and two presenters: the registry the Mac app answers when it runs this host,
// and today's dialog otherwise. The fake dialog is the only dialog any of these
// subtests can reach.
func TestM3ReadConfirmation(t *testing.T) {
	t.Run("a home folder read offers no trust", func(t *testing.T) {
		home := evalTempHome(t)
		page := filepath.Join(home, "Documents", "evil", "index.html")
		asset := filepath.Join(home, "Documents", "lib.js")
		writeTestFile(t, page, "<html><body>evil</body></html>")
		writeTestFile(t, asset, "console.log('lib')")

		a := newShellTestApp(t, home)
		s, _ := a.openForTest(t, page)

		done := make(chan int, 1)
		go func() {
			code, _, _ := fetchWithContext(context.Background(), s.url(filepath.Join("Documents", "lib.js")))
			done <- code
		}()
		request := waitForReadRequest(t, a, appcore.RequestFolderRead)
		if request.Detail != "main-folder" {
			t.Errorf("a main personal folder must be named as one, got detail %q", request.Detail)
		}
		want := []string{"deny", "allow-once"}
		if !slices.Equal(request.Answers, want) {
			t.Fatalf("a page-chosen personal folder offered %v, want %v", request.Answers, want)
		}

		if _, refusal := a.do(context.Background(), testAnswer(t, "r1", request, "deny"), ChannelShell); refusal != nil {
			t.Fatalf("the shell's answer was refused: %+v", refusal)
		}
		if code := testutil.Receive(t, 10*time.Second, "the denied read", done); code != 403 {
			t.Errorf("a denied read = %d, want 403", code)
		}
	})

	t.Run("the shell's allow-once resumes the read", func(t *testing.T) {
		home := evalTempHome(t)
		page := filepath.Join(home, "work", "review", "fable", "index.html")
		asset := filepath.Join(home, "work", "review", "shared", "redpen.js")
		writeTestFile(t, page, "<html><body>fable</body></html>")
		writeTestFile(t, asset, "console.log('redpen')")

		a := newShellTestApp(t, home)
		s, _ := a.openForTest(t, page)

		type result struct {
			code int
			body string
		}
		done := make(chan result, 1)
		go func() {
			code, body, _ := fetchWithContext(context.Background(), s.url(filepath.Join("work", "review", "shared", "redpen.js")))
			done <- result{code, body}
		}()
		request := waitForReadRequest(t, a, appcore.RequestFolderRead)
		want := []string{"deny", "allow-once", "trust-folder"}
		if !slices.Equal(request.Answers, want) {
			t.Fatalf("an ordinary folder offered %v, want %v", request.Answers, want)
		}
		if request.Path != filepath.Join(home, "work", "review", "shared") {
			t.Errorf("the question names %q, want the resolved folder it grants", request.Path)
		}

		if _, refusal := a.do(context.Background(), testAnswer(t, "r2", request, "allow-once"), ChannelShell); refusal != nil {
			t.Fatalf("the shell's answer was refused: %+v", refusal)
		}
		got := testutil.Receive(t, 10*time.Second, "the allowed read", done)
		if got.code != 200 || !strings.Contains(got.body, "redpen") {
			t.Fatalf("an allowed read = %d, %q", got.code, got.body)
		}
		if pending := a.registry().Pending(); len(pending) != 0 {
			t.Errorf("%d question(s) still open after the answer", len(pending))
		}
	})

	t.Run("the page's own folder shows as folderReadAgain", func(t *testing.T) {
		home := evalTempHome(t)
		proj := filepath.Join(home, "proj")
		file := filepath.Join(proj, "index.htmlclay")
		writeTestFile(t, file, "<html><body>doc</body></html>")

		a := newShellTestApp(t, home)

		type answer struct {
			url string
			ok  bool
		}
		done := make(chan answer, 1)
		go func() {
			url, ok := a.trustFromPage(file, true)
			done <- answer{url, ok}
		}()
		request := waitForReadRequest(t, a, appcore.RequestFolderReadAgain)
		if want := []string{"deny", "trust-folder"}; !slices.Equal(request.Answers, want) {
			t.Fatalf("the own-folder question offered %v, want %v", request.Answers, want)
		}
		if request.Document != file || request.Path != proj {
			t.Errorf("the own-folder question names %q / %q", request.Document, request.Path)
		}

		if _, refusal := a.do(context.Background(), testAnswer(t, "r3", request, "trust-folder"), ChannelShell); refusal != nil {
			t.Fatalf("the shell's answer was refused: %+v", refusal)
		}
		got := testutil.Receive(t, 10*time.Second, "the trust question to settle", done)
		if !got.ok {
			t.Fatal("trusting the page's own folder was reported as refused")
		}
		if !hasFolder(a.rt.cfg.TrustedFolderList(), proj) {
			t.Errorf("the trusted list does not hold %s: %v", proj, a.rt.cfg.TrustedFolderList())
		}
	})

	t.Run("without the shell today's dialog answers it", func(t *testing.T) {
		home := evalTempHome(t)
		page := filepath.Join(home, "work", "review", "fable", "index.html")
		asset := filepath.Join(home, "work", "review", "shared", "redpen.js")
		writeTestFile(t, page, "<html><body>fable</body></html>")
		writeTestFile(t, asset, "console.log('redpen')")

		a := newTestApp(t, home)
		a.rt.confirm = func(_, message string, allowTrust bool) (platform.ConfirmChoice, error) {
			if !allowTrust {
				t.Error("an ordinary folder's dialog was not offered the trust choice")
			}
			if !strings.Contains(message, filepath.Join(home, "work", "review", "shared")) {
				t.Errorf("the dialog must name the folder it grants, got %q", message)
			}
			return platform.ConfirmAllowAlways, nil
		}
		s, _ := a.openForTest(t, page)

		code, body := fetch(t, s.url(filepath.Join("work", "review", "shared", "redpen.js")))
		if code != 200 || !strings.Contains(body, "redpen") {
			t.Fatalf("a trusted read = %d, %q", code, body)
		}
		waitForTrusted(t, a, filepath.Join(home, "work", "review", "shared"))
		if pending := a.registry().Pending(); len(pending) != 0 {
			t.Errorf("a dialog answer left %d question(s) in the registry", len(pending))
		}
	})

	t.Run("the question leaves the snapshot with its page", func(t *testing.T) {
		home := evalTempHome(t)
		page := filepath.Join(home, "work", "review", "fable", "index.html")
		asset := filepath.Join(home, "work", "review", "shared", "redpen.js")
		writeTestFile(t, page, "<html><body>fable</body></html>")
		writeTestFile(t, asset, "console.log('redpen')")

		a := newShellTestApp(t, home)
		s, _ := a.openForTest(t, page)

		ctx, cancel := context.WithCancel(context.Background())
		gone := make(chan error, 1)
		go func() {
			_, _, err := fetchWithContext(ctx, s.url(filepath.Join("work", "review", "shared", "redpen.js")))
			gone <- err
		}()
		waitForReadRequest(t, a, appcore.RequestFolderRead)

		cancel()
		waitFor(t, "the withdrawn question to leave the snapshot", func() bool {
			return len(a.snapshot().Requests) == 0
		})
		testutil.Receive(t, 10*time.Second, "the abandoned read", gone)
		if pending := a.registry().Pending(); len(pending) != 0 {
			t.Errorf("%d question(s) still open", len(pending))
		}
		if _, _, ok := s.sessions.AssetRoot(asset); ok {
			t.Error("a withdrawn question installed a read root")
		}
	})

	t.Run("a late allow changes nothing", func(t *testing.T) {
		home := evalTempHome(t)
		page := filepath.Join(home, "work", "review", "fable", "index.html")
		asset := filepath.Join(home, "work", "review", "shared", "a.js")
		later := filepath.Join(home, "work", "review", "shared", "sub", "b.js")
		writeTestFile(t, page, "<html><body>fable</body></html>")
		writeTestFile(t, asset, "console.log('a')")
		writeTestFile(t, later, "console.log('b')")

		a := newTestApp(t, home)
		// The access log line for the abandoned read is written after the handler
		// returns, so it says the server stopped waiting. Without it the test
		// releases the answer while the client is merely gone, which is a moment
		// before the waiter itself is.
		logPath := filepath.Join(t.TempDir(), "app.log")
		logger, err := logging.New(logPath)
		if err != nil {
			t.Fatalf("log: %v", err)
		}
		t.Cleanup(logger.Close)
		a.rt.logger = logger
		var mu sync.Mutex
		calls := 0
		asked := make(chan struct{})
		release := make(chan struct{})
		a.rt.confirm = func(_, _ string, _ bool) (platform.ConfirmChoice, error) {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n == 1 {
				close(asked)
				<-release
				return platform.ConfirmAllowAlways, nil
			}
			return platform.ConfirmAllowOnce, nil
		}
		s, _ := a.openForTest(t, page)

		ctx, cancel := context.WithCancel(context.Background())
		gone := make(chan error, 1)
		go func() {
			_, _, err := fetchWithContext(ctx, s.url(filepath.Join("work", "review", "shared", "a.js")))
			gone <- err
		}()
		testutil.Receive(t, 10*time.Second, "the read question", asked)

		// The page stops waiting: every waiter is gone before the person answers.
		cancel()
		relAsset := filepath.Join("work", "review", "shared", "a.js")
		waitFor(t, "the server to stop waiting for the read", func() bool {
			logged, _ := os.ReadFile(logPath)
			return strings.Contains(string(logged), "GET /"+filepath.ToSlash(relAsset)+" 403")
		})
		<-gone
		close(release)

		// A question for another asset under the same folder proves the prompt's
		// work finished, because only one prompt runs at a time.
		second := make(chan int, 1)
		go func() {
			code, _, _ := fetchWithContext(context.Background(), s.url(filepath.Join("work", "review", "shared", "sub", "b.js")))
			second <- code
		}()
		// A late allow that installed anything would cover this asset too, so no
		// question would be raised for it. Waiting for that question is therefore
		// the same claim as asserting nothing was installed, and it is settled by
		// the broker itself rather than by a sleep.
		waitFor(t, "the second read question, which a late allow would have pre-answered", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return calls == 2
		})

		if trusted := a.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
			t.Errorf("a late allow recorded trust: %v", trusted)
		}
		if _, _, ok := s.sessions.AssetRoot(asset); ok {
			t.Error("a late allow installed a read root")
		}
		if code := testutil.Receive(t, 10*time.Second, "the later read", second); code != 200 {
			t.Errorf("the later read = %d, want 200", code)
		}
	})
}
