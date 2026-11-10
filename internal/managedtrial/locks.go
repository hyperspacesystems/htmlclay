//go:build managedtrial

package managedtrial

import "sync"

// filePathLocks serializes work on one canonical path. Local keys its write
// queue the same way: every reader and writer of a served file that must not
// interleave takes the lock for the path the resolver produced.
type filePathLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// lock returns the unlock function for one canonical path, creating the lock on
// demand.
func (l *filePathLocks) lock(path string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*sync.Mutex)
	}
	entry, ok := l.locks[path]
	if !ok {
		entry = &sync.Mutex{}
		l.locks[path] = entry
	}
	l.mu.Unlock()
	entry.Lock()
	return entry.Unlock
}

// fileLocks is the host's one table of per-file locks, shared by every route
// that reads or writes a served file under it.
var fileLocks = &filePathLocks{}
