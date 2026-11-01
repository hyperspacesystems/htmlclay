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
		a.reportHelperProgramError("Could not relink the program", err)
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
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".bat", ".cmd":
			return true, nil
		default:
			return false, errors.New("Windows helper programs must be .exe, .bat, or .cmd files; use a .cmd wrapper for scripts")
		}
	}
	if info.Mode()&0111 != 0 {
		return true, nil
	}

	firstLine, err := readProgramFirstLine(path)
	if err != nil {
		return false, fmt.Errorf("cannot read the first line of %s: %w", path, err)
	}
	message := fmt.Sprintf("This file is not executable:\n\n%s\n\nFirst line:\n%q\n\nMake it executable so HTML Clay can run it?", path, firstLine)
	ok, err := confirm("Make Program Executable", message, "Make Executable")
	if err != nil || !ok {
		return false, err
	}
	if err := os.Chmod(path, info.Mode()|0111); err != nil {
		return false, fmt.Errorf("cannot make %s executable: %w", path, err)
	}
	return true, nil
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
