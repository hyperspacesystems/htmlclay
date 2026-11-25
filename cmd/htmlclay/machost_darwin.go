//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/update"
)

// hostExecutable is the running binary with symlinks resolved; tests replace it.
var hostExecutable = update.InstallPath

// bundledMacHost answers the HTML Clay.app that holds this binary when it is
// the Go host inside the Swift app's bundle (X.app/Contents/MacOS/htmlclay-host).
func bundledMacHost() (string, bool) {
	exe, err := hostExecutable()
	if err != nil || filepath.Base(exe) != "htmlclay-host" {
		return "", false
	}
	return appBundleFor(exe)
}

// openThroughApp hands a launch to the Swift app; tests replace it.
var openThroughApp = func(bundle string, args []string) int {
	cmd := exec.Command("/usr/bin/open", append([]string{"-a", bundle}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

// handOffToMacApp: the bundled host never becomes the app. A launch that is not
// the app's own (--shell-stdin) and not a background check (--no-tray) opens the
// Swift app with the same document arguments.
func handOffToMacApp(shellStdin, noTray bool, args []string) (int, bool) {
	bundle, mac := bundledMacHost()
	if !mac || shellStdin || noTray {
		return 0, false
	}
	return openThroughApp(bundle, args), true
}
