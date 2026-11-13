package gridcodec

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var (
	countPattern = regexp.MustCompile(`^[0-9]{1,7}$`)
	widthPattern = regexp.MustCompile(`width:\s*([0-9]{1,7})px`)
)

func elementName(node *html.Node) string {
	if node.Type != html.ElementNode {
		return ""
	}
	return node.Data
}

func elementAttr(node *html.Node, name string) (string, bool) {
	for _, a := range node.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func attrOr(node *html.Node, name, fallback string) string {
	if value, ok := elementAttr(node, name); ok {
		return value
	}
	return fallback
}

func hasAttr(node *html.Node, name string) bool {
	_, ok := elementAttr(node, name)
	return ok
}

func elementChildren(node *html.Node) []*html.Node {
	out := []*html.Node{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			out = append(out, child)
		}
	}
	return out
}

func textContent(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return text.String()
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.ReplaceAll(text, "\x00", "")
}

// treeCount is canonical.js count: an attribute of one to seven digits, within [low, 9999999].
func treeCount(value string, present bool, low int) (int, bool) {
	if !present {
		return 0, false
	}
	trimmed := jsTrim(value)
	if !countPattern.MatchString(trimmed) {
		return 0, false
	}
	n, _ := strconv.Atoi(trimmed)
	if n < low || n > maxCount {
		return 0, false
	}
	return n, true
}

func cellModel(td *html.Node) *Cell {
	cell := &Cell{Text: cleanText(textContent(td))}
	bare := true
	if hasAttr(td, "n") {
		cell.N = true
		bare = false
	}
	if hasAttr(td, "t") {
		cell.T = true
		bare = false
	}
	for _, key := range []string{"f", "v", "fmt"} {
		value, ok := elementAttr(td, key)
		if !ok {
			continue
		}
		bare = false
		switch key {
		case "f":
			cell.F = strp(cleanText(value))
		case "v":
			cell.V = strp(cleanText(value))
		case "fmt":
			cell.Fmt = strp(cleanText(value))
		}
	}
	if n, ok := treeCount(attrOr(td, "colspan", ""), hasAttr(td, "colspan"), 2); ok {
		cell.Colspan = n
		bare = false
	}
	if n, ok := treeCount(attrOr(td, "rowspan", ""), hasAttr(td, "rowspan"), 2); ok {
		cell.Rowspan = n
		bare = false
	}
	if hasAttr(td, "hidden") {
		cell.Hidden = true
		cell.Text = ""
		bare = false
	}
	if style, ok := elementAttr(td, "style"); ok {
		cell.Style = strp(cleanText(style))
		bare = false
	}
	if bare && cell.Text == "" {
		return nil
	}
	return cell
}

func rowModel(tr *html.Node, frozen bool) Row {
	row := Row{ID: cleanText(attrOr(tr, "data-id", "")), Pos: cleanText(attrOr(tr, "data-pos", ""))}
	row.Frozen = frozen
	if style, ok := elementAttr(tr, "style"); ok {
		row.Style = strp(cleanText(style))
	}
	row.Cells = []*Cell{}
	for _, node := range elementChildren(tr) {
		name := elementName(node)
		if name == "td" || name == "th" {
			row.Cells = append(row.Cells, cellModel(node))
		}
	}
	return row
}

func colModel(node *html.Node) Col {
	col := Col{ID: cleanText(attrOr(node, "data-id", "")), Pos: cleanText(attrOr(node, "data-pos", ""))}
	if fmtValue, ok := elementAttr(node, "fmt"); ok {
		col.Fmt = strp(cleanText(fmtValue))
	}
	if match := widthPattern.FindStringSubmatch(attrOr(node, "style", "")); match != nil {
		width, _ := strconv.Atoi(match[1])
		col.Width = intp(width)
	}
	return col
}

// TreeModel is canonical.js treeModel for a parsed <table> element.
func TreeModel(table *html.Node) *Sheet {
	sheet := &Sheet{Cols: []Col{}, Rows: []Row{}}
	if n, ok := treeCount(attrOr(table, "freeze-cols", ""), hasAttr(table, "freeze-cols"), 1); ok {
		sheet.FreezeCols = n
	}
	if hasAttr(table, "stale") {
		sheet.Stale = true
	}
	frozen := []Row{}
	data := []Row{}
	for _, part := range elementChildren(table) {
		switch name := elementName(part); name {
		case "colgroup":
			for _, col := range elementChildren(part) {
				if elementName(col) == "col" {
					sheet.Cols = append(sheet.Cols, colModel(col))
				}
			}
		case "col":
			sheet.Cols = append(sheet.Cols, colModel(part))
		case "thead", "tbody", "tfoot":
			into := &data
			if name == "thead" {
				into = &frozen
			}
			for _, tr := range elementChildren(part) {
				if elementName(tr) == "tr" {
					*into = append(*into, rowModel(tr, name == "thead"))
				}
			}
		case "tr":
			data = append(data, rowModel(part, false))
		}
	}
	sheet.Rows = append(frozen, data...)
	return sheet
}

// CanonicalGrid is TableHTML(TreeModel(table), 50).
func CanonicalGrid(table *html.Node) (string, error) {
	return TableHTML(TreeModel(table), 50)
}
