package shellapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// storedTranscript is the event stream cmd/htmlclay records through the real
// host, which the Mac app's decoding tests read a copy of.
var storedTranscript = filepath.Join("..", "..", "cmd", "htmlclay", "testdata", "shell", "events.v1.ndjson")

// storedEvent is one line of the stored transcript, read with nothing ignored.
type storedEvent struct {
	Type     string            `json:"type"`
	Rev      uint64            `json:"rev"`
	Reason   string            `json:"reason"`
	Snapshot *appcore.Snapshot `json:"snapshot"`
}

func readStoredTranscript(t *testing.T) (lines [][]byte, events []storedEvent) {
	t.Helper()
	data, err := os.ReadFile(storedTranscript)
	if err != nil {
		t.Fatalf("read the stored transcript: %v", err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var event storedEvent
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&event); err != nil {
			t.Fatalf("the stored transcript holds a line this host cannot read: %v\n%s", err, line)
		}
		lines = append(lines, line)
		events = append(events, event)
	}
	return lines, events
}

// nextStreamed is the next line the door wrote that is not a ping.
func nextStreamed(t *testing.T, lines <-chan string) map[string]any {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("the event stream ended")
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("decode event %q: %v", line, err)
			}
			if event["type"] != "ping" {
				return event
			}
		case <-deadline:
			t.Fatal("no event within 5s")
		}
	}
}

// listed is the rows a snapshot the door wrote holds under one key.
func listed(event map[string]any, key string) []map[string]any {
	snapshot, _ := event["snapshot"].(map[string]any)
	rows, _ := snapshot[key].([]any)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if fields, ok := row.(map[string]any); ok {
			out = append(out, fields)
		}
	}
	return out
}

// TestM3ExportedContract sends every snapshot of the stored transcript through
// the events door and reads what the Mac app would. Each line the door writes
// says what the stored line says, so the door drops nothing the milestone added
// and the stored file holds nothing this host cannot say; and the lines carry
// each shape the milestone added.
func TestM3ExportedContract(t *testing.T) {
	lines, events := readStoredTranscript(t)
	h := newShellTest(t)
	resp := h.request(t, http.MethodGet, "/shell/v1/events?since=1", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET events = %d, want 200", resp.StatusCode)
	}
	streamed := eventLines(t, resp.Body)

	var written []map[string]any
	for i, event := range events {
		var stored map[string]any
		if err := json.Unmarshal(lines[i], &stored); err != nil {
			t.Fatalf("decode stored line %d: %v", i+1, err)
		}
		if event.Type != "snapshot" {
			if i != len(events)-1 || event.Type != "bye" {
				t.Fatalf("stored line %d is %q, want snapshots and then one bye", i+1, event.Type)
			}
			h.closeDone()
			if got := nextStreamed(t, streamed); !reflect.DeepEqual(got, stored) {
				t.Fatalf("the door ends the stream with %v, and the stored transcript ends with %v", got, stored)
			}
			continue
		}
		if event.Snapshot == nil || event.Snapshot.Rev != event.Rev {
			t.Fatalf("stored line %d names rev %d beside a snapshot that does not carry it", i+1, event.Rev)
		}
		h.fake.push(*event.Snapshot)
		got := nextStreamed(t, streamed)
		if !reflect.DeepEqual(got, stored) {
			gotJSON, _ := json.Marshal(got)
			t.Fatalf("the door writes rev %d differently from the stored transcript:\ndoor:   %s\nstored: %s", event.Rev, gotJSON, lines[i])
		}
		written = append(written, got)
	}
	if len(written) == 0 {
		t.Fatal("the stored transcript holds no snapshot")
	}

	t.Run("a request reaches the Mac app with its answers", func(t *testing.T) {
		var asked int
		for _, event := range written {
			for _, request := range listed(event, "requests") {
				asked++
				answers, _ := request["answers"].([]any)
				if len(answers) < 2 {
					t.Fatalf("rev %v: request %v reaches the Mac app with answers %v", event["rev"], request["id"], request["answers"])
				}
			}
		}
		if asked == 0 {
			t.Fatal("no line the door wrote carries a request")
		}
	})

	t.Run("program facts and a recent document reach the Mac app", func(t *testing.T) {
		for _, event := range written {
			programs := listed(event, "programs")
			if len(programs) == 0 {
				t.Fatalf("rev %v carries no program", event["rev"])
			}
			for _, key := range []string{"id", "name", "path", "fileMissing", "allowedDocuments", "allTrustedFolders", "removedDocuments"} {
				if _, sent := programs[0][key]; !sent {
					t.Fatalf("rev %v: the program reaches the Mac app without %q", event["rev"], key)
				}
			}
			documents := listed(event, "documents")
			if len(documents) == 0 {
				t.Fatalf("rev %v carries no recent document", event["rev"])
			}
			for _, key := range []string{"id", "name", "place", "folderID", "openNow", "openedAt", "fileMissing", "versionsKept"} {
				if _, sent := documents[0][key]; !sent {
					t.Fatalf("rev %v: the recent document reaches the Mac app without %q", event["rev"], key)
				}
			}
		}
	})

	t.Run("folder ids, frames and the folder around a folder reach the Mac app", func(t *testing.T) {
		ids := map[string]string{}
		var framed, inside int
		for _, event := range written {
			folders := listed(event, "folders")
			if len(folders) == 0 {
				t.Fatalf("rev %v carries no folder", event["rev"])
			}
			for _, folder := range folders {
				path, _ := folder["path"].(string)
				id, _ := folder["id"].(string)
				if id == "" {
					t.Fatalf("rev %v: the folder at %s has no id", event["rev"], path)
				}
				if before, seen := ids[path]; seen && before != id {
					t.Fatalf("the folder at %s is %s and then %s", path, before, id)
				}
				ids[path] = id
				if folder["allowsFraming"] == true {
					framed++
				}
				if parent, _ := folder["parentID"].(string); parent != "" {
					inside++
				}
			}
		}
		if framed == 0 {
			t.Fatal("no line the door wrote says a folder allows frames")
		}
		if inside == 0 {
			t.Fatal("no line the door wrote names the folder around a folder")
		}
	})
}
