package dataapi

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

// SheetQueryParams are the URL parameters a sheet read takes, in the order they are checked and
// reported.
var SheetQueryParams = []string{"shape", "sheet", "range", "render"}

// ParseSheetQuery is hyper-html-api's parseSheetQuery over a raw query string (no leading "?").
// It returns nil when none of the parameters is present (a whole read), else an *Object holding
// each present parameter, in SheetQueryParams order. A parameter sent twice or sent empty, or any
// of them mixed with data, is QueryRefused bad_query. Values are not checked here: the read
// refuses a bad one with its own code.
func ParseSheetQuery(raw string) (Value, error) {
	params := formPairs(raw)
	query := NewObject()
	var present []string
	for _, key := range SheetQueryParams {
		var values []string
		for _, p := range params {
			if p[0] == key {
				values = append(values, p[1])
			}
		}
		if len(values) == 0 {
			continue
		}
		if len(values) > 1 {
			return nil, &gridcodec.QueryRefused{Code: "bad_query", Message: fmt.Sprintf("%s appears %d times; send it once", key, len(values))}
		}
		if values[0] == "" {
			return nil, &gridcodec.QueryRefused{Code: "bad_query", Message: fmt.Sprintf("%s is empty; give it a value or leave it out", key)}
		}
		query.Set(key, values[0])
		present = append(present, key)
	}
	if len(present) == 0 {
		return nil, nil
	}
	for _, p := range params {
		if p[0] == "data" {
			return nil, &gridcodec.QueryRefused{Code: "bad_query", Message: fmt.Sprintf("data cannot be combined with %s; a sheet query reads the document's own rules", strings.Join(present, ", "))}
		}
	}
	return query, nil
}

// formPairs parses application/x-www-form-urlencoded the way URLSearchParams does (WHATWG URL
// standard): split on "&", skip empty pieces, split each at its first "=", turn "+" into a space,
// percent-decode leniently (a "%" not followed by two hex digits stays as it is), and decode the
// bytes as UTF-8 with invalid sequences replaced by U+FFFD. Go's url.ParseQuery is stricter (it
// refuses a bad escape and a ";"), which would refuse requests the other hosts answer.
func formPairs(raw string) [][2]string {
	var out [][2]string
	for _, piece := range strings.Split(raw, "&") {
		if piece == "" {
			continue
		}
		name, value, _ := strings.Cut(piece, "=")
		out = append(out, [2]string{formDecode(name), formDecode(value)})
	}
	return out
}

func formDecode(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b = append(b, unhex(s[i+1])<<4|unhex(s[i+2]))
			i += 2
			continue
		}
		b = append(b, s[i])
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return strings.ToValidUTF8(string(b), "�")
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}
