package appcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/pathrel"
)

func TestCheckServeFacts(t *testing.T) {
	home := t.TempDir()

	inputs := func(path string) ServeInputs {
		return ServeInputs{
			Path:          path,
			Name:          "site",
			Home:          home,
			NameAvailable: func(string) error { return nil },
		}
	}
	refusal := func(t *testing.T, in ServeInputs) *Refusal {
		t.Helper()
		return CheckServe(GatherServeFacts(in))
	}
	real := func(t *testing.T, path string) string {
		t.Helper()
		resolved, err := pathrel.RealPath(path)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
	mkdir := func(t *testing.T, path string) string {
		t.Helper()
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	markSync := func(t *testing.T, dir string) {
		t.Helper()
		marker := filepath.Join(dir, ".hyperclay")
		if err := os.MkdirAll(marker, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(marker, "sync-root.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expect := func(t *testing.T, got *Refusal, code, otherPath string) {
		t.Helper()
		if code == "" {
			if got != nil {
				t.Fatalf("refused %+v, want nil", got)
			}
			return
		}
		if got == nil {
			t.Fatalf("allowed the folder, want %q", code)
		}
		if got.Code != code {
			t.Fatalf("code = %q, want %q", got.Code, code)
		}
		if otherPath != "" && got.OtherPath != otherPath {
			t.Fatalf("other path = %q, want %q", got.OtherPath, otherPath)
		}
	}

	served := mkdir(t, filepath.Join(home, "Sites", "served"))
	servedIn := inputs(home)
	servedIn.Served = []config.ServedFolder{{ID: "0123456789abcdef", Name: "served", Path: served, Serving: true}}

	t.Run("symlink to a served folder", func(t *testing.T) {
		link := filepath.Join(home, "link-to-served")
		if err := os.Symlink(served, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		in := servedIn
		in.Path = link
		expect(t, refusal(t, in), RefuseOverlapsServed, real(t, served))
	})

	t.Run("inside a served folder", func(t *testing.T) {
		in := servedIn
		in.Path = mkdir(t, filepath.Join(served, "child"))
		expect(t, refusal(t, in), RefuseOverlapsServed, real(t, served))
	})

	t.Run("contains a served folder", func(t *testing.T) {
		in := servedIn
		in.Path = filepath.Join(home, "Sites")
		expect(t, refusal(t, in), RefuseOverlapsServed, real(t, served))
	})

	t.Run("served folder spelled in another letter case", func(t *testing.T) {
		mkdir(t, filepath.Join(home, "Probe"))
		if _, err := os.Stat(filepath.Join(home, "probe")); err != nil {
			t.Skip("volume is case sensitive")
		}
		caseServed := mkdir(t, filepath.Join(home, "CaseServed"))
		in := inputs(filepath.Join(home, "caseserved"))
		in.Served = []config.ServedFolder{{ID: "fedcba9876543210", Name: "case", Path: caseServed, Serving: true}}
		expect(t, refusal(t, in), RefuseOverlapsServed, real(t, caseServed))
	})

	t.Run("sync marker in the folder", func(t *testing.T) {
		dir := mkdir(t, filepath.Join(home, "marked"))
		markSync(t, dir)
		expect(t, refusal(t, inputs(dir)), RefuseBoundToLocalSync, "")
	})

	t.Run("sync marker in the parent", func(t *testing.T) {
		parent := mkdir(t, filepath.Join(home, "marked-parent"))
		markSync(t, parent)
		child := mkdir(t, filepath.Join(parent, "child"))
		expect(t, refusal(t, inputs(child)), RefuseBoundToLocalSync, "")
	})

	t.Run("sync marker two levels below", func(t *testing.T) {
		dir := mkdir(t, filepath.Join(home, "wide"))
		markSync(t, mkdir(t, filepath.Join(dir, "a", "b")))
		expect(t, refusal(t, inputs(dir)), RefuseBoundToLocalSync, "")
	})

	t.Run("sync marker four levels below is allowed", func(t *testing.T) {
		dir := mkdir(t, filepath.Join(home, "wider"))
		markSync(t, mkdir(t, filepath.Join(dir, "a", "b", "c", "d")))
		expect(t, refusal(t, inputs(dir)), "", "")
	})

	t.Run("a file is not a folder", func(t *testing.T) {
		file := filepath.Join(home, "index.html")
		if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		expect(t, refusal(t, inputs(file)), RefuseNotAFolder, "")
	})

	t.Run("a missing path is not a folder", func(t *testing.T) {
		expect(t, refusal(t, inputs(filepath.Join(home, "missing"))), RefuseNotAFolder, "")
	})

	t.Run("the home folder itself", func(t *testing.T) {
		expect(t, refusal(t, inputs(home)), RefuseIsHome, "")
	})

	t.Run("a folder outside the home", func(t *testing.T) {
		outside := mkdir(t, filepath.Join(t.TempDir(), "elsewhere"))
		expect(t, refusal(t, inputs(outside)), RefuseOutsideHome, "")
	})

	t.Run("a junction to a served folder", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions are a Windows feature")
		}
		junction := filepath.Join(home, "junction-to-served")
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", junction, served).CombinedOutput(); err != nil {
			t.Skipf("mklink unavailable: %v: %s", err, out)
		}
		in := servedIn
		in.Path = junction
		expect(t, refusal(t, in), RefuseOverlapsServed, real(t, served))
	})
}
