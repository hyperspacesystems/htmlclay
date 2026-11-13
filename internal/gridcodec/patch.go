package gridcodec

import (
	"crypto/rand"
	"fmt"
	"math"
	"sort"
	"strings"
)

const groupSize = 50

// Changed counts the cells written and the rows appended.
type Changed struct {
	Cells int `json:"cells"`
	Rows  int `json:"rows"`
}

func randomID() string {
	var bytes [6]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	var id strings.Builder
	id.WriteByte('r')
	for _, b := range bytes {
		id.WriteByte("0123456789abcdefghijklmnopqrstuvwxyz"[int(b)%36])
	}
	return id.String()
}

func badValue(message string) error {
	return &QueryRefused{Code: "bad_value", Message: message}
}

func content(value any) (Cell, error) {
	cell := Cell{}
	switch v := value.(type) {
	case nil:
	case float64:
		if math.IsInf(v, 0) {
			if v < 0 {
				return Cell{}, badValue("-Infinity is not a finite number")
			}
			return Cell{}, badValue("Infinity is not a finite number")
		}
		text, err := Shown(v)
		if err != nil {
			return Cell{}, err
		}
		cell.Text = text
		cell.N = true
	case bool:
		text, _ := Shown(v)
		cell.Text = text
	case string:
		if strings.IndexByte(v, 0) >= 0 {
			return Cell{}, badValue("text holds a NUL character")
		}
		text := strings.ReplaceAll(strings.ReplaceAll(v, "\r\n", "\n"), "\r", "\n")
		if strings.HasPrefix(text, "=") {
			cell.F = &text
			return cell, nil
		}
		typed := Typed(text, false)
		if plain, ok := typed.(string); (ok && plain == text) || text == "" {
			cell.Text = text
		} else {
			cell.Text = text
			cell.T = true
		}
	default:
		return Cell{}, badValue("a value is a number, true, false, text or null")
	}
	return cell, nil
}

func setCell(old *Cell, value any) (*Cell, error) {
	cell, err := content(value)
	if err != nil {
		return nil, err
	}
	if old != nil {
		cell.Fmt = old.Fmt
		cell.Colspan = old.Colspan
		cell.Rowspan = old.Rowspan
		cell.Style = old.Style
	}
	bare := cell.Text == "" && !cell.N && !cell.T && cell.F == nil && cell.Fmt == nil &&
		cell.Colspan == 0 && cell.Rowspan == 0 && cell.Style == nil
	if bare {
		return nil, nil
	}
	return &cell, nil
}

func checkGrid(values any, what string) ([]any, error) {
	rows, ok := values.([]any)
	if !ok || len(rows) == 0 {
		return nil, &QueryRefused{Code: "bad_patch", Message: what + " is a non-empty array of rows, each an array of values"}
	}
	for _, row := range rows {
		if _, ok := row.([]any); !ok {
			return nil, &QueryRefused{Code: "bad_patch", Message: what + " is a non-empty array of rows, each an array of values"}
		}
	}
	return rows, nil
}

type gridRow struct {
	row        Row
	start, end int
}

type gridGroup struct {
	start, end int
	frozen     bool
	rows       int
}

type gridState struct {
	cols     []Col
	rows     []gridRow
	groups   []gridGroup
	startEnd int
	table    Table
	end      int
}

func readGrid(src string) (*gridState, error) {
	grid := &gridState{}
	sink := &Sink{
		Table: func(table Table) { grid.table = table },
		Col:   func(col Col) { grid.cols = append(grid.cols, col) },
		Row: func(row Row, start, end, index int) {
			grid.rows = append(grid.rows, gridRow{row: row, start: start, end: end})
		},
		Group: func(start, end int, frozen bool) {
			grid.groups = append(grid.groups, gridGroup{start: start, end: end, frozen: frozen})
		},
	}
	end, err := ScanGrid(src, 0, sink)
	if err != nil {
		return nil, err
	}
	grid.end = end
	grid.startEnd = strings.IndexByte(src, '>') + 1
	row := 0
	for i := range grid.groups {
		group := &grid.groups[i]
		for row < len(grid.rows) && grid.rows[row].start < group.end {
			group.rows++
			row++
		}
	}
	return grid, nil
}

func applyRange(grid *gridState, op map[string]any, edited map[int]Row) (int, error) {
	rows, err := checkGrid(op["values"], "values")
	if err != nil {
		return 0, err
	}
	rng := op["range"].(string)
	bounds, err := ParseRange(rng)
	if err != nil {
		return 0, err
	}
	r0, c0, r1, c1 := bounds.R0, bounds.C0, bounds.R1, bounds.C1
	height := len(rows)
	width := 0
	for _, values := range rows {
		if n := len(values.([]any)); n > width {
			width = n
		}
	}
	single := r0 == r1 && c0 == c1
	if !single && (height != r1-r0+1 || !everyRowWidth(rows, c1-c0+1)) {
		return 0, &QueryRefused{Code: "bad_patch", Message: fmt.Sprintf("values must be %d rows of %d for range %s", r1-r0+1, c1-c0+1, rng)}
	}
	if r0+height > len(grid.rows) || c0+width > len(grid.cols) {
		return 0, &QueryRefused{Code: "outside_sheet", Message: fmt.Sprintf("range %s reaches past the sheet's rows or columns; append new rows instead", rng)}
	}
	cells := 0
	for i, values := range rows {
		index := r0 + i
		row, ok := edited[index]
		if !ok {
			row = grid.rows[index].row
			row.Cells = append([]*Cell{}, row.Cells...)
		}
		for j, value := range values.([]any) {
			col := c0 + j
			var old *Cell
			if col < len(row.Cells) {
				old = row.Cells[col]
			}
			if old != nil && old.Hidden {
				return 0, &QueryRefused{Code: "covered_cell", Message: fmt.Sprintf("range %s writes into a merged cell's covered position", rng)}
			}
			for len(row.Cells) <= col {
				row.Cells = append(row.Cells, nil)
			}
			cell, err := setCell(old, value)
			if err != nil {
				return 0, err
			}
			row.Cells[col] = cell
			cells++
		}
		edited[index] = row
	}
	return cells, nil
}

func everyRowWidth(rows []any, width int) bool {
	for _, row := range rows {
		if len(row.([]any)) != width {
			return false
		}
	}
	return true
}

type appendedRow struct{ row Row }

func lastKey(grid *gridState, appended []appendedRow) *string {
	if len(appended) > 0 {
		pos := appended[len(appended)-1].row.Pos
		return &pos
	}
	for i := len(grid.rows) - 1; i >= 0; i-- {
		pos := grid.rows[i].row.Pos
		if IsKey(pos) {
			return &pos
		}
	}
	return nil
}

type edit struct {
	start, end int
	bytes      string
}

// PatchGrid applies ops to one canonical grid (src is exactly the table's bytes) and returns the
// new bytes. ops is the decoded JSON list (each op a map[string]any or another JSON value). newID
// names appended rows; nil means random ids ("r" plus 6 base-36 characters from crypto/rand).
// Errors: *QueryRefused (bad_patch, bad_value, bad_range, outside_sheet, covered_cell) or *Miss
// for a table that is not canonical. Nothing is returned on error.
func PatchGrid(src string, ops []any, newID func() string) (string, Changed, error) {
	if len(ops) == 0 {
		return "", Changed{}, &QueryRefused{Code: "bad_patch", Message: "patch is a non-empty list of operations"}
	}
	if newID == nil {
		newID = randomID
	}
	grid, err := readGrid(src)
	if err != nil {
		return "", Changed{}, err
	}
	ids := make(map[string]bool, len(grid.rows)+len(grid.cols))
	for _, got := range grid.rows {
		ids[got.row.ID] = true
	}
	for _, col := range grid.cols {
		ids[col.ID] = true
	}
	edited := map[int]Row{}
	appended := []appendedRow{}
	cells := 0

	for _, raw := range ops {
		op, _ := raw.(map[string]any)
		rangeValue, hasRange := op["range"]
		_, rangeIsText := rangeValue.(string)
		_, hasAppend := op["append"]
		if op != nil && rangeIsText && !hasAppend {
			count, err := applyRange(grid, op, edited)
			if err != nil {
				return "", Changed{}, err
			}
			cells += count
			continue
		}
		if op != nil && hasAppend && !hasRange {
			values, err := checkGrid(op["append"], "append")
			if err != nil {
				return "", Changed{}, err
			}
			for _, raw := range values {
				rowValues := raw.([]any)
				if len(rowValues) > len(grid.cols) {
					return "", Changed{}, &QueryRefused{Code: "outside_sheet", Message: "an appended row has more values than the sheet has columns"}
				}
				id := newID()
				for ids[id] {
					id = newID()
				}
				ids[id] = true
				pos, err := KeyBetween(lastKey(grid, appended), nil)
				if err != nil {
					return "", Changed{}, err
				}
				row := Row{ID: id, Pos: pos}
				for _, value := range rowValues {
					cell, err := setCell(nil, value)
					if err != nil {
						return "", Changed{}, err
					}
					row.Cells = append(row.Cells, cell)
					if cell != nil {
						cells++
					}
				}
				appended = append(appended, appendedRow{row: row})
			}
			continue
		}
		return "", Changed{}, &QueryRefused{Code: "bad_patch", Message: "an operation is { range, values } or { append }"}
	}
	edits := []edit{}
	start := "<table grid"
	if grid.table.FreezeCols != 0 {
		start += fmt.Sprintf(" freeze-cols=\"%d\"", grid.table.FreezeCols)
	}
	start += " stale>"
	edits = append(edits, edit{start: 0, end: grid.startEnd, bytes: start})
	indexes := make([]int, 0, len(edited))
	for index := range edited {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		html, err := RowHTML(edited[index])
		if err != nil {
			return "", Changed{}, err
		}
		edits = append(edits, edit{start: grid.rows[index].start, end: grid.rows[index].end, bytes: html})
	}

	if len(appended) > 0 {
		room := 0
		at := grid.end - len("</table>")
		if len(grid.groups) > 0 {
			last := grid.groups[len(grid.groups)-1]
			if !last.frozen {
				room = groupSize - last.rows
				at = last.end - len("</tbody>")
			}
		}
		rest := appended
		if room > 0 {
			if room > len(rest) {
				room = len(rest)
			}
			var bytes strings.Builder
			for _, got := range rest[:room] {
				html, err := RowHTML(got.row)
				if err != nil {
					return "", Changed{}, err
				}
				bytes.WriteString(html + "\n")
			}
			edits = append(edits, edit{start: at, end: at, bytes: bytes.String()})
			rest = rest[room:]
		}
		at = grid.end - len("</table>")
		var bytes strings.Builder
		for i := 0; i < len(rest); i += groupSize {
			group := rest[i:min(i+groupSize, len(rest))]
			bytes.WriteString("<tbody>\n")
			for _, got := range group {
				html, err := RowHTML(got.row)
				if err != nil {
					return "", Changed{}, err
				}
				bytes.WriteString(html + "\n")
			}
			bytes.WriteString("</tbody>")
		}
		if bytes.Len() > 0 {
			edits = append(edits, edit{start: at, end: at, bytes: bytes.String()})
		}
	}

	sort.SliceStable(edits, func(a, b int) bool { return edits[a].start < edits[b].start })
	var out strings.Builder
	from := 0
	for _, got := range edits {
		out.WriteString(src[from:got.start])
		out.WriteString(got.bytes)
		from = got.end
	}
	out.WriteString(src[from:])
	return out.String(), Changed{Cells: cells, Rows: len(appended)}, nil
}
