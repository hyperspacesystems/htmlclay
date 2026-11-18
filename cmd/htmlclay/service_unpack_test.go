package main

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// An update's supervisor waits 20 s for the new version to report serving, and
// main reports it right after startServed. Extracting the bundled service can
// take longer than that on a slow disk, so startServed must come back, and the
// served folder must answer, while the extraction still runs. This fails if the
// extraction ever moves onto startServed's own goroutine.
func TestStartServedDoesNotWaitForTheServiceUnpack(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	cfgBase := t.TempDir()

	// A first run serves the folder, so the run under test finds it in the
	// config and asks for the service as it starts.
	first := newServedTestApp(t, home, cfgBase)
	if _, refusal, err := first.serveFolder(dir, "notes"); err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	a := buildTestApp(t, home, cfgBase)
	a.rt.port = servedTestPort
	a.service = newServiceRunner(a)
	unpacking := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	a.service.command = nil
	a.service.install = func() ([]string, error) {
		once.Do(func() { close(unpacking) })
		<-release
		return nil, errors.New("this test has no service to run")
	}

	started := make(chan struct{})
	go func() {
		a.startServed()
		close(started)
	}()
	t.Cleanup(func() {
		close(release)
		<-started
		a.service.close()
		a.served.Close()
	})

	select {
	case <-unpacking:
	case <-time.After(10 * time.Second):
		t.Fatal("the served folder never asked for the service, so this test proves nothing")
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("startServed is waiting for the service to be unpacked")
	}
	if got := a.snapshot().Host.Service; got != appcore.ServiceStarting {
		t.Fatalf("host.service = %q while the service unpacks, want %q", got, appcore.ServiceStarting)
	}
	rec := servedRequest(t, a, http.MethodGet, servedHostName("notes", servedTestPort), "/index.html", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "first") {
		t.Fatalf("GET while the service unpacks = %d (%s), want the document", rec.Code, rec.Body.String())
	}
}
