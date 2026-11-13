//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMetaGolden pins /_/meta and /_/blockeditor-state to Local's real answers: the
// host's extension list, the document block for every failure and success the
// recorder exercised, and the conditional blockeditor-state read.
func TestMetaGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout")
	}
	runGoldenCases(t, filepath.Join("testdata", "meta-golden"), localSource, normalizeMetaAnswer)
}

// ad1RecordedExtensions are the recorded names AD1 lets the trial leave out.
var ad1RecordedExtensions = map[string]bool{
	"format":            true,
	"scoped-stylesheet": true,
	"presence":          true,
	"sync-worker-data":  true,
}

// normalizeMetaAnswer applies AD1 to the recorded answer: remove from the
// recorded extension list the names the trial is allowed not to announce, so
// the rest of the answer is compared exactly.
func normalizeMetaAnswer(answer *serveAnswer) {
	if answer.Body.Text == nil {
		return
	}
	var body struct {
		Spec       int             `json:"spec"`
		Extensions []string        `json:"extensions"`
		Document   json.RawMessage `json:"document,omitempty"`
	}
	if err := json.Unmarshal([]byte(*answer.Body.Text), &body); err != nil || body.Extensions == nil {
		return
	}
	kept := body.Extensions[:0]
	for _, name := range body.Extensions {
		if ad1RecordedExtensions[name] || strings.HasPrefix(name, "live-") {
			continue
		}
		kept = append(kept, name)
	}
	body.Extensions = kept
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(body); err != nil {
		return
	}
	text := string(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	answer.Body.Text = &text
	// The recorded headers describe the recorded body, so the two facts that
	// derive from it move with it.
	answer.Headers["content-length"] = strconv.Itoa(len(text))
	answer.Headers["etag"] = weakEntityETag([]byte(text))
}
