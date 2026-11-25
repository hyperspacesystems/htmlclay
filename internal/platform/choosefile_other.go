//go:build !windows && !linux

package platform

import "errors"

func chooseFile(title string) (string, bool, error) {
	return "", false, errors.ErrUnsupported
}
