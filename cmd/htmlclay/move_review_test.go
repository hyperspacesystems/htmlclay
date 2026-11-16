package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/localimport"
)

func moveReviewApp(t *testing.T) (*app, string, string) {
	t.Helper()
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
	return a, userData, scratch
}

// Review finding 1: a Local folder holding a folder this app trusts stays with
// Local whole. Its marker is not handed over, so Local keeps syncing it.
func TestM5ReviewNestedTrustedStaysWithLocal(t *testing.T) {
	a, userData, _ := moveReviewApp(t)
	studio := filepath.Join(a.rt.home, "Studio")
	writeTestFile(t, filepath.Join(studio, "board.html"), "<p>board</p>")
	writeTestFile(t, filepath.Join(studio, "projects", "plan.htmlclay"), "<p>plan</p>")
	if err := a.trustFolder(filepath.Join(studio, "projects")); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(studio, ".hyperclay", "sync-root.json")
	writeTestFile(t, marker, `{"rootId":"`+moveLocalTeamRoot+`","createdAt":"2026-10-01T00:00:00.000Z"}`)
	writeTestFile(t, filepath.Join(userData, "settings.json"), fmt.Sprintf(`{"settingsVersion":2,"deviceId":"d","roots":[{"id":%q,"kind":"team","path":%q,"port":%d}],`+
		`"syncSessions":[{"id":%q,"rootId":%q,"accountId":40,"kind":"team","legacyMetaDir":null}]}`, moveLocalTeamRoot, studio, freeLoopbackPort(t), moveLocalSession, moveLocalTeamRoot))
	localMeta := filepath.Join(userData, "sync-meta", "v2", moveLocalSession)
	writeTestFile(t, filepath.Join(localMeta, "identity.json"), `{"rootId":"`+moveLocalTeamRoot+`","rootMarker":true}`)
	writeTestFile(t, filepath.Join(localMeta, "node-map.json"), `{"2":{"path":"board.html"}}`)

	for _, id := range []string{"m1", "m2"} {
		if refusal := moveThroughDo(t, a, id); refusal == nil || refusal.Code != appcore.RefuseNothingMoved {
			t.Fatalf("%s: refusal = %+v, want %s", id, refusal, appcore.RefuseNothingMoved)
		}
		if got := moveJSON(t, marker)["rootId"]; got != moveLocalTeamRoot {
			t.Fatalf("%s: the marker names %v; Local's %s must stay", id, got, moveLocalTeamRoot)
		}
		r, _ := localimport.LoadReceipt(a.moveReceiptFile(), userData)
		if e := r.Root(moveLocalTeamRoot); e == nil || e.State != localimport.StateSkipped || e.Skip != appcore.RefuseNestedTrusted {
			t.Fatalf("%s: receipt = %+v, want skipped %s", id, e, appcore.RefuseNestedTrusted)
		}
		if rows := savedSync(t, a).SyncSessions; len(rows) != 0 {
			t.Fatalf("%s: sync.json holds %+v", id, rows)
		}
	}
}

// Review finding 6: a personal session from before Local's version 2 has no
// account id. It moves into the connected person's own account.
func TestM5ReviewLegacyPersonalSessionMoves(t *testing.T) {
	a, userData, _ := moveReviewApp(t)
	home := filepath.Join(a.rt.home, "Hyperclay")
	writeTestFile(t, filepath.Join(home, "index.html"), "<p>home</p>")
	writeTestFile(t, filepath.Join(userData, "settings.json"), fmt.Sprintf(`{"settingsVersion":2,"deviceId":"d","roots":[{"id":"r-personal","kind":"personal","path":%q,"port":%d}],`+
		`"syncSessions":[{"id":"s-personal","rootId":"r-personal","accountId":null,"kind":"personal","legacyMetaDir":"3f9a1c2b4d5e"}]}`, home, freeLoopbackPort(t)))
	writeTestFile(t, filepath.Join(userData, "sync-meta", "3f9a1c2b4d5e", "node-map.json"), `{}`)
	if refusal := moveThroughDo(t, a, "m1"); refusal != nil {
		t.Fatalf("move refused: %+v", refusal)
	}
	r, _ := localimport.LoadReceipt(a.moveReceiptFile(), userData)
	if e := r.Root("r-personal"); e == nil || e.State != localimport.StateDone {
		t.Fatalf("receipt = %+v, want done", e)
	}
	own := savedSync(t, a).Accounts[0]
	rows := savedSync(t, a).SyncSessions
	if len(rows) != 1 || rows[0].AccountID != own.AccountID || rows[0].RootID != own.FolderID {
		t.Fatalf("sync.json = %+v, want one row for account %d", rows, own.AccountID)
	}
}

// Review finding 7: a number Local wrote down before a crash, held by another
// process now, blocks neither the offer nor the move.
func TestM5ReviewStaleNumberDoesNotBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no ps")
	}
	a, userData, _ := moveReviewApp(t)
	plain := filepath.Join(a.rt.home, "Recipes")
	writeTestFile(t, filepath.Join(plain, "index.html"), "<p>r</p>")
	port := freeLoopbackPort(t)
	writeTestFile(t, filepath.Join(userData, "settings.json"), fmt.Sprintf(`{"settingsVersion":2,"deviceId":"d","roots":[{"id":"r-plain","kind":"plain","path":%q,"port":%d}],"syncSessions":[]}`, plain, port))
	writeTestFile(t, filepath.Join(userData, "served-roots.json"), fmt.Sprintf(`{"v":1,"pid":%d,"roots":[{"path":%q,"port":%d}]}`, os.Getpid(), plain, port))
	if move := a.facts().Move; move == nil || move.LocalRunning {
		t.Fatalf("offer = %+v, want Local not running", move)
	}
	if _, refusal := a.do(context.Background(), coreAction(t, "q1", "move.quitLocal", nil), ChannelTray); refusal != nil {
		t.Fatalf("quitLocal refused: %+v", refusal)
	}
	if refusal := moveThroughDo(t, a, "m1"); refusal != nil {
		t.Fatalf("move refused: %+v", refusal)
	}
}

// Review finding 5: a folder this app already trusts under another id. A synced
// one stays with a reason and its marker untouched; a former team folder moves
// under the id it is trusted with.
func TestM5ReviewAlreadyTrustedSamePath(t *testing.T) {
	a, userData, _ := moveReviewApp(t)
	studio := filepath.Join(a.rt.home, "Studio")
	oldTeam := filepath.Join(a.rt.home, "Old team")
	writeTestFile(t, filepath.Join(studio, "board.htmlclay"), "<p>board</p>")
	writeTestFile(t, filepath.Join(oldTeam, "index.htmlclay"), "<p>old</p>")
	for _, dir := range []string{studio, oldTeam} {
		if err := a.trustFolder(dir); err != nil {
			t.Fatal(err)
		}
	}
	oldID, _ := a.trustedFolderID(oldTeam)
	marker := filepath.Join(studio, ".hyperclay", "sync-root.json")
	writeTestFile(t, marker, `{"rootId":"`+moveLocalTeamRoot+`","createdAt":"2026-10-01T00:00:00.000Z"}`)
	writeTestFile(t, filepath.Join(userData, "settings.json"), fmt.Sprintf(`{"settingsVersion":2,"deviceId":"d","roots":[{"id":%q,"kind":"team","path":%q,"port":%d},{"id":%q,"kind":"plain","path":%q,"port":%d,"formerAccount":{"id":41}}],`+
		`"syncSessions":[{"id":%q,"rootId":%q,"accountId":40,"kind":"team","legacyMetaDir":null}]}`, moveLocalTeamRoot, studio, freeLoopbackPort(t), moveLocalOldTeam, oldTeam, freeLoopbackPort(t), moveLocalSession, moveLocalTeamRoot))
	localMeta := filepath.Join(userData, "sync-meta", "v2", moveLocalSession)
	writeTestFile(t, filepath.Join(localMeta, "identity.json"), `{"rootId":"`+moveLocalTeamRoot+`","rootMarker":true}`)
	writeTestFile(t, filepath.Join(localMeta, "node-map.json"), `{"2":{"path":"board.htmlclay"}}`)
	if refusal := moveThroughDo(t, a, "m1"); refusal != nil {
		t.Fatalf("move refused: %+v", refusal)
	}
	r, _ := localimport.LoadReceipt(a.moveReceiptFile(), userData)
	if e := r.Root(moveLocalTeamRoot); e == nil || e.State != localimport.StateSkipped || e.Skip != moveSkipTrustedHere {
		t.Fatalf("studio = %+v, want skipped %s", e, moveSkipTrustedHere)
	}
	if got := moveJSON(t, marker)["rootId"]; got != moveLocalTeamRoot {
		t.Fatalf("the marker names %v; Local's must stay", got)
	}
	if e := r.Root(moveLocalOldTeam); e == nil || e.State != localimport.StateDone || e.FolderID != oldID {
		t.Fatalf("old team = %+v, want done as %s", e, oldID)
	}
}
