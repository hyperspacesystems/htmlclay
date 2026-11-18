package builtin

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// ConvertRequest is file-convert's payload. MaxWidth and MaxHeight shrink the
// image to fit, keeping its shape; 0 is no limit on that side.
type ConvertRequest struct {
	Path      string `json:"path"`
	To        string `json:"to"`
	MaxWidth  int    `json:"maxWidth"`
	MaxHeight int    `json:"maxHeight"`
	Quality   int    `json:"quality"`
}

// ConvertResult is the new file, relative to the program's folder.
type ConvertResult struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int64  `json:"bytes"`
}

// OutputFormats is what the converter writes, and the extension of each new file.
var OutputFormats = map[string]string{"png": ".png", "jpeg": ".jpg", "gif": ".gif", "bmp": ".bmp", "tiff": ".tiff"}

const (
	maxConvertInput  = 64 << 20
	maxConvertPixels = 50_000_000
	maxConvertSide   = 10000
	defaultQuality   = 85
	maxNameTries     = 99
)

// Convert reads a PNG, JPEG, GIF, BMP, TIFF or WebP image under root and writes
// it as req.To beside the original, under a name no file has. It never
// replaces a file.
func Convert(root string, req ConvertRequest) (ConvertResult, error) {
	ext, ok := OutputFormats[req.To]
	if !ok {
		return ConvertResult{}, invalid("to")
	}
	if req.MaxWidth < 0 || req.MaxWidth > maxConvertSide {
		return ConvertResult{}, invalid("maxWidth")
	}
	if req.MaxHeight < 0 || req.MaxHeight > maxConvertSide {
		return ConvertResult{}, invalid("maxHeight")
	}
	quality := req.Quality
	if quality == 0 {
		quality = defaultQuality
	}
	if quality < 1 || quality > 100 {
		return ConvertResult{}, invalid("quality")
	}
	rel, perr := relPath(req.Path)
	if perr != nil || rel == "." {
		return ConvertResult{}, invalid("path")
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return ConvertResult{}, failed(err)
	}
	defer r.Close()
	info, err := r.Stat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return ConvertResult{}, &Error{Code: "not_found", Message: "That image does not exist."}
	}
	if err != nil {
		return ConvertResult{}, failed(err)
	}
	if !info.Mode().IsRegular() {
		return ConvertResult{}, invalid("path")
	}
	if info.Size() > maxConvertInput {
		return ConvertResult{}, &Error{Code: "too_large", Message: "The image is larger than 64 MB."}
	}
	f, err := r.Open(rel)
	if err != nil {
		return ConvertResult{}, failed(err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConvertInput+1))
	f.Close()
	if err != nil {
		return ConvertResult{}, failed(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ConvertResult{}, &Error{Code: "unsupported", Message: "HTML Clay can read PNG, JPEG, GIF, BMP, TIFF and WebP images."}
	}
	if cfg.Width*cfg.Height > maxConvertPixels {
		return ConvertResult{}, &Error{Code: "too_large", Message: "The image has more than 50 million pixels."}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ConvertResult{}, &Error{Code: "unsupported", Message: "The image could not be read. Check that the file is not damaged."}
	}
	img = fit(img, req.MaxWidth, req.MaxHeight)
	var out bytes.Buffer
	if err := encode(&out, img, req.To, quality); err != nil {
		return ConvertResult{}, failed(err)
	}
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	name, err := createFree(r, filepath.Dir(rel), base, ext, out.Bytes())
	if err != nil {
		var coded *Error
		if errors.As(err, &coded) {
			return ConvertResult{}, coded
		}
		return ConvertResult{}, failed(err)
	}
	b := img.Bounds()
	return ConvertResult{Path: filepath.ToSlash(name), Width: b.Dx(), Height: b.Dy(), Bytes: int64(out.Len())}, nil
}

// fit shrinks img to fit maxW by maxH, keeping its shape. A zero side is not a
// limit, and an image that already fits is returned as it is. It never enlarges.
func fit(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := 1.0
	if maxW > 0 && w > maxW {
		scale = math.Min(scale, float64(maxW)/float64(w))
	}
	if maxH > 0 && h > maxH {
		scale = math.Min(scale, float64(maxH)/float64(h))
	}
	if scale == 1.0 {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func encode(w io.Writer, img image.Image, to string, quality int) error {
	switch to {
	case "png":
		return png.Encode(w, img)
	case "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "gif":
		return gif.Encode(w, img, nil)
	case "bmp":
		return bmp.Encode(w, img)
	case "tiff":
		return tiff.Encode(w, img, &tiff.Options{Compression: tiff.Deflate})
	}
	return fmt.Errorf("unknown format %q", to)
}

// createFree writes data to dir/base+ext, or base-2+ext and so on up to
// base-99+ext, with O_EXCL so it never replaces a file. It returns the path
// relative to the root.
func createFree(r *os.Root, dir, base, ext string, data []byte) (string, error) {
	for n := 1; n <= maxNameTries; n++ {
		name := base + ext
		if n > 1 {
			name = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		p := filepath.Join(dir, name)
		f, err := r.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			_ = r.Remove(p)
			return "", errors.Join(werr, cerr)
		}
		return p, nil
	}
	return "", &Error{Code: "exists", Message: "Every file name for the new image is already taken."}
}
