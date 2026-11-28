package mirror

import (
	"bufio"
	"io"
	"net/http"
	"strings"
)

// untyped says whether a Content-Type names no real kind, so the bytes decide.
func untyped(contentType string) bool {
	t, _, _ := strings.Cut(strings.ToLower(contentType), ";")
	switch strings.TrimSpace(t) {
	case "", "application/octet-stream", "binary/octet-stream":
		return true
	}
	return false
}

// sniffType replaces an untyped Content-Type with the one the bytes show, returning a
// reader that still yields them.
func sniffType(contentType string, r io.Reader) (string, io.Reader) {
	if !untyped(contentType) {
		return contentType, r
	}
	br := bufio.NewReaderSize(r, 512)
	head, _ := br.Peek(512)
	if sniffed := http.DetectContentType(head); sniffed != "application/octet-stream" {
		return sniffed, br
	}
	return contentType, br
}
