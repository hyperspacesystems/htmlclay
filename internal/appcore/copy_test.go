package appcore

import (
	"strings"
	"testing"
)

func TestEveryRefusalCodeHasText(t *testing.T) {
	codes := []string{
		RefuseNotAFolder,
		RefuseOutsideHome,
		RefuseIsHome,
		RefuseProtectedFolder,
		RefuseOverlapsServed,
		RefuseServedByLocal,
		RefuseBoundToLocalSync,
		RefuseNestedTrusted,
		RefuseNameInvalid,
		RefuseNameTaken,
		RefuseNameRetired,
		RefuseServedElsewhere,
	}
	for _, code := range codes {
		text, ok := RefusalText[code]
		if !ok || text == "" {
			t.Errorf("RefusalText has no line for %q", code)
			continue
		}
		if strings.Contains(text, "%!") {
			t.Errorf("RefusalText[%q] = %q, which is not a format string", code, text)
		}
	}
}

func TestRefusalMessageFillsInTheOtherFolder(t *testing.T) {
	byPath := RefusalMessage(&Refusal{Code: RefuseOverlapsServed, OtherPath: "/home/me/notes"})
	if !strings.Contains(byPath, "/home/me/notes") {
		t.Errorf("RefusalMessage with a path = %q, want the path in it", byPath)
	}
	byName := RefusalMessage(&Refusal{Code: RefuseOverlapsServed, OtherPath: "/home/me/notes", OtherName: "notes"})
	if !strings.Contains(byName, "notes") || strings.Contains(byName, "/home/me/notes") {
		t.Errorf("RefusalMessage with a name = %q, want the name instead of the path", byName)
	}
	if got := RefusalMessage(&Refusal{Code: RefuseIsHome}); got != RefusalText[RefuseIsHome] {
		t.Errorf("RefusalMessage of a text without %%s = %q, want it unchanged", got)
	}
	if got := RefusalMessage(&Refusal{Code: "not-a-code"}); got != "not-a-code" {
		t.Errorf("RefusalMessage of an unknown code = %q, want the code", got)
	}
}
