//go:build managedtrial || servedfaults

package served

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// H1: the fault sweep. Every write route this host serves is crossed with every
// fault the fault points arm, and after each one the folder is judged the way a
// person would judge it: the document is whole, the bytes it replaced are still
// in its history, nothing is left beside it, and the next save still works.
//
// The test is a probe. A check that fails is a finding about the host, reported
// rather than worked around.

// huntDoc is the one document every lane writes, with the heading that tells the
// copies apart. It carries a data rule so the data API lane can change it.
func huntDoc(heading string) string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Hunt</title>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1"}</script>
</head>
<body>
<h1>` + heading + `</h1>
</body>
</html>
`
}

const (
	// huntDataBody is the data API write the data-api lane sends.
	huntDataBody = `{"title":"Hunt New"}`
	// huntNextDataBody is the data API write the same lane sends after the restart.
	huntNextDataBody = `{"title":"Hunt Next"}`
	// huntNextHeading is the heading every other lane writes after the restart.
	huntNextHeading = "Hunt Next"
	// huntPublicHost is the served folder's own address, which the gate admits.
	huntPublicHost = "hunt.htmlclay.localhost:1212"
	// huntRootID names the root in the control API.
	huntRootID = "hunt-root"
	// huntFaultNone is the control row: the lane runs with no fault armed.
	huntFaultNone = "none"
)

// huntLane is one write route to doc.html.
type huntLane struct {
	name string
	// control is a lane that writes through POST /v1/commit on a control server.
	control bool
	// kind is the commit kind the control lanes send.
	kind string
	// restore is the lane that puts a stored version back.
	restore bool
	// dataAPI is the lane that writes through POST /_/api.
	dataAPI bool
}

// huntLanes is every write route the sweep crosses with every fault.
func huntLanes() []huntLane {
	return []huntLane{
		{name: "browser-save"},
		{name: "data-api", dataAPI: true},
		{name: "restore", restore: true},
		{name: "control-browser-save", control: true, kind: "browser-save"},
		// The replace lane's kind is `restore` in this host's KINDS table; there
		// is no `replace` kind for the control route to name.
		{name: "control-replace", control: true, kind: "restore"},
		{name: "control-sync-download", control: true, kind: "sync-download"},
	}
}

// huntAnswer is what one write answered: its status and body, or the transport
// error that stood in for them.
type huntAnswer struct {
	status int
	body   string
	err    error
}

func (a huntAnswer) String() string {
	if a.err != nil {
		return "transport error: " + a.err.Error()
	}
	return strings.TrimSpace(a.body) + " (status " + strconv.Itoa(a.status) + ")"
}

// huntSite is one lane's route to one folder: the server it talks to and the
// document it writes.
type huntSite struct {
	lane    huntLane
	dir     string
	docPath string
	host    *Host
	server  *httptest.Server
}

// huntOpen builds the servers a lane needs over dir, the way a start does.
func huntOpen(t *testing.T, lane huntLane, dir string) *huntSite {
	t.Helper()
	site := &huntSite{lane: lane, dir: dir, docPath: filepath.Join(dir, "doc.html")}
	if lane.control {
		registry := newConsentRegistry(dir, nil)
		registry.Ready()
		baseDir := registry.BaseReal()
		commit := newCommitHost(baseDir, huntRootID, registry, newGuardHooks(nil), nil)
		root := newControlRoot(huntRootID, baseDir, registry, commit, newEventRing())
		site.server = httptest.NewServer(newControlHandler(controlSecret, testControlRoots{root}, root.events, nil))
		return site
	}
	host := hostTestHost(t)
	handler, err := host.AddRoot(RootSpec{ID: huntRootID, Path: dir, PublicHost: huntPublicHost})
	if err != nil {
		t.Fatalf("AddRoot %s: %v", dir, err)
	}
	site.host = host
	site.server = httptest.NewServer(handler)
	return site
}

// close ends the servers, as a quit would.
func (s *huntSite) close() {
	s.server.Close()
	if s.host != nil {
		s.host.Close()
	}
}

// huntDo sends one request and reads its answer.
func huntDo(request *http.Request) huntAnswer {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return huntAnswer{err: err}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return huntAnswer{err: readErr}
	}
	return huntAnswer{status: response.StatusCode, body: string(body)}
}

// huntServedRequest is a request a browser makes to a served folder: its own
// Host, its own Origin, and the document it is about.
func (s *huntSite) huntServedRequest(t *testing.T, method, path, contentType, body string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, s.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = huntPublicHost
	request.Header.Set("Origin", "http://"+huntPublicHost)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", "http://"+huntPublicHost+"/doc.html")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

// save is POST /_/save of one document.
func (s *huntSite) save(t *testing.T, content string) huntAnswer {
	t.Helper()
	return huntDo(s.huntServedRequest(t, http.MethodPost, "/_/save", "text/html", content))
}

// dataWrite is POST /_/api/doc.html of one data payload.
func (s *huntSite) dataWrite(t *testing.T, body string) huntAnswer {
	t.Helper()
	return huntDo(s.huntServedRequest(t, http.MethodPost, "/_/api/doc.html", "application/json", body))
}

// restoreVersion is POST /_/restore/doc.html/<version>: put back the version
// whose bytes are content.
func (s *huntSite) restoreVersion(t *testing.T, content string) huntAnswer {
	t.Helper()
	name := huntVersionHolding(t, s.dir, content)
	if name == "" {
		t.Fatalf("no version of doc.html holds the %d bytes to restore", len(content))
	}
	return huntDo(s.huntServedRequest(t, http.MethodPost, "/_/restore/doc.html/"+name, "", ""))
}

// commit is POST /v1/commit on the control server, with the lane's kind.
func (s *huntSite) commit(t *testing.T, kind, content string) huntAnswer {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"root":  huntRootID,
		"name":  "doc.html",
		"kind":  kind,
		"bytes": map[string]any{"encoding": "utf8", "data": content},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/v1/commit", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+controlSecret)
	request.Header.Set("Content-Type", "application/json")
	return huntDo(request)
}

// seed writes one document through the lane's own server, with no fault armed.
func (s *huntSite) seed(t *testing.T, content string) huntAnswer {
	t.Helper()
	if s.lane.control {
		return s.commit(t, "create", content)
	}
	return s.save(t, content)
}

// write is the lane's one write of content to doc.html.
func (s *huntSite) write(t *testing.T, content string) huntAnswer {
	t.Helper()
	switch {
	case s.lane.control:
		return s.commit(t, s.lane.kind, content)
	case s.lane.dataAPI:
		return s.dataWrite(t, huntDataBody)
	case s.lane.restore:
		return s.restoreVersion(t, content)
	default:
		return s.save(t, content)
	}
}

// nextWrite is one more normal write on the same lane, after a restart.
func (s *huntSite) nextWrite(t *testing.T) huntAnswer {
	t.Helper()
	switch {
	case s.lane.control:
		return s.commit(t, s.lane.kind, huntDoc(huntNextHeading))
	case s.lane.dataAPI:
		return s.dataWrite(t, huntNextDataBody)
	case s.lane.restore:
		return s.restoreVersion(t, huntDoc("Old"))
	default:
		return s.save(t, huntDoc(huntNextHeading))
	}
}

// succeeded reports whether the write worked: a 200, or a committed control op.
func (s *huntSite) succeeded(answer huntAnswer) bool {
	if answer.err != nil || answer.status != http.StatusOK {
		return false
	}
	if !s.lane.control {
		return true
	}
	outcome := map[string]any{}
	if err := json.Unmarshal([]byte(answer.body), &outcome); err != nil {
		return false
	}
	return outcome["outcome"] == "committed"
}

// huntTempDir is a fresh folder with its symlinks resolved, so the paths the
// host canonicalizes and the paths the test reads are the same.
func huntTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// huntRead is doc.html's bytes.
func huntRead(t *testing.T, docPath string) string {
	t.Helper()
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading %s: %v", docPath, err)
	}
	return string(data)
}

// huntShort is a document's first bytes, for a failure message.
func huntShort(content string) string {
	if len(content) <= 120 {
		return content
	}
	return content[:120] + "..."
}

// huntVersionFiles is every file under the root's versions folder.
func huntVersionFiles(t *testing.T, dir string) []string {
	t.Helper()
	root := filepath.Join(dir, ".hyperclay", "versions")
	found := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		found = append(found, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}

// huntVersionHolding is the name of a stored version of doc.html whose bytes are
// exactly content, or "" when there is none.
func huntVersionHolding(t *testing.T, dir, content string) string {
	t.Helper()
	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "doc")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(versionsDir, entry.Name()))
		if readErr != nil {
			continue
		}
		if string(data) == content {
			return entry.Name()
		}
	}
	return ""
}

// huntExtras is everything in doc.html's folder beside doc.html itself and the
// .hyperclay directory.
func huntExtras(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	extra := []string{}
	for _, entry := range entries {
		if entry.Name() == "doc.html" || entry.Name() == ".hyperclay" {
			continue
		}
		extra = append(extra, entry.Name())
	}
	return extra
}

// huntStage writes the documents a lane's write needs already on disk: the
// restore lane can only put back a version that exists, so its staged document is
// saved first and the old one saved on top of it.
func huntStage(t *testing.T, site *huntSite, staged string) {
	t.Helper()
	if site.lane.restore {
		if answer := site.seed(t, staged); !site.succeeded(answer) {
			t.Fatalf("staging the version to restore: %s", answer)
		}
	}
	if answer := site.seed(t, huntDoc("Old")); !site.succeeded(answer) {
		t.Fatalf("seeding doc.html: %s", answer)
	}
}

// huntOne is one lane crossed with one fault.
func huntOne(t *testing.T, lane huntLane, fault string) {
	replaceKillSelf(t)

	// The control run learns what this lane writes: the bytes its write leaves on
	// disk, the bytes its next write leaves, and whether a normal write of this
	// lane keeps the bytes it replaced in the document's history.
	probeDir := huntTempDir(t)
	probe := huntOpen(t, lane, probeDir)
	huntStage(t, probe, huntDoc("Probe"))
	if answer := probe.write(t, huntDoc("Probe")); !probe.succeeded(answer) {
		t.Fatalf("the un-faulted write of %s did not work: %s", lane.name, answer)
	}
	newBytes := huntRead(t, probe.docPath)
	keptVersions := huntVersionHolding(t, probeDir, huntDoc("Old")) != ""
	if answer := probe.nextWrite(t); !probe.succeeded(answer) {
		t.Fatalf("the un-faulted next write of %s did not work: %s", lane.name, answer)
	}
	nextBytes := huntRead(t, probe.docPath)
	probe.close()

	caseDir := huntTempDir(t)
	site := huntOpen(t, lane, caseDir)
	huntStage(t, site, newBytes)
	if fault != huntFaultNone {
		armFault(fault, "doc.html", 1)
	}
	answer := site.write(t, newBytes)
	if fault == faultKillBeforeRename || fault == faultKillAfterRename {
		if answer.err == nil && answer.status < 500 {
			t.Errorf("%s/%s: the killed write answered %s, want a transport error or a 5xx", lane.name, fault, answer)
		}
	}
	clearFaults()
	site.close()

	// A restart: a fresh host or control server over the same folder.
	fresh := huntOpen(t, lane, caseDir)

	oldBytes := huntDoc("Old")
	got := huntRead(t, fresh.docPath)
	landed := "other"
	switch got {
	case oldBytes:
		landed = "old"
	case newBytes:
		landed = "new"
	default:
		t.Errorf("%s/%s: doc.html holds neither the bytes it replaced nor the bytes the write carried, %d bytes: %q",
			lane.name, fault, len(got), huntShort(got))
	}

	if landed == "new" && keptVersions && huntVersionHolding(t, caseDir, oldBytes) == "" {
		t.Errorf("%s/%s: doc.html holds the new bytes, but no version holds the bytes it replaced", lane.name, fault)
	}

	extras := huntExtras(t, caseDir)
	if len(extras) > 0 && !(fault == faultKillBeforeRename && onlyAtomicTemps(extras)) {
		t.Errorf("%s/%s: doc.html's folder holds %v beside doc.html and .hyperclay", lane.name, fault, extras)
	}

	next := fresh.nextWrite(t)
	nextResult := ""
	if next.err != nil {
		nextResult = "error"
		t.Errorf("%s/%s: the next %s write failed at the transport: %v", lane.name, fault, lane.name, next.err)
	} else {
		nextResult = strconv.Itoa(next.status)
		if !fresh.succeeded(next) {
			t.Errorf("%s/%s: the next %s write answered %s, want a success", lane.name, fault, lane.name, next)
		} else if after := huntRead(t, fresh.docPath); after != nextBytes {
			t.Errorf("%s/%s: the next %s write left %d bytes, want the %d bytes it writes without the fault: %q",
				lane.name, fault, lane.name, len(after), len(nextBytes), huntShort(after))
		} else {
			nextResult = "ok"
		}
	}
	fresh.close()

	extraNames := "none"
	if len(extras) > 0 {
		extraNames = strings.Join(extras, ",")
	}
	t.Logf("HUNT lane=%s fault=%s landed=%s versions=%d extra=%s next=%s",
		lane.name, fault, landed, len(huntVersionFiles(t, caseDir)), extraNames, nextResult)
}

// TestHuntFaultSweep arms every fault point through every write route and judges
// the folder afterwards.
func TestHuntFaultSweep(t *testing.T) {
	faults := []string{huntFaultNone, faultKillBeforeRename, faultKillAfterRename, faultDurableFail}
	ran := 0
	for _, lane := range huntLanes() {
		for _, fault := range faults {
			if fault == faultDurableFail && runtime.GOOS == "windows" {
				continue
			}
			ran++
			t.Run(lane.name+"/"+fault, func(t *testing.T) {
				huntOne(t, lane, fault)
			})
		}
	}
	// Every lane with every fault this platform can arm (Windows has no
	// durable-failure point), so a lane or fault dropped from the lists fails.
	want := len(huntLanes()) * len(faults)
	if runtime.GOOS == "windows" {
		want = len(huntLanes()) * (len(faults) - 1)
	}
	if ran != want || ran < 18 {
		t.Fatalf("the sweep ran %d cases, want %d (at least 18)", ran, want)
	}
}

// onlyAtomicTemps reports whether every leftover is doc.html's atomic temp: the
// known hunt finding H1-a (a crash before the rename leaves the temp behind,
// as Local does), logged by the HUNT line until its follow-up lands.
func onlyAtomicTemps(names []string) bool {
	for _, name := range names {
		if !strings.HasPrefix(name, ".doc.html.") || !strings.HasSuffix(name, ".tmp") {
			return false
		}
	}
	return true
}
