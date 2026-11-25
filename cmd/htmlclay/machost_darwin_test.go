//go:build darwin

package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestBundledMacHost(t *testing.T) {
	origExe, origOpen := hostExecutable, openThroughApp
	t.Cleanup(func() { hostExecutable, openThroughApp = origExe, origOpen })
	const bundle = "/Applications/HTML Clay.app"
	var opened []string
	openThroughApp = func(b string, args []string) int {
		opened = append([]string{b}, args...)
		return 7
	}

	hostExecutable = func() (string, error) { return bundle + "/Contents/MacOS/htmlclay-host", nil }
	if got, ok := bundledMacHost(); !ok || got != bundle {
		t.Fatalf("bundledMacHost = %q, %v", got, ok)
	}
	if (&app{}).ownsLoginItem() {
		t.Fatal("the bundled host must not own the login item")
	}
	if code, handled := handOffToMacApp(false, false, []string{"/tmp/a.htmlclay"}); !handled || code != 7 {
		t.Fatalf("a bare launch = %d, %v", code, handled)
	}
	if want := []string{bundle, "/tmp/a.htmlclay"}; !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened %v, want %v", opened, want)
	}
	opened = nil
	if _, handled := handOffToMacApp(true, false, nil); handled || opened != nil {
		t.Fatal("a host the app started must run, not reopen the app")
	}
	if _, handled := handOffToMacApp(false, true, nil); handled || opened != nil {
		t.Fatal("a --no-tray check must run, not open the app")
	}

	hostExecutable = func() (string, error) { return bundle + "/Contents/MacOS/htmlclay", nil }
	if _, ok := bundledMacHost(); ok {
		t.Fatal("the Go-main app's own binary is not the bundled host")
	}
	if !(&app{}).ownsLoginItem() {
		t.Fatal("a Go-main HTML Clay keeps the login item")
	}
	if (&app{shell: true}).ownsLoginItem() {
		t.Fatal("a host the Mac app started never owns the login item")
	}
	if _, handled := handOffToMacApp(false, false, nil); handled {
		t.Fatal("a Go-main launch is not handed off")
	}

	hostExecutable = func() (string, error) { return "", errors.New("no executable") }
	if _, ok := bundledMacHost(); ok {
		t.Fatal("an unknown executable is not the bundled host")
	}
}
