//go:build windows

package platform

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const mutexName = `Local\htmlclay-single-instance`

// mutexNameFor is the named mutex for a config folder. The default folder
// keeps the name every released version uses, so 1.x and 2.0 exclude each
// other and an update restart hands over cleanly. Any other folder (a test, a
// seeded profile under HTMLCLAY_CONFIG_BASE) gets a name of its own, so it
// neither blocks nor is blocked by the person's real HTML Clay.
func mutexNameFor(configDir string) string {
	dir := filepath.Clean(configDir)
	if base, err := os.UserConfigDir(); err == nil && strings.EqualFold(dir, filepath.Join(base, "htmlclay")) {
		return mutexName
	}
	sum := sha256.Sum256([]byte(strings.ToLower(dir)))
	return mutexName + "-" + hex.EncodeToString(sum[:8])
}

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

type tcpSingleInstance struct {
	configDir   string
	addrFile    string
	secretFile  string
	listener    net.Listener
	mutexHandle windows.Handle
	mu          sync.Mutex
	callback    func(string)
	pending     []string
	// secret is what a command frame must carry. It is empty when the secret
	// file could not be written, and then no command is taken.
	secret  string
	handler func(string) string
}

func NewSingleInstance(configDir string) SingleInstance {
	return &tcpSingleInstance{
		configDir:  configDir,
		addrFile:   filepath.Join(configDir, "addr"),
		secretFile: filepath.Join(configDir, instanceSecretFile),
	}
}

func (s *tcpSingleInstance) TryLock() (bool, error) {
	// A named mutex gives true mutual exclusion: even two simultaneous launches
	// cannot both become primary, unlike the addr-file check-then-listen below.
	name, err := windows.UTF16PtrFromString(mutexNameFor(s.configDir))
	if err != nil {
		return false, fmt.Errorf("cannot encode mutex name: %w", err)
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if handle != 0 {
		s.mutexHandle = handle
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		return false, nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false, fmt.Errorf("cannot create listener: %w", err)
	}
	s.listener = listener

	if err := os.WriteFile(s.addrFile, []byte(listener.Addr().String()), 0644); err != nil {
		listener.Close()
		return false, fmt.Errorf("cannot write addr file: %w", err)
	}
	// A failure here leaves the app running and opening files; it only takes no
	// commands, and says why to whoever sends one.
	if secret, err := newInstanceSecret(); err == nil && writeOwnerOnly(s.secretFile, []byte(secret)) == nil {
		s.secret = secret
	}

	go s.acceptLoop()
	return true, nil
}

func (s *tcpSingleInstance) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConnection(conn)
	}
}

func (s *tcpSingleInstance) handleConnection(c net.Conn) {
	defer c.Close()
	writeBanner(c)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(io.LimitReader(c, 64<<10))
	// Every local user can reach this loopback listener, so a command must
	// carry the instance secret, which only this user can read. Answering every
	// frame keeps one from being opened as a path.
	if isCommandFrame(r) {
		s.mu.Lock()
		handler := s.handler
		s.mu.Unlock()
		if s.secret == "" {
			answerCommand(c, r, "this HTML Clay could not write its instance secret, so it takes no commands", nil)
			return
		}
		answerCommand(c, r, "", sealedHandler(s.secret, handler))
		return
	}
	data, err := io.ReadAll(r)
	if err != nil || len(data) == 0 {
		return
	}
	path := string(data)

	s.mu.Lock()
	if s.callback != nil {
		cb := s.callback
		s.mu.Unlock()
		cb(path)
	} else {
		s.pending = append(s.pending, path)
		s.mu.Unlock()
	}
}

func (s *tcpSingleInstance) SendFilePath(path string) error {
	data, err := os.ReadFile(s.addrFile)
	if err != nil {
		return fmt.Errorf("cannot read addr file: %w", err)
	}
	addr := strings.TrimSpace(string(data))
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if !verifyBanner(conn) {
		return fmt.Errorf("peer on %s is not an htmlclay instance", addr)
	}
	// Explorer lets this launch take the foreground, and the running instance
	// cannot; pass the right on, or its dialogs for this file open behind
	// every window. ASFW_ANY is (DWORD)-1.
	procAllowSetForegroundWindow.Call(uintptr(^uint32(0)))
	_, err = conn.Write([]byte(path))
	return err
}

func (s *tcpSingleInstance) SendCommand(cmd string) (string, error) {
	data, err := os.ReadFile(s.addrFile)
	if err != nil {
		return "", fmt.Errorf("cannot read addr file: %w", err)
	}
	// A running HTML Clay from before 2.0 writes no secret and takes no commands.
	secret, err := os.ReadFile(s.secretFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoAnswer
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the instance secret: %w", err)
	}
	addr := strings.TrimSpace(string(data))
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if !verifyBanner(conn) {
		return "", fmt.Errorf("peer on %s is not an htmlclay instance", addr)
	}
	return exchangeCommand(conn, sealCommand(strings.TrimSpace(string(secret)), cmd))
}

func (s *tcpSingleInstance) OnCommand(handler func(string) string) {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
}

func (s *tcpSingleInstance) OnFileReceived(callback func(string)) {
	s.mu.Lock()
	s.callback = callback
	queued := s.pending
	s.pending = nil
	s.mu.Unlock()

	for _, path := range queued {
		callback(path)
	}
}

func (s *tcpSingleInstance) Unlock() error {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.addrFile)
	if s.secret != "" {
		os.Remove(s.secretFile)
	}
	if s.mutexHandle != 0 {
		windows.CloseHandle(s.mutexHandle)
		s.mutexHandle = 0
	}
	return nil
}
