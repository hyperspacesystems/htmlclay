package served

import (
	"net/http"
	"testing"
)

// TestM3ProgramWriteAsksRecover is which writes ask the service to keep an open
// room's unsaved edits as a recovered copy: every write a program makes, and no
// write a page makes, which carries what its own room holds.
func TestM3ProgramWriteAsksRecover(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path != "/v1/admit" {
			return nil
		}
		return map[string]any{"decision": "proceed", "logical": body["incoming"], "file": body["incoming"]}
	})
	peer := consultSavePeer(t, script)
	host := switchTestHost(t)
	s := guardV2Serve(t, host)
	if err := host.SetServicePeer(peer.URL, peer.Secret, peer.Epoch); err != nil {
		t.Fatalf("SetServicePeer: %v", err)
	}
	s.put(t, guardV2Doc("List", "first note", "alpha"))
	data := func(note string) string {
		return `{"title":"List","note":"` + note + `","items":[{"name":"alpha"}]}`
	}

	var saved DocumentSaved
	writes := []struct {
		name  string
		write func() error
		want  bool
	}{
		{"the lane's save", func() (err error) {
			_, etag := agentSaveRead(t, s)
			saved, err = agentSaveDo(s, guardV2Doc("List", "the agent's note", "alpha"), etag, "")
			return err
		}, true},
		{"the lane's restore", func() error {
			_, err := agentSaveRestoreDo(s, saved.Replaced, saved.Etag)
			return err
		}, true},
		{"the lane's data write", func() error {
			_, err := host.WriteData(guardV2RootID, guardV2Name, DataWrite{Data: []byte(data("the lane's note"))})
			return err
		}, true},
		{"a program's data write", func() error {
			if answer := s.programWrite(t, data("a program's note"), nil); answer.status != http.StatusOK {
				t.Fatalf("a program's data write answered %s, want 200", answer)
			}
			return nil
		}, true},
		{"a page's data write", func() error {
			if answer := s.pageWrite(t, data("the page's note")); answer.status != http.StatusOK {
				t.Fatalf("a page's data write answered %s, want 200", answer)
			}
			return nil
		}, false},
		{"a page's save", func() error {
			s.pageSave(t, guardV2Doc("List", "the page's saved note", "alpha"))
			return nil
		}, false},
	}
	for index, write := range writes {
		if err := write.write(); err != nil {
			t.Fatalf("%s: %v", write.name, err)
		}
		admits := script.find("/v1/admit")
		if len(admits) != index+1 {
			t.Fatalf("after %s the service was asked to admit %d writes, want %d", write.name, len(admits), index+1)
		}
		if got, ok := admits[index].Body["recover"].(bool); !ok || got != write.want {
			t.Fatalf("%s sent recover %v, want %v", write.name, admits[index].Body["recover"], write.want)
		}
	}
}
