package served

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/served/assets"
)

// switchTestHost is a host built the way the product builds one with a service
// link: no peer at first, so every semantic write waits.
func switchTestHost(t *testing.T) *Host {
	t.Helper()
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		Service:         true,
	})
	t.Cleanup(host.Close)
	return host
}

// switchTestServer serves one root of a host that is already built.
func switchTestServer(t *testing.T, host *Host, id string) (*httptest.Server, string) {
	t.Helper()
	dir := consultTestRoot(t)
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	handler, err := host.AddRoot(RootSpec{ID: id, Path: dir})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, dir
}

// switchTestHealth is the control listener's root count.
func switchTestHealth(t *testing.T, control *httptest.Server) int {
	t.Helper()
	status, answer := controlJSON(t, control, controlSecret, http.MethodGet, "/health", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /health = %d (%v), want 200", status, answer)
	}
	count, ok := answer["roots"].(float64)
	if !ok {
		t.Fatalf("GET /health = %v, want a roots count", answer)
	}
	return int(count)
}

// TestControlSeesRootsAddedLater is the control listener built before any root
// exists: it finds a root added afterwards, and stops finding it when removed.
func TestControlSeesRootsAddedLater(t *testing.T) {
	host := switchTestHost(t)
	control := httptest.NewServer(host.ControlHandler(controlSecret))
	t.Cleanup(control.Close)

	if count := switchTestHealth(t, control); count != 0 {
		t.Fatalf("GET /health = %d roots before any AddRoot, want 0", count)
	}

	dir := consultTestRoot(t)
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	readPath := "/v1/read?root=later&name=page.html"

	status, answer := controlJSON(t, control, controlSecret, http.MethodGet, readPath, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("GET %s before AddRoot = %d (%v), want 400 for an unknown root", readPath, status, answer)
	}

	if _, err := host.AddRoot(RootSpec{ID: "later", Path: dir}); err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	if count := switchTestHealth(t, control); count != 1 {
		t.Fatalf("GET /health = %d roots after AddRoot, want 1", count)
	}
	status, answer = controlJSON(t, control, controlSecret, http.MethodGet, readPath, nil)
	if status != http.StatusOK || answer["exists"] != true {
		t.Fatalf("GET %s after AddRoot = %d (%v), want the document", readPath, status, answer)
	}

	if !host.RemoveRoot("later") {
		t.Fatal("RemoveRoot = false, want the root removed")
	}
	if count := switchTestHealth(t, control); count != 0 {
		t.Fatalf("GET /health = %d roots after RemoveRoot, want 0", count)
	}
	status, answer = controlJSON(t, control, controlSecret, http.MethodGet, readPath, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("GET %s after RemoveRoot = %d (%v), want 400 for an unknown root", readPath, status, answer)
	}
}

// TestControlEventsAreTheHostsRing is GET /v1/events on a control listener built
// with no roots: the notice a root added afterwards produces arrives on it.
func TestControlEventsAreTheHostsRing(t *testing.T) {
	host := switchTestHost(t)
	control := httptest.NewServer(host.ControlHandler(controlSecret))
	t.Cleanup(control.Close)

	dir := consultTestRoot(t)
	handler, err := host.AddRoot(RootSpec{ID: "later", Path: dir})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	server, ok := handler.(*rootServer)
	if !ok {
		t.Fatalf("AddRoot returned %T, want *rootServer", handler)
	}

	since := host.cfg.events.currentCursor()
	server.live.notices("hello", map[string]any{"greeting": "hi"})

	frame := eventLine(t, control, since)
	if frame["type"] != "notice" || frame["root"] != "later" || frame["rel"] != "hello" {
		t.Fatalf("the stream wrote %v, want the notice the added root produced", frame)
	}
}

// TestServiceWaitingAnswers503 is the link with no peer: a semantic save is
// refused retryably and the file on disk is untouched.
func TestServiceWaitingAnswers503(t *testing.T) {
	host := switchTestHost(t)
	server, dir := switchTestServer(t, host, "waiting")

	sent := strings.Replace(consultSavePage, "Saved", "Waiting", 1)
	status, headers, answer := consultSavePost(t, server, "page.html", sent, nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("the save = %d (%s), want 503 while the service has no peer", status, answer)
	}
	if retry := headers.Get("Retry-After"); retry != "1" {
		t.Fatalf("Retry-After = %q, want \"1\"", retry)
	}
	if disk := consultTestDisk(t, filepath.Join(dir, "page.html")); strings.Contains(disk, "Waiting") {
		t.Fatalf("the file changed on disk while the service had no peer")
	}
}

// TestServiceDetachedRunsNative is the switch: detached the save runs Go's
// native lanes, attached again it waits for the service.
func TestServiceDetachedRunsNative(t *testing.T) {
	host := switchTestHost(t)
	server, dir := switchTestServer(t, host, "detached")

	host.DetachService()
	if !host.ServiceDetached() {
		t.Fatal("ServiceDetached = false after DetachService")
	}
	status, _, answer := consultSavePost(t, server, "page.html", strings.Replace(consultSavePage, "Saved", "Native", 1), nil)
	if status != http.StatusOK {
		t.Fatalf("the detached save = %d (%s), want the native 200", status, answer)
	}
	if disk := consultTestDisk(t, filepath.Join(dir, "page.html")); !strings.Contains(disk, "Native") {
		t.Fatalf("the detached save wrote nothing on disk: %q", disk)
	}

	host.AttachService()
	if host.ServiceDetached() {
		t.Fatal("ServiceDetached = true after AttachService")
	}
	status, headers, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusServiceUnavailable || headers.Get("Retry-After") != "1" {
		t.Fatalf("the attached save = %d (%s), want the recoverable 503", status, answer)
	}
}

// TestServicePeerAttached is a peer set while the host runs: the save consults
// the stub service; clearing the peer makes it wait again.
func TestServicePeerAttached(t *testing.T) {
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" {
			return map[string]any{"decision": "refuse", "status": http.StatusConflict, "code": "stale"}
		}
		return nil
	})
	peer := consultSavePeer(t, script)
	host := switchTestHost(t)
	server, _ := switchTestServer(t, host, "attached")

	if err := host.SetServicePeer(peer.URL, peer.Secret, peer.Epoch); err != nil {
		t.Fatalf("SetServicePeer: %v", err)
	}
	status, _, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusConflict {
		t.Fatalf("the consulted save = %d (%s), want the stub's 409", status, answer)
	}
	if called := script.count("/v1/admit"); called != 1 {
		t.Fatalf("the stub saw %d admits, want 1", called)
	}

	host.ClearServicePeer()
	status, headers, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusServiceUnavailable || headers.Get("Retry-After") != "1" {
		t.Fatalf("the save after ClearServicePeer = %d (%s), want the recoverable 503", status, answer)
	}
}

// TestServiceDetachedWriteIsUnconfirmed is a page document Go saved natively
// while the service was detached: once the service is back, a semantic write to
// it waits until the service has reconciled it, and only page documents count.
func TestServiceDetachedWriteIsUnconfirmed(t *testing.T) {
	script := newConsultScript(func(path string, body map[string]any, call int) any { return nil })
	peer := consultSavePeer(t, script)
	host := switchTestHost(t)
	server, dir := switchTestServer(t, host, "detached")
	consultWriteFile(t, dir, "style.css", []byte("p{}"))

	host.DetachService()
	if status, _, answer := consultSavePost(t, server, "page.html", strings.Replace(consultSavePage, "Saved", "Native", 1), nil); status != http.StatusOK {
		t.Fatalf("the detached save = %d (%s), want the native 200", status, answer)
	}
	if got := host.UnconfirmedCount(); got != 1 {
		t.Fatalf("UnconfirmedCount after a detached page save = %d, want 1", got)
	}

	host.AttachService()
	if err := host.SetServicePeer(peer.URL, peer.Secret, peer.Epoch); err != nil {
		t.Fatalf("SetServicePeer: %v", err)
	}
	status, _, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status == http.StatusOK {
		t.Fatalf("a semantic save before reconcile = 200 (%s), want it held", answer)
	}
	if disk := consultTestDisk(t, filepath.Join(dir, "page.html")); !strings.Contains(disk, "Native") {
		t.Fatalf("the held save changed the file: %q", disk)
	}
	if calls := script.count("/v1/admit"); calls != 0 {
		t.Fatalf("admit was called %d times before reconcile, want none", calls)
	}

	host.cfg.service.unconfirmed.lift(filepath.Join(host.byID["detached"].baseDir, "page.html"))
	if got := host.UnconfirmedCount(); got != 0 {
		t.Fatalf("UnconfirmedCount after reconcile = %d, want 0", got)
	}
	consultSavePost(t, server, "page.html", consultSavePage, nil)
	if calls := script.count("/v1/admit"); calls == 0 {
		t.Fatal("a save after reconcile did not consult the service")
	}
}

// TestServiceAttachedWriteIsNotMarked: the hook only fires while detached.
func TestServiceAttachedWriteIsNotMarked(t *testing.T) {
	host := switchTestHost(t)
	server, _ := switchTestServer(t, host, "plain")
	consultSavePost(t, server, "page.html", consultSavePage, nil)
	if got := host.UnconfirmedCount(); got != 0 {
		t.Fatalf("UnconfirmedCount with the service waiting = %d, want 0", got)
	}
}
