package platform

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os/exec"
	"runtime"
	"unicode/utf16"
)

// CopyText puts text on the clipboard through a tool the platform already has.
// The error is returned so the caller can log why nothing was copied, which is
// the only trace a failure leaves: no window is open.
func CopyText(text string) error {
	argv, err := clipboardCommand(runtime.GOOS, exec.LookPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(clipboardInput(runtime.GOOS, text))
	return cmd.Run()
}

// clipboardCommand is the program CopyText runs, and its arguments. Linux has no
// one clipboard tool, so the first of the usual three that is installed wins; the
// answer is an error when none is, which is the plain "nothing here can copy"
// a person can act on.
func clipboardCommand(goos string, lookPath func(string) (string, error)) ([]string, error) {
	switch goos {
	case "darwin":
		return []string{"pbcopy"}, nil
	case "windows":
		return []string{"clip.exe"}, nil
	}
	for _, argv := range [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	} {
		if _, err := lookPath(argv[0]); err == nil {
			return argv, nil
		}
	}
	return nil, errors.New("no clipboard tool found (install wl-clipboard or xclip)")
}

// clipboardInput is the bytes the tool reads from stdin. clip.exe reads the
// console encoding, so Windows gets UTF-16LE with a byte-order mark and every
// other platform gets the text as it is.
func clipboardInput(goos, text string) []byte {
	if goos != "windows" {
		return []byte(text)
	}
	units := utf16.Encode([]rune(text))
	buf := make([]byte, 2+2*len(units))
	buf[0], buf[1] = 0xff, 0xfe
	for i, unit := range units {
		binary.LittleEndian.PutUint16(buf[2+2*i:], unit)
	}
	return buf
}
