package secret

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

func TestM4Keychain(t *testing.T) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	k := newKeychain("com.htmlclay.test." + hex.EncodeToString(random))
	t.Cleanup(func() {
		for _, name := range []string{"sync-key", "gone", "absent"} {
			_ = k.Delete(name)
		}
	})
	if k.Kind() != KindSystem {
		t.Fatalf("Kind = %q", k.Kind())
	}
	contract(t, k)

	t.Run("the value is never an argument", func(t *testing.T) {
		const value = "hcsk_0123456789abcdef"
		cmd := k.command("add-generic-password -U -s x -a sync-key -w " + value)
		for _, arg := range cmd.Args {
			if strings.Contains(arg, value) {
				t.Fatalf("argument %q carries the value", arg)
			}
		}
	})
	t.Run("a value the line cannot carry is refused", func(t *testing.T) {
		for _, value := range []string{"two words", `quote"d`, `back\slash`, ""} {
			if err := k.Set("sync-key", value); err == nil {
				t.Errorf("Set(%q) succeeded", value)
			}
		}
	})
}
