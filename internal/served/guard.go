package served

import (
	"crypto/rand"
	"errors"
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

// guardNotRestorable is Local's refusal for data that cannot be written back
// onto the current page.
const guardNotRestorable = "Data could not be restored onto the current page; use Revert."

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

// guardV2Live is the transport of a root that runs data guard v2: its guard
// keeps a typing record, judges an outside write by it, and pins the version a
// quiet removal replaced. A served folder's does. The trial's roots answer as
// Local does, and a transport that says nothing runs the v1 guard.
type guardV2Live interface {
	dataGuardV2() bool
}

func guardV2Of(live guardLive) bool {
	v2, ok := live.(guardV2Live)
	return ok && v2.dataGuardV2()
}

// guardPreviewRow is one "now / yours" line of the recovery panel.
type guardPreviewRow = dataguard.PreviewRow

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
	// Reason, Typed and CanPutBack are data guard v2's, beside the v1 fields. An
	// event that has none of them is the v1 event byte for byte.
	Reason     string               `json:"reason,omitempty"`
	Typed      []dataguard.TypedRow `json:"typed,omitempty"`
	CanPutBack bool                 `json:"canPutBack,omitempty"`
}

// extractIsland is data-extractor.js's extractViaTag(html, "api"): the island
// the guard judges, wrapped under the rules name. A page with no rules tag is a
// nil island, not an error; an error means the bytes did not parse or the tag
// declares a version this engine does not know.
func extractIsland(html string) (dataapi.Value, error) {
	return dataguard.ExtractIsland([]byte(html))
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
	IfMatched   bool
	// ProvenReader is a program that named the exact version it read, and
	// FromPage a browser page's own data API write (dataguard.Observation).
	ProvenReader bool
	FromPage     bool
	Live         guardLive
}

// outside is dataguard.Observation's outside: a write no page sent and no
// program proved it read.
func (a runGuardArgs) outside() bool {
	return a.Prov == dataguard.External && !a.FromPage && !a.ProvenReader
}

// runDataLossGuard is one detection pass. It returns the client-safe event when
// a loss is raised, else nil, and never fails a write.
func runDataLossGuard(args runGuardArgs) *guardClientEvent {
	live := liveOf(args.Live)
	inc, readable := safeExtractIsland(args.NewHTML)
	var history dataguard.HistoryReader
	if guardV2Of(live) {
		history = servedHistory{baseDir: args.BaseDir, name: args.Name}
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

		if guard.Baseline != nil && args.PrevContent != nil {
			normalized := dataguard.NormalizeComposedBaseline(guard.Baseline.Data, []byte(*args.PrevContent))
			if normalized != guard.Baseline.Data {
				guard.Baseline = newGuardBaseline(normalized, guard.Baseline.At)
			}
		}
		if !readable && (guard.Baseline == nil || dataguard.IsEmptyIsland(guard.Baseline.Data)) {
			var seed dataapi.Value
			if args.PrevContent != nil {
				seed, _ = safeExtractIsland(*args.PrevContent)
			}
			if dataguard.IsEmptyIsland(seed) {
				return
			}
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
			if lossUndone(classify(dataguard.NormalizeComposedBaseline(guard.Event.RecoverableData, []byte(args.NewHTML)), inc)) {
				autoResolvedID = guard.Event.ID
				guard.Event = nil
				guard.Status = "restored"
				guard.UIWorkPending = false
				guard.Baseline = newGuardBaseline(inc, guardNow())
				writeGuard(args.BaseDir, args.Name, guard)
				return
			}
			fire, verdict := judgeWrite(args, history != nil, guard, base, inc, destruction)
			if fire {
				guard.Event.LastWriteAt = guardNow()
				if verdict != nil {
					guard.Event.Typed = dataguard.UnionTyped(guard.Event.Typed, verdict.Gone)
					if verdict.Reason != "" {
						guard.Event.Reason = verdict.Reason
					}
				}
			}
			if args.Prov == dataguard.External {
				guard.UIWorkPending = false
			}
			writeGuard(args.BaseDir, args.Name, guard)
			return
		}

		destruction := classify(base, inc)
		fire, verdict := judgeWrite(args, history != nil, guard, base, inc, destruction)
		// An outside write that removed data without firing is one the guard
		// judged intended. Silence is not permission to lose the copy, so the
		// version it replaced is pinned; when no copy can be kept, the guard
		// cannot stay silent.
		if !fire && history != nil && args.outside() && (destruction.AnyDestruction || len(dataguard.DropGone(guard.Typed, inc)) < len(guard.Typed)) {
			fire = !pinReplaced(args, history)
		}
		if fire {
			backup := captureRecoverPath(args, history)
			guard.Event = &guardRecordEvent{
				ID:                    guardNewID(),
				RecoverableData:       guard.Baseline.Data,
				RecoverableHTMLBackup: backup,
				FirstDetectedAt:       guardNow(),
				LastWriteAt:           guardNow(),
				LossSummary:           dataguard.LossSummaryOf(args.Prov, destruction),
				Status:                "pending",
			}
			if verdict != nil {
				guard.Event.Reason = verdict.Reason
				guard.Event.Typed = verdict.Gone
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
			now := guardNow()
			if history != nil {
				guard.Typed = dataguard.UpdateTyped(typedBefore(args, base), inc, guard.Typed, now)
			}
			guard.Baseline = newGuardBaseline(inc, now)
			guard.UIWorkPending = true
		} else if args.Prov == dataguard.External {
			guard.UIWorkPending = false
			if history != nil && (args.outside() || args.ProvenReader) {
				guard.Typed = dataguard.DropGone(guard.Typed, inc)
				if inc != nil && !dataguard.IsEmptyIsland(inc) {
					guard.Baseline = newGuardBaseline(inc, guardNow())
				}
			}
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

// judgeWrite is the fire decision for one write, the one dataguard.Guard.Observe
// makes. A root that runs data guard v2 judges an outside write by what a person
// typed (dataguard.JudgeOutside) and answers the verdict; every other write, and
// every write to a root that runs v1, is ShouldFire's.
func judgeWrite(args runGuardArgs, v2 bool, guard *guardRecord, base, inc dataapi.Value, destruction *dataguard.Destruction) (bool, *dataguard.OutsideVerdict) {
	fire := dataguard.ShouldFire(args.Prov, guard.UIWorkPending && !args.IfMatched, base, inc, destruction, args.ProvenReader)
	if !v2 || !args.outside() || dataguard.IsSheet(base) || dataguard.IsSheet(inc) {
		return fire, nil
	}
	verdict := dataguard.JudgeOutside(base, inc, destruction, guard.Typed, guardNow())
	return verdict.Fire, &verdict
}

// typedBefore is the data a gestured save started from: the page's previous
// bytes when the lane has them, else the baseline.
func typedBefore(args runGuardArgs, base dataapi.Value) dataapi.Value {
	if args.PrevContent != nil {
		if previous, ok := safeExtractIsland(*args.PrevContent); ok && previous != nil {
			return previous
		}
	}
	return base
}

// servedHistory is a served document's kept versions as the guard reads them
// (dataguard.HistoryReader): the copy an outside write replaced, and the pin
// that keeps it.
type servedHistory struct {
	baseDir string
	name    string
}

func (h servedHistory) dir() string {
	return versionsDirFor(h.baseDir, h.name)
}

func (h servedHistory) read(versionName string) ([]byte, bool) {
	full, err := versionFilePath(h.baseDir, h.name, versionName)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(full)
	return data, err == nil
}

// NewestDifferingFrom is the newest kept version that is not the bytes named by
// hash, the digest servedHistoryHash gives the bytes just written.
func (h servedHistory) NewestDifferingFrom(hash string) ([]byte, bool) {
	for _, entry := range listVersionEntries(h.dir(), false) {
		if data, ok := h.read(entry.Name); ok && guardSHA256(data) != hash {
			return data, true
		}
	}
	return nil, false
}

// Keep answers the version that holds exactly content, storing one when none does.
func (h servedHistory) Keep(content []byte) (string, error) {
	file, _ := splitPage(string(content))
	for _, entry := range listVersionEntries(h.dir(), false) {
		if entry.Size != int64(len(file)) {
			continue
		}
		if data, ok := h.read(entry.Name); ok && string(data) == file {
			return entry.Name, nil
		}
	}
	published := CreateBackup(h.baseDir, backupSiteName(h.name), []byte(file), nil)
	if published == nil {
		return "", errors.New("the version could not be kept")
	}
	return published.Filename, nil
}

func (h servedHistory) Pin(name string) error {
	return PinVersion(h.baseDir, h.dir(), name)
}

// servedHistoryHash is the digest of a document as its versions hold it: the
// file part, without the page state a version never carries.
func servedHistoryHash(html string) string {
	file, _ := splitPage(html)
	return guardSHA256([]byte(file))
}

// pinReplaced pins the version an outside write replaced, so the one copy of
// the data the guard stayed silent about is kept until the person deletes it.
// It reports whether that copy is now kept and pinned.
func pinReplaced(args runGuardArgs, history dataguard.HistoryReader) bool {
	var body []byte
	if args.PrevContent != nil {
		body = []byte(*args.PrevContent)
	} else {
		found, ok := history.NewestDifferingFrom(servedHistoryHash(args.NewHTML))
		if !ok {
			return false
		}
		body = found
	}
	name, err := history.Keep(body)
	if err != nil {
		log.Printf("[data-guard] could not keep the version %s replaced: %v", args.Name, err)
		return false
	}
	if err := history.Pin(name); err != nil {
		log.Printf("[data-guard] could not pin version %s of %s: %v", name, args.Name, err)
		return false
	}
	return true
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
// pins. It prefers the pre-write body, else the newest version entry; a root
// that runs data guard v2 passes over a version that is the bytes just written.
func captureRecoverPath(args runGuardArgs, history dataguard.HistoryReader) *string {
	baseDir, name := args.BaseDir, args.Name
	var body []byte
	if args.PrevContent != nil {
		body = []byte(*args.PrevContent)
	} else if history != nil {
		found, ok := history.NewestDifferingFrom(servedHistoryHash(args.NewHTML))
		if !ok {
			return nil
		}
		body = found
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
	if dataguard.HasCodecs(recoverable) {
		return "", false
	}
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
	recoverableData := dataguard.OrdinaryData(recoverable)
	var currentData dataapi.Value
	if currentIsland != nil {
		currentData = dataguard.OrdinaryData(currentIsland)
	}
	preview := dataguard.IslandPreview(recoverable, currentIsland)
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
		Reason:           event.Reason,
		Typed:            dataguard.TypedRows(event.Typed),
		CanPutBack:       dataguard.CanPutBack(event.Typed, currentIsland, recoverable),
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
		if currentOK && current != nil && lossUndone(classify(dataguard.NormalizeComposedBaseline(guard.Event.RecoverableData, []byte(*currentHTML)), current)) {
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

// resolveGuard is dismiss, revert, restore or put-back. writeBack performs the
// actual save through the caller's backup-then-write helper, so a resolution is
// versioned.
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
			return guardRefusal(422, guardNotRestorable)
		}
		if args.WriteBack != nil {
			if err := args.WriteBack(merged); err != nil {
				return guardRefusal(422, guardNotRestorable)
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

	case "put-back":
		// The typed values the loss took go back into the places the current
		// page still has, so every other change the writer made stays.
		current, _ := safeExtractIsland(args.CurrentHTML)
		if len(event.Typed) == 0 || current == nil || event.RecoverableData == nil || args.WriteBack == nil {
			return guardRefusal(422, guardNotRestorable)
		}
		merged, placed := dataguard.PutBack(current, event.Typed, event.RecoverableData)
		if placed == 0 {
			return guardRefusal(422, guardNotRestorable)
		}
		html, ok := restoreDataOntoHTML(args.CurrentHTML, merged)
		if !ok {
			return guardRefusal(422, guardNotRestorable)
		}
		if err := args.WriteBack(html); err != nil {
			return guardRefusal(422, guardNotRestorable)
		}
		kept := merged
		if after, afterOK := safeExtractIsland(html); afterOK && after != nil {
			kept = after
		}
		clearEvent(args.BaseDir, args.Name, kept, "put-back", false)
		notifyResolved(args.Name, event.ID, live)
		return guardResolveResult{OK: true, Choice: args.Choice, Status: "put-back"}

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
	// The typing record describes the data the person has accepted, so it follows
	// the island the resolution leaves behind.
	if len(guard.Typed) > 0 {
		guard.Typed = dataguard.DropGone(guard.Typed, keptIsland)
	}
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
		BaseDir:      args.BaseDir,
		Name:         args.Name,
		NewHTML:      args.NewHTML,
		PrevContent:  args.PrevContent,
		Prov:         dataguard.Provenance(args.Prov),
		IfMatched:    args.IfMatched != nil && *args.IfMatched,
		ProvenReader: args.ProvenReader,
		FromPage:     args.FromPage,
		Live:         h.live,
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
