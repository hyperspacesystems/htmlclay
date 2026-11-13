package gridcodec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type Region struct{ Start, End int }

var markerPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// HoleHTML is the placeholder a cut leaves.
func HoleHTML(marker string, n int) string {
	return fmt.Sprintf(`<table grid data-hole="%s-%d"></table>`, marker, n)
}

// HoleMarker returns "h" plus 24 lowercase hex digits that do not occur in src.
func HoleMarker(src string) string {
	for {
		bytes := make([]byte, 12)
		if _, err := rand.Read(bytes); err != nil {
			panic(err)
		}
		marker := "h" + hex.EncodeToString(bytes)
		if !strings.Contains(src, marker) {
			return marker
		}
	}
}

func checkMarker(marker, src string) error {
	if !markerPattern.MatchString(marker) {
		return fmt.Errorf("grid codec: a hole marker is letters, digits, - and _ only")
	}
	if strings.Contains(src, marker) {
		return fmt.Errorf("grid codec: the hole marker occurs in the document")
	}
	return nil
}

// CutGrids replaces every region from "<table grid" that ScanGrid accepts with
// HoleHTML(marker, n) and returns the rest and the regions.
func CutGrids(src, marker string) (string, []Region, error) {
	if err := checkMarker(marker, src); err != nil {
		return "", nil, err
	}
	regions := []Region{}
	var parts []string
	from := 0
	at := 0
	for {
		k := strings.Index(src[at:], "<table grid")
		if k < 0 {
			break
		}
		start := at + k
		end, err := ScanGrid(src, start, nil)
		if err != nil {
			if _, ok := err.(*Miss); !ok {
				return "", nil, err
			}
			at = start + 1
			continue
		}
		parts = append(parts, src[from:start], HoleHTML(marker, len(regions)))
		regions = append(regions, Region{Start: start, End: end})
		from = end
		at = end
	}
	parts = append(parts, src[from:])
	return strings.Join(parts, ""), regions, nil
}

// JoinGrids is the inverse of CutGrids: each hole n gets replace[n] when it is
// present, else the original region.
func JoinGrids(rest, marker, src string, regions []Region, replace map[int]string) (string, error) {
	var parts []string
	from := 0
	for n, region := range regions {
		hole := HoleHTML(marker, n)
		k := strings.Index(rest[from:], hole)
		if k < 0 {
			return "", fmt.Errorf("grid codec: hole %d is missing", n)
		}
		at := from + k
		bytes, ok := replace[n]
		if !ok {
			bytes = src[region.Start:region.End]
		}
		parts = append(parts, rest[from:at], bytes)
		from = at + len(hole)
	}
	parts = append(parts, rest[from:])
	return strings.Join(parts, ""), nil
}
