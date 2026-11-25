package gridcodec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Rule is one <rule> entry inside <template rules> (sapjs src/grid/codec/rules.js).
type Rule struct {
	ID      string   `json:"id"`
	Pos     string   `json:"pos"`
	Kind    string   `json:"kind"`
	Range   string   `json:"range"`
	Items   []string `json:"items,omitempty"`
	Source  *string  `json:"source,omitempty"`
	Open    bool     `json:"open,omitempty"`
	Test    *string  `json:"test,omitempty"`
	A       *string  `json:"a,omitempty"`
	B       *string  `json:"b,omitempty"`
	F       *string  `json:"f,omitempty"`
	Style   *string  `json:"style,omitempty"`
	Low     *string  `json:"low,omitempty"`
	Mid     *string  `json:"mid,omitempty"`
	High    *string  `json:"high,omitempty"`
	Type    *string  `json:"type,omitempty"`
	Title   *string  `json:"title,omitempty"`
	Stacked bool     `json:"stacked,omitempty"`
	Series  *string  `json:"series,omitempty"`
}

const (
	maxRuleItems  = 500
	maxItemLength = 255
	maxTitleLen   = 255
	maxBoxCol     = 16383
)

var (
	ruleColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	chartBoxPattern  = regexp.MustCompile(`^([A-Z]{1,3})([1-9][0-9]{0,6})(?::([A-Z]{1,3})([1-9][0-9]{0,6}))?$`)
)

var ruleFields = []string{"items", "source", "open", "test", "a", "b", "f", "style", "low", "mid", "high", "type", "title", "stacked", "series"}

var ruleTests = map[string][]string{
	"gt": {"a"}, "ge": {"a"}, "lt": {"a"}, "le": {"a"}, "eq": {"a"}, "ne": {"a"},
	"contains": {"a"}, "starts": {"a"}, "ends": {"a"},
	"between": {"a", "b"}, "outside": {"a", "b"},
	"empty": {}, "filled": {}, "duplicate": {}, "unique": {},
	"formula": {"f"},
}

func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func unsafeRuleText(s string) bool {
	return strings.ContainsAny(s, "\r\x00") || !utf8.ValidString(s)
}

func colLettersIndex(name string) int {
	n := 0
	for i := 0; i < len(name); i++ {
		n = n*26 + int(name[i]-'A'+1)
	}
	return n - 1
}

func validChartBox(r string) bool {
	m := chartBoxPattern.FindStringSubmatch(r)
	if m == nil {
		return false
	}
	left := colLettersIndex(m[1])
	top, _ := strconv.Atoi(m[2])
	rightCol, bottomRow := m[1], m[2]
	if m[3] != "" {
		rightCol, bottomRow = m[3], m[4]
	}
	right := colLettersIndex(rightCol)
	bottom, _ := strconv.Atoi(bottomRow)
	return right <= maxBoxCol && left <= right && (top-1) <= (bottom-1)
}

func nonEmptyPtr(p *string) bool {
	return p != nil && *p != ""
}

func ruleFieldPresent(r *Rule, key string) bool {
	switch key {
	case "items":
		return r.Items != nil
	case "source":
		return r.Source != nil
	case "open":
		return r.Open
	case "test":
		return r.Test != nil
	case "a":
		return r.A != nil
	case "b":
		return r.B != nil
	case "f":
		return r.F != nil
	case "style":
		return r.Style != nil
	case "low":
		return r.Low != nil
	case "mid":
		return r.Mid != nil
	case "high":
		return r.High != nil
	case "type":
		return r.Type != nil
	case "title":
		return r.Title != nil
	case "stacked":
		return r.Stacked
	case "series":
		return r.Series != nil
	default:
		return false
	}
}

func ruleFieldText(r *Rule, key string) *string {
	switch key {
	case "source":
		return r.Source
	case "test":
		return r.Test
	case "a":
		return r.A
	case "b":
		return r.B
	case "f":
		return r.F
	case "style":
		return r.Style
	case "low":
		return r.Low
	case "mid":
		return r.Mid
	case "high":
		return r.High
	case "type":
		return r.Type
	case "title":
		return r.Title
	case "series":
		return r.Series
	default:
		return nil
	}
}

func itemsProblem(items []string) string {
	if len(items) == 0 || len(items) > maxRuleItems {
		return fmt.Sprintf("a list rule has 1 to %d items", maxRuleItems)
	}
	for _, item := range items {
		if item == "" || utf16Len(item) > maxItemLength {
			return fmt.Sprintf("a list item is 1 to %d characters", maxItemLength)
		}
		if unsafeRuleText(item) {
			return "a list item holds a carriage return, a NUL or half a character"
		}
	}
	return ""
}

// RuleProblem returns a non-empty reason if rule violates the grammar, else "".
func RuleProblem(r *Rule) string {
	if r == nil {
		return "a rule is a record"
	}
	switch r.Kind {
	case "check", "list", "when", "scale", "chart":
	default:
		return fmt.Sprintf("unknown rule kind %q", r.Kind)
	}
	if r.ID == "" {
		return "a rule has no id"
	}
	if r.Pos == "" {
		return "a rule has no pos"
	}
	if r.Range == "" {
		return "a rule has no range"
	}
	allowed := map[string]bool{}
	need := func(keys ...string) string {
		for _, k := range keys {
			allowed[k] = true
			if !nonEmptyPtr(ruleFieldText(r, k)) {
				return fmt.Sprintf("a %s rule has no %s", r.Kind, k)
			}
		}
		return ""
	}
	problem := ""
	switch r.Kind {
	case "list":
		allowed["items"] = true
		allowed["source"] = true
		allowed["open"] = true
		if (r.Items == nil) == (r.Source == nil) {
			return "a list rule has items or a source, not both"
		}
		if r.Items != nil {
			problem = itemsProblem(r.Items)
		} else if !nonEmptyPtr(r.Source) {
			problem = "a list rule has no source"
		}
	case "when":
		if r.Test == nil {
			return "unknown rule test null"
		}
		compared, ok := ruleTests[*r.Test]
		if !ok {
			return fmt.Sprintf("unknown rule test %q", *r.Test)
		}
		allowed["test"] = true
		req := append([]string{"style"}, compared...)
		problem = need(req...)
	case "chart":
		allowed["source"] = true
		allowed["type"] = true
		allowed["title"] = true
		allowed["stacked"] = true
		allowed["series"] = true
		if !validChartBox(r.Range) {
			return "a chart's range is A1 or A1:B2, top-left cell first, within XFD and row 9999999, with no $"
		}
		if !nonEmptyPtr(r.Source) {
			return "a chart rule has no source"
		}
		if r.Type == nil {
			return "unknown chart type null"
		}
		switch *r.Type {
		case "column", "bar", "line", "area", "pie", "scatter":
		default:
			return fmt.Sprintf("unknown chart type %q", *r.Type)
		}
		if r.Title != nil && (*r.Title == "" || utf16Len(*r.Title) > maxTitleLen || unsafeRuleText(*r.Title)) {
			problem = fmt.Sprintf("a chart title is 1 to %d characters with no carriage return, NUL or half a character", maxTitleLen)
		}
		if problem == "" && r.Stacked {
			if *r.Type != "column" && *r.Type != "bar" && *r.Type != "area" {
				problem = "stacked is true or absent, and only on a column, bar or area chart"
			}
		}
		if problem == "" && r.Series != nil && *r.Series != "rows" {
			problem = "series is rows or absent"
		}
	case "scale":
		problem = need("low", "high")
		if r.Mid != nil {
			allowed["mid"] = true
		}
		for _, k := range []string{"low", "mid", "high"} {
			if problem == "" {
				if p := ruleFieldText(r, k); p != nil && !ruleColorPattern.MatchString(*p) {
					problem = fmt.Sprintf("%s is a color written #rrggbb", k)
				}
			}
		}
	}
	if problem != "" {
		return problem
	}
	for _, field := range ruleFields {
		if ruleFieldPresent(r, field) && !allowed[field] {
			return fmt.Sprintf("%s is not a field of a %s rule", field, r.Kind)
		}
	}
	return ""
}

func marshalItemsJSON(items []string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(items); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// RuleHTML writes one rule as canonical bytes.
func RuleHTML(rule Rule) (string, error) {
	if problem := RuleProblem(&rule); problem != "" {
		return "", fmt.Errorf("grid codec: %s", problem)
	}
	var b strings.Builder
	for _, pair := range [][2]string{
		{"data-id", rule.ID},
		{"data-pos", rule.Pos},
		{"kind", rule.Kind},
		{"range", rule.Range},
	} {
		a, err := attr(pair[0], pair[1])
		if err != nil {
			return "", err
		}
		b.WriteString(a)
	}
	for _, field := range ruleFields {
		if !ruleFieldPresent(&rule, field) {
			continue
		}
		if field == "open" || field == "stacked" {
			b.WriteString(" " + field)
			continue
		}
		var val string
		if field == "items" {
			var err error
			val, err = marshalItemsJSON(rule.Items)
			if err != nil {
				return "", err
			}
		} else {
			val = *ruleFieldText(&rule, field)
		}
		a, err := attr(field, val)
		if err != nil {
			return "", err
		}
		b.WriteString(a)
	}
	return "<rule" + b.String() + "></rule>", nil
}

// RulesHTML writes a table's <template rules> block, or "" when there are no rules.
func RulesHTML(rules []Rule) (string, error) {
	if len(rules) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		html, err := RuleHTML(rule)
		if err != nil {
			return "", err
		}
		lines = append(lines, html)
	}
	return "<template rules>\n" + strings.Join(lines, "\n") + "\n</template>", nil
}
