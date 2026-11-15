package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// `htmlclay sync status|connect|disconnect|pause|resume` asks the running app
// over the single-instance socket, as htmlclay serve does, with the same exit
// codes. connect reads the key from stdin, never from an argument, so it never
// shows in a process list or a shell history. status reads sync.json itself, so
// it answers whether or not the app is running.
const syncUsage = "usage: htmlclay sync status | connect [username] | disconnect | pause | resume\n" +
	"htmlclay sync connect reads the sync key from stdin."

func syncDispatch(argv []string) (int, bool) {
	if len(argv) < 2 || argv[1] != "sync" {
		return 0, false
	}
	dir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "htmlclay sync: cannot resolve the config dir: %v\n", err)
		return serveExitUsage, true
	}
	return runSync(platform.NewSingleInstance(dir), dir, argv[2:], os.Stdin, os.Stdout, os.Stderr), true
}

func runSync(si platform.SingleInstance, configDir string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, syncUsage)
		return serveExitUsage
	}
	if args[0] == "status" && len(args) == 1 {
		return syncStatus(configDir, stdout, stderr)
	}
	if serveOS == "windows" {
		fmt.Fprintln(stderr, appcore.TextSyncCLIWindows)
		return serveExitUsage
	}
	switch {
	case args[0] == "connect" && len(args) <= 1+1:
		if len(args) == 2 && syncKeyShape.MatchString(strings.TrimSpace(args[1])) {
			fmt.Fprintln(stderr, appcore.TextSyncKeyNoArgument)
			return serveExitUsage
		}
		username := "-"
		if len(args) == 2 {
			username = strings.TrimPrefix(args[1], "@")
			if username == "" || strings.ContainsAny(username, " \t") {
				fmt.Fprintln(stderr, syncUsage)
				return serveExitUsage
			}
		}
		key, err := readSyncKey(stdin, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "htmlclay sync connect: reading the key: %v\n", err)
			return serveExitUsage
		}
		if !syncKeyShape.MatchString(key) {
			fmt.Fprintln(stdout, appcore.RefusalMessage(&appcore.Refusal{Code: appcore.RefuseSyncKeyShape}))
			return serveExitRefused
		}
		return sendInstanceFrame(si, "sync-connect "+username+" "+key, "sync", stdout, stderr)
	case len(args) == 1 && (args[0] == "disconnect" || args[0] == "pause" || args[0] == "resume"):
		return sendInstanceFrame(si, "sync-"+args[0], "sync", stdout, stderr)
	case args[0] == "connect":
		fmt.Fprintln(stderr, appcore.TextSyncKeyNoArgument)
		return serveExitUsage
	}
	fmt.Fprintln(stderr, syncUsage)
	return serveExitUsage
}

// readSyncKey reads one line. On a terminal it asks, with echo off for the line.
func readSyncKey(stdin io.Reader, stderr io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprint(stderr, appcore.TextSyncKeyPrompt)
			if setEcho(f, false) == nil {
				defer func() {
					_ = setEcho(f, true)
					fmt.Fprintln(stderr)
				}()
			}
		}
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func setEcho(tty *os.File, on bool) error {
	mode := "-echo"
	if on {
		mode = "echo"
	}
	cmd := exec.Command("stty", mode)
	cmd.Stdin = tty
	return cmd.Run()
}

// syncStatus prints the connected account and each account and team row.
func syncStatus(configDir string, stdout, stderr io.Writer) int {
	record, err := config.LoadSync(configDir)
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay sync status: %v\n", err)
		return serveExitFailed
	}
	if record.Account == nil {
		fmt.Fprintln(stdout, appcore.TextSyncNotConnected)
		return serveExitOK
	}
	fmt.Fprintf(stdout, appcore.TextSyncConnectedAs+" (key ending %s)\n", record.Account.Username, record.Account.KeySuffix)
	if record.Paused {
		fmt.Fprintln(stdout, appcore.TextSyncPausedLine)
	} else {
		fmt.Fprintln(stdout, appcore.TextSyncResumedLine)
	}
	syncing := make(map[int64]bool, len(record.SyncSessions))
	for _, session := range record.SyncSessions {
		if session.Enabled {
			syncing[session.AccountID] = true
		}
	}
	for _, row := range record.Accounts {
		state := "not set up"
		switch {
		case !row.Sync.Enabled && row.Sync.Reason != nil:
			state = "can't sync: " + *row.Sync.Reason
		case syncing[row.AccountID]:
			state = "syncing"
		}
		fmt.Fprintf(stdout, "  %s (%s, %s): %s\n", row.Username, row.Kind, row.Role, state)
	}
	return serveExitOK
}

// handleSyncCommand answers the sync frames. The connect frame carries the key,
// so neither the frame nor any part of it is ever logged; only the answer is.
func (a *app) handleSyncCommand(verb string, fields []string) string {
	var refusal *appcore.Refusal
	var done string
	switch verb {
	case "sync-connect":
		if len(fields) != 2 {
			return "refused unknown command"
		}
		username := fields[0]
		if username == "-" {
			username = ""
		}
		refusal = a.trayDo("sync.connect", map[string]any{"username": username, "key": fields[1]})
		if refusal == nil {
			done = fmt.Sprintf(appcore.TextSyncConnectedAs, a.snapshot().Sync.Username)
		}
	case "sync-disconnect", "sync-pause", "sync-resume":
		if len(fields) != 0 {
			return "refused unknown command"
		}
		switch verb {
		case "sync-disconnect":
			refusal, done = a.trayDo("sync.disconnect", map[string]any{}), appcore.TextSyncDisconnected
		case "sync-pause":
			refusal, done = a.trayDo("sync.setPaused", map[string]any{"paused": true}), appcore.TextSyncPausedLine
		default:
			refusal, done = a.trayDo("sync.setPaused", map[string]any{"paused": false}), appcore.TextSyncResumedLine
		}
	default:
		return "refused unknown command"
	}
	answer := "done " + oneLine(done)
	if refusal != nil {
		answer = "refused " + refusal.Code + " " + oneLine(appcore.RefusalMessage(refusal))
		if refusal.Code == appcore.RefuseFailed {
			answer = "failed " + oneLine(appcore.RefusalMessage(refusal))
		}
	}
	a.rt.logger.Printf("%s: %s", verb, answer)
	return answer
}
