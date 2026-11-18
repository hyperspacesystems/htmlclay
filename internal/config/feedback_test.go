package config

import (
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

func TestBuiltinProgramsStartOff(t *testing.T) {
	cfg := &Config{}
	if cfg.BuiltinOn("file-search") {
		t.Fatal("a program with no entry must read as off")
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
}

func TestBuiltinNameIsReserved(t *testing.T) {
	cfg := &Config{}
	if _, err := cfg.AddHelperProgram("file-search", "/bin/echo"); err != ErrHelperNameReserved {
		t.Fatalf("AddHelperProgram(file-search) = %v, want ErrHelperNameReserved", err)
	}
}
