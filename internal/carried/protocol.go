package carried

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// The worker and the app speak one JSON object per line. The app writes a
// start message and then a reply for each call; the worker writes calls,
// console lines, status and one done or error message. Every line the worker writes
// is read as hostile: it is bounded, typed and checked before anything acts
// on it.

const (
	// WorkerArg, as the first argument, starts the HTML Clay binary as a
	// program worker.
	WorkerArg = "--carried-worker"

	maxWorkerLine = 8 << 20
	maxReplyLine  = 16 << 20
	maxStartLine  = 16 << 20
)

type startMessage struct {
	Entry   string            `json:"entry"`
	Modules map[string]string `json:"modules"`
	Input   json.RawMessage   `json:"input,omitempty"`
	Memory  uint64            `json:"memory"`
}

type workerMessage struct {
	T       string          `json:"t"`
	ID      int64           `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	Text    string          `json:"text,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
}

type replyMessage struct {
	T     string          `json:"t"`
	ID    int64           `json:"id"`
	Value json.RawMessage `json:"value,omitempty"`
	Error *Error          `json:"error,omitempty"`
}

var errLineTooLong = errors.New("carried: a line is longer than its limit")

// readLine reads one line of at most max bytes, without its newline.
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		if len(line)+len(part) > max+1 {
			return nil, errLineTooLong
		}
		line = append(line, part...)
		if err == nil {
			return bytes.TrimSuffix(line, []byte("\n")), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			if err == io.EOF && len(line) > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
	}
}

// lineWriter writes one JSON value per line, from any goroutine.
type lineWriter struct {
	mu  sync.Mutex
	w   io.Writer
	err error
}

func (l *lineWriter) send(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	_, l.err = l.w.Write(buf.Bytes())
	return l.err
}
