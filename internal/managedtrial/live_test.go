//go:build managedtrial

package managedtrial

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveKeepalive proves the idle-stream ping: with the keepalive shortened,
// a stream nobody publishes to is kept alive by its route's own comment line. The
// single-document route pings with `: ping`, and the list form with
// `: keepalive`.
func TestLiveKeepalive(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the handler reads that checkout's bundled runtime release")
	}
	previousSingle := singleStreamKeepalive
	previousShared := sharedStreamKeepalive
	singleStreamKeepalive = 50 * time.Millisecond
	sharedStreamKeepalive = 50 * time.Millisecond
	t.Cleanup(func() {
		singleStreamKeepalive = previousSingle
		sharedStreamKeepalive = previousShared
	})

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "idle.html"), []byte("<!doctype html><html><body><p>idle</p></body></html>\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	handler, err := newRootHandler(rootSpec{id: "root-live", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	address, port := serveTestServer(t, handler)

	single := openLiveStream(t, address, liveScriptStep{
		Name: "idle", Path: fmt.Sprintf("/_/sync?document-url=http://127.0.0.1:%d/idle.html", port),
	}, port, map[string]*liveStream{})
	defer single.close()
	shared := openLiveStream(t, address, liveScriptStep{
		Name:    "list",
		Path:    fmt.Sprintf("/_/sync?s=live:0:http://127.0.0.1:%d/idle.html", port),
		Headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
	}, port, map[string]*liveStream{})
	defer shared.close()

	// Frame 0 is the cursor and frame 1 is `: connected`; the ping follows on the
	// route's own interval.
	assertFrame(t, single, 2, ": ping")
	assertFrame(t, shared, 1, ": keepalive")
}

// assertFrame waits for the frame at index and fails unless it is want.
func assertFrame(t *testing.T, stream *liveStream, index int, want string) {
	t.Helper()
	got, ok := stream.waitFor(index)
	if !ok {
		t.Fatalf("%s never received frame %d (want %q)", stream.name, index, want)
	}
	if got != want {
		t.Fatalf("%s frame %d = %q, want %q", stream.name, index, got, want)
	}
}

// TestLiveBoundedWriter proves the response budget: a connection that stops
// reading is dropped at Local's limit, and the channel it was on keeps serving
// the others.
func TestLiveBoundedWriter(t *testing.T) {
	previous := live
	live = newLiveHub()
	live.configureFrameIDs(false)
	t.Cleanup(func() { live = previous })

	const file = "busy.html"
	frames := &frameSink{}
	refusing := &refusingConn{}
	dropped := 0
	writer := newBoundedWriter(refusing, func() { dropped++ })
	live.subscribe(file, writer, "live")
	live.subscribe(file, frames, "live")

	payload := livePayload{HTML: "<!doctype html><html><body><p>one</p></body></html>\n", Sender: "server-save"}
	writes := sseFalseWriteLimit + 1
	for i := 0; i < writes; i++ {
		live.broadcast(file, payload, "live")
	}

	if dropped != 1 {
		t.Fatalf("the writer was dropped %d times, want 1", dropped)
	}
	if !writer.droppedOut() {
		t.Fatal("the bounded writer kept writing after its limit")
	}
	// The limit is Local's: the connection is dropped on the write after the one
	// that refused its 32nd frame in a row.
	if refusing.writes != sseFalseWriteLimit+1 {
		t.Fatalf("the refused connection was written %d frames, want %d", refusing.writes, sseFalseWriteLimit+1)
	}
	if got := len(frames.frames()); got != writes {
		t.Fatalf("the reading connection got %d frames, want %d", got, writes)
	}

	live.broadcast(file, payload, "live")
	if refusing.writes != sseFalseWriteLimit+1 {
		t.Fatalf("the dropped connection was written again (%d frames)", refusing.writes)
	}
	if got := len(frames.frames()); got != writes+1 {
		t.Fatalf("the channel stopped serving the reading connection: %d frames, want %d", got, writes+1)
	}

	// The teardown the drop owes takes the connection off the channel, and the
	// others keep receiving.
	live.unsubscribe(file, writer)
	live.broadcast(file, payload, "live")
	if refusing.writes != sseFalseWriteLimit+1 {
		t.Fatalf("the connection was written after it left the channel (%d frames)", refusing.writes)
	}
	if got := len(frames.frames()); got != writes+2 {
		t.Fatalf("the channel stopped serving the reading connection: %d frames, want %d", got, writes+2)
	}
	if refusing.destroyed {
		t.Fatal("a bounded write destroyed the connection itself: only dropSlow does that, and the teardown owns it")
	}
}

// frameSink is a subscriber that reads everything it is handed.
type frameSink struct {
	mu  sync.Mutex
	got []string
}

func (s *frameSink) write(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, message)
	return nil
}

func (s *frameSink) frames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames := make([]string, len(s.got))
	copy(frames, s.got)
	return frames
}

// refusingConn is a socket that has stopped reading: every write is buffered
// rather than taken, which is what a bounded write counts.
type refusingConn struct {
	writes    int
	destroyed bool
}

func (c *refusingConn) write(string) error { c.writes++; return nil }
func (c *refusingConn) onDrain(func())     {}
func (c *refusingConn) accepted() bool     { return false }
func (c *refusingConn) backlogBytes() int  { return 0 }
func (c *refusingConn) destroy()           { c.destroyed = true }

// TestLiveReplayWindow proves the resume answer: the frames after the position a
// client named, a resync for a position older than the bucket holds, nothing for
// a position this channel has never reached, and a window that runs out. The
// window is shortened here, as the store keeps it in a variable for.
func TestLiveReplayWindow(t *testing.T) {
	previous := replayTTL
	replayTTL = 20 * time.Millisecond
	t.Cleanup(func() { replayTTL = previous })

	clock := int64(1000)
	store := newReplayStore()
	store.now = func() int64 { return clock }
	store.lastSeq = clock
	store.record(liveFrame{File: "a.html", Lane: "live", Seq: 1001, Message: "id: 1001\ndata: one\n\n"})
	store.record(liveFrame{File: "a.html", Lane: "live", Seq: 1002, Message: "id: 1002\ndata: two\n\n"})

	answer := store.resume("a.html", "live", 1001)
	if answer.Baseline != 1001 || answer.Resync || len(answer.Replay) != 1 || answer.Replay[0] != "id: 1002\ndata: two\n\n" {
		t.Fatalf("resume after 1001 = %+v", answer)
	}

	// A position older than the bucket's own start is the resync: the client is
	// handed everything the store holds and told to reload.
	answer = store.resume("a.html", "live", 1)
	if !answer.Resync || len(answer.Replay) != 2 {
		t.Fatalf("resume from another epoch = %+v", answer)
	}

	// A position this channel has never reached is no position at all: the store
	// answers its own last seq and no replay.
	answer = store.resume("a.html", "live", 5000)
	if answer.Baseline != 1002 || len(answer.Replay) != 0 || answer.Resync {
		t.Fatalf("resume from the future = %+v", answer)
	}

	// Past the window the frames are gone, and the client that asks is told to
	// resync because the bucket start is older than what it named.
	clock += 100
	answer = store.resume("a.html", "live", 1001)
	if len(answer.Replay) != 0 || !answer.Resync {
		t.Fatalf("resume past the window = %+v", answer)
	}
}

// TestLiveParseSharedEntry is the one `s` value shape: lane:since:document-url,
// split on its first two colons only so a port survives.
func TestLiveParseSharedEntry(t *testing.T) {
	entry, ok := parseSharedEntry("saved:1793512800000:http://127.0.0.1:8080/a.html")
	if !ok {
		t.Fatal("a well-formed entry was refused")
	}
	if entry.lane != "saved" || entry.since != 1793512800000 || entry.href != "http://127.0.0.1:8080/a.html" {
		t.Fatalf("entry = %+v", entry)
	}
	for _, bad := range []string{"", "saved", "saved:", "saved:1", "live:x:a", "other:1:a", "live:-1:a"} {
		if _, ok := parseSharedEntry(bad); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// TestLiveStalledReaderDropped proves a save is never held by a tab that stopped
// reading: the client below opens a stream and never reads it, and the
// broadcasts that fill its socket still all return, with the stream dropped.
func TestLiveStalledReaderDropped(t *testing.T) {
	previous := liveWriteLimit
	liveWriteLimit = 100 * time.Millisecond
	t.Cleanup(func() { liveWriteLimit = previous })

	hub := newLiveHub()
	subscribed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := startEventStream(w, "no-cache")
		hub.subscribe("doc.html", writer, "saved")
		close(subscribed)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.(*net.TCPConn).SetReadBuffer(4096); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("GET /stream HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscribed:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never subscribed")
	}

	page := strings.Repeat("x", 64<<10)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 400; i++ {
			hub.broadcast("doc.html", livePayload{HTML: page, Sender: "server-save"}, "saved")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("a broadcast is still blocked behind a client that stopped reading")
	}
	hub.mu.Lock()
	left := len(hub.clients["doc.html"])
	hub.mu.Unlock()
	if left != 0 {
		t.Fatalf("subscribers left = %d, want 0: the stalled stream should be dropped", left)
	}
}
