//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

const uploadGoldenBoundary = "----htmlclay-upload-golden"

func TestUploadGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and the handler reads that checkout's bundled runtime release")
	}
	caseDir := filepath.Join("testdata", "upload-golden")
	runGoldenCasesWith(t, caseDir, localSource, goldenOptions{ownStatCases: uploadStampedCases(t, caseDir)})
}

// uploadStampedCases names the two answers that serve the file an upload wrote:
// the name comes from the recording, and the folder from the document it was
// uploaded to, so a re-recorded golden needs no edit here.
func uploadStampedCases(t *testing.T, caseDir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(caseDir, "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden serveGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var upload struct {
		Uploads []struct {
			Name string `json:"name"`
		} `json:"uploads"`
	}
	found := false
	for _, answer := range golden.Cases {
		if answer.Name != "upload-png-apps" {
			continue
		}
		if err := json.Unmarshal(serveDecodeBody(t, answer.Body), &upload); err != nil {
			t.Fatal(err)
		}
		found = len(upload.Uploads) == 1
	}
	if !found {
		t.Fatal("the golden records no upload from apps/x.html")
	}
	stored := "uploads/" + assetsFolderName("apps/x.html") + "/" + upload.Uploads[0].Name
	return map[string]string{"upload-png-get": stored, "upload-png-head": stored}
}

// TestUploadReaderLimit is Local's limit: a file at the cap is stored, one byte
// over it is refused with Local's 413 and Local's message, and never truncated.
func TestUploadReaderLimit(t *testing.T) {
	contentType := "multipart/form-data; boundary=" + uploadGoldenBoundary

	exact := uploadReaderBody(uploadGoldenBoundary, "big.png", uploadMaxBytes)
	part, err := readUploadPart(bytes.NewReader(exact), contentType, uploadMaxBytes)
	if err != nil {
		t.Fatalf("a file at the limit: %v", err)
	}
	if part == nil || len(part.content) != uploadMaxBytes {
		t.Fatalf("a file at the limit was not read whole: %v", part)
	}

	over := uploadReaderBody(uploadGoldenBoundary, "big.png", uploadMaxBytes+1)
	part, err = readUploadPart(bytes.NewReader(over), contentType, uploadMaxBytes)
	if part != nil {
		t.Errorf("a file over the limit was read: %d bytes", len(part.content))
	}
	var pathErr *PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("a file over the limit answered %v, want a status error", err)
	}
	if pathErr.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", pathErr.Status, http.StatusRequestEntityTooLarge)
	}
	if pathErr.Message != "Files are limited to 26214400 bytes." {
		t.Errorf("message = %q, want Local's limit message", pathErr.Message)
	}
}

// TestUploadReaderAnswers pins the rest of the reader's answers: the three a body
// Local refuses answers with, and the one it answers with no part at all.
func TestUploadReaderAnswers(t *testing.T) {
	contentType := "multipart/form-data; boundary=" + uploadGoldenBoundary
	cases := []struct {
		name        string
		body        []byte
		contentType string
		status      int
		message     string
	}{
		{
			name:        "not-multipart",
			body:        []byte("not a form"),
			contentType: "text/plain",
			status:      http.StatusBadRequest,
			message:     "Expected a multipart form upload.",
		},
		{
			name:        "no-boundary",
			body:        []byte("not a form"),
			contentType: "multipart/form-data",
			status:      http.StatusBadRequest,
			message:     "Expected a multipart form upload.",
		},
		{
			name:        "malformed",
			body:        []byte("not a multipart body, and no boundary in it\r\n"),
			contentType: contentType,
			status:      http.StatusBadRequest,
			message:     "Could not read the upload.",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			part, err := readUploadPart(bytes.NewReader(testCase.body), testCase.contentType, uploadMaxBytes)
			var pathErr *PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("answer = %v (part %v), want a status error", err, part)
			}
			if pathErr.Status != testCase.status || pathErr.Message != testCase.message {
				t.Errorf("answer = %d %q, want %d %q", pathErr.Status, pathErr.Message, testCase.status, testCase.message)
			}
		})
	}

	// A form with no file part is not an error: the route answers "No file to
	// upload." for the absent part.
	part, err := readUploadPart(bytes.NewReader(uploadReaderFieldBody(uploadGoldenBoundary, "note", "no file here")), contentType, uploadMaxBytes)
	if err != nil || part != nil {
		t.Errorf("a form with no file part = %v, %v, want no part and no error", part, err)
	}
}

// TestUploadNameSplitting pins the stored name's parts: the cleanup a
// client-supplied filename gets, and the hash-suffixed name it becomes.
func TestUploadNameSplitting(t *testing.T) {
	cases := []struct {
		filename string
		stem     string
		ext      string
	}{
		{"photo.png", "photo", ".png"},
		{"../../etc/passwd", "passwd", ""},
		{".avatar.png", "avatar", ".png"},
		{"My Board.v2.JPG", "My Board.v2", ".JPG"},
		{"a<b>c:\"d|e?f*g.png", "abcdefg", ".png"},
		{"", "file", ""},
		{"...", "file", ""},
	}
	for _, testCase := range cases {
		stem, ext := splitUploadName(testCase.filename)
		if stem != testCase.stem || ext != testCase.ext {
			t.Errorf("splitUploadName(%q) = %q, %q, want %q, %q", testCase.filename, stem, ext, testCase.stem, testCase.ext)
		}
	}
	if got := assetsFolderName("My Board.v2.html"); got != "assets-my-board-v2" {
		t.Errorf("assetsFolderName = %q, want assets-my-board-v2", got)
	}
	if got := assetsFolderName("apps/index.html"); got != "assets-index" {
		t.Errorf("assetsFolderName = %q, want assets-index", got)
	}
	if got := strictSegment("My Board (1).png"); got != "My%20Board%20%281%29.png" {
		t.Errorf("strictSegment = %q", got)
	}
}

func uploadReaderBody(boundary, filename string, size int) []byte {
	var body bytes.Buffer
	fmt.Fprintf(&body, "--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: image/png\r\n\r\n", boundary, filename)
	body.Write(bytes.Repeat([]byte("x"), size))
	fmt.Fprintf(&body, "\r\n--%s--\r\n", boundary)
	return body.Bytes()
}

func uploadReaderFieldBody(boundary, name, value string) []byte {
	var body bytes.Buffer
	fmt.Fprintf(&body, "--%s\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n--%s--\r\n", boundary, name, value, boundary)
	return body.Bytes()
}
