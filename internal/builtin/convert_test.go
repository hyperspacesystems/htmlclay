package builtin

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, dir, name string, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestConvertWritesBesideTheOriginal(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "photo.png", 400, 300)
	result, err := Convert(dir, ConvertRequest{Path: "photo.png", To: "jpeg", MaxWidth: 200})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if result.Path != "photo.jpg" {
		t.Fatalf("Path = %q, want photo.jpg", result.Path)
	}
	if result.Width != 200 || result.Height != 150 {
		t.Fatalf("size = %dx%d, want 200x150", result.Width, result.Height)
	}
	data, err := os.ReadFile(filepath.Join(dir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the new file does not decode: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("format = %q, want jpeg", format)
	}
	if cfg.Width != 200 || cfg.Height != 150 {
		t.Fatalf("decoded size = %dx%d, want 200x150", cfg.Width, cfg.Height)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("jpeg decode: %v", err)
	}
}

func TestConvertNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	original := writePNG(t, dir, "photo.png", 40, 30)
	first, err := Convert(dir, ConvertRequest{Path: "photo.png", To: "png"})
	if err != nil {
		t.Fatalf("first Convert: %v", err)
	}
	if first.Path != "photo-2.png" {
		t.Fatalf("first Path = %q, want photo-2.png", first.Path)
	}
	second, err := Convert(dir, ConvertRequest{Path: "photo.png", To: "png"})
	if err != nil {
		t.Fatalf("second Convert: %v", err)
	}
	if second.Path != "photo-3.png" {
		t.Fatalf("second Path = %q, want photo-3.png", second.Path)
	}
	now, err := os.ReadFile(filepath.Join(dir, "photo.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(now, original) {
		t.Fatal("converting replaced the original")
	}
}

func TestConvertRefusesWhatItCannotRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", []byte("just words"))
	if _, err := Convert(dir, ConvertRequest{Path: "notes.txt", To: "png"}); codeOf(t, err) != "unsupported" {
		t.Fatalf("notes.txt = %v, want unsupported", err)
	}
	writePNG(t, dir, "photo.png", 20, 20)
	if _, err := Convert(dir, ConvertRequest{Path: "photo.png", To: "webp"}); codeOf(t, err) != "invalid_request" {
		t.Fatalf("to webp = %v, want invalid_request", err)
	}
	if _, err := Convert(dir, ConvertRequest{Path: "missing.png", To: "png"}); codeOf(t, err) != "not_found" {
		t.Fatalf("missing file = %v, want not_found", err)
	}
}
