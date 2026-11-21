//go:build linux

package carried

import "golang.org/x/sys/unix"

// hardLimit caps the worker's data segment, which holds every heap mapping
// Go makes, so an allocation past it fails in the kernel. Address space is
// not capped: Go reserves far more of it than it uses.
func hardLimit(bytes uint64) {
	_ = unix.Setrlimit(unix.RLIMIT_DATA, &unix.Rlimit{Cur: bytes, Max: bytes})
}
