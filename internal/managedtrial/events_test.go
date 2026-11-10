//go:build managedtrial

package managedtrial

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// eventFrames is every frame the ring currently holds, decoded.
func eventFrames(t *testing.T, ring *eventRing) []map[string]any {
	t.Helper()
	bodies := ring.heldFrames()
	frames := make([]map[string]any, 0, len(bodies))
	for _, body := range bodies {
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(body), &frame); err != nil {
			t.Fatalf("frame %q is not JSON: %v", body, err)
		}
		frames = append(frames, frame)
	}
	return frames
}

// eventLine reads the stream's first line and closes the stream.
func eventLine(t *testing.T, server *httptest.Server, since string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/events?since="+url.QueryEscape(since), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+controlSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the stream answered %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("the stream wrote no line for since=%q: %v", since, err)
		}
		line = strings.TrimSpace(line)
		if line == "{}" {
			continue
		}
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("the stream wrote %q, which is not JSON: %v", line, err)
		}
		return frame
	}
}

// TestEventsCursor is the frame stream: an HTTP save's commit frame and a
// control commit's, in the order they happened; the second replayed from the
// first's cursor; and a cursor from another process answered with a gap.
func TestEventsCursor(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the HTTP save route reads that checkout's bundled runtime release")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := newRootHandler(rootSpec{id: "root-a", path: dir}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	site := root.control
	rootServer := httptest.NewServer(root)
	defer rootServer.Close()
	controlServer := controlTestServer(t, site)

	doc := "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>Doc</title>\n</head>\n<body>\n<h1>One</h1>\n</body>\n</html>\n"
	request, err := http.NewRequest(http.MethodPost, rootServer.URL+"/_/save", strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Document-URL", rootServer.URL+"/a.html")
	request.Header.Set("Content-Type", "text/plain")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the HTTP save answered %d", response.StatusCode)
	}

	status, answer := controlJSON(t, controlServer, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  "control-1",
		"root":  "root-a",
		"name":  "b.html",
		"kind":  "create",
		"bytes": map[string]any{"encoding": "utf8", "data": doc},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("the control commit answered %d: %v", status, answer)
	}

	frames := eventFrames(t, site.events)
	if len(frames) != 2 {
		t.Fatalf("the ring holds %d frames (%v), want the two commits", len(frames), frames)
	}
	saved := frames[0]
	if saved["type"] != "commit" || saved["kind"] != "browser-save" || saved["rel"] != "a.html" || saved["outcome"] != "committed" {
		t.Fatalf("the first frame is %v, want the HTTP save's commit frame", saved)
	}
	if saved["opId"] == "" || saved["opId"] == nil {
		t.Fatalf("the HTTP save's frame carries no opId: %v", saved)
	}
	created := frames[1]
	if created["type"] != "commit" || created["kind"] != "create" || created["rel"] != "b.html" || created["opId"] != "control-1" {
		t.Fatalf("the second frame is %v, want the control commit's", created)
	}
	firstCursor, _ := saved["cursor"].(string)
	if firstCursor == "" {
		t.Fatalf("the first frame carries no cursor: %v", saved)
	}

	replayed := eventLine(t, controlServer, firstCursor)
	if replayed["type"] != "commit" || replayed["opId"] != "control-1" {
		t.Fatalf("resuming from %q replayed %v, want the control commit", firstCursor, replayed)
	}

	status, op := controlJSON(t, controlServer, controlSecret, http.MethodGet, "/v1/op/"+saved["opId"].(string), nil)
	if status != http.StatusOK || op["state"] != "committed" {
		t.Fatalf("the HTTP save's op answers %d: %v, want committed", status, op)
	}

	// A version restore over HTTP is a write too. It reaches the commit core
	// through publishHostWrite rather than the save route, and the stream and
	// P3's counts must see it. Every frame names the root it happened in, so an
	// adapter for one root can ignore another root's frames.
	versions, err := os.ReadDir(filepath.Join(dir, ".hyperclay", "versions", "a"))
	if err != nil || len(versions) == 0 {
		t.Fatalf("the save left no version to restore: %v", err)
	}
	restore, err := http.NewRequest(http.MethodPost, rootServer.URL+"/_/restore/a.html/"+versions[0].Name(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(restore)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the HTTP restore answered %d", response.StatusCode)
	}
	frames = eventFrames(t, site.events)
	last := frames[len(frames)-1]
	if last["type"] != "commit" || last["kind"] != "restore" || last["rel"] != "a.html" || last["outcome"] != "committed" {
		t.Fatalf("the newest frame is %v, want the HTTP restore's commit frame", last)
	}
	for _, frame := range frames {
		if frame["root"] != "root-a" {
			t.Errorf("frame %v does not name root-a", frame)
		}
	}

	// A restart: another process, so another epoch, and the old cursor is not
	// resumable.
	restarted := newControlRoot("root-a", site.baseDir, site.registry, site.commit, newEventRing())
	restartedServer := controlTestServer(t, restarted)
	gap := eventLine(t, restartedServer, firstCursor)
	if gap["type"] != "gap" || gap["cursor"] == "" {
		t.Fatalf("after a restart since=%q answered %v, want a gap first", firstCursor, gap)
	}
}
