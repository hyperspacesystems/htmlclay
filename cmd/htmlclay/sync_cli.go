package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
const syncUsage = "usage: htmlclay sync status | connect [username] | disconnect | pause | resume | setup <username> <folder> [--existing]\n" +
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
	case args[0] == "setup" && (len(args) == 3 || (len(args) == 4 && args[3] == "--existing")):
		return syncSetup(si, strings.TrimPrefix(args[1], "@"), args[2], len(args) == 4, stdin, stdout, stderr)
	}
	fmt.Fprintln(stderr, syncUsage)
	return serveExitUsage
}

// syncSetup is htmlclay sync setup: it prints what the first sync would do, asks,
// and only on yes sends folder.setUpTeam, confirmed. The folder travels base64
// encoded, so a path with spaces stays one frame field.
func syncSetup(si platform.SingleInstance, username, folder string, existing bool, stdin io.Reader, stdout, stderr io.Writer) int {
	if username == "" || strings.ContainsAny(username, " \t") {
		fmt.Fprintln(stderr, syncUsage)
		return serveExitUsage
	}
	abs, err := filepath.Abs(folder)
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay sync setup: %v\n", err)
		return serveExitUsage
	}
	mode := "empty"
	if existing {
		mode = "existing"
	}
	fields := username + " " + mode + " " + base64.RawURLEncoding.EncodeToString([]byte(abs))
	if code := sendInstanceFrame(si, "sync-preview "+fields, "sync", stdout, stderr); code != serveExitOK {
		return code
	}
	fmt.Fprint(stdout, appcore.TextSyncSetupAsk)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(stdout)
		return serveExitRefused
	}
	return sendInstanceFrame(si, "sync-setup "+fields, "sync", stdout, stderr)
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
	case "sync-preview", "sync-setup":
		row, path, existing, bad := a.syncSetupFields(fields)
		if bad != "" {
			return bad
		}
		if verb == "sync-preview" {
			done, refusal = a.syncSetupPreview(row, path, existing)
		} else {
			refusal = a.trayDo("folder.setUpTeam", map[string]any{"id": row.FolderID, "path": path, "existing": existing, "confirmed": existing})
			done = appcore.TextSyncSetupStarted
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

// syncSetupFields reads a setup frame's fields: username, mode and the base64 path.
func (a *app) syncSetupFields(fields []string) (config.SyncAccountRow, string, bool, string) {
	if len(fields) != 3 || (fields[1] != "empty" && fields[1] != "existing") {
		return config.SyncAccountRow{}, "", false, "refused unknown command"
	}
	raw, err := base64.RawURLEncoding.DecodeString(fields[2])
	if err != nil || !filepath.IsAbs(string(raw)) {
		return config.SyncAccountRow{}, "", false, "refused unknown command"
	}
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	var rows []config.SyncAccountRow
	if err == nil {
		rows = append(rows, record.Accounts...)
	}
	a.rt.syncMu.Unlock()
	for _, row := range rows {
		if strings.EqualFold(row.Username, fields[0]) {
			return row, string(raw), fields[1] == "existing", ""
		}
	}
	return config.SyncAccountRow{}, "", false, "refused " + appcore.RefuseNoSuchFolder + " " + oneLine(fmt.Sprintf(appcore.TextSyncSetupNoAccount, fields[0]))
}

// syncSetupPreview answers the sentence htmlclay sync setup prints before it asks:
// the size of a first download, or what syncing into files already there does.
func (a *app) syncSetupPreview(row config.SyncAccountRow, path string, existing bool) (string, *appcore.Refusal) {
	if a.runningService() == nil {
		return "", &appcore.Refusal{Code: appcore.RefuseServiceNeeded}
	}
	if existing {
		counts, refusal := a.previewExisting(row.AccountID, path)
		if refusal != nil {
			return "", refusal
		}
		if counts.ToUpload == 1 {
			return fmt.Sprintf(appcore.TextSyncSetupExistingOne, counts.ToDownload, counts.Differ), nil
		}
		return fmt.Sprintf(appcore.TextSyncSetupExisting, counts.ToUpload, counts.ToDownload, counts.Differ), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), syncCallTimeout)
	defer cancel()
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Files int    `json:"files"`
		Bytes int64  `json:"bytes"`
	}
	body := struct {
		AccountID int64 `json:"accountId"`
	}{AccountID: row.AccountID}
	if err := a.served.CallService(ctx, "/sync/v1/preview", body, &answer); err != nil || !answer.OK {
		if answer.Error == "offline" {
			return "", &appcore.Refusal{Code: appcore.RefuseSyncOffline}
		}
		return "", &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return fmt.Sprintf(appcore.TextSyncSetupDownload, answer.Files, humanBytes(answer.Bytes), path), nil
}

// humanBytes is a size as a person reads it: 482 KB, 3.1 MB.
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	value, suffix := float64(n)/unit, "KB"
	for _, next := range []string{"MB", "GB", "TB"} {
		if value < unit {
			break
		}
		value, suffix = value/unit, next
	}
	if value < 10 {
		return fmt.Sprintf("%.1f %s", value, suffix)
	}
	return fmt.Sprintf("%.0f %s", value, suffix)
}
