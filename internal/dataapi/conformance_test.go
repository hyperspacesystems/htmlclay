package dataapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

const corpusDir = "testdata/conformance"

// caseMeta is the case classification scripts/conformance.mjs used to write as a flat `key: value`
// sidecar beside each case, rebuilt from the fields data-api.json carries.
type caseMeta struct {
	tier   string
	face   string
	token  string
	expect string
	skip   map[string]string
}

// skipReason returns why this host skips the case, if it does. A skip names its host, so a case can
// be skipped here while still pinning the reference's behavior over there — which is exactly what
// the two deliberately-fixed props warts need.
func (m caseMeta) skipReason() string { return m.skip["htmlclay"] }

// conformanceCase is one case out of the package's data-api.json. That file is the packed form of
// what `make sync-conformance` used to copy into the case directory: the .html, the optional
// .rules and .data.json, the meta fields, and every artifact the reference produced, keyed by the
// suffix the case directory spelled as a separate file.
type conformanceCase struct {
	meta  caseMeta
	files map[string]string
}

type conformanceCorpus struct {
	order []string
	cases map[string]*conformanceCase
}

var (
	corpusOnce sync.Once
	corpusData *conformanceCorpus
)

func conformance(t testing.TB) *conformanceCorpus {
	t.Helper()
	corpusOnce.Do(func() { corpusData = loadConformance(t) })
	return corpusData
}

func loadConformance(t testing.TB) *conformanceCorpus {
	t.Helper()

	var packed []struct {
		Name   string   `json:"name"`
		HTML   string   `json:"html"`
		Rules  *string  `json:"rules"`
		Face   string   `json:"face"`
		Tier   int      `json:"tier"`
		Skip   []string `json:"skip"`
		Expect struct {
			Data  json.RawMessage            `json:"data"`
			Files map[string]json.RawMessage `json:"files"`
		} `json:"expect"`
	}
	testutil.LoadVectors(t, "data-api.json", &packed)
	if len(packed) < 83 {
		t.Fatalf("data-api.json carries %d cases, want at least the 83 the corpus held", len(packed))
	}

	corpus := &conformanceCorpus{cases: map[string]*conformanceCase{}}
	for _, c := range packed {
		if _, seen := corpus.cases[c.Name]; seen {
			t.Fatalf("data-api.json names %s twice", c.Name)
		}
		meta := caseMeta{tier: strconv.Itoa(c.Tier), face: c.Face, expect: "ok", token: "api", skip: map[string]string{}}
		if _, failed := c.Expect.Files["error.json"]; failed {
			meta.expect = "error"
		}
		for _, line := range c.Skip {
			host, reason, _ := strings.Cut(line, "=")
			meta.skip[strings.TrimSpace(host)] = strings.TrimSpace(reason)
		}
		files := map[string]string{".html": c.HTML}
		if c.Rules != nil {
			files[".rules"] = *c.Rules
		}
		if len(c.Expect.Data) > 0 {
			files[".data.json"] = compactJSON(t, c.Name+".data.json", c.Expect.Data)
		}
		for suffix, raw := range c.Expect.Files {
			if strings.HasSuffix(suffix, ".html") {
				var text string
				if err := json.Unmarshal(raw, &text); err != nil {
					t.Fatalf("data-api.json: %s.%s: %v", c.Name, suffix, err)
				}
				files["."+suffix] = text
				continue
			}
			files["."+suffix] = compactJSON(t, c.Name+"."+suffix, raw)
		}
		corpus.cases[c.Name] = &conformanceCase{meta: meta, files: files}
		corpus.order = append(corpus.order, c.Name)
	}
	return corpus
}

// compactJSON strips the whitespace from one expected artifact without touching its key order: the
// assertions compare through the package's own decoder and marshaller, so a reordered object would
// fail an extraction the reference got right.
func compactJSON(t testing.TB, what string, raw json.RawMessage) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		t.Fatalf("data-api.json: %s: %v", what, err)
	}
	return out.String()
}

func caseNames(t testing.TB) []string {
	t.Helper()
	return conformance(t).order
}

func parseMeta(t testing.TB, name string) caseMeta {
	t.Helper()
	c := conformance(t).cases[name]
	if c == nil {
		t.Fatalf("%s is not in data-api.json", name)
	}
	return c.meta
}

func readCaseFile(t testing.TB, name, ext string) (string, bool) {
	t.Helper()
	c := conformance(t).cases[name]
	if c == nil {
		t.Fatalf("%s is not in data-api.json", name)
	}
	source, ok := c.files[ext]
	return source, ok
}

// TestConformanceParseContract is the phase-2 half of the corpus: for every case whose rules come
// from a .rules file, ParseRelaxed must produce exactly the tree the JS engine produced.
//
// A case with no .parsed.json is one where parsing itself failed, and the assertion flips: the
// port has to fail too, with the same error TYPE. The message text is deliberately not compared —
// it wraps Go's JSON diagnostics rather than V8's, and htmlclay maps status by type. Extraction
// (.expected.json / .error.json) is phase 3 and is not touched here.
func TestConformanceParseContract(t *testing.T) {
	var checked, negative int

	for _, name := range caseNames(t) {
		meta := parseMeta(t, name)
		source, hasRules := readCaseFile(t, name, ".rules")
		if !hasRules {
			// face: tag — rules live in the document and need the script-tag reader (phase 3).
			continue
		}

		t.Run(name, func(t *testing.T) {
			if r := meta.skipReason(); r != "" {
				t.Skip(r)
			}

			got, err := ParseRelaxed(source)
			want, hasParsed := readCaseFile(t, name, ".parsed.json")

			if !hasParsed {
				if err == nil {
					t.Fatalf("ParseRelaxed(%q) succeeded, but the reference failed to parse it", source)
				}
				var rpe *RulesParseError
				if !errors.As(err, &rpe) {
					t.Fatalf("error is %T, want *RulesParseError: %v", err, err)
				}
				negative++
				return
			}

			if err != nil {
				t.Fatalf("ParseRelaxed(%q) = %v, want it to parse", source, err)
			}

			// Both sides go through the same decoder and marshaller, so this compares structure
			// and key ORDER without depending on the corpus's indentation.
			wantTree, err := decodeJSON(want)
			if err != nil {
				t.Fatalf("corpus file %s.parsed.json is not valid JSON: %v", name, err)
			}
			gotJSON, err := Marshal(got)
			if err != nil {
				t.Fatalf("Marshal(got): %v", err)
			}
			wantJSON, err := Marshal(wantTree)
			if err != nil {
				t.Fatalf("Marshal(want): %v", err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("rules: %s\n got: %s\nwant: %s", source, gotJSON, wantJSON)
			}
			checked++
		})
	}

	t.Logf("parse contract: %d cases matched, %d rejected as expected", checked, negative)
	if checked == 0 {
		t.Fatal("no cases were checked — the corpus is not wired up")
	}
}

// errNoRulesTag stands in for the library returning (nil, nil) when a page publishes no matching
// rules tag. That is not an engine error — the HTTP status is the server's call — but the corpus
// records the host's synthetic NoRulesTag, so the test needs something to compare against.
var errNoRulesTag = errors.New("no matching rules tag")

// jsErrorName maps a Go error onto the JS `err.name` the corpus recorded. Types are compared, not
// messages: message text is Go's, and the JS hosts' own status mapping sniffs those strings, which
// is the bug this port declines to reproduce.
func jsErrorName(err error) string {
	var (
		parse    *RulesParseError
		depth    *MaxRuleDepthExceeded
		version  *UnknownRulesVersion
		selector SelectorFailure
		token    *InvalidRulesToken
	)
	switch {
	case errors.Is(err, errNoRulesTag):
		return "NoRulesTag"
	case errors.As(err, &parse):
		return "RulesParseError"
	case errors.As(err, &depth):
		return "MaxRuleDepthExceeded"
	case errors.As(err, &version):
		return "UnknownRulesVersion"
	// css-select throws a plain Error for a selector it cannot parse, so the reference recorded the
	// bare name. Both selector failures land here, which does NOT hide the gate's divergences: a
	// gate rejection of a selector the reference ANSWERED shows up as "the reference succeeded", a
	// different and louder failure. This branch only covers cases where both sides errored.
	case errors.As(err, &selector), errors.As(err, &token):
		return "Error"
	}
	return fmt.Sprintf("%T", err)
}

func wantErrorType(t *testing.T, name string) string {
	t.Helper()
	raw, ok := readCaseFile(t, name, ".error.json")
	if !ok {
		return ""
	}
	var e struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("%s.error.json: %v", name, err)
	}
	return e.Type
}

// TestConformanceExtraction is the phase-3 half: resolve each case's rules the way its face says,
// extract, and compare against what the JS engine produced. It mirrors scripts/conformance.mjs's
// runCase step for step so the two stay comparable.
func TestConformanceExtraction(t *testing.T) {
	var ok, errored, skipped int

	for _, name := range caseNames(t) {
		meta := parseMeta(t, name)

		t.Run(name, func(t *testing.T) {
			if r := meta.skipReason(); r != "" {
				skipped++
				t.Skip(r)
			}

			if meta.face == "write" || meta.face == "write-query" {
				skipped++
				t.Skip("write face: covered by the write conformance test")
			}

			parsedDoc := parseCaseHTML(t, name)

			rules, err := resolveRules(t, name, meta, parsedDoc)
			if err == nil {
				var extracted Value
				extracted, err = parsedDoc.Extract(rules)
				if err == nil {
					want, hasExpected := readCaseFile(t, name, ".expected.json")
					if !hasExpected {
						t.Fatalf("extraction succeeded, but the reference produced %s", wantErrorType(t, name))
					}
					assertSameJSON(t, name, extracted, want)
					ok++
					return
				}
			}

			want := wantErrorType(t, name)
			if want == "" {
				t.Fatalf("extraction failed with %v, but the reference succeeded", err)
			}
			if got := jsErrorName(err); got != want {
				t.Fatalf("error type %q, want %q (from %v)", got, want, err)
			}
			errored++
		})
	}

	t.Logf("extraction: %d matched, %d errored as expected, %d skipped", ok, errored, skipped)
}

// resolveRules is the face split: query cases read a .rules file, tag cases read the document. A
// tag case with no matching script is not an engine error — the JS returns null and the host turns
// that into its own 400 — so the corpus records a synthetic NoRulesTag and this mirrors it.
func resolveRules(t *testing.T, name string, meta caseMeta, d *Document) (Value, error) {
	t.Helper()

	if meta.face == "tag" {
		found, err := d.FindRulesIn(meta.token)
		if err != nil {
			return nil, err
		}
		if found == nil {
			return nil, errNoRulesTag
		}
		return found.Rules, nil
	}

	source, hasRules := readCaseFile(t, name, ".rules")
	if !hasRules {
		t.Fatalf("query-face case has no .rules file")
	}
	return ParseRelaxed(source)
}

func parseCaseHTML(t *testing.T, name string) *Document {
	t.Helper()
	source, ok := readCaseFile(t, name, ".html")
	if !ok {
		t.Fatalf("%s has no .html", name)
	}
	d, err := Parse(strings.NewReader(source))
	if err != nil {
		t.Fatalf("parse %s.html: %v", name, err)
	}
	return d
}

// assertSameJSON compares through the package's own decoder and marshaller, so key order counts
// but the corpus's indentation does not.
func assertSameJSON(t *testing.T, name string, got Value, wantRaw string) {
	t.Helper()
	wantTree, err := decodeJSON(wantRaw)
	if err != nil {
		t.Fatalf("corpus file for %s is not valid JSON: %v", name, err)
	}
	gotJSON, err := Marshal(got)
	if err != nil {
		t.Fatalf("Marshal(got): %v", err)
	}
	wantJSON, err := Marshal(wantTree)
	if err != nil {
		t.Fatalf("Marshal(want): %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

// TestConformanceCorpusIsCurrent guards the failure mode a vendored corpus has: it goes stale
// quietly and the suite keeps passing against last month's contract. VERSION is written by
// `make sync-conformance` and carries the manifest it was copied with.
func TestConformanceCorpusIsCurrent(t *testing.T) {
	version, err := os.ReadFile(filepath.Join(corpusDir, "VERSION"))
	if err != nil {
		t.Fatalf("no VERSION — run `make sync-conformance`: %v", err)
	}
	i := strings.Index(string(version), "{")
	if i < 0 {
		t.Fatal("VERSION has no manifest block")
	}

	var manifest struct {
		Engine  string `json:"engine"`
		Cheerio string `json:"cheerio"`
		Cases   int    `json:"cases"`
	}
	if err := json.Unmarshal(version[i:], &manifest); err != nil {
		t.Fatalf("VERSION manifest is not valid JSON: %v", err)
	}

	if n := len(caseNames(t)); n != manifest.Cases {
		t.Errorf("corpus holds %d cases, VERSION says %d — re-run `make sync-conformance`", n, manifest.Cases)
	}
	if manifest.Engine == "" || manifest.Cheerio == "" {
		t.Errorf("VERSION does not pin both versions: %+v", manifest)
	}
}
