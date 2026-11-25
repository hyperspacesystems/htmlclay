package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// The three races this file hunts: a person renaming the document an open tab
// is autosaving, a person stopping the folder that tab autosaves into, and a
// person serving the folder whose document was already open as a trusted-folder
// file. Every save carries a marker of its own, so afterwards each answer can be
// matched against the bytes a person is left with.
const (
	huntRaceSaves  = 40
	huntRaceGap    = 25 * time.Millisecond
	huntRaceLedger = 8 // every 8th save is ~200 ms of saving
)

// huntRaceMark finds one save's marker: r-<subtest letter><two digits>.
var huntRaceMark = regexp.MustCompile(`r-[a-z][0-9][0-9]`)

// huntRaceTokenAttr finds the save token an open site hands its own page.
var huntRaceTokenAttr = regexp.MustCompile(`savetoken="([^"]+)"`)

// huntRaceSave is one save as the tab saw it: what it asked for, what the app
// answered, and whether the event this subtest races had already finished when
// the save went out.
type huntRaceSave struct {
	seq    int
	marker string
	status int
	body   string
	after  bool
	seen   []string // markers in the document right after this answer
}

// huntRaceExists reports whether rel, a root-relative name, is a file under dir.
func huntRaceExists(dir, rel string) bool {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

// huntRaceDoc is the document one save writes: servedTestDocument with this
// save's marker in it.
func huntRaceDoc(marker string) string {
	return strings.Replace(servedTestDocument, "<p>first</p>", `<p data-mark="`+marker+`">`+marker+`</p>`, 1)
}

func huntRaceMarker(prefix string, seq int) string {
	return fmt.Sprintf("r-%s%02d", prefix, seq)
}

// huntRaceFileMarks answers the distinct markers in one file, in the order they
// appear.
func huntRaceFileMarks(data []byte) []string {
	var marks []string
	seen := map[string]bool{}
	for _, mark := range huntRaceMark.FindAllString(string(data), -1) {
		if seen[mark] {
			continue
		}
		seen[mark] = true
		marks = append(marks, mark)
	}
	return marks
}

// huntRaceSeen reads every watched file and answers the markers in them, so a
// save that was not acknowledged but changed a document can be caught even when
// a later save overwrites it.
func huntRaceSeen(paths []string) []string {
	var marks []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		marks = append(marks, huntRaceFileMarks(data)...)
	}
	return marks
}

// huntRaceDrive is the one tab every subtest opens: it saves the document 40
// times, 25 ms apart, records each answer, and reports each save on the channel
// so the test can act between them. after is read before every save, so a save
// it flags went out after the subtest's event had finished. It never calls
// t.Fatal: the test checks the answers after the channel closes.
func huntRaceDrive(t *testing.T, prefix string, post func(seq int, marker string) (int, string), after *atomic.Bool, watch []string) <-chan huntRaceSave {
	events := make(chan huntRaceSave, huntRaceSaves)
	go func() {
		defer close(events)
		for seq := 0; seq < huntRaceSaves; seq++ {
			if seq > 0 {
				time.Sleep(huntRaceGap)
			}
			save := huntRaceSave{seq: seq, marker: huntRaceMarker(prefix, seq)}
			if after != nil {
				save.after = after.Load()
			}
			save.status, save.body = post(seq, save.marker)
			save.seen = huntRaceSeen(watch)
			if seq%huntRaceLedger == 0 {
				t.Logf("HUNT race2 ledger subtest=%s seq=%d marker=%s status=%d body=%s seen=%v",
					prefix, seq, save.marker, save.status, save.body, save.seen)
			}
			events <- save
		}
	}()
	return events
}

// huntRaceAcked is the set of markers whose save answered 200.
func huntRaceAcked(results []huntRaceSave) map[string]bool {
	acked := map[string]bool{}
	for _, save := range results {
		if save.status == http.StatusOK {
			acked[save.marker] = true
		}
	}
	return acked
}

// huntRaceRefused is the refusals by status, for the HUNT line.
func huntRaceRefused(results []huntRaceSave) string {
	counts := map[int]int{}
	for _, save := range results {
		if save.status == http.StatusOK {
			continue
		}
		counts[save.status]++
	}
	statuses := make([]int, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%d=%d", status, counts[status]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

// huntRaceScan walks every root and answers which files hold each save marker,
// plus every file that holds two saves at once.
func huntRaceScan(t *testing.T, roots []string) (map[string][]string, []string) {
	t.Helper()
	holds := map[string][]string{}
	var mixed []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			t.Fatalf("stat %s: %v", root, err)
		}
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
			marks := huntRaceFileMarks(data)
			for _, mark := range marks {
				holds[mark] = append(holds[mark], path)
			}
			if len(marks) > 1 {
				sort.Strings(marks)
				mixed = append(mixed, path+" holds "+strings.Join(marks, ","))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	sort.Strings(mixed)
	return holds, mixed
}

// huntRaceLost is the acknowledged markers no file holds: the saves a person was
// told were taken and that no document and no kept version has afterwards.
func huntRaceLost(acked map[string]bool, holds map[string][]string) []string {
	var lost []string
	for marker := range acked {
		if len(holds[marker]) == 0 {
			lost = append(lost, marker)
		}
	}
	sort.Strings(lost)
	return lost
}

// huntRaceSeenUnacked is every marker that was in a watched document even though
// its own save did not answer 200.
func huntRaceSeenUnacked(results []huntRaceSave, acked map[string]bool) []string {
	bad := map[string]bool{}
	for _, save := range results {
		for _, mark := range save.seen {
			if !acked[mark] {
				bad[mark] = true
			}
		}
	}
	marks := make([]string, 0, len(bad))
	for mark := range bad {
		marks = append(marks, mark)
	}
	sort.Strings(marks)
	return marks
}

// huntRaceStop stops a served folder, retrying while the service still holds
// unsaved edits, and answers how it went.
func huntRaceStop(t *testing.T, a *app, id string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	attempts := 0
	for {
		attempts++
		err := a.stopServing(id)
		if err == nil {
			return fmt.Sprintf("stopped-after-%d-attempts", attempts)
		}
		if time.Now().After(deadline) {
			t.Errorf("(b) a.stopServing(%s) never stopped in 5s after %d attempts: %v", id, attempts, err)
			return "never-stopped"
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// huntRaceToken is the save token the open site hands its own page, the way a
// tab's autosave gets it.
func huntRaceToken(t *testing.T, a *app, host, rel string) string {
	t.Helper()
	page := servedRequest(t, a, http.MethodGet, host, "/"+rel, "", nil)
	match := huntRaceTokenAttr.FindStringSubmatch(page.Body.String())
	if page.Code != http.StatusOK || match == nil {
		t.Fatalf("GET %s on the open site = %d with no save token (%s)", rel, page.Code, strings.TrimSpace(page.Body.String()))
	}
	return match[1]
}

// TestHuntSaveRaces autosaves a document while a person renames it, stops
// serving its folder, and starts serving that folder: no acknowledged save may
// be lost, and no file may hold a save the app did not acknowledge.
func TestHuntSaveRaces(t *testing.T) {
	t.Run("rename", func(t *testing.T) {
		serviceRepoTarget(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "page.html"), servedTestDocument)

		a := newServiceTestApp(t, home, t.TempDir())
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		board := openDashboard(t, a)
		id := board.rootIDOf()
		host := servedHostName("notes", servedTestPort)

		// What the folder looked like just before each save went out, which is
		// what tells a re-created page.html from a save that was refused.
		type before struct{ page, renamed bool }
		states := make([]before, huntRaceSaves)
		post := func(seq int, marker string) (int, string) {
			_, pageErr := os.Stat(filepath.Join(dir, "page.html"))
			_, renamedErr := os.Stat(filepath.Join(dir, "renamed.html"))
			states[seq] = before{page: pageErr == nil, renamed: renamedErr == nil}
			rec := servedRequest(t, a, http.MethodPost, host, "/_/save", huntRaceDoc(marker), map[string]string{
				"Document-URL": "http://" + host + "/page.html",
				"Content-Type": "text/html",
			})
			return rec.Code, strings.TrimSpace(rec.Body.String())
		}

		events := huntRaceDrive(t, "n", post, nil, []string{
			filepath.Join(dir, "page.html"),
			filepath.Join(dir, "renamed.html"),
		})
		results := make([]huntRaceSave, huntRaceSaves)
		renames := "ok"
		for save := range events {
			results[save.seq] = save
			switch save.seq {
			case 5:
				if status, body := board.command(id, map[string]any{"command": "rename", "path": "page.html", "name": "renamed.html"}); status != http.StatusOK {
					t.Errorf("(a) rename page.html to renamed.html = %d %s, want 200", status, body)
					renames = "refused"
				}
			case 20:
				if status, body := board.command(id, map[string]any{"command": "rename", "path": "renamed.html", "name": "page.html"}); status != http.StatusOK {
					t.Errorf("(a) rename renamed.html back to page.html = %d %s, want 200", status, body)
					renames = "refused"
				}
			}
		}

		acked := huntRaceAcked(results)
		if len(acked) == 0 {
			t.Errorf("(z) not one of the %d saves answered 200, so this subtest watched nothing: refused=%s", huntRaceSaves, huntRaceRefused(results))
		}
		holds, mixed := huntRaceScan(t, []string{dir, a.rt.versions.BaseDir()})
		lost := huntRaceLost(acked, holds)
		if len(lost) > 0 {
			t.Errorf("(a) %d of the %d acknowledged saves are in no document and in no version: %v", len(lost), len(acked), lost)
		}
		for _, entry := range mixed {
			t.Errorf("(a) one file holds two saves: %s", entry)
		}
		for _, mark := range huntRaceSeenUnacked(results, acked) {
			t.Errorf("(a) %s was in the document although its save was refused, so a save that was not acknowledged changed the file", mark)
		}
		for _, rel := range []string{"page.html", "renamed.html"} {
			data, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				continue // the name the last rename took the document away from
			}
			for _, mark := range huntRaceFileMarks(data) {
				if !acked[mark] {
					t.Errorf("(a) %s holds %s, a save that was not acknowledged", rel, mark)
				}
			}
		}

		recreated := 0
		for seq, save := range results {
			if save.status != http.StatusOK {
				continue
			}
			if !states[seq].page && states[seq].renamed {
				recreated++
			}
		}
		t.Logf("HUNT race2 subtest=rename acked=%d refused=%s lost=%d extra=recreated=%d renames=%s page-exists=%v renamed-exists=%v",
			len(acked), huntRaceRefused(results), len(lost), recreated, renames,
			huntRaceExists(dir, "page.html"), huntRaceExists(dir, "renamed.html"))
	})

	t.Run("stop", func(t *testing.T) {
		serviceRepoTarget(t)
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

		a := newServiceTestApp(t, home, t.TempDir())
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		board := openDashboard(t, a)
		id := board.rootIDOf()
		host := servedHostName("notes", servedTestPort)

		var stopped atomic.Bool
		post := func(seq int, marker string) (int, string) {
			rec := servedRequest(t, a, http.MethodPost, host, "/_/save", huntRaceDoc(marker), map[string]string{
				"Document-URL": "http://" + host + "/index.html",
				"Content-Type": "text/html",
			})
			return rec.Code, strings.TrimSpace(rec.Body.String())
		}

		events := huntRaceDrive(t, "s", post, &stopped, []string{filepath.Join(dir, "index.html")})
		results := make([]huntRaceSave, huntRaceSaves)
		stop := "not-run"
		for save := range events {
			results[save.seq] = save
			if save.seq == 15 {
				stop = huntRaceStop(t, a, id)
				stopped.Store(true)
			}
		}

		acked := huntRaceAcked(results)
		if len(acked) == 0 {
			t.Errorf("(z) not one of the %d saves answered 200, so this subtest watched nothing: refused=%s", huntRaceSaves, huntRaceRefused(results))
		}
		afterStop := 0
		for _, save := range results {
			if !save.after {
				continue
			}
			afterStop++
			if save.status == http.StatusOK {
				t.Errorf("(b) %s was saved after the stop finished and answered 200: %s", save.marker, save.body)
			}
		}

		holds, mixed := huntRaceScan(t, []string{dir, a.rt.versions.BaseDir()})
		lost := huntRaceLost(acked, holds)
		if len(lost) > 0 {
			t.Errorf("(b) %d of the %d acknowledged saves are in neither index.html nor a version: %v", len(lost), len(acked), lost)
		}
		for _, entry := range mixed {
			t.Errorf("(b) one file holds two saves: %s", entry)
		}
		for _, mark := range huntRaceSeenUnacked(results, acked) {
			t.Errorf("(b) %s was in index.html although its save was refused, so a save that was not acknowledged changed the file", mark)
		}

		order := map[string]int{}
		lastAcked := -1
		for _, save := range results {
			order[save.marker] = save.seq
			if save.status == http.StatusOK {
				lastAcked = save.seq
			}
		}
		disk := "missing"
		data, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Errorf("(b) index.html after the stop: %v", err)
		} else if marks := huntRaceFileMarks(data); len(marks) == 0 {
			disk = "unchanged"
		} else {
			disk = strings.Join(marks, ",")
			for _, mark := range marks {
				if !acked[mark] {
					t.Errorf("(b) index.html holds %s, a save that was not acknowledged", mark)
					continue
				}
				if order[mark] > lastAcked {
					t.Errorf("(b) index.html holds %s, which answered 200 after the last acknowledged save", mark)
				}
			}
		}
		t.Logf("HUNT race2 subtest=stop acked=%d refused=%s lost=%d extra=stop=%s after-stop-saves=%d after-stop-acked=%d disk=%s",
			len(acked), huntRaceRefused(results), len(lost), stop, afterStop, huntRaceAfterAcked(results), disk)
	})

	t.Run("handover", func(t *testing.T) {
		serviceRepoTarget(t)
		home := evalTempHome(t)
		proj := filepath.Join(home, "proj")
		page := filepath.Join(proj, "index.htmlclay")
		writeTestFile(t, page, servedTestDocument)

		a := newServiceTestApp(t, home, t.TempDir())
		if err := a.trustFolder(proj); err != nil {
			t.Fatalf("trustFolder: %v", err)
		}
		s, rel := a.openForTest(t, page)
		oldHost, oldURL := s.host, s.url(rel)
		token := huntRaceToken(t, a, oldHost, rel)

		var handed atomic.Bool
		post := func(seq int, marker string) (int, string) {
			rec := servedRequest(t, a, http.MethodPost, oldHost, "/_/save/"+token, huntRaceDoc(marker), map[string]string{
				"Origin":         "http://" + oldHost,
				"Sec-Fetch-Site": "same-origin",
				"Content-Type":   "text/html",
			})
			return rec.Code, strings.TrimSpace(rec.Body.String())
		}

		events := huntRaceDrive(t, "h", post, &handed, []string{page})
		results := make([]huntRaceSave, huntRaceSaves)
		handover := "not-run"
		for save := range events {
			results[save.seq] = save
			if save.seq == 10 {
				_, refusal, err := a.serveFolder(proj, "proj")
				switch {
				case err != nil || refusal != nil:
					t.Errorf("(c) a.serveFolder(%s, proj) = refusal %+v err %v", proj, refusal, err)
					handover = "refused"
				default:
					handover = "served"
				}
				handed.Store(true)
			}
		}

		acked := huntRaceAcked(results)
		if len(acked) == 0 {
			t.Errorf("(z) not one of the %d saves through the open site answered 200, so this subtest watched nothing: refused=%s", huntRaceSaves, huntRaceRefused(results))
		}
		afterHandover := 0
		for _, save := range results {
			if !save.after {
				continue
			}
			afterHandover++
			if save.status == http.StatusOK {
				t.Errorf("(c) %s was saved through the old address after serveFolder returned and answered 200: %s", save.marker, save.body)
			}
		}

		// The same address a document-URL client uses, the request a tab left
		// open keeps sending, must not reach the file any more either.
		oldSave := servedRequest(t, a, http.MethodPost, oldHost, "/_/save", huntRaceDoc("r-h98"), map[string]string{
			"Document-URL": oldURL,
			"Content-Type": "text/html",
		})
		if oldSave.Code == http.StatusOK {
			t.Errorf("(c) a Document-URL save to the old address after serveFolder answered 200: %s", strings.TrimSpace(oldSave.Body.String()))
		}

		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
		servedHost := servedHostName("proj", servedTestPort)
		servedMarker := "r-h99"
		postServed := func() (int, string) {
			rec := servedRequest(t, a, http.MethodPost, servedHost, "/_/save", huntRaceDoc(servedMarker), map[string]string{
				"Document-URL": "http://" + servedHost + "/index.htmlclay",
				"Content-Type": "text/html",
			})
			return rec.Code, strings.TrimSpace(rec.Body.String())
		}
		status, body := postServed()
		firstStatus, firstBody := status, body
		servedDeadline := time.Now().Add(5 * time.Second)
		for status == http.StatusServiceUnavailable && strings.Contains(body, "unconfirmed") && time.Now().Before(servedDeadline) {
			time.Sleep(25 * time.Millisecond)
			status, body = postServed()
		}
		if firstStatus != status {
			t.Logf("(c) the save on the new served address was first refused with %d %s and answered %d after the retry", firstStatus, firstBody, status)
		}
		if status != http.StatusOK {
			t.Errorf("(c) POST /_/save on the new served address = %d %s, want 200", status, body)
		} else {
			acked[servedMarker] = true
		}
		if onDisk, err := os.ReadFile(page); err != nil {
			t.Errorf("(c) read %s: %v", page, err)
		} else if !strings.Contains(string(onDisk), servedMarker) {
			t.Errorf("(c) %s does not hold the save the new served address took: %q", page, onDisk)
		}

		holds, mixed := huntRaceScan(t, []string{proj, a.rt.versions.BaseDir()})
		lost := huntRaceLost(acked, holds)
		if len(lost) > 0 {
			t.Errorf("(c) %d of the %d acknowledged saves are in neither the document nor a version: %v", len(lost), len(acked), lost)
		}
		for _, entry := range mixed {
			t.Errorf("(c) one file holds two saves: %s", entry)
		}
		for _, mark := range huntRaceSeenUnacked(results, acked) {
			t.Errorf("(c) %s was in the document although its save through the old address was refused, so a save that was not acknowledged changed the file", mark)
		}
		if onDisk, err := os.ReadFile(page); err == nil {
			for _, mark := range huntRaceFileMarks(onDisk) {
				if !acked[mark] {
					t.Errorf("(c) index.htmlclay holds %s, a save that was not acknowledged", mark)
				}
			}
		}
		t.Logf("HUNT race2 subtest=handover acked=%d refused=%s lost=%d extra=handover=%s after-handover-saves=%d after-handover-acked=%d served-save=%d old-address-save=%d",
			len(acked), huntRaceRefused(results), len(lost), handover, afterHandover, huntRaceAfterAcked(results), status, oldSave.Code)
	})
}

// huntRaceAfterAcked counts the saves that answered 200 after the subtest's
// event had finished: the number that must be zero.
func huntRaceAfterAcked(results []huntRaceSave) int {
	acked := 0
	for _, save := range results {
		if save.after && save.status == http.StatusOK {
			acked++
		}
	}
	return acked
}
