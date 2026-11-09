package dataguard

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Two saves of one document must be judged in the order they happened, whatever
// the interleaving of the two keys.
func TestQueueKeepsOrderPerKey(t *testing.T) {
	const jobs = 1000
	keys := []string{"board", "notes"}

	var q Queue
	var mu sync.Mutex
	seen := map[string][]int{}

	for i := 0; i < jobs; i++ {
		key, n := keys[i%len(keys)], i
		q.Do(key, func() {
			mu.Lock()
			seen[key] = append(seen[key], n)
			mu.Unlock()
		})
	}
	q.Settle()

	if len(seen) != len(keys) {
		t.Fatalf("got %d keys, want %d", len(seen), len(keys))
	}
	for ki, key := range keys {
		got := seen[key]
		if len(got) != jobs/len(keys) {
			t.Fatalf("%s: got %d jobs, want %d", key, len(got), jobs/len(keys))
		}
		for i, n := range got {
			if want := i*len(keys) + ki; n != want {
				t.Fatalf("%s: job %d ran out of order at position %d, want job %d", key, n, i, want)
			}
		}
	}
}

// Settle must not return until the last job has finished.
func TestQueueSettleWaitsForLastJob(t *testing.T) {
	var q Queue
	var finished atomic.Bool

	q.Do("board", func() {
		time.Sleep(20 * time.Millisecond)
	})
	q.Do("board", func() {
		time.Sleep(20 * time.Millisecond)
		finished.Store(true)
	})
	q.Do("notes", func() {
		time.Sleep(20 * time.Millisecond)
	})

	start := time.Now()
	q.Settle()
	elapsed := time.Since(start)

	if !finished.Load() {
		t.Fatal("Settle returned before the last job finished")
	}
	if elapsed < 20*time.Millisecond {
		t.Fatalf("Settle returned after %v, before the queued work could run", elapsed)
	}
}

// A panicking guard must not stop the saves queued behind it or crash the process.
func TestQueueRecoversPanicAndKeepsDraining(t *testing.T) {
	const jobs = 10

	var q Queue
	var ran atomic.Int32

	q.Do("board", func() {
		panic("guard exploded")
	})
	for i := 0; i < jobs; i++ {
		q.Do("board", func() {
			ran.Add(1)
		})
	}
	q.Settle()

	if got := ran.Load(); got != jobs {
		t.Fatalf("got %d later jobs, want %d", got, jobs)
	}
}
