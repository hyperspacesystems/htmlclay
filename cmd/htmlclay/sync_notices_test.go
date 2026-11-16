package main

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// TestM4Notices is 08-sync.md step 8.40: a new conflict, a refused key and a
// team the person was added to each tell the person once, as C6's frame with
// the Mac app attached and as the system's notification without it.
func TestM4Notices(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	a, setDiscovery, _ := connectTestApp(t, true)
	var bannersMu sync.Mutex
	var banners []string
	a.rt.notify = func(title, message string) error {
		bannersMu.Lock()
		banners = append(banners, title)
		bannersMu.Unlock()
		return nil
	}
	shown := func() []string {
		bannersMu.Lock()
		defer bannersMu.Unlock()
		return slices.Clone(banners)
	}
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	if got := shown(); len(got) != 0 {
		t.Fatalf("connecting notified: %v", got)
	}
	row := savedSync(t, a).Accounts[0]
	if refusal := setUpThroughDo(t, a, "s1", row.FolderID, filepath.Join(a.rt.home, "maya")); refusal != nil {
		t.Fatalf("set up refused: %+v", refusal)
	}
	waitForBinds(t, a)
	session := savedSync(t, a).SyncSessions[0]
	conflicts := filepath.Join(a.conflictsMetaDir(session), "conflicts.json")

	t.Run("a new conflict notifies once, without the Mac app", func(t *testing.T) {
		writeTestFile(t, conflicts, `{"901": {"kind": "both-edited", "path": "board.html", "detectedAt": 1790000000000}}`)
		a.readHeldConflicts()
		a.readHeldConflicts()
		if got := shown(); len(got) != 1 || got[0] != "board.html changed in both places" {
			t.Fatalf("banners = %v", got)
		}
		if !slices.Contains(savedSync(t, a).Notified, session.ID+":901:1790000000000") {
			t.Fatalf("notified = %v", savedSync(t, a).Notified)
		}
	})

	t.Run("a held conflict does not notify again after a restart", func(t *testing.T) {
		a.rt.syncMu.Lock()
		a.rt.syncConflicts = syncConflicts{}
		a.rt.syncMu.Unlock()
		a.readHeldConflicts()
		if got := shown(); len(got) != 1 {
			t.Fatalf("banners after a restart = %v", got)
		}
	})

	t.Run("with the Mac app attached a notice is a frame on the shell stream, not a banner", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		const secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		done := make(chan struct{})
		srv := httptest.NewUnstartedServer(shellapi.New(shellBackend{a}, secret, ln.Addr().(*net.TCPAddr).Port, done))
		srv.Listener = ln
		srv.Start()
		defer func() { close(done); srv.Close() }()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/shell/v1/events?since=0", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		frames := make(chan map[string]any, 16)
		go func() {
			scanner := bufio.NewScanner(resp.Body)
			scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
			for scanner.Scan() {
				var frame map[string]any
				if json.Unmarshal(scanner.Bytes(), &frame) == nil && frame["type"] == "notice" {
					frames <- frame
				}
			}
		}()
		deadline := time.Now().Add(5 * time.Second)
		for {
			a.rt.notices.mu.Lock()
			attached := len(a.rt.notices.subs)
			a.rt.notices.mu.Unlock()
			if attached > 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the stream never subscribed to notices")
			}
			time.Sleep(10 * time.Millisecond)
		}

		writeTestFile(t, conflicts, `{
  "901": {"kind": "both-edited", "path": "board.html", "detectedAt": 1790000000000},
  "902": {"kind": "remote-deleted", "path": "gone.html", "detectedAt": 1790000000100}
}`)
		a.readHeldConflicts()
		select {
		case frame := <-frames:
			want := map[string]any{"type": "notice", "id": session.ID + ":902:1790000000100", "kind": "conflict",
				"folderID": row.FolderID, "name": "gone.html", "detail": "remote-deleted"}
			for k, v := range want {
				if frame[k] != v {
					t.Fatalf("frame %s = %v, want %v (%v)", k, frame[k], v, frame)
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no notice frame")
		}
		if got := shown(); len(got) != 1 {
			t.Fatalf("a banner was shown as well: %v", got)
		}
	})

	report := func(seq int64, keyRefused bool, accounts string) {
		a.takeSyncReport(served.SyncReport{Seq: seq, KeyRefused: keyRefused, Accounts: json.RawMessage(accounts),
			Sessions: []served.SyncSessionReport{{SessionID: session.ID, RootID: row.FolderID, Status: "idle", Running: true, Paused: json.RawMessage("null")}}})
	}

	t.Run("a team the person was added to notifies once", func(t *testing.T) {
		accounts := `[{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
			`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}},` +
			`{"id":41,"kind":"team","username":"press","displayName":"Press","role":"editor","sync":{"enabled":true,"reason":null}}]`
		report(10, false, accounts)
		report(11, false, accounts)
		got := shown()
		if len(got) != 2 || got[1] != strings.Replace(appcore.TextNoticeAddedToTeam, "%s", "Press", 1) {
			t.Fatalf("banners = %v", got)
		}
	})

	t.Run("a refused key notifies once", func(t *testing.T) {
		report(12, true, "null")
		report(13, true, "null")
		got := shown()
		if len(got) != 3 || got[2] != appcore.TextWaitingSyncKeyRefused {
			t.Fatalf("banners = %v", got)
		}
	})
}

// TestM4AddedToTeamNoticeNamesTheCommandOnLinux: Linux sets a team up from the
// command line, so its notice says how (M4 review UX).
func TestM4AddedToTeamNoticeNamesTheCommandOnLinux(t *testing.T) {
	previous := serveOS
	t.Cleanup(func() { serveOS = previous })
	n := shellapi.Notice{Kind: noticeAddedToTeam, Name: "Studio"}
	serveOS = "linux"
	if _, body := noticeText(n); body != appcore.TextNoticeAddedToTeamBodyCLI {
		t.Fatalf("linux body = %q", body)
	}
	serveOS = "darwin"
	if _, body := noticeText(n); body != appcore.TextNoticeAddedToTeamBody {
		t.Fatalf("mac body = %q", body)
	}
}
