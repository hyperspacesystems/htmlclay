//go:build windows

package platform

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func windowsPrimary(t *testing.T) (SingleInstance, string) {
	t.Helper()
	dir := t.TempDir()
	si := NewSingleInstance(dir)
	ok, err := si.TryLock()
	if err != nil || !ok {
		t.Fatalf("TryLock = %v, %v", ok, err)
	}
	t.Cleanup(func() { si.Unlock() })
	return si, dir
}

// rawFrame sends bytes to the primary as any local process could and returns
// what comes back.
func rawFrame(t *testing.T, dir, frame string) string {
	t.Helper()
	addr, err := os.ReadFile(filepath.Join(dir, "addr"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", strings.TrimSpace(string(addr)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !verifyBanner(conn) {
		t.Fatal("no banner")
	}
	conn.Write([]byte(frame))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	return strings.TrimSpace(string(buf[:n]))
}

func TestWindowsCommandNeedsTheInstanceSecret(t *testing.T) {
	si, dir := windowsPrimary(t)
	opened := make(chan string, 1)
	si.OnFileReceived(func(p string) { opened <- p })
	var ran []string
	si.OnCommand(func(cmd string) string { ran = append(ran, cmd); return "ran" })

	answer, err := NewSingleInstance(dir).SendCommand(`serve C:\Users\me\My Site --name site`)
	if err != nil || answer != "ran" {
		t.Fatalf("SendCommand = %q, %v; want ran", answer, err)
	}
	if len(ran) != 1 || ran[0] != `serve C:\Users\me\My Site --name site` {
		t.Fatalf("the handler got %q", ran)
	}

	if answer := rawFrame(t, dir, "\x00serve C:\\site\n"); !strings.HasPrefix(answer, "refused ") {
		t.Fatalf("a command with no secret was answered %q", answer)
	}
	wrong, _ := newInstanceSecret()
	if answer := rawFrame(t, dir, "\x00"+sealCommand(wrong, "serve C:\\site")+"\n"); !strings.HasPrefix(answer, "refused ") {
		t.Fatalf("a command with another secret was answered %q", answer)
	}
	if len(ran) != 1 {
		t.Fatalf("a refused command reached the handler: %q", ran)
	}
	select {
	case p := <-opened:
		t.Fatalf("a command frame was opened as the file %q", p)
	case <-time.After(100 * time.Millisecond):
	}

	// A second instance of any version still hands over a file with no secret.
	if err := NewSingleInstance(dir).SendFilePath(`C:\docs\a.html`); err != nil {
		t.Fatalf("SendFilePath: %v", err)
	}
	select {
	case p := <-opened:
		if p != `C:\docs\a.html` {
			t.Fatalf("opened %q", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a bare file path was not opened")
	}
}

func TestInstanceSecretFileIsReadableByItsOwnerOnly(t *testing.T) {
	_, dir := windowsPrimary(t)
	path := filepath.Join(dir, instanceSecretFile)
	data, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(data))) != 64 {
		t.Fatalf("the secret file holds %d bytes, %v", len(data), err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sddl, sid := sd.String(), user.User.Sid.String()
	if !strings.HasPrefix(sddl, "D:P") || strings.Count(sddl, "(") != 1 || !strings.Contains(sddl, ";;;"+sid+")") {
		t.Fatalf("the secret file's access list is %s; want one entry, for %s, with inheritance off", sddl, sid)
	}
}

func TestWindowsCommandToAPrimaryWithNoSecretFileReadsAsTooOld(t *testing.T) {
	_, dir := windowsPrimary(t)
	if err := os.Remove(filepath.Join(dir, instanceSecretFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSingleInstance(dir).SendCommand("update-now 9.9.9"); err != ErrNoAnswer {
		t.Fatalf("SendCommand with no secret file = %v, want ErrNoAnswer", err)
	}
}

func TestMutexNameIsItsOwnForACustomConfigFolder(t *testing.T) {
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	if got := mutexNameFor(filepath.Join(base, "htmlclay")); got != mutexName {
		t.Fatalf("the default folder's mutex is %q, want the released name %q", got, mutexName)
	}
	a, b := mutexNameFor(`C:\scratch\one\htmlclay`), mutexNameFor(`C:\scratch\two\htmlclay`)
	if a == mutexName || b == mutexName || a == b {
		t.Fatalf("custom folders got %q and %q", a, b)
	}
	if mutexNameFor(`C:\Scratch\One\htmlclay\`) != a {
		t.Fatal("the same folder in another spelling got another mutex")
	}
}
