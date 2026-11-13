package served

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	racePublicHost = "hunt-race.htmlclay.localhost:1212"
	raceRootID     = "hunt-race-root"
	raceDocName    = "doc.html"
)

type raceAnswer struct {
	status int
	body   string
	err    error
}

func (a raceAnswer) String() string {
	if a.err != nil {
		return "transport error: " + a.err.Error()
	}
	return strings.TrimSpace(a.body) + " (status " + fmt.Sprint(a.status) + ")"
}

type raceWrite struct {
	marker string
	data   bool
	answer raceAnswer
}

func raceDocument(body string) string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Race</title>
<script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1"}</script>
</head>
<body>
` + body + `
</body>
</html>
`
}

func raceBrowserBody(marker string) string {
	return raceDocument(`<h1>hunt</h1>
<p data-mark="` + marker + `">` + marker + `</p>`)
}

func raceOutsideBody(marker string) string {
	return raceDocument(`<h1>` + marker + `</h1>`)
}

func raceTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func raceServer(t *testing.T, dir string, watch bool) (*httptest.Server, *rootServer) {
	t.Helper()
	host := hostTestHost(t)
	if watch {
		host.Start()
	}
	handler, err := host.AddRoot(RootSpec{ID: raceRootID, Path: dir, PublicHost: racePublicHost})
	if err != nil {
		t.Fatalf("AddRoot %s: %v", dir, err)
	}
	root, ok := handler.(*rootServer)
	if !ok {
		t.Fatalf("AddRoot returned %T, want *rootServer", handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, root
}

func raceSend(serverURL, path, contentType, body string) raceAnswer {
	request, err := http.NewRequest(http.MethodPost, serverURL+path, strings.NewReader(body))
	if err != nil {
		return raceAnswer{err: err}
	}
	request.Host = racePublicHost
	request.Header.Set("Origin", "http://"+racePublicHost)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", "http://"+racePublicHost+"/"+raceDocName)
	request.Header.Set("Content-Type", contentType)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return raceAnswer{err: err}
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		return raceAnswer{err: err}
	}
	return raceAnswer{status: response.StatusCode, body: string(answer)}
}

func raceSendWrite(serverURL, marker string, data bool) raceAnswer {
	if data {
		return raceSend(serverURL, "/_/api/"+raceDocName, "application/json", `{"title":"`+marker+`"}`)
	}
	return raceSend(serverURL, "/_/save", "text/html", raceBrowserBody(marker))
}

func raceRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

type raceVersion struct {
	name string
	body string
}

func raceVersions(t *testing.T, dir string) []raceVersion {
	t.Helper()
	root := filepath.Join(dir, ".hyperclay", "versions")
	found := []raceVersion{}
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
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		found = append(found, raceVersion{name: path, body: string(data)})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}

func raceHeld(versions []raceVersion, marker string) bool {
	for _, version := range versions {
		if strings.Contains(version.body, marker) {
			return true
		}
	}
	return false
}

func raceHostFilesHolding(t *testing.T, dir, marker string) []string {
	t.Helper()
	root := filepath.Join(dir, ".hyperclay")
	holding := []string{}
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
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), marker) {
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				rel = path
			}
			holding = append(holding, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return holding
}

func raceExtras(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	extra := []string{}
	for _, entry := range entries {
		if entry.Name() == raceDocName || entry.Name() == ".hyperclay" {
			continue
		}
		extra = append(extra, entry.Name())
	}
	return extra
}

func raceRefused(writes []raceWrite) string {
	counts := map[string]int{}
	for _, write := range writes {
		if write.answer.err != nil {
			counts["transport"]++
			continue
		}
		if write.answer.status == http.StatusOK {
			continue
		}
		counts[fmt.Sprint(write.answer.status)]++
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+fmt.Sprint(counts[name]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func raceSeed(t *testing.T, server *httptest.Server) {
	t.Helper()
	answer := raceSend(server.URL, "/_/save", "text/html", raceDocument(`<h1>seed</h1>`))
	if answer.err != nil || answer.status != http.StatusOK {
		t.Fatalf("seeding %s answered %s", raceDocName, answer)
	}
}

func raceChangeFrames(root *rootServer) int {
	frames := 0
	for _, frame := range root.control.events.heldFrames() {
		if strings.Contains(frame, `"type":"change"`) && strings.Contains(frame, `"rel":"`+raceDocName+`"`) {
			frames++
		}
	}
	return frames
}

func TestHuntConcurrentWriters(t *testing.T) {
	t.Run("tabs-and-api", huntRaceTabsAndAPI)
	t.Run("outside-edits", huntRaceOutsideEdits)
}

func huntRaceTabsAndAPI(t *testing.T) {
	dir := raceTempDir(t)
	server, _ := raceServer(t, dir, false)
	raceSeed(t, server)

	const browserWriters = 4
	const dataWriters = 2
	const writesEach = 40
	const writers = browserWriters + dataWriters

	results := make([][]raceWrite, writers)
	for index := range results {
		results[index] = make([]raceWrite, writesEach)
	}

	var wait sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wait.Add(1)
		go func(writer int) {
			defer wait.Done()
			data := writer >= browserWriters
			for write := 1; write <= writesEach; write++ {
				marker := fmt.Sprintf("b%d-n%02d", writer, write)
				if data {
					marker = fmt.Sprintf("a%d-n%02d", writer-browserWriters, write)
				}
				results[writer][write-1] = raceWrite{marker: marker, data: data, answer: raceSendWrite(server.URL, marker, data)}
			}
		}(writer)
	}
	wait.Wait()

	all := []raceWrite{}
	for _, lane := range results {
		all = append(all, lane...)
	}

	doc := raceRead(t, filepath.Join(dir, raceDocName))
	versions := raceVersions(t, dir)

	acked := 0
	lost := []string{}
	for _, write := range all {
		if write.answer.err != nil || write.answer.status != http.StatusOK {
			continue
		}
		acked++
		if !strings.Contains(doc, write.marker) && !raceHeld(versions, write.marker) {
			lost = append(lost, write.marker)
		}
	}

	browserInDoc := []string{}
	dataInDoc := []string{}
	for _, write := range all {
		if !strings.Contains(doc, write.marker) {
			continue
		}
		if write.data {
			dataInDoc = append(dataInDoc, write.marker)
			continue
		}
		browserInDoc = append(browserInDoc, write.marker)
	}

	t.Logf("HUNT race subtest=tabs-and-api acked=%d refused=%s lost=%d unobserved=%d versions=%d",
		acked, raceRefused(all), len(lost), 0, len(versions))

	for _, marker := range lost {
		t.Errorf("tabs-and-api: %s answered 200, but neither %s nor any version holds its bytes", marker, raceDocName)
	}
	if len(browserInDoc) > 1 {
		t.Errorf("tabs-and-api: %s holds markers of %d browser saves (%v), want at most one", raceDocName, len(browserInDoc), browserInDoc)
	}
	if len(dataInDoc) > 1 {
		t.Errorf("tabs-and-api: %s holds the result of %d data API writes (%v), want at most one", raceDocName, len(dataInDoc), dataInDoc)
	}
	if len(browserInDoc) == 0 && len(dataInDoc) == 0 {
		t.Errorf("tabs-and-api: %s holds no marker of any write, not even the last acknowledged one", raceDocName)
	}
	if extras := raceExtras(t, dir); len(extras) > 0 {
		t.Errorf("tabs-and-api: %s's folder holds %v beside %s and .hyperclay", raceDocName, extras, raceDocName)
	}
	if acked == 0 {
		t.Fatalf("tabs-and-api: acknowledged no write at all, so nothing was proved")
	}
}

func huntRaceOutsideEdits(t *testing.T) {
	dir := raceTempDir(t)
	server, root := raceServer(t, dir, true)
	raceSeed(t, server)
	docPath := filepath.Join(dir, raceDocName)

	// A tab autosaving while another editor saves the same file, both faster
	// than people do. Saves run back to back would spend a measurable share of
	// the time between the host's last read and its rename, a gap no program
	// can close without OS file locks, so the load stays realistic.
	const savers = 1
	const savesEach = 30
	const outsideWrites = 80
	const saveEvery = 200 * time.Millisecond
	const outsideEvery = 70 * time.Millisecond

	results := make([][]raceWrite, savers)
	for index := range results {
		results[index] = make([]raceWrite, savesEach)
	}
	outsideMarkers := make([]string, outsideWrites)
	outsideErrors := make([]error, outsideWrites)

	var wait sync.WaitGroup
	for saver := 0; saver < savers; saver++ {
		wait.Add(1)
		go func(saver int) {
			defer wait.Done()
			for write := 1; write <= savesEach; write++ {
				marker := fmt.Sprintf("s%d-n%02d", saver, write)
				results[saver][write-1] = raceWrite{marker: marker, answer: raceSendWrite(server.URL, marker, false)}
				time.Sleep(saveEvery)
			}
		}(saver)
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		for write := 1; write <= outsideWrites; write++ {
			marker := fmt.Sprintf("o-n%02d", write)
			outsideMarkers[write-1] = marker
			outsideErrors[write-1] = os.WriteFile(docPath, []byte(raceOutsideBody(marker)), 0o644)
			time.Sleep(outsideEvery)
		}
	}()
	wait.Wait()

	time.Sleep(3 * time.Second)

	all := []raceWrite{}
	for _, lane := range results {
		all = append(all, lane...)
	}

	doc := raceRead(t, docPath)
	versions := raceVersions(t, dir)

	acked := 0
	lost := []string{}
	for _, write := range all {
		if write.answer.err != nil || write.answer.status != http.StatusOK {
			continue
		}
		acked++
		if !strings.Contains(doc, write.marker) && !raceHeld(versions, write.marker) {
			lost = append(lost, write.marker)
		}
	}

	unobserved := []string{}
	outsideLost := []string{}
	outsideFailed := []string{}
	for index, marker := range outsideMarkers {
		if outsideErrors[index] != nil {
			outsideFailed = append(outsideFailed, marker)
			continue
		}
		if strings.Contains(doc, marker) || raceHeld(versions, marker) {
			continue
		}
		superseded := false
		for later := index + 1; later < len(outsideMarkers); later++ {
			if strings.Contains(doc, outsideMarkers[later]) || raceHeld(versions, outsideMarkers[later]) {
				superseded = true
				break
			}
		}
		if superseded {
			unobserved = append(unobserved, marker)
			continue
		}
		outsideLost = append(outsideLost, marker)
	}

	inDoc := []string{}
	for _, write := range all {
		if strings.Contains(doc, write.marker) {
			inDoc = append(inDoc, write.marker)
		}
	}
	for _, marker := range outsideMarkers {
		if strings.Contains(doc, marker) {
			inDoc = append(inDoc, marker)
		}
	}

	t.Logf("HUNT race subtest=outside-edits acked=%d refused=%s lost=%d unobserved=%d versions=%d",
		acked, raceRefused(all), len(lost)+len(outsideLost), len(unobserved), len(versions))
	recovered := 0
	for _, marker := range outsideMarkers {
		if len(raceHostFilesHolding(t, dir, marker)) > 0 {
			recovered++
		}
	}
	t.Logf("HUNT race note=outside-edits change-frames=%d host-files-holding-an-outside-marker=%d outside-writes=%d",
		raceChangeFrames(root), recovered, len(outsideMarkers))
	if len(unobserved) > 0 {
		t.Logf("HUNT race subtest=outside-edits unobserved=%v", unobserved)
	}
	if len(outsideFailed) > 0 {
		t.Logf("HUNT race subtest=outside-edits outside-write-errors=%d (%v)", len(outsideFailed), outsideFailed)
	}

	for _, marker := range lost {
		t.Errorf("outside-edits: browser save %s answered 200, but neither %s nor any version holds its bytes", marker, raceDocName)
	}
	for _, marker := range outsideLost {
		t.Errorf("outside-edits: outside write %s is in neither %s nor any version, and no later outside write replaced it", marker, raceDocName)
	}
	if len(inDoc) != 1 {
		t.Errorf("outside-edits: %s holds %d markers (%v), want exactly one", raceDocName, len(inDoc), inDoc)
	}
	if acked == 0 {
		t.Fatalf("outside-edits: acknowledged no write at all, so nothing was proved")
	}
}
