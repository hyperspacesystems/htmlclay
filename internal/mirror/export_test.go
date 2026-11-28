package mirror

import "time"

// SetFetchIdleForTest lowers fetchIdle for one test and returns a function that puts it
// back.
func SetFetchIdleForTest(d time.Duration) func() {
	old := fetchIdle
	fetchIdle = d
	return func() { fetchIdle = old }
}
