package gridcodec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Sheet is GRAMMAR.md's model of one grid.
type Sheet struct {
	FreezeCols int   `json:"freezeCols,omitempty"`
	Stale      bool  `json:"stale,omitempty"`
	Cols       []Col `json:"cols"`
	Rows       []Row `json:"rows"`
}

// QueryRefused is a read or patch the caller asked for wrongly. Code is one of bad_range,
// bad_render (and later bad_patch, bad_value, outside_sheet, covered_cell, unknown_sheet).
// Hosts answer 400.
type QueryRefused struct {
	Code    string
	Message string
}

func (e *QueryRefused) Error() string { return e.Message }

// GridModel reads the grid at byte offset at into a Sheet, or returns the *Miss.
func GridModel(src string, at int) (*Sheet, error) {
	sheet := &Sheet{Cols: []Col{}, Rows: []Row{}}
	sink := &Sink{
		Table: func(table Table) {
			sheet.FreezeCols = table.FreezeCols
			sheet.Stale = table.Stale
		},
		Col: func(col Col) { sheet.Cols = append(sheet.Cols, col) },
		Row: func(row Row, start, end, index int) { sheet.Rows = append(sheet.Rows, row) },
	}
	if _, err := ScanGrid(src, at, sink); err != nil {
		return nil, err
	}
	return sheet, nil
}

// CellObject is a decorated cell in the tab shape, fields in the JavaScript key order.
type CellObject struct {
	F       *string `json:"f,omitempty"`
	V       any     `json:"v"`
	Text    *string `json:"text,omitempty"`
	Fmt     *string `json:"fmt,omitempty"`
	Style   *string `json:"style,omitempty"`
	Colspan int     `json:"colspan,omitempty"`
	Rowspan int     `json:"rowspan,omitempty"`
}

// CellJSON is read.js cellJson: nil for a nil or hidden cell; the typed value (nil, bool,
// float64 or string) when the cell has no f, v, fmt, style, colspan or rowspan; else a
// *CellObject whose Text is set only when the text differs from Shown(v).
func CellJSON(cell *Cell) any {
	if cell == nil || cell.Hidden {
		return nil
	}
	value := CellValue(cell)
	plain := cell.F == nil && cell.V == nil && cell.Fmt == nil && cell.Style == nil &&
		cell.Colspan == 0 && cell.Rowspan == 0
	if plain {
		return value
	}
	out := &CellObject{V: value}
	if cell.F != nil {
		out.F = cell.F
	}
	shown, _ := Shown(value)
	if cell.Text != shown {
		text := cell.Text
		out.Text = &text
	}
	if cell.Fmt != nil {
		out.Fmt = cell.Fmt
	}
	if cell.Style != nil {
		out.Style = cell.Style
	}
	if cell.Colspan != 0 {
		out.Colspan = cell.Colspan
	}
	if cell.Rowspan != 0 {
		out.Rowspan = cell.Rowspan
	}
	return out
}

type RowJSON struct {
	ID    string `json:"id"`
	Cells []any  `json:"cells"`
}

func rowJSON(row Row) RowJSON {
	cells := make([]any, 0, len(row.Cells))
	for _, cell := range row.Cells {
		cells = append(cells, CellJSON(cell))
	}
	for len(cells) > 0 && cells[len(cells)-1] == nil {
		cells = cells[:len(cells)-1]
	}
	return RowJSON{ID: row.ID, Cells: cells}
}

// TabJSON is the tab shape without id and name, which the document level adds.
type TabJSON struct {
	Cols  []string  `json:"cols"`
	Rows  []RowJSON `json:"rows"`
	Stale bool      `json:"stale,omitempty"`
}

type TabIndex struct {
	Rows       int `json:"rows"`
	Cols       int `json:"cols"`
	FrozenRows int `json:"frozenRows"`
}

func Tab(sheet *Sheet) TabJSON {
	tab := TabJSON{Cols: make([]string, 0, len(sheet.Cols)), Rows: make([]RowJSON, 0, len(sheet.Rows))}
	for _, col := range sheet.Cols {
		tab.Cols = append(tab.Cols, col.ID)
	}
	for _, row := range sheet.Rows {
		tab.Rows = append(tab.Rows, rowJSON(row))
	}
	tab.Stale = sheet.Stale
	return tab
}

func Index(sheet *Sheet) TabIndex {
	index := TabIndex{Rows: len(sheet.Rows), Cols: len(sheet.Cols)}
	for _, row := range sheet.Rows {
		if row.Frozen {
			index.FrozenRows++
		}
	}
	return index
}

// Bounds are zero-based inclusive.
type Bounds struct{ R0, C0, R1, C1 int }

var (
	a1Pattern = regexp.MustCompile(`^([A-Z]{1,3})([1-9][0-9]{0,6})$`)

	maxColumn = 16384
)

func columnNumber(letters string) int {
	n := 0
	for i := 0; i < len(letters); i++ {
		n = n*26 + int(letters[i]) - 64
	}
	return n
}

func parseCell(text, rng string) (int, int, error) {
	match := a1Pattern.FindStringSubmatch(text)
	column := 0
	if match != nil {
		column = columnNumber(match[1])
	}
	if match == nil || column > maxColumn {
		return 0, 0, &QueryRefused{Code: "bad_range", Message: fmt.Sprintf("range %s is not an A1 cell or rectangle such as B2 or A1:C3", jsQuote(rng))}
	}
	row, _ := strconv.Atoi(match[2])
	return row - 1, column - 1, nil
}

// ParseRange is read.js parseRange. rng is any because a JSON query may carry a number; a
// non-string is refused with "range must be text such as A1:C3".
func ParseRange(rng any) (Bounds, error) {
	text, ok := rng.(string)
	if !ok {
		return Bounds{}, &QueryRefused{Code: "bad_range", Message: "range must be text such as A1:C3"}
	}
	parts := strings.Split(text, ":")
	if len(parts) > 2 {
		return Bounds{}, &QueryRefused{Code: "bad_range", Message: fmt.Sprintf("range %s has more than one colon", jsQuote(text))}
	}
	r0, c0, err := parseCell(parts[0], text)
	if err != nil {
		return Bounds{}, err
	}
	r1, c1 := r0, c0
	if len(parts) == 2 {
		r1, c1, err = parseCell(parts[1], text)
		if err != nil {
			return Bounds{}, err
		}
	}
	if r1 < r0 || c1 < c0 {
		return Bounds{}, &QueryRefused{Code: "bad_range", Message: fmt.Sprintf("range %s is reversed: write the top left cell first", jsQuote(text))}
	}
	return Bounds{R0: r0, C0: c0, R1: r1, C1: c1}, nil
}

type ValuesRange struct {
	Values [][]any `json:"values"`
}

type CellsRange struct {
	Cols []string  `json:"cols"`
	Rows []RowJSON `json:"rows"`
}

var renders = []string{"values", "formulas", "cells"}

// ReadRange is read.js readRange. render nil means "values"; any value that is not one of the
// strings "values", "formulas", "cells" (including "" and non-strings) is bad_render. The render
// check comes BEFORE the range is parsed, as in JavaScript. Returns ValuesRange or CellsRange.
func ReadRange(sheet *Sheet, rng any, render any) (any, error) {
	kind := "values"
	if render != nil {
		text, ok := render.(string)
		if !ok {
			return nil, badRender()
		}
		kind = text
	}
	validRender := false
	for _, name := range renders {
		if kind == name {
			validRender = true
		}
	}
	if !validRender {
		return nil, badRender()
	}
	bounds, err := ParseRange(rng)
	if err != nil {
		return nil, err
	}
	lastRow := min(bounds.R1, len(sheet.Rows)-1)
	lastCol := min(bounds.C1, len(sheet.Cols)-1)
	rows := sliceRows(sheet.Rows, bounds.R0, lastRow+1)
	width := max(0, lastCol-bounds.C0+1)

	if kind == "cells" {
		cols := make([]string, 0, max(0, lastCol-bounds.C0+1))
		for _, col := range sliceCols(sheet.Cols, bounds.C0, lastCol+1) {
			cols = append(cols, col.ID)
		}
		out := CellsRange{Cols: cols, Rows: make([]RowJSON, 0, len(rows))}
		for _, row := range rows {
			out.Rows = append(out.Rows, rowJSON(Row{
				ID:    row.ID,
				Cells: sliceCells(row.Cells, bounds.C0, bounds.C0+width),
			}))
		}
		return out, nil
	}

	values := make([][]any, 0, len(rows))
	for _, row := range rows {
		cells := make([]any, width)
		for i := 0; i < width; i++ {
			var cell *Cell
			if bounds.C0+i < len(row.Cells) {
				cell = row.Cells[bounds.C0+i]
			}
			if kind == "formulas" && cell != nil && !cell.Hidden && cell.F != nil {
				cells[i] = *cell.F
			} else {
				cells[i] = CellValue(cell)
			}
		}
		values = append(values, cells)
	}
	return ValuesRange{Values: values}, nil
}

func badRender() error {
	return &QueryRefused{Code: "bad_render", Message: "render must be one of " + strings.Join(renders, ", ")}
}

func sliceRows(rows []Row, from, to int) []Row {
	if from > to || from >= len(rows) {
		return []Row{}
	}
	return rows[from:min(to, len(rows))]
}

func sliceCols(cols []Col, from, to int) []Col {
	if from > to || from >= len(cols) {
		return []Col{}
	}
	return cols[from:min(to, len(cols))]
}

func sliceCells(cells []*Cell, from, to int) []*Cell {
	if from > to || from >= len(cells) {
		return []*Cell{}
	}
	return cells[from:min(to, len(cells))]
}

// jsQuote is JSON.stringify for a string: an encoder with SetEscapeHTML(false) and no newline.
func jsQuote(text string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(text); err != nil {
		return `""`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
