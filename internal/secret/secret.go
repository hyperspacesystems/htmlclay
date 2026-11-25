// Package secret keeps a credential, such as the sync key, out of every config
// file. A Store is the system's secret store (the login Keychain, Credential
// Manager, Secret Service), a private file where a system has none, or memory in
// tests. Names are short identifiers chosen by the program, never user input.
package secret

import (
	"errors"
	"fmt"
	"regexp"
)

// Store holds secrets by name.
type Store interface {
	Get(name string) (value string, found bool, err error)
	Set(name, value string) error
	Delete(name string) error
	// Kind is "system", "file" or "memory", so a screen can say where the key is.
	Kind() string
}

const (
	KindSystem = "system"
	KindFile   = "file"
	KindMemory = "memory"
)

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ErrBadName refuses a name outside validName, which keeps every store's item a
// plain file name and a plain Keychain account.
var ErrBadName = errors.New("secret: bad name")

func checkName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrBadName, name)
	}
	return nil
}
