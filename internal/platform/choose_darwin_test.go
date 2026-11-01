//go:build darwin

package platform

import (
	"strings"
	"testing"
)

func TestChooseDialogHasNoDefaultButtonOnMac(t *testing.T) {
	script := confirmDialogScript("Allow", "message", chooseLabels("Deny", "More Options…", "Allow clay-search"))
	if strings.Contains(script, "default button") || strings.Contains(script, "cancel button") {
		t.Fatalf("script sets a default or cancel button:\n%s", script)
	}
	if !strings.Contains(script, `buttons {"Deny", "More Options…", "Allow clay-search"}`) {
		t.Fatalf("buttons out of order:\n%s", script)
	}
}
