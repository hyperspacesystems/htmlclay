package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// feedServer is a TLS feed and artifact host, the way download.htmlclay.com
// serves them, with the feed signed by a key the test controls.
type feedServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	feed     []byte
	artifact []byte
}

func (fs *feedServer) setFeed(feed []byte) {
	fs.mu.Lock()
	fs.feed = feed
	fs.mu.Unlock()
}

func newFeedServer(t *testing.T) *feedServer {
	t.Helper()
	fs := &feedServer{artifact: []byte("htmlclay 1.14.0 binary")}
	fs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/update/stable.json":
			fs.mu.Lock()
			feed := fs.feed
			fs.mu.Unlock()
			w.Write(feed)
		case "/update/1.14.0/htmlclay-1.14.0-test.bin":
			w.Write(fs.artifact)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *feedServer) release(version string) Release {
	sum := sha256.Sum256(fs.artifact)
	return Release{
		Schema: 1, App: "htmlclay", Channel: "stable", Version: version,
		Artifacts: map[string]Artifact{
			"linux-amd64": {URL: fs.srv.URL + "/update/1.14.0/htmlclay-1.14.0-test.bin", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(fs.artifact))},
		},
	}
}

func newTestUpdater(t *testing.T, fs *feedServer, keys map[string]ed25519.PublicKey) *Updater {
	t.Helper()
	root := t.TempDir()
	install := filepath.Join(root, "bin", "htmlclay")
	os.MkdirAll(filepath.Dir(install), 0o755)
	os.WriteFile(install, []byte("htmlclay 1.13.3 binary"), 0o755)
	return &Updater{
		FeedURL:     fs.srv.URL + "/update/stable.json",
		Keys:        keys,
		Client:      fs.srv.Client(),
		Current:     "1.13.3",
		GOOS:        "linux",
		GOARCH:      "amd64",
		Dir:         filepath.Join(root, "config", "updates"),
		InstallPath: install,
	}
}

func TestUpdaterStagesANewerSignedRelease(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	feed, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed(feed)
	u := newTestUpdater(t, fs, keys)

	r, err := u.CheckOnce()
	if err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	if r == nil || r.Version != "1.14.0" {
		t.Fatalf("Ready %+v, want 1.14.0", r)
	}
	if r.StagedPath != stagedPathFor(u.InstallPath) {
		t.Fatalf("staged at %s, want beside the install", r.StagedPath)
	}
	data, err := os.ReadFile(r.StagedPath)
	if err != nil || string(data) != string(fs.artifact) {
		t.Fatalf("staged content %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(u.Dir, "1.14.0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the download folder outlived staging: %v", err)
	}
	if got, _ := u.readSeen(); got != "1.14.0" {
		t.Fatalf("highest seen %q, want 1.14.0", got)
	}
}

func TestUpdaterIgnoresATamperedFeed(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	signed, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	var env Envelope
	json.Unmarshal(signed, &env)
	env.Payload = base64.StdEncoding.EncodeToString(payloadJSON(t, fs.release("1.15.0")))
	tampered, _ := json.Marshal(env)
	fs.setFeed(tampered)
	u := newTestUpdater(t, fs, keys)

	r, err := u.CheckOnce()
	if !errors.Is(err, ErrUntrusted) || r != nil {
		t.Fatalf("CheckOnce = %+v, %v; want ErrUntrusted", r, err)
	}
	if _, err := os.Stat(stagedPathFor(u.InstallPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a tampered feed staged a binary")
	}
	if got, _ := u.readSeen(); got != "" {
		t.Fatalf("a tampered feed raised the highest version seen to %q", got)
	}
}

func TestUpdaterSkipsAVersionThatFailedBefore(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	feed, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed(feed)
	u := newTestUpdater(t, fs, keys)
	os.MkdirAll(u.Dir, 0o700)
	writeJSONAtomic(filepath.Join(u.Dir, stateFile), &State{
		Schema: StateSchema, Phase: PhaseRolledBack, From: "1.13.3", To: "1.14.0",
		Failed: &Failure{Version: "1.14.0", Reason: "exited before it reported serving"},
	})

	r, err := u.CheckOnce()
	if err != nil || r != nil {
		t.Fatalf("CheckOnce = %+v, %v; want nothing to do", r, err)
	}
	if _, err := os.Stat(stagedPathFor(u.InstallPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a failed version was staged again")
	}
}

func TestUpdaterRefusesAReplayedOlderFeed(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	feed, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed(feed)
	u := newTestUpdater(t, fs, keys)
	os.MkdirAll(u.Dir, 0o700)
	writeJSONAtomic(filepath.Join(u.Dir, seenFileName), &seen{Schema: seenSchema, Highest: "1.15.0"})

	if r, err := u.CheckOnce(); err == nil || r != nil {
		t.Fatalf("CheckOnce = %+v, %v; want a refusal", r, err)
	}
}

func TestUpdaterDoesNothingWhenCurrent(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	feed, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed(feed)
	u := newTestUpdater(t, fs, keys)
	u.Current = "1.14.0"

	if r, err := u.CheckOnce(); err != nil || r != nil {
		t.Fatalf("CheckOnce = %+v, %v; want nothing to do", r, err)
	}
}

func TestUpdaterRunRetriesThenReportsReady(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	good, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed([]byte("not a feed yet"))
	u := newTestUpdater(t, fs, keys)
	u.FirstCheck = time.Millisecond
	u.Interval = 50 * time.Millisecond

	ready := make(chan Ready, 1)
	stop := make(chan struct{})
	defer close(stop)
	go u.Run(stop, func(r Ready) { ready <- r })
	time.Sleep(20 * time.Millisecond)
	fs.setFeed(good)

	select {
	case r := <-ready:
		if r.Version != "1.14.0" {
			t.Fatalf("Ready %+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run never reported the staged update")
	}
}

func TestRunChecksNowAndSaysWhatItFound(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	current, _ := SignPayload(payloadJSON(t, fs.release("1.13.3")), "test-a", priv)
	newer, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed(current)
	u := newTestUpdater(t, fs, keys)
	u.FirstCheck = time.Hour

	checkNow := make(chan struct{}, 1)
	u.CheckNow = checkNow
	var mu sync.Mutex
	var calls []bool
	var heard []string
	u.Checking = func(on bool) {
		mu.Lock()
		calls = append(calls, on)
		mu.Unlock()
	}
	u.Found = func(version string) {
		mu.Lock()
		heard = append(heard, "found "+version)
		mu.Unlock()
	}
	results := make(chan CheckResult, 3)
	u.Checked = func(r CheckResult) { results <- r }
	ready := make(chan Ready, 1)

	stop := make(chan struct{})
	defer close(stop)
	go u.Run(stop, func(r Ready) {
		mu.Lock()
		heard = append(heard, "ready "+r.Version)
		mu.Unlock()
		ready <- r
	})
	next := func() CheckResult {
		t.Helper()
		checkNow <- struct{}{}
		select {
		case r := <-results:
			return r
		case <-time.After(10 * time.Second):
			t.Fatal("a check started by Check now never said how it ended")
		}
		return CheckResult{}
	}

	if r := next(); r.Found != "" || r.Err != nil {
		t.Fatalf("a check with nothing newer = %+v, want the newest", r)
	}
	fs.setFeed([]byte("not a feed"))
	if r := next(); r.Err == nil {
		t.Fatalf("a check of a broken feed = %+v, want a failure", r)
	}
	fs.setFeed(newer)
	if r := next(); r.Found != "1.14.0" || r.Err != nil {
		t.Fatalf("a check of a newer feed = %+v, want 1.14.0 found", r)
	}
	<-ready
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"found 1.14.0", "ready 1.14.0"}; !slices.Equal(heard, want) {
		t.Fatalf("heard %v, want %v: the download is announced before it is staged", heard, want)
	}
	if want := []bool{true, false, true, false, true, false}; !slices.Equal(calls, want) {
		t.Fatalf("Checking calls %v, want %v", calls, want)
	}
}

func TestTimedChecksStopWhenUpdatesAreOff(t *testing.T) {
	priv, keys := testKeys(t)
	fs := newFeedServer(t)
	feed, _ := SignPayload(payloadJSON(t, fs.release("1.14.0")), "test-a", priv)
	fs.setFeed(feed)
	u := newTestUpdater(t, fs, keys)
	u.FirstCheck = time.Millisecond
	var automatic atomic.Bool
	u.Automatic = automatic.Load
	now := make(chan struct{}, 1)
	u.CheckNow = now

	ready := make(chan Ready, 1)
	stop := make(chan struct{})
	defer close(stop)
	go u.Run(stop, func(r Ready) { ready <- r })
	select {
	case r := <-ready:
		t.Fatalf("Run checked on its timer with updates off: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}
	now <- struct{}{}
	select {
	case r := <-ready:
		if r.Version != "1.14.0" {
			t.Fatalf("Ready %+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Check now did not check with updates off")
	}
}
