package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestM4SyncFile(t *testing.T) {
	t.Run("a missing file is a new record with a device id, not yet written", func(t *testing.T) {
		dir := t.TempDir()
		s, err := LoadSync(dir)
		if err != nil {
			t.Fatal(err)
		}
		if s.V != SyncVersion || s.Account != nil || s.Paused {
			t.Fatalf("new record = %+v", s)
		}
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(s.DeviceID) {
			t.Fatalf("deviceId %q is not a v4 UUID", s.DeviceID)
		}
		if _, err := os.Stat(filepath.Join(dir, "sync.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("LoadSync wrote the file: %v", err)
		}
	})

	t.Run("a saved record reads back the same, readable only by this user", func(t *testing.T) {
		dir := t.TempDir()
		s, err := LoadSync(dir)
		if err != nil {
			t.Fatal(err)
		}
		reason := "plan-lapsed"
		s.Account = &SyncAccount{ActorID: 12, Username: "maya", FullName: "Maya Lin", KeySuffix: "9f3c", KeyStore: "system", ConnectedAt: "2026-11-15T06:00:00Z"}
		s.Paused = true
		s.Accounts = []SyncAccountRow{{AccountID: 40, Kind: "team", Username: "studio", DisplayName: "Studio", Role: "editor", FolderID: "0123456789abcdef", Sync: SyncAccountSync{Enabled: false, Reason: &reason}}}
		s.SyncSessions = []SyncSession{{ID: "6f0d6c7e-6b1c-4b8e-9a51-1d2f3e4a5b6c", RootID: "0123456789abcdef", AccountID: 40, Kind: "team", Cached: SyncSessionCached{Username: "studio", DisplayName: "Studio", Role: "editor"}, Enabled: true}}
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
		again, err := LoadSync(dir)
		if err != nil {
			t.Fatal(err)
		}
		again.path, s.path = "", ""
		if !reflect.DeepEqual(again, s) {
			t.Fatalf("read back\n%+v\nwant\n%+v", again, s)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(filepath.Join(dir, "sync.json"))
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Fatalf("mode %o, want 600", perm)
			}
		}
	})

	t.Run("a newer or broken file is refused and left exactly as it is", func(t *testing.T) {
		for name, body := range map[string]string{
			"newer":  `{"v": 2, "deviceId": "x"}`,
			"broken": `{"v": 1, "deviceId": `,
		} {
			dir := t.TempDir()
			path := filepath.Join(dir, "sync.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSync(dir); err == nil {
				t.Fatalf("%s: LoadSync accepted it", name)
			}
			if name == "newer" {
				if _, err := LoadSync(dir); !errors.Is(err, ErrSyncVersion) {
					t.Fatalf("newer: err = %v, want ErrSyncVersion", err)
				}
			}
			if data, _ := os.ReadFile(path); string(data) != body {
				t.Fatalf("%s: file changed to %q", name, data)
			}
		}
	})

	t.Run("the notified list keeps the newest 500", func(t *testing.T) {
		s, err := LoadSync(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 520; i++ {
			s.Notified = append(s.Notified, "n"+strings.Repeat("x", i%3)+string(rune('a'+i%26)))
		}
		last := s.Notified[519]
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
		if len(s.Notified) != 500 || s.Notified[499] != last {
			t.Fatalf("len %d, last %q", len(s.Notified), s.Notified[len(s.Notified)-1])
		}
	})

	t.Run("no field can hold a key", func(t *testing.T) {
		allowed := map[string]bool{"keySuffix": true, "keyStore": true, "keyRefused": true}
		seen := map[reflect.Type]bool{}
		var walk func(reflect.Type, string)
		walk = func(ty reflect.Type, at string) {
			for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice {
				ty = ty.Elem()
			}
			if ty.Kind() != reflect.Struct || seen[ty] {
				return
			}
			seen[ty] = true
			for i := 0; i < ty.NumField(); i++ {
				f := ty.Field(i)
				if !f.IsExported() {
					continue
				}
				name := strings.Split(f.Tag.Get("json"), ",")[0]
				if name == "" {
					name = f.Name
				}
				if strings.Contains(strings.ToLower(name), "key") && !allowed[name] {
					t.Errorf("%s.%s (json %q) could hold a key", at, f.Name, name)
				}
				if strings.Contains(strings.ToLower(name), "secret") || strings.Contains(strings.ToLower(name), "token") {
					t.Errorf("%s.%s (json %q) could hold a secret", at, f.Name, name)
				}
				walk(f.Type, at+"."+f.Name)
			}
		}
		walk(reflect.TypeOf(Sync{}), "Sync")
		if len(seen) < 6 {
			t.Fatalf("walked only %d types; the walk is not reaching the record", len(seen))
		}
	})
}
