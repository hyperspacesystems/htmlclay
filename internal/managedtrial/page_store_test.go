//go:build managedtrial

package managedtrial

import "testing"

// TestHasPageStore pins Local's hasPageStore: a <script> with a page merge
// attribute whose clay attribute names the `live` token.
func TestHasPageStore(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     bool
	}{
		{"page", `<script merge="blockeditor" clay="live"></script>`, true},
		{"numbered page", `<script merge="blockeditor.2" clay="x live"></script>`, true},
		{"upper-case names", `<script MERGE="blockeditor" CLAY="live"></script>`, true},
		{"lively is not live", `<script merge="blockeditor" clay="lively"></script>`, false},
		{"blockeditors is not blockeditor", `<script merge="blockeditors" clay="live"></script>`, false},
		{"no clay", `<script merge="blockeditor"></script>`, false},
		{"not a script", `<div merge="blockeditor" clay="live"></div>`, false},
		{"first merge wins", `<script merge="blockeditor" clay="live" merge="x"></script>`, true},
		{"first merge is not page", `<script merge="x" clay="live" merge="blockeditor"></script>`, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document := "<!doctype html><html><head>" + testCase.document + "</head><body></body></html>"
			if got := hasPageStore(document); got != testCase.want {
				t.Fatalf("hasPageStore(%s) = %v, want %v", testCase.document, got, testCase.want)
			}
		})
	}
}
