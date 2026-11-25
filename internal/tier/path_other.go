//go:build !darwin && !windows

package tier

// foldCase keeps a path as it is where letter case names different files.
func foldCase(path string) string { return path }
