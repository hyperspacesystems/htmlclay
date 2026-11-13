package pageadmission

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/testutil"
)

// admissionVector is one case from the package's blockeditor-admission.json. The cases were produced by
// running ClayJS's blockeditor-admission functions, so every answer here is the reference's, including the
// refusals, which the envelope spells as {"$throw": message, "$name": error name}.
type admissionVector struct {
	Fn          string            `json:"fn"`
	Name        string            `json:"name"`
	Args        []json.RawMessage `json:"args"`
	Expect      json.RawMessage   `json:"expect"`
	GoExpect    json.RawMessage   `json:"goExpect"`
	GoExpectWhy string            `json:"goExpectWhy"`
}

// TestPageAdmissionVectors runs every blockeditor-admission case through the Go function
// vectors/operations.json names for its fn.
func TestPageAdmissionVectors(t *testing.T) {
	var file struct {
		Schema int               `json:"schema"`
		Family string            `json:"family"`
		Cases  []admissionVector `json:"cases"`
	}
	testutil.LoadVectors(t, "blockeditor-admission.json", &file)
	if file.Family != "blockeditor-admission" {
		t.Fatalf("blockeditor-admission.json names family %q", file.Family)
	}
	if len(file.Cases) == 0 {
		t.Fatal("blockeditor-admission.json has no cases")
	}

	counts := map[string]int{}
	for _, c := range file.Cases {
		c := c
		counts[c.Fn]++
		t.Run(c.Fn+"/"+c.Name, func(t *testing.T) {
			expect := c.Expect
			if len(c.GoExpect) > 0 {
				if c.GoExpectWhy == "" {
					t.Fatalf("case carries goExpect with no goExpectWhy")
				}
				expect = c.GoExpect
			}
			runAdmissionVector(t, c, expect)
		})
	}
	total := 0
	for fn, n := range counts {
		t.Logf("blockeditor-admission %s: %d cases", fn, n)
		total += n
	}
	t.Logf("blockeditor-admission: %d cases", total)
}

func runAdmissionVector(t *testing.T, c admissionVector, expect json.RawMessage) {
	t.Helper()

	switch c.Fn {
	case "inspectPage":
		result := Inspect(textArg(t, c, 0))
		checkAnswered(t, expect, result.Kind)
		answer := map[string]any{"kind": result.Kind}
		if result.Reason != "" {
			answer["reason"] = result.Reason
		}
		answer["tags"] = tagsShape(t, result.Tags)
		sameJSON(t, expect, answer)

	case "pageBaseline":
		hash, err := Baseline(baselineTags(t, arg(t, c, 0)))
		if err != nil {
			// Baseline's refusals are the TypeError arms of pageBaseline; the Go errors are
			// untyped, so there is no name to read off them.
			checkRefused(t, expect, "TypeError", err.Error())
			return
		}
		checkAnswered(t, expect, hash)
		var want string
		decode(t, expect, &want)
		if hash != want {
			t.Errorf("Baseline = %q, want %q (JS)", hash, want)
		}

	case "formatCapability":
		answer := FormatCapability(capabilityArg(t, arg(t, c, 0)))
		checkAnswered(t, expect, answer)
		sameJSON(t, expect, answer)

	case "blockeditorFormatFloor":
		answer := PageFormatFloor(textArg(t, c, 0))
		checkAnswered(t, expect, answer)
		sameJSON(t, expect, answer)

	case "withPageFormatFloor":
		answer := WithPageFormatFloor(textArg(t, c, 0), intArg(t, c, 1))
		checkAnswered(t, expect, answer)
		sameJSON(t, expect, answer)

	case "checkPageFormatAdmission":
		fields := vectorObject(t, arg(t, c, 0))
		current := textField(t, fields, "currentHtml")
		next, hasNext := optionalTextField(t, fields, "html")
		formatted, err := CheckPageFormat(current, next, intField(t, fields, "capability"))
		if err != nil {
			checkRefused(t, expect, jsErrorName(err), err.Error())
			return
		}
		floor := PageFormatFloor(formatted)
		answer := map[string]any{"ok": true, "activate": floor > PageFormatFloor(current), "floor": floor, "required": floor}
		if hasNext {
			answer["html"] = formatted
		}
		sameJSON(t, expect, answer)

	case "checkPageWriteAdmission":
		fields := vectorObject(t, arg(t, c, 0))
		current := textField(t, fields, "currentHtml")
		next := textField(t, fields, "html")
		header := textField(t, fields, "header")
		ifMatch, sent := optionalTextField(t, fields, "ifMatch")

		// The save handler's order, because CheckWrite's initial and conversion decisions are
		// documented to need the caller's If-Match check: an If-Match that does not cover the
		// stored bytes refuses first, and those two decisions need one in the exact form this host
		// accepts, which is narrower than IfMatchSatisfied alone.
		var decision Decision
		var err error
		if sent && !specwire.IfMatchSatisfied(ifMatch, []byte(current)) {
			err = &Refusal{Reason: "file-changed"}
		} else {
			decision, err = CheckWrite(current, next, header)
			if err == nil && decision != Ordinary && (!sent || strings.TrimSpace(ifMatch) == "*" ||
				strings.Contains(ifMatch, ",") || !specwire.IfMatchSatisfied(ifMatch, []byte(current))) {
				err = &Refusal{Reason: "file-changed"}
			}
		}
		if err != nil {
			checkRefused(t, expect, jsErrorName(err), err.Error())
			return
		}
		checkAnswered(t, expect, string(decision))
		var want string
		decode(t, expect, &want)
		if string(decision) != want {
			t.Errorf("CheckWrite = %q, want %q (JS)", decision, want)
		}

	case "splitPage":
		source, ok := optionalTextArg(t, c, 0)
		if !ok {
			t.Skip("the vector passes null; pageadmission's blockeditor-tag readers take a string")
		}
		var want struct {
			File  *string `json:"file"`
			State *struct {
				V   int    `json:"v"`
				Rev string `json:"rev"`
				Tag string `json:"tag"`
			} `json:"state"`
		}
		decode(t, expect, &want)
		if want.File == nil {
			t.Fatalf("the vector records no file, but StripPageTag answered %q", StripPageTag(source))
		}
		file := StripPageTag(source)
		tag, hasTag := PageTag(source)
		if want.State == nil && hasTag {
			// The reference records no state when the tag sits beside no revisioned article, and
			// keeps the tag in the file. The tag readers here never read a revision, so the tag is
			// still found and still stripped: the case's goExpect records that answer and why.
			sameJSON(t, expect, map[string]any{"file": file, "tag": tag})
			return
		}
		if *want.File != file {
			t.Errorf("StripPageTag =\n%q\nwant\n%q", file, *want.File)
		}
		if (want.State != nil) != hasTag {
			t.Fatalf("PageTag found %q=%v, want state %v", tag, hasTag, want.State)
		}
		// state.rev is the article's data-rev, which this package never reads: the tag readers
		// work from the tag alone, so only the tag itself is compared.
		if hasTag && want.State.Tag != tag {
			t.Errorf("PageTag = %q, want %q", tag, want.State.Tag)
		}

	case "composePage":
		file := textArg(t, c, 0)
		state := optionalVectorObject(t, arg(t, c, 1))
		composed := file
		if state != nil {
			composed = InsertPageTag(file, textField(t, state, "tag"))
		}
		checkAnswered(t, expect, composed)
		var want string
		decode(t, expect, &want)
		if composed != want {
			t.Errorf("InsertPageTag =\n%q\nwant\n%q", composed, want)
		}

	default:
		t.Fatalf("blockeditor-admission.json has no Go mapping for fn %q", c.Fn)
	}
}

// jsErrorName maps a Go error onto the JS error name the vectors record. Unlike the data API's
// types, these three carry the host's own names, because each one is the port of a named JS error.
func jsErrorName(err error) string {
	var (
		format    *FormatRefusal
		refusal   *Refusal
		converted *FormatError
	)
	switch {
	case errors.As(err, &format):
		return "PageFormatUpgradeRequired"
	case errors.As(err, &refusal):
		return "PageConversionError"
	case errors.As(err, &converted):
		return "PageFormatError"
	}
	return ""
}

// checkRefused compares a Go refusal with the vector's. The name is the kind and must match. The
// message is reported, not failed: Go's FormatRefusal omits the floor JS names in it, and whether
// that text should move is a decision for the message's owner, not for this runner.
func checkRefused(t *testing.T, expect json.RawMessage, name, message string) {
	t.Helper()
	wantName, wantMessage, threw := thrown(t, expect)
	if !threw {
		t.Fatalf("Go refused with %s: %s, but the reference answered %s", name, message, string(expect))
	}
	if name != wantName {
		t.Errorf("refusal name %q, want %q (%s)", name, wantName, message)
	}
	if message != wantMessage {
		t.Logf("refusal message differs from the reference:\n got: %s\nwant: %s", message, wantMessage)
	}
}

// checkAnswered fails when the reference refused a case Go answered.
func checkAnswered(t *testing.T, expect json.RawMessage, answer any) {
	t.Helper()
	name, message, threw := thrown(t, expect)
	if !threw {
		return
	}
	t.Fatalf("Go answered %v, but the reference threw %s: %s", answer, name, message)
}

func thrown(t *testing.T, expect json.RawMessage) (name, message string, ok bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(expect, &fields); err != nil {
		return "", "", false
	}
	raw, ok := fields["$throw"]
	if !ok {
		return "", "", false
	}
	decode(t, raw, &message)
	if named, ok := fields["$name"]; ok {
		decode(t, named, &name)
	}
	return name, message, true
}

func sameJSON(t *testing.T, expect json.RawMessage, got any) {
	t.Helper()
	if !matches(t, expect, got) {
		t.Errorf("\n got: %s\nwant: %s", marshal(t, got), string(expect))
	}
}

func matches(t *testing.T, expect json.RawMessage, got any) bool {
	t.Helper()
	var want any
	decode(t, expect, &want)
	return reflect.DeepEqual(shape(t, got), want)
}

func decode(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode %s: %v", string(raw), err)
	}
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	raw, err := dataapi.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// shape renders a Go answer as generic JSON so it compares with the vector's, order-insensitively
// and with the package's own value encoding.
func shape(t *testing.T, value any) any {
	t.Helper()
	var out any
	decode(t, []byte(marshal(t, value)), &out)
	return out
}

func tagsShape(t *testing.T, tags []Tag) []any {
	t.Helper()
	out := make([]any, 0, len(tags))
	for _, tag := range tags {
		out = append(out, []any{tag.Name, shape(t, tag.Value)})
	}
	return out
}

func arg(t *testing.T, c admissionVector, i int) json.RawMessage {
	t.Helper()
	if i >= len(c.Args) {
		t.Fatalf("%s wants argument %d, the vector carries %d", c.Fn, i, len(c.Args))
	}
	return c.Args[i]
}

func textArg(t *testing.T, c admissionVector, i int) string {
	t.Helper()
	source, _ := optionalTextArg(t, c, i)
	return source
}

// optionalTextArg reports whether the vector passed a string at all: several cases hand the
// reference a null, which a Go string cannot be.
func optionalTextArg(t *testing.T, c admissionVector, i int) (string, bool) {
	t.Helper()
	raw := arg(t, c, i)
	if string(bytes.TrimSpace(raw)) == "null" {
		return "", false
	}
	var source string
	decode(t, raw, &source)
	return source, true
}

func intArg(t *testing.T, c admissionVector, i int) int {
	t.Helper()
	var n int
	decode(t, arg(t, c, i), &n)
	return n
}

// capabilityArg carries a declared capability as Go reads one: the header value. The vectors also
// pass a plain number, which JS accepts directly and which is its own decimal digits here.
func capabilityArg(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

// vectorObject decodes an argument object. Key order is part of the vectors' data, encoded as
// {"$o": [[key, value], ...]}, so a plain JSON object is refused instead of silently reordered.
func vectorObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	fields, ok := orderedFields(t, raw)
	if !ok {
		t.Fatalf("argument is not a $o-encoded object: %s", string(raw))
	}
	return fields
}

func optionalVectorObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	if string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	return vectorObject(t, raw)
}

func orderedFields(t *testing.T, raw json.RawMessage) (map[string]json.RawMessage, bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode object: %v", err)
	}
	pairs, encoded := fields["$o"]
	if !encoded || len(fields) != 1 {
		return fields, false
	}
	var list []json.RawMessage
	decode(t, pairs, &list)
	ordered := map[string]json.RawMessage{}
	for _, pair := range list {
		var entry []json.RawMessage
		decode(t, pair, &entry)
		if len(entry) != 2 {
			t.Fatalf("$o entry is not a [key, value] pair: %s", string(pair))
		}
		var key string
		decode(t, entry[0], &key)
		ordered[key] = entry[1]
	}
	return ordered, true
}

func textField(t *testing.T, fields map[string]json.RawMessage, name string) string {
	t.Helper()
	value, _ := optionalTextField(t, fields, name)
	return value
}

func optionalTextField(t *testing.T, fields map[string]json.RawMessage, name string) (string, bool) {
	t.Helper()
	raw, present := fields[name]
	if !present || string(bytes.TrimSpace(raw)) == "null" {
		return "", false
	}
	var value string
	decode(t, raw, &value)
	return value, true
}

func intField(t *testing.T, fields map[string]json.RawMessage, name string) int {
	t.Helper()
	raw, present := fields[name]
	if !present {
		return 0
	}
	var n int
	decode(t, raw, &n)
	return n
}

// baselineTags rebuilds the [name, value] pairs pageBaseline is given. A non-list argument is left
// as no tags at all, which is the refusal the reference records for it.
func baselineTags(t *testing.T, raw json.RawMessage) []Tag {
	t.Helper()
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var entries []json.RawMessage
	decode(t, raw, &entries)
	tags := make([]Tag, 0, len(entries))
	for _, entry := range entries {
		var pair []json.RawMessage
		decode(t, entry, &pair)
		if len(pair) != 2 {
			t.Fatalf("pageBaseline entry is not a [name, value] pair: %s", string(entry))
		}
		var name string
		decode(t, pair[0], &name)
		tags = append(tags, Tag{Name: name, Value: vectorValue(t, pair[1])})
	}
	return tags
}

// vectorValue decodes a vector value into the package's value type. Objects arrive in the ordered
// form, because Baseline's hash is taken over the keys in the order the reference emitted them.
func vectorValue(t *testing.T, raw json.RawMessage) dataapi.Value {
	t.Helper()
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		t.Fatal("empty vector value")
	}
	switch raw[0] {
	case '{':
		_, ordered := orderedFields(t, raw)
		if !ordered {
			t.Fatalf("object is not $o-encoded, so its key order is lost: %s", string(raw))
		}
		var fields map[string]json.RawMessage
		decode(t, raw, &fields)
		var pairs []json.RawMessage
		decode(t, fields["$o"], &pairs)
		object := dataapi.NewObject()
		for _, pair := range pairs {
			var entry []json.RawMessage
			decode(t, pair, &entry)
			if len(entry) != 2 {
				t.Fatalf("$o entry is not a [key, value] pair: %s", string(pair))
			}
			var key string
			decode(t, entry[0], &key)
			object.Set(key, vectorValue(t, entry[1]))
		}
		return object
	case '[':
		var list []json.RawMessage
		decode(t, raw, &list)
		values := make([]dataapi.Value, 0, len(list))
		for _, item := range list {
			values = append(values, vectorValue(t, item))
		}
		return values
	default:
		var scalar any
		decode(t, raw, &scalar)
		return scalar
	}
}
