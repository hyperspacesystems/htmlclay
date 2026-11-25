// Package sse holds the Server-Sent Events framing shared by the live-sync
// streams and the wire: one frame per event, the cursor frame, the resume point
// a client sends back, and one write with a deadline.
package sse

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// WriteDeadline bounds one write, so an evicted slow subscriber unblocks
	// instead of leaking a goroutine stuck inside Write.
	WriteDeadline = 2 * time.Second
	// KeepaliveInterval is how often a silent stream gets a keepalive, so
	// intermediaries don't close it.
	KeepaliveInterval = 25 * time.Second
)

// Frame builds one SSE frame (an id line and a JSON data line) from v, with
// HTML escaping off. It returns nil when v does not encode.
func Frame(seq int64, v interface{}) []byte {
	var buf bytes.Buffer
	buf.WriteString("id: ")
	buf.WriteString(strconv.FormatInt(seq, 10))
	buf.WriteString("\ndata: ")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil
	}
	// Encode appends one newline; an SSE frame ends with a blank line.
	buf.WriteByte('\n')
	return buf.Bytes()
}

// CursorFrame builds the `event: cursor` frame carrying seq and, when resync is
// set, `"resync":true`.
func CursorFrame(seq int64, resync bool) []byte {
	var buf bytes.Buffer
	buf.WriteString("event: cursor\nid: ")
	buf.WriteString(strconv.FormatInt(seq, 10))
	buf.WriteString("\ndata: {\"seq\":")
	buf.WriteString(strconv.FormatInt(seq, 10))
	if resync {
		buf.WriteString(",\"resync\":true")
	}
	buf.WriteString("}\n\n")
	return buf.Bytes()
}

// ParseLastEventID reads the client's resume point from the Last-Event-ID
// header or the lastEventId query parameter, and answers 0 when it is absent,
// malformed or negative.
func ParseLastEventID(r *http.Request) int64 {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("lastEventId")
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// Write writes one frame under WriteDeadline and flushes it. It answers false
// when the write or the flush fails.
func Write(rc *http.ResponseController, w http.ResponseWriter, msg []byte) bool {
	if err := rc.SetWriteDeadline(time.Now().Add(WriteDeadline)); err != nil {
		return false
	}
	if _, err := w.Write(msg); err != nil {
		return false
	}
	return rc.Flush() == nil
}
