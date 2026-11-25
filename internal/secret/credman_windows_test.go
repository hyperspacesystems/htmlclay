package secret

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func testProfile(t *testing.T) string {
	t.Helper()
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	return "test-" + hex.EncodeToString(random)
}

func TestM4CredentialManager(t *testing.T) {
	c := newCredman(testProfile(t))
	t.Cleanup(func() {
		for _, name := range []string{"sync-key", "gone", "absent"} {
			_ = c.Delete(name)
		}
	})
	if c.Kind() != KindSystem {
		t.Fatalf("Kind = %q", c.Kind())
	}
	contract(t, c)

	t.Run("two profiles do not share a key", func(t *testing.T) {
		other := newCredman(testProfile(t))
		t.Cleanup(func() { _ = other.Delete("sync-key") })
		if err := c.Set("sync-key", "hcsk_first"); err != nil {
			t.Fatal(err)
		}
		if err := other.Set("sync-key", "hcsk_second"); err != nil {
			t.Fatal(err)
		}
		if value, _, _ := c.Get("sync-key"); value != "hcsk_first" {
			t.Fatalf("first profile reads %q", value)
		}
	})
}
