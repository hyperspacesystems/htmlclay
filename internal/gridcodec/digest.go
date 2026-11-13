package gridcodec

import (
	"encoding/json"
	"strconv"
	"strings"
)

// FNVOffset is FNV-1a's 32 bit starting value.
const FNVOffset uint32 = 0x811c9dc5

const fnvPrime uint32 = 0x01000193

// FNV1a is 32-bit FNV-1a over the UTF-8 bytes of text, continuing from hash.
func FNV1a(text string, hash uint32) uint32 {
	for i := 0; i < len(text); i++ {
		hash ^= uint32(text[i])
		hash *= fnvPrime
	}
	return hash
}

// Hex is the hash as 8 lowercase hex digits.
func Hex(hash uint32) string {
	digits := strconv.FormatUint(uint64(hash), 16)
	if len(digits) < 8 {
		digits = strings.Repeat("0", 8-len(digits)) + digits
	}
	return digits
}

// CellInput is what a person typed: F, else V, else the text; "" for a nil or
// hidden cell.
func CellInput(cell *Cell) string {
	if cell == nil || cell.Hidden {
		return ""
	}
	if cell.F != nil {
		return *cell.F
	}
	if cell.V != nil {
		return *cell.V
	}
	return cell.Text
}

func utf16Length(text string) int {
	units := 0
	for _, r := range text {
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	return units
}

func columnID(cols []Col, index int) string {
	if index < len(cols) && cols[index].ID != "" {
		return cols[index].ID
	}
	return "#" + strconv.Itoa(index)
}

// RowDigest is [rowId, filled, mass, inputsHash, columnsHash, mask] as []any, or
// nil for a row with no filled cell.
func RowDigest(row Row, cols []Col) []any {
	filled := 0
	mass := 0
	inputs := FNVOffset
	columns := uint32(0)
	occupied := []int{}
	for index, cell := range row.Cells {
		input := CellInput(cell)
		inputs = FNV1a(input+"\x00", inputs)
		if input == "" {
			continue
		}
		filled++
		mass += utf16Length(jsTrim(input))
		columns ^= FNV1a(columnID(cols, index), FNVOffset)
		occupied = append(occupied, index)
	}
	if filled == 0 {
		return nil
	}
	return []any{row.ID, filled, mass, Hex(inputs), Hex(columns), MaskHex(occupied)}
}

// GridDigest is every non-nil RowDigest in row order (never a nil slice).
func GridDigest(sheet *Sheet) [][]any {
	out := make([][]any, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		if digest := RowDigest(row, sheet.Cols); digest != nil {
			out = append(out, digest)
		}
	}
	return out
}

// MaskHex is the column indexes as a hex occupancy mask, lowest bit column 0,
// built nibble by nibble so it has no width limit.
func MaskHex(indexes []int) string {
	nibbles := []int{}
	for _, index := range indexes {
		at := index >> 2
		for len(nibbles) <= at {
			nibbles = append(nibbles, 0)
		}
		nibbles[at] |= 1 << (index & 3)
	}
	for len(nibbles) > 1 && nibbles[len(nibbles)-1] == 0 {
		nibbles = nibbles[:len(nibbles)-1]
	}
	if len(nibbles) == 0 {
		return "0"
	}
	var out strings.Builder
	for i := len(nibbles) - 1; i >= 0; i-- {
		out.WriteString(strconv.FormatUint(uint64(nibbles[i]), 16))
	}
	return out.String()
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

// MaskLost is true when some bit set in before is clear in after.
func MaskLost(before, after string) bool {
	for i := 1; i <= len(before); i++ {
		was := hexDigit(before[len(before)-i])
		now := 0
		if i <= len(after) {
			now = hexDigit(after[len(after)-i])
		}
		if was&^now != 0 {
			return true
		}
	}
	return false
}

func asInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func asText(value any) string {
	text, _ := value.(string)
	return text
}

// RowChange is "same", "modification" or "destruction"; after is nil when the
// row is gone.
func RowChange(before, after []any, columns ...[]string) string {
	if after == nil {
		return "destruction"
	}
	if asInt(after[1]) < asInt(before[1]) {
		return "destruction"
	}
	var beforeCols, afterCols []string
	if len(columns) > 0 {
		beforeCols = columns[0]
	}
	if len(columns) > 1 {
		afterCols = columns[1]
	}
	nowIDs := map[string]bool{}
	nowIndexes := map[int]bool{}
	for _, index := range maskIndexes(asText(after[5])) {
		nowIDs[columnName(afterCols, index)] = true
		nowIndexes[index] = true
	}
	positional := true
	for index, id := range beforeCols {
		if id != "" && id != "#"+strconv.Itoa(index) {
			positional = false
		}
	}
	for _, index := range maskIndexes(asText(before[5])) {
		id := columnName(beforeCols, index)
		if id == "#"+strconv.Itoa(index) {
			if !nowIndexes[index] {
				return "destruction"
			}
		} else if !nowIDs[id] {
			return "destruction"
		}
	}
	if before[3] == after[3] && (before[4] == after[4] || positional) && before[5] == after[5] {
		return "same"
	}
	return "modification"
}

func columnName(cols []string, index int) string {
	if index < len(cols) && cols[index] != "" {
		return cols[index]
	}
	return "#" + strconv.Itoa(index)
}

func maskIndexes(mask string) []int {
	indexes := []int{}
	for i := 0; i < len(mask); i++ {
		nibble := hexDigit(mask[len(mask)-1-i])
		for bit := 0; bit < 4; bit++ {
			if nibble&(1<<bit) != 0 {
				indexes = append(indexes, i*4+bit)
			}
		}
	}
	return indexes
}

func ColumnIDs(sheet *Sheet) []string {
	cols := make([]string, len(sheet.Cols))
	for index := range cols {
		cols[index] = columnID(sheet.Cols, index)
	}
	return cols
}

func TabDigest(sheet *Sheet) map[string]any {
	return map[string]any{"cols": ColumnIDs(sheet), "rows": GridDigest(sheet)}
}
