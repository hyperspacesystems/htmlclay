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
)

var faultPoints = map[string]bool{
	faultKillBeforeRename: true,
	faultKillAfterRename:  true,
	faultDurableFail:      true,
	faultDropReply:        true,
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
