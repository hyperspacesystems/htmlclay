package gitread

import (
	"os"

	"github.com/go-git/go-billy/v5"
)

// readFS is the one filesystem go-git reads a repository through. A read
// refuses anything that could hang on a named pipe, a socket or a device, and
// every write is refused, so nothing a repository says, such as a submodule,
// writes into the repository or the person's folder.
type readFS struct {
	billy.Filesystem
}

var writeFlags = os.O_WRONLY | os.O_RDWR | os.O_CREATE | os.O_TRUNC | os.O_APPEND

func (f readFS) Open(name string) (billy.File, error) {
	if err := readable(f.Filesystem, name); err != nil {
		return nil, err
	}
	return f.Filesystem.Open(name)
}

func (f readFS) OpenFile(name string, flag int, perm os.FileMode) (billy.File, error) {
	if flag&writeFlags != 0 {
		return nil, os.ErrPermission
	}
	if err := readable(f.Filesystem, name); err != nil {
		return nil, err
	}
	return f.Filesystem.OpenFile(name, flag, perm)
}

func (f readFS) Create(name string) (billy.File, error) {
	return nil, os.ErrPermission
}

func (f readFS) MkdirAll(name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (f readFS) Remove(name string) error {
	return os.ErrPermission
}

func (f readFS) Rename(from, to string) error {
	return os.ErrPermission
}

func (f readFS) Chmod(name string, mode os.FileMode) error {
	return os.ErrPermission
}

func (f readFS) Symlink(target, link string) error {
	return os.ErrPermission
}

func (f readFS) TempFile(dir, prefix string) (billy.File, error) {
	return nil, os.ErrPermission
}

func (f readFS) Chroot(path string) (billy.Filesystem, error) {
	fs, err := f.Filesystem.Chroot(path)
	if err != nil {
		return nil, err
	}
	return readFS{fs}, nil
}

// readable refuses a name that is not a regular file, a folder, or a link to
// one of those: opening a named pipe, a socket or a device blocks or reads
// forever.
func readable(fs billy.Filesystem, name string) error {
	fi, err := fs.Lstat(name)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		if fi, err = fs.Stat(name); err != nil {
			return err
		}
	}
	if !fi.Mode().IsRegular() && !fi.IsDir() {
		return os.ErrPermission
	}
	return nil
}
