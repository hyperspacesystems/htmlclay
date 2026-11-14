package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeIdentities answers a folder's fingerprint from a map, so a test can rename a
// folder on disk and have it keep the identity it was recorded with.
type fakeIdentities map[string]string

func (f fakeIdentities) identityOf(path string) string { return f[path] }

func mustScope(t *testing.T, cfg *Config, path string, identities fakeIdentities, want Scope) {
	t.Helper()
	if got := cfg.ScopeOf(path, identities.identityOf); got != want {
		t.Fatalf("ScopeOf(%q) = %+v, want %+v", path, got, want)
	}
}

func markTeam(t *testing.T, cfg *Config, folder TeamFolder, identities fakeIdentities) {
	t.Helper()
	if err := cfg.MarkTeamFolder(folder); err != nil {
		t.Fatalf("MarkTeamFolder: %v", err)
	}
}

func TestM3TeamRecord(t *testing.T) {
	t.Run("a document under a team path is team", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		teamDir := filepath.Join(base, "team")
		if err := os.MkdirAll(filepath.Join(teamDir, "sub"), 0755); err != nil {
			t.Fatal(err)
		}
		identities := fakeIdentities{teamDir: "16777234:100"}
		markTeam(t, cfg, TeamFolder{
			FolderID: "0123456789abcdef",
			Path:     teamDir,
			Identity: identities[teamDir],
			Kind:     TeamFolderKindTeam,
			Key:      "acme",
		}, identities)

		team := Scope{Kind: ScopeTeam, Key: "team:acme"}
		mustScope(t, cfg, filepath.Join(teamDir, "sub", "doc.htmlclay"), identities, team)
		mustScope(t, cfg, teamDir, identities, team)
		// The record is its own file, and config.json carries only the marker.
		if _, err := os.Stat(filepath.Join(DirFrom(base), "team-folders.json")); err != nil {
			t.Fatalf("team-folders.json: %v", err)
		}
		if !cfg.TeamRecord {
			t.Fatal("writing the first team entry did not set the teamRecord marker")
		}
		if _, err := os.Stat(filepath.Join(base, "outside", "doc.htmlclay")); err == nil {
			t.Fatal("test setup: outside folder should not exist")
		}
		mustScope(t, cfg, filepath.Join(base, "outside", "doc.htmlclay"), identities, Scope{Kind: ScopeLocal})

		// Marking the same path twice is one entry, keeping the first Since.
		first := cfg.TeamFolderList()
		if len(first) != 1 || first[0].Since == "" {
			t.Fatalf("team record = %+v, want one entry with a Since", first)
		}
		markTeam(t, cfg, TeamFolder{
			FolderID: "0123456789abcdef",
			Path:     teamDir,
			Identity: identities[teamDir],
			Kind:     TeamFolderKindTeam,
			Key:      "acme",
			Since:    "2020-01-01T00:00:00Z",
		}, identities)
		second := cfg.TeamFolderList()
		if len(second) != 1 || second[0].Since != first[0].Since {
			t.Fatalf("marking one path twice = %+v, want one entry keeping Since %q", second, first[0].Since)
		}

		// A decision carries the scope it was decided about, and an empty scope is
		// not written at all.
		data, err := json.Marshal(HelperDecision{Document: "/docs/a.htmlclay", Name: "search", Allowed: true, Scope: team})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"scope":{"kind":"team","key":"team:acme"}`) {
			t.Fatalf("decision JSON = %s, want it to carry the team scope", data)
		}
		data, err = json.Marshal(HelperDecision{Document: "/docs/a.htmlclay", Name: "search", Allowed: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "scope") {
			t.Fatalf("decision JSON = %s, want no scope for an unscoped decision", data)
		}
	})

	t.Run("the folder renamed on disk is still team", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		oldPath := filepath.Join(base, "team")
		if err := os.MkdirAll(oldPath, 0755); err != nil {
			t.Fatal(err)
		}
		identities := fakeIdentities{oldPath: "16777234:101"}
		markTeam(t, cfg, TeamFolder{
			FolderID: "0123456789abcdea",
			Path:     oldPath,
			Identity: identities[oldPath],
			Kind:     TeamFolderKindTeam,
			Key:      "acme",
		}, identities)

		newPath := filepath.Join(base, "renamed")
		if err := os.Rename(oldPath, newPath); err != nil {
			t.Fatal(err)
		}
		identities[newPath] = "16777234:101"
		delete(identities, oldPath)
		if err := os.MkdirAll(filepath.Join(newPath, "sub"), 0755); err != nil {
			t.Fatal(err)
		}

		mustScope(t, cfg, filepath.Join(newPath, "sub", "doc.htmlclay"), identities, Scope{Kind: ScopeTeam, Key: "team:acme"})
	})

	t.Run("an account entry is local", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		accountDir := filepath.Join(base, "mine")
		if err := os.MkdirAll(filepath.Join(accountDir, "sub"), 0755); err != nil {
			t.Fatal(err)
		}
		identities := fakeIdentities{accountDir: "16777234:102"}
		markTeam(t, cfg, TeamFolder{
			FolderID: "0123456789abcdef",
			Path:     accountDir,
			Identity: identities[accountDir],
			Kind:     TeamFolderKindAccount,
			Key:      "acme",
		}, identities)

		mustScope(t, cfg, filepath.Join(accountDir, "sub", "doc.htmlclay"), identities, Scope{Kind: ScopeLocal})
	})

	t.Run("the served and trusted entries are removed and the folder is trusted again, still team", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		teamDir := filepath.Join(base, "team")
		if err := os.MkdirAll(filepath.Join(teamDir, "sub"), 0755); err != nil {
			t.Fatal(err)
		}
		identities := fakeIdentities{teamDir: "16777234:103"}
		cfg.AddTrustedFolder(teamDir, identities[teamDir])
		served, err := cfg.AddServedFolder(teamDir, identities[teamDir], "team")
		if err != nil {
			t.Fatalf("AddServedFolder: %v", err)
		}
		markTeam(t, cfg, TeamFolder{
			FolderID: served.ID,
			Path:     teamDir,
			Identity: identities[teamDir],
			Kind:     TeamFolderKindTeam,
			Key:      "acme",
		}, identities)

		doc := filepath.Join(teamDir, "sub", "doc.htmlclay")
		team := Scope{Kind: ScopeTeam, Key: "team:acme"}
		mustScope(t, cfg, doc, identities, team)

		if _, ok := cfg.RemoveServedFolder(served.ID); !ok {
			t.Fatal("RemoveServedFolder removed nothing")
		}
		if _, ok := cfg.RemoveTrustedFolder(teamDir); !ok {
			t.Fatal("RemoveTrustedFolder removed nothing")
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		mustScope(t, cfg, doc, identities, team)

		// Trusted again: a new entry, the same folder, so the same team.
		if !cfg.AddTrustedFolder(teamDir, identities[teamDir]) {
			t.Fatal("trusting the folder again added nothing")
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		mustScope(t, cfg, doc, identities, team)
	})

	t.Run("config.json saved by an older build is still team", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		teamDir := filepath.Join(base, "team")
		if err := os.MkdirAll(filepath.Join(teamDir, "sub"), 0755); err != nil {
			t.Fatal(err)
		}
		identities := fakeIdentities{teamDir: "16777234:104"}
		markTeam(t, cfg, TeamFolder{
			FolderID: "0123456789abcdef",
			Path:     teamDir,
			Identity: identities[teamDir],
			Kind:     TeamFolderKindTeam,
			Key:      "acme",
		}, identities)

		// An older build knows none of the new fields, so its struct drops them when
		// it saves. The team record has to survive that.
		older := struct {
			StartOnLogin   bool            `json:"startOnLogin"`
			TrustedFolders []TrustedFolder `json:"workspaceFolders,omitempty"`
		}{TrustedFolders: []TrustedFolder{{Path: teamDir, Identity: identities[teamDir]}}}
		data, err := json.MarshalIndent(older, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(DirFrom(base), "config.json"), data, 0600); err != nil {
			t.Fatal(err)
		}

		reloaded, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.TeamRecord {
			t.Fatal("test setup: the older build's config.json should not carry the marker")
		}
		mustScope(t, reloaded, filepath.Join(teamDir, "sub", "doc.htmlclay"), identities, Scope{Kind: ScopeTeam, Key: "team:acme"})
	})

	t.Run("broken JSON makes every path unknown and the file's bytes do not change", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		teamDir := filepath.Join(base, "team")
		if err := os.MkdirAll(teamDir, 0755); err != nil {
			t.Fatal(err)
		}
		identities := fakeIdentities{teamDir: "16777234:105"}
		recordPath := filepath.Join(DirFrom(base), "team-folders.json")
		if err := os.MkdirAll(filepath.Dir(recordPath), 0755); err != nil {
			t.Fatal(err)
		}
		broken := []byte(`{"folders": [{"path": "` + teamDir + `"`)
		if err := os.WriteFile(recordPath, broken, 0600); err != nil {
			t.Fatal(err)
		}

		unknown := Scope{Kind: ScopeUnknown, Key: ScopeUnknown}
		mustScope(t, cfg, filepath.Join(teamDir, "doc.htmlclay"), identities, unknown)
		mustScope(t, cfg, filepath.Join(base, "elsewhere", "doc.htmlclay"), identities, unknown)

		after, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(broken) {
			t.Fatalf("an unreadable record was rewritten: %q, want %q", after, broken)
		}
	})

	t.Run("a missing record with the marker is unknown", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(DirFrom(base), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(DirFrom(base), "config.json"), []byte(`{"teamRecord": true}`), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}

		unknown := Scope{Kind: ScopeUnknown, Key: ScopeUnknown}
		mustScope(t, cfg, filepath.Join(base, "team", "doc.htmlclay"), fakeIdentities{}, unknown)
		mustScope(t, cfg, filepath.Join(base, "elsewhere", "doc.htmlclay"), fakeIdentities{}, unknown)
	})

	t.Run("a missing record with no marker is local", func(t *testing.T) {
		base := t.TempDir()
		cfg, _, err := LoadFrom(base, noIdentity)
		if err != nil {
			t.Fatal(err)
		}

		mustScope(t, cfg, filepath.Join(base, "team", "doc.htmlclay"), fakeIdentities{}, Scope{Kind: ScopeLocal})
		mustScope(t, cfg, filepath.Join(base, "elsewhere", "doc.htmlclay"), fakeIdentities{}, Scope{Kind: ScopeLocal})
	})
}

// m3ScopeConfig is one machine with a team folder and a local folder, two
// programs, and the folder-identity answers the scope is read with.
type m3ScopeConfig struct {
	cfg      *Config
	ident    fakeIdentities
	teamDir  string
	teamDoc  string
	localDoc string
	owned    HelperProgram
	broad    HelperProgram
}

func newM3ScopeConfig(t *testing.T) m3ScopeConfig {
	t.Helper()
	base := t.TempDir()
	cfg, _, err := LoadFrom(base, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	teamDir := filepath.Join(base, "team")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ident := fakeIdentities{teamDir: "16777234:200"}
	markTeam(t, cfg, TeamFolder{
		FolderID: "0123456789abcdef",
		Path:     teamDir,
		Identity: ident[teamDir],
		Kind:     TeamFolderKindTeam,
		Key:      "acme",
	}, ident)
	owned, err := cfg.AddHelperProgram("search", "/helpers/search")
	if err != nil {
		t.Fatal(err)
	}
	broad, err := cfg.AddHelperProgram("ocr", "/helpers/ocr")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.SetHelperAnyDocument(broad.ID, true); !ok {
		t.Fatal("fixture could not grant the broad program")
	}
	return m3ScopeConfig{
		cfg:      cfg,
		ident:    ident,
		teamDir:  teamDir,
		teamDoc:  filepath.Join(teamDir, "doc.htmlclay"),
		localDoc: filepath.Join(base, "mine", "doc.htmlclay"),
		owned:    owned,
		broad:    broad,
	}
}

func (f m3ScopeConfig) decide(t *testing.T, document, name, program string, scope Scope) {
	t.Helper()
	if _, _, err := f.cfg.DecideHelper(HelperDecision{
		Document: document,
		Name:     name,
		Program:  program,
		Allowed:  true,
		Scope:    scope,
	}); err != nil {
		t.Fatalf("DecideHelper: %v", err)
	}
}

func (f m3ScopeConfig) resolve(t *testing.T, document, name string, trusted bool) (HelperResolution, bool) {
	t.Helper()
	return f.cfg.ResolveHelper(document, name, trusted, f.cfg.ScopeOf(document, f.ident.identityOf))
}

// TestM3ResolveScope is step 7.5's resolution half: a stored decision applies
// only in the scope it was decided about, and the broad any-document grant
// applies only in a trusted local folder — never in a team folder, whatever the
// folder's trust, and never when the record of what is a team folder is unreadable.
func TestM3ResolveScope(t *testing.T) {
	t.Run("a local decision applies in a local folder", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		f.decide(t, f.localDoc, "search", f.owned.ID, Scope{Kind: ScopeLocal})
		resolution, ok := f.resolve(t, f.localDoc, "search", false)
		if !ok || !resolution.Allowed || resolution.Program.ID != f.owned.ID {
			t.Fatalf("local decision did not apply locally: %+v, %v", resolution, ok)
		}
	})

	t.Run("an old-format decision with no scope reads as local", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		f.decide(t, f.localDoc, "search", f.owned.ID, Scope{})
		if resolution, _ := f.resolve(t, f.localDoc, "search", false); !resolution.Allowed {
			t.Fatalf("a decision stored before scopes existed stopped working: %+v", resolution)
		}
		if resolution, _ := f.cfg.ResolveHelper(f.teamDoc, "search", true, Scope{Kind: ScopeTeam, Key: "team:acme"}); resolution.Allowed {
			t.Fatalf("an old-format decision applied to a team folder: %+v", resolution)
		}
	})

	t.Run("a decision stored for a team does not apply locally", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		f.decide(t, f.localDoc, "search", f.owned.ID, Scope{Kind: ScopeTeam, Key: "team:acme"})
		if resolution, _ := f.resolve(t, f.localDoc, "search", false); resolution.Allowed {
			t.Fatalf("a team decision applied in a local folder: %+v", resolution)
		}
	})

	t.Run("a decision stored for one team does not apply in another team", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		other := filepath.Join(t.TempDir(), "other")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		markTeam(t, f.cfg, TeamFolder{
			FolderID: "fedcba9876543210",
			Path:     other,
			Identity: "16777234:201",
			Kind:     TeamFolderKindTeam,
			Key:      "other",
		}, f.ident)
		otherDoc := filepath.Join(other, "doc.htmlclay")
		f.decide(t, otherDoc, "search", f.owned.ID, Scope{Kind: ScopeTeam, Key: "team:acme"})
		if resolution, _ := f.resolve(t, otherDoc, "search", false); resolution.Allowed {
			t.Fatalf("a decision for team:acme applied in team:other: %+v", resolution)
		}
	})

	t.Run("a team decision applies in its own team folder", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		f.decide(t, f.teamDoc, "search", f.owned.ID, Scope{Kind: ScopeTeam, Key: "team:acme"})
		resolution, ok := f.resolve(t, f.teamDoc, "search", false)
		if !ok || !resolution.Allowed || resolution.Program.ID != f.owned.ID {
			t.Fatalf("a team decision did not apply in its own team: %+v, %v", resolution, ok)
		}
	})

	t.Run("a broad grant applies in a trusted local folder", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		resolution, ok := f.resolve(t, f.localDoc, "ocr", true)
		if !ok || !resolution.Allowed || resolution.Program.ID != f.broad.ID {
			t.Fatalf("the broad grant did not apply in a trusted local folder: %+v, %v", resolution, ok)
		}
	})

	t.Run("a broad grant does not apply in an untrusted local folder", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		if resolution, _ := f.resolve(t, f.localDoc, "ocr", false); resolution.Allowed {
			t.Fatalf("the broad grant applied in an untrusted folder: %+v", resolution)
		}
	})

	t.Run("a broad grant does not apply in a team folder, even trusted", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		if resolution, _ := f.resolve(t, f.teamDoc, "ocr", true); resolution.Allowed {
			t.Fatalf("the broad grant applied in a team folder: %+v", resolution)
		}
	})

	t.Run("a broad grant does not apply when the team record is unreadable", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		if err := os.Remove(f.cfg.teamFoldersPath()); err != nil {
			t.Fatal(err)
		}
		if resolution, _ := f.resolve(t, f.teamDoc, "ocr", true); resolution.Allowed {
			t.Fatalf("the broad grant applied on a machine whose team record cannot be read: %+v", resolution)
		}
	})

	t.Run("no stored decision applies when the team record is unreadable", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		f.decide(t, f.teamDoc, "search", f.owned.ID, Scope{Kind: ScopeTeam, Key: "team:acme"})
		if err := os.Remove(f.cfg.teamFoldersPath()); err != nil {
			t.Fatal(err)
		}
		if resolution, _ := f.resolve(t, f.teamDoc, "search", true); resolution.Allowed {
			t.Fatalf("a decision applied in an unknown scope: %+v", resolution)
		}
	})

	t.Run("a revocation stored for a team does not revoke a local document", func(t *testing.T) {
		f := newM3ScopeConfig(t)
		revocation := HelperDecision{
			Document: f.localDoc,
			Name:     "ocr",
			Scope:    Scope{Kind: ScopeTeam, Key: "team:acme"},
		}
		if restored := f.cfg.RestoreHelperDecisions([]HelperDecision{revocation}); restored != 1 {
			t.Fatalf("fixture restored %d revocations", restored)
		}
		if resolution, _ := f.resolve(t, f.localDoc, "ocr", true); !resolution.Allowed || resolution.Revoked {
			t.Fatalf("a team revocation revoked a local document: %+v", resolution)
		}
	})
}
