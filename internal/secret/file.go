package secret

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

type file struct{ dir string }

// NewFile answers a store that keeps one file per name in dir, readable only by
// this user, as the agent lane's token is kept. It is the fallback for a system
// with no secret store.
func NewFile(dir string) Store { return file{dir: dir} }

func (f file) path(name string) string { return filepath.Join(f.dir, name) }

// checkPrivate refuses a file that is a link or that anyone but its owner may
// read or write, so a value is never read from a place another user controls.
func checkPrivate(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("secret: %s is not a regular file", path)
	}
	// Windows reports every file as 0666; its access lists are not modes.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("secret: %s is readable by other users (mode %o)", path, info.Mode().Perm())
	}
	return nil
}

func (f file) Get(name string) (string, bool, error) {
	if err := checkName(name); err != nil {
		return "", false, err
	}
	path := f.path(name)
	if err := checkPrivate(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func (f file) Set(name, value string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return err
	}
	path := f.path(name)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("secret: %s is not a regular file", path)
	}
	tmp, err := os.CreateTemp(f.dir, "."+name+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (f file) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := os.Remove(f.path(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (f file) Kind() string { return KindFile }
