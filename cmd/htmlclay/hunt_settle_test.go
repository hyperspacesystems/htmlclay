//go:build managedtrial

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// huntSettleLostCase is one workspace command whose settle is lost on purpose.
// newPlace is the one other place the document may end up; duplicate is the one
// command that leaves the original where it was and may leave a copy beside it.
type huntSettleLostCase struct {
	name      string
	body      map[string]any
	newPlace  string
	duplicate bool
}

var huntSettleLostCases = []huntSettleLostCase{
	{name: "rename", body: map[string]any{"command": "rename", "path": "page.html", "name": "renamed.html"}, newPlace: "renamed.html"},
	{name: "move", body: map[string]any{"command": "move", "path": "page.html", "parent": "shelf"}, newPlace: "shelf/page.html"},
	{name: "trash", body: map[string]any{"command": "trash", "path": "page.html"}, newPlace: ".trash/page.html"},
	{name: "duplicate", body: map[string]any{"command": "duplicate", "path": "page.html", "name": "copy.html"}, newPlace: "copy.html", duplicate: true},
}

// huntFault posts one body to the app's control listener's fault route and
// answers the numbers the route reports.
func huntFault(t *testing.T, a *app, body map[string]string) map[string]int {
	t.Helper()
	a.service.mu.Lock()
	controlURL, secret := a.service.controlURL, a.service.controlSecret
	a.service.mu.Unlock()
	if controlURL == "" {
		t.Fatalf("the app has no control listener, so %v reaches no fault route", body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, controlURL+"/v1/fault", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /v1/fault %v: %v", body, err)
	}
	defer response.Body.Close()
	answerBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	answer := map[string]int{}
	if response.StatusCode != http.StatusOK || json.Unmarshal(answerBody, &answer) != nil {
		t.Fatalf("POST /v1/fault %v = %d %s", body, response.StatusCode, answerBody)
	}
	return answer
}

// huntExists reports whether rel, a root-relative name, is a file under dir.
func huntExists(dir, rel string) bool {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

// huntFilesHolding walks dir, skips Go's own .hyperclay folder, and answers every
// root-relative file whose bytes equal want.
func huntFilesHolding(t *testing.T, dir string, want []byte) []string {
	t.Helper()
	var holding []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != dir && entry.Name() == ".hyperclay" {
				return filepath.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(data, want) {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		holding = append(holding, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return holding
}

// huntPlacement answers where the document is now, and reports every place it is
// not allowed to be: the old name, the expected new name, or the Trash, each at
// most once.
func huntPlacement(t *testing.T, dir string, want []byte, c huntSettleLostCase) string {
	t.Helper()
	holding := huntFilesHolding(t, dir, want)
	if c.duplicate {
		if !huntExists(dir, "page.html") {
			t.Errorf("(a) exactly one place: the original page.html is gone; its bytes are at %v", holding)
			return ""
		}
		var copies []string
		for _, rel := range holding {
			if rel != "page.html" {
				copies = append(copies, rel)
			}
		}
		if len(copies) > 1 {
			t.Errorf("(a) exactly one place: page.html has %d copies at %v, want at most one", len(copies), copies)
		}
		for _, rel := range copies {
			if rel != c.newPlace {
				t.Errorf("(a) exactly one place: the duplicate is at %q, want %s", rel, c.newPlace)
			}
		}
		if len(copies) == 1 {
			return "page.html+" + copies[0]
		}
		return "page.html"
	}

	places := []string{"page.html", c.newPlace}
	if c.name == "trash" {
		entries, err := os.ReadDir(filepath.Join(dir, ".trash"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read .trash: %v", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				places = append(places, ".trash/"+entry.Name())
			}
		}
	}
	seen := map[string]bool{}
	var present []string
	for _, rel := range places {
		if seen[rel] {
			continue
		}
		seen[rel] = true
		if huntExists(dir, rel) {
			present = append(present, rel)
		}
	}
	if len(present) != 1 {
		t.Errorf("(a) exactly one place: after %s the document is at %v (its bytes are at %v), want exactly one of page.html, %s or .trash/",
			c.name, present, holding, c.newPlace)
	}
	for _, rel := range holding {
		if rel == "page.html" || rel == c.newPlace || strings.HasPrefix(rel, ".trash/") {
			continue
		}
		t.Errorf("(a) exactly one place: another copy of the document's bytes is at %q", rel)
	}
	if len(present) == 0 {
		return ""
	}
	return present[0]
}

// huntBytes checks the placed document still holds exactly the bytes written.
func huntBytes(t *testing.T, dir string, want []byte, c huntSettleLostCase, placed string) {
	t.Helper()
	target := placed
	if target == "" || strings.HasPrefix(target, "page.html+") {
		target = "page.html"
	}
	got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(target)))
	if err != nil {
		t.Errorf("(b) the bytes are unchanged: reading %s: %v", target, err)
	} else if !bytes.Equal(got, want) {
		t.Errorf("(b) the bytes are unchanged: %s holds %q, want the %d bytes written", target, got, len(want))
	}
	if c.duplicate && huntExists(dir, c.newPlace) {
		copied, copyErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.newPlace)))
		if copyErr != nil || !bytes.Equal(copied, want) {
			t.Errorf("(b) the bytes are unchanged: the copy %s holds %q, %v, want the %d bytes written", c.newPlace, copied, copyErr, len(want))
		}
	}
}

// huntNextCommand renames another document through the workspace and answers
// "ok" when the command and the file move both went through.
func huntNextCommand(t *testing.T, board *dashboardClient, id, dir string) string {
	t.Helper()
	status, body := board.command(id, map[string]any{"command": "rename", "path": "shelf/kept.html", "name": "kept2.html"})
	moved := !huntExists(dir, "shelf/kept.html") && huntExists(dir, "shelf/kept2.html")
	if status != http.StatusOK {
		t.Errorf("(c) the next command: rename shelf/kept.html = %d %s, want 200", status, body)
	}
	if !moved {
		t.Errorf("(c) the next command: after rename shelf/kept.html, shelf/kept.html exists %v and shelf/kept2.html exists %v",
			huntExists(dir, "shelf/kept.html"), huntExists(dir, "shelf/kept2.html"))
	}
	if status != http.StatusOK || !moved {
		return strconv.Itoa(status)
	}
	return "ok"
}

// huntSaveName is the document the next save goes to: the moved one, or the
// original for a duplicate. A trashed document sits under .trash/, which the
// served routes do not address, so the surviving document is saved instead.
func huntSaveName(c huntSettleLostCase, placed string) string {
	if c.duplicate {
		return "page.html"
	}
	if placed == "page.html" || (placed == c.newPlace && !strings.HasPrefix(placed, ".trash/")) {
		return placed
	}
	if placed == "" {
		return "page.html"
	}
	return "shelf/kept2.html"
}

// huntNextSave saves a document through its current name and answers "ok" when
// the save was taken and the file holds the new bytes. A document Go marked
// unconfirmed before the service reconciled it is refused as retryable, so that
// one refusal is retried, and only that one.
func huntNextSave(t *testing.T, a *app, dir, rel string) string {
	t.Helper()
	host := servedHostName("notes", servedTestPort)
	saved := strings.Replace(servedTestDocument, "<p>first</p>", "<p>second</p>", 1)
	post := func() (int, string) {
		rec := servedRequest(t, a, http.MethodPost, host, "/_/save", saved, map[string]string{
			"Document-URL": "http://" + host + "/" + rel,
			"Content-Type": "text/html",
		})
		return rec.Code, rec.Body.String()
	}
	status, body := post()
	firstStatus, firstBody := status, body
	deadline := time.Now().Add(5 * time.Second)
	for status == http.StatusServiceUnavailable && strings.Contains(body, "unconfirmed") && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		status, body = post()
	}
	if firstStatus != status {
		t.Logf("(d) the next save of %s was first refused with %d %s and answered %d after the retry", rel, firstStatus, firstBody, status)
	}
	if status != http.StatusOK {
		t.Errorf("(d) the next save: POST /_/save of %s = %d %s, want 200", rel, status, body)
		return strconv.Itoa(status)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil || !strings.Contains(string(onDisk), "<p>second</p>") {
		t.Errorf("(d) the next save: %s on disk = %q, %v, want the saved bytes", rel, onDisk, err)
		return "200"
	}
	return "ok"
}

// TestHuntSettleLost loses the settle of each workspace command and asks what a
// person is left with: the document in exactly one place, its bytes intact, and
// the next command and the next save still working.
func TestHuntSettleLost(t *testing.T) {
	ran := 0
	for _, testCase := range huntSettleLostCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			ran++
			serviceRepoTarget(t)
			home := evalTempHome(t)
			dir := filepath.Join(home, "notes")
			want := []byte(servedTestDocument)
			writeTestFile(t, filepath.Join(dir, "page.html"), servedTestDocument)
			writeTestFile(t, filepath.Join(dir, "shelf", "kept.html"), strings.Replace(servedTestDocument, "<p>first</p>", "<p>kept</p>", 1))
			writeTestFile(t, pageStateFile(dir, "page.html"), `{"v":1,"rev":"abc","tag":"tag"}`)
			writeTestFile(t, pageStateFile(dir, "shelf/kept.html"), `{"v":1,"rev":"def","tag":"tag"}`)

			a := newServiceTestApp(t, home, t.TempDir())
			serveThroughDo(t, a, "serve-1", dir, "notes")
			waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
			board := openDashboard(t, a)
			id := board.rootIDOf()

			if armed := huntFault(t, a, map[string]string{"point": "settle-lost", "rel": "page.html"})["armed"]; armed != 1 {
				t.Fatalf("arming settle-lost for page.html answered armed %d, want 1", armed)
			}

			status, body := board.command(id, testCase.body)
			t.Logf("%s: %v answered %d %s", testCase.name, testCase.body["command"], status, body)

			if left := huntFault(t, a, map[string]string{"point": "remaining", "rel": "page.html"})["remaining"]; left != 0 {
				t.Errorf("the settle-lost fault armed for page.html was not taken (%d uses left), so this %s never lost a settle", left, testCase.name)
			}

			placed := huntPlacement(t, dir, want, testCase)
			huntBytes(t, dir, want, testCase, placed)
			nextCommand := huntNextCommand(t, board, id, dir)
			nextSave := huntNextSave(t, a, dir, huntSaveName(testCase, placed))
			t.Logf("HUNT settle command=%s status=%d placed=%s next-command=%s next-save=%s",
				testCase.name, status, placed, nextCommand, nextSave)
		})
	}
	if ran < 4 {
		t.Fatalf("only %d of the four subtests ran", ran)
	}
}
