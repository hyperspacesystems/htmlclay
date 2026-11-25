package runner

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// blockedEnv are names a caller's extra entries may never set: each one changes
// what a program loads or where git looks.
var blockedEnv = []string{
	"NODE_OPTIONS", "NODE_PATH", "LD_PRELOAD", "LD_LIBRARY_PATH", "SSH_AUTH_SOCK",
	"BASH_ENV", "ENV", "ZDOTDIR", "PYTHONPATH", "PYTHONSTARTUP", "PERL5OPT", "RUBYOPT",
	"GIT_DIR", "GIT_WORK_TREE", "GIT_EXEC_PATH", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_ASKPASS",
	"GIT_EXTERNAL_DIFF", "GIT_PAGER", "GIT_EDITOR",
}

var blockedEnvPrefixes = []string{"DYLD_", "GIT_CONFIG", "GIT_TRACE", "LD_"}

// windowsSystemEnv are the names a Windows program needs to start at all
// (SystemRoot for the network stack, ComSpec and PATHEXT to find programs).
// None of them carries a secret.
var windowsSystemEnv = []string{
	"SystemRoot", "windir", "ComSpec", "PATHEXT", "SystemDrive", "ProgramFiles", "ProgramFiles(x86)",
	"ProgramW6432", "ProgramData", "CommonProgramFiles", "CommonProgramFiles(x86)", "CommonProgramW6432",
	"NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE", "OS", "USERNAME", "COMPUTERNAME",
}

// Windows reads environment names in any case, so the check does too.
func blocked(name string) bool {
	name = strings.ToUpper(name)
	for _, b := range blockedEnv {
		if name == b {
			return true
		}
	}
	for _, p := range blockedEnvPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// CleanEnv is the whole environment for a child. Nothing comes from HTML
// Clay's own environment except, on Windows, the system names a program needs
// to start. HOME (USERPROFILE on Windows) is the scratch folder, so an engine
// keeps its own files there and never reads the person's own settings or
// hooks. extra entries ("NAME=value") come last and win, except the names in
// blockedEnv, which are dropped. The GIT_CONFIG_* pairs turn off fsmonitor,
// hooks and signature checks for git calls the child makes itself.
func CleanEnv(scratch, path string, extra ...string) []string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	env := []string{
		"HOME=" + scratch,
		"USER=" + name,
		"LOGNAME=" + name,
		"PATH=" + path,
		"TMPDIR=" + scratch,
		"LANG=en_US.UTF-8",
		"XDG_CONFIG_HOME=" + filepath.Join(scratch, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(scratch, "cache"),
		"XDG_DATA_HOME=" + filepath.Join(scratch, "data"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=core.fsmonitor",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=core.hooksPath",
		"GIT_CONFIG_VALUE_1=/dev/null",
		"GIT_CONFIG_KEY_2=log.showSignature",
		"GIT_CONFIG_VALUE_2=false",
		"GIT_PAGER=cat",
		"PAGER=cat",
	}
	env = append(env, systemEnv(runtime.GOOS, os.Environ(), scratch)...)
	for _, kv := range extra {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || k == "" || blocked(k) {
			continue
		}
		env = setEnv(env, kv)
	}
	return env
}

func systemEnv(goos string, environ []string, scratch string) []string {
	if goos != "windows" {
		return nil
	}
	var env []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		for _, name := range windowsSystemEnv {
			if strings.EqualFold(k, name) {
				env = append(env, kv)
				break
			}
		}
	}
	return append(env,
		"USERPROFILE="+scratch,
		"APPDATA="+filepath.Join(scratch, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(scratch, "AppData", "Local"),
		"TEMP="+scratch,
		"TMP="+scratch,
	)
}

func setEnv(env []string, kv string) []string {
	k, _, _ := strings.Cut(kv, "=")
	for i, e := range env {
		if ek, _, _ := strings.Cut(e, "="); ek == k || runtime.GOOS == "windows" && strings.EqualFold(ek, k) {
			env[i] = kv
			return env
		}
	}
	return append(env, kv)
}
