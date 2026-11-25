package aiedit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// errScriptOnly reports a Windows .cmd or .bat shim (such as an npm install).
// AI editing refuses to start those because Windows runs them through cmd.exe,
// which splits arguments at newlines and can drop lockdown flags or execute
// page text as commands.
var errScriptOnly = errors.New("script-only install on windows")

// lookPath finds a program the way the child would: against the PATH inside its
// own environment, not this process's.
func lookPath(name string, env []string) (string, error) {
	return lookPathOS(runtime.GOOS, name, env)
}

func lookPathOS(goos, name string, env []string) (string, error) {
	if name == "" {
		return "", exec.ErrNotFound
	}
	if filepath.IsAbs(name) || strings.ContainsRune(name, filepath.Separator) || strings.Contains(name, "/") {
		if goos == "windows" && isWindowsScript(name) {
			return "", errScriptOnly
		}
		if isExecutableOS(goos, name) {
			return name, nil
		}
		return "", exec.ErrNotFound
	}
	extensions := []string{""}
	if goos == "windows" {
		extensions = windowsBinaryExtensions(env)
	}
	sawScript := false
	for _, dir := range filepath.SplitList(envValue(env, "PATH")) {
		if dir == "" {
			continue
		}
		for _, extension := range extensions {
			candidate := filepath.Join(dir, name+extension)
			if isExecutableOS(goos, candidate) {
				if goos == "windows" && isWindowsScript(candidate) {
					sawScript = true
					continue
				}
				return candidate, nil
			}
		}
		if goos == "windows" && !sawScript {
			for _, scriptExt := range []string{".cmd", ".bat"} {
				if isExecutableOS(goos, filepath.Join(dir, name+scriptExt)) {
					sawScript = true
					break
				}
			}
		}
	}
	if sawScript {
		return "", errScriptOnly
	}
	return "", exec.ErrNotFound
}

func isWindowsScript(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
		return true
	}
	return false
}

// windowsBinaryExtensions returns only native Windows executable extensions
// (.EXE, .COM) so cmd.exe batch wrappers (.CMD, .BAT) are never launched.
func windowsBinaryExtensions(env []string) []string {
	raw := envValue(env, "PATHEXT")
	if raw == "" {
		return []string{".EXE", ".COM"}
	}
	extensions := make([]string, 0, 2)
	for _, ext := range strings.Split(raw, ";") {
		switch strings.ToUpper(strings.TrimSpace(ext)) {
		case ".EXE", ".COM":
			extensions = append(extensions, ext)
		}
	}
	if len(extensions) == 0 {
		return []string{".EXE", ".COM"}
	}
	return extensions
}

func isExecutable(path string) bool {
	return isExecutableOS(runtime.GOOS, path)
}

func isExecutableOS(goos, path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode().Perm()&0111 != 0
}

func envValue(env []string, key string) string {
	for _, entry := range env {
		name, value, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}
