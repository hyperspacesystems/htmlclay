package secret

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// credman keeps items in the Windows Credential Manager as generic credentials,
// one per name, under a target that carries the profile, so two profiles never
// share a key.
type credman struct{ prefix string }

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW = advapi32.NewProc("CredWriteW")
	procCredReadW  = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	// credMaxBlob is CRED_MAX_CREDENTIAL_BLOB_SIZE.
	credMaxBlob = 5 * 512
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func newCredman(profile string) *credman {
	return &credman{prefix: "htmlclay/sync/" + profile + "/"}
}

func (c *credman) target(name string) (*uint16, error) {
	return windows.UTF16PtrFromString(c.prefix + name)
}

func (c *credman) Get(name string) (string, bool, error) {
	if err := checkName(name); err != nil {
		return "", false, err
	}
	target, err := c.target(name)
	if err != nil {
		return "", false, err
	}
	var cred *credential
	r, _, callErr := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("secret: CredReadW: %w", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	if cred.CredentialBlobSize == 0 {
		return "", true, nil
	}
	value := string(unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize))
	return value, true, nil
}

func (c *credman) Set(name, value string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if len(value) == 0 || len(value) > credMaxBlob {
		return errors.New("secret: a credential must be 1 to 2560 bytes")
	}
	target, err := c.target(name)
	if err != nil {
		return err
	}
	blob := []byte(value)
	cred := credential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
	}
	r, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if r == 0 {
		return fmt.Errorf("secret: CredWriteW: %w", callErr)
	}
	return nil
}

func (c *credman) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	target, err := c.target(name)
	if err != nil {
		return err
	}
	r, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	if r == 0 && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
		return fmt.Errorf("secret: CredDeleteW: %w", callErr)
	}
	return nil
}

func (c *credman) Kind() string { return KindSystem }
