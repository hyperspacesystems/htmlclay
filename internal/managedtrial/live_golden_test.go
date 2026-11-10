//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/dataguard"
)

// TestLiveGolden replays a script recorded against Local's real server: every
// live-sync stream opened, the frames each one received in the order it received
// them, every request's answer, and the tree the whole sequence left behind.

// liveScriptStep is one step of a case's script.json: a stream opened or closed,
// an HTTP request, or a wait.
type liveScriptStep struct {
	Op              string            `json:"op"`
	Name            string            `json:"name"`
	Path            string            `json:"path"`
	Method          string            `json:"method"`
	Headers         map[string]string `json:"headers"`
	Body            *string           `json:"body"`
	BodyFile        string            `json:"bodyFile"`
	BodyRepeat      *liveBodyRepeat   `json:"bodyRepeat"`
	FromCase        *serveIfMatchFrom `json:"fromCase"`
	IfMatchFrom     *serveIfMatchFrom `json:"ifMatchFrom"`
	IfNoneMatchFrom string            `json:"ifNoneMatchFrom"`
	MS              int               `json:"ms"`
}

// liveBodyRepeat is one request body the recorder builds by repeating a unit: a
// body over the route's limit is too large to write out and too large to record.
type liveBodyRepeat struct {
	Unit  string `json:"unit"`
	Times int    `json:"times"`
}

type liveStreamAnswer struct {
	Name    string            `json:"name"`
	Request liveStreamRequest `json:"request"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Frames  []string          `json:"frames"`
}

type liveStreamRequest struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type liveGolden struct {
	Streams []liveStreamAnswer `json:"streams"`
	Cases   []serveAnswer      `json:"cases"`
	Files   []serveFileEntry   `json:"files"`
	Clock   int64              `json:"clock"`
	Pages   []string           `json:"pages"`
}

// liveGoldenSettle is the recorder's 300 ms after the last step: every frame a
// step caused has arrived by then.
const liveGoldenSettle = 300 * time.Millisecond

// liveJudgedCase is the one decided byte difference in this case directory: a
// guard Restore writes Local's data in dataapi's bytes, not cheerio's, so the
// document that GET answers, the version the write-back makes and the frame that
// carries it are judged by the data they hold.
const liveJudgedCase = "notes-restored"

// liveFrameReference is a recorded frame's id, referenced from a later step's
// header the way a reconnecting EventSource sends Last-Event-ID.
var liveFrameReference = regexp.MustCompile(`\{FRAME:([^:}]+):(\d+)\}`)

func TestLiveGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout")
	}
	runLiveGolden(t, filepath.Join("testdata", "live-golden"), localSource, liveJudgedCase)
}

// runLiveGolden replays one recorded case directory. judgedCase is the one case
// whose answer, files and frames are judged by the data they hold rather than
// byte for byte; a directory with no such case passes "".
func runLiveGolden(t *testing.T, caseDir, localSource, judgedCase string) {
	t.Helper()
	var golden liveGolden
	raw, err := os.ReadFile(filepath.Join(caseDir, "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var script []liveScriptStep
	raw, err = os.ReadFile(filepath.Join(caseDir, "script.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &script); err != nil {
		t.Fatal(err)
	}
	if golden.Clock == 0 {
		t.Fatal("the live golden records no pinned clock")
	}
	freezeLiveClock(t, golden.Clock)

	root := serveGoldenRoot(t, caseDir)
	handler, err := newRootHandler(rootSpec{id: "root-a", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderLimit}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	address := fmt.Sprintf("127.0.0.1:%d", port)

	streams := make(map[string]*liveStream, len(golden.Streams))
	var streamOrder []string
	answered := make(map[string]serveAnswer, len(golden.Cases))
	var answerOrder []string
	for _, step := range script {
		switch step.Op {
		case "wait":
			time.Sleep(time.Duration(step.MS) * time.Millisecond)
		case "open":
			streams[step.Name] = openLiveStream(t, address, step, port, streams)
			streamOrder = append(streamOrder, step.Name)
		case "close":
			stream, ok := streams[step.Name]
			if !ok {
				t.Fatalf("no open stream named %q", step.Name)
			}
			stream.close()
		case "request":
			headers := resolveGoldenHeaders(t, serveRequest{
				Name:            step.Name,
				Headers:         step.Headers,
				IfNoneMatchFrom: step.IfNoneMatchFrom,
				IfMatchFrom:     step.IfMatchFrom,
			}, answered, port)
			requestPath := resolveLiveValue(t, step.Path, port, streams)
			if step.FromCase != nil {
				requestPath = strings.ReplaceAll(requestPath, "{FROM}", resolveJSONField(t, step.Name, *step.FromCase, answered))
			}
			body := serveRequestBody(t, caseDir, serveRequest{Body: step.Body, BodyFile: step.BodyFile})
			if step.BodyRepeat != nil {
				body = []byte(strings.Repeat(step.BodyRepeat.Unit, step.BodyRepeat.Times))
			}
			status, gotHeaders, answer := serveSend(t, address, step.Method, requestPath, headers, body, port)
			answered[step.Name] = serveAnswer{Name: step.Name, Status: status, Headers: gotHeaders, Answer: answer}
			answerOrder = append(answerOrder, step.Name)
		default:
			t.Fatalf("unknown script op %q", step.Op)
		}
	}

	time.Sleep(liveGoldenSettle)
	for _, name := range streamOrder {
		streams[name].close()
	}

	if len(streamOrder) != len(golden.Streams) {
		t.Fatalf("the script opened %d streams, the golden holds %d", len(streamOrder), len(golden.Streams))
	}
	if len(answerOrder) != len(golden.Cases) {
		t.Fatalf("the script sent %d requests, the golden holds %d", len(answerOrder), len(golden.Cases))
	}
	var restored []byte
	if judgedCase != "" {
		restored = liveGoldenCaseBody(t, golden, judgedCase)
	}
	for i, want := range golden.Streams {
		if streamOrder[i] != want.Name {
			t.Fatalf("stream %d is %q in the script and %q in the golden", i, streamOrder[i], want.Name)
		}
		got := streams[want.Name]
		t.Run("stream-"+want.Name, func(t *testing.T) {
			if got.status != want.Status {
				t.Errorf("status = %d, want %d", got.status, want.Status)
			}
			if !reflect.DeepEqual(got.headers, want.Headers) {
				t.Errorf("headers = %v, want %v", got.headers, want.Headers)
			}
			compareLiveFrames(t, want, got.frames(), restored)
		})
	}
	for i, want := range golden.Cases {
		if answerOrder[i] != want.Name {
			t.Fatalf("answer %d is %q in the script and %q in the golden", i, answerOrder[i], want.Name)
		}
		got := answered[want.Name]
		t.Run("case-"+want.Name, func(t *testing.T) {
			compareLiveAnswer(t, got, want, judgedCase)
		})
	}
	compareLiveFiles(t, root, serveFileList(t, root), golden.Files, restored)
}

// freezeLiveClock pins everything the recording pinned: the frame sequence
// numbers, the guard's stamps and event ids, and the version names. The hub and
// the replay store start empty here, the way a freshly required module does, with
// the recording's clock as the store's own last seq.
func freezeLiveClock(t *testing.T, clock int64) {
	t.Helper()
	previousNow := liveNow
	previousClock := versionClock
	previousGuardNow := guardNow
	previousGuardID := guardNewID
	liveNow = func() int64 { return clock }
	versionClock = func() int64 { return clock }
	guardNow = func() int64 { return clock }
	ids := 0
	guardNewID = func() string {
		value := fmt.Sprintf("00000000-0000-4000-8000-%012d", ids)
		ids++
		return value
	}
	live = newLiveHub()
	live.configureFrameIDs(true)
	replay = newReplayStore()
	live.onFrame(replay.record)
	t.Cleanup(func() {
		liveNow = previousNow
		versionClock = previousClock
		guardNow = previousGuardNow
		guardNewID = previousGuardID
	})
}

// resolveLiveValue fills a step's {PORT} with this host's port and its
// {FRAME:<stream>:<index>} with the id line of a frame that stream received.
func resolveLiveValue(t *testing.T, value string, port int, streams map[string]*liveStream) string {
	t.Helper()
	text := strings.ReplaceAll(value, "{PORT}", strconv.Itoa(port))
	return liveFrameReference.ReplaceAllStringFunc(text, func(whole string) string {
		match := liveFrameReference.FindStringSubmatch(whole)
		stream, ok := streams[match[1]]
		if !ok {
			t.Fatalf("%s: no open stream named %q", whole, match[1])
		}
		index, err := strconv.Atoi(match[2])
		if err != nil {
			t.Fatalf("%s: %v", whole, err)
		}
		frame, ok := stream.waitFor(index)
		if !ok {
			t.Fatalf("%s: stream %q never received frame %d", whole, match[1], index)
		}
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "id: ") {
				return strings.TrimPrefix(line, "id: ")
			}
		}
		t.Fatalf("%s: stream %q frame %d carries no id", whole, match[1], index)
		return ""
	})
}

// liveStream is one open stream as it is read frame by frame.
type liveStream struct {
	name      string
	port      int
	status    int
	headers   map[string]string
	body      io.ReadCloser
	transport *http.Transport
	mu        sync.Mutex
	buffer    string
	received  []string
	closed    bool
}

func openLiveStream(t *testing.T, address string, step liveScriptStep, port int, streams map[string]*liveStream) *liveStream {
	t.Helper()
	transport := &http.Transport{}
	request, err := http.NewRequest(http.MethodGet, "http://"+address+resolveLiveValue(t, step.Path, port, streams), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range step.Headers {
		request.Header.Set(name, resolveLiveValue(t, value, port, streams))
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("open %s: %v", step.Name, err)
	}
	stream := &liveStream{
		name:      step.Name,
		port:      port,
		status:    response.StatusCode,
		headers:   normalizeServeHeaders(response.Header, port),
		body:      response.Body,
		transport: transport,
	}
	go stream.read()
	return stream
}

func (s *liveStream) read() {
	buffer := make([]byte, 4096)
	for {
		n, err := s.body.Read(buffer)
		if n > 0 {
			s.feed(string(buffer[:n]))
		}
		if err != nil {
			return
		}
	}
}

// feed splits what arrived on blank lines, which is how SSE frames are told
// apart, and swaps the port back out so a frame is comparable across runs.
func (s *liveStream) feed(chunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer += chunk
	for {
		at := strings.Index(s.buffer, "\n\n")
		if at < 0 {
			return
		}
		s.received = append(s.received, strings.ReplaceAll(s.buffer[:at], fmt.Sprintf(":%d", s.port), ":{PORT}"))
		s.buffer = s.buffer[at+2:]
	}
}

// waitFor waits until this stream has received the frame at index, and answers
// it. The reader is a goroutine, so a frame a step caused may not have been read
// by the time a later step names it.
func (s *liveStream) waitFor(index int) (string, bool) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		if index < len(s.received) {
			frame := s.received[index]
			s.mu.Unlock()
			return frame, true
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			return "", false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *liveStream) frames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames := make([]string, len(s.received))
	copy(frames, s.received)
	return frames
}

func (s *liveStream) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.body.Close()
	s.transport.CloseIdleConnections()
}

// compareLiveFrames compares every frame exactly, except one that carries the
// document a guard Restore wrote: its html is judged by the data it holds, and
// every other byte of that frame stays exact.
func compareLiveFrames(t *testing.T, want liveStreamAnswer, got []string, restored []byte) {
	t.Helper()
	if len(got) != len(want.Frames) {
		t.Errorf("frames = %d, want %d\ngot:  %v\nwant: %v", len(got), len(want.Frames), got, want.Frames)
		return
	}
	for index, wantFrame := range want.Frames {
		gotFrame := got[index]
		if gotFrame == wantFrame {
			continue
		}
		wantHTML, wantOK := liveFrameHTML(wantFrame)
		gotHTML, gotOK := liveFrameHTML(gotFrame)
		if !wantOK || !gotOK || !sameIslandData([]byte(wantHTML), restored) || !sameIslandData([]byte(gotHTML), []byte(wantHTML)) {
			t.Errorf("frame %d = %q, want %q", index, gotFrame, wantFrame)
			continue
		}
		// Put the html back and the rest of the frame has to be byte-exact: the
		// id line, the sender and every other field Local wrote.
		gotJSON, err := jsonStringify(gotHTML)
		if err != nil {
			t.Fatal(err)
		}
		wantJSON, err := jsonStringify(wantHTML)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Replace(gotFrame, string(gotJSON), string(wantJSON), 1) != wantFrame {
			t.Errorf("frame %d = %q, want %q", index, gotFrame, wantFrame)
			continue
		}
		t.Logf("known difference: %s frame %d holds Local's data in dataapi's bytes", want.Name, index)
	}
}

// compareLiveAnswer compares one request's answer exactly, except the document a
// guard Restore wrote: its body is judged by the data it holds and its two body
// stamps are recomputed from this host's own bytes.
func compareLiveAnswer(t *testing.T, got, want serveAnswer, judgedCase string) {
	t.Helper()
	if got.Status != want.Status {
		t.Errorf("status = %d, want %d", got.Status, want.Status)
	}
	wantBody := serveDecodeBody(t, want.Body)
	if want.Name == judgedCase {
		if expected := ownStampHeaders(want.Headers, got.Answer); !reflect.DeepEqual(got.Headers, expected) {
			t.Errorf("headers = %v, want %v", got.Headers, expected)
		}
		assertSameData(t, want.Name, got.Answer, wantBody)
		return
	}
	if !reflect.DeepEqual(got.Headers, want.Headers) {
		t.Errorf("headers = %v, want %v", got.Headers, want.Headers)
	}
	if !bytes.Equal(got.Answer, wantBody) {
		t.Errorf("body = %q, want %q", got.Answer, wantBody)
	}
}

// compareLiveFiles compares the tree exactly, except a file that holds the
// document a guard Restore wrote: that one is judged by the data it holds.
func compareLiveFiles(t *testing.T, root string, got, want []serveFileEntry, restored []byte) {
	t.Helper()
	gotByPath := make(map[string]string, len(got))
	for _, entry := range got {
		gotByPath[entry.Path] = entry.SHA256
	}
	wantPaths := make(map[string]bool, len(want))
	for _, entry := range want {
		wantPaths[entry.Path] = true
		sha, ok := gotByPath[entry.Path]
		if !ok {
			t.Errorf("%s: missing from this host's tree", entry.Path)
			continue
		}
		if sha == entry.SHA256 {
			continue
		}
		onDisk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err != nil {
			t.Errorf("%s: %v", entry.Path, err)
			continue
		}
		if !sameIslandData(onDisk, restored) {
			t.Errorf("%s: this host wrote other bytes than Local did", entry.Path)
			continue
		}
		t.Logf("known difference: %s holds Local's data in dataapi's bytes", entry.Path)
	}
	for _, entry := range got {
		if !wantPaths[entry.Path] {
			t.Errorf("%s: this host wrote a file Local did not", entry.Path)
		}
	}
}

// liveFrameHTML is the html a frame's payload carries, when it carries one.
func liveFrameHTML(frame string) (string, bool) {
	for _, line := range strings.Split(frame, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var payload struct {
			HTML string `json:"html"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			return "", false
		}
		return payload.HTML, payload.HTML != ""
	}
	return "", false
}

// sameIslandData reports whether two documents carry the same rules data.
func sameIslandData(a, b []byte) bool {
	left, leftOK := safeExtractIsland(string(a))
	right, rightOK := safeExtractIsland(string(b))
	if !leftOK || !rightOK || left == nil || right == nil {
		return false
	}
	return dataguard.IslandsEqual(left, right)
}

func liveGoldenCaseBody(t *testing.T, golden liveGolden, name string) []byte {
	t.Helper()
	for _, answer := range golden.Cases {
		if answer.Name == name {
			return serveDecodeBody(t, answer.Body)
		}
	}
	t.Fatalf("no recorded case %q", name)
	return nil
}
