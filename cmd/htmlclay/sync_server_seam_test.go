package main

import "testing"

// setSyncServer points sync at a stand-in for one test.
func setSyncServer(t *testing.T, url string) {
	t.Helper()
	previous := syncServerForTest
	syncServerForTest = url
	t.Cleanup(func() { syncServerForTest = previous })
}
