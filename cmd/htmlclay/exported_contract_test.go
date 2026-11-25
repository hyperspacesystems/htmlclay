package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/testutil"
)

// transcriptProgram is the program the recorded documents declare.
const transcriptProgram = "search"

// transcriptHost answers the host the transcript records: the second launch on a
// home whose first launch, which nobody recorded, did what milestone 3 made real
// through the actions the Mac app sends. It trusted notes, writing and the drafts
// folder inside writing, registered a program, opened one document and allowed
// the program for it. The restart is what shows the folder ids, the recent row
// and the program's permissions are kept. asking is the document whose page asks
// for the program while the stream is recorded.
func transcriptHost(t *testing.T, home, notes string) (host *app, asking string) {
	t.Helper()
	settledSetup(t)
	writing := filepath.Join(home, "writing")
	drafts := filepath.Join(writing, "drafts")
	programs := filepath.Join(home, "apps", "bin")
	for _, dir := range []string{drafts, programs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	allowed := writeHelperPageNamed(t, writing, "todo", transcriptProgram)
	asking = writeHelperPageNamed(t, drafts, "essay", transcriptProgram)
	program := writeTestProgram(t, programs, transcriptProgram, 0o755)

	cfgBase, configDir := t.TempDir(), t.TempDir()
	tools := t.TempDir()
	for _, name := range []string{"claude", "codex", "agy"} {
		writeTestProgram(t, tools, name, 0o755)
	}
	launch := func() *app {
		a := buildTestApp(t, home, cfgBase)
		a.aiToolsEnv = []string{"PATH=" + tools}
		a.rt.configDir = configDir
		a.rt.port = servedTestPort
		a.rt.launchBrowser = func(string) error { return nil }
		a.shell = true
		a.startServed()
		t.Cleanup(a.shutdown)
		return a
	}

	first := launch()
	sent := 0
	send := func(actionType string, args any) {
		t.Helper()
		sent++
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		action := appcore.Action{ID: fmt.Sprintf("before-%d", sent), Type: actionType, Args: raw}
		if _, refusal := first.do(context.Background(), action, ChannelShell); refusal != nil {
			t.Fatalf("%s before the restart refused: %+v", actionType, refusal)
		}
	}
	send("folder.trust", map[string]any{"path": notes})
	send("folder.trust", map[string]any{"path": writing})
	send("folder.trust", map[string]any{"path": drafts, "acknowledged": []string{warnInsideTrusted}})
	send("program.add", map[string]any{"name": transcriptProgram, "path": program, "acknowledged": []string{}})
	send("file.open", map[string]any{"path": allowed})
	asked := startSetup(t, first, allowed, transcriptProgram)
	request := waitForSetupRequest(t, first, appcore.RequestFound)
	if err := answerSetup(t, first, "before-allow", request, helperAnswerAllowDocument, ""); err != nil {
		t.Fatal(err)
	}
	testutil.Receive(t, transcriptTimeout, "the setup flow before the restart", asked)
	first.shutdown()

	host = launch()
	// The page's question is the stream's first change, so the watcher's first
	// look at the facts must not be.
	host.core.watchEvery = time.Hour
	return host, asking
}

// storedSnapshots reads the stored transcript: its lines, and the snapshot each
// snapshot line carries.
func storedSnapshots(t *testing.T) ([]string, []appcore.Snapshot) {
	t.Helper()
	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	var snapshots []appcore.Snapshot
	for i, line := range lines {
		var event struct {
			Type     string           `json:"type"`
			Snapshot appcore.Snapshot `json:"snapshot"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d of %s does not decode: %v", i+1, transcriptPath, err)
		}
		if event.Type == "snapshot" {
			snapshots = append(snapshots, event.Snapshot)
		}
	}
	return lines, snapshots
}

// snapshotAtRev is the stored snapshot with one revision.
func snapshotAtRev(t *testing.T, snapshots []appcore.Snapshot, rev uint64) appcore.Snapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.Rev == rev {
			return snapshot
		}
	}
	t.Fatalf("%s holds no snapshot at revision %d", transcriptPath, rev)
	return appcore.Snapshot{}
}

// transcriptDifference names the first place two decoded lines differ.
func transcriptDifference(path string, stored, sent any) string {
	switch want := stored.(type) {
	case map[string]any:
		got, ok := sent.(map[string]any)
		if !ok {
			break
		}
		keys := make([]string, 0, len(want)+len(got))
		for key := range want {
			keys = append(keys, key)
		}
		for key := range got {
			if _, both := want[key]; !both {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		for _, key := range keys {
			if difference := transcriptDifference(path+"."+key, want[key], got[key]); difference != "" {
				return difference
			}
		}
		return ""
	case []any:
		got, ok := sent.([]any)
		if !ok || len(got) != len(want) {
			break
		}
		for i := range want {
			if difference := transcriptDifference(fmt.Sprintf("%s[%d]", path, i), want[i], got[i]); difference != "" {
				return difference
			}
		}
		return ""
	}
	if reflect.DeepEqual(stored, sent) {
		return ""
	}
	return fmt.Sprintf("%s is %v in the stored transcript and the host sends %v", path, stored, sent)
}

var (
	contractFolderID   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	contractProgramID  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	contractRequestID  = regexp.MustCompile(`^r[0-9a-f]{12}$`)
	contractDocumentID = regexp.MustCompile(`^d[0-9a-f]{15}$`)
)

// TestM3ExportedContract proves step 7.47: the transcript the Mac app's decoding
// tests read carries every shape milestone 3 added to the snapshot, and it is
// what the real host sends today. With -update the exporter it runs first records
// the transcript again; scripts/export-shell-fixtures.sh then copies it to the
// Swift tests.
func TestM3ExportedContract(t *testing.T) {
	t.Run("the exporter records the transcript, or checks the stored one and its Swift copy", TestShellEventsTranscript)

	lines, snapshots := storedSnapshots(t)
	notes := transcriptHome + "/notes"
	writing := transcriptHome + "/writing"
	drafts := writing + "/drafts"
	allowed := writing + "/todo.htmlclay"
	asking := drafts + "/essay.htmlclay"
	program := transcriptHome + "/apps/bin/" + transcriptProgram

	t.Run("the stream is the five revisions the Mac app's stream tests name, then bye", func(t *testing.T) {
		var revs []uint64
		for _, snapshot := range snapshots {
			revs = append(revs, snapshot.Rev)
			if snapshot.Epoch != transcriptEpoch {
				t.Fatalf("revision %d has epoch %q, want %q", snapshot.Rev, snapshot.Epoch, transcriptEpoch)
			}
		}
		if want := []uint64{0, 1, 3, 5, 7}; !slices.Equal(revs, want) {
			t.Fatalf("the snapshot revisions are %v, want %v: HostStreamTests and LoopbackHostClientTests name them", revs, want)
		}
		if last := lines[len(lines)-1]; len(lines) != 6 || last != `{"reason":"quit","type":"bye"}` {
			t.Fatalf("the transcript is %d lines ending %s, want 6 ending with the quit bye", len(lines), last)
		}
	})

	t.Run("the Mac app's host offers every action it runs", func(t *testing.T) {
		for _, snapshot := range snapshots {
			for _, action := range implementedActions {
				// A host the Mac app started leaves these to the app: Sparkle
				// checks for updates, and the app sets its own file types.
				switch action {
				case "update.check", "fileTypes.makeDefault", "fileTypes.stopOpening", "fileTypes.setHTMLOpenWith":
					continue
				}
				if !snapshot.Host.Can[action] {
					t.Fatalf("revision %d says the host cannot run %s", snapshot.Rev, action)
				}
			}
		}
	})

	t.Run("a request carries its answers until it is answered", func(t *testing.T) {
		if before := snapshotAtRev(t, snapshots, 0); len(before.Requests) != 0 {
			t.Fatalf("revision 0 already holds %d request(s)", len(before.Requests))
		}
		asked := snapshotAtRev(t, snapshots, 1)
		if len(asked.Requests) != 1 {
			t.Fatalf("revision 1 holds %d request(s), want the page's one question", len(asked.Requests))
		}
		request := asked.Requests[0]
		if want := []string{helperAnswerDeny, helperAnswerAllowDocument, helperAnswerAllowAllTrusted, helperAnswerChooseFile}; !slices.Equal(request.Answers, want) {
			t.Fatalf("the request offers %v, want %v", request.Answers, want)
		}
		if !contractRequestID.MatchString(request.ID) || request.Kind != appcore.RequestFound || request.Revision != 1 {
			t.Fatalf("the request is %+v, want a found question at revision 1", request)
		}
		if request.Document != asking || request.Program != transcriptProgram || request.Path != program {
			t.Fatalf("the request names %q, %q and %q, want %q, %q and %q", request.Document, request.Program, request.Path, asking, transcriptProgram, program)
		}
		if request.DocumentID == nil || *request.DocumentID == "" {
			t.Fatal("the request does not carry its document's identity")
		}
		for _, rev := range []uint64{3, 5} {
			if later := snapshotAtRev(t, snapshots, rev).Requests; !reflect.DeepEqual(later, asked.Requests) {
				t.Fatalf("revision %d holds %+v, want the same open request", rev, later)
			}
		}
		if answered := snapshotAtRev(t, snapshots, 7); len(answered.Requests) != 0 {
			t.Fatalf("the answered request is still listed: %+v", answered.Requests)
		}
	})

	t.Run("a waiting row names the document that asks, not its path", func(t *testing.T) {
		asked := snapshotAtRev(t, snapshots, 1)
		if len(asked.Waiting) != 1 || len(asked.Requests) != 1 {
			t.Fatalf("revision 1 holds %d waiting row(s) and %d request(s), want one of each", len(asked.Waiting), len(asked.Requests))
		}
		row := asked.Waiting[0]
		if want := fmt.Sprintf(appcore.TextWaitingProgramRequest, "essay.htmlclay", transcriptProgram); row.Title != want {
			t.Fatalf("the waiting row reads %q, want %q", row.Title, want)
		}
		if row.Kind != appcore.WaitingProgramRequest || row.SubjectID == nil || *row.SubjectID != asked.Requests[0].ID {
			t.Fatalf("the waiting row is %+v, want a program request about %s", row, asked.Requests[0].ID)
		}
		if !asked.Counts.Dot || asked.Counts.ByPane[appcore.PanePrograms] != 1 {
			t.Fatalf("the counts are %+v, want one item in programs and the dot", asked.Counts)
		}
		if answered := snapshotAtRev(t, snapshots, 7); len(answered.Waiting) != 0 || answered.Counts.Dot {
			t.Fatalf("the answered request still waits: %+v, %+v", answered.Waiting, answered.Counts)
		}
	})

	t.Run("a program carries its facts", func(t *testing.T) {
		for _, snapshot := range snapshots {
			if len(snapshot.Programs) != 1+len(builtin.IDs) {
				t.Fatalf("revision %d lists %d program(s), want the registered one and the built-in programs", snapshot.Rev, len(snapshot.Programs))
			}
			got := snapshot.Programs[len(builtin.IDs)]
			if !contractProgramID.MatchString(got.ID) || got.Name != transcriptProgram || got.Path != program || got.FileMissing {
				t.Fatalf("revision %d lists the program as %+v", snapshot.Rev, got)
			}
			if !slices.Equal(got.AllowedDocuments, []string{allowed}) || got.AllTrustedFolders {
				t.Fatalf("revision %d says the program may run for %v (all trusted folders: %v), want only %s", snapshot.Rev, got.AllowedDocuments, got.AllTrustedFolders, allowed)
			}
			if got.RemovedDocuments == nil || len(got.RemovedDocuments) != 0 {
				t.Fatalf("revision %d lists removed documents as %v, want an empty list", snapshot.Rev, got.RemovedDocuments)
			}
		}
	})

	t.Run("a recent document is listed after a restart", func(t *testing.T) {
		for _, snapshot := range snapshots {
			if len(snapshot.Documents) != 1 {
				t.Fatalf("revision %d lists %d document(s), want the one opened before the restart", snapshot.Rev, len(snapshot.Documents))
			}
			row := snapshot.Documents[0]
			if !contractDocumentID.MatchString(row.ID) || row.ID != recentID(allowed) {
				t.Fatalf("revision %d gives the row the id %q, want %q", snapshot.Rev, row.ID, recentID(allowed))
			}
			if row.Name != "todo.htmlclay" || row.Place != "~/writing" || row.OpenNow || row.URL != "" || row.FileMissing {
				t.Fatalf("revision %d lists the row as %+v, want a closed todo.htmlclay in ~/writing", snapshot.Rev, row)
			}
			if row.OpenedAt == nil {
				t.Fatalf("revision %d does not say when the document was opened", snapshot.Rev)
			}
			if folder := snapshotFolderAt(t, snapshot, writing); row.FolderID == nil || *row.FolderID != folder.ID {
				t.Fatalf("revision %d puts the row in folder %v, want %s", snapshot.Rev, row.FolderID, folder.ID)
			}
		}
	})

	t.Run("a folder keeps one id", func(t *testing.T) {
		ids := map[string]string{}
		for _, snapshot := range snapshots {
			if len(snapshot.Folders) != 3 {
				t.Fatalf("revision %d lists %d folder(s), want 3", snapshot.Rev, len(snapshot.Folders))
			}
			for _, folder := range snapshot.Folders {
				if !contractFolderID.MatchString(folder.ID) {
					t.Fatalf("revision %d gives %s the id %q, want 16 hex characters", snapshot.Rev, folder.Path, folder.ID)
				}
				if first, seen := ids[folder.Path]; seen && first != folder.ID {
					t.Fatalf("%s is %s and then %s at revision %d", folder.Path, first, folder.ID, snapshot.Rev)
				}
				ids[folder.Path] = folder.ID
			}
		}
		if ids[notes] == ids[writing] || ids[writing] == ids[drafts] || ids[notes] == ids[drafts] {
			t.Fatalf("two folders share an id: %v", ids)
		}
		before, after := snapshotFolderAt(t, snapshotAtRev(t, snapshots, 1), notes), snapshotFolderAt(t, snapshotAtRev(t, snapshots, 3), notes)
		if before.Served || !after.Served || after.Address == "" {
			t.Fatalf("notes is served %v and then %v at %q, want it served from revision 3", before.Served, after.Served, after.Address)
		}
	})

	t.Run("a served folder says whether other sites may frame it", func(t *testing.T) {
		served := snapshotFolderAt(t, snapshotAtRev(t, snapshots, 3), notes)
		if served.AllowsFraming || !served.Can.SetFraming {
			t.Fatalf("the folder just served is %+v, want framing off and offered", served)
		}
		for _, rev := range []uint64{5, 7} {
			if !snapshotFolderAt(t, snapshotAtRev(t, snapshots, rev), notes).AllowsFraming {
				t.Fatalf("revision %d does not say notes may be framed", rev)
			}
		}
		for _, snapshot := range snapshots {
			for _, path := range []string{writing, drafts} {
				if folder := snapshotFolderAt(t, snapshot, path); folder.AllowsFraming || folder.Can.SetFraming {
					t.Fatalf("revision %d offers framing for %s, which is not served", snapshot.Rev, path)
				}
			}
		}
	})

	t.Run("a folder trusted inside a trusted folder names the folder around it", func(t *testing.T) {
		for _, snapshot := range snapshots {
			around, inside := snapshotFolderAt(t, snapshot, writing), snapshotFolderAt(t, snapshot, drafts)
			if inside.ParentID == nil || *inside.ParentID != around.ID {
				t.Fatalf("revision %d gives drafts the parent %v, want %s", snapshot.Rev, inside.ParentID, around.ID)
			}
			if !inside.Can.Merge {
				t.Fatalf("revision %d does not offer to merge drafts into writing", snapshot.Rev)
			}
			if around.ParentID != nil || snapshotFolderAt(t, snapshot, notes).ParentID != nil {
				t.Fatalf("revision %d gives a folder that is inside no other a parent", snapshot.Rev)
			}
		}
	})

	t.Run("the host sends the stored transcript today", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("the transcript is the Mac app's host's stream, and it is recorded on a Mac")
		}
		sent := recordShellEvents(t, evalTempHome(t))
		if len(sent) != len(lines) {
			t.Fatalf("the host sent %d lines and %s holds %d; record it again with -update, then scripts/export-shell-fixtures.sh", len(sent), transcriptPath, len(lines))
		}
		for i := range lines {
			if sent[i] == lines[i] {
				continue
			}
			var stored, live any
			if err := json.Unmarshal([]byte(lines[i]), &stored); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(sent[i]), &live); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("line %d: %s; record it again with -update, then scripts/export-shell-fixtures.sh", i+1, transcriptDifference("", stored, live))
		}
	})
}
