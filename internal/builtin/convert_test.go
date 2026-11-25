package builtin

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepteams/webp"
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
	if _, err := Convert(dir, ConvertRequest{Path: "photo.png", To: "avif"}); codeOf(t, err) != "invalid_request" {
		t.Fatalf("to avif = %v, want invalid_request", err)
	}
	if _, err := Convert(dir, ConvertRequest{Path: "missing.png", To: "png"}); codeOf(t, err) != "not_found" {
		t.Fatalf("missing file = %v, want not_found", err)
	}
}

// An iPhone photo converts: HEIC is read through image.Decode.
func TestConvertReadsHEIC(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "alpha.heic"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.heic"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Convert(dir, ConvertRequest{Path: "photo.heic", To: "png"})
	if err != nil {
		t.Fatalf("Convert HEIC: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, result.Path))
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil || format != "png" {
		t.Fatalf("output decodes as %q: %v", format, err)
	}
	if img.Bounds().Dx() != result.Width || img.Bounds().Dy() != result.Height || result.Width == 0 {
		t.Fatalf("output %v, result %dx%d", img.Bounds(), result.Width, result.Height)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a == 0xffff {
		t.Fatalf("the corner of an image with transparency came out opaque")
	}
}

// WebP is an output: the file is new, beside the original, and reads back as WebP.
func TestConvertWritesWebP(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "photo.png", 64, 48)
	result, err := Convert(dir, ConvertRequest{Path: "photo.png", To: "webp"})
	if err != nil {
		t.Fatalf("Convert to WebP: %v", err)
	}
	if result.Path != "photo.webp" {
		t.Fatalf("Path = %q, want photo.webp", result.Path)
	}
	out, err := os.ReadFile(filepath.Join(dir, "photo.webp"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || format != "webp" || cfg.Width != 64 || cfg.Height != 48 {
		t.Fatalf("output = %q %dx%d, %v; want webp 64x48", format, cfg.Width, cfg.Height, err)
	}
}

// A decoder that panics answers unsupported instead of ending the host.
func TestConvertSurvivesADecoderPanic(t *testing.T) {
	image.RegisterFormat("panics", "PANICS", func(io.Reader) (image.Image, error) { panic("damaged") },
		func(io.Reader) (image.Config, error) {
			return image.Config{ColorModel: color.RGBAModel, Width: 8, Height: 8}, nil
		})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.img"), []byte("PANICS and then nothing"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Convert(dir, ConvertRequest{Path: "bad.img", To: "png"})
	var coded *Error
	if !errors.As(err, &coded) || coded.Code != "unsupported" {
		t.Fatalf("Convert of an image whose decoder panics = %v, want an unsupported error", err)
	}
}

// A portrait photo comes out upright: the HEIC rotation box is applied.
func TestConvertTurnsAHEICTheWayItWasTaken(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "portrait.heic"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "portrait.heic"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Convert(dir, ConvertRequest{Path: "portrait.heic", To: "png"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if result.Width != 32 || result.Height != 64 {
		t.Fatalf("a 64x32 photo turned 90 degrees came out %dx%d, want 32x64", result.Width, result.Height)
	}
}

// A HEIC whose header claims a small picture but whose coded picture is
// larger than the pixel limit is refused, not decoded at full size.
func TestConvertRefusesAHEICLargerThanItsHeaderSays(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "oversize.heic"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.heic"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Convert(dir, ConvertRequest{Path: "small.heic", To: "png"})
	var coded *Error
	if !errors.As(err, &coded) || coded.Code != "unsupported" {
		t.Fatalf("Convert of an 8192x8192 HEIC claiming 100x100 = %v, want an unsupported error", err)
	}
}

// WebP output keeps soft transparency, such as a logo's shadow.
func TestConvertWritesWebPWithTransparency(t *testing.T) {
	dir := t.TempDir()
	src := image.NewNRGBA(image.Rect(0, 0, 256, 8))
	for x := 0; x < 256; x++ {
		for y := 0; y < 8; y++ {
			src.SetNRGBA(x, y, color.NRGBA{40, 90, 160, uint8(x)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fade.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Convert(dir, ConvertRequest{Path: "fade.png", To: "webp"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, result.Path))
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	levels := map[uint32]bool{}
	for x := 0; x < 256; x++ {
		_, _, _, a := img.At(x, 4).RGBA()
		levels[a>>8] = true
	}
	if len(levels) < 64 {
		t.Fatalf("a 256-step transparency fade kept %d levels, want at least 64", len(levels))
	}
}

// WebP holds at most 16,383 pixels a side: a wider picture is told so, not
// answered with a failed conversion.
func TestConvertToWebPRefusesASideWebPCannotHold(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "panorama.png", webp.MaxDimension+1, 2)
	_, err := Convert(dir, ConvertRequest{Path: "panorama.png", To: "webp"})
	var coded *Error
	if !errors.As(err, &coded) || coded.Code != "too_large" {
		t.Fatalf("Convert of a %d-pixel-wide picture to WebP = %v, want too_large", webp.MaxDimension+1, err)
	}
	if _, err := Convert(dir, ConvertRequest{Path: "panorama.png", To: "webp", MaxWidth: 4000}); err != nil {
		t.Fatalf("with a maximum width it converts: %v", err)
	}
}
