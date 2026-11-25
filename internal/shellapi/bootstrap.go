package shellapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Bootstrap is the one line the Mac app writes to the host's stdin when it starts
// it with --shell-stdin. The secret never travels as an argument, an environment
// variable or a file, so no other process can read it.
type Bootstrap struct {
	V      int    `json:"v"`
	Secret string `json:"secret"`
}

// maxBootstrapLine bounds the first line, so a parent that never sends a newline
// cannot make the host buffer without end.
const maxBootstrapLine = 4096

// ReadBootstrap reads and checks the first line of in. It returns the reader
// positioned after that line: the caller keeps reading it, and end of file means
// the Mac app is gone.
func ReadBootstrap(in io.Reader) (Bootstrap, *bufio.Reader, error) {
	r := bufio.NewReaderSize(in, maxBootstrapLine)
	line, err := r.ReadSlice('\n')
	if err != nil {
		return Bootstrap{}, r, fmt.Errorf("%w: %v", ErrBadBootstrap, err)
	}
	var b Bootstrap
	dec := json.NewDecoder(strings.NewReader(string(line)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil || b.V != 1 || !isSecret(b.Secret) {
		return Bootstrap{}, r, ErrBadBootstrap
	}
	return b, r, nil
}

func isSecret(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Ready is the one line the host prints on stdout once the listener is bound.
type Ready struct {
	V       int    `json:"v"`
	Shell   string `json:"shell"`
	Port    int    `json:"port"`
	Epoch   string `json:"epoch"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

// WriteReady prints READY {json} and a newline.
func WriteReady(out io.Writer, r Ready) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "READY %s\n", raw)
	return err
}

// Failure is the one line the host prints instead of READY when it cannot start.
// The Mac app shows the designed alert for Code and does not restart the host.
type Failure struct {
	V       int    `json:"v"`
	Code    string `json:"code"`
	Port    int    `json:"port,omitempty"`
	Message string `json:"message,omitempty"`
}

// The failure codes. A new code needs a matching alert in the Mac app.
const (
	FailBadBootstrap   = "bad-bootstrap"
	FailAlreadyRunning = "already-running"
	FailPortTaken      = "port-taken"
	FailFatal          = "fatal"
)

// WriteFailed prints FAILED {json} and a newline.
func WriteFailed(out io.Writer, f Failure) error {
	f.V = 1
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "FAILED %s\n", raw)
	return err
}
