package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMovePortKeepsFourFormerPorts(t *testing.T) {
	cases := []struct {
		name  string
		ports []int
		want  []int
	}{{
		name:  "five moves keep the newest four former ports",
		ports: []int{4545, 9797, 3434, 1111, 2222},
		want:  []int{4545, 9797, 3434, 1111},
	}, {
		name:  "one move keeps the port it left",
		ports: []int{4545},
		want:  []int{1212},
	}, {
		name:  "no move keeps nothing",
		ports: nil,
		want:  nil,
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{ListenPort: 1212}
			for _, port := range tc.ports {
				cfg.MovePort(port)
			}
			wantLast := 1212
			if len(tc.ports) > 0 {
				wantLast = tc.ports[len(tc.ports)-1]
			}
			if got := cfg.Port(); got != wantLast {
				t.Fatalf("Port() = %d, want %d", got, wantLast)
			}
			if got := cfg.FormerPortList(); !slices.Equal(got, tc.want) {
				t.Fatalf("FormerPortList() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMovePortBackDropsTheNewPortFromFormer(t *testing.T) {
	cfg := &Config{ListenPort: 1212}
	cfg.MovePort(4545)
	cfg.MovePort(1212)
	if got, want := cfg.FormerPortList(), []int{4545}; !slices.Equal(got, want) {
		t.Fatalf("FormerPortList() = %v, want %v", got, want)
	}
	if got := cfg.Port(); got != 1212 {
		t.Fatalf("Port() = %d, want 1212", got)
	}
}

func TestBuiltinProgramsStartOn(t *testing.T) {
	cfg := &Config{}
	if !cfg.BuiltinOn("file-search") {
		t.Fatal("a program with no entry must read as on")
	}
	cfg.AllowBuiltin("file-convert", "/a.htmlclay")
	if !cfg.BuiltinOn("file-convert") {
		t.Fatal("an allow on a program with no entry switched it off")
	}
	cfg.SetBuiltinOn("file-search", false)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.BuiltinOn("file-search") {
		t.Fatalf("a switch the person turned off came back on after a save: %s", data)
	}
	cfg.SetBuiltinOn("file-search", true)
	if !cfg.BuiltinOn("file-search") {
		t.Fatal("SetBuiltinOn(true) did not switch the program on")
	}
	cfg.AllowBuiltin("file-search", "/a.htmlclay")
	cfg.AllowBuiltin("file-search", "/a.htmlclay")
	if got, want := cfg.BuiltinDocuments("file-search"), []string{"/a.htmlclay"}; !slices.Equal(got, want) {
		t.Fatalf("BuiltinDocuments() = %v, want %v", got, want)
	}
	if !cfg.BuiltinAllowed("file-search", "/a.htmlclay") {
		t.Fatal("an allowed document must read as allowed")
	}
	cfg.SetBuiltinAllTrusted("file-search", true)
	if !cfg.BuiltinAllTrusted("file-search") {
		t.Fatal("SetBuiltinAllTrusted(true) did not stick")
	}
	if got := cfg.ForgetBuiltin("file-search", ""); len(got) != 0 || !cfg.BuiltinAllowed("file-search", "/a.htmlclay") || !cfg.BuiltinAllTrusted("file-search") {
		t.Fatalf("ForgetBuiltin with no document removed %v; it must remove nothing", got)
	}
	if got, want := cfg.ForgetBuiltinAll("file-search"), []string{"/a.htmlclay"}; !slices.Equal(got, want) {
		t.Fatalf("ForgetBuiltinAll() = %v, want %v", got, want)
	}
	if cfg.BuiltinAllTrusted("file-search") {
		t.Fatal("forgetting every document must turn All documents in my trusted folders off")
	}
	if !cfg.BuiltinRemoved("file-search", "/a.htmlclay") {
		t.Fatal("a forgotten document must read as removed")
	}
	if !cfg.BuiltinOn("file-search") {
		t.Fatal("forgetting a document must leave the switch on")
	}
	if got, want := cfg.BuiltinRemovedDocuments("file-search"), []string{"/a.htmlclay"}; !slices.Equal(got, want) {
		t.Fatalf("BuiltinRemovedDocuments() = %v, want %v", got, want)
	}
	cfg.AllowBuiltin("file-search", "/a.htmlclay")
	if cfg.BuiltinRemoved("file-search", "/a.htmlclay") {
		t.Fatal("allowing again must clear the removed mark")
	}
	cfg.ForgetBuiltinAll("file-search")
	cfg.AskBuiltinAgain("file-search", "")
	if cfg.BuiltinRemoved("file-search", "/a.htmlclay") {
		t.Fatal("AskBuiltinAgain must clear the removed mark")
	}
}

func TestSetAIEditModelRefusesSpaces(t *testing.T) {
	cfg := &Config{}
	if err := cfg.SetAIEditModel("claude", "two words"); err != ErrModelName {
		t.Fatalf("SetAIEditModel(two words) = %v, want ErrModelName", err)
	}
	if err := cfg.SetAIEditModel("claude", "claude-fable-5-2"); err != nil {
		t.Fatalf("SetAIEditModel: %v", err)
	}
	if got := cfg.AIEditModels()["claude"]; got != "claude-fable-5-2" {
		t.Fatalf("AIEditModels()[claude] = %q, want claude-fable-5-2", got)
	}
	if err := cfg.SetAIEditModel("claude", ""); err != nil {
		t.Fatalf("SetAIEditModel(\"\") : %v", err)
	}
	if _, ok := cfg.AIEditModels()["claude"]; ok {
		t.Fatal("SetAIEditModel(\"\") must remove the engine's model")
	}
}

func TestFeedbackFieldsRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("HTMLCLAY_CONFIG_BASE", baseDir)
	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.MovePort(4545)
	cfg.MovePort(9797)
	cfg.SetBuiltinOn("file-search", true)
	cfg.SetBuiltinAllTrusted("file-search", true)
	cfg.AllowBuiltin("file-search", "/docs/a.htmlclay")
	cfg.AllowBuiltin("file-convert", "/docs/b.htmlclay")
	cfg.ForgetBuiltin("file-convert", "/docs/b.htmlclay")
	cfg.SetStopOpeningFiles(true)
	cfg.SetHTMLOpenWith(false)
	cfg.SetAIEditDefault("claude")
	if err := cfg.SetAIEditModel("claude", "claude-fable-5-2"); err != nil {
		t.Fatalf("SetAIEditModel: %v", err)
	}
	cfg.SetOfflineCopies(false)
	cfg.SetOfflineSettings(OfflineSettings{Mode: "full", Space: "10gb", Limited: true, Images: false, Video: true, Audio: true, LimitMB: 5000})
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := loaded.Port(); got != 9797 {
		t.Fatalf("Port() = %d, want 9797", got)
	}
	if got, want := loaded.FormerPortList(), []int{4545}; !slices.Equal(got, want) {
		t.Fatalf("FormerPortList() = %v, want %v", got, want)
	}
	if !loaded.BuiltinOn("file-search") {
		t.Fatal("the file search switch did not survive the round trip")
	}
	if !loaded.BuiltinAllTrusted("file-search") {
		t.Fatal("All documents in my trusted folders did not survive the round trip")
	}
	if got, want := loaded.BuiltinDocuments("file-search"), []string{"/docs/a.htmlclay"}; !slices.Equal(got, want) {
		t.Fatalf("BuiltinDocuments(file-search) = %v, want %v", got, want)
	}
	if got, want := loaded.BuiltinRemovedDocuments("file-convert"), []string{"/docs/b.htmlclay"}; !slices.Equal(got, want) {
		t.Fatalf("BuiltinRemovedDocuments(file-convert) = %v, want %v", got, want)
	}
	if !loaded.StoppedOpeningFiles() {
		t.Fatal("StopOpeningFiles did not survive the round trip")
	}
	if loaded.HTMLOpenWith() {
		t.Fatal("HTMLOpenWithOff did not survive the round trip")
	}
	if got := loaded.AIEditModels()["claude"]; got != "claude-fable-5-2" {
		t.Fatalf("AIEditModels()[claude] = %q, want claude-fable-5-2", got)
	}
	if loaded.OfflineCopies() {
		t.Fatal("the offline copies switch did not survive the round trip")
	}
	want := OfflineSettings{Mode: "full", Space: "10gb", Limited: true, Images: false, Video: true, Audio: true, LimitMB: 5000}
	if got := loaded.OfflineSettings(); got != want {
		t.Fatalf("OfflineSettings() = %+v, want %+v", got, want)
	}

	fresh, _, err := LoadFrom(t.TempDir(), noIdentity)
	if err != nil {
		t.Fatalf("load a fresh config: %v", err)
	}
	if got, want := fresh.OfflineSettings(), (OfflineSettings{Mode: "default", Space: "500mb", Images: true, Video: true, Audio: true}); got != want {
		t.Fatalf("a fresh config reads OfflineSettings() = %+v, want %+v", got, want)
	}
	lowBase := t.TempDir()
	if err := os.MkdirAll(DirFrom(lowBase), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(DirFrom(lowBase), "config.json"),
		[]byte(`{"offlineMode":"full","offlineLimitMB":10,"offlineNoImages":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	low, _, err := LoadFrom(lowBase, noIdentity)
	if err != nil {
		t.Fatalf("load a config whose limit is under 50 MB: %v", err)
	}
	if got, want := low.OfflineSettings(), (OfflineSettings{Mode: "full", Space: "500mb", Limited: false, Video: true, Audio: true}); got != want {
		t.Fatalf("a stored 10 MB limit reads as %+v, want %+v", got, want)
	}
	if !fresh.OfflineCopies() {
		t.Fatal("a fresh config reads offline copies as off, want on")
	}
}

func TestBuiltinNameIsReserved(t *testing.T) {
	cfg := &Config{}
	if _, err := cfg.AddHelperProgram("file-search", "/bin/echo"); err != ErrHelperNameReserved {
		t.Fatalf("AddHelperProgram(file-search) = %v, want ErrHelperNameReserved", err)
	}
	if _, err := cfg.AddHelperProgram("ai-tools", "/bin/echo"); err != ErrHelperNameReserved {
		t.Fatalf("AddHelperProgram(ai-tools) = %v, want ErrHelperNameReserved", err)
	}
	if _, err := cfg.AddHelperProgram("programs", "/bin/echo"); err != ErrHelperNameReserved {
		t.Fatalf("AddHelperProgram(programs) = %v, want ErrHelperNameReserved", err)
	}
}
