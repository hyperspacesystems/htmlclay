package dataapi

import (
	"bytes"
	"encoding/json"
	"fmt"

	"golang.org/x/net/html"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

// SheetCodec reads the sheet template's value: the tabs under the element a
// "[items=sheets]@codec(sheet)" rule selects. Port of hyper-html-api src/codecs/sheet.js read().
var SheetCodec Codec = sheetCodec{}

type sheetCodec struct{}

type sheetTab struct {
	id, name string
	table    *html.Node
	sheet    *gridcodec.Sheet
	region   *gridcodec.Region
}

func firstChildElement(n *html.Node, match func(*html.Node) bool) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && match(c) {
			return c
		}
	}
	return nil
}

func hasAttribute(n *html.Node, name string) bool {
	_, ok := attrValue(n, name)
	return ok
}

// sheetTabNodes is sheet.js tabNodes: each <section item> child, its data-id, the text of its
// <h2 bind="name"> child (textContent, never trimmed), and its <table grid> child.
func sheetTabNodes(node *html.Node) []sheetTab {
	tabs := []sheetTab{}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "section" || !hasAttribute(c, "item") {
			continue
		}
		id, _ := attrValue(c, "data-id")
		tab := sheetTab{id: id}
		heading := firstChildElement(c, func(n *html.Node) bool {
			bind, ok := attrValue(n, "bind")
			return n.Data == "h2" && ok && bind == "name"
		})
		if heading != nil {
			tab.name = textContent(heading)
		}
		tab.table = firstChildElement(c, func(n *html.Node) bool { return n.Data == "table" && hasAttribute(n, "grid") })
		tabs = append(tabs, tab)
	}
	return tabs
}

// SheetWriteUnavailableMessage is sheet.js WRITE_UNAVAILABLE_MESSAGE.
const SheetWriteUnavailableMessage = `A sheet cannot be replaced as a whole value yet: send patch operations instead, for example [{ "sheet": "Budget", "range": "B3", "values": [[450]] }].`

// Write refuses: replacing a sheet as a whole value has its own spec (whether a missing tab is
// deleted changes user data). Hosts answer this refusal with 409.
func (sheetCodec) Write(*Document, *html.Node, Value, CodecContext) error {
	return &CodecRefused{Code: "sheet-write-unavailable", Message: SheetWriteUnavailableMessage}
}

func (sheetCodec) Read(d *Document, node *html.Node, ctx CodecContext) (Value, error) {
	tabs := sheetTabNodes(node)
	for i := range tabs {
		t := &tabs[i]
		bytes, cut := "", false
		if t.table != nil && ctx.Region != nil {
			bytes, cut = ctx.Region(t.table)
		}
		switch {
		case cut:
			sheet, err := gridcodec.GridModel(bytes, 0)
			if err != nil {
				return nil, err
			}
			t.sheet = sheet
		case t.table != nil:
			t.sheet = gridcodec.TreeModel(t.table)
		default:
			t.sheet = &gridcodec.Sheet{Cols: []gridcodec.Col{}, Rows: []gridcodec.Row{}}
		}
	}

	query, _ := ctx.Query.(*Object)
	get := func(key string) (Value, bool) {
		if query == nil {
			return nil, false
		}
		return query.Get(key)
	}

	if ctx.View == "digest" {
		out := []Value{}
		for _, t := range tabs {
			obj := NewObject()
			obj.Set("id", t.id)
			obj.Set("name", t.name)
			cols, err := jsonValue(gridcodec.ColumnIDs(t.sheet))
			if err != nil {
				return nil, err
			}
			obj.Set("cols", cols)
			rows, err := jsonValue(gridcodec.GridDigest(t.sheet))
			if err != nil {
				return nil, err
			}
			obj.Set("rows", rows)
			out = append(out, obj)
		}
		return out, nil
	}

	shape, hasShape := get("shape")
	if hasShape && shape != "index" {
		return nil, &gridcodec.QueryRefused{Code: "bad_shape", Message: fmt.Sprintf("unknown shape %s; the only shape is index", jsJSON(shape))}
	}
	_, hasRender := get("render")
	rng, hasRange := get("range")
	if hasRender && !hasRange {
		return nil, &gridcodec.QueryRefused{Code: "bad_render", Message: "render needs a range, for example range=A1:C10&render=formulas"}
	}

	if hasShape {
		out := []Value{}
		for _, t := range tabs {
			obj := NewObject()
			obj.Set("id", t.id)
			obj.Set("name", t.name)
			if err := spreadJSON(obj, gridcodec.Index(t.sheet)); err != nil {
				return nil, err
			}
			out = append(out, obj)
		}
		return out, nil
	}

	wanted, hasSheet := get("sheet")
	if hasRange {
		tab, err := pickTab(tabs, wanted, hasSheet)
		if err != nil {
			return nil, err
		}
		render, _ := get("render")
		if render == nil {
			render = "values"
		}
		read, err := gridcodec.ReadRange(tab.sheet, rng, render)
		if err != nil {
			return nil, err
		}
		obj := NewObject()
		obj.Set("sheet", tab.name)
		obj.Set("range", rng)
		if err := spreadJSON(obj, read); err != nil {
			return nil, err
		}
		return obj, nil
	}

	if hasSheet {
		tab, err := pickTab(tabs, wanted, true)
		if err != nil {
			return nil, err
		}
		return tabValue(tab)
	}

	out := []Value{}
	for i := range tabs {
		v, err := tabValue(&tabs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func tabValue(t *sheetTab) (Value, error) {
	obj := NewObject()
	obj.Set("id", t.id)
	obj.Set("name", t.name)
	if err := spreadJSON(obj, gridcodec.Tab(t.sheet)); err != nil {
		return nil, err
	}
	return obj, nil
}

// pickTab is sheet.js pickTab: by name first, then by id; no sheet means the first tab.
func pickTab(tabs []sheetTab, wanted Value, has bool) (*sheetTab, error) {
	if !has {
		if len(tabs) == 0 {
			return nil, &gridcodec.QueryRefused{Code: "unknown_sheet", Message: "this document has no sheets"}
		}
		return &tabs[0], nil
	}
	for i := range tabs {
		if wanted == tabs[i].name {
			return &tabs[i], nil
		}
	}
	for i := range tabs {
		if wanted == tabs[i].id {
			return &tabs[i], nil
		}
	}
	return nil, &gridcodec.QueryRefused{Code: "unknown_sheet", Message: fmt.Sprintf("no sheet named %s; ask for shape=index to list them", jsJSON(wanted))}
}

// jsJSON is JSON.stringify for a message: no HTML escaping, no trailing newline.
func jsJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}

// jsonValue turns a gridcodec shape (structs whose fields are in JavaScript key order) into an
// ordered Value.
func jsonValue(v any) (Value, error) {
	return ParseStrict(jsJSON(v))
}

// spreadJSON is JavaScript's {...a, ...b}: it appends the shape's keys, in order, to obj.
func spreadJSON(obj *Object, shape any) error {
	v, err := jsonValue(shape)
	if err != nil {
		return err
	}
	from, ok := v.(*Object)
	if !ok {
		return fmt.Errorf("sheet codec: %T is not an object", shape)
	}
	for _, key := range from.Keys() {
		value, _ := from.Get(key)
		obj.Set(key, value)
	}
	return nil
}
