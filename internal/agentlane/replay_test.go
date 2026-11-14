package agentlane

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestM3AgentReplay(t *testing.T) {
	newReplay := func() (*Replay, *testClock) {
		clock := &testClock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
		return NewReplay(clock.now), clock
	}
	write := func(writes *atomic.Int32, status int) func() Answer {
		return func() Answer {
			n := writes.Add(1)
			return Answer{Status: status, Body: []byte(fmt.Sprintf("write %d", n))}
		}
	}

	t.Run("a repeat answers the first answer without writing again", func(t *testing.T) {
		p, _ := newReplay()
		var writes atomic.Int32
		first, err := p.Do("doc:s1", "PUT a etag1 body", write(&writes, 200))
		if err != nil {
			t.Fatal(err)
		}
		again, err := p.Do("doc:s1", "PUT a etag1 body", write(&writes, 200))
		if err != nil {
			t.Fatal(err)
		}
		if writes.Load() != 1 {
			t.Fatalf("writes = %d, want 1", writes.Load())
		}
		if string(again.Body) != string(first.Body) || again.Status != first.Status {
			t.Fatalf("repeat = %+v, want %+v", again, first)
		}
	})

	t.Run("a repeat after a later write still answers the first answer", func(t *testing.T) {
		p, _ := newReplay()
		var writes atomic.Int32
		first, _ := p.Do("doc:s1", "f1", write(&writes, 200))
		if _, err := p.Do("doc:s2", "f2", write(&writes, 200)); err != nil {
			t.Fatal(err)
		}
		again, _ := p.Do("doc:s1", "f1", write(&writes, 200))
		if string(again.Body) != string(first.Body) || writes.Load() != 2 {
			t.Fatalf("repeat = %q after %d writes, want %q after 2", again.Body, writes.Load(), first.Body)
		}
	})

	t.Run("the same Save-ID with another request is refused and writes nothing", func(t *testing.T) {
		p, _ := newReplay()
		var writes atomic.Int32
		p.Do("doc:s1", "f1", write(&writes, 200))
		if _, err := p.Do("doc:s1", "f-other", write(&writes, 200)); !errors.Is(err, ErrOperationReused) {
			t.Fatalf("err = %v, want ErrOperationReused", err)
		}
		if writes.Load() != 1 {
			t.Fatalf("writes = %d, want 1", writes.Load())
		}
	})

	t.Run("two calls at once write once and both get the answer", func(t *testing.T) {
		p, _ := newReplay()
		var writes atomic.Int32
		release := make(chan struct{})
		started := make(chan struct{})
		slow := func() Answer {
			writes.Add(1)
			close(started)
			<-release
			return Answer{Status: 200, Body: []byte("once")}
		}
		var wg sync.WaitGroup
		answers := make([]Answer, 2)
		wg.Add(1)
		go func() { defer wg.Done(); answers[0], _ = p.Do("doc:s1", "f1", slow) }()
		<-started
		wg.Add(1)
		go func() { defer wg.Done(); answers[1], _ = p.Do("doc:s1", "f1", write(&writes, 500)) }()
		time.Sleep(20 * time.Millisecond)
		close(release)
		wg.Wait()
		if writes.Load() != 1 || string(answers[0].Body) != "once" || string(answers[1].Body) != "once" {
			t.Fatalf("writes = %d, answers = %q %q", writes.Load(), answers[0].Body, answers[1].Body)
		}
	})

	t.Run("an answer is forgotten after ten minutes", func(t *testing.T) {
		p, clock := newReplay()
		var writes atomic.Int32
		p.Do("doc:s1", "f1", write(&writes, 200))
		clock.add(9 * time.Minute)
		p.Do("doc:s1", "f1", write(&writes, 200))
		if writes.Load() != 1 {
			t.Fatalf("writes inside ten minutes = %d, want 1", writes.Load())
		}
		clock.add(2 * time.Minute)
		p.Do("doc:s1", "f1", write(&writes, 200))
		if writes.Load() != 2 {
			t.Fatalf("writes after ten minutes = %d, want 2", writes.Load())
		}
	})

	t.Run("past 256 answers the oldest finished one is forgotten", func(t *testing.T) {
		p, _ := newReplay()
		var writes atomic.Int32
		for i := 0; i <= replayCap; i++ {
			p.Do(fmt.Sprintf("doc:s%d", i), "f", write(&writes, 200))
		}
		before := writes.Load()
		p.Do("doc:s1", "f", write(&writes, 200))
		if writes.Load() != before {
			t.Fatal("a recent answer was forgotten")
		}
		p.Do("doc:s0", "f", write(&writes, 200))
		if writes.Load() != before+1 {
			t.Fatal("the oldest answer was kept past the cap")
		}
	})

	t.Run("a call still running is never dropped", func(t *testing.T) {
		p, clock := newReplay()
		var writes atomic.Int32
		release := make(chan struct{})
		started := make(chan struct{})
		done := make(chan Answer)
		go func() {
			a, _ := p.Do("doc:slow", "f", func() Answer {
				close(started)
				<-release
				return Answer{Status: 200, Body: []byte("slow")}
			})
			done <- a
		}()
		<-started
		for i := 0; i < replayCap+10; i++ {
			p.Do(fmt.Sprintf("doc:s%d", i), "f", write(&writes, 200))
		}
		clock.add(time.Hour)
		p.Do("doc:other", "f", write(&writes, 200))
		waiter := make(chan Answer)
		go func() {
			a, _ := p.Do("doc:slow", "f", write(&writes, 500))
			waiter <- a
		}()
		time.Sleep(20 * time.Millisecond)
		close(release)
		if a := <-done; string(a.Body) != "slow" {
			t.Fatalf("first = %q", a.Body)
		}
		if a := <-waiter; string(a.Body) != "slow" {
			t.Fatalf("the repeat of a running call = %q, want the running call's answer", a.Body)
		}
	})
}
