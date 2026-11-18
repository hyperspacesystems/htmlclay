package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/service"
	"github.com/panphora/htmlclay/internal/tailwind"
	"github.com/panphora/htmlclay/internal/update"
)

// `htmlclay update-now --version X.Y.Z` asks the running app to update itself
// now, through the same restart as the tray's Restart to Update row. A release
// runs it from scripts/update-canary.sh to prove the update path on a real
// install. It is a client of the running app, like the wire CLI: it never
// takes the single-instance lock and never starts the app. The request travels
// as a command frame over the single-instance socket, which only this user can
// reach, and the app only ever installs a version its own updater downloaded
// from the signed feed and verified.
const (
	// accepted: the app is updating to that version, or will as soon as
	// Sparkle has it prepared. "already X.Y.Z": it is running that version.
	updateNowExitOK = 0
	// Bad arguments.
	updateNowExitUsage = 1
	// Nothing answers on the single-instance socket: the app is not running.
	updateNowExitNotRunning = 2
	// "refused <reason>": the app cannot update itself, so nothing happened.
	updateNowExitRefused = 3
	// The app read the request and did not answer: it predates update-now.
	updateNowExitTooOld = 4
	// "failed <reason>": the update cannot go on, and installing by hand now
	// would race an install already set up (a cancelled restart, or Sparkle
	// holding another version that installs at the next quit).
	updateNowExitFailed = 5
	// "starting": the app holds the lock but is not taking commands yet, so
	// the request can be sent again shortly. "checking": on Windows and Linux
	// the app does not have that version downloaded yet and is looking for it
	// now, so the same holds.
	updateNowExitStarting = 6
)

// updateNowChecking is the answer while the self-updater looks for the version.
const updateNowChecking = "checking"

// updateNowOS is the platform update-now runs on. Tests change it.
var updateNowOS = runtime.GOOS

// beginSparkle is update.BeginSparkle; tests replace it.
var beginSparkle = update.BeginSparkle

// shellUpdateStep is told each stage update-prepare passes; tests record it.
var shellUpdateStep = func(stage string) {}

// shellInstallPath is update.InstallPath; tests replace it with a fake bundle.
var shellInstallPath = update.InstallPath

// updateNowDispatch runs `htmlclay update-now ...` and reports whether it
// handled argv. Like wireDispatch it runs before flag.Parse and the lock.
func updateNowDispatch(argv []string) (int, bool) {
	if len(argv) < 2 {
		return 0, false
	}
	switch argv[1] {
	case "update-info":
		return runUpdateInfo(argv[2:], os.Stdout, os.Stderr), true
	case "update-now", "update-confirm", "update-prepare", "update-abort", "update-status":
	default:
		return 0, false
	}
	dir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "htmlclay %s: cannot resolve the config dir: %v\n", argv[1], err)
		return updateNowExitUsage, true
	}
	si := platform.NewSingleInstance(dir)
	switch argv[1] {
	case "update-confirm":
		return runUpdateConfirm(si, argv[2:], os.Stdout, os.Stderr), true
	case "update-prepare":
		if len(argv) == 2 {
			fmt.Fprintln(os.Stderr, "usage: htmlclay update-prepare X.Y.Z PID restart|quit [force]")
			return updateNowExitUsage, true
		}
		return runHostCommand(si, argv[1], argv[2:], []string{"prepared"}, os.Stdout, os.Stderr), true
	case "update-abort":
		if len(argv) == 2 {
			fmt.Fprintln(os.Stderr, "usage: htmlclay update-abort TXID")
			return updateNowExitUsage, true
		}
		return runHostCommand(si, argv[1], argv[2:], []string{"aborted"}, os.Stdout, os.Stderr), true
	case "update-status":
		return runHostCommand(si, argv[1], argv[2:], []string{"ok", "off", "failed"}, os.Stdout, os.Stderr), true
	}
	return runUpdateNow(si, argv[2:], os.Stdout, os.Stderr), true
}

// updateInfo is what this binary is: the version and channel it was built
// with, and whether it carries the live-room service and the Tailwind
// compiler. The Mac build checks it against the bundle's Info.plist, because
// the update supervisor only accepts a host whose version matches the feed.
type updateInfo struct {
	Version  string `json:"version"`
	Channel  string `json:"channel"`
	Service  bool   `json:"service"`
	Tailwind bool   `json:"tailwind"`
}

// runUpdateInfo prints one JSON line and needs no running app and no config
// folder.
func runUpdateInfo(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: htmlclay update-info")
		return updateNowExitUsage
	}
	line, err := json.Marshal(updateInfo{Version: version, Channel: updateChannel, Service: service.Bundled(), Tailwind: tailwind.Bundled()})
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay update-info: %v\n", err)
		return updateNowExitUsage
	}
	fmt.Fprintln(stdout, string(line))
	return updateNowExitOK
}

// runUpdateNow sends the request and prints the app's answer on stdout.
func runUpdateNow(si platform.SingleInstance, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update-now", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("version", "", "the version to update to, X.Y.Z")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !update.IsStrictVersion(*target) {
		fmt.Fprintln(stderr, "usage: htmlclay update-now --version X.Y.Z")
		return updateNowExitUsage
	}
	answer, err := si.SendCommand("update-now " + *target)
	if errors.Is(err, platform.ErrNoAnswer) {
		fmt.Fprintln(stdout, "refused the running HTML Clay predates update-now")
		return updateNowExitTooOld
	}
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay update-now: HTML Clay is not running: %v\n", err)
		return updateNowExitNotRunning
	}
	fmt.Fprintln(stdout, answer)
	switch {
	case answer == "accepted", answer == "already "+*target:
		return updateNowExitOK
	case answer == platform.CommandStarting, answer == updateNowChecking:
		return updateNowExitStarting
	case strings.HasPrefix(answer, "failed "):
		return updateNowExitFailed
	default:
		return updateNowExitRefused
	}
}

// runUpdateConfirm is how the Mac app tells its host it is up: the host then
// reports serving to an update supervisor and settles the config saves it held
// while the update was being judged. It answers "confirmed" and exits 0.
func runUpdateConfirm(si platform.SingleInstance, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: htmlclay update-confirm")
		return updateNowExitUsage
	}
	answer, err := si.SendCommand("update-confirm")
	if errors.Is(err, platform.ErrNoAnswer) {
		fmt.Fprintln(stdout, "refused the running HTML Clay predates update-confirm")
		return updateNowExitTooOld
	}
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay update-confirm: HTML Clay is not running: %v\n", err)
		return updateNowExitNotRunning
	}
	fmt.Fprintln(stdout, answer)
	switch answer {
	case "confirmed":
		return updateNowExitOK
	case platform.CommandStarting:
		return updateNowExitStarting
	}
	return updateNowExitRefused
}

// runHostCommand sends one of the update commands the Mac app drives and maps
// the answer to an exit code. The host checks the arguments; the client only
// knows which first words mean the host took the request.
func runHostCommand(si platform.SingleInstance, name string, args []string, ok []string, stdout, stderr io.Writer) int {
	answer, err := si.SendCommand(strings.Join(append([]string{name}, args...), " "))
	if errors.Is(err, platform.ErrNoAnswer) {
		fmt.Fprintf(stdout, "refused the running HTML Clay predates %s\n", name)
		return updateNowExitTooOld
	}
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay %s: HTML Clay is not running: %v\n", name, err)
		return updateNowExitNotRunning
	}
	fmt.Fprintln(stdout, answer)
	if answer == platform.CommandStarting {
		return updateNowExitStarting
	}
	first, _, _ := strings.Cut(answer, " ")
	for _, word := range ok {
		if first == word {
			return updateNowExitOK
		}
	}
	return updateNowExitRefused
}

// handleInstanceCommand answers one command frame from the single-instance
// socket. The socket only carries frames from this user's own processes.
func (a *app) handleInstanceCommand(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "refused unknown command"
	}
	switch fields[0] {
	case "update-now":
		if len(fields) != 2 {
			return "refused unknown command"
		}
		answer := a.updateNow(fields[1])
		a.rt.logger.Printf("update-now %s: %s", fields[1], answer)
		return answer
	case "update-confirm":
		if len(fields) != 1 {
			return "refused unknown command"
		}
		markUpdateHealth(update.HealthServing)
		go a.settleUpgradeSave()
		return "confirmed"
	case "update-prepare":
		if len(fields) != 4 && len(fields) != 5 {
			return "refused unknown command"
		}
		pid, err := strconv.Atoi(fields[2])
		if err != nil || (fields[3] != "restart" && fields[3] != "quit") {
			return "refused unknown command"
		}
		if len(fields) == 5 && fields[4] != "force" {
			return "refused unknown command"
		}
		answer := a.prepareShellUpdate(fields[1], pid, fields[3] == "quit", len(fields) == 5)
		a.rt.logger.Printf("update-prepare %s: %s", strings.Join(fields[1:], " "), answer)
		return answer
	case "update-abort":
		if len(fields) != 2 {
			return "refused unknown command"
		}
		answer := a.abortShellUpdate(fields[1])
		a.rt.logger.Printf("update-abort %s: %s", fields[1], answer)
		return answer
	case "update-status":
		if len(fields) != 1 {
			return "refused unknown command"
		}
		answer := a.shellUpdateStatus()
		a.rt.logger.Printf("update-status: %s", answer)
		return answer
	case "serve":
		return a.handleServeCommand(fields[1:])
	case "stop-serving":
		return a.handleStopServingCommand(fields[1:])
	case "sync-connect", "sync-disconnect", "sync-pause", "sync-resume", "sync-preview", "sync-setup":
		return a.handleSyncCommand(fields[0], fields[1:])
	}
	return "refused unknown command"
}

// updateNow answers update-now. On macOS the app owns Sparkle and answers it
// itself, so this host only says why it cannot. On Windows and Linux it starts
// the same restart as Restart to update when the self-updater has target
// downloaded. While nothing is downloaded it makes the updater check now and
// answers "checking", so the caller asks again shortly.
func (a *app) updateNow(target string) string {
	if !update.IsStrictVersion(target) {
		return fmt.Sprintf("refused %q is not X.Y.Z", target)
	}
	if target == version {
		return "already " + version
	}
	if !(&update.Release{Version: target}).Newer(version) {
		return fmt.Sprintf("refused v%s is not newer than the running v%s", target, version)
	}
	if a.updateOff != "" {
		return "refused self-update is off: " + a.updateOff
	}
	if updateNowOS == "darwin" {
		return "refused update-now is answered by the HTML Clay app, not by htmlclay-host"
	}
	a.updates.mu.Lock()
	ready, apply, ch, checking := a.updates.ready, a.updates.apply, a.updates.checkNow, a.updates.checking
	a.updates.mu.Unlock()
	if ready == "" {
		if ch == nil {
			return "refused this build does not update itself"
		}
		if !checking {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		return updateNowChecking
	}
	if ready != target {
		return fmt.Sprintf("refused the update HTML Clay has downloaded is v%s, not v%s", ready, target)
	}
	if apply == nil {
		return "refused this build does not update itself"
	}
	go apply()
	return "accepted"
}
