package platform

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestClipboardCommandPicksTheToolForEachSystem(t *testing.T) {
	missing := func(string) (string, error) { return "", errors.New("not found") }
	found := func(name string) (string, error) { return "/usr/bin/" + name, nil }

	for _, c := range []struct {
		name    string
		goos    string
		look    func(string) (string, error)
		want    []string
		wantErr bool
	}{
		{"macOS", "darwin", missing, []string{"pbcopy"}, false},
		{"Windows", "windows", missing, []string{"clip.exe"}, false},
		{"Linux wl-copy", "linux", found, []string{"wl-copy"}, false},
		{"Linux xclip", "linux", func(name string) (string, error) {
			if name == "wl-copy" {
				return "", errors.New("not found")
			}
			return found(name)
		}, []string{"xclip", "-selection", "clipboard"}, false},
		{"Linux xsel", "linux", func(name string) (string, error) {
			if name == "xsel" {
				return found(name)
			}
			return "", errors.New("not found")
		}, []string{"xsel", "--clipboard", "--input"}, false},
		{"Linux with none installed", "linux", missing, nil, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := clipboardCommand(c.goos, c.look)
			if c.wantErr {
				if err == nil {
					t.Fatalf("clipboardCommand(%s) = %q, want an error", c.goos, got)
				}
				if want := "no clipboard tool found (install wl-clipboard or xclip)"; err.Error() != want {
					t.Fatalf("error = %q, want %q", err.Error(), want)
				}
				return
			}
			if err != nil {
				t.Fatalf("clipboardCommand(%s): %v", c.goos, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("clipboardCommand(%s) = %q, want %q", c.goos, got, c.want)
			}
		})
	}
}

// clip.exe reads the console encoding, so the text must arrive as UTF-16LE after
// a byte-order mark or an accented name or an em dash reaches the clipboard
// mangled.
func TestClipboardInputIsUTF16WithABOMOnWindows(t *testing.T) {
	const text = "Anna’s notes — 1"
	got := clipboardInput("windows", text)
	if len(got) < 2 || got[0] != 0xff || got[1] != 0xfe {
		t.Fatalf("windows input starts with %x, want a UTF-16LE byte-order mark", got)
	}
	units := make([]uint16, 0, (len(got)-2)/2)
	for i := 2; i+1 < len(got); i += 2 {
		units = append(units, binary.LittleEndian.Uint16(got[i:]))
	}
	if back := string(utf16.Decode(units)); back != text {
		t.Fatalf("windows input decodes to %q, want %q", back, text)
	}
	if plain := clipboardInput("darwin", text); !bytes.Equal(plain, []byte(text)) {
		t.Fatalf("macOS input = %q, want the text unchanged", plain)
	}
}
