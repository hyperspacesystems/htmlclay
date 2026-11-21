//go:build windows

package carried

import "errors"

func mkfifo(string) error { return errors.New("no named pipes in the file system") }
