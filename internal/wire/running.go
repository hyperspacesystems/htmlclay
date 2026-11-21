package wire

import (
	"strings"

	"github.com/panphora/htmlclay/internal/builtin"
)

// RunningWork counts the AI edits, helper program requests and built-in
// program requests in flight. An update or port restart stops all of them, so
// the app asks before it restarts while this is above zero. The folder watcher
// is left out: it runs as long as its page is open and resumes after a restart.
func (s *Core) RunningWork() int {
	s.AIEdit.Mu.Lock()
	n := len(s.AIEdit.Running)
	s.AIEdit.Mu.Unlock()

	s.AITools.Mu.Lock()
	n += len(s.AITools.Running)
	s.AITools.Mu.Unlock()

	s.Programs.Mu.Lock()
	n += len(s.Programs.Running)
	s.Programs.Mu.Unlock()

	s.Builtin.mu.Lock()
	for key := range s.Builtin.running {
		if !strings.HasPrefix(key, builtin.FolderWatch+"\x00") {
			n++
		}
	}
	s.Builtin.mu.Unlock()

	s.HelperMu.Lock()
	dispatchers := make([]*Dispatcher, 0, len(s.Helpers))
	for _, d := range s.Helpers {
		dispatchers = append(dispatchers, d)
	}
	s.HelperMu.Unlock()
	for _, d := range dispatchers {
		d.Mu.Lock()
		n += len(d.Live)
		d.Mu.Unlock()
	}
	return n
}
