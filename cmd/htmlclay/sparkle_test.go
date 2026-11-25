package main

import (
	"testing"
	"time"
)

func TestAppBundleFor(t *testing.T) {
	cases := []struct {
		exe    string
		bundle string
		ok     bool
	}{
		{"/Applications/HTMLClay.app/Contents/MacOS/htmlclay", "/Applications/HTMLClay.app", true},
		{"/Users/x/Apps/HTML Clay.app/Contents/MacOS/htmlclay", "/Users/x/Apps/HTML Clay.app", true},
		{"/Users/x/go/bin/htmlclay", "", false},
		{"/Applications/HTMLClay.app/Contents/Resources/htmlclay", "", false},
		{"/tmp/Contents/MacOS/htmlclay", "", false},
	}
	for _, tc := range cases {
		bundle, ok := appBundleFor(tc.exe)
		if bundle != tc.bundle || ok != tc.ok {
			t.Errorf("appBundleFor(%q) = %q, %v; want %q, %v", tc.exe, bundle, ok, tc.bundle, tc.ok)
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
