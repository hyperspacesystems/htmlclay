//go:build managedtrial

package managedtrial

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
)

// The data-loss guard: Local's src/main/data-loss-guard.js over dataguard's
// detection core. When a save or an outside edit wipes a page's saved data, the
// guard raises an event, keeps a recoverable copy and lets the editor restore
// it. Every write of a managed trial document comes through the Go host, so the
// host runs the same guard and writes the same records as the desktop app.

const guardRulesName = "api"

const (
	guardPreviewLimit = 80
	guardEmptyLabel   = "\u2014 empty \u2014"
	guardEllipsis     = "\u2026"
	guardItemsLabel   = "items"
)

// guardNow and guardNewID are Date.now() and crypto.randomUUID(). Tests replace
// them with counters so a record's stamps and ids are reproducible.
var guardNow = func() int64 { return time.Now().UnixMilli() }

var guardNewID = newGuardUUID

func newGuardUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", guardNow())
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// guardNotification is Local's live.notify payload for a guard event.
type guardNotification struct {
	MsgType string `json:"msgType"`
	Action  string `json:"action"`
	Msg     string `json:"msg"`
	Data    any    `json:"data"`
}

// guardLive is the live transport a guard notification goes to. G9 attaches the
// real one; until then the binary notifies nobody.
type guardLive interface {
	Notify(name string, message guardNotification)
}

type noopGuardLive struct{}

func (noopGuardLive) Notify(string, guardNotification) {}

func liveOf(live guardLive) guardLive {
	if live == nil {
		return noopGuardLive{}
	}
	return live
}

// guardPreviewRow is one "now / yours" line of the recovery panel.
type guardPreviewRow struct {
	Key   string `json:"key"`
	Now   string `json:"now"`
	Yours string `json:"yours"`
}

// guardClientEvent is the event shape the panel is allowed to see. It carries no
// filesystem path: a recover path would tell a page where the app stores things.
type guardClientEvent struct {
	ID               string                `json:"id"`
	FirstDetectedAt  int64                 `json:"firstDetectedAt"`
	LastWriteAt      int64                 `json:"lastWriteAt"`
	LossSummary      dataguard.LossSummary `json:"lossSummary"`
	FieldCount       int                   `json:"fieldCount"`
	Preview          []guardPreviewRow     `json:"preview"`
	DroppedAdditions int                   `json:"droppedAdditions"`
	Restorable       bool                  `json:"restorable"`
	CanRevert        bool                  `json:"canRevert"`
	Status           string                `json:"status"`
}

// extractIsland is data-extractor.js's extractViaTag(html, "api"): the island
// the guard judges, wrapped under the rules name. A page with no rules tag is a
// nil island, not an error; an error means the bytes did not parse or the tag
// declares a version this engine does not know.
func extractIsland(html string) (dataapi.Value, error) {
	document, err := dataapi.ParseBytes([]byte(html))
	if err != nil {
		return nil, err
	}
	tag, err := document.FindRulesIn(guardRulesName)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, nil
	}
	value, err := document.Extract(tag.Rules)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	island := dataapi.NewObject()
	island.Set(guardRulesName, value)
	return island, nil
}

func safeExtractIsland(html string) (dataapi.Value, bool) {
	island, err := extractIsland(html)
	if err != nil {
		return nil, false
	}
	return island, true
}

func islandField(island dataapi.Value, key string) dataapi.Value {
	object, ok := island.(*dataapi.Object)
	if !ok || object == nil {
		return nil
	}
	value, _ := object.Get(key)
	return value
}

// classify is classifyDestruction plus the cost-guard fallback's log line.
func classify(base, inc dataapi.Value) *dataguard.Destruction {
	destruction := dataguard.ClassifyDestruction(base, inc)
	if destruction.FuzzyFallback {
		log.Printf("[data-guard] item list too large for fuzzy matching; used a coarser index-aligned compare (detection may be less precise).")
	}
	return destruction
}

// lossUndone is true only when the current island still contains every pinned
// recoverable atom, additions allowed: a recoverable atom merely modified to a
// new value has not been restored.
func lossUndone(destruction *dataguard.Destruction) bool {
	return !destruction.AnyDestruction && destruction.ModifiedAtoms == 0 && destruction.PlaceholderResets == 0
}

func buildPreview(recoverableData, currentData dataapi.Value) []guardPreviewRow {
	rows := []guardPreviewRow{}
	recoverable := previewContainer(recoverableData)
	current := previewContainer(currentData)

	yoursList, yoursIsList := recoverable.([]dataapi.Value)
	nowList, nowIsList := current.([]dataapi.Value)
	if yoursIsList || nowIsList {
		yoursCount := len(yoursList)
		nowCount := len(nowList)
		if yoursCount != nowCount {
			rows = append(rows, guardPreviewRow{Key: guardItemsLabel, Now: countLabel(nowCount), Yours: countLabel(yoursCount)})
		}
		return rows
	}

	for _, key := range previewKeys(recoverable, current) {
		yours := formatValue(guardLookup(recoverable, key))
		now := formatValue(guardLookup(current, key))
		if yours != now {
			rows = append(rows, guardPreviewRow{Key: key, Now: now, Yours: yours})
		}
	}
	return rows
}

func previewContainer(v dataapi.Value) dataapi.Value {
	switch v.(type) {
	case []dataapi.Value, *dataapi.Object:
		return v
	}
	return dataapi.NewObject()
}

func guardLookup(container dataapi.Value, key string) dataapi.Value {
	switch typed := container.(type) {
	case *dataapi.Object:
		value, _ := typed.Get(key)
		return value
	case []dataapi.Value:
		for index := range typed {
			if fmt.Sprintf("%d", index) == key {
				return typed[index]
			}
		}
	}
	return nil
}

func previewKeys(recoverable, current dataapi.Value) []string {
	seen := map[string]bool{}
	keys := []string{}
	for _, source := range []dataapi.Value{recoverable, current} {
		switch typed := source.(type) {
		case []dataapi.Value:
			for index := range typed {
				key := fmt.Sprintf("%d", index)
				if !seen[key] {
					seen[key] = true
					keys = append(keys, key)
				}
			}
		case *dataapi.Object:
			for _, key := range typed.Keys() {
				if !seen[key] {
					seen[key] = true
					keys = append(keys, key)
				}
			}
		}
	}
	return keys
}

func countLabel(n int) string {
	if n == 0 {
		return guardEmptyLabel
	}
	return fmt.Sprintf("%d %s", n, guardItemsLabel)
}

func formatValue(v dataapi.Value) string {
	if !dataguard.Meaningful(v) {
		return guardEmptyLabel
	}
	if s, ok := v.(string); ok {
		return truncate(dataguard.JSStringTrim(s))
	}
	if items, ok := v.([]dataapi.Value); ok {
		allObjects := true
		for _, item := range items {
			object, isObject := item.(*dataapi.Object)
			if !isObject || object == nil {
				allObjects = false
				break
			}
		}
		if allObjects {
			if len(items) == 1 {
				return "1 item"
			}
			return fmt.Sprintf("%d items", len(items))
		}
		parts := make([]string, len(items))
		for index, item := range items {
			if object, isObject := item.(*dataapi.Object); isObject && object != nil {
				parts[index] = dataguard.Stringify(item)
			} else {
				parts[index] = jsStringOf(item)
			}
		}
		return truncate(strings.Join(parts, ", "))
	}
	if _, ok := v.(*dataapi.Object); ok {
		return truncate(dataguard.Stringify(v))
	}
	return jsStringOf(v)
}

func jsStringOf(v dataapi.Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	return dataguard.Stringify(v)
}

func truncate(s string) string {
	units := utf16.Encode([]rune(s))
	if len(units) <= guardPreviewLimit {
		return s
	}
	return string(utf16.Decode(units[:guardPreviewLimit-1])) + guardEllipsis
}

// runGuardArgs is the object Local's lanes hand runDataLossGuard.
type runGuardArgs struct {
	BaseDir     string
	Name        string
	NewHTML     string
	PrevContent *string
	Prov        dataguard.Provenance
	Live        guardLive
}

// runDataLossGuard is one detection pass. It returns the client-safe event when
// a loss is raised, else nil, and never fails a write.
func runDataLossGuard(args runGuardArgs) *guardClientEvent {
	live := liveOf(args.Live)
	inc, ok := safeExtractIsland(args.NewHTML)
	if !ok {
		return nil
	}

	var raised *guardRecordEvent
	autoResolvedID := ""

	withGuardLock(guardKey(args.BaseDir, args.Name), func() {
		guard, err := readGuard(args.BaseDir, args.Name)
		if err != nil {
			guard = emptyGuard()
		}
		if guard == nil {
			guard = emptyGuard()
		}

		if guard.Baseline == nil {
			if args.PrevContent != nil {
				seed, seedOK := safeExtractIsland(*args.PrevContent)
				if seedOK && seed != nil && !dataguard.IsEmptyIsland(seed) {
					guard.Baseline = newGuardBaseline(seed, guardNow())
					guard.UIWorkPending = false
				} else {
					seedBlind(guard, inc)
					writeGuard(args.BaseDir, args.Name, guard)
					return
				}
			} else {
				seedBlind(guard, inc)
				writeGuard(args.BaseDir, args.Name, guard)
				return
			}
		}

		base := guard.Baseline.Data

		if guard.Event != nil {
			destruction := classify(base, inc)
			if lossUndone(destruction) {
				autoResolvedID = guard.Event.ID
				guard.Event = nil
				guard.Status = "restored"
				guard.UIWorkPending = false
				guard.Baseline = newGuardBaseline(inc, guardNow())
				writeGuard(args.BaseDir, args.Name, guard)
				return
			}
			if dataguard.ShouldFire(args.Prov, guard.UIWorkPending, base, inc, destruction) {
				guard.Event.LastWriteAt = guardNow()
			}
			if args.Prov == dataguard.External {
				guard.UIWorkPending = false
			}
			writeGuard(args.BaseDir, args.Name, guard)
			return
		}

		destruction := classify(base, inc)
		if dataguard.ShouldFire(args.Prov, guard.UIWorkPending, base, inc, destruction) {
			backup := captureRecoverPath(args.BaseDir, args.Name, args.PrevContent)
			guard.Event = &guardRecordEvent{
				ID:                    guardNewID(),
				RecoverableData:       guard.Baseline.Data,
				RecoverableHTMLBackup: backup,
				FirstDetectedAt:       guardNow(),
				LastWriteAt:           guardNow(),
				LossSummary:           dataguard.LossSummaryOf(args.Prov, destruction),
				Status:                "pending",
			}
			guard.Status = "pending"
			if args.Prov == dataguard.External {
				guard.UIWorkPending = false
			}
			writeGuard(args.BaseDir, args.Name, guard)
			raised = guard.Event
			return
		}

		if args.Prov == dataguard.UIGestured {
			guard.Baseline = newGuardBaseline(inc, guardNow())
			guard.UIWorkPending = true
		} else if args.Prov == dataguard.External {
			guard.UIWorkPending = false
		}
		writeGuard(args.BaseDir, args.Name, guard)
	})

	if autoResolvedID != "" {
		notifyResolved(args.Name, autoResolvedID, live)
		return nil
	}
	if raised != nil {
		clientEvent := toClientEvent(raised, args.NewHTML)
		notifyRaised(args.Name, clientEvent, live)
		return clientEvent
	}
	return nil
}

// seedBlind records the island just written as the baseline, for content nobody
// vouched for.
func seedBlind(guard *guardRecord, island dataapi.Value) {
	if island != nil && !dataguard.IsEmptyIsland(island) {
		guard.Baseline = newGuardBaseline(island, guardNow())
	}
	guard.UIWorkPending = false
}

// captureRecoverPath is the whole last-good file for Revert, always copied into
// the guard's own storage so the retention pruner can never delete what an event
// pins. It prefers the pre-write body, else the newest version entry.
func captureRecoverPath(baseDir, name string, prevContent *string) *string {
	var body []byte
	if prevContent != nil {
		body = []byte(*prevContent)
	} else {
		versionPath := newestVersionPath(baseDir, name)
		if versionPath == "" {
			return nil
		}
		read, err := os.ReadFile(versionPath)
		if err != nil {
			log.Printf("[data-guard] captureRecoverPath read failed: %v", err)
			return nil
		}
		body = read
	}

	abs, err := recoverHTMLPath(baseDir, name)
	if err != nil {
		log.Printf("[data-guard] captureRecoverPath write failed: %v", err)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o777); err != nil {
		log.Printf("[data-guard] captureRecoverPath write failed: %v", err)
		return nil
	}
	if err := os.WriteFile(abs, body, 0o666); err != nil {
		log.Printf("[data-guard] captureRecoverPath write failed: %v", err)
		return nil
	}
	return &abs
}

// provenanceForLocalSave is what caused a local save: a human gesture, an
// autosave, or neither when the client said nothing.
func provenanceForLocalSave(userDriven bool) string {
	return string(dataguard.UIProvenanceOf(&userDriven))
}

func notifyRaised(name string, event *guardClientEvent, live guardLive) {
	liveOf(live).Notify(name, guardNotification{
		MsgType: "data-loss",
		Action:  "raised",
		Msg:     "Saved data overwritten",
		Data:    event,
	})
}

func notifyResolved(name, eventID string, live guardLive) {
	liveOf(live).Notify(name, guardNotification{
		MsgType: "data-loss",
		Action:  "resolved",
		Msg:     "Data guard resolved",
		Data:    map[string]string{"id": eventID},
	})
}

// restoreDataOntoHTML writes recoverable back onto the current document, and
// only answers ok when reading it back out reproduces the data exactly: a
// restore overwrites the person's file.
func restoreDataOntoHTML(currentHTML string, recoverable dataapi.Value) (string, bool) {
	data := islandField(recoverable, guardRulesName)
	if data == nil {
		return "", false
	}
	document, err := dataapi.ParseBytes([]byte(currentHTML))
	if err != nil {
		return "", false
	}
	tag, err := document.FindRulesIn(guardRulesName)
	if err != nil || tag == nil {
		return "", false
	}
	result, err := dataapi.WriteDocument([]byte(currentHTML), data, guardRulesName)
	if err != nil {
		return "", false
	}
	merged := string(result.HTML)
	reExtracted, err := extractIsland(merged)
	if err != nil || reExtracted == nil {
		return "", false
	}
	if !dataguard.IslandsEqual(reExtracted, recoverable) {
		return "", false
	}
	return merged, true
}

// toClientEvent is the panel payload: the preview and the Restore dry-run.
func toClientEvent(event *guardRecordEvent, currentHTML string) *guardClientEvent {
	recoverable := dataapi.Value(nil)
	if event.RecoverableData != nil {
		recoverable = event.RecoverableData
	}
	var currentIsland dataapi.Value
	if island, ok := safeExtractIsland(currentHTML); ok {
		currentIsland = island
	}
	_, restorable := restoreDataOntoHTML(currentHTML, recoverable)
	recoverableData := islandField(recoverable, guardRulesName)
	var currentData dataapi.Value
	if currentIsland != nil {
		currentData = islandField(currentIsland, guardRulesName)
	}
	preview := buildPreview(recoverableData, currentData)
	status := event.Status
	if status == "" {
		status = "pending"
	}
	return &guardClientEvent{
		ID:               event.ID,
		FirstDetectedAt:  event.FirstDetectedAt,
		LastWriteAt:      event.LastWriteAt,
		LossSummary:      event.LossSummary,
		FieldCount:       len(preview),
		Preview:          preview,
		DroppedAdditions: dataguard.DroppedItems(recoverableData, currentData),
		Restorable:       restorable,
		CanRevert:        event.RecoverableHTMLBackup != nil,
		Status:           status,
	}
}

// getGuardEvent is the page-load read. An event whose loss is already undone is
// cleared instead of shown, and a first sight of a document seeds its baseline.
func getGuardEvent(baseDir, name string, currentHTML *string, live guardLive) *guardClientEvent {
	guard, err := readGuard(baseDir, name)
	if err != nil {
		guard = nil
	}
	if guard != nil && guard.Event != nil {
		var current dataapi.Value
		currentOK := false
		if currentHTML != nil {
			current, currentOK = safeExtractIsland(*currentHTML)
		}
		if currentOK && current != nil && lossUndone(classify(guard.Event.RecoverableData, current)) {
			resolvedID := guard.Event.ID
			clearEvent(baseDir, name, current, "restored", false)
			notifyResolved(name, resolvedID, live)
			return nil
		}
		currentText := ""
		if currentHTML != nil {
			currentText = *currentHTML
		}
		return toClientEvent(guard.Event, currentText)
	}
	if currentHTML != nil {
		seed, seedOK := safeExtractIsland(*currentHTML)
		if seedOK && seed != nil && !dataguard.IsEmptyIsland(seed) {
			withGuardLock(guardKey(baseDir, name), func() {
				stored, err := readGuard(baseDir, name)
				if err != nil || stored == nil {
					stored = emptyGuard()
				}
				if stored.Baseline == nil {
					stored.Baseline = newGuardBaseline(seed, guardNow())
					stored.UIWorkPending = false
					writeGuard(baseDir, name, stored)
				}
			})
		}
	}
	return nil
}

// markPending records the bytes a save is about to land, before the host renames
// them into place.
func markPending(baseDir, name string, bytes []byte) {
	guardSeen.setPending(guardKey(baseDir, name), guardSHA256(bytes))
}

// noteSeen records the bytes the host last saw. A stamp that arrives before the
// document has a baseline waits for the first baseline write to record it.
func noteSeen(baseDir, name string, bytes []byte) {
	key := guardKey(baseDir, name)
	hash := guardSHA256(bytes)
	guardSeen.setPending(key, hash)
	withGuardLock(key, func() {
		guard, err := readGuard(baseDir, name)
		if err != nil {
			guard = nil
		}
		if guard != nil && guard.Baseline != nil {
			guardSeen.deleteUnstamped(key)
			if guard.LastSeen == nil || *guard.LastSeen != hash {
				stamped := hash
				guard.LastSeen = &stamped
				writeGuard(baseDir, name, guard)
			}
		} else {
			guardSeen.setUnstamped(key, hash)
		}
	})
	guardSeen.clearPending(key, hash)
}

// catchUpOutsideEdit is the pass an edit made while the app was closed never
// got: the bytes on disk against the bytes the host last saw.
func catchUpOutsideEdit(baseDir, name string, bytes []byte, html string, live guardLive) {
	hash := guardSHA256(bytes)
	if guardSeen.pendingFor(guardKey(baseDir, name)) == hash {
		return
	}
	guard, err := readGuard(baseDir, name)
	if err != nil {
		guard = nil
	}
	if guard == nil || guard.Baseline == nil {
		return
	}
	if guard.LastSeen != nil && *guard.LastSeen == hash {
		return
	}
	if guard.LastSeen != nil {
		runDataLossGuard(runGuardArgs{BaseDir: baseDir, Name: name, NewHTML: html, PrevContent: nil, Prov: dataguard.External, Live: live})
	}
	noteSeen(baseDir, name, bytes)
}

// guardControl is the payload a dismiss hands back so the platform can nudge the
// owner's other devices.
type guardControl struct {
	FileKey             string `json:"fileKey"`
	RecoverableDataHash string `json:"recoverableDataHash"`
}

// guardResolveResult is Local's resolve answer: success names the choice and the
// status, refusal names a status code and a message.
type guardResolveResult struct {
	OK         bool          `json:"ok"`
	Choice     string        `json:"choice,omitempty"`
	Status     string        `json:"status,omitempty"`
	Control    *guardControl `json:"control,omitempty"`
	StatusCode int           `json:"statusCode,omitempty"`
	Error      string        `json:"error,omitempty"`
}

type resolveGuardArgs struct {
	BaseDir     string
	Name        string
	ID          string
	Choice      string
	CurrentHTML string
	WriteBack   func(html string) error
	Live        guardLive
}

func guardRefusal(statusCode int, message string) guardResolveResult {
	return guardResolveResult{StatusCode: statusCode, Error: message}
}

// resolveGuard is dismiss, revert or restore. writeBack performs the actual save
// through the caller's backup-then-write helper, so a resolution is versioned.
func resolveGuard(args resolveGuardArgs) guardResolveResult {
	live := liveOf(args.Live)
	guard, err := readGuard(args.BaseDir, args.Name)
	if err != nil || guard == nil || guard.Event == nil {
		return guardRefusal(404, "No pending data-loss event")
	}
	if args.ID != "" && guard.Event.ID != args.ID {
		return guardRefusal(409, "Event id mismatch (already resolved?)")
	}
	event := guard.Event

	switch args.Choice {
	case "dismiss":
		current, currentOK := safeExtractIsland(args.CurrentHTML)
		var kept dataapi.Value
		if currentOK {
			kept = current
		}
		clearEvent(args.BaseDir, args.Name, kept, "dismissed", true)
		notifyResolved(args.Name, event.ID, live)
		return guardResolveResult{
			OK:     true,
			Choice: args.Choice,
			Status: "dismissed",
			Control: &guardControl{
				FileKey:             args.Name,
				RecoverableDataHash: dataguard.IslandHash(event.RecoverableData),
			},
		}

	case "restore":
		merged, ok := restoreDataOntoHTML(args.CurrentHTML, event.RecoverableData)
		if !ok {
			return guardRefusal(422, "Data could not be restored onto the current page; use Revert.")
		}
		if args.WriteBack != nil {
			if err := args.WriteBack(merged); err != nil {
				return guardRefusal(422, "Data could not be restored onto the current page; use Revert.")
			}
		}
		after, afterOK := safeExtractIsland(merged)
		kept := dataapi.Value(event.RecoverableData)
		if afterOK {
			kept = after
		}
		clearEvent(args.BaseDir, args.Name, kept, "restored", false)
		notifyResolved(args.Name, event.ID, live)
		return guardResolveResult{OK: true, Choice: args.Choice, Status: "restored"}

	case "revert":
		if event.RecoverableHTMLBackup == nil {
			return guardRefusal(422, "No whole-file backup is available to revert to.")
		}
		recoverPath := filepath.Clean(*event.RecoverableHTMLBackup)
		root := filepath.Clean(args.BaseDir)
		if !strings.HasPrefix(recoverPath, root+string(filepath.Separator)) {
			return guardRefusal(422, "Recover path is outside the sync folder.")
		}
		read, err := os.ReadFile(recoverPath)
		if err != nil {
			return guardRefusal(422, "Could not read the backup to revert to.")
		}
		html := string(read)
		if args.WriteBack != nil {
			if err := args.WriteBack(html); err != nil {
				return guardRefusal(422, "Could not read the backup to revert to.")
			}
		}
		after, afterOK := safeExtractIsland(html)
		var kept dataapi.Value
		if afterOK {
			kept = after
		}
		clearEvent(args.BaseDir, args.Name, kept, "reverted", false)
		notifyResolved(args.Name, event.ID, live)
		return guardResolveResult{OK: true, Choice: args.Choice, Status: "reverted"}
	}

	return guardRefusal(400, "Unknown choice")
}

// clearEventLocked is the read-modify-write of a clear without taking the lock.
func clearEventLocked(baseDir, name string, keptIsland dataapi.Value, status string, forceBaseline bool) {
	guard, err := readGuard(baseDir, name)
	if err != nil || guard == nil {
		guard = emptyGuard()
	}
	guard.Event = nil
	guard.Status = status
	guard.UIWorkPending = false
	if forceBaseline {
		guard.Baseline = nil
		if keptIsland != nil {
			guard.Baseline = newGuardBaseline(keptIsland, guardNow())
		}
	} else if keptIsland != nil && !dataguard.IsEmptyIsland(keptIsland) {
		guard.Baseline = newGuardBaseline(keptIsland, guardNow())
	}
	writeGuard(baseDir, name, guard)
}

func clearEvent(baseDir, name string, keptIsland dataapi.Value, status string, forceBaseline bool) {
	withGuardLock(guardKey(baseDir, name), func() {
		clearEventLocked(baseDir, name, keptIsland, status, forceBaseline)
	})
}

// applyRemoteResolution applies an inbound cross-device Dismiss: it clears this
// device's matching event iff the pinned recoverable-data hash equals the
// message's, never creates one, and never clears on a mismatch.
func applyRemoteResolution(baseDir, name, recoverableDataHash string, live guardLive) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return false
	}
	if !isDocumentName(name) {
		return false
	}
	if !isLowerHex64(recoverableDataHash) {
		return false
	}
	resolvedID := ""
	withGuardLock(guardKey(baseDir, name), func() {
		guard, err := readGuard(baseDir, name)
		if err != nil || guard == nil || guard.Event == nil {
			return
		}
		if guard.Event.RecoverableData == nil || dataguard.IslandHash(guard.Event.RecoverableData) != recoverableDataHash {
			return
		}
		resolvedID = guard.Event.ID
		var current dataapi.Value
		currentOK := false
		if read, err := os.ReadFile(filepath.Join(baseDir, name)); err == nil {
			current, currentOK = safeExtractIsland(string(read))
		}
		var kept dataapi.Value
		if currentOK {
			kept = current
		}
		clearEventLocked(baseDir, name, kept, "dismissed", true)
	})
	if resolvedID != "" {
		notifyResolved(name, resolvedID, live)
	}
	return resolvedID != ""
}

func isLowerHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// guardHooks is the guard half of the commit core's hooks: every save stamps its
// bytes, and every document write is judged by the ported guard.
type guardHooks struct {
	noopCommitHooks
	live guardLive
}

func newGuardHooks(live guardLive) *guardHooks {
	return &guardHooks{live: liveOf(live)}
}

func (h *guardHooks) NoteSeen(baseDir, name string, stored any) {
	noteSeen(baseDir, name, hookBytes(stored))
}

func (h *guardHooks) MarkPending(baseDir, name string, stored any) {
	markPending(baseDir, name, hookBytes(stored))
}

func (h *guardHooks) RunDataLossGuard(args dataLossGuardArgs) {
	runDataLossGuard(runGuardArgs{
		BaseDir:     args.BaseDir,
		Name:        args.Name,
		NewHTML:     args.NewHTML,
		PrevContent: args.PrevContent,
		Prov:        dataguard.Provenance(args.Prov),
		Live:        h.live,
	})
}

func (h *guardHooks) ProvenanceForLocalSave(userDriven bool) string {
	return provenanceForLocalSave(userDriven)
}

func hookBytes(stored any) []byte {
	switch typed := stored.(type) {
	case string:
		return []byte(typed)
	case []byte:
		return typed
	case nil:
		return nil
	}
	return nil
}
