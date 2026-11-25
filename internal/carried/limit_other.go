//go:build !linux

package carried

// hardLimit does nothing here: macOS enforces no data limit on mappings, and
// Windows holds the limit in the worker's job object (runner.Spec.MemoryLimit).
func hardLimit(uint64) {}
