package agentlane

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

// ErrOperationReused is a Save-ID sent again with a different request. The lane
// answers it 400 operation-reused, because running it would make one id mean two
// writes.
var ErrOperationReused = errors.New("this Save-ID was already used for a different request")

const (
	replayCap = 256
	replayTTL = 10 * time.Minute
)

// Answer is a write's whole HTTP answer, kept so a repeat gets the same bytes.
type Answer struct {
	Status int
	Header http.Header
	Body   []byte
}

type replayEntry struct {
	fingerprint string
	done        chan struct{}
	answer      Answer
	finished    time.Time
}

// Replay remembers each write's answer by document id and Save-ID, so a client
// that retries after a dropped connection gets the first answer instead of a
// second write.
type Replay struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]*replayEntry
	order   []string // keys in the order they were started, oldest first
}

func NewReplay(now func() time.Time) *Replay {
	if now == nil {
		now = time.Now
	}
	return &Replay{now: now, entries: make(map[string]*replayEntry)}
}

// Do runs the write named by key once. The same key and fingerprint answers what
// the first call answered, waiting for it if it is still running, also after a
// later write to the document. The same key with another fingerprint is
// ErrOperationReused and runs nothing. An answer of 500 or more is not kept: every
// write carries If-Match, so running it again cannot write twice, and a write that
// kept nothing deserves a real retry.
func (p *Replay) Do(key, fingerprint string, run func() Answer) (Answer, error) {
	p.mu.Lock()
	p.pruneLocked()
	if e, ok := p.entries[key]; ok {
		p.mu.Unlock()
		if e.fingerprint != fingerprint {
			return Answer{}, ErrOperationReused
		}
		<-e.done
		return e.answer, nil
	}
	e := &replayEntry{fingerprint: fingerprint, done: make(chan struct{})}
	p.entries[key] = e
	p.order = append(p.order, key)
	p.mu.Unlock()

	answer := run()

	p.mu.Lock()
	e.answer = answer
	e.finished = p.now()
	close(e.done)
	if answer.Status >= http.StatusInternalServerError {
		p.forgetLocked(key, e)
	}
	p.pruneLocked()
	p.mu.Unlock()
	return answer, nil
}

// forgetLocked drops e, so the next call with its key runs again. Callers already
// waiting on e still get its answer.
func (p *Replay) forgetLocked(key string, e *replayEntry) {
	if p.entries[key] != e {
		return
	}
	delete(p.entries, key)
	for i, k := range p.order {
		if k == key {
			p.order = append(p.order[:i], p.order[i+1:]...)
			return
		}
	}
}

// pruneLocked drops finished answers older than replayTTL, then the oldest
// finished answers past replayCap. A call that is still running is never
// dropped, so its waiters always get its answer.
func (p *Replay) pruneLocked() {
	now := p.now()
	finished := func(e *replayEntry) bool {
		select {
		case <-e.done:
			return true
		default:
			return false
		}
	}
	kept := p.order[:0]
	for _, key := range p.order {
		e := p.entries[key]
		if finished(e) && now.Sub(e.finished) >= replayTTL {
			delete(p.entries, key)
			continue
		}
		kept = append(kept, key)
	}
	p.order = kept
	for over := len(p.order) - replayCap; over > 0; {
		dropped := false
		for i, key := range p.order {
			if finished(p.entries[key]) {
				delete(p.entries, key)
				p.order = append(p.order[:i], p.order[i+1:]...)
				over--
				dropped = true
				break
			}
		}
		if !dropped {
			break
		}
	}
}
