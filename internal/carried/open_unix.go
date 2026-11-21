//go:build !windows

package carried

import "syscall"

// openNonBlock keeps a read from waiting forever on a named pipe that took a
// file's place after it was checked.
const openNonBlock = syscall.O_NONBLOCK
