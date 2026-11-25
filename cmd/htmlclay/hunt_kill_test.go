//go:build managedtrial

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

const (
	huntKillChildMode = "HUNT_KILL_CHILD"
	huntKillDocName   = "index.html"
	huntKillFolder    = "notes"
)

// The three documents a kill case moves between: what was there, what the save
// that dies was writing, and what the save after the restart writes.
var (
	huntKillOld  = []byte(servedTestDocument)
	huntKillNew  = []byte(strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1))
	huntKillNext = []byte(strings.Replace(servedTestDocument, "<p>first</p>", "<p>third</p>", 1))
)

// huntKillFault posts one body to the app's control listener's fault route, the
// way the settle hunt does, and answers the numbers the route reports.
func huntKillFault(t *testing.T, a *app, body map[string]string) map[string]int {
	t.Helper()
	a.service.mu.Lock()
	controlURL, secret := a.service.controlURL, a.service.controlSecret
	a.service.mu.Unlock()
	if controlURL == "" {
		t.Fatalf("the app has no control listener, so %v reaches no fault route", body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, controlURL+"/v1/fault", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /v1/fault %v: %v", body, err)
	}
	defer response.Body.Close()
	answerBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	answer := map[string]int{}
	if response.StatusCode != http.StatusOK || json.Unmarshal(answerBody, &answer) != nil {
		t.Fatalf("POST /v1/fault %v = %d %s", body, response.StatusCode, answerBody)
	}
	return answer
}

// huntKillServe serves the folder the app was restarted over. A config that
// still says the folder is being served mounts it at startup, so only a folder
// the crash left unserved is served here.
func huntKillServe(t *testing.T, a *app, dir string) {
	t.Helper()
	if entry, ok := a.rt.cfg.ServedFolderForPath(dir); ok && entry.Serving {
		fmt.Printf("HUNT-CHILD serving=startup name=%s\n", entry.Name)
		return
	}
	serveThroughDo(t, a, "hunt-kill-serve", dir, huntKillFolder)
	fmt.Printf("HUNT-CHILD serving=fresh name=%s\n", huntKillFolder)
}

// huntKillServicePids answers the child's own children, which are the service
// process it started, as a comma-separated list.
func huntKillServicePids() string {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(out)), ",")
}

// huntKillRead answers the bytes in the file the parent named.
func huntKillRead(t *testing.T, env string) []byte {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Fatalf("%s is not set, so the child has no bytes to save", env)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// huntKillSave posts one save of the document and answers the status.
func huntKillSave(t *testing.T, a *app, body []byte) (int, string) {
	t.Helper()
	host := servedHostName(huntKillFolder, servedTestPort)
	rec := servedRequest(t, a, http.MethodPost, host, "/_/save", string(body), map[string]string{
		"Document-URL": "http://" + host + "/" + huntKillDocName,
		"Content-Type": "text/html",
	})
	return rec.Code, rec.Body.String()
}

// huntKillSaveable saves the next bytes, retrying only while the refusal is the
// retryable 503 unconfirmed, for up to ten seconds.
func huntKillSaveable(t *testing.T, a *app, body []byte) (int, string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	attempts := 0
	for {
		attempts++
		status, answer := huntKillSave(t, a, body)
		retryable := status == http.StatusServiceUnavailable && strings.Contains(answer, "unconfirmed")
		if !retryable {
			if attempts > 1 {
				fmt.Printf("HUNT-CHILD next retried=%d\n", attempts)
			}
			return status, answer
		}
		if time.Now().After(deadline) {
			fmt.Printf("HUNT-CHILD next retried=%d\n", attempts)
			return status, answer
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestHuntKillChild is the app process a kill case runs: it saves until a real
// SIGKILL lands, or it is the restart that makes the next save.
func TestHuntKillChild(t *testing.T) {
	mode := os.Getenv(huntKillChildMode)
	if mode == "" {
		t.Skip("not the helper process")
	}
	home, dir, cfgBase := os.Getenv("HUNT_HOME"), os.Getenv("HUNT_DIR"), os.Getenv("HUNT_CFG")
	if home == "" || dir == "" || cfgBase == "" {
		t.Fatalf("HUNT_HOME=%q HUNT_DIR=%q HUNT_CFG=%q, want all three set by the parent", home, dir, cfgBase)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	serviceRepoTarget(t)

	a := newServiceTestApp(t, home, cfgBase)
	huntKillServe(t, a, dir)
	waitForServiceState(t, a, appcore.ServiceRunning, 30*time.Second)
	fmt.Printf("HUNT-CHILD service pids=%s\n", huntKillServicePids())

	switch mode {
	case "save":
		if point := os.Getenv("HUNT_POINT"); point != "" && point != "none" {
			if armed := huntKillFault(t, a, map[string]string{"point": point, "rel": huntKillDocName})["armed"]; armed != 1 {
				t.Fatalf("arming %s for %s answered armed %d, want 1", point, huntKillDocName, armed)
			}
			fmt.Printf("HUNT-CHILD armed=%s\n", point)
		}
		status, answer := huntKillSave(t, a, huntKillRead(t, "HUNT_NEW"))
		fmt.Printf("HUNT-CHILD saved status=%d\n", status)
		if status != http.StatusOK {
			t.Fatalf("POST /_/save = %d %s, want 200", status, answer)
		}
	case "check":
		status, answer := huntKillSaveable(t, a, huntKillRead(t, "HUNT_NEXT"))
		fmt.Printf("HUNT-CHILD next status=%d\n", status)
		if status != http.StatusOK {
			t.Fatalf("the next POST /_/save = %d %s, want 200", status, answer)
		}
	default:
		t.Fatalf("%s=%q, want save or check", huntKillChildMode, mode)
	}
}

// huntKillRunChild runs one child app to its end and answers its output, how it
// ended, and whether the deadline ended it instead.
func huntKillRunChild(t *testing.T, within time.Duration, env map[string]string) (string, error, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHuntKillChild$", "-test.v")
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = os.Environ()
	for name, value := range env {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err, ctx.Err() != nil
}

// huntKillSignal answers the process state a signal death leaves, and whether
// the child died from a signal at all.
func huntKillSignal(err error) (string, bool) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return "", false
	}
	state := exitErr.ProcessState
	return state.String(), state.ExitCode() == -1
}

// huntKillStatus answers the number a HUNT-CHILD line reported, or "" when the
// child never reached that line.
func huntKillStatus(output, label string) string {
	marker := "HUNT-CHILD " + label + " status="
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, marker))
	}
	return ""
}

// huntKillPids answers the service pids the child reported.
func huntKillPids(output string) []int {
	marker := "HUNT-CHILD service pids="
	var pids []int
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		for _, field := range strings.Split(strings.TrimPrefix(line, marker), ",") {
			if pid, err := strconv.Atoi(strings.TrimSpace(field)); err == nil && pid > 0 {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

// huntKillAlive reports whether pid is still a process.
func huntKillAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// huntKillParent answers pid's parent pid, or 0 when it cannot be read.
func huntKillParent(pid int) int {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	parent, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return parent
}

// huntKillWaitService waits for the service process the killed child started to
// go, and kills it only when it is still there after the deadline.
func huntKillWaitService(t *testing.T, pids []int, within time.Duration) {
	t.Helper()
	if len(pids) == 0 {
		t.Logf("HUNT the killed child reported no service pid, so there was no service process to wait for")
		return
	}
	deadline := time.Now().Add(within)
	for _, pid := range pids {
		orphaned := false
		for huntKillAlive(pid) {
			if !orphaned {
				if parent := huntKillParent(pid); parent == 1 {
					orphaned = true
					t.Logf("HUNT service pid=%d is orphaned, re-parented to %d", pid, parent)
				}
			}
			if time.Now().After(deadline) {
				t.Logf("HUNT service pid=%d was still running %v after the kill, so it was killed here", pid, within)
				if process, err := os.FindProcess(pid); err == nil {
					process.Kill()
				}
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !huntKillAlive(pid) {
			t.Logf("HUNT service pid=%d is gone (orphaned=%v)", pid, orphaned)
		}
	}
}

// huntKillVersions answers every file under <dir>/.hyperclay/versions/<name>/,
// which is where a save keeps the bytes it replaces.
func huntKillVersions(t *testing.T, dir, name string) []string {
	t.Helper()
	root := filepath.Join(dir, ".hyperclay", "versions", name)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// huntKillHolding answers every file under root whose bytes equal want.
func huntKillHolding(t *testing.T, root string, want []byte) []string {
	t.Helper()
	var holding []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(data, want) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		holding = append(holding, filepath.ToSlash(rel))
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return holding
}

// huntKillBeside answers every name in dir other than the document.
func huntKillBeside(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() == huntKillDocName {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// huntKillLeftovers separates the atomic temp files H1-a already knows about
// from the other things a killed save left beside the document.
func huntKillLeftovers(t *testing.T, names []string) string {
	t.Helper()
	var known, other []string
	for _, name := range names {
		if strings.HasPrefix(name, "."+huntKillDocName+".") && strings.HasSuffix(name, ".tmp") {
			known = append(known, name)
			continue
		}
		other = append(other, name)
	}
	if len(known) > 0 {
		t.Logf("HUNT H1-a: the killed save left atomic temp file(s) beside the document: %v", known)
	}
	return strings.Join(names, ",")
}

// TestHuntKill kills the app outright in the middle of a save and asks what a
// person finds after it starts again on the same folder and config: the old or
// the new bytes, the replaced bytes in a version, a next save that works, and a
// document that is not left barred.
func TestHuntKill(t *testing.T) {
	serviceRepoTarget(t)
	ran := 0
	for _, point := range []string{"none", "kill-before-rename", "kill-after-rename"} {
		point := point
		t.Run(point, func(t *testing.T) {
			ran++
			home := evalTempHome(t)
			dir := filepath.Join(home, huntKillFolder)
			cfgBase := t.TempDir()
			writeTestFile(t, filepath.Join(dir, huntKillDocName), string(huntKillOld))
			newPath := filepath.Join(t.TempDir(), "new.html")
			writeTestFile(t, newPath, string(huntKillNew))
			nextPath := filepath.Join(t.TempDir(), "next.html")
			writeTestFile(t, nextPath, string(huntKillNext))

			output, err, timedOut := huntKillRunChild(t, 60*time.Second, map[string]string{
				huntKillChildMode: "save",
				"HUNT_HOME":       home,
				"HUNT_DIR":        dir,
				"HUNT_CFG":        cfgBase,
				"HUNT_POINT":      point,
				"HUNT_NEW":        newPath,
			})
			if timedOut {
				t.Fatalf("the save child did not finish within 60s at %s: %s", point, output)
			}
			died := "exit"
			if point == "none" {
				if err != nil {
					t.Fatalf("the save child failed without a kill point: %v\n%s", err, output)
				}
				if status := huntKillStatus(output, "saved"); status != "200" {
					t.Fatalf("the save child answered HUNT-CHILD saved status=%q, want 200\n%s", status, output)
				}
			} else {
				if err == nil {
					t.Fatalf("the save child finished at %s without dying, so no kill landed\n%s", point, output)
				}
				state, signaled := huntKillSignal(err)
				if !signaled {
					t.Fatalf("the save child ended with %v (%s) at %s, want a signal death\n%s", err, state, point, output)
				}
				died = state
			}

			huntKillWaitService(t, huntKillPids(output), 15*time.Second)

			onDisk, readErr := os.ReadFile(filepath.Join(dir, huntKillDocName))
			if readErr != nil {
				t.Fatalf("read %s after the kill: %v\n%s", huntKillDocName, readErr, output)
			}
			landed := "other"
			if bytes.Equal(onDisk, huntKillOld) {
				landed = "old"
			}
			if bytes.Equal(onDisk, huntKillNew) {
				landed = "new"
			}
			if landed == "other" {
				t.Errorf("(a) %s holds neither the old nor the new bytes: %q", huntKillDocName, onDisk)
			}

			versions := huntKillVersions(t, dir, "index")
			versionRoot := filepath.Join(dir, ".hyperclay", "versions")
			oldHolders := huntKillHolding(t, versionRoot, huntKillOld)
			newHolders := huntKillHolding(t, versionRoot, huntKillNew)
			if landed == "new" {
				if held := oldHolders; len(held) == 0 {
					t.Errorf("(b) %s holds the new bytes but no file under .hyperclay/versions holds the old bytes; versions are %v",
						huntKillDocName, versions)
				}
			}

			restartOutput, restartErr, restartTimedOut := huntKillRunChild(t, 60*time.Second, map[string]string{
				huntKillChildMode: "check",
				"HUNT_HOME":       home,
				"HUNT_DIR":        dir,
				"HUNT_CFG":        cfgBase,
				"HUNT_NEXT":       nextPath,
			})
			if restartTimedOut {
				t.Fatalf("the restart child did not finish within 60s at %s:\n%s", point, restartOutput)
			}
			next := huntKillStatus(restartOutput, "next")
			if restartErr != nil {
				t.Fatalf("(c) the restart child failed at %s: %v\n%s", point, restartErr, restartOutput)
			}
			if next != "200" {
				t.Errorf("(c) the next save after the restart answered %q, want 200\n%s", next, restartOutput)
			}
			after, afterErr := os.ReadFile(filepath.Join(dir, huntKillDocName))
			if afterErr != nil {
				t.Fatalf("(c) read %s after the next save: %v", huntKillDocName, afterErr)
			}
			if !bytes.Equal(after, huntKillNext) {
				t.Errorf("(c) %s after the next save = %q, want the next bytes %q", huntKillDocName, after, huntKillNext)
			}

			leftovers := huntKillLeftovers(t, huntKillBeside(t, dir))
			t.Logf("HUNT kill point=%s landed=%s versions=%d leftovers=%s next=%s",
				point, landed, len(versions), leftovers, next)
			t.Logf("HUNT kill point=%s died=%s versions=%v old-holders=%v new-holders=%v", point, died, versions, oldHolders, newHolders)
		})
	}
	if ran < 3 {
		t.Fatalf("only %d of the three kill cases ran", ran)
	}
}
