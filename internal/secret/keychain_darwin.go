package secret

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// keychain keeps items in the login Keychain through /usr/bin/security. Every
// command, the value included, goes to the tool's stdin (its -i mode), so a
// value never appears in a process's arguments. The item belongs to the system
// tool, which never changes, so a rebuilt HTML Clay is not asked again.
type keychain struct {
	service string
	tool    string
}

const securityTool = "/usr/bin/security"

// errNotFound is security's answer for a missing item: errSecItemNotFound.
const errNotFound = "-25300"

// A value goes on a command line the tool splits on spaces, so it may not hold
// spaces, quotes or a backslash. A sync key is hcsk_ and 64 hex characters.
var validValue = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]{1,1000}$`)

var errBadValue = errors.New("secret: value has characters the Keychain line cannot carry")

func newKeychain(service string) *keychain {
	return &keychain{service: service, tool: securityTool}
}

func (k *keychain) command(line string) *exec.Cmd {
	cmd := exec.Command(k.tool, "-i")
	cmd.Stdin = strings.NewReader(line + "\n")
	return cmd
}

func (k *keychain) run(line string) (stdout string, notFound bool, err error) {
	cmd := k.command(line)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	runErr := cmd.Run()
	if strings.Contains(errOut.String(), errNotFound) {
		return "", true, nil
	}
	if runErr != nil {
		return "", false, fmt.Errorf("secret: keychain: %v: %s", runErr, strings.TrimSpace(errOut.String()))
	}
	return out.String(), false, nil
}

func (k *keychain) Get(name string) (string, bool, error) {
	if err := checkName(name); err != nil {
		return "", false, err
	}
	out, notFound, err := k.run(fmt.Sprintf("find-generic-password -s %s -a %s -w", k.service, name))
	if err != nil || notFound {
		return "", false, err
	}
	return strings.TrimRight(out, "\r\n"), true, nil
}

func (k *keychain) Set(name, value string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if !validValue.MatchString(value) {
		return errBadValue
	}
	_, _, err := k.run(fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s", k.service, name, value))
	return err
}

func (k *keychain) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	_, _, err := k.run(fmt.Sprintf("delete-generic-password -s %s -a %s", k.service, name))
	return err
}

func (k *keychain) Kind() string { return KindSystem }
