package wire

// RunningWork counts the AI edits and helper program requests in flight. An
// update restart stops all of them, so the app asks before it restarts while
// this is above zero.
func (s *Core) RunningWork() int {
	s.AIEdit.Mu.Lock()
	n := len(s.AIEdit.Running)
	s.AIEdit.Mu.Unlock()

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
