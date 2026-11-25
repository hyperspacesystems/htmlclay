//go:build linux

package platform

import (
	"slices"
	"testing"
)

func TestZenityFileArgsFilterDocuments(t *testing.T) {
	args := zenityFileArgs("Open a file")
	if !slices.Contains(args, "--file-selection") {
		t.Fatalf("args %q do not ask zenity for a file", args)
	}
	if !slices.Contains(args, "--file-filter=HTML Clay documents | *.htmlclay *.html *.htm") {
		t.Fatalf("args %q do not filter for HTML Clay documents", args)
	}
}
