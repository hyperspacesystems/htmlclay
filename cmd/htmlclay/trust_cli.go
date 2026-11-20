package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/codeset"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
)

// `htmlclay trust <file>`, `htmlclay untrust <file>`, `htmlclay
// developer-mode [on|off]` and `htmlclay programs [enable|disable]` write the
// trust store (internal/tier) directly.
// Each record is its own file written by a rename, so these need neither the
// running app nor its lock, and the app reads the answer on its next look.
const (
	trustExitOK     = 0
	trustExitUsage  = 1
	trustExitFailed = 2
)

const trustUsage = `usage: htmlclay trust <file>
   or: htmlclay untrust <file>
   or: htmlclay developer-mode [on|off]
   or: htmlclay programs [enable|disable]

trust           Trust a file like one you made
untrust         Clear your saved choice for a file
developer-mode  Show or set developer mode
programs        Enable or disable your programs folder`

var trustCommands = map[string]bool{"trust": true, "untrust": true, "developer-mode": true, "programs": true}

// trustDispatch runs the trust commands and reports whether it handled argv.
func trustDispatch(argv []string) (int, bool) {
	if len(argv) < 2 || !trustCommands[argv[1]] {
		return 0, false
	}
	dir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "htmlclay %s: cannot resolve the config dir: %v\n", argv[1], err)
		return trustExitFailed, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "htmlclay %s: cannot resolve the home folder: %v\n", argv[1], err)
		return trustExitFailed, true
	}
	return runTrust(filepath.Join(dir, "trust"), home, argv[1:], os.Stdout, os.Stderr), true
}

func runTrust(storeDir, home string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || len(args) > 2 {
		fmt.Fprintln(stderr, trustUsage)
		return trustExitUsage
	}
	store, _, err := tier.Open(storeDir)
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay %s: cannot open the trust record: %v\n", args[0], err)
		return trustExitFailed
	}
	switch args[0] {
	case "developer-mode":
		return switchCmd(args, []string{"on", "off"}, store.SetDeveloper, store.Developer, home, stdout, stderr)
	case "programs":
		return switchCmd(args, []string{"enable", "disable"}, store.SetPrograms, store.Programs, home, stdout, stderr)
	}
	if len(args) != 2 {
		fmt.Fprintln(stderr, trustUsage)
		return trustExitUsage
	}
	if args[0] == "untrust" {
		// Untrusting works on a document that moved, changed or is gone: it
		// takes back the decision the person remembers, not the file.
		path := closedPath(args[1])
		_, code, _, codeErr := trustTarget(args[1])
		if codeErr != nil {
			code = ""
		}
		if err := store.Forget(path, code); err != nil {
			fmt.Fprintf(stderr, "htmlclay untrust: %v\n", err)
			return trustExitFailed
		}
		fmt.Fprintf(stdout, "Cleared your saved choice for %s. HTML Clay will ask before it uses your files.\n", filepath.Base(path))
		return trustExitOK
	}
	path, code, parts, err := trustTarget(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay %s: %v\n", args[0], err)
		return trustExitFailed
	}
	seen, _ := store.Seen(path)
	if err := store.TrustPath(path, platform.Downloaded(path) || seen.Downloaded, tier.HowCommand); err == nil && parts > 0 {
		err = store.TrustCodeViaPath(code, path, tier.HowCommand)
	}
	if err != nil {
		fmt.Fprintf(stderr, "htmlclay trust: %v\n", err)
		return trustExitFailed
	}
	fmt.Fprintf(stdout, "HTML Clay now treats %s like a file you made. Any file with the same code runs without asking.\n", filepath.Base(path))
	return trustExitOK
}

// trustTarget resolves the file the person named and reads its code set.
func trustTarget(arg string) (path, code string, parts int, err error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", "", 0, err
	}
	if path, err = resolveSymlinks(abs); err != nil {
		return "", "", 0, fmt.Errorf("cannot find %s", arg)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", 0, fmt.Errorf("%s is not a file", arg)
	}
	if info.Size() > codeset.MaxDocument {
		return "", "", 0, fmt.Errorf("%s is larger than HTML Clay opens", arg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", 0, err
	}
	set, err := codeset.Of(data)
	if err != nil {
		return "", "", 0, fmt.Errorf("cannot read %s as HTML: %v", arg, err)
	}
	return path, set.Hash, set.Parts, nil
}

// switchCmd runs developer-mode or programs: with on (words[0]) or off
// (words[1]) it sets the switch, and either way it says where things stand.
// Switching on creates ~/htmlclay/programs/ so the person can put programs in.
func switchCmd(args, words []string, set func(bool) error, get func() bool, home string, stdout, stderr io.Writer) int {
	if len(args) == 2 && args[1] != words[0] && args[1] != words[1] {
		fmt.Fprintln(stderr, trustUsage)
		return trustExitUsage
	}
	if len(args) == 2 {
		if err := set(args[1] == words[0]); err != nil {
			fmt.Fprintf(stderr, "htmlclay %s: %v\n", args[0], err)
			return trustExitFailed
		}
		if args[1] == words[0] {
			if err := os.MkdirAll(programsFolder(home), 0o755); err != nil {
				fmt.Fprintf(stderr, "htmlclay %s: cannot create %s: %v\n", args[0], programsFolder(home), err)
				return trustExitFailed
			}
		}
	}
	on := get()
	switch {
	case args[0] == "developer-mode" && on:
		fmt.Fprintf(stdout, "Developer mode is on. Local files you open that were not downloaded count as yours, and programs in %s can run.\n", programsFolder(home))
	case args[0] == "developer-mode":
		fmt.Fprint(stdout, "Developer mode is off.\n")
	case on:
		fmt.Fprintf(stdout, "Programs in %s are on. Documents you trust can run them.\n", programsFolder(home))
	default:
		fmt.Fprintf(stdout, "Programs in %s are off. Run htmlclay programs enable to turn them on.\n", programsFolder(home))
	}
	return trustExitOK
}
