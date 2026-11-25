//go:build linux

package secret

import (
	"bufio"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeSecrets is the part of the Secret Service the store calls, exported on a
// private bus the test starts, so no real keyring is touched.
type fakeSecrets struct {
	mu         sync.Mutex
	conn       *dbus.Conn
	items      map[dbus.ObjectPath]*fakeItem
	next       int
	locked     bool
	unlockable bool
}

type fakeItem struct {
	owner *fakeSecrets
	path  dbus.ObjectPath
	attrs map[string]string
	value []byte
}

func (f *fakeSecrets) OpenSession(algorithm string, input dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	if algorithm != "plain" {
		return dbus.Variant{}, "", dbus.MakeFailedError(fmt.Errorf("unsupported %s", algorithm))
	}
	return dbus.MakeVariant(""), "/org/freedesktop/secrets/session/1", nil
}

func (f *fakeSecrets) SearchItems(attrs map[string]string) ([]dbus.ObjectPath, []dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []dbus.ObjectPath
	for path, item := range f.items {
		if maps.Equal(item.attrs, attrs) {
			found = append(found, path)
		}
	}
	if f.locked {
		return []dbus.ObjectPath{}, append([]dbus.ObjectPath{}, found...), nil
	}
	return append([]dbus.ObjectPath{}, found...), []dbus.ObjectPath{}, nil
}

func (f *fakeSecrets) Unlock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.unlockable {
		return []dbus.ObjectPath{}, "/org/freedesktop/secrets/prompt/1", nil
	}
	f.locked = false
	return objects, "/", nil
}

func (f *fakeSecrets) CreateItem(properties map[string]dbus.Variant, secret ssSecret, replace bool) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locked {
		return "/", "/org/freedesktop/secrets/prompt/2", nil
	}
	attrs, ok := properties[ssItem+".Attributes"].Value().(map[string]string)
	if !ok {
		return "", "", dbus.MakeFailedError(fmt.Errorf("no attributes"))
	}
	if replace {
		for _, item := range f.items {
			if maps.Equal(item.attrs, attrs) {
				item.value = secret.Value
				return item.path, "/", nil
			}
		}
	}
	f.next++
	item := &fakeItem{owner: f, path: dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/secrets/collection/login/%d", f.next)), attrs: attrs, value: secret.Value}
	f.items[item.path] = item
	if err := f.conn.Export(item, item.path, ssItem); err != nil {
		return "", "", dbus.MakeFailedError(err)
	}
	return item.path, "/", nil
}

// setLock sets the collection's lock under the fake's lock: D-Bus calls run on
// the connection's own goroutines.
func (f *fakeSecrets) setLock(locked, unlockable bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locked, f.unlockable = locked, unlockable
}

// Get is the collection's org.freedesktop.DBus.Properties.Get.
func (f *fakeSecrets) Get(iface, property string) (dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if iface == ssCollection && property == "Locked" {
		return dbus.MakeVariant(f.locked), nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("no property %s.%s", iface, property))
}

func (i *fakeItem) GetSecret(session dbus.ObjectPath) (ssSecret, *dbus.Error) {
	i.owner.mu.Lock()
	defer i.owner.mu.Unlock()
	return ssSecret{Session: session, Value: i.value, ContentType: "text/plain"}, nil
}

func (i *fakeItem) Delete() (dbus.ObjectPath, *dbus.Error) {
	i.owner.mu.Lock()
	defer i.owner.mu.Unlock()
	delete(i.owner.items, i.path)
	i.owner.conn.Export(nil, i.path, ssItem)
	return "/", nil
}

// privateBus starts a session bus of its own with the fake Secret Service on it
// and points DBUS_SESSION_BUS_ADDRESS at it.
func privateBus(t *testing.T) *fakeSecrets {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Fatalf("dbus-daemon is not installed (the box setup installs the dbus package): %v", err)
	}
	socket := filepath.Join(t.TempDir(), "bus")
	cmd := exec.Command(daemon, "--session", "--nofork", "--print-address=1", "--address=unix:path="+socket)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	address, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("dbus-daemon printed no address: %v", err)
	}
	address = strings.TrimSpace(address)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fake := &fakeSecrets{conn: conn, items: map[dbus.ObjectPath]*fakeItem{}}
	for _, export := range []struct {
		path  dbus.ObjectPath
		iface string
	}{{ssPath, ssService}, {ssDefault, ssCollection}, {ssDefault, "org.freedesktop.DBus.Properties"}} {
		if err := conn.Export(fake, export.path, export.iface); err != nil {
			t.Fatal(err)
		}
	}
	if reply, err := conn.RequestName(ssName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName = %v, %v", reply, err)
	}
	return fake
}

func kindOf(s Store) string {
	if s == nil {
		return "no store"
	}
	return s.Kind()
}

func TestM4SecretServiceLinux(t *testing.T) {
	t.Run("the Secret Service keeps the key", func(t *testing.T) {
		fake := privateBus(t)
		configDir := t.TempDir()
		s, err := Open("default", configDir)
		if err != nil || kindOf(s) != KindSystem {
			t.Fatalf("Open = %v, kind %q", err, kindOf(s))
		}
		contract(t, s)
		if matches, _ := filepath.Glob(filepath.Join(configDir, "secrets", "*")); len(matches) != 0 {
			t.Fatalf("the store wrote files: %v", matches)
		}
		fake.mu.Lock()
		count := len(fake.items)
		fake.mu.Unlock()
		if count != 1 {
			t.Fatalf("items left = %d, want only sync-key", count)
		}
	})

	t.Run("each profile has its own items", func(t *testing.T) {
		privateBus(t)
		one, _ := Open("default", t.TempDir())
		two, _ := Open("work", t.TempDir())
		if err := one.Set("sync-key", "hcsk_default"); err != nil {
			t.Fatal(err)
		}
		if _, found, err := two.Get("sync-key"); err != nil || found {
			t.Fatalf("the work profile read the default profile's key: found=%v err=%v", found, err)
		}
	})

	t.Run("a collection that unlocks without asking is used", func(t *testing.T) {
		fake := privateBus(t)
		fake.setLock(true, true)
		if s, err := Open("default", t.TempDir()); err != nil || kindOf(s) != KindSystem {
			t.Fatalf("Open = %v, kind %q", err, kindOf(s))
		}
	})

	t.Run("a collection that stays locked falls back to the file", func(t *testing.T) {
		fake := privateBus(t)
		fake.setLock(true, false)
		s, err := Open("default", t.TempDir())
		if err != nil || kindOf(s) != KindFile {
			t.Fatalf("Open = %v, kind %q", err, kindOf(s))
		}
	})

	t.Run("no bus falls back to the file", func(t *testing.T) {
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(t.TempDir(), "no-bus"))
		configDir := t.TempDir()
		s, err := Open("default", configDir)
		if err != nil || kindOf(s) != KindFile {
			t.Fatalf("Open = %v, kind %q", err, kindOf(s))
		}
		contract(t, s)
	})
}
