package builtin

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// nextBatch waits for one batch of changes, failing the test after ten seconds:
// the watch looks every 20 ms, so the limit only guards against a lost batch, and
// a loaded test box can stall a scan for seconds.
func nextBatch(t *testing.T, batches <-chan []Change) []Change {
	t.Helper()
	select {
	case batch := <-batches:
		return batch
	case <-time.After(10 * time.Second):
		t.Fatal("no batch of changes arrived within ten seconds")
		return nil
	}
}

func TestWatchReportsAddedChangedRemoved(t *testing.T) {
	dir := t.TempDir()
	batches := make(chan []Change, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = Watch(ctx, dir, "adds", WatchRequest{}, 20*time.Millisecond, func(changes []Change) {
			batches <- changes
		})
	}()
	time.Sleep(100 * time.Millisecond)

	path := writeFile(t, dir, "a.txt", []byte("one"))
	if got, want := nextBatch(t, batches), []Change{{Path: "a.txt", Change: "added"}}; !slices.Equal(got, want) {
		t.Fatalf("first batch = %+v, want %+v", got, want)
	}

	if err := os.WriteFile(path, []byte("two, longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if got, want := nextBatch(t, batches), []Change{{Path: "a.txt", Change: "changed"}}; !slices.Equal(got, want) {
		t.Fatalf("second batch = %+v, want %+v", got, want)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got, want := nextBatch(t, batches), []Change{{Path: "a.txt", Change: "removed"}}; !slices.Equal(got, want) {
		t.Fatalf("third batch = %+v, want %+v", got, want)
	}
}

func TestWatchResumesFromItsLastLook(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = Watch(ctx, dir, "resume", WatchRequest{}, 20*time.Millisecond, func([]Change) {})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	writeFile(t, dir, "b.txt", []byte("new"))

	batches := make(chan []Change, 16)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go func() {
		_ = Watch(ctx2, dir, "resume", WatchRequest{}, 20*time.Millisecond, func(changes []Change) {
			batches <- changes
		})
	}()
	if got, want := nextBatch(t, batches), []Change{{Path: "b.txt", Change: "added"}}; !slices.Equal(got, want) {
		t.Fatalf("first batch = %+v, want %+v", got, want)
	}
}

func TestWatchRefusesAHugeFolder(t *testing.T) {
	dir := t.TempDir()
	for i := range 10001 {
		name := filepath.Join(dir, "f"+itoa(i))
		f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	reported := false
	err := Watch(context.Background(), dir, "huge", WatchRequest{}, 20*time.Millisecond, func([]Change) {
		reported = true
	})
	if got := codeOf(t, err); got != "too_large" {
		t.Fatalf("Watch = %v, want too_large", err)
	}
	if reported {
		t.Fatal("Watch reported changes for a folder it refuses")
	}
}

func TestWatchHasEightSlots(t *testing.T) {
	for range maxWatches {
		watchSlots <- struct{}{}
	}
	err := Watch(context.Background(), t.TempDir(), "busy", WatchRequest{}, 20*time.Millisecond, func([]Change) {})
	if got := codeOf(t, err); got != "busy" {
		t.Fatalf("Watch = %v, want busy", err)
	}
	for range maxWatches {
		<-watchSlots
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
