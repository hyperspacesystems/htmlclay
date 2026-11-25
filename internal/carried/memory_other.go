//go:build (darwin && !cgo) || !(darwin || linux || windows)

package carried

// memoryWatched is false where HTML Clay cannot read another process's
// memory, and Run then refuses to start a program.
const memoryWatched = false

func memoryOf(int) (uint64, bool) { return 0, false }
