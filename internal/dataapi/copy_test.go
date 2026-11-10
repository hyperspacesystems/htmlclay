package dataapi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/testutil"
)

type copyFixture struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

// TestStripCopyContentFixture runs the accepted cross-implementation fixture set: the same inputs
// the reference filters, so a difference here is a difference from the export's answer.
func TestStripCopyContentFixture(t *testing.T) {
	raw, err := os.ReadFile(testutil.VectorPath(t, "copy-content.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var cases []copyFixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("fixture holds no cases")
	}
	t.Logf("%d fixture cases", len(cases))

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := StripCopyContent([]byte(c.Input))
			if err != nil {
				t.Fatalf("StripCopyContent(%q): %v", c.Input, err)
			}
			if string(got) != c.Expected {
				t.Fatalf("StripCopyContent(%q) =\n%q\nwant\n%q", c.Input, got, c.Expected)
			}
		})
	}
}

// copyCase is one hand-written document: what the export must keep, byte for byte.
type copyCase struct {
	name  string
	input string
	want  string
}

var copyCases = []copyCase{
	{
		name:  "implied list item end after a self closing child",
		input: "<ul><li clay=\"no-copy\">a<br/>\n<li>b</ul>",
		want:  "<ul><li>b</ul>",
	},
	{
		name:  "implied paragraph end after a self closing child",
		input: "<p clay=\"no-copy\">text<br/>\n<p>keep</p>",
		want:  "<p>keep</p>",
	},
	{
		name:  "implied cell end with a quoted angle bracket in its child",
		input: "<table><tr><td clay=\"no-copy\">x<img src=\"a>b.png\"/>\n<td>y</tr></table>",
		want:  "<table><tr><td>y</tr></table>",
	},
	{
		name:  "markup-looking text in an unmarked script is not markup",
		input: "<body>\n  <script>var s = '<div clay=\"no-copy\">x</div>';</script>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <script>var s = '<div clay=\"no-copy\">x</div>';</script>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "a marked script goes whole, markup-looking text and all",
		input: "<body>\n  <script clay=\"no-copy\">var s = '<div clay=\"no-copy\">x</div>';</script>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "an entity-encoded marker value is still the marker",
		input: "<body>\n  <div clay=\"no&#45;copy\">Tracked</div>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "marked element inside a template is removed",
		input: "<body>\n  <template>\n    <div clay=\"no-copy\">Tracked</div>\n  </template>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <template>\n  </template>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "marked element inside a template nested in a template is removed",
		input: "<body>\n  <template>\n    <template>\n      <div clay=\"no-copy\">T</div>\n    </template>\n  </template>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <template>\n    <template>\n    </template>\n  </template>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "an unmarked template keeps its content",
		input: "<body>\n  <template>\n    <div>Kept</div>\n  </template>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <template>\n    <div>Kept</div>\n  </template>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "omitted p and li ends with inline formatting inside the cut",
		input: "<body>\n  <p>Intro\n  <ul>\n    <li>One</li><li clay=\"no-copy\">Two <b>bold</b>\n    <li>Three\n  </ul>\n  <p>Keep\n</body>\n",
		want:  "<body>\n  <p>Intro\n  <ul>\n    <li>One</li><li>Three\n  </ul>\n  <p>Keep\n</body>\n",
	},
	{
		name:  "a marked list item holding a list is removed whole",
		input: "<body>\n  <ul>\n    <li clay=\"no-copy\">Outer\n      <ul>\n        <li>Inner</li>\n      </ul>\n    </li>\n    <li>Keep</li>\n  </ul>\n</body>\n",
		want:  "<body>\n  <ul>\n    <li>Keep</li>\n  </ul>\n</body>\n",
	},
	{
		name:  "svg with self-closing children is removed whole",
		input: "<body>\n  <svg clay=\"no-copy\"><path d=\"M0\"/><g><rect/></g></svg>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "a self-closing svg child is removed on its own",
		input: "<body>\n  <svg><path clay=\"no-copy\" d=\"M0\"/><rect/></svg>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <svg><rect/></svg>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "a byte order mark survives the cut",
		input: "\ufeff<body>\n  <div clay=\"no-copy\">A</div>\n  <p>Keep</p>\n</body>\n",
		want:  "\ufeff<body>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "invalid utf-8 outside the cut survives",
		input: "<body>\n  <p>caf\xe9 raw</p>\n  <div clay=\"no-copy\">A</div>\n  <p>tail \xff</p>\n</body>\n",
		want:  "<body>\n  <p>caf\xe9 raw</p>\n  <p>tail \xff</p>\n</body>\n",
	},
	{
		name:  "multibyte text right before the cut does not move it",
		input: "<body>\n  <p>\u00e9\u4e2d\U0001F389</p><div clay=\"no-copy\">x</div>\n  <p>Keep \u00e9</p>\n</body>\n",
		want:  "<body>\n  <p>\u00e9\u4e2d\U0001F389</p>\n  <p>Keep \u00e9</p>\n</body>\n",
	},
	{
		name:  "two adjacent cuts keep their crlf line endings",
		input: "<body>\r\n  <div clay=\"no-copy\">A</div>\r\n  <div clay=\"no-copy\">B</div>\r\n  <p>Keep</p>\r\n</body>\r\n",
		want:  "<body>\r\n  <p>Keep</p>\r\n</body>\r\n",
	},
	{
		name:  "an uppercase attribute name with the exact conditional value is removed",
		input: "<body>\n  <div CLAY=\"no-copy-unless:paid-forms\">A</div>\n  <p>Keep</p>\n</body>\n",
		want:  "<body>\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "malformed markup with no marker is left alone",
		input: "<body>\n  <table>\n    <div>fostered</div>\n    <tr><td>cell</td></tr>\n  </table>\n</body>\n",
		want:  "<body>\n  <table>\n    <div>fostered</div>\n    <tr><td>cell</td></tr>\n  </table>\n</body>\n",
	},
}

func TestStripCopyContent(t *testing.T) {
	if len(copyCases) == 0 {
		t.Fatal("no cases")
	}
	t.Logf("%d hand-written cases", len(copyCases))

	for _, c := range copyCases {
		t.Run(strings.ReplaceAll(c.name, " ", "-"), func(t *testing.T) {
			got, err := StripCopyContent([]byte(c.input))
			if err != nil {
				t.Fatalf("StripCopyContent(%q): %v", c.input, err)
			}
			if string(got) != c.want {
				t.Fatalf("StripCopyContent(%q) =\n%q\nwant\n%q", c.input, got, c.want)
			}
		})
	}
}

// An unmarked document must come back as the very same bytes, not a copy: an export of ordinary
// content is not allowed to disturb the file's encoding, line endings or trailing bytes.
func TestStripCopyContentReturnsTheInputWhenUnmarked(t *testing.T) {
	in := []byte("<!doctype html>\n<p>plain</p>\n")
	out, err := StripCopyContent(in)
	if err != nil {
		t.Fatalf("StripCopyContent: %v", err)
	}
	if len(out) != len(in) || (len(out) > 0 && &out[0] != &in[0]) {
		t.Fatalf("StripCopyContent copied an unmarked document: %q", out)
	}

	// The marker word in text is not a marker, and neither is a section cut that leaves bytes.
	text := []byte("<body>\n  <p>the clay class mentions data-paid-only</p>\n</body>\n")
	got, err := StripCopyContent(text)
	if err != nil {
		t.Fatalf("StripCopyContent: %v", err)
	}
	if string(got) != string(text) {
		t.Fatalf("StripCopyContent(%q) = %q, want it unchanged", text, got)
	}
}

// copyRefusalCases are documents with a marked section whose byte range cannot be recovered. The
// export refuses rather than writing half a copy or a re-rendered one.
var copyRefusalCases = []copyCase{
	{
		name:  "foster-parented marked div",
		input: "<body>\n  <table>\n    <div clay=\"no-copy\">x</div>\n    <tr><td>cell</td></tr>\n  </table>\n</body>\n",
	},
	{
		name:  "self-closing non-void html marker with content after it",
		input: "<body>\n  <div clay=\"no-copy\" />\n  <p>Keep</p>\n</body>\n",
	},
	{
		name:  "adoption-agency misnesting with a marked section",
		input: "<body>\n  <b><p clay=\"no-copy\">x</b>y\n  <p>Keep</p>\n</body>\n",
	},
}

func TestStripCopyContentRefusesUnlocatableRanges(t *testing.T) {
	if len(copyRefusalCases) == 0 {
		t.Fatal("no cases")
	}
	t.Logf("%d refusal cases", len(copyRefusalCases))

	for _, c := range copyRefusalCases {
		t.Run(strings.ReplaceAll(c.name, " ", "-"), func(t *testing.T) {
			in := []byte(c.input)
			out, err := StripCopyContent(in)
			if err == nil {
				t.Fatalf("StripCopyContent(%q) = %q, want a refusal", c.input, out)
			}
			if out != nil {
				t.Fatalf("refusal returned bytes %q", out)
			}
			if !strings.Contains(err.Error(), "no-copy section could not be located safely") {
				t.Fatalf("error = %v, want the actionable refusal", err)
			}
			if string(in) != c.input {
				t.Fatalf("refusal mutated the source: %q", in)
			}
		})
	}
}
