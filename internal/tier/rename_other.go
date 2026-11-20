//go:build !windows

package tier

import "os"

// renameWithRetry publishes a record. Posix renames over a file another
// process holds open, so there is nothing to wait for.
func renameWithRetry(from, to string) error { return os.Rename(from, to) }
