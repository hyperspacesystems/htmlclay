//go:build managedtrial && windows

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
	handle, err := os.Open(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(handle.Fd()), &data); err != nil {
		return nil, err
	}
	index := uint64(data.FileIndexHigh)<<32 | uint64(data.FileIndexLow)
	return &fileIdentity{
		Dev:     strconv.FormatUint(uint64(data.VolumeSerialNumber), 10),
		Ino:     strconv.FormatUint(index, 10),
		Size:    info.Size(),
		MtimeNs: strconv.FormatInt(info.ModTime().UnixNano(), 10),
	}, nil
}
