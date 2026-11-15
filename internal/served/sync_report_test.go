package served

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestM4SyncReport(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	link := newServiceLink(root.events, servicePeer{URL: "http://127.0.0.1:9", Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa"})
	host := &Host{cfg: config{service: link}}
	server := httptest.NewServer(newControlHandler(controlSecret, testControlRoots{root}, root.events, link))
	t.Cleanup(server.Close)

	post := func(body string) (int, map[string]any) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/sync/report", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", controlScheme+controlSecret)
		request.Header.Set("X-Trial-Epoch", link.currentPeer().Epoch)
		request.Header.Set("X-Trial-Peer-Epoch", link.selfEpoch)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		answer := map[string]any{}
		if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return response.StatusCode, answer
	}
	report := `{"seq":41,"sessions":[{"sessionId":"s1","rootId":"a1a2a3a4a5a6a7a8","accountId":12,"status":"syncing","running":true,"paused":null,"pendingCount":2,"conflicts":0,"blocked":0,"lastSyncAt":null,"lastError":null,"progress":{"phase":"download","done":3,"total":31,"bytesDone":1,"bytesTotal":2},"removalHeld":null,"record":{"id":"s1","paused":null}}],"keyRefused":false,"accounts":null,"events":[{"id":"s1:1","sessionId":"s1","rootId":"a1a2a3a4a5a6a7a8","at":"2026-11-15T07:00:00Z","action":"download","file":"apps/index.html","type":"site"}]}`

	t.Run("no sink means not acknowledged", func(t *testing.T) {
		status, answer := post(report)
		if status != http.StatusServiceUnavailable || answer["error"] != "not-ready" {
			t.Fatalf("answer = %d %v, want 503 not-ready", status, answer)
		}
	})

	var got []SyncReport
	host.SetSyncSink(func(r SyncReport) { got = append(got, r) })

	t.Run("an acknowledged report was delivered", func(t *testing.T) {
		status, answer := post(report)
		if status != http.StatusOK || answer["ok"] != true || answer["seq"] != float64(41) {
			t.Fatalf("answer = %d %v, want 200 ok seq 41", status, answer)
		}
		if len(got) != 1 {
			t.Fatalf("the sink got %d reports, want 1", len(got))
		}
		r := got[0]
		if r.Seq != 41 || len(r.Sessions) != 1 || r.Sessions[0].Progress == nil || r.Sessions[0].Progress.Total != 31 {
			t.Fatalf("report = %+v", r)
		}
		if string(r.Sessions[0].Record) != `{"id":"s1","paused":null}` {
			t.Fatalf("record = %s", r.Sessions[0].Record)
		}
		if len(r.Events) != 1 || r.Events[0].ID != "s1:1" || r.Events[0].Action != "download" {
			t.Fatalf("events = %+v", r.Events)
		}
	})

	t.Run("a garbled report is refused and not delivered", func(t *testing.T) {
		before := len(got)
		if status, _ := post(`{"seq":"x"}`); status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if status, _ := post(`{"seq":-1}`); status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if len(got) != before {
			t.Fatalf("the sink got a refused report")
		}
	})

	t.Run("a detached service is not heard", func(t *testing.T) {
		link.detached.Store(true)
		t.Cleanup(func() { link.detached.Store(false) })
		before := len(got)
		if status, _ := post(report); status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
		if len(got) != before {
			t.Fatalf("the sink got a report while detached")
		}
	})
}
