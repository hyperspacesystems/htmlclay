package confirm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

var requestIDPattern = regexp.MustCompile(`^r[0-9a-f]{12}$`)

// chosenFile is absolute on every system, which is what choose-file requires.
var chosenFile = filepath.Join(os.TempDir(), "note.html")

type harness struct {
	t       *testing.T
	r       *Registry
	changed chan struct{}
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, changed: make(chan struct{}, 256)}
	h.r = New(func() {
		select {
		case h.changed <- struct{}{}:
		default:
		}
	})
	t.Cleanup(h.r.Stop)
	return h
}

// awaitChange waits for one call to the change callback.
func (h *harness) awaitChange() {
	h.t.Helper()
	select {
	case <-h.changed:
	case <-time.After(5 * time.Second):
		h.t.Fatal("the change callback never ran")
	}
}

// awaitPending waits for the open questions to satisfy ok.
func (h *harness) awaitPending(ok func([]Pending) bool) []Pending {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending := h.r.Pending()
		if ok(pending) {
			return pending
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("pending never reached the wanted state; last was %+v", pending)
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) awaitOne() Pending {
	h.t.Helper()
	pending := h.awaitPending(func(p []Pending) bool { return len(p) == 1 })
	return pending[0]
}

type outcome struct {
	reply Reply
	err   error
}

func (h *harness) ask(ctx context.Context, ask Ask) <-chan outcome {
	out := make(chan outcome, 1)
	go func() {
		reply, err := h.r.Ask(ctx, ask)
		out <- outcome{reply: reply, err: err}
	}()
	return out
}

// awaitAsk waits for one Ask to return.
func (h *harness) awaitAsk(out <-chan outcome) outcome {
	h.t.Helper()
	select {
	case res := <-out:
		return res
	case <-time.After(5 * time.Second):
		h.t.Fatal("Ask did not return")
		return outcome{}
	}
}

func TestM3RequestLifetime(t *testing.T) {
	t.Run("an empty id makes a new one at revision 1", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{
			Kind:     "document-found",
			Document: "note.html",
			Answers:  []string{"deny", "allow-document"},
		})
		p := h.awaitOne()
		if !requestIDPattern.MatchString(p.ID) {
			t.Fatalf("id = %q, want r and twelve hex digits", p.ID)
		}
		if p.Ask.ID != p.ID {
			t.Fatalf("ask id = %q, want the request id %q", p.Ask.ID, p.ID)
		}
		if p.Revision != 1 {
			t.Fatalf("revision = %d, want 1", p.Revision)
		}
		if err := h.r.Answer(p.ID, 1, "allow-document", ""); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		res := h.awaitAsk(out)
		if res.err != nil {
			t.Fatalf("Ask: %v", res.err)
		}
		if want := (Reply{ID: p.ID, Choice: "allow-document"}); res.reply != want {
			t.Fatalf("reply = %+v, want %+v", res.reply, want)
		}
	})

	t.Run("an earlier id replaces the request and bumps the revision", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{
			Kind:    "no-file",
			Detail:  "first wording",
			Answers: []string{"deny", "choose-file"},
		})
		first := h.awaitOne()
		out2 := h.ask(context.Background(), Ask{
			ID:      first.ID,
			Kind:    "no-file",
			Detail:  "second wording",
			Answers: []string{"deny", "choose-file"},
		})
		second := h.awaitPending(func(p []Pending) bool {
			return len(p) == 1 && p[0].Revision == 2
		})[0]
		if second.ID != first.ID {
			t.Fatalf("id = %q, want the same request %q", second.ID, first.ID)
		}
		if second.Ask.Detail != "second wording" {
			t.Fatalf("detail = %q, want the new wording", second.Ask.Detail)
		}
		if err := h.r.Answer(first.ID, 2, "deny", ""); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		for _, out := range []<-chan outcome{out, out2} {
			res := h.awaitAsk(out)
			if res.err != nil {
				t.Fatalf("Ask: %v", res.err)
			}
			if want := (Reply{ID: first.ID, Choice: "deny"}); res.reply != want {
				t.Fatalf("reply = %+v, want %+v", res.reply, want)
			}
		}
	})

	t.Run("equal asks are one request", func(t *testing.T) {
		h := newHarness(t)
		ask := Ask{
			Kind:     "folder-read",
			Folder:   "/Users/somebody/Desktop",
			Identity: "folder-1",
			Scope:    "document",
			Answers:  []string{"deny", "allow-once", "trust-folder"},
		}
		one := h.ask(context.Background(), ask)
		h.awaitChange()
		two := h.ask(context.Background(), ask)
		h.awaitChange()
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "allow-once", ""); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		firstRes := h.awaitAsk(one)
		secondRes := h.awaitAsk(two)
		if firstRes.err != nil || secondRes.err != nil {
			t.Fatalf("Ask: %v, %v", firstRes.err, secondRes.err)
		}
		want := Reply{ID: p.ID, Choice: "allow-once"}
		if firstRes.reply != want || secondRes.reply != want {
			t.Fatalf("replies = %+v and %+v, want %+v for both", firstRes.reply, secondRes.reply, want)
		}
	})

	t.Run("no such request", func(t *testing.T) {
		h := newHarness(t)
		if err := h.r.Answer("r000000000000", 1, "deny", ""); !errors.Is(err, ErrNoSuchRequest) {
			t.Fatalf("Answer = %v, want ErrNoSuchRequest", err)
		}
		out := h.ask(context.Background(), Ask{Kind: "no-file", Answers: []string{"deny"}})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "deny", ""); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		h.awaitAsk(out)
		if err := h.r.Answer(p.ID, p.Revision, "deny", ""); !errors.Is(err, ErrNoSuchRequest) {
			t.Fatalf("Answer after removal = %v, want ErrNoSuchRequest", err)
		}
	})

	t.Run("stale revision", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{Kind: "no-file", Answers: []string{"deny", "choose-file"}})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision+1, "deny", ""); !errors.Is(err, ErrRequestChanged) {
			t.Fatalf("Answer at a later revision = %v, want ErrRequestChanged", err)
		}
		if err := h.r.Answer(p.ID, p.Revision-1, "deny", ""); !errors.Is(err, ErrRequestChanged) {
			t.Fatalf("Answer at an earlier revision = %v, want ErrRequestChanged", err)
		}
		pending := h.r.Pending()
		if len(pending) != 1 || pending[0].Revision != p.Revision {
			t.Fatalf("pending after a stale answer = %+v, want the request unchanged at revision %d", pending, p.Revision)
		}
		if err := h.r.Answer(p.ID, p.Revision, "deny", ""); err != nil {
			t.Fatalf("Answer at the shown revision: %v", err)
		}
		h.awaitAsk(out)
	})

	t.Run("already answered", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{Kind: "no-file", Answers: []string{"deny", "choose-file"}})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "choose-file", chosenFile); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		h.awaitAsk(out)
		if err := h.r.Answer(p.ID, p.Revision, "deny", ""); !errors.Is(err, ErrRequestChanged) {
			t.Fatalf("second Answer = %v, want ErrRequestChanged", err)
		}
	})

	t.Run("not offered", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{
			Kind:    "document-found",
			Answers: []string{"deny", "allow-document"},
		})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "allow-all-trusted", ""); !errors.Is(err, ErrAnswerNotOffered) {
			t.Fatalf("Answer with a choice it does not offer = %v, want ErrAnswerNotOffered", err)
		}
		if pending := h.r.Pending(); len(pending) != 1 {
			t.Fatalf("pending after an unoffered answer = %d, want the request still listed", len(pending))
		}
		if err := h.r.Answer(p.ID, p.Revision, "allow-document", ""); err != nil {
			t.Fatalf("Answer with an offered choice: %v", err)
		}
		h.awaitAsk(out)
	})

	t.Run("bad path", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{Kind: "no-file", Answers: []string{"deny", "choose-file"}})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "choose-file", "note.html"); !errors.Is(err, ErrBadPath) {
			t.Fatalf("choose-file with a relative path = %v, want ErrBadPath", err)
		}
		if err := h.r.Answer(p.ID, p.Revision, "choose-file", ""); !errors.Is(err, ErrBadPath) {
			t.Fatalf("choose-file with no path = %v, want ErrBadPath", err)
		}
		if err := h.r.Answer(p.ID, p.Revision, "deny", chosenFile); !errors.Is(err, ErrBadPath) {
			t.Fatalf("deny with a path = %v, want ErrBadPath", err)
		}
		if pending := h.r.Pending(); len(pending) != 1 || pending[0].Revision != p.Revision {
			t.Fatalf("pending after bad paths = %+v, want the request unchanged", pending)
		}
		if err := h.r.Answer(p.ID, p.Revision, "choose-file", chosenFile); err != nil {
			t.Fatalf("Answer with an absolute path: %v", err)
		}
		res := h.awaitAsk(out)
		if want := (Reply{ID: p.ID, Choice: "choose-file", Path: chosenFile}); res.reply != want {
			t.Fatalf("reply = %+v, want %+v", res.reply, want)
		}
	})

	t.Run("choose-file keeps the request listed until asked again or closed", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{Kind: "no-file", Answers: []string{"deny", "choose-file"}})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "choose-file", chosenFile); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		h.awaitAsk(out)
		if pending := h.r.Pending(); len(pending) != 1 {
			t.Fatalf("pending right after a choose-file answer = %d, want the request still listed", len(pending))
		}
		again := h.ask(context.Background(), Ask{
			ID:      p.ID,
			Kind:    "no-file",
			Detail:  "the file did not open",
			Answers: []string{"deny", "choose-file"},
		})
		reasked := h.awaitPending(func(list []Pending) bool {
			return len(list) == 1 && list[0].Revision == p.Revision+1
		})[0]
		if reasked.ID != p.ID {
			t.Fatalf("id = %q, want %q", reasked.ID, p.ID)
		}
		h.r.Close(p.ID)
		if pending := h.r.Pending(); len(pending) != 0 {
			t.Fatalf("pending after Close = %d, want 0", len(pending))
		}
		res := h.awaitAsk(again)
		if res.err != nil {
			t.Fatalf("Ask: %v", res.err)
		}
		if want := (Reply{ID: p.ID, Choice: "deny"}); res.reply != want {
			t.Fatalf("reply after Close = %+v, want %+v", res.reply, want)
		}
	})

	t.Run("any other choice removes the request before Answer returns", func(t *testing.T) {
		h := newHarness(t)
		out := h.ask(context.Background(), Ask{
			Kind:    "document-found",
			Answers: []string{"deny", "allow-document"},
		})
		p := h.awaitOne()
		if err := h.r.Answer(p.ID, p.Revision, "allow-document", ""); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		if pending := h.r.Pending(); len(pending) != 0 {
			t.Fatalf("pending in the same breath as Answer = %d, want 0", len(pending))
		}
		h.awaitAsk(out)
	})

	t.Run("sixteen pending at most", func(t *testing.T) {
		h := newHarness(t)
		outs := make([]<-chan outcome, 0, maxPending)
		for i := 0; i < maxPending; i++ {
			outs = append(outs, h.ask(context.Background(), Ask{
				Kind:    "folder-read",
				Folder:  fmt.Sprintf("/tmp/folder-%d", i),
				Answers: []string{"deny", "allow-once"},
			}))
		}
		h.awaitPending(func(p []Pending) bool { return len(p) == maxPending })
		over := h.ask(context.Background(), Ask{
			Kind:    "folder-read",
			Folder:  "/tmp/folder-over",
			Answers: []string{"deny", "allow-once"},
		})
		res := h.awaitAsk(over)
		if res.err != nil {
			t.Fatalf("Ask beyond the limit: %v", res.err)
		}
		if res.reply.Choice != ChoiceDeny {
			t.Fatalf("choice beyond the limit = %q, want deny", res.reply.Choice)
		}
		if !requestIDPattern.MatchString(res.reply.ID) {
			t.Fatalf("id beyond the limit = %q, want r and twelve hex digits", res.reply.ID)
		}
		if pending := h.r.Pending(); len(pending) != maxPending {
			t.Fatalf("pending = %d, want the %d already open", len(pending), maxPending)
		}
		h.r.Stop()
		for _, out := range outs {
			if res := h.awaitAsk(out); res.reply.Choice != ChoiceDeny {
				t.Fatalf("choice after Stop = %q, want deny", res.reply.Choice)
			}
		}
	})

	t.Run("a cancelled ask is denied and the last one removed", func(t *testing.T) {
		h := newHarness(t)
		ask := Ask{Kind: "folder-read", Folder: "/tmp/folder", Answers: []string{"deny", "allow-once"}}
		ctxOne, cancelOne := context.WithCancel(context.Background())
		defer cancelOne()
		ctxTwo, cancelTwo := context.WithCancel(context.Background())
		defer cancelTwo()
		one := h.ask(ctxOne, ask)
		h.awaitChange()
		two := h.ask(ctxTwo, ask)
		h.awaitChange()
		p := h.awaitOne()

		cancelOne()
		first := h.awaitAsk(one)
		if first.err != nil {
			t.Fatalf("Ask for the cancelled waiter: %v", first.err)
		}
		if want := (Reply{ID: p.ID, Choice: ChoiceDeny}); first.reply != want {
			t.Fatalf("reply to the cancelled waiter = %+v, want %+v", first.reply, want)
		}
		if pending := h.r.Pending(); len(pending) != 1 {
			t.Fatalf("pending with one waiter left = %d, want 1", len(pending))
		}

		cancelTwo()
		second := h.awaitAsk(two)
		if second.err != nil {
			t.Fatalf("Ask for the last waiter: %v", second.err)
		}
		if want := (Reply{ID: p.ID, Choice: ChoiceDeny}); second.reply != want {
			t.Fatalf("reply to the last waiter = %+v, want %+v", second.reply, want)
		}
		h.awaitPending(func(p []Pending) bool { return len(p) == 0 })
		if err := h.r.Answer(p.ID, p.Revision, "deny", ""); !errors.Is(err, ErrNoSuchRequest) {
			t.Fatalf("Answer after cancellation = %v, want ErrNoSuchRequest", err)
		}
	})

	t.Run("stop denies every pending ask", func(t *testing.T) {
		h := newHarness(t)
		asks := []Ask{
			{Kind: "document-found", Document: "a.html", Answers: []string{"deny", "allow-document"}},
			{Kind: "document-found", Document: "b.html", Answers: []string{"deny", "allow-document"}},
			{Kind: "program-add", Program: "Terminal", Answers: []string{"deny", "allow-once"}},
		}
		outs := make([]<-chan outcome, 0, len(asks))
		for _, ask := range asks {
			outs = append(outs, h.ask(context.Background(), ask))
		}
		pending := h.awaitPending(func(p []Pending) bool { return len(p) == len(asks) })
		ids := make([]string, len(pending))
		for i, p := range pending {
			ids[i] = p.ID
		}
		h.r.Stop()
		denied := make(map[string]bool, len(ids))
		for _, out := range outs {
			res := h.awaitAsk(out)
			if res.err != nil {
				t.Fatalf("Ask: %v", res.err)
			}
			if res.reply.Choice != ChoiceDeny || res.reply.Path != "" {
				t.Fatalf("reply after Stop = %+v, want a plain deny", res.reply)
			}
			denied[res.reply.ID] = true
		}
		for _, id := range ids {
			if !denied[id] {
				t.Fatalf("no waiter was denied for the open request %q", id)
			}
		}
		if len(denied) != len(ids) {
			t.Fatalf("denied ids = %v, want one per open request", denied)
		}
		if pending := h.r.Pending(); len(pending) != 0 {
			t.Fatalf("pending after Stop = %d, want 0", len(pending))
		}
	})

	t.Run("changed may read pending", func(t *testing.T) {
		var r *Registry
		read := make(chan []Pending, 8)
		r = New(func() { read <- r.Pending() })
		t.Cleanup(r.Stop)
		out := make(chan outcome, 1)
		go func() {
			reply, err := r.Ask(context.Background(), Ask{Kind: "no-file", Answers: []string{"deny"}})
			out <- outcome{reply: reply, err: err}
		}()
		var pending []Pending
		select {
		case pending = <-read:
		case <-time.After(5 * time.Second):
			t.Fatal("a change callback that reads Pending deadlocked")
		}
		if len(pending) != 1 {
			t.Fatalf("pending read from the callback = %d, want 1", len(pending))
		}
		if err := r.Answer(pending[0].ID, pending[0].Revision, "deny", ""); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		select {
		case res := <-out:
			if res.err != nil {
				t.Fatalf("Ask: %v", res.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Ask did not return")
		}
	})
}
