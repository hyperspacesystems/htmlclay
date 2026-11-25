//go:build darwin || linux

package platform

import (
	"errors"

	"golang.org/x/sys/unix"
)

func keepDownloadMark(from, to string) error {
	mark, err := readXattr(from, downloadMarkAttr)
	if err != nil || mark == nil {
		return err
	}
	return unix.Setxattr(to, downloadMarkAttr, mark, 0)
}

// readXattr returns the attribute's bytes, or nil when the file has none or
// its file system keeps no attributes.
func readXattr(path, name string) ([]byte, error) {
	for {
		size, err := unix.Getxattr(path, name, nil)
		if errors.Is(err, noXattr) || errors.Is(err, unix.ENOTSUP) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size)
		n, err := unix.Getxattr(path, name, buf)
		if errors.Is(err, unix.ERANGE) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
}
