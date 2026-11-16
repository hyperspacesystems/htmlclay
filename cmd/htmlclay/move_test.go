package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/localimport"
)

const (
	moveLocalTeamRoot  = "0b6a4c2e-7b1d-4f0a-9d53-2f9c1e7a4b10"
	moveLocalPlainRoot = "5d2f8a10-3c4b-4e6f-8a9b-0c1d2e3f4a5b"
	moveLocalOldTeam   = "7e1a9b3c-2d4f-4a6b-9c8d-1e2f3a4b5c6d"
	moveLocalSession   = "f3a1c7d2-58a4-4c0b-8a57-6d3e2b1a9c44"
)

func moveThroughDo(t *testing.T, a *app, id string, folderIDs ...string) *appcore.Refusal {
	t.Helper()
	_, refusal := a.do(context.Background(), coreAction(t, id, "move.start", map[string]any{"folderIDs": folderIDs}), ChannelTray)
	return refusal
}

func moveJSON(t *testing.T, file string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return out
}

// TestM5MoveFromLocal moves three folders from a disposable Hyperclay Local data
// folder: one synced to a team, one only served, one that once held a team's pages.
func TestM5MoveFromLocal(t *testing.T) {
	scratch := t.TempDir()
	configureLog := filepath.Join(scratch, "configure")
	t.Setenv("HTMLCLAY_STUB_CONFIGURE_LOG", configureLog)
	userData := filepath.Join(scratch, "Hyperclay Local")
	t.Setenv("HTMLCLAY_LOCAL_USERDATA", userData)

	a, setDiscovery, _ := connectTestApp(t, true)
	if move := a.facts().Move; move != nil {
		t.Fatalf("with no Hyperclay Local the move is offered: %+v", move)
	}
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	team := savedSync(t, a).Accounts[1]

	studio := filepath.Join(a.rt.home, "Studio")
	plain := filepath.Join(a.rt.home, "Recipes")
	oldTeam := filepath.Join(a.rt.home, "Old team")
	writeTestFile(t, filepath.Join(studio, "board.html"), "<p>board</p>")
	writeTestFile(t, filepath.Join(plain, "index.html"), "<p>recipes</p>")
	writeTestFile(t, filepath.Join(oldTeam, "index.html"), "<p>old</p>")
	// The ports Local served these folders on. They are free, as they are once Local quits.
	studioPort, plainPort, oldTeamPort := freeLoopbackPort(t), freeLoopbackPort(t), freeLoopbackPort(t)
	marker := filepath.Join(studio, ".hyperclay", "sync-root.json")
	writeTestFile(t, marker, `{"rootId":"`+moveLocalTeamRoot+`","createdAt":"2026-10-01T00:00:00.000Z"}`)
	writeTestFile(t, filepath.Join(userData, "settings.json"), fmt.Sprintf(`{"settingsVersion":2,"deviceId":"local-device","actor":{"id":12,"username":"maya"},"roots":[`+
		`{"id":%q,"kind":"team","path":%q,"port":%d},`+
		`{"id":%q,"kind":"plain","path":%q,"port":%d},`+
		`{"id":%q,"kind":"plain","path":%q,"port":%d,"formerAccount":{"id":41,"kind":"team"}}],`+
		`"syncSessions":[{"id":%q,"rootId":%q,"accountId":40,"kind":"team","legacyMetaDir":null}]}`,
		moveLocalTeamRoot, studio, studioPort, moveLocalPlainRoot, plain, plainPort, moveLocalOldTeam, oldTeam, oldTeamPort, moveLocalSession, moveLocalTeamRoot))
	localMeta := filepath.Join(userData, "sync-meta", "v2", moveLocalSession)
	writeTestFile(t, filepath.Join(localMeta, "identity.json"), `{"rootId":"`+moveLocalTeamRoot+`","rootMarker":true,"rootDir":"1:2","accountId":40}`)
	writeTestFile(t, filepath.Join(localMeta, "node-map.json"), `{"2":{"path":"board.html"}}`)
	ourMeta := filepath.Join(a.rt.configDir, "service", "sync-meta", "v2", moveLocalSession)

	move := a.facts().Move
	if move == nil || move.State != appcore.MoveOffered || len(move.Folders) != 3 || move.Folders[0].ID != moveLocalTeamRoot || move.Folders[0].Name != "Studio" || move.Folders[0].Port != studioPort || move.Folders[0].Address == "" {
		t.Fatalf("the offer = %+v", move)
	}

	t.Run("while Hyperclay Local runs nothing moves", func(t *testing.T) {
		running := filepath.Join(userData, "served-roots.json")
		writeTestFile(t, running, fmt.Sprintf(`{"v":1,"pid":%d,"roots":[]}`, os.Getpid()))
		refusal := moveThroughDo(t, a, "m1")
		if refusal == nil || refusal.Code != appcore.RefuseLocalRunning {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseLocalRunning)
		}
		if got := moveJSON(t, marker)["rootId"]; got != moveLocalTeamRoot {
			t.Fatalf("the marker changed to %v", got)
		}
		if _, err := os.Stat(ourMeta); !os.IsNotExist(err) {
			t.Fatal("bookkeeping was copied")
		}
		if got := savedSync(t, a).SyncSessions; len(got) != 0 {
			t.Fatalf("session rows = %+v", got)
		}
		if move := a.facts().Move; move == nil || !move.LocalRunning || move.NeedsKey || !move.Folders[0].Synced || move.Folders[1].Synced {
			t.Fatalf("the offer while Local runs = %+v", move)
		}

		// A process that listens on none of Local's ports is never signalled.
		var asked []int
		a.rt.quitProcess = func(pid int, force bool) error {
			asked = append(asked, pid)
			if force {
				t.Fatal("Hyperclay Local was forced to quit")
			}
			return os.Remove(running)
		}
		a.rt.portOwner = func(int) (int, string, bool) { return 99999, "other", true }
		_, refusal = a.do(context.Background(), coreAction(t, "q1", "move.quitLocal", nil), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseLocalRunning || len(asked) != 0 {
			t.Fatalf("an unknown process: refusal = %+v, asked = %v", refusal, asked)
		}
		writeTestFile(t, running, fmt.Sprintf(`{"v":1,"pid":%d,"roots":[{"path":%q,"port":%d}]}`, os.Getpid(), studio, studioPort))
		a.rt.portOwner = func(port int) (int, string, bool) { return os.Getpid(), "Hyperclay Local", port == studioPort }
		if _, refusal = a.do(context.Background(), coreAction(t, "q2", "move.quitLocal", nil), ChannelTray); refusal != nil {
			t.Fatalf("quit refused: %+v", refusal)
		}
		if len(asked) != 1 || asked[0] != os.Getpid() {
			t.Fatalf("asked to quit = %v", asked)
		}
		if move := a.facts().Move; move == nil || move.LocalRunning {
			t.Fatalf("after the quit the offer = %+v", move)
		}
	})

	t.Run("every folder moves and Local's data is unchanged", func(t *testing.T) {
		before := len(configuresSent(t, configureLog))
		if refusal := moveThroughDo(t, a, "m2"); refusal != nil {
			t.Fatalf("move refused: %+v", refusal)
		}
		if got := moveJSON(t, marker); got["rootId"] != team.FolderID || got["createdAt"] != "2026-10-01T00:00:00.000Z" {
			t.Fatalf("the marker = %v, want rootId %s", got, team.FolderID)
		}
		identity := moveJSON(t, filepath.Join(ourMeta, "identity.json"))
		if identity["rootId"] != team.FolderID || identity["rootDir"] != "1:2" || identity["accountId"] != float64(40) {
			t.Fatalf("our identity.json = %v", identity)
		}
		if same, err := localimport.SameBookkeeping(localMeta, ourMeta); err != nil || !same {
			t.Fatalf("our bookkeeping differs from Local's: %v %v", same, err)
		}
		if got := moveJSON(t, filepath.Join(localMeta, "identity.json"))["rootId"]; got != moveLocalTeamRoot {
			t.Fatalf("Local's identity.json changed to %v", got)
		}
		rows := savedSync(t, a).SyncSessions
		if len(rows) != 1 || rows[0].ID != moveLocalSession || rows[0].RootID != team.FolderID || rows[0].AccountID != 40 || !rows[0].Enabled {
			t.Fatalf("session rows = %+v", rows)
		}
		served, ok := a.rt.cfg.ServedFolderByID(team.FolderID)
		if !ok || served.Path != studio {
			t.Fatalf("the synced folder is served as %+v, %v", served, ok)
		}
		if scope := a.scopeOf(filepath.Join(studio, "board.html")); scope != (config.Scope{Kind: config.ScopeTeam, Key: "team:40"}) {
			t.Fatalf("scope of a moved team page = %+v, want team:40", scope)
		}
		if scope := a.scopeOf(filepath.Join(oldTeam, "index.html")); scope.Kind != config.ScopeTeam {
			t.Fatalf("scope of a page in a former team folder = %+v, want a team scope", scope)
		}
		if scope := a.scopeOf(filepath.Join(plain, "index.html")); scope.Kind == config.ScopeTeam {
			t.Fatalf("a plain folder became a team folder: %+v", scope)
		}
		wanted := waitForConfigure(t, configureLog, before, func(w syncWanted) bool {
			return len(w.Sessions) == 1 && w.Sessions[0].ID == moveLocalSession
		})
		if wanted.Sessions[0].RootID != team.FolderID {
			t.Fatalf("the service was sent %+v", wanted.Sessions)
		}
		receipt, err := localimport.LoadReceipt(a.moveReceiptFile(), userData)
		if err != nil || len(receipt.Roots) != 3 {
			t.Fatalf("receipt = %+v, %v", receipt, err)
		}
		for _, entry := range receipt.Roots {
			if entry.State != localimport.StateDone || entry.FolderID == "" {
				t.Fatalf("receipt entry = %+v, want done", entry)
			}
		}
		if got := receipt.Root(moveLocalTeamRoot); got.FolderID != team.FolderID || got.SessionID != moveLocalSession || got.LocalPort != studioPort {
			t.Fatalf("the synced folder's receipt = %+v", got)
		}
		if move := a.facts().Move; move == nil || move.State != appcore.MoveDone || len(move.Folders) != 3 {
			t.Fatalf("after the move the facts say %+v", move)
		}
	})

	t.Run("an address Local served sends a page navigation to the folder's address now", func(t *testing.T) {
		served, _ := a.rt.cfg.ServedFolderByID(team.FolderID)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		ask := func(path string, navigation bool) *http.Response {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", studioPort, path), nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = fmt.Sprintf("localhost:%d", studioPort)
			if navigation {
				req.Header.Set("Sec-Fetch-Mode", "navigate")
				req.Header.Set("Sec-Fetch-Dest", "document")
				req.Header.Set("Sec-Fetch-Site", "none")
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatalf("GET %s on Local's old port: %v", path, err)
			}
			res.Body.Close()
			return res
		}
		if res := ask("/board.html?x=1", true); res.StatusCode != http.StatusFound || res.Header.Get("Location") != a.servedAddress(served)+"/board.html?x=1" {
			t.Fatalf("a bookmark answered %d to %q, want a redirect to %s/board.html?x=1", res.StatusCode, res.Header.Get("Location"), a.servedAddress(served))
		}
		if res := ask("/board.html", false); res.StatusCode != http.StatusNotFound || res.Header.Get("Location") != "" {
			t.Fatalf("a request that is not a page navigation answered %d %q", res.StatusCode, res.Header.Get("Location"))
		}
		if res := ask("/photo.png", true); res.StatusCode != http.StatusNotFound {
			t.Fatalf("a file that is not a page answered %d", res.StatusCode)
		}
		if !a.holdLocalPort(team.FolderID, studioPort) {
			t.Fatal("holding a port twice must answer that it is held")
		}
	})

	t.Run("a second move has nothing to move", func(t *testing.T) {
		if rows := savedSync(t, a).SyncSessions; len(rows) != 1 {
			t.Fatalf("session rows = %+v, want one", rows)
		}
		refusal := moveThroughDo(t, a, "m3")
		if refusal == nil || refusal.Code != appcore.RefuseNothingToMove {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseNothingToMove)
		}
	})
}

// TestM5MoveAgainAfterAnInterruptedMove starts from what a move leaves when the
// app stops between adding the session row and rewriting the marker: the folder
// is still Local's, and the next move finishes it with one session row.
func TestM5MoveAgainAfterAnInterruptedMove(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HTMLCLAY_STUB_CONFIGURE_LOG", filepath.Join(scratch, "configure"))
	userData := filepath.Join(scratch, "Hyperclay Local")
	t.Setenv("HTMLCLAY_LOCAL_USERDATA", userData)
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	team := savedSync(t, a).Accounts[1]
	studio := filepath.Join(a.rt.home, "Studio")
	writeTestFile(t, filepath.Join(studio, "board.html"), "<p>board</p>")
	marker := filepath.Join(studio, ".hyperclay", "sync-root.json")
	writeTestFile(t, marker, `{"rootId":"`+moveLocalTeamRoot+`","createdAt":"2026-10-01T00:00:00.000Z"}`)
	writeTestFile(t, filepath.Join(userData, "settings.json"), fmt.Sprintf(`{"settingsVersion":2,"deviceId":"local-device","roots":[{"id":%q,"kind":"team","path":%q,"port":%d}],`+
		`"syncSessions":[{"id":%q,"rootId":%q,"accountId":40,"kind":"team","legacyMetaDir":null}]}`, moveLocalTeamRoot, studio, freeLoopbackPort(t), moveLocalSession, moveLocalTeamRoot))
	localMeta := filepath.Join(userData, "sync-meta", "v2", moveLocalSession)
	writeTestFile(t, filepath.Join(localMeta, "identity.json"), `{"rootId":"`+moveLocalTeamRoot+`","rootMarker":true}`)
	writeTestFile(t, filepath.Join(localMeta, "node-map.json"), `{"2":{"path":"board.html"},"3":{"path":"later.html"}}`)

	// What the interrupted move left: a receipt entry that says started, a stale
	// copy of the bookkeeping, and the session row.
	receipt, _ := localimport.LoadReceipt(a.moveReceiptFile(), userData)
	receipt.Put(localimport.ReceiptRoot{LocalRootID: moveLocalTeamRoot, Path: studio, FolderID: team.FolderID, SessionID: moveLocalSession, State: localimport.StateStarted})
	if err := receipt.Save(a.moveReceiptFile()); err != nil {
		t.Fatal(err)
	}
	ourMeta := filepath.Join(a.rt.configDir, "service", "sync-meta", "v2", moveLocalSession)
	writeTestFile(t, filepath.Join(ourMeta, "identity.json"), `{"rootId":"`+team.FolderID+`","rootMarker":true}`)
	writeTestFile(t, filepath.Join(ourMeta, "node-map.json"), `{"2":{"path":"board.html"}}`)
	if err := a.moveAddSession(config.SyncSession{ID: moveLocalSession, RootID: team.FolderID, AccountID: 40, Kind: "team", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	a.rt.syncMu.Lock()
	delete(a.rt.syncBinding, moveLocalSession)
	a.rt.syncMu.Unlock()

	if move := a.facts().Move; move == nil || move.State != appcore.MoveOffered || len(move.Folders) != 1 {
		t.Fatalf("a started folder must still be offered: %+v", move)
	}
	if refusal := moveThroughDo(t, a, "m1"); refusal != nil {
		t.Fatalf("move refused: %+v", refusal)
	}
	if rows := savedSync(t, a).SyncSessions; len(rows) != 1 || rows[0].RootID != team.FolderID {
		t.Fatalf("session rows = %+v, want one for %s", rows, team.FolderID)
	}
	if same, err := localimport.SameBookkeeping(localMeta, ourMeta); err != nil || !same {
		t.Fatalf("the stale copy was kept: %v %v", same, err)
	}
	if got := moveJSON(t, marker)["rootId"]; got != team.FolderID {
		t.Fatalf("the marker names %v, want %s", got, team.FolderID)
	}
	if entry := func() *localimport.ReceiptRoot {
		r, _ := localimport.LoadReceipt(a.moveReceiptFile(), userData)
		return r.Root(moveLocalTeamRoot)
	}(); entry == nil || entry.State != localimport.StateDone {
		t.Fatalf("receipt entry = %+v, want done", entry)
	}
}
