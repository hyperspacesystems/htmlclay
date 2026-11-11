//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

type commitGoldenFile struct {
	Scenarios []commitGoldenScenario `json:"scenarios"`
}

type commitGoldenScenario struct {
	Name            string                 `json:"name"`
	Seed            []commitGoldenSeed     `json:"seed"`
	StoredRecord    json.RawMessage        `json:"storedRecord"`
	StoredRecordB64 string                 `json:"storedRecordB64"`
	VersionAtRevA   string                 `json:"versionAtRevA"`
	Before          []commitGoldenTreeFile `json:"before"`
	Requests        []commitGoldenRequest  `json:"requests"`
	Results         []json.RawMessage      `json:"results"`
	ModifiedTimes   map[string]float64     `json:"modifiedTimes"`
	Hooks           []json.RawMessage      `json:"hooks"`
	Pages           []string               `json:"pages"`
	Tree            []commitGoldenTreeFile `json:"tree"`
}

type commitGoldenSeed struct {
	Path string `json:"path"`
	B64  string `json:"b64"`
}

type commitGoldenTreeFile struct {
	Path string `json:"path"`
	B64  string `json:"b64"`
}

type commitGoldenRequest struct {
	Clock        int64                  `json:"clock"`
	Kind         string                 `json:"kind"`
	Name         string                 `json:"name"`
	Text         *string                `json:"text"`
	B64          *string                `json:"b64"`
	UserDriven   *bool                  `json:"userDriven"`
	SaveID       *string                `json:"saveId"`
	DataLossPrev *string                `json:"dataLossPrev"`
	Expected     *commitExpectation     `json:"expected"`
	ModifiedAt   *int64                 `json:"modifiedAt"`
	PageRecord   *PageRecord            `json:"blockeditorRecord"`
	Edit         *bool                  `json:"edit"`
	Safety       *bool                  `json:"safety"`
	Structure    *commitGoldenStructure `json:"structure"`
}

// commitGoldenStructure is one workspace step, recorded as the logical paths the
// route resolved into canonical ones.
type commitGoldenStructure struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

// TestCommitGolden compares every scenario but AD2 exactly: the answers, the
// hook calls and the tree.
func TestCommitGolden(t *testing.T) {
	golden := loadCommitGolden(t)
	for _, scenario := range golden.Scenarios {
		if scenario.Name == "ad2-stale-lineage" {
			continue
		}
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			runCommitGoldenScenario(t, scenario)
		})
	}
}

func loadCommitGolden(t *testing.T) commitGoldenFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "commit-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden commitGoldenFile
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Scenarios) == 0 {
		t.Fatal("commit golden has no scenarios")
	}
	return golden
}

// commitGoldenRoot builds one scenario's temp root from its seed.
func commitGoldenRoot(t *testing.T, scenario commitGoldenScenario) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range scenario.Seed {
		content, err := base64.StdEncoding.DecodeString(seed.B64)
		if err != nil {
			t.Fatal(err)
		}
		writeCommitFile(t, root, seed.Path, content)
	}
	if scenario.StoredRecordB64 != "" {
		record, err := base64.StdEncoding.DecodeString(scenario.StoredRecordB64)
		if err != nil {
			t.Fatal(err)
		}
		writeCommitFile(t, root, filepath.Join(".hyperclay", "blockeditor-state", "stale.html.json"), record)
	}
	if scenario.VersionAtRevA != "" {
		writeCommitFile(t, root, filepath.Join(".hyperclay", "versions", "stale", "2026-09-01-00-00-00-000+0000.html"), []byte(scenario.VersionAtRevA))
	}
	return root
}

func writeCommitFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	target := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitClock drives the version store's clock and the blockeditor-state generation
// sequence from the golden, so a scenario's version names and generations are
// the ones Local wrote.
type commitClock struct {
	now       int64
	pages     []string
	pageIndex int
	exhausted bool
}

func (c *commitClock) set(value int64) { c.now = value }

func (c *commitClock) nextGeneration() string {
	if c.pageIndex >= len(c.pages) {
		c.exhausted = true
		return strings.Repeat("0", 32)
	}
	generation := c.pages[c.pageIndex]
	c.pageIndex++
	return generation
}

func freezeCommitClock(t *testing.T, scenario commitGoldenScenario) *commitClock {
	t.Helper()
	// Version names carry the local zone offset, and gen.cjs records in
	// America/New_York, so the test runs in that zone whatever the machine's is.
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	clock := &commitClock{pages: scenario.Pages}
	previousLocal := time.Local
	previousClock := versionClock
	previousGeneration := pageStateNewGeneration
	time.Local = zone
	versionClock = func() int64 { return clock.now }
	pageStateNewGeneration = clock.nextGeneration
	t.Cleanup(func() {
		time.Local = previousLocal
		versionClock = previousClock
		pageStateNewGeneration = previousGeneration
	})
	return clock
}

// commitRequestFromGolden rebuilds one request.
func commitRequestFromGolden(t *testing.T, request commitGoldenRequest) *commitRequest {
	t.Helper()
	out := &commitRequest{
		Kind:         request.Kind,
		Name:         request.Name,
		SaveID:       "",
		DataLossPrev: request.DataLossPrev,
		Expected:     request.Expected,
		ModifiedAt:   request.ModifiedAt,
		PageRecord:   request.PageRecord,
	}
	if request.Text != nil {
		out.Bytes = []byte(*request.Text)
		out.Text = true
	}
	if request.B64 != nil {
		bytes, err := base64.StdEncoding.DecodeString(*request.B64)
		if err != nil {
			t.Fatal(err)
		}
		out.Bytes = bytes
	}
	if request.SaveID != nil {
		out.SaveID = *request.SaveID
	}
	if request.UserDriven != nil {
		out.UserDriven = *request.UserDriven
		out.UserDrivenSet = true
	}
	if request.Edit != nil {
		out.Edit = *request.Edit
	}
	if request.Safety != nil {
		out.Safety = *request.Safety
	}
	return out
}

func runCommitGoldenScenario(t *testing.T, scenario commitGoldenScenario) {
	t.Helper()
	root := commitGoldenRoot(t, scenario)
	clock := freezeCommitClock(t, scenario)
	hooks := &recordingCommitHooks{root: root}
	host := newCommitHost(root, "root-a", newConsentRegistry(root, nil), hooks, nil)

	for index, request := range scenario.Requests {
		clock.set(request.Clock)
		if request.Structure != nil {
			got := marshalCommitJSON(t, structureResultJSON(runCommitGoldenStructure(t, host, root, *request.Structure)))
			if want := scenario.Results[index]; !jsonEqual(t, got, want) {
				t.Errorf("request %d result:\n got %s\nwant %s", index, got, want)
			}
			continue
		}
		result, err := host.commit(commitRequestFromGolden(t, request))
		if err != nil {
			t.Fatalf("request %d (%s) failed: %v", index, request.Name, err)
		}
		got := marshalCommitJSON(t, commitResultJSON(result))
		want := scenario.Results[index]
		if !jsonEqual(t, got, want) {
			t.Errorf("request %d result:\n got %s\nwant %s", index, got, want)
		}
		if modified, ok := scenario.ModifiedTimes[request.Name]; ok && request.ModifiedAt != nil {
			info, err := os.Stat(filepath.Join(root, request.Name))
			if err != nil {
				t.Fatal(err)
			}
			if got := float64(info.ModTime().UnixMilli()); got != modified {
				t.Errorf("%s mtime: got %v want %v", request.Name, got, modified)
			}
		}
	}
	if clock.exhausted {
		t.Error("the host asked for more blockeditor-state generations than Local did")
	}

	gotHooks := marshalCommitJSON(t, hooks.recorded())
	if !jsonEqual(t, gotHooks, marshalCommitJSON(t, scenario.Hooks)) {
		t.Errorf("hook calls:\n got %s\nwant %s", gotHooks, marshalCommitJSON(t, scenario.Hooks))
	}
	gotTree := commitTree(t, root)
	if !reflect.DeepEqual(gotTree, scenario.Tree) {
		t.Errorf("tree:\n got %v\nwant %v", gotTree, scenario.Tree)
	}
}

// runCommitGoldenStructure replays one recorded workspace step: the route
// resolves both names and hands the host the canonical pair.
func runCommitGoldenStructure(t *testing.T, host *commitHost, root string, step commitGoldenStructure) structureResult {
	t.Helper()
	entry := structureEntry{
		FromPath: filepath.Join(root, step.From),
		ToPath:   filepath.Join(root, step.To),
	}
	if info, err := os.Stat(entry.FromPath); err == nil {
		entry.IsDir = info.IsDir()
	}
	result, err := host.restructure(step.Kind, []structureEntry{entry})
	if err != nil {
		t.Fatalf("%s %s -> %s failed: %v", step.Kind, step.From, step.To, err)
	}
	return result
}

// structureResultJSON is Local's structure answer: the outcome, how many steps
// completed, and the reason a failure carries.
func structureResultJSON(result structureResult) map[string]any {
	out := map[string]any{"outcome": result.Outcome, "completed": result.Completed, "reason": nil}
	if result.Reason != "" {
		out["reason"] = result.Reason
	}
	return out
}

// TestCommitAD2StaleLineage is the one allowed refusal: Local rebases this save
// onto the stored page, and this host refuses it with nothing written.
func TestCommitAD2StaleLineage(t *testing.T) {
	golden := loadCommitGolden(t)
	var scenario *commitGoldenScenario
	for index := range golden.Scenarios {
		if golden.Scenarios[index].Name == "ad2-stale-lineage" {
			scenario = &golden.Scenarios[index]
		}
	}
	if scenario == nil {
		t.Fatal("commit golden has no ad2-stale-lineage scenario")
	}

	root := commitGoldenRoot(t, *scenario)
	clock := freezeCommitClock(t, *scenario)
	hooks := &recordingCommitHooks{root: root}
	host := newCommitHost(root, "root-a", newConsentRegistry(root, nil), hooks, nil)

	for _, request := range scenario.Requests {
		clock.set(request.Clock)
		result, err := host.commit(commitRequestFromGolden(t, request))
		if err != nil {
			t.Fatalf("%s failed: %v", request.Name, err)
		}
		if result.Outcome != "refused" || result.Reason != "stale-blockeditor-lineage" {
			t.Fatalf("got %s/%s, want refused/stale-blockeditor-lineage", result.Outcome, result.Reason)
		}
		if len(hooks.calls) != 0 {
			t.Fatalf("a refused save called %d hooks, want none", len(hooks.calls))
		}
	}

	if got := commitTree(t, root); !reflect.DeepEqual(got, scenario.Before) {
		t.Fatalf("a refused save changed the tree:\n got %v\nwant %v", got, scenario.Before)
	}
}

// TestCommitIndeterminate injects a directory-sync failure after the rename: a
// save whose bytes landed but whose directory entry may not survive is never
// reported as committed.
func TestCommitIndeterminate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no directory sync for an I/O error to fail")
	}
	previous := atomicDirSync
	atomicDirSync = func(dir string) error { return syscall.EIO }
	t.Cleanup(func() { atomicDirSync = previous })

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, root, "a.html", []byte("<!doctype html>\n<html><body><h1>One</h1></body></html>\n"))

	previousClock := versionClock
	versionClock = func() int64 { return time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).UnixMilli() }
	t.Cleanup(func() { versionClock = previousClock })

	hooks := &recordingCommitHooks{root: root}
	host := newCommitHost(root, "root-indeterminate", newConsentRegistry(root, nil), hooks, nil)
	result, err := host.commit(&commitRequest{
		Kind:  "browser-save",
		Name:  "a.html",
		Bytes: []byte("<!doctype html>\n<html><body><h1>Two</h1></body></html>\n"),
		Text:  true,
	})
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if result.Outcome != "indeterminate" {
		t.Fatalf("got outcome %q, want indeterminate", result.Outcome)
	}
	if result.Outcome == "committed" {
		t.Fatal("a write whose directory entry may not survive was reported as committed")
	}
	if result.Reason != "EIO" {
		t.Errorf("got reason %q, want EIO", result.Reason)
	}
}

// TestCommitExpectedUnderLock proves the precondition is judged under the
// document's lock: a save holding the lock delays a second save whose `expected`
// names the bytes the first one replaces, and that second save is refused.
func TestCommitExpectedUnderLock(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := []byte("<!doctype html>\n<html><body><h1>One</h1></body></html>\n")
	second := []byte("<!doctype html>\n<html><body><h1>Two</h1></body></html>\n")
	third := []byte("<!doctype html>\n<html><body><h1>Three</h1></body></html>\n")
	writeCommitFile(t, root, "a.html", first)

	previousClock := versionClock
	versionClock = func() int64 { return time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).UnixMilli() }
	t.Cleanup(func() { versionClock = previousClock })

	locked := make(chan struct{})
	release := make(chan struct{})
	inside := &blockingCommitHooks{after: func() {
		close(locked)
		<-release
	}}
	host := newCommitHost(root, "root-lock", newConsentRegistry(root, nil), inside, nil)

	firstDone := make(chan commitResult, 1)
	go func() {
		result, _ := host.commit(&commitRequest{Kind: "browser-save", Name: "a.html", Bytes: second, Text: true})
		firstDone <- result
	}()
	select {
	case <-locked:
	case <-time.After(10 * time.Second):
		t.Fatal("the first save never reached the document's lock")
	}

	secondDone := make(chan commitResult, 1)
	go func() {
		result, _ := host.commit(&commitRequest{
			Kind:     "browser-save",
			Name:     "a.html",
			Bytes:    third,
			Text:     true,
			Expected: &commitExpectation{Type: "checksum", Value: commitChecksum(first)},
		})
		secondDone <- result
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)

	select {
	case result := <-firstDone:
		if result.Outcome != "committed" {
			t.Fatalf("the first save got %q, want committed", result.Outcome)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the first save never finished")
	}
	select {
	case result := <-secondDone:
		if result.Outcome != "refused" || result.Reason != "changed" {
			t.Fatalf("the second save got %s/%s, want refused/changed", result.Outcome, result.Reason)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second save never finished")
	}
}

// blockingCommitHooks lets a test hold one save inside the document's lock.
type blockingCommitHooks struct {
	noopCommitHooks
	after func()
}

func (h *blockingCommitHooks) MarkPending(baseDir, name string, stored any) {
	if h.after != nil {
		after := h.after
		h.after = nil
		after()
	}
}

func commitChecksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])[:16]
}

// recordingCommitHooks records every hook call in the shape the golden
// generator records Local's own calls.
type recordingCommitHooks struct {
	noopCommitHooks
	root  string
	calls []map[string]any
}

func (h *recordingCommitHooks) record(fn string, returns any, args ...any) {
	normalized := make([]any, len(args))
	for index, arg := range args {
		normalized[index] = normalizeCommitValue(arg, h.root)
	}
	h.calls = append(h.calls, map[string]any{"fn": fn, "args": normalized, "returns": returns})
}

func (h *recordingCommitHooks) recorded() any {
	if h.calls == nil {
		return []map[string]any{}
	}
	return h.calls
}

func (h *recordingCommitHooks) NoteSeen(baseDir, name string, stored any) {
	h.record("dataGuard.noteSeen", "<promise>", baseDir, name, stored)
}

func (h *recordingCommitHooks) MarkPending(baseDir, name string, stored any) {
	h.record("dataGuard.markPending", nil, baseDir, name, stored)
}

func (h *recordingCommitHooks) RunDataLossGuard(args dataLossGuardArgs) {
	payload := map[string]any{
		"baseDir":     args.BaseDir,
		"name":        args.Name,
		"newHtml":     args.NewHTML,
		"prov":        args.Prov,
		"live":        "<live>",
		"prevContent": nil,
	}
	if args.PrevContent != nil {
		payload["prevContent"] = *args.PrevContent
	}
	h.record("dataGuard.runDataLossGuard", "<promise>", payload)
}

func (h *recordingCommitHooks) ProvenanceForLocalSave(userDriven bool) string {
	provenance := localSaveProvenance(userDriven)
	h.record("dataGuard.provenanceForLocalSave", provenance, userDriven)
	return provenance
}

func (h *recordingCommitHooks) MarkBrowserSave(name string) {
	h.record("live.markBrowserSave", nil, name)
}

func (h *recordingCommitHooks) Broadcast(name string, message liveMessage, options broadcastOptions) {
	h.record("live.broadcast", nil, name, map[string]any{"html": message.HTML, "sender": message.Sender}, map[string]any{"lane": options.Lane})
}

func (h *recordingCommitHooks) Notify(name string, message liveNotification) {
	data := map[string]any{"kind": message.Data.Kind, "sender": message.Data.Sender}
	if message.Data.HasHTML {
		data["html"] = message.Data.HTML
		data["etag"] = message.Data.Etag
	}
	h.record("live.notify", nil, name, map[string]any{
		"msgType": message.MsgType,
		"msg":     message.Msg,
		"action":  message.Action,
		"data":    data,
	})
}

func (h *recordingCommitHooks) Emit(event string, fields map[string]any) {
	h.record("emit", nil, event, fields)
}

func (h *recordingCommitHooks) RefreshTailwind(content string) error {
	h.record("compileTailwind", "<promise>", content)
	return nil
}

func (h *recordingCommitHooks) RefreshDerivedArtifacts(baseDir, name, content string) error {
	h.record("refreshDerivedArtifacts", "<promise>", baseDir, name, content)
	return nil
}

// normalizeCommitValue is the golden generator's normalization: a path becomes
// its relative form, a byte string or a long string its sha256, an object its
// sorted keys.
func normalizeCommitValue(value any, root string) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		for _, prefix := range []string{root, filepath.ToSlash(root)} {
			if prefix == "" {
				continue
			}
			if typed == prefix {
				return map[string]any{"path": "."}
			}
			if strings.HasPrefix(typed, prefix+"/") {
				return map[string]any{"path": strings.TrimPrefix(typed, prefix+"/")}
			}
		}
		if len(typed) > 40 {
			return map[string]any{"sha256": commitSha256([]byte(typed)), "length": len(typed)}
		}
		return typed
	case []byte:
		return map[string]any{"sha256": commitSha256(typed)}
	case bool, int, int64, float64:
		return typed
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = normalizeCommitValue(item, root)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeCommitValue(item, root)
		}
		return out
	}
	return value
}

func commitSha256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// commitResultJSON is Local's answer shape: the lanes name different fields, and
// a missing field is absent rather than null.
func commitResultJSON(result commitResult) map[string]any {
	out := map[string]any{"outcome": result.Outcome, "reason": nil}
	if result.Reason != "" {
		out["reason"] = result.Reason
	}
	if result.HasChecksum {
		out["checksum"] = nil
		if result.Checksum != "" {
			out["checksum"] = result.Checksum
		}
	}
	out["etag"] = nil
	if result.Etag != "" {
		out["etag"] = result.Etag
	}
	out["stored"] = commitResultBytes(result.Stored)
	out["content"] = commitResultBytes(result.Content)
	if result.HasFile {
		out["file"] = result.File
	}
	return out
}

func commitResultBytes(value any) any {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return map[string]any{"b64": base64.StdEncoding.EncodeToString(typed)}
	}
	return nil
}

// commitTree is the final file tree, with `.hyperclay/guard` left out: G7 owns
// those files and its calls are in the hook record.
func commitTree(t *testing.T, root string) []commitGoldenTreeFile {
	t.Helper()
	files := []commitGoldenTreeFile{}
	var walk func(dir, prefix string)
	walk = func(dir, prefix string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			rel := entry.Name()
			if prefix != "" {
				rel = prefix + "/" + entry.Name()
			}
			if rel == ".hyperclay/guard" || strings.HasPrefix(rel, ".hyperclay/guard/") {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				walk(full, rel)
				continue
			}
			content, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, commitGoldenTreeFile{Path: rel, B64: base64.StdEncoding.EncodeToString(content)})
		}
	}
	walk(root, "")
	return files
}

func marshalCommitJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// jsonEqual compares two JSON documents structurally.
func jsonEqual(t *testing.T, got json.RawMessage, want json.RawMessage) bool {
	t.Helper()
	var left, right any
	if err := json.Unmarshal(got, &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &right); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(left, right)
}
