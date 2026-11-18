package platform

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

type SingleInstance interface {
	TryLock() (bool, error)
	SendFilePath(path string) error
	OnFileReceived(callback func(path string))
	// SendCommand sends cmd to the primary instance as a command frame and
	// returns its one-line answer. ErrNoAnswer means the primary read the frame
	// and closed without answering, which is what a build from before command
	// frames does.
	SendCommand(cmd string) (string, error)
	// OnCommand sets the handler for command frames. Its answer is sent back as
	// one line. Until it is set every command is answered CommandStarting.
	OnCommand(handler func(cmd string) string)
	Unlock() error
}

// handshakeBanner is sent by the primary instance to every connecting peer.
// A forwarding instance verifies it before writing a file path, so a stale
// addr/socket pointing at an unrelated local listener never receives the path.
const handshakeBanner = "HTMLCLAY-SINGLE-INSTANCE-1\n"

func writeBanner(c net.Conn) {
	c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	c.Write([]byte(handshakeBanner))
}

func verifyBanner(c net.Conn) bool {
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, len(handshakeBanner))
	if _, err := io.ReadFull(c, buf); err != nil {
		return false
	}
	return string(buf) == handshakeBanner
}

// A command frame is a NUL byte, the command and a newline. A file path never
// contains NUL, so a frame can never be taken for one. A build from before
// command frames reads until the peer closes, and SendCommand never closes
// first, so such a build times out and drops the frame instead of opening it.
const (
	commandPrefix = 0
	maxCommand    = 4096
	maxAnswer     = 1024
)

// CommandStarting is the answer to every command until OnCommand is set. It
// means the app is still starting and the command can be sent again.
const CommandStarting = "starting"

// ErrNoAnswer means the primary instance closed the connection without
// answering a command.
var ErrNoAnswer = errors.New("the running instance did not answer the command")

// commandAnswerTimeout is how long SendCommand waits for the answer. It is
// longer than the 5 s an older primary waits before it drops the frame, so
// that case reads as ErrNoAnswer rather than as a timeout.
var commandAnswerTimeout = 10 * time.Second

// isCommandFrame reports whether the connection's first byte starts a command
// frame. It waits as long as the read deadline allows.
func isCommandFrame(r *bufio.Reader) bool {
	b, err := r.Peek(1)
	return err == nil && b[0] == commandPrefix
}

// readCommand reads one command frame: the NUL, at most maxCommand printable
// ASCII bytes, and the newline.
func readCommand(r *bufio.Reader) (string, bool) {
	if b, err := r.ReadByte(); err != nil || b != commandPrefix {
		return "", false
	}
	var cmd []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", false
		}
		if b == '\n' {
			return string(cmd), len(cmd) > 0
		}
		if b < 0x20 || b > 0x7e || len(cmd) == maxCommand {
			return "", false
		}
		cmd = append(cmd, b)
	}
}

// answerCommand reads one command frame from c and writes back one line.
// refusal, when set, is the answer to every command on this connection.
func answerCommand(c net.Conn, r *bufio.Reader, refusal string, handler func(string) string) {
	cmd, ok := readCommand(r)
	var answer string
	switch {
	case !ok:
		answer = "refused malformed command"
	case refusal != "":
		answer = "refused " + refusal
	case handler == nil:
		answer = CommandStarting
	default:
		answer = handler(cmd)
	}
	answer = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, answer)
	c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	io.WriteString(c, answer+"\n")
}

// exchangeCommand sends cmd on a connection whose banner was verified and
// reads the one-line answer.
func exchangeCommand(c net.Conn, cmd string) (string, error) {
	if cmd == "" || len(cmd) > maxCommand || strings.ContainsAny(cmd, "\x00\n") {
		return "", errors.New("a command is one line of at most 4096 bytes")
	}
	c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(c, "\x00"+cmd+"\n"); err != nil {
		return "", err
	}
	c.SetReadDeadline(time.Now().Add(commandAnswerTimeout))
	line, err := bufio.NewReader(io.LimitReader(c, maxAnswer)).ReadString('\n')
	if err != nil {
		if line == "" && errors.Is(err, io.EOF) {
			return "", ErrNoAnswer
		}
		return "", err
	}
	return strings.TrimSuffix(line, "\n"), nil
}

// On Windows the channel is a loopback TCP port, which every local user can
// reach, so a command frame there also carries the instance secret. The primary
// writes the secret to a file only its own user can read; a client that can
// read the file is that user. The sealed form is "secret <hex> <command>". A
// file path frame carries no secret and is unchanged, so a 1.x second instance
// still hands its file to a 2.0 primary.
const sealedPrefix = "secret "

// instanceSecretFile is the secret's file name in the config folder.
const instanceSecretFile = "instance-secret"

// newInstanceSecret returns 32 random bytes as hex.
func newInstanceSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// sealCommand is the command frame's text for a channel that needs the secret.
func sealCommand(secret, cmd string) string { return sealedPrefix + secret + " " + cmd }

// sealedHandler wraps handler so that it only sees commands sealed with
// secret. Every other frame is refused without reaching it. A nil handler
// answers CommandStarting, as on the other platforms.
func sealedHandler(secret string, handler func(string) string) func(string) string {
	return func(frame string) string {
		rest, sealed := strings.CutPrefix(frame, sealedPrefix)
		got, cmd, found := strings.Cut(rest, " ")
		if !sealed || !found || secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			return "refused the command did not carry this instance's secret"
		}
		if handler == nil {
			return CommandStarting
		}
		return handler(cmd)
	}
}
