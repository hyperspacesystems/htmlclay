package carried

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const envSentinel = "HTMLCLAY_CARRIED_TEST_SENTINEL"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == WorkerArg {
		if os.Getenv(envSentinel) != "" {
			os.Exit(7)
		}
		os.Exit(RunWorker(os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func runJob(t *testing.T, job Job) (Result, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if job.Worker == "" {
		job.Worker = exe
	}
	if job.Entry == "" {
		job.Entry = "main"
	}
	if job.Timeout == 0 {
		job.Timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	return Run(ctx, job)
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestAProgramRunsMainWithItsInputAndAnswersJSON(t *testing.T) {
	var statuses []string
	res, err := runJob(t, Job{
		Modules: map[string]string{"main": `async function main(input, host) {
			console.log("adding", input.n, {to: 1});
			host.status("almost");
			await new Promise((r) => setTimeout(r, 5));
			return { sum: input.n + 1, same: host === clay };
		}`},
		Input:  json.RawMessage(`{"n": 41}`),
		Status: func(s string) { statuses = append(statuses, s) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(res.Data) != `{"sum":42,"same":true}` {
		t.Fatalf("data = %s, want {\"sum\":42,\"same\":true}", res.Data)
	}
	if res.Log != "adding 41 {\"to\":1}\n" || len(statuses) != 1 || statuses[0] != "almost" {
		t.Fatalf("log %q statuses %q", res.Log, statuses)
	}
	if _, err = runJob(t, Job{Modules: map[string]string{"main": `const helper = 1;`}}); code(err) != CodeProgramInvalid {
		t.Fatalf("a program with no main: %v", err)
	}
}

func TestTheWorkerGetsOnlyCleanEnvAndNoWayOut(t *testing.T) {
	t.Setenv(envSentinel, "the app's own environment")
	res, err := runJob(t, Job{Modules: map[string]string{"main": `async function main() {
		const out = { process: typeof process, fetch: typeof fetch, xhr: typeof XMLHttpRequest, buffer: typeof Buffer };
		for (const name of ["child_process", "fs", "os", "process", "net"]) {
			try { require(name); out[name] = "loaded"; } catch (e) { out[name] = "refused"; }
		}
		return out;
	}`}})
	if err != nil {
		t.Fatalf("run: %v (exit 7 means the worker saw HTML Clay's own environment)", err)
	}
	var out map[string]string
	if err := json.Unmarshal(res.Data, &out); err != nil {
		t.Fatal(err)
	}
	for k, v := range out {
		if v != "undefined" && v != "refused" {
			t.Errorf("%s = %q inside the worker, want nothing", k, v)
		}
	}
}

func TestTheDeadlineKillsTheWorker(t *testing.T) {
	start := time.Now()
	_, err := runJob(t, Job{Modules: map[string]string{"main": `async function main() { while (true) {} }`}, Timeout: time.Second})
	if code(err) != CodeProgramTimeout {
		t.Fatalf("err = %v, want %s", err, CodeProgramTimeout)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the run ended after %v", d)
	}
}

func TestCancelKillsTheWorker(t *testing.T) {
	exe, _ := os.Executable()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, Job{Worker: exe, Entry: "main", Timeout: time.Minute,
			Modules: map[string]string{"main": `async function main() { clay.status("spinning"); while (true) {} }`},
			Status:  func(string) { started <- struct{}{} }})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the program never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel: the worker outlived it")
	}
}

func TestTheMemoryCapStopsHostilePrograms(t *testing.T) {
	for name, src := range map[string]string{
		"one huge string": `async function main() { const s = "x".repeat(2 ** 30); return s.length }`,
		"slow growth":     `async function main() { const keep = []; while (true) keep.push(new Array(100000).fill(Math.random())) }`,
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			_, err := runJob(t, Job{Modules: map[string]string{"main": src}, Memory: 128 << 20})
			if code(err) != CodeProgramMemory {
				t.Fatalf("err = %v, want %s", err, CodeProgramMemory)
			}
			if d := time.Since(start); d > 10*time.Second {
				t.Fatalf("the memory cap took %v, want well under the 20s timeout", d)
			}
		})
	}
}

func TestACrashedWorkerLeavesHTMLClayRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for the crashing worker")
	}
	crash := filepath.Join(t.TempDir(), "crash")
	if err := os.WriteFile(crash, []byte("#!/bin/sh\nread line\nkill -SEGV $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := runJob(t, Job{Worker: crash, Modules: map[string]string{"main": `async function main() {}`}})
	if code(err) != CodeProgramCrashed {
		t.Fatalf("err = %v, want %s", err, CodeProgramCrashed)
	}
	res, err := runJob(t, Job{Modules: map[string]string{"main": `async function main() { return "still here" }`}})
	if err != nil || string(res.Data) != `"still here"` {
		t.Fatalf("the next run: %s, %v", res.Data, err)
	}
}

func TestRequireLoadsOnlyTheDocumentsOwnPrograms(t *testing.T) {
	res, err := runJob(t, Job{Modules: map[string]string{
		"main": `const lib = require("./lib"); async function main() { return lib.add(2, 3) }`,
		"lib":  `exports.add = (a, b) => a + b;`,
	}})
	if err != nil || string(res.Data) != "5" {
		t.Fatalf("data %s, err %v", res.Data, err)
	}
	_, err = runJob(t, Job{Modules: map[string]string{"main": `const fs = require("fs"); async function main() {}`}})
	if code(err) != CodeProgramFailed || !strings.Contains(err.Error(), "no program named fs") {
		t.Fatalf("err = %v, want the require refused", err)
	}
}

// Eight programs of 256 KiB are the most a page carries, and quotes and
// backslashes double when the start message escapes them.
func TestEightFullProgramsReachTheWorker(t *testing.T) {
	full := "/*" + strings.Repeat(`"\`, (256<<10-64)/2) + "*/"
	modules := map[string]string{"main": full + "\nasync function main() { return require(\"./p7\").n }"}
	for i := 1; i < 8; i++ {
		modules[fmt.Sprintf("p%d", i)] = full + fmt.Sprintf("\nmodule.exports = { n: %d }", i)
	}
	res, err := runJob(t, Job{Modules: modules, Input: json.RawMessage(`"` + strings.Repeat(`\"`, 100<<10) + `"`)})
	if err != nil || string(res.Data) != "7" {
		t.Fatalf("data %s, err %v", res.Data, err)
	}
}

func TestImportAndExportAreRefusedWithTheWayThatWorks(t *testing.T) {
	for name, modules := range map[string]map[string]string{
		"entry":  {"main": "import fs from \"fs\";\nexport default async function main() {}"},
		"module": {"main": `const lib = require("./lib"); async function main() {}`, "lib": "export const add = (a, b) => a + b;"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runJob(t, Job{Modules: modules})
			if code(err) != CodeProgramInvalid || !strings.Contains(err.Error(), `require("./name")`) {
				t.Fatalf("err = %v, want program_invalid naming require", err)
			}
		})
	}
}

func TestTheWorkerEndsWhenHTMLClayIsGone(t *testing.T) {
	exe, _ := os.Executable()
	cmd := exec.Command(exe, WorkerArg)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	start, _ := json.Marshal(startMessage{Entry: "main", Modules: map[string]string{"main": `async function main() { clay.status("up"); while (true) {} }`}})
	stdin.Write(append(start, '\n'))
	if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatalf("the worker never started: %v", err)
	}
	stdin.Close()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("the worker kept running after its input closed")
	}
}

func TestAnUncaughtRefusalKeepsItsCode(t *testing.T) {
	_, err := runJob(t, Job{Modules: map[string]string{"main": `async function main() { await clay.fetch("https://example.com") }`}, Calls: Calls{Files: true, Git: true}})
	if code(err) != CodeCallRefused {
		t.Fatalf("err = %v, want %s", err, CodeCallRefused)
	}
	_, err = runJob(t, Job{Modules: map[string]string{"main": `async function main() { return 1n }`}})
	if code(err) != CodeProgramFailed {
		t.Fatalf("a result that is not JSON: %v", err)
	}
}

// A timer callback that throws once main's promise never settles would
// otherwise hang the run until its deadline: nothing turns the exception into
// an answer.
func TestAnExceptionInATimerEndsTheRun(t *testing.T) {
	_, err := runJob(t, Job{Modules: map[string]string{"main": `async function main() {
		setTimeout(() => { throw new Error("boom"); }, 10);
		await new Promise(() => {});
	}`}, Timeout: 5 * time.Second})
	if code(err) != CodeProgramFailed || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want %s with boom", err, CodeProgramFailed)
	}
}

// Go prints an OOM fatal error at the top of a multi-goroutine stack dump
// that can exceed the 16 KiB stderr tail; tail must remember the OOM across
// chunk boundaries and truncation.
func TestStderrTailRemembersOOMPastTruncation(t *testing.T) {
	tr := &tail{max: 16 << 10}
	_, _ = tr.Write([]byte("fatal error: runtime: out of "))
	_, _ = tr.Write([]byte("memory\n"))
	_, _ = tr.Write([]byte(strings.Repeat("goroutine 1 [running]:\n", 2000)))
	if !tr.isOOM() {
		t.Fatal("tail forgot an out-of-memory error once the stack trace pushed it past 16 KiB")
	}
}
