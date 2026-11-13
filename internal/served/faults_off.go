//go:build !managedtrial && !servedfaults

package served

import "net/http"

// takeFault reports whether a fault armed for point names target. Untagged
// builds arm nothing.
func takeFault(point, target string) bool { return false }

// holdTailwind blocks a compile of name while a harness holds it. Untagged
// builds hold nothing.
func holdTailwind(name string) bool { return false }

func finishHeldTailwind(name string) {}

var killSelf = func() {}

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

func (a *controlAPI) faultRoute(*http.Request) http.HandlerFunc { return nil }
