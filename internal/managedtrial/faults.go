//go:build managedtrial

package managedtrial

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The trial's fault points (P9, P12). A harness arms one through
// POST /v1/fault, and the next matching write takes it. Nothing arms a fault
// unless a harness asks.
const (
	faultKillBeforeRename = "kill-before-rename"
	faultKillAfterRename  = "kill-after-rename"
	faultDurableFail      = "durable-fail"
	faultDropReply        = "drop-reply"
	faultTailwindHold     = "tailwind-hold"
)

var faultPoints = map[string]bool{
	faultKillBeforeRename: true,
	faultKillAfterRename:  true,
	faultDurableFail:      true,
	faultDropReply:        true,
	faultTailwindHold:     true,
}

type armedFault struct {
	point     string
	rel       string
	remaining int
}

var faults struct {
	sync.Mutex
	armed []*armedFault
}

// killSelf ends the process the way a crash or a force quit does: no deferred
// call, no flush. A test replaces it.
var killSelf = func() {
	if process, err := os.FindProcess(os.Getpid()); err == nil {
		process.Kill()
	}
	select {}
}

func armFault(point, rel string, count int) int {
	faults.Lock()
	defer faults.Unlock()
	faults.armed = append(faults.armed, &armedFault{point: point, rel: filepath.ToSlash(rel), remaining: count})
	return len(faults.armed)
}

func clearFaults() {
	faults.Lock()
	faults.armed = nil
	faults.Unlock()
	releaseTailwind("")
	finishedTailwind.Lock()
	finishedTailwind.count = nil
	finishedTailwind.Unlock()
}

// tailwindHolds are compiles a tailwind-hold fault stopped, by stylesheet name,
// each waiting for its channel to close.
var tailwindHolds struct {
	sync.Mutex
	waiting map[string][]chan struct{}
}

// holdTailwind blocks a compile of name while a harness holds it, and reports
// whether it did.
func holdTailwind(name string) bool {
	if !takeFault(faultTailwindHold, name) {
		return false
	}
	released := make(chan struct{})
	tailwindHolds.Lock()
	if tailwindHolds.waiting == nil {
		tailwindHolds.waiting = map[string][]chan struct{}{}
	}
	tailwindHolds.waiting[name] = append(tailwindHolds.waiting[name], released)
	tailwindHolds.Unlock()
	<-released
	return true
}

// releaseTailwind lets every held compile of name go on, and answers how many
// were waiting. An empty name releases them all.
func releaseTailwind(name string) int {
	tailwindHolds.Lock()
	defer tailwindHolds.Unlock()
	count := 0
	for held, channels := range tailwindHolds.waiting {
		if name != "" && held != name {
			continue
		}
		for _, channel := range channels {
			close(channel)
			count++
		}
		delete(tailwindHolds.waiting, held)
	}
	return count
}

// heldTailwind answers how many compiles of name are waiting.
func heldTailwind(name string) int {
	tailwindHolds.Lock()
	defer tailwindHolds.Unlock()
	return len(tailwindHolds.waiting[name])
}

// remainingFaults answers how many uses are left on faults armed for rel, so a
// harness can prove the fault it armed was taken.
func remainingFaults(rel string) int {
	faults.Lock()
	defer faults.Unlock()
	slashed := filepath.ToSlash(rel)
	count := 0
	for _, fault := range faults.armed {
		if fault.rel == slashed {
			count += fault.remaining
		}
	}
	return count
}

// finishedTailwind counts, by stylesheet name, compiles a tailwind-hold stopped
// that have since run to their end, whether they installed or not.
var finishedTailwind struct {
	sync.Mutex
	count map[string]int
}

func finishHeldTailwind(name string) {
	finishedTailwind.Lock()
	defer finishedTailwind.Unlock()
	if finishedTailwind.count == nil {
		finishedTailwind.count = map[string]int{}
	}
	finishedTailwind.count[name]++
}

// heldTailwindFinished answers how many held compiles of name have finished.
func heldTailwindFinished(name string) int {
	finishedTailwind.Lock()
	defer finishedTailwind.Unlock()
	return finishedTailwind.count[name]
}

// takeFault reports whether a fault armed for point names target, a file path
// or a logical document name, and uses one of its counts.
func takeFault(point, target string) bool {
	faults.Lock()
	defer faults.Unlock()
	slashed := filepath.ToSlash(target)
	for index, fault := range faults.armed {
		if fault.point != point {
			continue
		}
		if slashed != fault.rel && !strings.HasSuffix(slashed, "/"+fault.rel) {
			continue
		}
		fault.remaining--
		if fault.remaining <= 0 {
			faults.armed = append(faults.armed[:index], faults.armed[index+1:]...)
		}
		return true
	}
	return false
}

// dropControlReply closes the connection with no answer, as a reply lost on the
// wire leaves it: the write happened, and the caller must ask /v1/op before it
// retries.
func dropControlReply(w http.ResponseWriter) {
	if hijacker, ok := w.(http.Hijacker); ok {
		if conn, _, err := hijacker.Hijack(); err == nil {
			conn.Close()
			return
		}
	}
	panic(http.ErrAbortHandler)
}
