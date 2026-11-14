package dataguard

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/versions"
)

// extractor reads the island the guard judges out of a document, and dry-runs a
// restore of one. Both need the document's own rules tag and its save token, so
// the server owns that knowledge and the guard stays free of it.
type Extractor interface {
	// Extract returns the island carried by a document. A nil island is a page
	// with no api rules tag, not an error; an error means the bytes did not parse.
	Extract(html []byte) (dataapi.Value, error)
	// Restore writes recoverable back onto html and returns the merged document.
	// It fails, rather than writing, when the data cannot be reproduced, because
	// a restore overwrites the user's file.
	Restore(html []byte, recoverable dataapi.Value) ([]byte, error)
	// CanRestore reports whether recoverable can be written back onto html and read
	// back out unchanged.
	CanRestore(html []byte, recoverable dataapi.Value) bool
}

// HistoryReader offers the newest version of a document whose bytes are not the
// ones named by hash, so an outside edit can be recovered from, and lets the guard
// pin the version such a write replaced.
type HistoryReader interface {
	NewestDifferingFrom(hash string) ([]byte, bool)
	// Keep stores content as a version and returns the name of an entry that holds
	// exactly those bytes.
	Keep(content []byte) (string, error)
	// Pin marks an entry as kept until the user deletes it.
	Pin(name string) error
}

// Observation is one write the guard is asked to judge.
type Observation struct {
	Key, AbsPath string
	NewHTML      []byte
	PrevHTML     []byte // nil for an outside edit
	Prov         Provenance
	IfMatched    bool
	ProvenReader bool // a program (not a page) named the exact version it read
	FromPage     bool // a browser page sent this write (its own data API write)
	History      HistoryReader
	Notify       func(kind string, event *ClientEvent, id string)
}

// PreviewRow is one "now / yours" line of the recovery panel.
type PreviewRow struct {
	Key   string `json:"key"`
	Now   string `json:"now"`
	Yours string `json:"yours"`
}

// TypedRow is one typed value a loss took, for the recovery panel.
type TypedRow struct {
	Path  string `json:"path"`
	Yours string `json:"yours"`
	At    int64  `json:"at"`
}

// ClientEvent is the event shape the panel is allowed to see. It carries no
// filesystem paths: a recover path would tell a page where the app stores things.
type ClientEvent struct {
	ID               string       `json:"id"`
	FirstDetectedAt  int64        `json:"firstDetectedAt"`
	LastWriteAt      int64        `json:"lastWriteAt"`
	LossSummary      LossSummary  `json:"lossSummary"`
	FieldCount       int          `json:"fieldCount"`
	Preview          []PreviewRow `json:"preview"`
	DroppedAdditions int          `json:"droppedAdditions"`
	Restorable       bool         `json:"restorable"`
	CanRevert        bool         `json:"canRevert"`
	Status           string       `json:"status"`
	Reason           string       `json:"reason,omitempty"`
	Typed            []TypedRow   `json:"typed,omitempty"`
	CanPutBack       bool         `json:"canPutBack"`
}

// outside reports a write that no page sent and that showed no proof of reading
// the version it was based on.
func (o Observation) outside() bool { return o.Prov == External && !o.FromPage && !o.ProvenReader }

// Guard judges one document's writes against the baseline it last observed.
type Guard struct {
	store *Store
	ext   Extractor
	now   func() int64
	queue *Queue
}

// NewGuard returns a guard that keeps state through store, reads islands with
// ext, and stamps times with now. A nil now reads the wall clock.
func NewGuard(store *Store, ext Extractor, now func() int64) *Guard {
	if now == nil {
		now = wallClockMillis
	}
	return &Guard{store: store, ext: ext, now: now, queue: &Queue{}}
}

// SetQueue makes the guard run its resolves on q, so the server can serialize an
// observe and a resolve for one document through the same queue.
func (g *Guard) SetQueue(q *Queue) {
	if q != nil {
		g.queue = q
	}
}

// Observe runs one detection pass for one write. It never fails a write: every
// problem is logged and answered with no event.
func (g *Guard) Observe(o Observation) (*ClientEvent, error) {
	if o.Key == "" {
		log.Printf("dataguard: %s has no history key; guard skipped", o.AbsPath)
		return nil, nil
	}

	inc, extractErr := g.ext.Extract(o.NewHTML)
	if extractErr != nil {
		log.Printf("dataguard: could not read the data of %s: %v", o.AbsPath, extractErr)
		inc = nil
	}
	hash := versions.Hash(o.NewHTML)

	st, err := g.store.Read(o.Key, o.AbsPath)
	if err != nil {
		return nil, err
	}

	if extractErr != nil && st.Baseline == nil {
		seed, _ := g.ext.Extract(o.PrevHTML)
		if IsEmptyIsland(seed) {
			return nil, nil
		}
	}
	// Cold start: seed from KNOWN-GOOD content only.
	if st.Baseline == nil {
		if o.PrevHTML != nil {
			seed, seedErr := g.ext.Extract(o.PrevHTML)
			if seedErr == nil && seed != nil && !IsEmptyIsland(seed) {
				st.Baseline = g.baselineOf(seed)
				st.UIWorkPending = false
			} else {
				seedBlind(st, inc, g.now())
				st.LastSeenHash = hash
				return nil, g.store.Write(o.Key, o.AbsPath, st)
			}
		} else {
			seedBlind(st, inc, g.now())
			st.LastSeenHash = hash
			return nil, g.store.Write(o.Key, o.AbsPath, st)
		}
	}

	base, err := decodeIsland(st.Baseline.Data)
	if err != nil {
		log.Printf("dataguard: could not read the stored baseline of %s: %v", o.AbsPath, err)
		return nil, nil
	}

	normalized := NormalizeComposedBaseline(base, o.PrevHTML)
	baseObject, _ := base.(*dataapi.Object)
	normalizedObject, _ := normalized.(*dataapi.Object)
	if normalizedObject != baseObject {
		base = normalized
		at := st.Baseline.At
		st.Baseline = g.baselineOf(base)
		st.Baseline.At = at
	}
	if extractErr != nil && IsEmptyIsland(base) {
		return nil, nil
	}

	if st.Event != nil {
		// The loss is undone only when the current island still CONTAINS every
		// pinned recoverable atom (additions allowed).
		D := g.classify(base, inc)
		recoverable, err := decodeIsland(st.Event.RecoverableData)
		if err != nil {
			return nil, err
		}
		comparable := NormalizeComposedBaseline(recoverable, o.NewHTML)
		if lossUndone(g.classify(comparable, inc)) {
			id := st.Event.ID
			st.Event = nil
			st.Status = "restored"
			st.UIWorkPending = false
			st.Baseline = g.baselineOf(inc)
			st.LastSeenHash = hash
			if err := g.store.Write(o.Key, o.AbsPath, st); err != nil {
				return nil, err
			}
			if o.Notify != nil {
				o.Notify("resolved", nil, id)
			}
			return nil, nil
		}
		fire := ShouldFire(o.Prov, st.UIWorkPending && !o.IfMatched, base, inc, D, o.ProvenReader)
		verdict := OutsideVerdict{}
		if o.outside() {
			if IsSheet(base) || IsSheet(inc) {
				fire = ShouldFire(o.Prov, st.UIWorkPending && !o.IfMatched, base, inc, D, o.ProvenReader)
			} else {
				verdict = JudgeOutside(base, inc, D, st.Typed, g.now())
				fire = verdict.Fire
			}
		}
		if fire {
			st.Event.LastWriteAt = g.now()
			if o.outside() {
				st.Event.Typed = unionTyped(st.Event.Typed, verdict.Gone)
				if verdict.Reason != "" {
					st.Event.Reason = verdict.Reason
				}
			}
		}
		if o.Prov == External {
			st.UIWorkPending = false
		}
		st.LastSeenHash = hash
		return nil, g.store.Write(o.Key, o.AbsPath, st)
	}

	D := g.classify(base, inc)
	verdict := OutsideVerdict{}
	fire := ShouldFire(o.Prov, st.UIWorkPending && !o.IfMatched, base, inc, D, o.ProvenReader)
	if o.outside() {
		if IsSheet(base) || IsSheet(inc) {
			fire = ShouldFire(o.Prov, st.UIWorkPending && !o.IfMatched, base, inc, D, o.ProvenReader)
		} else {
			verdict = JudgeOutside(base, inc, D, st.Typed, g.now())
			fire = verdict.Fire
		}
	}
	// An outside write that removed data without firing is one the guard judged
	// intended: a person typed here long ago, or the page carried its own work
	// over. Silence is not permission to lose the copy, so the version it replaced
	// is pinned and pruning can never take it. When no copy can be kept, the guard
	// cannot stay silent.
	if !fire && o.outside() && (D.AnyDestruction || len(DropGone(st.Typed, inc)) < len(st.Typed)) {
		fire = !g.pinReplaced(o, hash)
	}

	if fire {
		hasCopy := g.captureRecover(o, hash)
		id, err := newEventID()
		if err != nil {
			return nil, err
		}
		st.Event = &Event{
			ID:              id,
			RecoverableData: st.Baseline.Data,
			HasRecoverCopy:  hasCopy,
			FirstDetectedAt: g.now(),
			LastWriteAt:     g.now(),
			LossSummary:     LossSummaryOf(o.Prov, D),
			Status:          "pending",
			Reason:          verdict.Reason,
			Typed:           verdict.Gone,
		}
		st.Status = "pending"
		if o.Prov == External {
			st.UIWorkPending = false
		}
		st.LastSeenHash = hash
		if err := g.store.Write(o.Key, o.AbsPath, st); err != nil {
			return nil, err
		}
		event := g.clientEvent(st.Event, inc, o.NewHTML)
		if o.Notify != nil {
			o.Notify("raised", event, "")
		}
		return event, nil
	}

	if o.Prov == UIGestured {
		prev := base
		if o.PrevHTML != nil {
			if p, err := g.ext.Extract(o.PrevHTML); err == nil && p != nil {
				prev = p
			}
		}
		st.Typed = UpdateTyped(prev, inc, st.Typed, g.now())
		st.Baseline = g.baselineOf(inc)
		st.UIWorkPending = true
	} else if o.Prov == External {
		st.UIWorkPending = false
		if o.outside() || o.ProvenReader {
			st.Typed = DropGone(st.Typed, inc)
			if inc != nil && !IsEmptyIsland(inc) {
				st.Baseline = g.baselineOf(inc)
			}
		}
	}
	st.LastSeenHash = hash
	return nil, g.store.Write(o.Key, o.AbsPath, st)
}

// captureRecover copies the last-good whole file into the guard's own storage,
// preferring the pre-write body and falling back to the newest version that is not
// the bytes just written. It reports whether a copy is stored.
func (g *Guard) captureRecover(o Observation, hash string) bool {
	body := o.PrevHTML
	if body == nil {
		if o.History == nil {
			return false
		}
		found, ok := o.History.NewestDifferingFrom(hash)
		if !ok {
			return false
		}
		body = found
	}
	if err := g.store.WriteRecover(o.Key, o.AbsPath, body); err != nil {
		log.Printf("dataguard: could not store the recover copy of %s: %v", o.AbsPath, err)
		return false
	}
	return true
}

// pinReplaced pins the version an outside write replaced, so the one copy of the
// data the guard stayed silent about is kept until the user deletes it. The bytes
// are found the way a recover copy finds them: the pre-write body when there is
// one, else the newest version that is not the bytes just written. It reports
// whether that copy is now kept and pinned. A failure here is logged and never
// fails the observation.
func (g *Guard) pinReplaced(o Observation, hash string) bool {
	if o.History == nil {
		return false
	}
	body := o.PrevHTML
	if body == nil {
		found, ok := o.History.NewestDifferingFrom(hash)
		if !ok {
			return false
		}
		body = found
	}
	name, err := o.History.Keep(body)
	if err != nil {
		log.Printf("dataguard: could not keep the version %s replaced: %v", o.AbsPath, err)
		return false
	}
	if err := o.History.Pin(name); err != nil {
		log.Printf("dataguard: could not pin version %s of %s: %v", name, o.AbsPath, err)
		return false
	}
	return true
}

func (g *Guard) baselineOf(island dataapi.Value) *Baseline {
	island = StoredIsland(island)
	data, err := dataapi.Marshal(island)
	if err != nil {
		log.Printf("dataguard: could not encode an island: %v", err)
		data = []byte("null")
	}
	return &Baseline{Data: data, Hash: islandHash(island), At: g.now()}
}

// classify logs the cost-guard fallback, which the pure core only flags.
func (g *Guard) classify(base, inc dataapi.Value) *Destruction {
	D := ClassifyDestruction(base, inc)
	if D.FuzzyFallback {
		log.Printf("dataguard: item list too large for fuzzy matching; used a coarser index-aligned compare")
	}
	return D
}

func seedBlind(st *State, island dataapi.Value, at int64) {
	island = StoredIsland(island)
	if island != nil && !IsEmptyIsland(island) {
		data, err := dataapi.Marshal(island)
		if err != nil {
			log.Printf("dataguard: could not encode an island: %v", err)
			data = []byte("null")
		}
		st.Baseline = &Baseline{Data: data, Hash: islandHash(island), At: at}
	}
	st.UIWorkPending = false
}

// lossUndone reports that the current island still holds every pinned recoverable
// atom. Destruction alone is too loose: it also passes when an atom was MODIFIED
// or reset to a placeholder, which would drop the notice without the data
// coming back.
func lossUndone(D *Destruction) bool {
	return !D.AnyDestruction && D.ModifiedAtoms == 0 && D.PlaceholderResets == 0
}

func decodeIsland(raw json.RawMessage) (dataapi.Value, error) {
	return decodeValue(json.NewDecoder(bytes.NewReader(raw)))
}

func (g *Guard) clientEvent(event *Event, current dataapi.Value, currentHTML []byte) *ClientEvent {
	recoverable, err := decodeIsland(event.RecoverableData)
	if err != nil {
		log.Printf("dataguard: could not read an event's recoverable data: %v", err)
		recoverable = nil
	}
	rows := IslandPreview(recoverable, current)
	return &ClientEvent{
		ID:               event.ID,
		FirstDetectedAt:  event.FirstDetectedAt,
		LastWriteAt:      event.LastWriteAt,
		LossSummary:      event.LossSummary,
		FieldCount:       len(rows),
		Preview:          rows,
		DroppedAdditions: DroppedItems(OrdinaryData(recoverable), OrdinaryData(current)),
		Restorable:       !HasCodecs(recoverable) && g.ext.CanRestore(currentHTML, recoverable),
		CanRevert:        event.HasRecoverCopy,
		CanPutBack:       canPutBack(event.Typed, current, recoverable),
		Status:           event.Status,
		Reason:           event.Reason,
		Typed:            typedRows(event.Typed),
	}
}

// canPutBack reports whether at least one typed value can still be written back
// into a place the current page kept.
func canPutBack(typed []TypedValue, current, recoverable dataapi.Value) bool {
	if len(typed) == 0 || current == nil || recoverable == nil {
		return false
	}
	_, placed := PutBack(current, typed, recoverable)
	return placed > 0
}

// typedRows is the newest typed values a loss took, capped for the panel.
func typedRows(typed []TypedValue) []TypedRow {
	if len(typed) == 0 {
		return nil
	}
	order := make([]int, len(typed))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return typed[order[a]].At > typed[order[b]].At })
	n := len(order)
	if n > 12 {
		n = 12
	}
	rows := make([]TypedRow, n)
	for i, idx := range order[:n] {
		rows[i] = TypedRow{Path: typed[idx].Path, Yours: typed[idx].Preview, At: typed[idx].At}
	}
	return rows
}

// islandField reads one top-level value of a wrapped island, the way the JS reads
// recoverable[RULES_NAME]. A nil or unwrapped island has no such field.
func islandField(island dataapi.Value, key string) dataapi.Value {
	obj, ok := island.(*dataapi.Object)
	if !ok {
		return nil
	}
	val, _ := obj.Get(key)
	return val
}

func buildPreview(recoverableData, currentData dataapi.Value) []PreviewRow {
	rows := []PreviewRow{}
	rd := previewContainer(recoverableData)
	cd := previewContainer(currentData)

	rdList, rdIsList := rd.([]dataapi.Value)
	cdList, cdIsList := cd.([]dataapi.Value)
	if rdIsList || cdIsList {
		yoursN, nowN := 0, 0
		if rdIsList {
			yoursN = len(rdList)
		}
		if cdIsList {
			nowN = len(cdList)
		}
		if yoursN != nowN {
			rows = append(rows, PreviewRow{Key: "items", Now: countLabel(nowN), Yours: countLabel(yoursN)})
		}
		return rows
	}

	for _, key := range previewKeys(rd, cd) {
		rdVal, _ := objectLookup(rd, key)
		cdVal, _ := objectLookup(cd, key)
		yours := formatValue(rdVal)
		now := formatValue(cdVal)
		if yours != now {
			rows = append(rows, PreviewRow{Key: key, Now: now, Yours: yours})
		}
	}
	return rows
}

// previewContainer treats anything that is not a list or an object as no data,
// the way the panel reads a non-object recoverable island.
func previewContainer(v dataapi.Value) dataapi.Value {
	switch v.(type) {
	case []dataapi.Value, *dataapi.Object:
		return v
	}
	return dataapi.NewObject()
}

func previewKeys(rd, cd dataapi.Value) []string {
	seen := map[string]bool{}
	keys := []string{}
	for _, src := range []dataapi.Value{rd, cd} {
		switch t := src.(type) {
		case []dataapi.Value:
			for i := range t {
				if k := strconv.Itoa(i); !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		case *dataapi.Object:
			for _, k := range jsKeys(t) {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
	}
	return keys
}

func countLabel(n int) string {
	if n == 0 {
		return emptyLabel
	}
	return fmt.Sprintf("%d items", n)
}

const emptyLabel = "\u2014 empty \u2014"

const previewLimit = 80

func formatValue(v dataapi.Value) string {
	if !Meaningful(v) {
		return emptyLabel
	}
	if s, ok := v.(string); ok {
		return truncatePreview(jsTrim(s))
	}
	if items, ok := v.([]dataapi.Value); ok {
		allObjects := true
		for _, item := range items {
			if _, isObject := item.(*dataapi.Object); !isObject {
				allObjects = false
				break
			}
		}
		if allObjects {
			n := len(items)
			if n == 1 {
				return "1 item"
			}
			return fmt.Sprintf("%d items", n)
		}
		parts := make([]string, len(items))
		for i, item := range items {
			if _, isObject := item.(*dataapi.Object); isObject {
				parts[i] = Stringify(item)
			} else {
				parts[i] = jsStringOf(item)
			}
		}
		return truncatePreview(strings.Join(parts, ", "))
	}
	if _, ok := v.(*dataapi.Object); ok {
		return truncatePreview(Stringify(v))
	}
	return jsStringOf(v)
}

// jsStringOf is String(v) for the values extraction produces.
func jsStringOf(v dataapi.Value) string {
	if v == nil {
		return "null"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return Stringify(v)
}

// truncatePreview cuts at previewLimit in UTF-16 code units, the length the
// panel measures, and marks the cut with an ellipsis.
func truncatePreview(s string) string {
	units := codeUnits(s)
	if len(units) <= previewLimit {
		return s
	}
	return string(utf16.Decode(units[:previewLimit-1])) + "\u2026"
}

func wallClockMillis() int64 { return time.Now().UnixMilli() }

func newEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// IslandsEqual reports whether two islands carry the same data, whatever order
// their keys arrived in.
func IslandsEqual(a, b dataapi.Value) bool { return islandsEqual(a, b) }
