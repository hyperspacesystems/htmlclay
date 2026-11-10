//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRelayGolden pins POST /_/sync and POST /live-sync/save to Local's own
// answers: every validation refusal in its order, the frames the relay puts on
// the live and saved lanes, and the tree the whole sequence leaves untouched.
func TestRelayGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout")
	}
	runLiveGolden(t, filepath.Join("testdata", "relay-golden"), localSource, "")
}

// relayTestDocument is a complete document, the bar both lanes hold a snapshot
// and a save to.
const relayTestDocument = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Notes</title>
  </head>
  <body>
    <h1>Reading list</h1>
  </body>
</html>
`

// snapshotTakeTestRoot is one served root with its control listener, both
// built the way the binary builds them.
func snapshotTakeTestRoot(t *testing.T, localSource, id string) (*httptest.Server, *httptest.Server) {
	t.Helper()
	root := serveGoldenRoot(t, filepath.Join("testdata", "relay-golden"))
	handler, err := newRootHandler(rootSpec{id: id, path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, controlTestServer(t, handler.control)
}

// relayPost sends one relay body and returns its status and answer.
func relayPost(t *testing.T, server *httptest.Server, path, name, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Document-URL", server.URL+"/"+name)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, answer
}

// relaySave is one POST /_/save of a complete document.
func relaySave(t *testing.T, server *httptest.Server, name string, headers map[string]string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/_/save", bytes.NewReader([]byte(relayTestDocument)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "text/html")
	request.Header.Set("Document-URL", server.URL+"/"+name)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save answered %d: %s", response.StatusCode, answer)
	}
}

// takeSnapshot asks the control API what a document owes the platform.
func takeSnapshot(t *testing.T, control *httptest.Server, id, name string) any {
	t.Helper()
	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/snapshot/take",
		map[string]any{"root": id, "name": name})
	if status != http.StatusOK {
		t.Fatalf("take answered %d: %v", status, answer)
	}
	snapshot, ok := answer["snapshot"]
	if !ok {
		t.Fatalf("take answered %v, which names no snapshot", answer)
	}
	return snapshot
}

// TestSnapshotTake is the hand-off itself: the relay's working snapshot and the
// save's provenance bit reach the control API as one entry, exactly once.
func TestSnapshotTake(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the handler reads that checkout's bundled runtime release")
	}
	const id = "root-snapshot-take"
	server, control := snapshotTakeTestRoot(t, localSource, id)

	t.Run("relay then user save", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{"snapshot": relayTestDocument, "sender": "tab-A"})
		if err != nil {
			t.Fatal(err)
		}
		if status, answer := relayPost(t, server, "/_/sync", "notes.html", string(body), nil); status != http.StatusOK {
			t.Fatalf("relay answered %d: %s", status, answer)
		}
		relaySave(t, server, "notes.html", map[string]string{"Save-Trigger": "user"})

		snapshot, ok := takeSnapshot(t, control, id, "notes.html").(map[string]any)
		if !ok {
			t.Fatalf("take answered no snapshot")
		}
		if snapshot["html"] != relayTestDocument {
			t.Errorf("html = %v, want the relayed working snapshot", snapshot["html"])
		}
		if snapshot["userDriven"] != true {
			t.Errorf("userDriven = %v, want true", snapshot["userDriven"])
		}
		if again := takeSnapshot(t, control, id, "notes.html"); again != nil {
			t.Errorf("a second take answered %v, want null", again)
		}
	})

	t.Run("user save then relay", func(t *testing.T) {
		relaySave(t, server, "floor.html", map[string]string{"Save-Trigger": "user"})
		body, err := json.Marshal(map[string]any{"snapshot": relayTestDocument, "sender": "tab-A"})
		if err != nil {
			t.Fatal(err)
		}
		if status, answer := relayPost(t, server, "/_/sync", "floor.html", string(body), map[string]string{"X-Clay-Page-Format": "1"}); status != http.StatusOK {
			t.Fatalf("relay answered %d: %s", status, answer)
		}

		snapshot, ok := takeSnapshot(t, control, id, "floor.html").(map[string]any)
		if !ok {
			t.Fatalf("take answered no snapshot")
		}
		if snapshot["html"] != relayTestDocument {
			t.Errorf("html = %v, want the relayed working snapshot", snapshot["html"])
		}
		if snapshot["userDriven"] != true {
			t.Errorf("userDriven = %v, want true: the relay must keep the save's bit", snapshot["userDriven"])
		}
	})

	t.Run("save alone", func(t *testing.T) {
		relaySave(t, server, "other.html", map[string]string{"X-Hyperclay-User-Driven": "true"})
		snapshot, ok := takeSnapshot(t, control, id, "other.html").(map[string]any)
		if !ok {
			t.Fatalf("take answered no snapshot")
		}
		if snapshot["html"] != nil {
			t.Errorf("html = %v, want null: this document was never relayed", snapshot["html"])
		}
		if snapshot["userDriven"] != true {
			t.Errorf("userDriven = %v, want true", snapshot["userDriven"])
		}
	})

	t.Run("stale entry is swept", func(t *testing.T) {
		now := snapshotNow()
		previous := snapshotNow
		t.Cleanup(func() { snapshotNow = previous })
		snapshotNow = func() int64 { return now - 6*60*1000 }
		body, err := json.Marshal(map[string]any{"snapshot": relayTestDocument, "sender": "tab-A"})
		if err != nil {
			t.Fatal(err)
		}
		if status, answer := relayPost(t, server, "/_/sync", "floor.html", string(body), map[string]string{"X-Clay-Page-Format": "1"}); status != http.StatusOK {
			t.Fatalf("relay answered %d: %s", status, answer)
		}
		snapshotNow = func() int64 { return now }
		sweepExpiredSnapshots(snapshotNow())

		if left := takeSnapshot(t, control, id, "floor.html"); left != nil {
			t.Errorf("take answered %v, want null: the entry is older than five minutes", left)
		}
	})
}
