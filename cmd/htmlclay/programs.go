package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/panphora/htmlclay/internal/tray"
)

const maxProgramFirstLine = 4096

// helperProgramRows lists the programs whose file is gone, which are the rows
// the tray's Programs menu shows. Clicking one relinks it through a native
// dialog. Everything else lives on the Programs page.
func (a *app) helperProgramRows() []tray.Row {
	var rows []tray.Row
	for _, program := range a.rt.cfg.HelperProgramList() {
		if helperProgramMissing(program.Path) {
			rows = append(rows, tray.Row{Path: program.ID, Label: "Locate “" + program.Name + "” (file missing)…"})
		}
	}
	return rows
}

// helperProgramsTitle is the tray's Programs item, which counts the programs
// whose file is gone so a broken setup shows without opening the menu.
func (a *app) helperProgramsTitle() string {
	if n := len(a.helperProgramRows()); n > 0 {
		return fmt.Sprintf("Programs (%d missing)", n)
	}
	return "Programs"
}

// relinkHelperProgramFromTray is a click on a missing program's row.
func (a *app) relinkHelperProgramFromTray(id string) []tray.Row {
	a.helperMu.Lock()
	program, ok := a.rt.cfg.LookupHelperProgram(id)
	var outcome string
	var err error
	if ok {
		outcome, err = a.relinkHelperProgram(program, a.systemHelperApprovalDialogs())
	}
	a.helperMu.Unlock()
	if err != nil {
		a.reportHelperProgramError("Could not update the program", err)
	} else if outcome == "relinked" {
		a.rt.logger.Printf("Helper program relinked: id=%s name=%s", program.ID, program.Name)
		a.refreshHelperDispatchers()
	}
	return a.helperProgramRows()
}

func helperProgramMissing(path string) bool {
	info, err := os.Stat(path)
	return err != nil || !info.Mode().IsRegular()
}

func helperProgramExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}

func prepareHelperProgram(path string, confirm func(title, message, affirmative string) (bool, error)) (bool, error) {
	return prepareHelperProgramOn(runtime.GOOS, path, confirm)
}

// The platform is a parameter so the Windows extension rule can be exercised
// from any machine. There is no Windows hardware here, and a branch that can
// only run where nobody can test it ships unverified.
func prepareHelperProgramOn(goos, path string, confirm func(title, message, affirmative string) (bool, error)) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a file", path)
	}
	if goos == "windows" {
		if windowsHelperProgramExtension(path) {
			return true, nil
		}
		return false, errors.New("Programs on Windows must be .exe, .bat or .cmd files. Use a .cmd file to run a script.")
	}
	if info.Mode()&0111 != 0 {
		return true, nil
	}

	firstLine, err := readProgramFirstLine(path)
	if err != nil {
		return false, fmt.Errorf("cannot read the first line of %s: %w", path, err)
	}
	message := fmt.Sprintf("This file is not executable:\n\n%s\n\nFirst line:\n%q\n\nMake it executable so HTML Clay can run it?", path, firstLine)
	ok, err := confirm("Make program executable?", message, "Make executable")
	if err != nil || !ok {
		return false, err
	}
	if err := os.Chmod(path, info.Mode()|0111); err != nil {
		return false, fmt.Errorf("cannot make %s executable: %w", path, err)
	}
	return true, nil
}

// helperProgramNeedsExecutable reports whether path is a regular program file
// that still lacks every executable bit, which is the one case the request
// window asks about as not-executable. A file the platform refuses for another
// reason comes back as an error.
func helperProgramNeedsExecutable(path string) (bool, error) {
	ok, err := prepareHelperProgramOn(runtime.GOOS, path, func(string, string, string) (bool, error) {
		return false, nil
	})
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// makeHelperProgramExecutable runs the chmod prepareHelperProgram asks to do,
// once the person answered allow-anyway.
func makeHelperProgramExecutable(path string) (bool, error) {
	return prepareHelperProgramOn(runtime.GOOS, path, func(string, string, string) (bool, error) {
		return true, nil
	})
}

func windowsHelperProgramExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".bat", ".cmd":
		return true
	}
	return false
}

func readProgramFirstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	line, err := bufio.NewReader(io.LimitReader(f, maxProgramFirstLine+1)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if len(line) > maxProgramFirstLine {
		line = line[:maxProgramFirstLine] + "…"
	}
	return strings.ToValidUTF8(line, "�"), nil
}

func (a *app) reportHelperProgramError(title string, err error) {
	a.rt.logger.Printf("%s: %v", title, err)
	go func() {
		if notifyErr := a.notifyUser(title, err.Error()); notifyErr != nil {
			a.rt.logger.Printf("Could not show the program error: %v", notifyErr)
		}
	}()
}
