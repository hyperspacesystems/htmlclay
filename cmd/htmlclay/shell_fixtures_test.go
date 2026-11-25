package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/shellapi"
	"github.com/panphora/htmlclay/internal/testutil"
)

var updateTranscript = flag.Bool("update", false, "rewrite testdata/shell/events.v1.ndjson from a recorded event stream")

const (
	transcriptPath    = "testdata/shell/events.v1.ndjson"
	transcriptSecret  = "5f2b8c1d4e6a79035f2b8c1d4e6a7903"
	transcriptHome    = "/Users/sam"
	transcriptEpoch   = "0123456789abcdef"
	transcriptVolume  = "0123456789ABCDEF0123456789ABCDEF"
	transcriptPort    = 1212
	transcriptVersion = "2.0.0"
	transcriptTimeout = 5 * time.Second
)

var transcriptTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`)

var transcriptHashID = regexp.MustCompile(`^(?:r[0-9a-f]{12}|[0-9a-f]{16}|[0-9a-f]{32})$`)

// TestShellEventsTranscript records the event stream the host really sends while
// the app does real actions, and checks the Swift tests' copy of it. A snapshot
// field added or renamed in Go fails this test until the transcript is regenerated:
// go test ./cmd/htmlclay -run TestShellEventsTranscript -update, then
// scripts/export-shell-fixtures.sh.
func TestShellEventsTranscript(t *testing.T) {
	if *updateTranscript {
		home, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		lines := recordShellEvents(t, home)
		if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(transcriptPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	checkTranscript(t, strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"))
	swift, err := os.ReadFile(filepath.Join("..", "..", "macos", "Tests", "PreviewCoreTests", "Fixtures", "v1", "events.v1.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, swift) {
		t.Errorf("events.v1.ndjson differs from the Swift copy; run scripts/export-shell-fixtures.sh")
	}
}

// transcriptStream holds the lines the host sent, as the reader gets them.
type transcriptStream struct {
	mu    sync.Mutex
	lines []string
	end   chan struct{}
}

func (s *transcriptStream) add(line string) {
	s.mu.Lock()
	s.lines = append(s.lines, line)
	s.mu.Unlock()
}

func (s *transcriptStream) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}

// waitSnapshot waits until a snapshot line past rev has arrived.
func (s *transcriptStream) waitSnapshot(t *testing.T, past uint64) uint64 {
	t.Helper()
	deadline := time.Now().Add(transcriptTimeout)
	for {
		rev, ok := s.latestRev()
		if ok && rev > past {
			return rev
		}
		if time.Now().After(deadline) {
			t.Fatalf("no snapshot past rev %d within %s; the stream so far is %v", past, transcriptTimeout, s.all())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *transcriptStream) latestRev() (uint64, bool) {
	var rev uint64
	found := false
	for _, line := range s.all() {
		var event struct {
			Type string `json:"type"`
			Rev  uint64 `json:"rev"`
		}
		if json.Unmarshal([]byte(line), &event) != nil || event.Type != "snapshot" {
			continue
		}
		if !found || event.Rev > rev {
			rev, found = event.Rev, true
		}
	}
	return rev, found
}

// recordShellEvents runs the app the way the Mac app does and answers the event
// stream it sent, with everything this run alone decided replaced. transcriptHost
// builds the host and the home it starts on.
func recordShellEvents(t *testing.T, home string) []string {
	t.Helper()
	notes := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(notes, "index.html"), servedTestDocument)

	a, asking := transcriptHost(t, home, notes)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	done := make(chan struct{})
	srv := httptest.NewUnstartedServer(shellapi.New(shellBackend{a}, transcriptSecret, port, done))
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/shell/v1/events?since=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+transcriptSecret)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET events = %d, want 200", resp.StatusCode)
	}

	stream := &transcriptStream{end: make(chan struct{})}
	go func() {
		defer close(stream.end)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if line := strings.TrimSpace(scanner.Text()); line != "" {
				stream.add(line)
			}
		}
	}()

	// The page asks only once the stream has opened, so the question is the
	// first change the stream carries.
	waitFor(t, "the snapshot the stream opens with", func() bool {
		_, opened := stream.latestRev()
		return opened
	})
	asked := startSetup(t, a, asking, transcriptProgram)
	rev := stream.waitSnapshot(t, 0)
	request := waitForSetupRequest(t, a, appcore.RequestFound)

	do := func(id, actionType string, args any) {
		t.Helper()
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		if _, refusal := a.do(context.Background(), appcore.Action{ID: id, Type: actionType, Args: raw}, ChannelShell); refusal != nil {
			t.Fatalf("%s refused: %+v", actionType, refusal)
		}
		rev = stream.waitSnapshot(t, rev)
	}
	do("1", "folder.serve", map[string]string{"path": notes, "name": "notes"})
	folderID := ""
	for _, folder := range a.snapshot().Folders {
		if folder.Path == notes {
			folderID = folder.ID
		}
	}
	if folderID == "" {
		t.Fatalf("the snapshot does not list %s", notes)
	}
	do("2", "folder.setFraming", map[string]any{"id": folderID, "allow": true})
	do("3", "request.answer", map[string]any{"id": request.ID, "revision": request.Revision, "answer": helperAnswerDeny})
	testutil.Receive(t, transcriptTimeout, "the setup flow the answer ends", asked)

	close(done)
	select {
	case <-stream.end:
	case <-time.After(transcriptTimeout):
		t.Fatalf("the stream did not end within %s after shutdown", transcriptTimeout)
	}

	epoch := a.snapshot().Epoch
	return normalizeTranscript(t, stream.all(), home, a.rt.port, epoch, version)
}

// normalizeTranscript replaces what only this run decided — the temp home, the
// epoch, the ids, the port, the version and the clock — with fixed values.
func normalizeTranscript(t *testing.T, lines []string, home string, port int, epoch, version string) []string {
	t.Helper()
	events := make([]any, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal([]byte(line), &events[i]); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
	}
	replacements := []string{home, transcriptHome}
	// A file's identity ends in a number, so it is replaced before the port is.
	replacements = append(replacements, transcriptFileIDs(t, home)...)
	replacements = append(replacements,
		epoch, transcriptEpoch,
		version, transcriptVersion,
		":"+strconv.Itoa(port), ":"+strconv.Itoa(transcriptPort),
	)
	for id, stable := range transcriptIDs(events, home) {
		replacements = append(replacements, id, stable)
	}
	fix := func(s string) string {
		for i := 0; i+1 < len(replacements); i += 2 {
			s = strings.ReplaceAll(s, replacements[i], replacements[i+1])
		}
		return transcriptTimestamp.ReplaceAllString(s, "2026-11-13T12:00:00Z")
	}
	out := make([]string, len(events))
	for i, event := range events {
		raw, err := json.Marshal(transcriptFix(event, fix, port))
		if err != nil {
			t.Fatal(err)
		}
		out[i] = string(raw)
	}
	return out
}

// transcriptIDs maps the ids this run minted to ids derived from what each one
// names: a folder or a program from its path, a request from its document and
// the program it asks for.
func transcriptIDs(events []any, home string) map[string]string {
	ids := map[string]string{}
	var walk func(any)
	walk = func(v any) {
		switch value := v.(type) {
		case map[string]any:
			id, _ := value["id"].(string)
			named, _ := value["path"].(string)
			if document, asked := value["document"].(string); asked {
				program, _ := value["program"].(string)
				named = document + " " + program
			}
			if transcriptHashID.MatchString(id) && strings.HasPrefix(named, home) {
				ids[id] = transcriptID(id, transcriptHome+strings.TrimPrefix(named, home))
			}
			for _, item := range value {
				walk(item)
			}
		case []any:
			for _, item := range value {
				walk(item)
			}
		}
	}
	for _, event := range events {
		walk(event)
	}
	return ids
}

func transcriptID(id, named string) string {
	sum := sha256.Sum256([]byte(named))
	h := hex.EncodeToString(sum[:])
	if id[0] == 'r' {
		return "r" + h[:len(id)-1]
	}
	return h[:len(id)]
}

// transcriptFileIDs pairs what the host derives from each file in the temp home,
// a recent document's id and a document's identity, with the same thing derived
// from the file's path under the fixed home.
func transcriptFileIDs(t *testing.T, home string) []string {
	t.Helper()
	var pairs []string
	err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		stable := transcriptHome + strings.TrimPrefix(path, home)
		pairs = append(pairs, recentID(path), recentID(stable))
		if identity := platform.DirIdentity(path); identity != "" {
			sum := sha256.Sum256([]byte(stable))
			pairs = append(pairs, identity, fmt.Sprintf("v2:%s:%d", transcriptVolume, binary.BigEndian.Uint32(sum[:4])))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pairs
}

// transcriptFix walks one decoded event, fixing every string and the host's port.
func transcriptFix(v any, fix func(string) string, port int) any {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			if key == "port" {
				if n, ok := item.(float64); ok && int(n) == port {
					value[key] = transcriptPort
					continue
				}
			}
			value[key] = transcriptFix(item, fix, port)
		}
		return value
	case []any:
		for i, item := range value {
			value[i] = transcriptFix(item, fix, port)
		}
		return value
	case string:
		return fix(value)
	}
	return v
}

// checkTranscript holds the stored stream to what the Mac app's parser expects.
func checkTranscript(t *testing.T, lines []string) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatalf("%s is empty; regenerate it and run scripts/export-shell-fixtures.sh", transcriptPath)
	}
	var lastRev uint64
	hadRev := false
	snapshots, byes := 0, 0
	for i, line := range lines {
		var event struct {
			Type     string          `json:"type"`
			Rev      uint64          `json:"rev"`
			Snapshot json.RawMessage `json:"snapshot"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d of %s is not JSON: %v", i+1, transcriptPath, err)
		}
		switch event.Type {
		case "snapshot":
			snapshots++
			if hadRev && event.Rev <= lastRev {
				t.Fatalf("line %d of %s: snapshot rev %d after rev %d; revisions must strictly increase", i+1, transcriptPath, event.Rev, lastRev)
			}
			lastRev, hadRev = event.Rev, true
			checkSnapshotDecodes(t, i+1, event.Snapshot)
		case "ping":
		case "bye":
			byes++
			if i != len(lines)-1 {
				t.Fatalf("line %d of %s: a bye line that is not the last line", i+1, transcriptPath)
			}
		default:
			t.Fatalf("line %d of %s: type %q is not snapshot, ping or bye", i+1, transcriptPath, event.Type)
		}
	}
	if snapshots < 3 {
		t.Fatalf("%s holds %d snapshot lines, want at least 3", transcriptPath, snapshots)
	}
	if byes != 1 {
		t.Fatalf("%s holds %d bye lines, want exactly 1", transcriptPath, byes)
	}
}

// checkSnapshotDecodes holds one stored snapshot to appcore.Snapshot: it decodes
// with every field known, and re-encoding it loses no field.
func checkSnapshotDecodes(t *testing.T, line int, raw json.RawMessage) {
	t.Helper()
	stale := func(why string) {
		t.Helper()
		t.Fatalf("line %d of %s: %s; regenerate it with go test ./cmd/htmlclay -run TestShellEventsTranscript -update, then scripts/export-shell-fixtures.sh", line, transcriptPath, why)
	}
	var snapshot appcore.Snapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		stale("the stored snapshot does not decode into appcore.Snapshot: " + err.Error())
	}
	var stored any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var round any
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	before, after := map[string]bool{}, map[string]bool{}
	keyPaths(stored, "", before)
	keyPaths(round, "", after)
	for key := range before {
		if !after[key] {
			stale("the stored snapshot has " + key + ", which appcore.Snapshot no longer writes")
		}
	}
	for key := range after {
		if !before[key] {
			stale("appcore.Snapshot writes " + key + ", which the stored snapshot lacks")
		}
	}
}

// keyPaths lists every key in v by its path, with [] marking a list's items.
func keyPaths(v any, prefix string, out map[string]bool) {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			path := prefix + "." + key
			out[path] = true
			keyPaths(item, path, out)
		}
	case []any:
		for _, item := range value {
			keyPaths(item, prefix+"[]", out)
		}
	}
}
