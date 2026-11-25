// Package commitrecord reads and folds a sheet's commit record: the one
// <script type="application/json" sheet-commit> element a ClayJS live-room sheet keeps inside its
// [sheet-workbook] region (plan 09 section 1.10; the sheet codec is sapjs src/grid/room-codec.js).
//
// HTML Clay runs no room, so it never writes a record of its own. It changes one only when a writer
// other than a tab replaces the document (a version restore, a data API write): OutsideFold is the
// Go port of ClayJS's file-only outsideFold (src/live-room/core/fileonly.js), so the open tabs learn
// the file moved to a new generation and nothing counted in the ledger is applied twice.
package commitrecord

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"golang.org/x/net/html"
)

// LedgerCap is ClayJS's REAL_LIMITS.ledgerCap: the bytes the ledger may count before it is trimmed.
const LedgerCap = 2 * 1024

// labDoc is the core's DOC, the id a record without one is given (record.js).
const labDoc = "doc0123456789abc"

// Tag is where a record sits: the whole element, and its JSON between the tags.
type Tag struct {
	Start, End         int
	BodyStart, BodyEnd int
}

// Find returns the first sheet-commit script element in doc. The attribute is matched by name, as
// the sheet codec reads it.
func Find(doc []byte) (Tag, bool) {
	if !mentionsAttr(doc) {
		return Tag{}, false
	}
	z := html.NewTokenizer(bytes.NewReader(doc))
	at := 0
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			return Tag{}, false
		}
		raw := len(z.Raw())
		if kind == html.StartTagToken && isCommitTag(z) {
			tag := Tag{Start: at, BodyStart: at + raw}
			at += raw
			for {
				kind = z.Next()
				if kind == html.ErrorToken {
					return Tag{}, false
				}
				raw = len(z.Raw())
				if kind == html.EndTagToken {
					tag.BodyEnd, tag.End = at, at+raw
					return tag, true
				}
				at += raw
			}
		}
		at += raw
	}
}

// Has reports whether doc carries a commit record: the documents the file contract applies to.
// mentionsAttr reports whether doc holds the attribute name in any letter case. Every save of every
// document asks Find, and most documents never mention it, so this skips the tokenizer for them.
func mentionsAttr(doc []byte) bool {
	const name = "sheet-commit"
	for at := 0; ; {
		i := bytes.IndexByte(doc[at:], '-')
		if i < 0 {
			return false
		}
		i += at
		if i >= 5 && i+7 <= len(doc) && bytes.EqualFold(doc[i-5:i+7], []byte(name)) {
			return true
		}
		at = i + 1
	}
}

// Ordered reports whether doc carries a readable version 2 record: the documents the file contract,
// the live stamp and the quit flush apply to. A placeholder such as {"rev":1} does not count.
func Ordered(doc []byte) bool {
	tag, ok := Find(doc)
	if !ok {
		return false
	}
	_, err := Read(doc, tag)
	return err == nil
}

func Has(doc []byte) bool {
	_, ok := Find(doc)
	return ok
}

func isCommitTag(z *html.Tokenizer) bool {
	name, more := z.TagName()
	if string(name) != "script" {
		return false
	}
	for more {
		var key []byte
		key, _, more = z.TagAttr()
		if string(key) == "sheet-commit" {
			return true
		}
	}
	return false
}

// Entry is one client's line in the ledger: the highest request number counted, and the epoch it
// was counted in.
type Entry struct {
	Client string
	N, At  int64
}

// Record is what the host reads from a record: the header fields a fold or a save check needs, and
// the ledger in the order the file holds it (trimLedger breaks ties in age by that order).
type Record struct {
	Doc     *string
	Gen     string
	Epoch   int64
	Commit  string
	Parent  *string
	Through int64
	Floor   int64
	Seen    []Entry
}

// DocID is the record's document id, with the core's default for a record that has none.
func (r *Record) DocID() string {
	if r.Doc == nil {
		return labDoc
	}
	return *r.Doc
}

// ErrUnreadable is a record whose JSON this host cannot read.
var ErrUnreadable = errors.New("the sheet's commit record cannot be read")

// Read parses the record at tag.
func Read(doc []byte, tag Tag) (*Record, error) {
	var raw struct {
		V       json.Number     `json:"v"`
		Doc     *string         `json:"doc"`
		Gen     string          `json:"gen"`
		Epoch   json.Number     `json:"epoch"`
		Commit  string          `json:"commit"`
		Parent  *string         `json:"parent"`
		Through json.Number     `json:"through"`
		Floor   json.Number     `json:"floor"`
		Seen    json.RawMessage `json:"seen"`
	}
	dec := json.NewDecoder(bytes.NewReader(doc[tag.BodyStart:tag.BodyEnd]))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if raw.V != "2" || raw.Commit == "" {
		return nil, fmt.Errorf("%w: not a version 2 record with a commit", ErrUnreadable)
	}
	rec := &Record{Doc: raw.Doc, Gen: raw.Gen, Commit: raw.Commit, Parent: raw.Parent, Floor: -1}
	var err error
	if rec.Epoch, err = number(raw.Epoch, 0); err != nil {
		return nil, err
	}
	if rec.Through, err = number(raw.Through, 0); err != nil {
		return nil, err
	}
	if rec.Floor, err = number(raw.Floor, -1); err != nil {
		return nil, err
	}
	if rec.Seen, err = readSeen(raw.Seen); err != nil {
		return nil, err
	}
	return rec, nil
}

func number(n json.Number, missing int64) (int64, error) {
	if n == "" {
		return missing, nil
	}
	v, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return v, nil
}

// readSeen reads the ledger object keeping the order a JavaScript object iterates it in.
func readSeen(raw json.RawMessage) ([]Entry, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if open, err := dec.Token(); err != nil || open != json.Delim('{') {
		return nil, fmt.Errorf("%w: seen is not an object", ErrUnreadable)
	}
	var seen []Entry
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
		}
		var pair []json.Number
		if err := dec.Decode(&pair); err != nil || len(pair) < 2 {
			return nil, fmt.Errorf("%w: a ledger entry is not [n, epoch]", ErrUnreadable)
		}
		n, errN := pair[0].Int64()
		at, errAt := pair[1].Int64()
		if errN != nil || errAt != nil {
			return nil, fmt.Errorf("%w: a ledger entry is not two integers", ErrUnreadable)
		}
		entry := Entry{Client: key.(string), N: n, At: at}
		replaced := false
		for i := range seen {
			if seen[i].Client == entry.Client {
				seen[i], replaced = entry, true
			}
		}
		if !replaced {
			seen = append(seen, entry)
		}
	}
	return jsOrder(seen), nil
}

// jsOrder puts entries in the order a JavaScript object keeps its keys: keys that are array indexes
// first, ascending, then the rest in insertion order.
func jsOrder(entries []Entry) []Entry {
	var indexes, rest []Entry
	for _, entry := range entries {
		if _, ok := arrayIndex(entry.Client); ok {
			indexes = append(indexes, entry)
		} else {
			rest = append(rest, entry)
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		a, _ := arrayIndex(indexes[i].Client)
		b, _ := arrayIndex(indexes[j].Client)
		return a < b
	})
	return append(indexes, rest...)
}

func arrayIndex(key string) (uint64, bool) {
	n, err := strconv.ParseUint(key, 10, 32)
	if err != nil || n == 1<<32-1 || strconv.FormatUint(n, 10) != key {
		return 0, false
	}
	return n, true
}

// IDs hands out the numbers a new generation and commit are named with, one counter per name, as
// the reference host's port.next does.
type IDs interface {
	Next(counter string) int64
}

type randomIDs struct {
	mu       sync.Mutex
	counters map[string]int64
}

// NewIDs starts every counter at a random number below 2^40, as the reference host does.
func NewIDs() IDs { return &randomIDs{counters: map[string]int64{}} }

func (r *randomIDs) Next(counter string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.counters[counter]
	if !ok {
		start, err := rand.Int(rand.Reader, big.NewInt(1<<40))
		if err == nil {
			n = start.Int64()
		}
	}
	n++
	r.counters[counter] = n
	return n
}

// OutsideFold returns source with its record replaced by the fold of source's and current's
// records, for a writer that replaces current with source. current must carry a record. A source
// with no record gets one inserted at the end of its [sheet-workbook] element; with no workbook,
// source is returned unchanged.
func OutsideFold(source, current []byte, ids IDs) ([]byte, error) {
	currentTag, ok := Find(current)
	if !ok {
		return source, nil
	}
	cur, err := Read(current, currentTag)
	if err != nil {
		return source, nil
	}
	var src *Record
	sourceTag, hasSource := Find(source)
	if hasSource {
		if src, err = Read(source, sourceTag); err != nil {
			src = nil
		}
	}
	body := foldBody(src, cur, ids)
	if hasSource {
		return splice(source, sourceTag.BodyStart, sourceTag.BodyEnd, body), nil
	}
	at, ok := workbookEnd(source)
	if !ok {
		return source, nil
	}
	element := `<script type="application/json" sheet-commit="">` + body + `</script>`
	return splice(source, at, at, element), nil
}

func splice(doc []byte, start, end int, middle string) []byte {
	out := make([]byte, 0, len(doc)-(end-start)+len(middle))
	out = append(out, doc[:start]...)
	out = append(out, middle...)
	return append(out, doc[end:]...)
}

// workbookEnd is the offset of the end tag closing the first [sheet-workbook] element.
func workbookEnd(doc []byte) (int, bool) {
	z := html.NewTokenizer(bytes.NewReader(doc))
	at, depth := 0, 0
	var name string
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			return 0, false
		}
		raw := len(z.Raw())
		switch {
		case depth == 0 && kind == html.StartTagToken:
			tag, more := z.TagName()
			for more {
				var key []byte
				key, _, more = z.TagAttr()
				if string(key) == "sheet-workbook" {
					name, depth = string(tag), 1
				}
			}
		case depth > 0 && kind == html.StartTagToken:
			if tag, _ := z.TagName(); string(tag) == name {
				depth++
			}
		case depth > 0 && kind == html.EndTagToken:
			if tag, _ := z.TagName(); string(tag) == name {
				if depth--; depth == 0 {
					return at, true
				}
			}
		}
		at += raw
	}
}

// foldBody is fileonly.js outsideFold's record: a new generation one epoch past the current file and
// its floor, a cut naming the replaced generation, and the two ledgers merged and trimmed.
func foldBody(source, current *Record, ids IDs) string {
	if source == nil {
		source = &Record{Floor: -1}
	}
	inSource := map[string]Entry{}
	inCurrent := map[string]Entry{}
	var order []string
	for _, entry := range source.Seen {
		inSource[entry.Client] = entry
		order = append(order, entry.Client)
	}
	for _, entry := range current.Seen {
		inCurrent[entry.Client] = entry
		if _, seen := inSource[entry.Client]; !seen {
			order = append(order, entry.Client)
		}
	}
	var merged []Entry
	for _, client := range order {
		low, hasLow := inSource[client]
		high, hasHigh := inCurrent[client]
		if !hasLow {
			low = Entry{N: 0, At: -1}
		}
		if !hasHigh {
			high = Entry{N: 0, At: -1}
		}
		dropped := !hasHigh && low.At <= current.Floor
		stale := !hasLow && high.At <= source.Floor
		if max(low.N, high.N) > 0 && !dropped && !stale {
			merged = append(merged, Entry{Client: client, N: max(low.N, high.N), At: max(low.At, high.At)})
		}
	}
	kept, floor := trimLedger(merged, max(source.Floor, current.Floor), LedgerCap)
	epoch := max(current.Epoch, floor) + 1
	doc := current.DocID()

	var b strings.Builder
	b.WriteString(`{"v":2,"doc":`)
	b.WriteString(quote(doc))
	b.WriteString(`,"gen":`)
	b.WriteString(quote(fmt.Sprintf("%d.%08d", epoch, ids.Next("nonce"))))
	fmt.Fprintf(&b, `,"epoch":%d,"commit":`, epoch)
	b.WriteString(quote(fmt.Sprintf("c%011d", ids.Next("commits"))))
	b.WriteString(`,"parent":`)
	b.WriteString(quote(current.Commit))
	b.WriteString(`,"through":0,"room":null,"mode":"file"`)
	if source.Commit != "" {
		b.WriteString(`,"restored":`)
		b.WriteString(quote(source.Commit))
	}
	b.WriteString(`,"lineage":[],"cut":[`)
	b.WriteString(quote(current.Gen))
	fmt.Fprintf(&b, `,%d,{}],"floor":%d,"seen":{`, current.Through, floor)
	for i, entry := range jsOrder(kept) {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `%s:[%d,%d]`, quote(entry.Client), entry.N, entry.At)
	}
	b.WriteString(`}}`)
	return strings.ReplaceAll(b.String(), "<", "\\u003c")
}

// trimLedger is ledger.js trimLedger: oldest entries first (ties keep their order), and when the
// ledger is over three quarters of cap it drops the oldest until it is at most half, raising the
// floor to each dropped entry's epoch.
func trimLedger(entries []Entry, floor int64, cap int) ([]Entry, int64) {
	rotate, trim := cap*3/4, cap/2
	byAge := append([]Entry(nil), entries...)
	sort.SliceStable(byAge, func(i, j int) bool { return byAge[i].At < byAge[j].At })
	size := 2
	for _, entry := range byAge {
		size += entryBytes(entry)
	}
	gone := 0
	if size > rotate {
		for ; gone < len(byAge) && size > trim; gone++ {
			size -= entryBytes(byAge[gone])
			floor = max(floor, byAge[gone].At)
		}
	}
	return byAge[gone:], floor
}

// entryBytes is record.js entryBytes: a client id counted as at least 16 characters (UTF-16 code
// units, as JavaScript counts), the two numbers, and the framing.
func entryBytes(entry Entry) int {
	return max(len(utf16.Encode([]rune(entry.Client))), 16) +
		len(strconv.FormatInt(entry.N, 10)) + len(strconv.FormatInt(entry.At, 10)) + 7
}

// quote is JSON.stringify for a string: only the quote, the backslash and control characters are
// escaped, so the bytes match what the sheet codec writes.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
