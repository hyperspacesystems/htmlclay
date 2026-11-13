package dataguard

import (
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/versions"
)

// ResolveResult is the outcome of a resolve: which event was answered, what was
// asked for, and the status the page and the panel now see.
type ResolveResult struct {
	ID     string `json:"id"`
	Choice string `json:"choice"`
	Status string `json:"status"`
}

// A resolve can fail in ways the transport answers with its own status: no event
// is 404, a mismatched id is 409, a restore that cannot reproduce its data and a
// revert with nothing to read are 422, and an unknown choice is 400.
var (
	ErrNoEvent       = errors.New("dataguard: no pending data-loss event")
	ErrEventMismatch = errors.New("dataguard: event id mismatch (already resolved?)")
	ErrNotRestorable = errors.New("dataguard: data could not be restored onto the current page; use revert")
	ErrNoRecoverCopy = errors.New("dataguard: no whole-file backup is available to revert to")
	ErrUnknownChoice = errors.New("dataguard: unknown choice")
)

// CurrentEvent is the page-load read. It re-validates a pending event against the
// current bytes, so an event whose loss was already undone elsewhere is cleared
// instead of offered again, and it seeds a baseline on first sight so an
// already-open file is covered.
func (g *Guard) CurrentEvent(key, absPath string, read func() ([]byte, error)) (*ClientEvent, error) {
	if key == "" {
		log.Printf("dataguard: %s has no history key; guard skipped", absPath)
		return nil, nil
	}

	var event *ClientEvent
	var err error
	done := make(chan struct{})
	g.queue.Do(key, func() {
		defer close(done)
		current, readErr := read()
		if readErr != nil {
			if !errors.Is(readErr, fs.ErrNotExist) {
				err = readErr
			}
			return
		}
		event, err = g.currentEvent(key, absPath, current)
	})
	<-done
	return event, err
}

// currentEvent answers from bytes read inside the document's queue job, so an
// answer can never be computed from bytes older than a write already judged.
func (g *Guard) currentEvent(key, absPath string, current []byte) (*ClientEvent, error) {
	if key == "" {
		log.Printf("dataguard: %s has no history key; guard skipped", absPath)
		return nil, nil
	}

	st, err := g.store.Read(key, absPath)
	if err != nil {
		return nil, err
	}
	cur := g.extractOrNil(absPath, current)

	if st.Event != nil {
		// The loss is undone only when the current island still CONTAINS every
		// pinned recoverable atom, the same judgement Observe makes on a write.
		recoverable, err := decodeIsland(st.Event.RecoverableData)
		if err != nil {
			log.Printf("dataguard: could not read an event's recoverable data: %v", err)
			recoverable = nil
		}
		if cur != nil && recoverable != nil && lossUndone(g.classify(NormalizeComposedBaseline(recoverable, current), cur)) {
			g.clearEvent(st, cur, "restored", false)
			return nil, g.store.Write(key, absPath, st)
		}
		return g.clientEvent(st.Event, cur, current), nil
	}

	if cur != nil && !IsEmptyIsland(cur) && st.Baseline == nil {
		st.Baseline = g.baselineOf(cur)
		st.UIWorkPending = false
		st.LastSeenHash = versions.Hash(current)
		if err := g.store.Write(key, absPath, st); err != nil && !errors.Is(err, versions.ErrNoHistory) {
			return nil, err
		}
	}
	return nil, nil
}

// Resolve answers a pending event: dismiss it and adopt the current page,
// restore the recoverable data onto the current page, or revert to the recover
// copy. It runs on the document's queue, so it cannot interleave with an
// Observe, and it records the hash of the bytes it leaves on disk before
// returning, so neither the watcher nor the catch-up pass reads a restore as a
// new outside edit.
func (g *Guard) Resolve(key, absPath, id, choice string, current []byte, writeBack func([]byte) error) (*ResolveResult, error) {
	if key == "" {
		log.Printf("dataguard: %s has no history key; guard skipped", absPath)
		return nil, ErrNoEvent
	}

	var res *ResolveResult
	var err error
	done := make(chan struct{})
	g.queue.Do(key, func() {
		defer close(done)
		res, err = g.resolve(key, absPath, id, choice, current, writeBack)
	})
	<-done
	return res, err
}

func (g *Guard) resolve(key, absPath, id, choice string, current []byte, writeBack func([]byte) error) (*ResolveResult, error) {
	st, err := g.store.Read(key, absPath)
	if err != nil {
		return nil, err
	}
	if st.Event == nil {
		return nil, ErrNoEvent
	}
	event := st.Event
	if id != "" && event.ID != id {
		return nil, ErrEventMismatch
	}

	switch choice {
	case "dismiss":
		// forceBaseline: dismiss accepts the current page even when the clobber
		// emptied the island, else the old baseline stays and the next write
		// re-fires.
		g.clearEvent(st, g.extractOrNil(absPath, current), "dismissed", true)
		return g.resolved(key, absPath, current, st, event, choice)

	case "restore":
		recoverable, err := decodeIsland(event.RecoverableData)
		if err != nil {
			log.Printf("dataguard: could not read an event's recoverable data: %v", err)
			recoverable = nil
		}
		merged, err := g.ext.Restore(current, recoverable)
		if err != nil {
			if errors.Is(err, ErrNotRestorable) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: %v", ErrNotRestorable, err)
		}
		if writeBack == nil {
			return nil, fmt.Errorf("%w: a restore needs a write-back", ErrNotRestorable)
		}
		if err := writeBack(merged); err != nil {
			return nil, err
		}
		kept := g.extractOrNil(absPath, merged)
		if kept == nil {
			kept = recoverable
		}
		g.clearEvent(st, kept, "restored", false)
		return g.resolved(key, absPath, merged, st, event, choice)

	case "revert":
		if !event.HasRecoverCopy {
			return nil, ErrNoRecoverCopy
		}
		// The recover copy is read through the store, and the event carries no
		// path, so a corrupt state file cannot point this read elsewhere.
		html, err := g.store.ReadRecover(key, absPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNoRecoverCopy, err)
		}
		if html == nil {
			return nil, ErrNoRecoverCopy
		}
		if writeBack == nil {
			return nil, fmt.Errorf("%w: a revert needs a write-back", ErrNoRecoverCopy)
		}
		if err := writeBack(html); err != nil {
			return nil, err
		}
		g.clearEvent(st, g.extractOrNil(absPath, html), "reverted", false)
		return g.resolved(key, absPath, html, st, event, choice)

	default:
		return nil, ErrUnknownChoice
	}
}

// resolved stores the post-resolve state with the hash of the bytes now on disk.
func (g *Guard) resolved(key, absPath string, seen []byte, st *State, event *Event, choice string) (*ResolveResult, error) {
	st.LastSeenHash = versions.Hash(seen)
	if err := g.store.Write(key, absPath, st); err != nil {
		return nil, err
	}
	return &ResolveResult{ID: event.ID, Choice: choice, Status: st.Status}, nil
}

// clearEvent is the read-modify-write body of a resolve: the event is gone, the
// status records how, and the baseline becomes the island that stays on the
// page. A forced baseline adopts an empty island too, so a dismissed wipe stays
// dismissed.
func (g *Guard) clearEvent(st *State, kept dataapi.Value, status string, forceBaseline bool) {
	st.Event = nil
	st.Status = status
	st.UIWorkPending = false
	if forceBaseline {
		if kept == nil {
			st.Baseline = nil
			return
		}
		st.Baseline = g.baselineOf(kept)
		return
	}
	if kept != nil && !IsEmptyIsland(kept) {
		st.Baseline = g.baselineOf(kept)
	}
}

// extractOrNil reads the island of a document, failing open the way Observe
// does: bytes the guard cannot read are no data, never a failed resolve.
func (g *Guard) extractOrNil(absPath string, html []byte) dataapi.Value {
	island, err := g.ext.Extract(html)
	if err != nil {
		log.Printf("dataguard: could not read the data of %s: %v", absPath, err)
		return nil
	}
	return island
}
