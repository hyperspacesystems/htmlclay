package appcore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// rowShape is the part of a row the rules tests name: its id, the status row's
// kind, and whether it is enabled.
type rowShape struct {
	ID       string
	Kind     string
	Enabled  bool
	Children []rowShape
}

func shapes(rows []MenuRow) []rowShape {
	if len(rows) == 0 {
		return nil
	}
	out := make([]rowShape, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowShape{ID: r.ID, Kind: r.Kind, Enabled: r.Enabled, Children: shapes(r.Children)})
	}
	return out
}

func TestMenuRowsRules(t *testing.T) {
	documents := func(n int) []Document {
		docs := make([]Document, 0, n)
		for i := range n {
			docs = append(docs, Document{ID: fmt.Sprintf("d%d", i)})
		}
		return docs
	}
	cases := []struct {
		name  string
		snap  Snapshot
		phase UpdatePhase
		want  []rowShape
	}{
		{
			name: "an empty snapshot",
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusVersion},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name:  "checking",
			phase: UpdatePhase{Checking: true},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusChecking},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name:  "ready",
			phase: UpdatePhase{Ready: "2.0.1"},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusReady, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name: "synced and pausable",
			snap: Snapshot{Sync: SyncState{Connected: true}, Host: HostState{Can: map[string]bool{"sync.setPaused": true}}},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusVersion},
				{ID: MenuSeparator},
				{ID: MenuSyncLine},
				{ID: MenuPauseSync, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name: "synced and not pausable",
			snap: Snapshot{Sync: SyncState{Connected: true}},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusVersion},
				{ID: MenuSeparator},
				{ID: MenuSyncLine},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name: "two waiting items",
			snap: Snapshot{Counts: Counts{Total: 2}, Waiting: []WaitingItem{{ID: "w1"}, {ID: "w2"}}},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusVersion},
				{ID: MenuSeparator},
				{ID: MenuNeedsYou, Enabled: true, Children: []rowShape{{ID: "w1", Enabled: true}, {ID: "w2", Enabled: true}}},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name: "ten documents",
			snap: Snapshot{Documents: documents(10)},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusVersion},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{
					{ID: "d0", Enabled: true}, {ID: "d1", Enabled: true}, {ID: "d2", Enabled: true}, {ID: "d3", Enabled: true},
					{ID: "d4", Enabled: true}, {ID: "d5", Enabled: true}, {ID: "d6", Enabled: true}, {ID: "d7", Enabled: true},
				}},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
		{
			name: "recovered changes and nothing recent",
			snap: Snapshot{RecoveredChanges: 3},
			want: []rowShape{
				{ID: MenuStatus, Kind: MenuStatusVersion},
				{ID: MenuSeparator},
				{ID: MenuOpenDashboard, Enabled: true},
				{ID: MenuOpenFile, Enabled: true},
				{ID: MenuRecent, Enabled: true, Children: []rowShape{{ID: MenuNoRecent}}},
				{ID: MenuRecovered, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuSettings, Enabled: true},
				{ID: MenuCheckUpdates, Enabled: true},
				{ID: MenuSeparator},
				{ID: MenuQuit, Enabled: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shapes(MenuRows(tc.snap, tc.phase))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rows = %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestForSystem(t *testing.T) {
	linux := RefusalText[RefusePortTaken]
	if got, want := ForSystem(linux, "linux"), "Port %s is already being used by another program on this computer. Try another number."; got != want {
		t.Fatalf("ForSystem(linux) = %q, want %q", got, want)
	}
	windows := ForSystem(linux, "windows")
	if want := "Port %s is already being used by another app on this PC. Try another number."; windows != want {
		t.Fatalf("ForSystem(windows) = %q, want %q", windows, want)
	}
}

func TestMenuTitleQuitBySystem(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin":  TextMenuQuitMac,
		"windows": TextMenuQuitWindows,
		"linux":   TextMenuQuitLinux,
	} {
		s := Snapshot{Host: HostState{OS: goos}}
		if got := MenuTitle(MenuRow{ID: MenuQuit, Enabled: true}, s, UpdatePhase{}); got != want {
			t.Errorf("MenuTitle(quit) on %s = %q, want %q", goos, got, want)
		}
	}
}

func TestMenuTitleStatus(t *testing.T) {
	s := Snapshot{Update: UpdateState{Current: "2.0.0"}}
	if got, want := MenuTitle(MenuRow{ID: MenuStatus, Kind: MenuStatusVersion}, s, UpdatePhase{}), fmt.Sprintf(TextMenuVersion, "2.0.0"); got != want {
		t.Fatalf("version row = %q, want %q", got, want)
	}
	s.Update.DownloadURL = "https://hyperclay.com/download"
	if got, want := MenuTitle(MenuRow{ID: MenuStatus, Kind: MenuStatusReady}, s, UpdatePhase{Ready: "2.0.1"}), fmt.Sprintf(TextMenuDownloadUpdate, "2.0.1"); got != want {
		t.Fatalf("download row = %q, want %q", got, want)
	}
}

// menuFixture is testdata/v1/menu.json: the same menu the Mac app's Swift test
// builds, from the same goldens.
type menuFixture struct {
	V     int        `json:"v"`
	Cases []menuCase `json:"cases"`
}

type menuCase struct {
	Name             string      `json:"name"`
	Snapshot         string      `json:"snapshot"`
	Update           UpdatePhase `json:"update"`
	RecoveredChanges int         `json:"recoveredChanges"`
	Rows             []MenuRow   `json:"rows"`
}

func TestMenuRowsMatchFixture(t *testing.T) {
	path := filepath.Join("testdata", "v1", "menu.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture menuFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for i, c := range fixture.Cases {
		snapRaw, err := os.ReadFile(filepath.Join("testdata", "v1", c.Snapshot+".snapshot.json"))
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		var s Snapshot
		if err := json.Unmarshal(snapRaw, &s); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		s.RecoveredChanges = c.RecoveredChanges
		rows := MenuRows(s, c.Update)
		encoded, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		var got []MenuRow
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if *update {
			fixture.Cases[i].Rows = rows
			continue
		}
		if !reflect.DeepEqual(got, c.Rows) {
			t.Errorf("%s: rows differ\n got %s\nwant %s", c.Name, encoded, mustJSON(c.Rows))
		}
	}
	if *update {
		var out bytes.Buffer
		encoder := json.NewEncoder(&out)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(fixture); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(raw)
}
