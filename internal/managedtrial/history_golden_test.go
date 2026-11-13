//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestHistoryGolden replays the data-loss and version-history routes against
// answers recorded from Local's real server: every status, header and body of the
// sequence, and the guard and version trees it leaves behind.
func TestHistoryGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and the handler reads that checkout's bundled runtime release")
	}
	freezeHistoryGuardClock(t, historyGoldenClock(t))
	// The guard Restore writes Local's data in dataapi's bytes (17-managed-trial.md,
	// "native data writes store different bytes"): the restored document, the
	// version its write-back makes, and the recover copy the next destructive save
	// takes of it are judged by the data they hold.
	runGoldenCasesWith(t, filepath.Join("testdata", "history-golden"), localSource, goldenOptions{
		dataJudgedCases: map[string]bool{"document-restored": true},
		dataJudgedFiles: map[string]string{
			".hyperclay/versions/notes/2026-11-01-01-00-00-000-0500-004.html": "document-restored",
			".hyperclay/guard/notes.recover.html":                             "document-restored",
		},
	})
}

// historyGoldenClock is the instant the recorder pinned every write in this case
// to. A data-loss guard event is stamped with it and named with the recorder's
// uuid counter, so both sequences are replayed here.
func historyGoldenClock(t *testing.T) int64 {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "history-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden serveGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if golden.Clock == 0 {
		t.Fatal("the history golden records no pinned clock")
	}
	return golden.Clock
}

// freezeHistoryGuardClock is the guard's half of the recorder's pin-clock.cjs:
// Date.now is the pinned instant, and crypto.randomUUID is a counter in the last
// group, which is where an event id comes from. The recorder's first id went to
// what Local mints for itself before it answers anything, so the first event the
// sequence sees is the second id.
func freezeHistoryGuardClock(t *testing.T, clock int64) {
	t.Helper()
	previousNow := guardNow
	previousID := guardNewID
	ids := 1
	guardNow = func() int64 { return clock }
	guardNewID = func() string {
		value := fmt.Sprintf("00000000-0000-4000-8000-%012d", ids)
		ids++
		return value
	}
	t.Cleanup(func() {
		guardNow = previousNow
		guardNewID = previousID
	})
}
