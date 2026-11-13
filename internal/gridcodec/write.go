package gridcodec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var forbidden = regexp.MustCompile("[\r\x00]")

const (
	maxCount  = 9999999
	emptyCell = "<td></td>"
)

func clean(value, what string) (string, error) {
	if forbidden.MatchString(value) {
		return "", fmt.Errorf("grid codec: %s holds a carriage return or NUL", what)
	}
	return value, nil
}

var (
	textReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrReplacer = strings.NewReplacer("&", "&amp;", `"`, "&quot;")
)

// TextEscape escapes cell text, refusing a carriage return or NUL.
func TextEscape(value string) (string, error) {
	text, err := clean(value, "cell text")
	if err != nil {
		return "", err
	}
	return textReplacer.Replace(text), nil
}

// AttrEscape escapes an attribute value, refusing a carriage return or NUL.
func AttrEscape(value string) (string, error) {
	text, err := clean(value, "attribute value")
	if err != nil {
		return "", err
	}
	return attrReplacer.Replace(text), nil
}

func attr(name, value string) (string, error) {
	escaped, err := AttrEscape(value)
	if err != nil {
		return "", err
	}
	return " " + name + `="` + escaped + `"`, nil
}

func count(value, low int, what string) (int, error) {
	if value < low || value > maxCount {
		return 0, fmt.Errorf("grid codec: %s must be a whole number from %d to %d", what, low, maxCount)
	}
	return value, nil
}

// CellHTML writes one cell as canonical bytes. A nil cell is a bare <td></td>.
func CellHTML(cell *Cell) (string, error) {
	if cell == nil {
		return emptyCell, nil
	}
	if cell.Hidden && cell.Text != "" {
		return "", fmt.Errorf("grid codec: a hidden cell holds no text")
	}
	var attrs strings.Builder
	if cell.N {
		attrs.WriteString(" n")
	}
	if cell.T {
		attrs.WriteString(" t")
	}
	if cell.F != nil {
		a, err := attr("f", *cell.F)
		if err != nil {
			return "", err
		}
		attrs.WriteString(a)
	}
	if cell.V != nil {
		a, err := attr("v", *cell.V)
		if err != nil {
			return "", err
		}
		attrs.WriteString(a)
	}
	if cell.Fmt != nil {
		a, err := attr("fmt", *cell.Fmt)
		if err != nil {
			return "", err
		}
		attrs.WriteString(a)
	}
	if cell.Colspan != 0 {
		n, err := count(cell.Colspan, 2, "colspan")
		if err != nil {
			return "", err
		}
		attrs.WriteString(` colspan="` + strconv.Itoa(n) + `"`)
	}
	if cell.Rowspan != 0 {
		n, err := count(cell.Rowspan, 2, "rowspan")
		if err != nil {
			return "", err
		}
		attrs.WriteString(` rowspan="` + strconv.Itoa(n) + `"`)
	}
	if cell.Hidden {
		attrs.WriteString(" hidden")
	}
	if cell.Style != nil {
		a, err := attr("style", *cell.Style)
		if err != nil {
			return "", err
		}
		attrs.WriteString(a)
	}
	text, err := TextEscape(cell.Text)
	if err != nil {
		return "", err
	}
	return "<td" + attrs.String() + ">" + text + "</td>", nil
}

// RowHTML writes one row as canonical bytes, dropping trailing empty cells.
func RowHTML(row Row) (string, error) {
	cells := make([]string, 0, len(row.Cells))
	for _, cell := range row.Cells {
		html, err := CellHTML(cell)
		if err != nil {
			return "", err
		}
		cells = append(cells, html)
	}
	for len(cells) > 0 && cells[len(cells)-1] == emptyCell {
		cells = cells[:len(cells)-1]
	}
	style := ""
	if row.Style != nil {
		a, err := attr("style", *row.Style)
		if err != nil {
			return "", err
		}
		style = a
	}
	id, err := attr("data-id", row.ID)
	if err != nil {
		return "", err
	}
	pos, err := attr("data-pos", row.Pos)
	if err != nil {
		return "", err
	}
	return "<tr" + id + pos + style + ">" + strings.Join(cells, "") + "</tr>", nil
}

// ColHTML writes one column as canonical bytes.
func ColHTML(col Col) (string, error) {
	id, err := attr("data-id", col.ID)
	if err != nil {
		return "", err
	}
	pos, err := attr("data-pos", col.Pos)
	if err != nil {
		return "", err
	}
	attrs := id + pos
	if col.Fmt != nil {
		a, err := attr("fmt", *col.Fmt)
		if err != nil {
			return "", err
		}
		attrs += a
	}
	if col.Width != nil {
		n, err := count(*col.Width, 0, "width")
		if err != nil {
			return "", err
		}
		attrs += ` style="width:` + strconv.Itoa(n) + `px"`
	}
	return "<col" + attrs + ">", nil
}

// TableHTML writes the whole table as canonical bytes, groupSize data rows per tbody.
// A groupSize of 0 means 50.
func TableHTML(sheet *Sheet, groupSize int) (string, error) {
	if groupSize == 0 {
		groupSize = 50
	}
	groupSize, err := count(groupSize, 1, "groupSize")
	if err != nil {
		return "", err
	}
	firstData := -1
	for i, row := range sheet.Rows {
		if !row.Frozen {
			firstData = i
			break
		}
	}
	frozen := []Row{}
	data := []Row{}
	if firstData == -1 {
		frozen = sheet.Rows
	} else {
		frozen = sheet.Rows[:firstData]
		data = sheet.Rows[firstData:]
	}
	for _, row := range data {
		if row.Frozen {
			return "", fmt.Errorf("grid codec: frozen rows come before every other row")
		}
	}

	html := "<table grid"
	if sheet.FreezeCols != 0 {
		n, err := count(sheet.FreezeCols, 1, "freezeCols")
		if err != nil {
			return "", err
		}
		html += ` freeze-cols="` + strconv.Itoa(n) + `"`
	}
	if sheet.Stale {
		html += " stale"
	}
	cols := make([]string, 0, len(sheet.Cols))
	for _, col := range sheet.Cols {
		colHTML, err := ColHTML(col)
		if err != nil {
			return "", err
		}
		cols = append(cols, colHTML)
	}
	html += "><colgroup>" + strings.Join(cols, "") + "</colgroup>\n"

	if len(frozen) > 0 {
		rows := make([]string, 0, len(frozen))
		for _, row := range frozen {
			rowHTML, err := RowHTML(row)
			if err != nil {
				return "", err
			}
			rows = append(rows, rowHTML)
		}
		html += "<thead>\n" + strings.Join(rows, "\n") + "\n</thead>"
	}

	groups := max(1, (len(data)+groupSize-1)/groupSize)
	for g := 0; g < groups; g++ {
		group := data[min(g*groupSize, len(data)):min((g+1)*groupSize, len(data))]
		html += "<tbody>\n"
		if len(group) > 0 {
			rows := make([]string, 0, len(group))
			for _, row := range group {
				rowHTML, err := RowHTML(row)
				if err != nil {
					return "", err
				}
				rows = append(rows, rowHTML)
			}
			html += strings.Join(rows, "\n") + "\n"
		}
		html += "</tbody>"
	}

	return html + "</table>", nil
}
