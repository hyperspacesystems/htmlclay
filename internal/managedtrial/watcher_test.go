//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/dataguard"
)

// watcherDocument is a document with a rules tag over one item list, so losing
// an item is a loss the guard judges.
func watcherDocument(items ...string) string {
	var list strings.Builder
	for _, item := range items {
		list.WriteString("\n      <li>" + item + "</li>")
	}
	return strings.Join([]string{
		"<!doctype html>",
		"<html lang=\"en\">",
		"  <head>",
		"    <meta charset=\"utf-8\">",
		"    <title>Reading list</title>",
		"    <script type=\"application/json\" data-rules-name=\"api\" data-rules-version=\"1\">{\"items\":\"ul.items li[]\"}</script>",
		"  </head>",
		"  <body>",
		"    <ul class=\"items\">" + list.String(),
		"    </ul>",
		"  </body>",
		"</html>",
		"",
	}, "\n")
}

// watcherFrames is every frame of one type the ring holds for one relative name.
func watcherFrames(t *testing.T, ring *eventRing, rel, kind string) []map[string]any {
	t.Helper()
	frames := make([]map[string]any, 0, 1)
	for _, body := range ring.heldFrames() {
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(body), &frame); err != nil {
			t.Fatalf("frame %q is not JSON: %v", body, err)
		}
		if frame["rel"] == rel && frame["type"] == kind {
			frames = append(frames, frame)
		}
	}
	return frames
}

// watcherCommit writes one document through the control API, the way Local's
// own save does, and fails unless it landed.
func watcherCommit(t *testing.T, server *httptest.Server, opID, kind, name, html string) {
	t.Helper()
	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  opID,
		"root":  "root-a",
		"name":  name,
		"kind":  kind,
		"bytes": map[string]any{"encoding": "utf8", "data": html},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("%s %s answered %d: %v", kind, name, status, answer)
	}
}

// TestWatcherOutsideEdit is the watcher's two jobs: an outside edit that loses
// data is one change frame after the file settles plus the guard's outside-edit
// pass, and the host's own write is a commit frame with no outside-edit run.
func TestWatcherOutsideEdit(t *testing.T) {
	site := controlTestRoot(t, "root-a")
	server := controlTestServer(t, site)
	watcher := site.watcher
	base := time.Now()

	// The outside edit that loses data: three items on disk, then a person's
	// editor rewrites the file in place.
	watcherCommit(t, server, "a1a1a1a1a1a1a1a1", "browser-save", "lose.html", watcherDocument("alpha", "bravo", "charlie"))
	watcher.known = watcher.snapshot()

	lose := filepath.Join(site.baseDir, "lose.html")
	for step, content := range []string{
		watcherDocument("alpha"),
		watcherDocument("alpha", "bravo"),
		watcherDocument("alpha"),
	} {
		if err := os.WriteFile(lose, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		watcher.examine(base.Add(time.Duration(step) * 100 * time.Millisecond))
	}

	// The file is still moving as far as the threshold is concerned: nothing is
	// published yet.
	watcher.examine(base.Add(200 * time.Millisecond))
	if frames := watcherFrames(t, site.events, "lose.html", "change"); len(frames) != 0 {
		t.Fatalf("a file younger than the threshold was published: %v", frames)
	}

	watcher.examine(base.Add(1500 * time.Millisecond))
	watcher.examine(base.Add(1600 * time.Millisecond))
	watcher.examine(base.Add(1700 * time.Millisecond))

	frames := watcherFrames(t, site.events, "lose.html", "change")
	if len(frames) != 1 {
		t.Fatalf("the outside edit made %d frames (%v), want one", len(frames), frames)
	}
	if frames[0]["type"] != "change" || frames[0]["event"] != "change" || frames[0]["cursor"] == "" {
		t.Fatalf("the outside edit's frame is %v, want one change frame with a cursor", frames[0])
	}
	guard, err := readGuard(site.baseDir, "lose.html")
	if err != nil {
		t.Fatalf("the outside edit left no guard record: %v", err)
	}
	if guard == nil || guard.Event == nil {
		t.Fatalf("the outside edit raised no data-loss event: %+v", guard)
	}
	if guard.Event.LossSummary.Provenance != dataguard.External {
		t.Fatalf("the data-loss event's provenance is %q, want external", guard.Event.LossSummary.Provenance)
	}

	// The host's own write is a commit frame, and the outside-edit pass must not
	// judge it: a guard run on these bytes would raise an external event.
	watcherCommit(t, server, "b2b2b2b2b2b2b2b2", "browser-save", "own.html", watcherDocument("alpha", "bravo", "charlie"))
	watcher.known = watcher.snapshot()
	watcherCommit(t, server, "c3c3c3c3c3c3c3c3", "create", "own.html", watcherDocument("alpha"))
	watcher.examine(base.Add(2 * time.Second))
	watcher.examine(base.Add(3 * time.Second))

	commits := watcherFrames(t, site.events, "own.html", "commit")
	changes := watcherFrames(t, site.events, "own.html", "change")
	if len(commits) != 2 || commits[1]["kind"] != "create" {
		t.Fatalf("the host's own writes made the commit frames %v, want the save and the create", commits)
	}
	if len(changes) != 1 {
		t.Fatalf("the host's own write made the change frames %v, want one", changes)
	}
	ownGuard, err := readGuard(site.baseDir, "own.html")
	if err != nil {
		t.Fatalf("the host's write left no guard record: %v", err)
	}
	if ownGuard == nil || ownGuard.Event != nil {
		t.Fatalf("the watcher ran the outside-edit pass on the host's own bytes: %+v", ownGuard)
	}
}
