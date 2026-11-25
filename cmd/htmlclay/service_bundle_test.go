//go:build service_bundle

package main

import "testing"

// TestServiceBundledIgnoresEnv is the release rule: a build that carries the
// service runs it and never a command from the environment.
func TestServiceBundledIgnoresEnv(t *testing.T) {
	t.Setenv("HTMLCLAY_SERVICE_CMD", "/bin/false")
	home := evalTempHome(t)
	a := newServiceTestApp(t, home, t.TempDir())
	if a.service.command != nil {
		t.Fatalf("the bundled build took a command from the environment: %v", a.service.command)
	}
	if a.service.install == nil || !a.service.enabled() {
		t.Fatal("the bundled build has no service to run")
	}
}
