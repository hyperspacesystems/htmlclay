//go:build managedtrial

package managedtrial

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// consultDeadline is how long Go waits for one answer from the JS service while
// it holds a document's lock (protocol v1, "Deadlines"). A variable so a test
// can shorten it.
var consultDeadline = 10 * time.Second

// abortRetryInterval is how often a settle-failed abort is resent.
var abortRetryInterval = time.Second

// errServiceUnavailable is a call that got no answer: no service attached, a
// refused connection, a broken reply, or an expired deadline.
var errServiceUnavailable = errors.New("service-unavailable")

// serviceEpochError is a 409 epoch answer: the service is not the one Go knows,
// or Go is not the one the service knows.
type serviceEpochError struct{ epoch string }

func (e *serviceEpochError) Error() string { return "service epoch changed: " + e.epoch }

// servicePeer is the service's address, secret and epoch, from the bootstrap
// line or a later POST /v1/peer.
type servicePeer struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
	Epoch  string `json:"epoch"`
}

// serviceLink is Go's side of the JS service: where it listens, its secret and
// epoch, this process's epoch, and the page documents Go wrote without the
// service confirming them.
type serviceLink struct {
	selfEpoch   string
	events      *eventRing
	unconfirmed *unconfirmedSet
	client      *http.Client

	mu    sync.Mutex
	peer  servicePeer
	roots map[string]string
}

func newServiceLink(events *eventRing, peer servicePeer) *serviceLink {
	return &serviceLink{
		selfEpoch:   events.epoch,
		events:      events,
		unconfirmed: newUnconfirmedSet(),
		client:      &http.Client{Transport: &http.Transport{Proxy: nil, DisableCompression: true}},
		peer:        peer,
		roots:       make(map[string]string),
	}
}

// setRoot records one served root's real folder, where a recovered copy for
// that root is written.
func (l *serviceLink) setRoot(id, baseDir string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.roots[id] = baseDir
}

func (l *serviceLink) rootDir(id string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	dir, ok := l.roots[id]
	return dir, ok
}

// barred reports a document Go wrote without the service confirming it: every
// semantic write to it waits for POST /v1/reconciled.
func (l *serviceLink) barred(canonicalPath string) bool {
	return l.unconfirmed.has(canonicalPath)
}

func (l *serviceLink) setPeer(peer servicePeer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.peer = peer
}

func (l *serviceLink) currentPeer() servicePeer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.peer
}

// call posts one JSON request to the service and decodes its answer. A
// transport failure, a broken reply or an expired context is
// errServiceUnavailable; a 409 epoch answer is a *serviceEpochError; any other
// status that is not 200 is an error naming it.
func (l *serviceLink) call(ctx context.Context, path string, body, out any) error {
	peer := l.currentPeer()
	if peer.URL == "" {
		return errServiceUnavailable
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, peer.URL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", controlScheme+peer.Secret)
	request.Header.Set("X-Trial-Epoch", l.selfEpoch)
	request.Header.Set("X-Trial-Peer-Epoch", peer.Epoch)
	response, err := l.client.Do(request)
	if err != nil {
		return errServiceUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxControlBody+1))
	if err != nil {
		return errServiceUnavailable
	}
	if response.StatusCode == http.StatusConflict {
		var answer struct {
			Error string `json:"error"`
			Epoch string `json:"epoch"`
		}
		if json.Unmarshal(raw, &answer) == nil && answer.Error == "epoch" {
			return &serviceEpochError{epoch: answer.Epoch}
		}
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("service %s answered %d: %s", path, response.StatusCode, bytes.TrimSpace(raw))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errServiceUnavailable
	}
	return nil
}

// consult is one call made while Go holds a document's lock, bounded by
// consultDeadline.
func (l *serviceLink) consult(path string, body, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), consultDeadline)
	defer cancel()
	return l.call(ctx, path, body, out)
}

// abort tells the service a consultation ended without a settle. For
// `settle-failed` the bytes are already on disk and the service still holds the
// reservation, so it is resent every abortRetryInterval until the service
// answers or its epoch changes (a new service never held it). Any other reason
// is sent once. It never blocks the caller.
func (l *serviceLink) abort(consultID, reason string) {
	body := map[string]string{"consultId": consultID, "reason": reason}
	epoch := l.currentPeer().Epoch
	go func() {
		for {
			if l.consult("/v1/abort", body, nil) == nil || reason != "settle-failed" {
				return
			}
			time.Sleep(abortRetryInterval)
			if l.currentPeer().Epoch != epoch {
				return
			}
		}
	}()
}

// markUnconfirmed records a page document Go wrote that the service has not
// confirmed, and tells the service on the event stream.
func (l *serviceLink) markUnconfirmed(canonicalPath, root, name string) {
	if l.unconfirmed.mark(canonicalPath, root, name) {
		l.events.append(func(cursor string) orderedObject {
			return orderedObject{
				{Key: "type", Value: "unconfirmed"},
				{Key: "root", Value: root},
				{Key: "rel", Value: name},
				{Key: "cursor", Value: cursor},
			}
		})
	}
}

// newConsultID is one consultation's id: 32 lowercase hex.
func newConsultID() string { return controlRandomHex(16) }

// unconfirmedEntry is one page document Go wrote without a confirmation.
type unconfirmedEntry struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// unconfirmedSet is the page documents Go wrote while the service could not
// confirm them (protocol v1, "Failures"), keyed by canonical path. Every
// semantic write to one is refused until the service posts /v1/reconciled.
type unconfirmedSet struct {
	mu    sync.Mutex
	paths map[string]unconfirmedEntry
}

func newUnconfirmedSet() *unconfirmedSet {
	return &unconfirmedSet{paths: make(map[string]unconfirmedEntry)}
}

// mark reports whether the path was newly marked.
func (s *unconfirmedSet) mark(canonicalPath, root, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.paths[canonicalPath]; ok {
		return false
	}
	s.paths[canonicalPath] = unconfirmedEntry{Root: root, Name: name}
	return true
}

func (s *unconfirmedSet) lift(canonicalPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.paths, canonicalPath)
}

func (s *unconfirmedSet) has(canonicalPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.paths[canonicalPath]
	return ok
}

func (s *unconfirmedSet) list() []unconfirmedEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := make([]unconfirmedEntry, 0, len(s.paths))
	for _, entry := range s.paths {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Root != entries[j].Root {
			return entries[i].Root < entries[j].Root
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}
