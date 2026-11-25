package wire

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSetupBarPostAddress(t *testing.T) {
	loose := string(SetupBarHTML([]string{"ocr"}, true))
	if !strings.Contains(loose, "getAttribute('savetoken')") {
		t.Fatalf("the loose bar does not read the savetoken")
	}
	if !strings.Contains(loose, "'/_/helper-setup/'+") {
		t.Fatalf("the loose bar does not post to the token address")
	}

	served := string(SetupBarHTML([]string{"ocr"}, false))
	if !strings.Contains(served, "fetch('/_/helper-setup',") {
		t.Fatalf("the served bar does not post to /_/helper-setup")
	}
	if strings.Contains(served, "savetoken") {
		t.Fatalf("the served bar reads a savetoken")
	}

	golden, err := os.ReadFile("testdata/setup_bar_token.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := SetupBarHTML([]string{"ocr", "search"}, true); !bytes.Equal(got, golden) {
		t.Fatalf("the loose bar bytes changed:\ngot  %q\nwant %q", got, golden)
	}
}
