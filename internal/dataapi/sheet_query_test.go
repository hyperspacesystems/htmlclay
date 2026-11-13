package dataapi

import (
	"errors"
	"testing"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

func TestParseSheetQueryAbsent(t *testing.T) {
	for _, raw := range []string{"", "v=7", "data=%7B%7D"} {
		query, err := ParseSheetQuery(raw)
		if err != nil {
			t.Errorf("%q: unexpected error %v", raw, err)
			continue
		}
		if query != nil {
			t.Errorf("%q: query = %#v, want nil", raw, query)
		}
	}
}

func TestParseSheetQueryValues(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"sheet=Budget&range=A1%3AC3&render=formulas", `{"sheet":"Budget","range":"A1:C3","render":"formulas"}`},
		{"render=values&shape=index", `{"shape":"index","render":"values"}`},
		{"sheet=a+b%20c", `{"sheet":"a b c"}`},
		{"sheet=%41", `{"sheet":"A"}`},
		{"sheet=%zz", `{"sheet":"%zz"}`},
		{"sheet=%E2%82", `{"sheet":"` + "\uFFFD" + `"}`},
	}
	for _, c := range cases {
		query, err := ParseSheetQuery(c.raw)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.raw, err)
			continue
		}
		out, err := Marshal(query)
		if err != nil {
			t.Errorf("%q: marshal: %v", c.raw, err)
			continue
		}
		if string(out) != c.want {
			t.Errorf("%q: got %s, want %s", c.raw, out, c.want)
		}
	}
}

func TestParseSheetQueryRefusals(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"range=A1&range=B2", "range appears 2 times; send it once"},
		{"sheet=", "sheet is empty; give it a value or leave it out"},
		{"shape=index&data=x", "data cannot be combined with shape; a sheet query reads the document's own rules"},
		{"shape=index&data=", "data cannot be combined with shape; a sheet query reads the document's own rules"},
		{"range=A1&sheet=S&data=1", "data cannot be combined with sheet, range; a sheet query reads the document's own rules"},
	}
	for _, c := range cases {
		query, err := ParseSheetQuery(c.raw)
		if err == nil {
			t.Errorf("%q: got %#v, want a refusal", c.raw, query)
			continue
		}
		var refused *gridcodec.QueryRefused
		if !errors.As(err, &refused) {
			t.Errorf("%q: error %T, want *gridcodec.QueryRefused", c.raw, err)
			continue
		}
		if refused.Code != "bad_query" {
			t.Errorf("%q: code = %q, want bad_query", c.raw, refused.Code)
		}
		if refused.Message != c.want {
			t.Errorf("%q: message = %q, want %q", c.raw, refused.Message, c.want)
		}
	}
}
