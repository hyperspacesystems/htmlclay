//go:build service_bundle

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestExtractedServiceRuns is the packaged-folder check: the embedded program,
// extracted, passes its self-check (every lazily loaded part and every file it
// reads by path, service/self-check.cjs) and starts and attaches, with no other
// Node, npm or Bun reachable.
func TestExtractedServiceRuns(t *testing.T) {
	if !Bundled() {
		t.Fatal("built with service_bundle but carries no program")
	}
	cmd, err := Install(t.TempDir(), false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	noPath := []string{"PATH=" + filepath.Join(t.TempDir(), "empty")}

	check := exec.Command(cmd[0], cmd[1], "--self-check")
	check.Env = append(os.Environ(), noPath...)
	out, err := check.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "SELF-CHECK OK") {
		t.Fatalf("self-check: %v\n%s", err, out)
	}

	server := stubGo(t)
	log := &lineLog{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	p, ready, err := Start(ctx, Spec{Command: cmd, DataDir: t.TempDir(), Env: noPath, Log: log.add}, fakeSecret, 20*time.Second)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(p.Kill)
	if ready.URL == "" || ready.Epoch == "" {
		t.Fatalf("ready %+v", ready)
	}
	if err := p.Attach(Host{URL: server.URL, Secret: fakeSecret, Epoch: "a1b2c3d4e5f60718"}, 20*time.Second); err != nil {
		t.Fatalf("attach: %v; log %q", err, log.all())
	}
	if err := p.Close(5 * time.Second); err != nil {
		t.Fatalf("close: %v", err)
	}
	if code := p.ExitCode(); code != 0 {
		t.Fatalf("exit code %d, want 0; log %q", code, log.all())
	}
}
