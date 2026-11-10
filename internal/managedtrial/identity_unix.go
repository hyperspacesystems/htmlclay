//go:build managedtrial && !windows

package managedtrial

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"syscall"
)

// fileIdentity is the identity Local compares before it replaces a document:
// device, inode, size and modification time, as Node's bigint stat reports them.
type fileIdentity struct {
	Dev     string
	Ino     string
	Size    int64
	MtimeNs string
}

// identityOfPath is the identity of the file at a path, or nil when there is none.
func identityOfPath(filePath string) (*fileIdentity, error) {
	info, err := os.Stat(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("no device and inode for " + filePath)
	}
	return &fileIdentity{
		Dev:     strconv.FormatUint(uint64(stat.Dev), 10),
		Ino:     strconv.FormatUint(uint64(stat.Ino), 10),
		Size:    info.Size(),
		MtimeNs: strconv.FormatInt(info.ModTime().UnixNano(), 10),
	}, nil
}
