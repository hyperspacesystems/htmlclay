//go:build windows

package platform

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ownerOnlySDDL is a protected DACL with one entry: full access for sid. No
// entry is inherited from the folder, so nobody else can read the file,
// whatever the folder allows.
func ownerOnlySDDL(sid string) string { return "D:P(A;;FA;;;" + sid + ")" }

// writeOwnerOnly writes data to a new file at path that only the current user
// can open. A security descriptor only applies when a file is created, so a
// file already there is removed first and the create fails if another one
// appears in between.
func writeOwnerOnly(path string, data []byte) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(ownerOnlySDDL(user.User.Sid.String()))
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(handle), path)
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}
