package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// completeMarker is written last into an extracted copy; a folder without it
// is an interrupted extraction and is never run.
const completeMarker = ".complete"

var installMu sync.Mutex

// Bundled reports whether this build carries the service program (the
// service_bundle tag). A release build runs only the bundled program.
func Bundled() bool { return len(bundled) > 0 }

// Install extracts the bundled service under root, once per checksum, and
// answers the command that runs it: the bundled Node binary and the service's
// entry script. Other extracted versions under root are removed.
func Install(root string) ([]string, error) {
	if !Bundled() {
		return nil, errors.New("service: this build carries no service program")
	}
	sum, size, err := parseSum(bundledSum)
	if err != nil {
		return nil, err
	}
	return install(root, bundled, sum, size)
}

func parseSum(s string) (string, int64, error) {
	f := strings.Fields(s)
	if len(f) != 2 || len(f[0]) != 64 {
		return "", 0, errors.New("service: malformed bundled checksum")
	}
	size, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil || size <= 0 {
		return "", 0, errors.New("service: malformed bundled size")
	}
	return f[0], size, nil
}

func nodeName() string {
	if runtime.GOOS == "windows" {
		return "node.exe"
	}
	return "node"
}

func commandIn(dir string) []string {
	return []string{filepath.Join(dir, nodeName()), filepath.Join(dir, "app", "main.cjs")}
}

func install(root string, blob []byte, sum string, size int64) ([]string, error) {
	installMu.Lock()
	defer installMu.Unlock()

	dir := filepath.Join(root, sum[:16])
	if complete(dir, sum) {
		prune(root, dir)
		return commandIn(dir), nil
	}
	got := sha256.Sum256(blob)
	if int64(len(blob)) != size || hex.EncodeToString(got[:]) != sum {
		return nil, errors.New("service: bundled program does not match its checksum")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	var suffix [6]byte
	rand.Read(suffix[:])
	tmp := filepath.Join(root, ".extract-"+hex.EncodeToString(suffix[:]))
	defer os.RemoveAll(tmp)
	if err := untar(tmp, blob); err != nil {
		return nil, fmt.Errorf("service: extracting the bundled program: %w", err)
	}
	if _, err := os.Stat(commandIn(tmp)[0]); err != nil {
		return nil, fmt.Errorf("service: the bundled program has no %s", nodeName())
	}
	if _, err := os.Stat(commandIn(tmp)[1]); err != nil {
		return nil, errors.New("service: the bundled program has no app/main.cjs")
	}
	if err := os.WriteFile(filepath.Join(tmp, completeMarker), []byte(sum+"\n"), 0o600); err != nil {
		return nil, err
	}
	os.RemoveAll(dir)
	if err := renameRetry(tmp, dir); err != nil {
		if !complete(dir, sum) {
			return nil, err
		}
	}
	prune(root, dir)
	return commandIn(dir), nil
}

func complete(dir, sum string) bool {
	b, err := os.ReadFile(filepath.Join(dir, completeMarker))
	if err != nil || strings.TrimSpace(string(b)) != sum {
		return false
	}
	_, err = os.Stat(commandIn(dir)[0])
	return err == nil
}

// untar writes the archive's directories and regular files under dest. Any
// other entry, and any name that is absolute or leaves dest, fails it.
func untar(dest string, blob []byte) error {
	zr, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported entry %q (type %c)", h.Name, h.Typeflag)
		}
	}
}

// Windows refuses a rename while a scanner holds a file; retry briefly.
func renameRetry(from, to string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(from, to); err == nil || runtime.GOOS != "windows" {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return err
}

// prune removes other extracted versions and extractions an interrupted run
// left behind more than ten minutes ago (a fresh one may belong to another
// HTML Clay extracting now).
func prune(root, keep string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	old := time.Now().Add(-10 * time.Minute)
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		if !e.IsDir() || p == keep {
			continue
		}
		if strings.HasPrefix(e.Name(), ".extract-") {
			if info, err := e.Info(); err != nil || info.ModTime().After(old) {
				continue
			}
		}
		os.RemoveAll(p)
	}
}
