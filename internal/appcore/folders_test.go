package appcore

import "testing"

func TestFolderRules(t *testing.T) {
	account := Origin{Kind: OriginAccount, Username: "sam"}
	team := Origin{Kind: OriginTeam, Team: "Design"}
	local := Origin{Kind: OriginLocal}
	present := func(ff FolderFact) FolderFact { ff.OnThisComputer = true; ff.Trusted = true; return ff }
	code := func(r *Reason) string {
		if r == nil {
			return ""
		}
		return r.Code
	}

	cases := []struct {
		name      string
		connected bool
		folder    FolderFact
		parent    bool
		want      FolderCan
		serveWhy  string
		syncWhy   string
		trustWhy  string
	}{
		{name: "a trusted local folder can be served and stop being trusted",
			folder: present(FolderFact{Origin: local}),
			want:   FolderCan{Serve: true, StopTrusting: true}, syncWhy: WhyNotFromAccount},
		{name: "a served local folder can stop serving and open its workspace, not stop trusting",
			folder: present(FolderFact{Origin: local, Served: true, ServedName: "notes"}),
			want:   FolderCan{StopServing: true, OpenWorkspace: true}, syncWhy: WhyNotFromAccount, trustWhy: WhyStopServingFirst},
		{name: "an account folder needs an account connected to sync",
			folder: present(FolderFact{Origin: account, Served: true}),
			want:   FolderCan{StopServing: true, OpenWorkspace: true}, syncWhy: WhyConnectFirst, trustWhy: WhyStopServingFirst},
		{name: "a served account folder with an account connected can sync",
			connected: true, folder: present(FolderFact{Origin: account, Served: true}),
			want: FolderCan{StopServing: true, Sync: true, OpenWorkspace: true}, trustWhy: WhyStopServingFirst},
		{name: "an account folder must be served before it syncs",
			connected: true, folder: present(FolderFact{Origin: account}),
			want: FolderCan{Serve: true, StopTrusting: true}, syncWhy: WhyServeFirst},
		{name: "a synced folder can stop syncing",
			connected: true, folder: present(FolderFact{Origin: team, Served: true, Synced: true}),
			want: FolderCan{StopServing: true, StopSyncing: true, OpenWorkspace: true}, trustWhy: WhyStopServingFirst},
		{name: "a viewer cannot sync but can serve",
			connected: true, folder: present(FolderFact{Origin: team, Viewer: true}),
			want: FolderCan{Serve: true, StopTrusting: true}, syncWhy: WhyViewer},
		{name: "a missing folder cannot be served or synced",
			connected: true, folder: present(FolderFact{Origin: account, Missing: true}),
			want: FolderCan{StopTrusting: true}, serveWhy: WhyMissing, syncWhy: WhyMissing},
		{name: "a folder whose identity changed cannot be served",
			folder: present(FolderFact{Origin: local, IdentityChanged: true}),
			want:   FolderCan{StopTrusting: true}, serveWhy: WhyIdentityChanged, syncWhy: WhyNotFromAccount},
		{name: "a folder Hyperclay Local serves cannot be served",
			folder: present(FolderFact{Origin: local, ServedByLocal: true}),
			want:   FolderCan{StopTrusting: true}, serveWhy: WhyServedByLocal, syncWhy: WhyNotFromAccount},
		{name: "a folder bound to Local sync cannot be served",
			folder: present(FolderFact{Origin: local, BoundToLocalSync: true}),
			want:   FolderCan{StopTrusting: true}, serveWhy: WhyBoundToLocalSync, syncWhy: WhyNotFromAccount},
		{name: "a team folder not on this computer can do nothing yet",
			connected: true, folder: FolderFact{Origin: team},
			want: FolderCan{}, serveWhy: WhyNotOnComputer, syncWhy: WhyNotOnComputer},
		{name: "a folder inside another trusted folder can merge",
			folder: present(FolderFact{Origin: local, ParentID: "parent"}), parent: true,
			want: FolderCan{Serve: true, StopTrusting: true, Merge: true}, syncWhy: WhyNotFromAccount},
		{name: "a folder whose parent is gone cannot merge",
			folder: present(FolderFact{Origin: local, ParentID: "parent"}),
			want:   FolderCan{Serve: true, StopTrusting: true}, syncWhy: WhyNotFromAccount},
		{name: "a served folder cannot merge",
			folder: present(FolderFact{Origin: local, ParentID: "parent", Served: true}), parent: true,
			want: FolderCan{StopServing: true, OpenWorkspace: true}, syncWhy: WhyNotFromAccount, trustWhy: WhyStopServingFirst},
		{name: "a served but missing folder can stop serving and has no workspace",
			folder: present(FolderFact{Origin: local, Served: true, Missing: true}),
			want:   FolderCan{StopServing: true}, syncWhy: WhyNotFromAccount, trustWhy: WhyStopServingFirst},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.folder.ID = "f"
			f := Facts{Sync: SyncFacts{Connected: c.connected}, Folders: []FolderFact{c.folder}}
			if c.parent {
				f.Folders = append(f.Folders, present(FolderFact{ID: "parent", Origin: local}))
			}
			can, why := folderRules(f, c.folder, folderState(c.folder))
			if can != c.want {
				t.Errorf("can = %+v, want %+v", can, c.want)
			}
			if got := code(why.Serve); got != c.serveWhy {
				t.Errorf("why.serve = %q, want %q", got, c.serveWhy)
			}
			if got := code(why.Sync); got != c.syncWhy {
				t.Errorf("why.sync = %q, want %q", got, c.syncWhy)
			}
			if got := code(why.StopTrusting); got != c.trustWhy {
				t.Errorf("why.stopTrusting = %q, want %q", got, c.trustWhy)
			}
			for _, r := range []*Reason{why.Serve, why.Sync, why.StopTrusting, why.Merge} {
				if r != nil && r.Text == "" {
					t.Errorf("reason %q has no text", r.Code)
				}
			}
		})
	}
}
