package dataapi

import "testing"

func TestStaleMarkGoesInTheHead(t *testing.T) {
	const mark = `<meta name="clay-stale" clay="no-save">`
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			// A mark anywhere but the head is a mark a page's save never finds and strips.
			"closed-head",
			`<html><head><title>t</title></head><body></body></html>`,
			`<html><head><title>t</title>` + mark + `</head><body></body></html>`,
		},
		{
			// A head left open would otherwise take the mark at the end of the file, after the body.
			"unclosed-head",
			`<html><head><title>t</title><body></body></html>`,
			`<html><head>` + mark + `<title>t</title><body></body></html>`,
		},
		{
			// A document with no head at all still needs one, or the mark has no home.
			"no-head",
			`<html><body><p>x</p></body></html>`,
			`<html><head>` + mark + `</head><body><p>x</p></body></html>`,
		},
		{
			// A document with no head and no body has nowhere to put the mark; it must not gain one
			// in the middle of the content.
			"no-head-no-body",
			`<p>x</p>`,
			`<p>x</p>`,
		},
		{
			// A </head> written inside a raw-text element is text, not the head's end: taking it
			// would drop the mark inside the title, where nothing reads it.
			"raw-text-close-in-title",
			`<html><head><title>a</head>b</title></head><body></body></html>`,
			`<html><head><title>a</head>b</title>` + mark + `</head><body></body></html>`,
		},
		{
			// A second mark makes a file that never settles: every write would look like it changed
			// the head again.
			"already-marked",
			`<html><head>` + mark + `</head><body></body></html>`,
			`<html><head>` + mark + `</head><body></body></html>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(withStaleMark([]byte(c.src)))
			if got != c.want {
				t.Errorf("withStaleMark(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}
