//go:build managedtrial

package managedtrial

import "sync"

// filePathLocks serializes work on one canonical path, inside one process-wide
// gate. Local keys its write queue the same way: every reader and writer of a
// served file that must not interleave takes the lock for the path the resolver
// produced, and a structure change takes the whole gate, so no document write
// runs while a tree moves. A holder of one path lock never takes another: Go's
// RWMutex blocks a second read lock while a structure change waits.
type filePathLocks struct {
	gate  sync.RWMutex
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// lock returns the unlock function for one canonical path, creating the lock on
// demand. It waits for any structure change to finish first.
func (l *filePathLocks) lock(path string) func() {
	l.gate.RLock()
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
	return func() {
		entry.Unlock()
		l.gate.RUnlock()
	}
}

// lockStructure is Local's withStructureChange: exclusive against every
// document write and every other structure change in the process.
func (l *filePathLocks) lockStructure() func() {
	l.gate.Lock()
	return l.gate.Unlock
}

// fileLocks is the host's one table of per-file locks, shared by every route
// that reads or writes a served file under it.
var fileLocks = &filePathLocks{}
