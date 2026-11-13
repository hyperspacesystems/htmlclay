//go:build managedtrial

package managedtrial

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
)

// uploadPart is the one file part readUploadPart answers with.
type uploadPart struct {
	filename string
	content  []byte
}

// readUploadPart is Local's readUploadPart: one file part named `file`, and
// anything else in the body drained and ignored rather than refused, so a client
// that also sends fields still works. The three answers are Local's: a body that
// is not a multipart form at all, a body multipart cannot read, and a file over
// the limit, which busboy would otherwise truncate and store silently short.
func readUploadPart(body io.Reader, contentType string, limit int64) (*uploadPart, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return nil, &PathError{Status: http.StatusBadRequest, Message: "Expected a multipart form upload."}
	}

	// Go's reader answers io.EOF both for a body that ended at its final
	// delimiter and for one that holds no delimiter at all, and Local answers
	// those two differently: a form with no file part, or a body it cannot read.
	marked := &closingBoundaryReader{reader: body, needle: []byte("--" + params["boundary"] + "--")}
	reader := multipart.NewReader(marked, params["boundary"])
	var part *uploadPart
	for {
		next, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			if !marked.seen {
				return nil, &PathError{Status: http.StatusBadRequest, Message: "Could not read the upload."}
			}
			return part, nil
		}
		if err != nil {
			return nil, &PathError{Status: http.StatusBadRequest, Message: "Could not read the upload."}
		}
		filename, isFile := uploadPartFilename(next)
		if next.FormName() != "file" || part != nil || !isFile {
			_, _ = io.Copy(io.Discard, next)
			continue
		}
		content, err := io.ReadAll(io.LimitReader(next, limit+1))
		if err != nil {
			return nil, &PathError{Status: http.StatusBadRequest, Message: "Could not read the upload."}
		}
		if int64(len(content)) > limit {
			return nil, &PathError{
				Status:  http.StatusRequestEntityTooLarge,
				Message: fmt.Sprintf("Files are limited to %d bytes.", limit),
			}
		}
		part = &uploadPart{filename: filename, content: content}
	}
}

// closingBoundaryReader notes whether the body holds a multipart closing
// delimiter, the one thing Go's reader does not report.
type closingBoundaryReader struct {
	reader io.Reader
	needle []byte
	tail   []byte
	seen   bool
}

func (c *closingBoundaryReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if n > 0 && !c.seen {
		window := append(c.tail, p[:n]...)
		if bytes.Contains(window, c.needle) {
			c.seen = true
			c.tail = nil
		} else if len(window) > len(c.needle) {
			c.tail = window[len(window)-len(c.needle):]
		} else {
			c.tail = window
		}
	}
	return n, err
}

// uploadPartFilename is the filename a part declares, and whether it declares
// one at all: a part without the parameter is a field, never a file.
func uploadPartFilename(part *multipart.Part) (string, bool) {
	disposition, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil || disposition != "form-data" {
		return "", false
	}
	filename, ok := params["filename"]
	if !ok {
		return "", false
	}
	return filename, true
}
