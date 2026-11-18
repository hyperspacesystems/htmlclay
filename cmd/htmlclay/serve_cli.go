package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// `htmlclay serve <folder> [--name <name>]` and `htmlclay stop-serving <name>`
// ask the running app to serve a folder or stop serving one, over the same
// single-instance socket update-now uses. Like update-now they are clients of a
// running app: they never take the single-instance lock and never start it.
const (
	// "served <name> <address>", or "stopped <name>".
	serveExitOK = 0
	// Bad arguments.
	serveExitUsage = 1
	// Nothing answers on the single-instance socket: the app is not running.
	serveExitNotRunning = 2
	// "refused <code> ...": the app will not serve that folder, so nothing happened.
	serveExitRefused = 3
	// The app read the request and did not answer: it predates htmlclay serve.
	serveExitTooOld = 4
	// "failed <reason>": the app tried and could not.
	serveExitFailed = 5
)

const (
	serveUsage       = "usage: htmlclay serve <folder> [--name <name>]"
	stopServingUsage = "usage: htmlclay stop-serving <name>"
)

// serveOS is the platform the command line and the notices are worded for.
// Tests change it.
var serveOS = runtime.GOOS

// serveDispatch runs `htmlclay serve ...` or `htmlclay stop-serving ...` and
// reports whether it handled argv. Like updateNowDispatch it runs before
// flag.Parse and the lock.
func serveDispatch(argv []string) (int, bool) {
	if len(argv) < 2 || (argv[1] != "serve" && argv[1] != "stop-serving") {
		return 0, false
	}
	dir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "htmlclay %s: cannot resolve the config dir: %v\n", argv[1], err)
		return serveExitUsage, true
	}
	return runServe(platform.NewSingleInstance(dir), argv[1:], os.Stdout, os.Stderr), true
}

// runServe sends the request args[0] names and prints the app's answer on stdout.
func runServe(si platform.SingleInstance, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s\n%s\n", serveUsage, stopServingUsage)
		return serveExitUsage
	}
	switch args[0] {
	case "serve":
		return serveCmd(si, args[1:], stdout, stderr)
	case "stop-serving":
		return stopServingCmd(si, args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s\n%s\n", serveUsage, stopServingUsage)
	return serveExitUsage
}

func serveCmd(si platform.SingleInstance, args []string, stdout, stderr io.Writer) int {
	folder, name, ok := parseServeArgs(args)
	if !ok {
		fmt.Fprintln(stderr, serveUsage)
		return serveExitUsage
	}
	if name != "" {
		if err := config.ValidServedName(name); err != nil {
			fmt.Fprintln(stdout, appcore.RefusalMessage(&appcore.Refusal{Code: appcore.RefuseNameInvalid}))
			return serveExitRefused
		}
	}
	abs, err := filepath.Abs(folder)
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay serve: cannot resolve %s: %v\n", folder, err)
		return serveExitUsage
	}
	asked := name
	if asked == "" {
		asked = "-"
	}
	return sendServeFrame(si, "serve "+asked+" "+base64.RawURLEncoding.EncodeToString([]byte(abs)), stdout, stderr)
}

// parseServeArgs reads a folder and an optional --name, which may come in either
// order. The folder is the one argument that is not a flag.
func parseServeArgs(args []string) (folder, name string, ok bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--name":
			if i+1 >= len(args) || name != "" {
				return "", "", false
			}
			i++
			name = args[i]
		case strings.HasPrefix(arg, "--name="):
			if name != "" {
				return "", "", false
			}
			name = strings.TrimPrefix(arg, "--name=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return "", "", false
		case folder == "":
			folder = arg
		default:
			return "", "", false
		}
	}
	return folder, name, folder != ""
}

func stopServingCmd(si platform.SingleInstance, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(stderr, stopServingUsage)
		return serveExitUsage
	}
	name := args[0]
	if strings.ContainsAny(name, " \t") {
		fmt.Fprintln(stdout, fmt.Sprintf(appcore.TextNotServed, name))
		return serveExitRefused
	}
	return sendServeFrame(si, "stop-serving "+name, stdout, stderr)
}

// sendServeFrame sends one frame and prints the app's answer as people read it.
func sendServeFrame(si platform.SingleInstance, frame string, stdout, stderr io.Writer) int {
	return sendInstanceFrame(si, frame, "serve", stdout, stderr)
}

// sendInstanceFrame sends one frame for the htmlclay <command> command line. The
// error it prints names only the frame's first word, which never carries a key.
func sendInstanceFrame(si platform.SingleInstance, frame, command string, stdout, stderr io.Writer) int {
	answer, err := si.SendCommand(frame)
	// "starting": the app holds the lock but runs no command yet, so this one ran
	// nothing and is safe to send again. Someone who opens HTML Clay and types a
	// command a moment later waits for it instead of reading "starting".
	for deadline := time.Now().Add(instanceStartingWait); err == nil && answer == platform.CommandStarting && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
		answer, err = si.SendCommand(frame)
	}
	if err == nil && answer == platform.CommandStarting {
		fmt.Fprintf(stderr, "htmlclay %s: HTML Clay is still starting; try again in a moment\n", command)
		return serveExitNotRunning
	}
	if errors.Is(err, platform.ErrNoAnswer) {
		fmt.Fprintf(stdout, "the running HTML Clay predates htmlclay %s\n", command)
		return serveExitTooOld
	}
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay %s: HTML Clay is not running: %v\n", strings.Fields(frame)[0], err)
		return serveExitNotRunning
	}
	return servePrintAnswer(answer, stdout, stderr)
}

// instanceStartingWait is how long a command keeps resending while the app
// answers "starting". Tests shorten it.
var instanceStartingWait = 10 * time.Second

// servePrintAnswer turns one answer line into the sentence a person reads and the
// exit code that goes with it.
func servePrintAnswer(answer string, stdout, stderr io.Writer) int {
	parts := strings.SplitN(answer, " ", 3)
	switch {
	case parts[0] == "served" && len(parts) == 3:
		fmt.Fprintln(stdout, fmt.Sprintf(appcore.TextNowServed, parts[1], parts[2]))
		return serveExitOK
	case parts[0] == "done" && len(parts) >= 2:
		fmt.Fprintln(stdout, strings.TrimPrefix(answer, "done "))
		return serveExitOK
	case parts[0] == "stopped" && len(parts) == 2:
		fmt.Fprintln(stdout, fmt.Sprintf(appcore.TextStopped, parts[1]))
		return serveExitOK
	case parts[0] == "refused":
		message := answer
		if len(parts) == 3 {
			message = parts[2]
		} else if len(parts) == 2 {
			message = parts[1]
		}
		fmt.Fprintln(stdout, message)
		return serveExitRefused
	case parts[0] == "failed":
		message := answer
		if len(parts) >= 2 {
			message = strings.TrimSpace(strings.TrimPrefix(answer, "failed "))
		}
		fmt.Fprintln(stderr, message)
		return serveExitFailed
	}
	fmt.Fprintln(stdout, answer)
	return serveExitFailed
}

// handleServeCommand answers `serve <name or -> <base64url path>`. The name and
// the path travel as two fields with no spaces, which the encoding guarantees.
func (a *app) handleServeCommand(fields []string) string {
	if len(fields) != 2 {
		return "refused unknown command"
	}
	name := fields[0]
	if name == "-" {
		name = ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(fields[1])
	if err != nil {
		return "refused bad path"
	}
	if refusal := a.trayDo("folder.serve", map[string]any{"path": string(raw), "name": name}); refusal != nil {
		answer := "refused " + refusal.Code + " " + oneLine(appcore.RefusalMessage(refusal))
		if refusal.Code == appcore.RefuseFailed {
			answer = "failed " + oneLine(appcore.RefusalMessage(refusal))
		}
		a.rt.logger.Printf("serve %s: %s", string(raw), answer)
		return answer
	}
	resolved, rErr := filepath.EvalSymlinks(string(raw))
	if rErr != nil {
		resolved = string(raw)
	}
	entry, ok := a.rt.cfg.ServedFolderForPath(resolved)
	if !ok {
		answer := "failed the folder was served but is no longer listed"
		a.rt.logger.Printf("serve %s: %s", string(raw), answer)
		return answer
	}
	answer := "served " + entry.Name + " " + a.servedAddress(entry) + "/"
	a.rt.logger.Printf("serve %s: %s", string(raw), answer)
	return answer
}

// handleStopServingCommand answers `stop-serving <name>`.
func (a *app) handleStopServingCommand(fields []string) string {
	if len(fields) != 1 {
		return "refused unknown command"
	}
	name := fields[0]
	err := errNoServedFolder
	if f, ok := a.rt.cfg.ServedFolderByName(name); ok && f.Serving {
		err = nil
		if refusal := a.trayDo("folder.stopServing", map[string]any{"id": f.ID}); refusal != nil {
			err = errors.New(appcore.RefusalMessage(refusal))
		}
	}
	switch {
	case errors.Is(err, errNoServedFolder):
		answer := "refused not-served " + oneLine(fmt.Sprintf(appcore.TextNotServed, name))
		a.rt.logger.Printf("stop-serving %s: %s", name, answer)
		return answer
	case err != nil:
		answer := "failed " + oneLine(err.Error())
		a.rt.logger.Printf("stop-serving %s: %s", name, answer)
		return answer
	}
	answer := "stopped " + name
	a.rt.logger.Printf("stop-serving %s: %s", name, answer)
	return answer
}

// oneLine keeps an answer to the one line the socket carries.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
