package dataguard

import (
	"fmt"
	"github.com/panphora/htmlclay/internal/dataapi"
	"strings"
	"testing"
)

func TestSheetStoredBaselineNormalization(t *testing.T) {
	article := `<article data-blockeditor data-rev="saved"><p>Text</p></article>`
	tag := `<script type="application/json" merge="blockeditor" clay="live">{"v":1,"y":{"seed":"saved"}}</script>`
	old := dataapi.NewObject()
	data := dataapi.NewObject()
	data.Set("content", tag+article)
	old.Set("api", data)
	migrated := NormalizeComposedBaseline(old, []byte(article))
	if textField(migrated.(*dataapi.Object), FormKey) != "stored" {
		t.Fatal("migration did not mark baseline")
	}
	if textField(islandField(migrated, "api").(*dataapi.Object), "content") != article {
		t.Fatal("state tag remained")
	}
	for _, previous := range []string{`<pre>code</pre>`, `<article data-page data-rev="old"><p>Old paragraph</p></article>`, `<article data-blockeditor>no revision</article>`, `<template>` + article + `</template>`} {
		if got := NormalizeComposedBaseline(old, []byte(previous)); got != old {
			t.Fatalf("normalized a non-Block editor document: %s", previous)
		}
	}
	marked := StoredIsland(old)
	if got := NormalizeComposedBaseline(marked, []byte(article)); got != marked {
		t.Fatal("marked baseline normalized again")
	}
	if got := textField(islandField(marked, "api").(*dataapi.Object), "content"); got != tag+article {
		t.Fatal("marked example text changed")
	}
	rules := `<script data-rules-name="api" data-rules-version="1">{"content":"body@innerHTML"}</script>`
	clean, err := ExtractIsland([]byte(`<html><head>` + rules + `</head><body>` + article + `</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	composed, err := ExtractIsland([]byte(`<html><head>` + rules + `</head><body>` + tag + article + `</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if !IslandsEqual(clean, composed) {
		t.Fatal("stored and composed reads differ")
	}
	ordinary, err := ExtractIsland([]byte(`<h1>Saved</h1><script data-rules-name="api" data-rules-version="1">{"title":"h1"}</script>`))
	if err != nil {
		t.Fatal(err)
	}
	if textField(ordinary.(*dataapi.Object), FormKey) != "stored" {
		t.Fatal("ordinary island not marked")
	}
	empty, err := ExtractIsland([]byte(`<p>No rules</p>`))
	if err != nil || empty != nil {
		t.Fatalf("no-rules island = %v, %v", empty, err)
	}
}

func TestSheetPreviewPrioritizesDestroyedRows(t *testing.T) {
	makeIsland := func(destroy bool) dataapi.Value {
		rows := []dataapi.Value{}
		for i := 0; i < 61; i++ {
			if destroy && i == 60 {
				continue
			}
			hash := "old"
			if destroy {
				hash = "new"
			}
			rows = append(rows, []dataapi.Value{fmt.Sprintf("r%d", i), float64(1), float64(3), hash, "col", "1"})
		}
		tab := dataapi.NewObject()
		tab.Set("id", "s1")
		tab.Set("name", "Budget")
		tab.Set("cols", []dataapi.Value{"c1"})
		tab.Set("rows", rows)
		data := dataapi.NewObject()
		data.Set("sheets", []dataapi.Value{tab})
		declaration := dataapi.NewObject()
		declaration.Set("sheets", "sheet")
		codecs := dataapi.NewObject()
		codecs.Set("api", declaration)
		island := dataapi.NewObject()
		island.Set("api", data)
		island.Set(CodecsKey, codecs)
		island.Set(FormKey, "stored")
		return island
	}
	base, inc := makeIsland(false), makeIsland(true)
	rows := IslandPreview(base, inc)
	if len(rows) != 51 || !strings.HasSuffix(rows[0].Key, "row r60") || rows[0].Now != "— removed —" || rows[50].Yours != "11 more" {
		t.Fatalf("preview = %+v", rows)
	}
	if DroppedItems(OrdinaryData(base), OrdinaryData(inc)) != 0 {
		t.Fatal("digest rows counted as user list additions")
	}
	if !HasCodecs(base) {
		t.Fatal("digest Restore was not disabled")
	}
}
