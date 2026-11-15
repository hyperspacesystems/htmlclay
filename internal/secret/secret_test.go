package secret

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// contract is every store's behaviour; 8.2 to 8.4 run it on the system stores.
func contract(t *testing.T, s Store) {
	t.Helper()
	t.Run("a missing item is not found", func(t *testing.T) {
		value, found, err := s.Get("absent")
		if err != nil || found || value != "" {
			t.Fatalf("Get(absent) = %q, %v, %v", value, found, err)
		}
	})
	t.Run("a set item reads back and is replaced by a second set", func(t *testing.T) {
		if err := s.Set("sync-key", "hcsk_one"); err != nil {
			t.Fatal(err)
		}
		if err := s.Set("sync-key", "hcsk_two"); err != nil {
			t.Fatal(err)
		}
		value, found, err := s.Get("sync-key")
		if err != nil || !found || value != "hcsk_two" {
			t.Fatalf("Get = %q, %v, %v", value, found, err)
		}
	})
	t.Run("a deleted item is gone, and deleting it again is not an error", func(t *testing.T) {
		if err := s.Set("gone", "x"); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete("gone"); err != nil {
			t.Fatal(err)
		}
		if _, found, err := s.Get("gone"); err != nil || found {
			t.Fatalf("after Delete: found=%v err=%v", found, err)
		}
		if err := s.Delete("gone"); err != nil {
			t.Fatalf("second Delete: %v", err)
		}
	})
	t.Run("a bad name is refused", func(t *testing.T) {
		for _, name := range []string{"", "../x", "a/b", "Upper", ".hidden", "a b"} {
			if err := s.Set(name, "x"); !errors.Is(err, ErrBadName) {
				t.Errorf("Set(%q) = %v, want ErrBadName", name, err)
			}
		}
	})
}

func TestM4SecretStore(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		s := NewMemory()
		if s.Kind() != KindMemory {
			t.Fatalf("Kind = %q", s.Kind())
		}
		contract(t, s)
	})
	t.Run("file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "config")
		s := NewFile(dir)
		if s.Kind() != KindFile {
			t.Fatalf("Kind = %q", s.Kind())
		}
		contract(t, s)
		t.Run("the file is private", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows has no file modes")
			}
			if err := s.Set("private", "hcsk_x"); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(dir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Fatalf("mode = %o, want 600", perm)
			}
		})
		t.Run("a file other users can read is refused", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows has no file modes")
			}
			path := filepath.Join(dir, "wide")
			if err := os.WriteFile(path, []byte("hcsk_x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, found, err := s.Get("wide"); err == nil || found {
				t.Fatalf("Get(wide) found=%v err=%v, want a refusal", found, err)
			}
		})
		t.Run("a link is refused", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("links need privileges on Windows")
			}
			target := filepath.Join(t.TempDir(), "elsewhere")
			if err := os.WriteFile(target, []byte("hcsk_x"), 0o600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "linked")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if _, found, err := s.Get("linked"); err == nil || found {
				t.Fatalf("Get(linked) found=%v err=%v, want a refusal", found, err)
			}
			if err := s.Set("linked", "hcsk_y"); err == nil {
				t.Fatal("Set over a link succeeded")
			}
			if data, _ := os.ReadFile(target); string(data) != "hcsk_x" {
				t.Fatalf("the link's target changed to %q", data)
			}
		})
	})
}
