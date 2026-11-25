package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/tier"
)

func TestTrustCommand(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "trust")
	home := t.TempDir()
	doc := filepath.Join(real(t, t.TempDir()), "app.html")
	writeTestFile(t, doc, tierTestPage)
	run := func(args ...string) (int, string) {
		var out, errs bytes.Buffer
		code := runTrust(storeDir, home, args, &out, &errs)
		return code, out.String() + errs.String()
	}
	tierNow := func() tier.Verdict {
		store, _, err := tier.Open(storeDir)
		if err != nil {
			t.Fatal(err)
		}
		store.NoteSeen(doc, true)
		return store.Of(tier.Input{Path: doc, Data: []byte(tierTestPage)})
	}
	if v := tierNow(); v.Tier != tier.Unknown {
		t.Fatalf("before trust the download is %s", v.Tier)
	}
	if code, out := run("trust", doc); code != trustExitOK || !strings.Contains(out, "app.html") {
		t.Fatalf("trust = %d %q", code, out)
	}
	if v := tierNow(); v.Tier != tier.Mine {
		t.Fatalf("after trust the download is %s %s", v.Tier, v.Reason)
	}
	store, _, _ := tier.Open(storeDir)
	store.Deny(doc)
	if code, _ := run("untrust", doc); code != trustExitOK {
		t.Fatalf("untrust = %d", code)
	}
	if v := tierNow(); v.Tier != tier.Unknown || v.Denied {
		t.Fatalf("after untrust the download is %s %s denied=%v; untrust must also take back Don't allow", v.Tier, v.Reason, v.Denied)
	}
	if code, out := run("developer-mode", "on"); code != trustExitOK || !strings.Contains(out, "on") {
		t.Fatalf("developer-mode on = %d %q", code, out)
	}
	if code, out := run("developer-mode"); code != trustExitOK || !strings.Contains(out, "is on") {
		t.Fatalf("developer-mode status = %d %q", code, out)
	}
	if code, out := run("developer-mode", "off"); code != trustExitOK || !strings.Contains(out, "is off") {
		t.Fatalf("developer-mode off = %d %q", code, out)
	}
	if code, out := run("programs", "enable"); code != trustExitOK || !store.Programs() || store.Developer() {
		t.Fatalf("programs enable = %d %q, programs=%v developer=%v", code, out, store.Programs(), store.Developer())
	}
	if info, err := os.Stat(programsFolder(home)); err != nil || !info.IsDir() {
		t.Fatalf("programs enable did not create %s: %v", programsFolder(home), err)
	}
	if code, out := run("programs", "disable"); code != trustExitOK || store.Programs() || !strings.Contains(out, "off") {
		t.Fatalf("programs disable = %d %q", code, out)
	}
	if code, out := run("untrust", filepath.Join(t.TempDir(), "missing.html")); code != trustExitOK || !strings.Contains(out, "missing.html") {
		t.Fatalf("untrust of a missing file = %d %q, want it to clear the saved choice", code, out)
	}
	for _, args := range [][]string{{"trust"}, {"developer-mode", "maybe"}, {"programs", "on"}, {"trust", filepath.Join(t.TempDir(), "missing.html")}} {
		if code, _ := run(args...); code == trustExitOK {
			t.Fatalf("%v succeeded", args)
		}
	}
}
