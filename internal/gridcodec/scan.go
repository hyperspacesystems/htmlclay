package gridcodec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Miss is the reader's refusal: the bytes are not canonical, so the caller
// reads the table with an HTML parser instead (or refuses a patch).
type Miss struct {
	Why string
	At  int
}

func (m *Miss) Error() string {
	return fmt.Sprintf("grid codec: %s at %d", m.Why, m.At)
}

type Table struct {
	FreezeCols int  `json:"freezeCols,omitempty"`
	Stale      bool `json:"stale,omitempty"`
}

type Col struct {
	ID     string  `json:"id"`
	Pos    string  `json:"pos"`
	Fmt    *string `json:"fmt,omitempty"`
	Hidden bool    `json:"hidden,omitempty"`
	Width  *int    `json:"width,omitempty"`
}

// A nil *Cell in Row.Cells is a bare empty <td></td>.
type Cell struct {
	Text    string  `json:"text"`
	N       bool    `json:"n,omitempty"`
	T       bool    `json:"t,omitempty"`
	F       *string `json:"f,omitempty"`
	V       *string `json:"v,omitempty"`
	Fmt     *string `json:"fmt,omitempty"`
	Colspan int     `json:"colspan,omitempty"`
	Rowspan int     `json:"rowspan,omitempty"`
	Hidden  bool    `json:"hidden,omitempty"`
	Style   *string `json:"style,omitempty"`
}

type Row struct {
	ID     string  `json:"id"`
	Pos    string  `json:"pos"`
	Frozen bool    `json:"frozen,omitempty"`
	Hidden bool    `json:"hidden,omitempty"`
	Style  *string `json:"style,omitempty"`
	Cells  []*Cell `json:"cells"`
}

// Sink receives what the reader finds; any field may be nil. Row gets the row,
// the byte range [start, end) of its <tr>...</tr> in src, and its index across
// the whole table. Group gets each <thead> or <tbody> byte range.
type Sink struct {
	Table func(Table)
	Col   func(Col)
	Row   func(row Row, start, end, index int)
	Group func(start, end int, frozen bool)
	Rule  func(rule Rule, start, end int)
	Rules func(start, end int)
}

const maxCountDigits = 7

type scanner struct {
	src    string
	cursor int
}

func strp(s string) *string { return &s }

func intp(n int) *int { return &n }

func (s *scanner) miss(why string, at int) {
	panic(&Miss{Why: why, At: at})
}

// at is charCodeAt: -1 stands in for NaN past either end, which is no space, no
// digit and no ">".
func (s *scanner) at(i int) int {
	if i < 0 || i >= len(s.src) {
		return -1
	}
	return int(s.src[i])
}

func (s *scanner) startsWith(literal string, i int) bool {
	if i < 0 || i > len(s.src) {
		return false
	}
	return strings.HasPrefix(s.src[i:], literal)
}

func (s *scanner) slice(from, to int) string {
	if from < 0 {
		from = 0
	}
	if from > len(s.src) {
		from = len(s.src)
	}
	if to > len(s.src) {
		to = len(s.src)
	}
	if to < from {
		to = from
	}
	return s.src[from:to]
}

func (s *scanner) expect(i int, literal string) int {
	if !s.startsWith(literal, i) {
		s.miss(fmt.Sprintf("expected %q", literal), i)
	}
	return i + len(literal)
}

func (s *scanner) skipSpace(i int) int {
	for {
		c := s.at(i)
		if c != 32 && c != 10 && c != 9 {
			return i
		}
		i++
	}
}

func (s *scanner) flag(i int, literal string) bool {
	if !s.startsWith(literal, i) {
		return false
	}
	next := s.at(i + len(literal))
	return next == 32 || next == 62
}

func (s *scanner) decode(raw string, at int, references [][2]string, plain string) string {
	var out strings.Builder
	out.Grow(len(raw))
	for i := 0; i < len(raw); {
		c := raw[i]
		if c == '&' {
			matched := false
			for _, ref := range references {
				if strings.HasPrefix(raw[i:], ref[0]) {
					out.WriteString(ref[1])
					i += len(ref[0])
					matched = true
					break
				}
			}
			if !matched {
				s.miss(fmt.Sprintf("character %q outside the grammar", "&"), at)
			}
			continue
		}
		if strings.IndexByte(plain, c) >= 0 {
			s.miss(fmt.Sprintf("character %q outside the grammar", string(c)), at)
		}
		out.WriteByte(c)
		i++
	}
	decoded := out.String()
	if !utf8.ValidString(decoded) {
		s.miss("text that is not valid UTF-8", at)
	}
	return decoded
}

var (
	attrReferences = [][2]string{{"&amp;", "&"}, {"&quot;", `"`}}
	textReferences = [][2]string{{"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"}}
)

func (s *scanner) readAval(i int) string {
	end := -1
	if i >= 0 && i <= len(s.src) {
		if k := strings.IndexByte(s.src[i:], '"'); k >= 0 {
			end = i + k
		}
	}
	if end == -1 {
		s.miss("open attribute value", i)
	}
	s.cursor = end + 1
	return s.decode(s.src[i:end], i, attrReferences, "\r\x00")
}

func (s *scanner) readCount(i, min int) int {
	end := i
	for end-i <= maxCountDigits {
		c := s.at(end)
		if c < 48 || c > 57 {
			break
		}
		end++
	}
	digits := s.slice(i, end)
	if len(digits) == 0 || len(digits) > maxCountDigits {
		s.miss("bad count", i)
	}
	if len(digits) > 1 && digits[0] == '0' {
		s.miss("count with a leading zero", i)
	}
	value, _ := strconv.Atoi(digits)
	if value < min {
		s.miss(fmt.Sprintf("count below %d", min), i)
	}
	s.cursor = end
	return value
}

func (s *scanner) readCol(i int) Col {
	i = s.expect(i, `<col data-id="`)
	col := Col{ID: s.readAval(i)}
	i = s.expect(s.cursor, ` data-pos="`)
	col.Pos = s.readAval(i)
	i = s.cursor
	if s.startsWith(` fmt="`, i) {
		col.Fmt = strp(s.readAval(i + 6))
		i = s.cursor
	}
	if s.flag(i, " hidden") {
		col.Hidden = true
		i += 7
	}
	if s.startsWith(` style="width:`, i) {
		col.Width = intp(s.readCount(i+14, 0))
		i = s.expect(s.cursor, `px"`)
	}
	s.cursor = s.expect(i, ">")
	return col
}

func (s *scanner) readCell(i int) *Cell {
	i = s.expect(i, "<td")
	cell := Cell{}
	bare := true
	if s.flag(i, " n") {
		cell.N = true
		i += 2
		bare = false
	}
	if s.flag(i, " t") {
		cell.T = true
		i += 2
		bare = false
	}
	if s.startsWith(` f="`, i) {
		cell.F = strp(s.readAval(i + 4))
		i = s.cursor
		bare = false
	}
	if s.startsWith(` v="`, i) {
		cell.V = strp(s.readAval(i + 4))
		i = s.cursor
		bare = false
	}
	if s.startsWith(` fmt="`, i) {
		cell.Fmt = strp(s.readAval(i + 6))
		i = s.cursor
		bare = false
	}
	if s.startsWith(` colspan="`, i) {
		cell.Colspan = s.readCount(i+10, 2)
		i = s.expect(s.cursor, `"`)
		bare = false
	}
	if s.startsWith(` rowspan="`, i) {
		cell.Rowspan = s.readCount(i+10, 2)
		i = s.expect(s.cursor, `"`)
		bare = false
	}
	if s.flag(i, " hidden") {
		cell.Hidden = true
		i += 7
		bare = false
	}
	if s.startsWith(` style="`, i) {
		cell.Style = strp(s.readAval(i + 8))
		i = s.cursor
		bare = false
	}
	i = s.expect(i, ">")
	lt := -1
	if i <= len(s.src) {
		if k := strings.IndexByte(s.src[i:], '<'); k >= 0 {
			lt = i + k
		}
	}
	if lt == -1 {
		s.miss("open cell", i)
	}
	text := s.decode(s.src[i:lt], i, textReferences, ">\r\x00")
	s.cursor = s.expect(lt, "</td>")
	if cell.Hidden && text != "" {
		s.miss("a hidden cell with text", i)
	}
	if bare && text == "" {
		return nil
	}
	cell.Text = text
	return &cell
}

func (s *scanner) readRow(i int, frozen bool) Row {
	i = s.expect(i, `<tr data-id="`)
	row := Row{ID: s.readAval(i)}
	i = s.expect(s.cursor, ` data-pos="`)
	row.Pos = s.readAval(i)
	i = s.cursor
	if frozen {
		row.Frozen = true
	}
	if s.flag(i, " hidden") {
		row.Hidden = true
		i += 7
	}
	if s.startsWith(` style="`, i) {
		row.Style = strp(s.readAval(i + 8))
		i = s.cursor
	}
	i = s.expect(i, ">")
	row.Cells = []*Cell{}
	for !s.startsWith("</tr>", i) {
		row.Cells = append(row.Cells, s.readCell(i))
		i = s.cursor
	}
	if len(row.Cells) > 0 && row.Cells[len(row.Cells)-1] == nil {
		s.miss("a row that ends with an empty cell", i)
	}
	s.cursor = i + 5
	return row
}

func (s *scanner) readGroup(i int, frozen bool, sink *Sink, index int) int {
	start := i
	closeTag := "</tbody>"
	openTag := "<tbody>"
	if frozen {
		closeTag = "</thead>"
		openTag = "<thead>"
	}
	i = s.expect(i, openTag)
	for {
		i = s.skipSpace(i)
		if s.startsWith(closeTag, i) {
			break
		}
		rowStart := i
		row := s.readRow(i, frozen)
		i = s.cursor
		if sink != nil && sink.Row != nil {
			sink.Row(row, rowStart, i, index)
		}
		index++
	}
	s.cursor = i + len(closeTag)
	if sink != nil && sink.Group != nil {
		sink.Group(start, s.cursor, frozen)
	}
	return index
}

func (s *scanner) readRule(i int) Rule {
	start := i
	i = s.expect(i, `<rule data-id="`)
	rule := Rule{ID: s.readAval(i)}
	rule.Pos = s.readAval(s.expect(s.cursor, ` data-pos="`))
	rule.Kind = s.readAval(s.expect(s.cursor, ` kind="`))
	rule.Range = s.readAval(s.expect(s.cursor, ` range="`))
	i = s.cursor
	for _, field := range ruleFields {
		if field == "open" || field == "stacked" {
			if s.flag(i, " "+field) {
				if field == "open" {
					rule.Open = true
				} else {
					rule.Stacked = true
				}
				i += len(field) + 1
			}
			continue
		}
		prefix := " " + field + `="`
		if !s.startsWith(prefix, i) {
			continue
		}
		raw := s.readAval(i + len(prefix))
		i = s.cursor
		switch field {
		case "items":
			var items []string
			if err := json.Unmarshal([]byte(raw), &items); err != nil || items == nil {
				s.miss("rule items that are not JSON", start)
			}
			canonical, err := marshalItemsJSON(items)
			if err != nil || canonical != raw {
				s.miss("rule items that are not a list written as the writer writes it", start)
			}
			rule.Items = items
		case "source":
			rule.Source = strp(raw)
		case "test":
			rule.Test = strp(raw)
		case "a":
			rule.A = strp(raw)
		case "b":
			rule.B = strp(raw)
		case "f":
			rule.F = strp(raw)
		case "style":
			rule.Style = strp(raw)
		case "low":
			rule.Low = strp(raw)
		case "mid":
			rule.Mid = strp(raw)
		case "high":
			rule.High = strp(raw)
		case "type":
			rule.Type = strp(raw)
		case "title":
			rule.Title = strp(raw)
		case "series":
			rule.Series = strp(raw)
		}
	}
	if problem := RuleProblem(&rule); problem != "" {
		s.miss(problem, start)
	}
	s.cursor = s.expect(i, "></rule>")
	return rule
}

// ScanGrid reads the grid that starts at byte offset at and returns the offset
// just past "</table>", or a *Miss.
func ScanGrid(src string, at int, sink *Sink) (end int, err error) {
	s := &scanner{src: src}
	defer func() {
		if r := recover(); r != nil {
			m, ok := r.(*Miss)
			if !ok {
				panic(r)
			}
			end, err = 0, m
		}
	}()
	return s.scanGrid(at, sink), nil
}

func (s *scanner) scanGrid(at int, sink *Sink) int {
	i := s.expect(at, "<table grid")
	table := Table{}
	if s.startsWith(` freeze-cols="`, i) {
		table.FreezeCols = s.readCount(i+14, 1)
		i = s.expect(s.cursor, `"`)
	}
	if s.flag(i, " stale") {
		table.Stale = true
		i += 6
	}
	i = s.expect(i, ">")
	if sink != nil && sink.Table != nil {
		sink.Table(table)
	}

	i = s.skipSpace(i)
	if s.startsWith("<colgroup>", i) {
		i += 10
		for {
			i = s.skipSpace(i)
			if s.startsWith("</colgroup>", i) {
				break
			}
			col := s.readCol(i)
			i = s.cursor
			if sink != nil && sink.Col != nil {
				sink.Col(col)
			}
		}
		i = s.skipSpace(i + 11)
	}

	index := 0
	if s.startsWith("<thead>", i) {
		index = s.readGroup(i, true, sink, index)
		i = s.skipSpace(s.cursor)
	}
	for s.startsWith("<tbody>", i) {
		index = s.readGroup(i, false, sink, index)
		i = s.skipSpace(s.cursor)
	}
	if s.startsWith("<template rules>", i) {
		start := i
		i += 16
		count := 0
		for {
			i = s.skipSpace(i)
			if s.startsWith("</template>", i) {
				break
			}
			ruleStart := i
			rule := s.readRule(i)
			i = s.cursor
			if sink != nil && sink.Rule != nil {
				sink.Rule(rule, ruleStart, i)
			}
			count++
		}
		if count == 0 {
			s.miss("an empty rules block", start)
		}
		i += 11
		if sink != nil && sink.Rules != nil {
			sink.Rules(start, i)
		}
		i = s.skipSpace(i)
	}
	return s.expect(i, "</table>")
}
