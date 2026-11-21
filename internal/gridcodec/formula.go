package gridcodec

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// FrameSheet is one tab in a formula frame: its id, its name, and its row and column record ids in order.
type FrameSheet struct {
	ID   string
	Name string
	Rows []string
	Cols []string
}

type frameSheet struct {
	id    string
	name  string
	rows  []string
	cols  []string
	rowAt map[string]int
	colAt map[string]int
}

// Frame is sapjs frameFrom: every tab by id, in the order given.
type Frame struct {
	order []*frameSheet
	byID  map[string]*frameSheet
	at    map[string]int
}

// ErrFormulaTooDeep is returned when a formula nests deeper than maxFormulaDepth.
var ErrFormulaTooDeep = errors.New("a formula nests too deeply to read")

// The tokenizer: text after "=" to tokens, each with a [start, end) span into
// that text. The matchers classify one token at a time and never rewrite.

const maxFormulaDepth = 1000

var errFormulaSyntax = errors.New("#ERROR!")

type token struct {
	typ     string
	start   int
	end     int
	value   string
	number  float64
	boolean bool
	code    string
	sheet   *string
	col     string
	row     float64
	absCol  bool
	absRow  bool
}

type scan struct {
	tokens []token
	next   int
}

func unit(src []uint16, i int) uint16 {
	if i < 0 || i >= len(src) {
		return 0
	}
	return src[i]
}

func isDigit(ch uint16) bool { return ch >= '0' && ch <= '9' }

func isLetter(ch uint16) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isSpace(ch uint16) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func isWordChar(ch uint16) bool {
	return isLetter(ch) || isDigit(ch) || ch == '_' || ch == '.'
}

// jsString is the formula's text as JavaScript holds it: UTF-16 code units.
func jsString(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func jsText(units []uint16) string {
	return string(utf16.Decode(units))
}

func jsSlice(units []uint16, from, to int) string {
	if from < 0 {
		from = 0
	}
	if to > len(units) {
		to = len(units)
	}
	if from > to {
		return ""
	}
	return jsText(units[from:to])
}

// matchNumber is NUMBER: the length of the greedy match anchored at start, 0 for none.
func matchNumber(src []uint16, start int) int {
	i := start
	matched := false
	if isDigit(unit(src, i)) {
		for isDigit(unit(src, i)) {
			i++
		}
		if unit(src, i) == '.' {
			i++
			for isDigit(unit(src, i)) {
				i++
			}
		}
		matched = true
	} else if unit(src, i) == '.' && isDigit(unit(src, i+1)) {
		i += 2
		for isDigit(unit(src, i)) {
			i++
		}
		matched = true
	}
	if !matched {
		return 0
	}
	if ch := unit(src, i); ch == 'e' || ch == 'E' {
		j := i + 1
		if ch := unit(src, j); ch == '+' || ch == '-' {
			j++
		}
		if isDigit(unit(src, j)) {
			for isDigit(unit(src, j)) {
				j++
			}
			i = j
		}
	}
	return i - start
}

// matchError is ERROR: the length of the match anchored at start, 0 for none.
func matchError(src []uint16, start int) int {
	i := start
	if unit(src, i) != '#' {
		return 0
	}
	i++
	j := i
	for ch := unit(src, j); isLetter(ch) || isDigit(ch) || ch == '/'; ch = unit(src, j) {
		j++
	}
	if j > i {
		if ch := unit(src, j); ch == '!' || ch == '?' {
			return j + 1 - start
		}
	}
	if ch := unit(src, i); ch == 'N' || ch == 'n' {
		if unit(src, i+1) == '/' {
			if ch := unit(src, i+2); ch == 'A' || ch == 'a' {
				return i + 3 - start
			}
		}
	}
	return 0
}

// matchWord is WORD: the length of the match anchored at start, 0 for none.
func matchWord(src []uint16, start int) int {
	if ch := unit(src, start); !isLetter(ch) && ch != '_' {
		return 0
	}
	i := start + 1
	for isWordChar(unit(src, i)) {
		i++
	}
	return i - start
}

// matchColumn is COLUMN: the length of the match anchored at start, 0 for none.
func matchColumn(src []uint16, start int) int {
	i := start
	for i < start+3 && isLetter(unit(src, i)) {
		i++
	}
	return i - start
}

// matchRow is ROW: the length of the match anchored at start, 0 for none.
func matchRow(src []uint16, start int) int {
	i := start
	for isDigit(unit(src, i)) {
		i++
	}
	return i - start
}

// jsNumber is Number() of an ASCII digit run, Infinity for a run too long to read.
func jsNumber(src []uint16, start, length int) float64 {
	n, err := strconv.ParseFloat(jsSlice(src, start, start+length), 64)
	if err != nil {
		return math.Inf(1)
	}
	return n
}

func invalid(start, end int) scan {
	if start+1 > end {
		end = start + 1
	}
	return scan{tokens: []token{{typ: "invalid", start: start, end: end}}, next: end}
}

func readString(src []uint16, start int) scan {
	i := start + 1
	var value []uint16
	for i < len(src) {
		ch := src[i]
		if ch == '"' {
			if unit(src, i+1) == '"' {
				value = append(value, '"')
				i += 2
				continue
			}
			return scan{tokens: []token{{typ: "str", value: jsText(value), start: start, end: i + 1}}, next: i + 1}
		}
		value = append(value, ch)
		i++
	}
	return invalid(start, len(src))
}

func readError(src []uint16, start int) scan {
	match := matchError(src, start)
	if match == 0 {
		return invalid(start, start+1)
	}
	end := start + match
	return scan{tokens: []token{{typ: "err", code: strings.ToUpper(jsSlice(src, start, end)), start: start, end: end}}, next: end}
}

func readCoord(src []uint16, i int, sheet *string, origin int) scan {
	start := origin
	absCol := false
	if unit(src, i) == '$' {
		absCol = true
		i++
	}
	if colMatch := matchColumn(src, i); colMatch > 0 {
		col := strings.ToUpper(jsSlice(src, i, i+colMatch))
		i += colMatch
		absRow := false
		if unit(src, i) == '$' {
			absRow = true
			i++
		}
		if rowMatch := matchRow(src, i); rowMatch > 0 {
			i += rowMatch
			if isWordChar(unit(src, i)) {
				return invalid(start, i)
			}
			return scan{tokens: []token{{
				typ:    "cell",
				sheet:  sheet,
				col:    col,
				row:    jsNumber(src, i-rowMatch, rowMatch),
				absCol: absCol,
				absRow: absRow,
				start:  start,
				end:    i,
			}}, next: i}
		}
		if absRow {
			return invalid(start, i)
		}
		return scan{tokens: []token{{typ: "col", sheet: sheet, col: col, absCol: absCol, start: start, end: i}}, next: i}
	}
	if absCol {
		absRowMatch := matchRow(src, i)
		if absRowMatch == 0 {
			return invalid(start, i)
		}
		i += absRowMatch
		if isWordChar(unit(src, i)) {
			return invalid(start, i)
		}
		return scan{tokens: []token{{
			typ:    "row",
			sheet:  sheet,
			row:    jsNumber(src, i-absRowMatch, absRowMatch),
			absRow: true,
			start:  start,
			end:    i,
		}}, next: i}
	}
	rowMatch := matchRow(src, i)
	if rowMatch > 0 {
		i += rowMatch
		if isWordChar(unit(src, i)) {
			return invalid(start, i)
		}
		return scan{tokens: []token{{
			typ:    "row",
			sheet:  sheet,
			row:    jsNumber(src, i-rowMatch, rowMatch),
			absRow: false,
			start:  start,
			end:    i,
		}}, next: i}
	}
	return invalid(start, start+1)
}

func readQuoted(src []uint16, start int) (string, int, bool) {
	i := start + 1
	var name []uint16
	for i < len(src) {
		if src[i] == '\'' {
			if unit(src, i+1) == '\'' {
				name = append(name, '\'')
				i += 2
				continue
			}
			return jsText(name), i + 1, true
		}
		name = append(name, src[i])
		i++
	}
	return "", 0, false
}

func readWordOrRef(src []uint16, start int) scan {
	if unit(src, start) == '\'' {
		name, next, ok := readQuoted(src, start)
		if !ok || unit(src, next) != '!' {
			if ok {
				return invalid(start, next)
			}
			return invalid(start, len(src))
		}
		return readCoord(src, next+1, &name, start)
	}

	word := matchWord(src, start)
	if word > 0 && unit(src, start+word) == '!' {
		end := start + word
		name := jsSlice(src, start, end)
		return readCoord(src, end+1, &name, start)
	}

	if unit(src, start) == '$' {
		return readCoord(src, start, nil, start)
	}

	if word == 0 {
		return invalid(start, start+1)
	}
	end := start + word
	if unit(src, end) == '(' {
		return scan{tokens: []token{{typ: "name", value: strings.ToUpper(jsSlice(src, start, end)), start: start, end: end}}, next: end}
	}
	upper := strings.ToUpper(jsSlice(src, start, end))
	if upper == "TRUE" || upper == "FALSE" {
		return scan{tokens: []token{{typ: "bool", boolean: upper == "TRUE", start: start, end: end}}, next: end}
	}
	return readCoord(src, start, nil, start)
}

func readNumber(src []uint16, start int) scan {
	match := matchNumber(src, start)
	end := start + match
	if unit(src, end) == ':' && isDigit(unit(src, end+1)) {
		row := matchRow(src, end+1)
		rowEnd := end + 1 + row
		return scan{
			tokens: []token{
				{typ: "row", row: jsNumber(src, start, match), absRow: false, start: start, end: end},
				{typ: "punct", value: ":", start: end, end: end + 1},
				{typ: "row", row: jsNumber(src, end+1, row), absRow: false, start: end + 1, end: rowEnd},
			},
			next: rowEnd,
		}
	}
	return scan{tokens: []token{{typ: "num", number: jsNumber(src, start, match), start: start, end: end}}, next: end}
}

func tokenize(src []uint16) []token {
	tokens := []token{}
	i := 0

	for i < len(src) {
		ch := unit(src, i)

		if isSpace(ch) {
			i++
			continue
		}

		var prev *token
		if len(tokens) > 0 {
			prev = &tokens[len(tokens)-1]
		}
		if prev != nil && prev.typ == "punct" && prev.value == ":" && len(tokens) > 1 && tokens[len(tokens)-2].typ == "row" && (isDigit(ch) || ch == '$') {
			scanned := readCoord(src, i, nil, i)
			tokens = append(tokens, scanned.tokens...)
			i = scanned.next
			continue
		}

		if ch == '(' || ch == ')' || ch == ',' || ch == ':' || ch == '%' {
			tokens = append(tokens, token{typ: "punct", value: string(rune(ch)), start: i, end: i + 1})
			i++
			continue
		}

		if ch == '"' {
			scanned := readString(src, i)
			tokens = append(tokens, scanned.tokens...)
			i = scanned.next
			continue
		}

		if ch == '#' {
			scanned := readError(src, i)
			tokens = append(tokens, scanned.tokens...)
			i = scanned.next
			continue
		}

		two := jsSlice(src, i, i+2)
		if two == "<=" || two == ">=" || two == "<>" {
			tokens = append(tokens, token{typ: "op", value: two, start: i, end: i + 2})
			i += 2
			continue
		}

		if ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '^' || ch == '&' || ch == '=' || ch == '<' || ch == '>' {
			tokens = append(tokens, token{typ: "op", value: string(rune(ch)), start: i, end: i + 1})
			i++
			continue
		}

		if isDigit(ch) || (ch == '.' && isDigit(unit(src, i+1))) {
			scanned := readNumber(src, i)
			tokens = append(tokens, scanned.tokens...)
			i = scanned.next
			continue
		}

		if ch == '$' || ch == '\'' || isLetter(ch) || ch == '_' {
			scanned := readWordOrRef(src, i)
			tokens = append(tokens, scanned.tokens...)
			i = scanned.next
			continue
		}

		tokens = append(tokens, token{typ: "invalid", start: i, end: i + 1})
		i++
	}

	return tokens
}

// The parser: a Pratt parser over the tokens, keeping every node's source span.
// A formula that does not parse returns errFormulaSyntax and never throws.

var binaryPower = map[string]int{
	"=":  1,
	"<>": 1,
	"<":  1,
	">":  1,
	"<=": 1,
	">=": 1,
	"&":  2,
	"+":  3,
	"-":  3,
	"*":  4,
	"/":  4,
	"^":  5,
}

const unaryPower = 6
const postfixPower = 7

type parseFailure struct{ at int }

type depthFailure struct{}

type part struct {
	col     string
	row     float64
	colNull bool
	rowNull bool
	absCol  bool
	absRow  bool
}

type node struct {
	typ     string
	span    [2]int
	number  float64
	str     string
	boolean bool
	code    string
	name    string
	op      string
	sheet   *string
	from    part
	to      part
	left    *node
	right   *node
	arg     *node
	args    []*node
}

type parsed struct {
	ast  *node
	refs []*node
}

type state struct {
	tokens []token
	index  int
	refs   []*node
	end    int
	depth  int
}

func peek(state *state) *token {
	if state.index >= len(state.tokens) {
		return nil
	}
	return &state.tokens[state.index]
}

func next(state *state) *token {
	token := peek(state)
	state.index++
	return token
}

func fail(state *state, token *token) {
	if token == nil {
		panic(parseFailure{at: state.end})
	}
	panic(parseFailure{at: token.start})
}

func isCoordinate(token *token) bool {
	return token.typ == "cell" || token.typ == "col" || token.typ == "row"
}

func refEnd(token *token) part {
	if token.typ == "cell" {
		return part{col: token.col, row: token.row, absCol: token.absCol, absRow: token.absRow}
	}
	if token.typ == "col" {
		return part{col: token.col, rowNull: true, absCol: token.absCol}
	}
	return part{colNull: true, row: token.row, absRow: token.absRow}
}

func parseReference(state *state, first *token) *node {
	colon := peek(state)
	if colon == nil || colon.typ != "punct" || colon.value != ":" {
		if first.typ != "cell" {
			fail(state, first)
		}
		one := part{col: first.col, row: first.row, absCol: first.absCol, absRow: first.absRow}
		ref := &node{
			typ:   "ref",
			sheet: first.sheet,
			from:  one,
			to:    one,
			span:  [2]int{first.start, first.end},
		}
		state.refs = append(state.refs, ref)
		return ref
	}

	next(state)
	second := next(state)
	if second == nil || !isCoordinate(second) {
		fail(state, second)
	}

	from := refEnd(first)
	to := refEnd(second)
	if from.colNull != to.colNull {
		fail(state, second)
	}
	if from.rowNull && !to.rowNull {
		fail(state, second)
	}

	sheet := first.sheet
	if sheet == nil {
		sheet = second.sheet
	}
	if first.sheet != nil && *first.sheet != "" && second.sheet != nil && *second.sheet != "" && *first.sheet != *second.sheet {
		fail(state, second)
	}

	rangeNode := &node{typ: "range", from: from, to: to, sheet: sheet, span: [2]int{first.start, second.end}}
	state.refs = append(state.refs, rangeNode)
	return rangeNode
}

func parsePrefix(state *state) *node {
	token := next(state)
	if token == nil {
		fail(state, token)
	}

	switch token.typ {
	case "num":
		return &node{typ: "num", number: token.number, span: [2]int{token.start, token.end}}
	case "str":
		return &node{typ: "str", str: token.value, span: [2]int{token.start, token.end}}
	case "bool":
		return &node{typ: "bool", boolean: token.boolean, span: [2]int{token.start, token.end}}
	case "err":
		return &node{typ: "err", code: token.code, span: [2]int{token.start, token.end}}
	case "cell", "col", "row":
		return parseReference(state, token)
	case "op":
		if token.value != "-" && token.value != "+" {
			fail(state, token)
		}
		arg := parseExpr(state, unaryPower)
		return &node{typ: "unary", op: token.value, arg: arg, span: [2]int{token.start, arg.span[1]}}
	case "punct":
		if token.value != "(" {
			fail(state, token)
		}
		inner := parseExpr(state, 0)
		close := next(state)
		if close == nil || close.typ != "punct" || close.value != ")" {
			fail(state, close)
		}
		return inner
	case "name":
		open := next(state)
		if open == nil || open.typ != "punct" || open.value != "(" {
			fail(state, open)
		}
		args := []*node{}
		after := peek(state)
		if after != nil && after.typ == "punct" && after.value == ")" {
			close := next(state)
			return &node{typ: "call", name: token.value, args: args, span: [2]int{token.start, close.end}}
		}
		for {
			args = append(args, parseExpr(state, 0))
			separator := next(state)
			if separator == nil || separator.typ != "punct" {
				fail(state, separator)
			}
			if separator.value == ")" {
				return &node{typ: "call", name: token.value, args: args, span: [2]int{token.start, separator.end}}
			}
			if separator.value != "," {
				fail(state, separator)
			}
		}
	default:
		fail(state, token)
	}
	return nil
}

func parseExpr(state *state, minPower int) *node {
	state.depth++
	if state.depth > maxFormulaDepth {
		panic(depthFailure{})
	}
	defer func() { state.depth-- }()

	left := parsePrefix(state)

	for {
		token := peek(state)
		if token == nil {
			break
		}

		if token.typ == "punct" && token.value == "%" {
			if postfixPower <= minPower {
				break
			}
			next(state)
			left = &node{typ: "postfix", op: "%", arg: left, span: [2]int{left.span[0], token.end}}
			continue
		}

		if token.typ != "op" {
			break
		}
		power, ok := binaryPower[token.value]
		if !ok || power <= minPower {
			break
		}

		next(state)
		// ^ groups from the right, as in Sheets: 2^3^2 is 2^9.
		rightPower := power
		if token.value == "^" {
			rightPower = power - 1
		}
		right := parseExpr(state, rightPower)
		left = &node{
			typ:   "binary",
			op:    token.value,
			left:  left,
			right: right,
			span:  [2]int{left.span[0], right.span[1]},
		}
	}

	return left
}

func parse(src []uint16) (result *parsed, err error) {
	state := &state{tokens: tokenize(src), index: 0, refs: []*node{}, end: len(src)}
	defer func() {
		failure := recover()
		switch failure.(type) {
		case parseFailure:
			result, err = nil, errFormulaSyntax
		case depthFailure:
			result, err = nil, ErrFormulaTooDeep
		case nil:
		default:
			panic(failure)
		}
	}()
	ast := parseExpr(state, 0)
	if rest := peek(state); rest != nil {
		fail(state, rest)
	}
	return &parsed{ast: ast, refs: state.refs}, nil
}

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func colIndex(letters string) int {
	if len(letters) > 12 {
		return math.MaxInt
	}
	n := 0
	for _, ch := range letters {
		n = n*26 + (int(unicode.ToUpper(ch)) - 64)
	}
	return n - 1
}

func colText(index int) string {
	n := index + 1
	out := ""
	for n > 0 {
		out = string(letters[(n-1)%26]) + out
		n = (n - 1) / 26
	}
	return out
}

// NewFrame is sapjs frameFrom: every tab by id, in the order given.
func NewFrame(sheets []FrameSheet) *Frame {
	frame := &Frame{byID: map[string]*frameSheet{}, at: map[string]int{}}
	for _, s := range sheets {
		sheet := &frameSheet{
			id:    s.ID,
			name:  s.Name,
			rows:  s.Rows,
			cols:  s.Cols,
			rowAt: map[string]int{},
			colAt: map[string]int{},
		}
		for i, id := range s.Rows {
			sheet.rowAt[id] = i
		}
		for i, id := range s.Cols {
			sheet.colAt[id] = i
		}
		if i, ok := frame.at[s.ID]; ok {
			frame.order[i] = sheet
		} else {
			frame.at[s.ID] = len(frame.order)
			frame.order = append(frame.order, sheet)
		}
		frame.byID[s.ID] = sheet
	}
	return frame
}

func byName(frame *Frame, name string) *frameSheet {
	low := strings.ToLower(name)
	for _, s := range frame.order {
		if strings.ToLower(s.name) == low {
			return s
		}
	}
	return nil
}

type idAt struct {
	open bool
	set  bool
	id   string
}

func endIds(sheet *frameSheet, end part) (row idAt, col idAt) {
	if end.rowNull {
		row = idAt{open: true}
	} else if at := end.row - 1; at >= 0 && at == math.Trunc(at) && at < float64(len(sheet.rows)) {
		row = idAt{set: true, id: sheet.rows[int(at)]}
	}
	if end.colNull {
		col = idAt{open: true}
	} else if at := colIndex(end.col); at >= 0 && at < len(sheet.cols) {
		col = idAt{set: true, id: sheet.cols[at]}
	}
	return row, col
}

// A bound reference: the sheet id and the record ids of its ends. An unset end marks an end past
// the sheet (kept as typed), an open one an open axis (a whole column or a whole row).
type boundRef struct {
	dead    bool
	unbound bool
	name    string
	s       string
	cell    bool
	r0      idAt
	c0      idAt
	r1      idAt
	c1      idAt
}

type bound struct {
	f      string
	parsed *parsed
	refs   []boundRef
}

func bind(f, homeID string, frame *Frame) (*bound, error) {
	units := jsString(f)
	src := units[1:]
	parsed, err := parse(src)
	if err != nil {
		if err == errFormulaSyntax {
			return nil, nil
		}
		return nil, err
	}
	home := frame.byID[homeID]
	refs := make([]boundRef, 0, len(parsed.refs))
	for _, n := range parsed.refs {
		var sheet *frameSheet
		if n.sheet == nil {
			sheet = home
		} else {
			sheet = byName(frame, *n.sheet)
		}
		if sheet == nil {
			name := ""
			if n.sheet != nil {
				name = *n.sheet
			}
			refs = append(refs, boundRef{unbound: true, name: name})
			continue
		}
		if n.typ == "ref" {
			row, col := endIds(sheet, n.from)
			refs = append(refs, boundRef{s: sheet.id, r0: row, c0: col, r1: row, c1: col, cell: true})
			continue
		}
		aRow, aCol := endIds(sheet, n.from)
		bRow, bCol := endIds(sheet, n.to)
		refs = append(refs, boundRef{s: sheet.id, r0: aRow, c0: aCol, r1: bRow, c1: bCol})
	}
	return &bound{f: f, parsed: parsed, refs: refs}, nil
}

func wordFull(name string) bool {
	units := jsString(name)
	if len(units) == 0 {
		return false
	}
	for i, ch := range units {
		if i == 0 {
			if !isLetter(ch) && ch != '_' {
				return false
			}
			continue
		}
		if !isWordChar(ch) {
			return false
		}
	}
	return true
}

func cellShape(name string) bool {
	units := jsString(name)
	i := 0
	for i < len(units) && i < 3 && isLetter(units[i]) {
		i++
	}
	if i == 0 || i >= len(units) {
		return false
	}
	for _, ch := range units[i:] {
		if !isDigit(ch) {
			return false
		}
	}
	return true
}

func quoteName(name string) string {
	if wordFull(name) && !cellShape(name) {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

type endValue struct {
	dead  bool
	value float64
}

type axisEnd struct {
	open  bool
	dead  bool
	value float64
}

func lookup(at map[string]int, id string) (int, bool) {
	if at == nil {
		return 0, false
	}
	value, ok := at[id]
	return value, ok
}

// One axis of a range in the after frame: [from, to] indexes, or false when nothing is left. A
// deleted end moves inward along the before order to the first record that is still there.
func axisSpan(first, last string, beforeAt map[string]int, beforeList []string, afterAt map[string]int) (int, int, bool) {
	a, aok := lookup(afterAt, first)
	b, bok := lookup(afterAt, last)
	if aok && bok {
		if a <= b {
			return a, b, true
		}
		return b, a, true
	}
	i0, i0ok := lookup(beforeAt, first)
	i1, i1ok := lookup(beforeAt, last)
	if !i0ok || !i1ok {
		return 0, 0, false
	}
	lo := i0
	if i1 < lo {
		lo = i1
	}
	hi := i0
	if i1 > hi {
		hi = i1
	}
	// A range typed bottom to top (B10:B2) has its first end at the bottom: search from each end's
	// own side, so the top end moves down and the bottom end moves up.
	top, topOk := a, aok
	bottom, bottomOk := b, bok
	if i0 > i1 {
		top, topOk = b, bok
		bottom, bottomOk = a, aok
	}
	if !topOk {
		for i := lo; i <= hi && i >= 0 && i < len(beforeList); i++ {
			if value, ok := lookup(afterAt, beforeList[i]); ok {
				top, topOk = value, true
				break
			}
		}
	}
	if !bottomOk {
		for i := hi; i >= lo && i >= 0 && i < len(beforeList); i-- {
			if value, ok := lookup(afterAt, beforeList[i]); ok {
				bottom, bottomOk = value, true
				break
			}
		}
	}
	if !topOk || !bottomOk {
		return 0, 0, false
	}
	if top <= bottom {
		return top, bottom, true
	}
	return bottom, top, true
}

// A reference with an end past the sheet's edge. A bound end follows its record and the tab's name
// follows the tab; a floating end keeps its typed row number or column letters. A bound end whose
// record is gone moves toward the edge to the next record that is left, and past the new edge when
// none is.
func floatText(n *node, ref *boundRef, prefix string, from, to part, was *frameSheet, sheet *frameSheet) string {
	var wasRowAt, wasColAt map[string]int
	var wasRows, wasCols []string
	if was != nil {
		wasRowAt, wasColAt, wasRows, wasCols = was.rowAt, was.colAt, was.rows, was.cols
	}
	one := func(id idAt, typed float64, afterAt, beforeAt map[string]int, beforeList []string, afterLen int) endValue {
		if !id.set {
			return endValue{value: typed}
		}
		if at, ok := lookup(afterAt, id.id); ok {
			return endValue{value: float64(at)}
		}
		if n.typ == "ref" {
			return endValue{dead: true}
		}
		i, ok := lookup(beforeAt, id.id)
		if !ok {
			return endValue{dead: true}
		}
		for k := i + 1; k < len(beforeList); k++ {
			if next, ok := lookup(afterAt, beforeList[k]); ok {
				return endValue{value: float64(next)}
			}
		}
		return endValue{value: float64(afterLen)}
	}
	// An open end (an open axis) stays open; the other end follows its record or floats.
	axis := func(a, b idAt, typedA, typedB float64, afterAt, beforeAt map[string]int, beforeList []string, afterLen int) [2]axisEnd {
		if a.open && b.open {
			return [2]axisEnd{{open: true}, {open: true}}
		}
		if a.set && b.set {
			if n.typ == "ref" {
				if at, ok := lookup(afterAt, a.id); ok {
					return [2]axisEnd{{value: float64(at)}, {value: float64(at)}}
				}
				return [2]axisEnd{{dead: true}, {dead: true}}
			}
			if top, bottom, ok := axisSpan(a.id, b.id, beforeAt, beforeList, afterAt); ok {
				return [2]axisEnd{{value: float64(top)}, {value: float64(bottom)}}
			}
			return [2]axisEnd{{dead: true}, {dead: true}}
		}
		end := func(id idAt, typed float64) axisEnd {
			if id.open {
				return axisEnd{open: true}
			}
			value := one(id, typed, afterAt, beforeAt, beforeList, afterLen)
			return axisEnd{dead: value.dead, value: value.value}
		}
		return [2]axisEnd{end(a, typedA), end(b, typedB)}
	}
	rowOf := func(p part) float64 {
		if p.rowNull {
			return 0
		}
		return p.row - 1
	}
	colOf := func(p part) float64 {
		if p.colNull {
			return 0
		}
		return float64(colIndex(p.col))
	}
	rows := axis(ref.r0, ref.r1, rowOf(from), rowOf(to), sheet.rowAt, wasRowAt, wasRows, len(sheet.rows))
	cols := axis(ref.c0, ref.c1, colOf(from), colOf(to), sheet.colAt, wasColAt, wasCols, len(sheet.cols))
	if rows[0].dead || rows[1].dead || cols[0].dead || cols[1].dead {
		return "#REF!"
	}
	end := func(p part, r, c axisEnd) string {
		out := ""
		if !c.open {
			if p.absCol {
				out += "$"
			}
			out += colText(int(c.value))
		}
		if !r.open {
			if p.absRow {
				out += "$"
			}
			out += formatRow(r.value + 1)
		}
		return out
	}
	if n.typ == "ref" {
		return prefix + end(from, rows[0], cols[0])
	}
	return prefix + end(from, rows[0], cols[0]) + ":" + end(to, rows[1], cols[1])
}

// formatRow is JavaScript's String(row): plain digits below 1e21, exponent form from there.
func formatRow(row float64) string {
	if row < 1e21 {
		return strconv.FormatFloat(row, 'f', -1, 64)
	}
	return strconv.FormatFloat(row, 'g', -1, 64)
}

func refText(n *node, ref *boundRef, homeID string, before, after *Frame) (string, bool) {
	if ref.dead {
		return "#REF!", true
	}
	if ref.unbound {
		return "", false
	}
	sheet := after.byID[ref.s]
	if sheet == nil {
		return "#REF!", true
	}
	var was *frameSheet
	if before != nil {
		was = before.byID[ref.s]
	}
	prefix := ""
	if !(ref.s == homeID && n.sheet == nil) {
		prefix = quoteName(sheet.name) + "!"
	}
	// A range open at one end of an axis (rows from 2 to the tab's end) prints as floating ends do:
	// the bound end follows its record, the open end stays open.
	half := func(x, y idAt) bool { return x.open != y.open }
	floats := !ref.r0.set || !ref.c0.set || !ref.r1.set || !ref.c1.set || half(ref.r0, ref.r1) || half(ref.c0, ref.c1)
	from := n.from
	to := n.to
	if floats {
		return floatText(n, ref, prefix, from, to, was, sheet), true
	}
	var rows *[2]int
	var cols *[2]int
	if !ref.r0.open {
		if ref.cell {
			if at, ok := sheet.rowAt[ref.r0.id]; ok {
				rows = &[2]int{at, at}
			}
		} else {
			var wasRowAt map[string]int
			var wasRows []string
			if was != nil {
				wasRowAt, wasRows = was.rowAt, was.rows
			}
			if top, bottom, ok := axisSpan(ref.r0.id, ref.r1.id, wasRowAt, wasRows, sheet.rowAt); ok {
				rows = &[2]int{top, bottom}
			}
		}
		if rows == nil {
			return "#REF!", true
		}
	}
	if !ref.c0.open {
		if ref.cell {
			if at, ok := sheet.colAt[ref.c0.id]; ok {
				cols = &[2]int{at, at}
			}
		} else {
			var wasColAt map[string]int
			var wasCols []string
			if was != nil {
				wasColAt, wasCols = was.colAt, was.cols
			}
			if top, bottom, ok := axisSpan(ref.c0.id, ref.c1.id, wasColAt, wasCols, sheet.colAt); ok {
				cols = &[2]int{top, bottom}
			}
		}
		if cols == nil {
			return "#REF!", true
		}
	}
	end := func(p part, r, c *int) string {
		out := ""
		if c != nil {
			if p.absCol {
				out += "$"
			}
			out += colText(*c)
		}
		if r != nil {
			if p.absRow {
				out += "$"
			}
			out += formatRow(float64(*r + 1))
		}
		return out
	}
	at := func(axis *[2]int, index int) *int {
		if axis == nil {
			return nil
		}
		value := axis[index]
		return &value
	}
	if n.typ == "ref" {
		return prefix + end(from, at(rows, 0), at(cols, 0)), true
	}
	return prefix + end(from, at(rows, 0), at(cols, 0)) + ":" + end(to, at(rows, 1), at(cols, 1)), true
}

// The bound formula printed in `after`. `before` is the frame it was bound in (nil when it came
// from an action's ids, so a deleted end has no order to shrink along and gives #REF!).
func print(bound *bound, homeID string, before, after *Frame) string {
	src := jsString(bound.f)[1:]
	var out []uint16
	at := 0
	for i, n := range bound.parsed.refs {
		typed := src[n.span[0]:n.span[1]]
		text, ok := refText(n, &bound.refs[i], homeID, before, after)
		out = append(out, src[at:n.span[0]]...)
		if ok {
			out = append(out, jsString(text)...)
		} else {
			out = append(out, typed...)
		}
		at = n.span[1]
	}
	out = append(out, src[at:]...)
	return "=" + jsText(out)
}

// Standard is sapjs standard(f, homeID, frame): a formula in the standard spelling, or f unchanged
// when it does not parse. Function names and TRUE and FALSE are in capitals, every reference is
// printed from the tab's own names (`=budget!a1` is `=Budget!A1`), and the spacing and everything
// else stay as typed.
func Standard(f, homeID string, frame *Frame) (string, error) {
	if !strings.HasPrefix(f, "=") {
		return f, nil
	}
	bound, err := bind(f, homeID, frame)
	if err != nil {
		return "", err
	}
	if bound == nil {
		return f, nil
	}
	src := jsString(print(bound, homeID, frame, frame))[1:]
	var out []uint16
	at := 0
	for _, token := range tokenize(src) {
		if token.typ != "name" && token.typ != "bool" {
			continue
		}
		out = append(out, src[at:token.start]...)
		if token.typ == "name" {
			out = append(out, jsString(token.value)...)
		} else if token.boolean {
			out = append(out, jsString("TRUE")...)
		} else {
			out = append(out, jsString("FALSE")...)
		}
		at = token.end
	}
	out = append(out, src[at:]...)
	return "=" + jsText(out), nil
}
