package dataguard

import (
	"fmt"
	"github.com/panphora/htmlclay/internal/dataapi"
)

func cellCount(n int) string {
	if n == 1 {
		return "1 filled cell"
	}
	return fmt.Sprintf("%d filled cells", n)
}

func IslandPreview(recoverable, current dataapi.Value) []PreviewRow {
	rd, cd := OrdinaryData(recoverable), OrdinaryData(current)
	_, rlist := rd.([]dataapi.Value)
	_, clist := cd.([]dataapi.Value)
	if rlist || clist {
		return buildPreview(rd, cd)
	}
	rows := []PreviewRow{}
	for _, key := range previewKeys(rd, cd) {
		if declared(recoverable, "api", key) != "" {
			continue
		}
		before, _ := objectLookup(rd, key)
		after, _ := objectLookup(cd, key)
		yours, now := formatValue(before), formatValue(after)
		if codec := declared(current, "api", key); codec != "" {
			now = "replaced by a " + codec
		}
		if yours != now {
			rows = append(rows, PreviewRow{Key: key, Now: now, Yours: yours})
		}
	}
	destroyed, changed := []PreviewRow{}, []PreviewRow{}
	declarations := codecDeclarations(recoverable, "api")
	if declarations == nil {
		return rows
	}
	for _, key := range declarations.Keys() {
		name := textField(declarations, key)
		codec := name == "sheet" && declared(current, "api", key) == name
		before, _ := objectLookup(islandField(recoverable, "api"), key)
		after, _ := objectLookup(islandField(current, "api"), key)
		for _, change := range codecRowChanges(before, after, codec) {
			if change.change != "destruction" && change.change != "modification" {
				continue
			}
			tabName := textField(change.tab, "name")
			if tabName == "" {
				tabName = textField(change.tab, "id")
			}
			if tabName == "" {
				tabName = fmt.Sprintf("Sheet %d", change.tabIndex+1)
			}
			rowName := "a row"
			if id := entryID(change.before[0]); id != "" {
				rowName = "row " + id
			}
			now := "— removed —"
			if change.now != nil {
				now = cellCount(digestCount(change.now[1]))
				if change.change == "modification" {
					now = "changed, " + now
				}
			}
			row := PreviewRow{Key: key + " › " + tabName + " › " + rowName, Now: now, Yours: cellCount(digestCount(change.before[1]))}
			if change.change == "destruction" {
				destroyed = append(destroyed, row)
			} else {
				changed = append(changed, row)
			}
		}
	}
	all := append(destroyed, changed...)
	shown := min(50, len(all))
	rows = append(rows, all[:shown]...)
	if shown < len(all) {
		rows = append(rows, PreviewRow{Key: "more rows", Yours: fmt.Sprintf("%d more", len(all)-shown)})
	}
	return rows
}
