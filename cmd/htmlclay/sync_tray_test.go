package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/panphora/htmlclay/internal/dashboard"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/tray"
)

// TestM4TrayItems covers the Windows and Linux tray's sync rows (08-sync.md 8.47).
func TestM4TrayItems(t *testing.T) {
	conflict := appcore.WaitingItem{Kind: appcore.WaitingConflict}
	other := appcore.WaitingItem{Kind: appcore.WaitingFolderMissing}
	for _, c := range []struct {
		name    string
		sync    appcore.SyncState
		waiting []appcore.WaitingItem
		want    []tray.SyncItem
	}{
		{"not connected shows nothing", appcore.SyncState{}, []appcore.WaitingItem{conflict}, nil},
		{"connected offers pause", appcore.SyncState{Connected: true}, nil,
			[]tray.SyncItem{{Action: syncTrayPause, Title: appcore.TextTrayPauseSyncing}}},
		{"paused offers resume", appcore.SyncState{Connected: true, Paused: true}, []appcore.WaitingItem{other},
			[]tray.SyncItem{{Action: syncTrayResume, Title: appcore.TextTrayResumeSyncing}}},
		{"one conflict", appcore.SyncState{Connected: true}, []appcore.WaitingItem{conflict, other},
			[]tray.SyncItem{{Action: syncTrayPause, Title: appcore.TextTrayPauseSyncing}, {Action: syncTrayConflicts, Title: appcore.TextTrayOneConflict}}},
		{"three conflicts", appcore.SyncState{Connected: true}, []appcore.WaitingItem{conflict, conflict, other, conflict},
			[]tray.SyncItem{{Action: syncTrayPause, Title: appcore.TextTrayPauseSyncing}, {Action: syncTrayConflicts, Title: fmt.Sprintf(appcore.TextTrayConflicts, 3)}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := syncTrayItems(appcore.Snapshot{Sync: c.sync, Waiting: c.waiting})
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("items = %+v, want %+v", got, c.want)
			}
		})
	}

	t.Run("the conflicts row opens Needs you", func(t *testing.T) {
		a, _, _ := connectTestApp(t, false)
		host := fmt.Sprintf("%s:%d", appcore.DashboardHost, a.rt.port)
		board, err := dashboard.New(dashboard.Options{Host: host, ProductName: "HTML Clay", API: a.dashboardAPI(http.NotFoundHandler())})
		if err != nil {
			t.Fatal(err)
		}
		a.dashboard = board
		server := httptest.NewServer(board)
		t.Cleanup(server.Close)
		var opened string
		a.rt.launchBrowser = func(target string) error {
			opened = target
			return nil
		}
		a.doSyncTray(syncTrayConflicts)
		u, err := url.Parse(opened)
		if err != nil || opened == "" {
			t.Fatalf("the tray opened %q", opened)
		}
		r, _ := http.NewRequest(http.MethodGet, server.URL+u.RequestURI(), nil)
		r.Host = host
		res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/needs-you" {
			t.Fatalf("entering = %d to %q, want 302 to /needs-you", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("pause and resume act on the host", func(t *testing.T) {
		a, _, _ := connectTestApp(t, false)
		if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
			t.Fatalf("connect refused: %+v", refusal)
		}
		a.doSyncTray(syncTrayPause)
		if !a.snapshot().Sync.Paused {
			t.Fatal("Pause Syncing left sync running")
		}
		if items := syncTrayItems(a.snapshot()); items[0].Action != syncTrayResume {
			t.Fatalf("after pausing the tray offers %q", items[0].Action)
		}
		a.doSyncTray(syncTrayResume)
		if a.snapshot().Sync.Paused {
			t.Fatal("Resume Syncing left sync paused")
		}
	})
}
