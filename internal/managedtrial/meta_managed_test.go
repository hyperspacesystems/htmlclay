//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// localMetaExtensions is Local's own /_/meta list, written out here as Local
// sorts it so the full-host announcement is pinned to a copy rather than to the
// variable it comes from.
const localMetaExtensions = "conditional,data-guard-v1,data-read,data-write,format,live-data,live-provenance,page,blockeditor-awareness-v1,blockeditor-format-v1,blockeditor-recovery-v1,people,presence,receipts,record-awareness-v1,scoped-stylesheet,sync,sync-worker,sync-worker-data,upload,wire"

// metaExtensions reads the extensions array out of one /_/meta answer.
func metaExtensions(t *testing.T, server *httptest.Server) []string {
	t.Helper()
	status, _, answer := forwardSend(t, server, http.MethodGet, "/_/meta", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (answer %q)", status, answer)
	}
	var body struct {
		Extensions []string `json:"extensions"`
	}
	if err := json.Unmarshal(answer, &body); err != nil {
		t.Fatalf("answer = %q, want JSON: %v", answer, err)
	}
	return body.Extensions
}

// TestMetaManagedExtensions proves a root with the JS service attached announces
// exactly what Local announces.
func TestMetaManagedExtensions(t *testing.T) {
	fake := newForwardService(t, nil)
	server, _, _, _ := forwardRoot(t, fake.peer())

	got := metaExtensions(t, server)
	if !slices.Equal(got, managedMetaExtensions) {
		t.Fatalf("extensions = %v, want %v", got, managedMetaExtensions)
	}
	if joined := strings.Join(got, ","); joined != localMetaExtensions {
		t.Fatalf("extensions = %q, want Local's %q", joined, localMetaExtensions)
	}
}

// TestMetaWithoutServiceAnnouncesTrialExtensions proves a root with no service
// still announces the trial's own list.
func TestMetaWithoutServiceAnnouncesTrialExtensions(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the root server reads that checkout's bundled runtime release")
	}
	handler, err := newRootHandler(rootSpec{id: "root-meta", path: consultTestRoot(t)}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	got := metaExtensions(t, server)
	if !slices.Equal(got, trialMetaExtensions) {
		t.Fatalf("extensions = %v, want %v", got, trialMetaExtensions)
	}
}

// TestDataWorkerServedBehindMarker proves the data worker a `sync-worker-data`
// announcement promises is served at /_/sync/data-worker.js, as Local serves it,
// and that the bare address is left to a user folder called `sync`.
func TestDataWorkerServedBehindMarker(t *testing.T) {
	fake := newForwardService(t, nil)
	server, _, _, _ := forwardRoot(t, fake.peer())
	want, err := os.ReadFile(filepath.Join(os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE"), "src", "main", "assets", "sync-data-worker.js"))
	if err != nil {
		t.Fatal(err)
	}

	status, header, answer := forwardSend(t, server, http.MethodGet, "/_/sync/data-worker.js", nil, nil)
	if status != http.StatusOK || !bytes.Equal(answer, want) {
		t.Fatalf("marked: status = %d, %d bytes, want 200 and the checkout's data worker (%d bytes)", status, len(answer), len(want))
	}
	if got := header.Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if header.Get("Cache-Control") != "no-cache" || header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("Cache-Control = %q, X-Content-Type-Options = %q", header.Get("Cache-Control"), header.Get("X-Content-Type-Options"))
	}

	status, _, answer = forwardSend(t, server, http.MethodGet, "/sync/data-worker.js", nil, nil)
	if status == http.StatusOK && bytes.Equal(answer, want) {
		t.Fatalf("bare: the data worker answered without the marker")
	}
}

// TestManagedMetaExtensionsSorted proves the managed list is in Local's order:
// sorted.
func TestManagedMetaExtensionsSorted(t *testing.T) {
	if !slices.IsSorted(managedMetaExtensions) {
		t.Fatalf("managedMetaExtensions = %v, want sorted", managedMetaExtensions)
	}
}
