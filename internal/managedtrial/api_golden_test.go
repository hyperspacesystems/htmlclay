//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestAPIGolden replays the data API routes against answers recorded from
// Local's real server: every status, header and body of the sequence, and the
// sidecar, version and guard trees it leaves behind.
func TestAPIGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and the handler reads that checkout's bundled runtime release")
	}
	freezeAPIGuardClock(t, apiGoldenClock(t))
	// Local's data writes and dataapi's gave the same bytes for every write here,
	// so every answer and file is compared exactly. The data-judged allowance
	// (17-managed-trial.md, "native data writes store different bytes") is added
	// only where a difference actually appears.
	runGoldenCases(t, filepath.Join("testdata", "api-golden"), localSource, nil)
}

// apiGoldenClock is the instant the recorder pinned every write in this case to.
func apiGoldenClock(t *testing.T) int64 {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "api-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden serveGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if golden.Clock == 0 {
		t.Fatal("the api golden records no pinned clock")
	}
	return golden.Clock
}

// freezeAPIGuardClock is the guard's half of the recorder's pin-clock.cjs: a
// data-loss event is stamped with the pinned instant and named with the
// recorder's uuid counter, and Local minted fifteen ids of its own before this
// sequence began, so the first event here is the sixteenth.
func freezeAPIGuardClock(t *testing.T, clock int64) {
	t.Helper()
	previousNow := guardNow
	previousID := guardNewID
	ids := 15
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
