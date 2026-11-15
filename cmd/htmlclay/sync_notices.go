package main

import (
	"fmt"
	"slices"
	"sync"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/shellapi"
)

// The three things sync tells the person about, each once (08-sync.md decision
// 19, contract C6).
const (
	noticeConflict    = "conflict"
	noticeKeyStopped  = "keyStopped"
	noticeAddedToTeam = "addedToTeam"
)

// notifiedKept is how many shown notice ids sync.json keeps.
const notifiedKept = 500

// noticeHub is the attached Mac app streams. A stream that cannot keep up
// misses a notice rather than holding the app.
type noticeHub struct {
	mu   sync.Mutex
	subs map[chan shellapi.Notice]struct{}
}

func (h *noticeHub) subscribe() (<-chan shellapi.Notice, func()) {
	ch := make(chan shellapi.Notice, 16)
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[chan shellapi.Notice]struct{}{}
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// deliver hands n to every attached stream, and reports whether one was attached.
func (h *noticeHub) deliver(n shellapi.Notice) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- n:
		default:
		}
	}
	return len(h.subs) > 0
}

// notice tells the person once: the id is kept in sync.json first, so a restart
// does not repeat it. With the Mac app attached it goes as C6's frame; otherwise
// as the system's notification.
func (a *app) notice(n shellapi.Notice) {
	a.rt.syncMu.Lock()
	record, err := a.syncRecordLocked()
	if err != nil {
		a.rt.syncMu.Unlock()
		return
	}
	if slices.Contains(record.Notified, n.ID) {
		a.rt.syncMu.Unlock()
		return
	}
	record.Notified = append(record.Notified, n.ID)
	if len(record.Notified) > notifiedKept {
		record.Notified = record.Notified[len(record.Notified)-notifiedKept:]
	}
	err = record.Save()
	a.rt.syncMu.Unlock()
	if err != nil {
		a.rt.logger.Printf("Could not keep a shown notice: sync.json: %v", err)
	}
	n.Type = "notice"
	if a.rt.notices.deliver(n) {
		return
	}
	title, body := noticeText(n)
	if err := a.notifyUser(title, body); err != nil {
		a.rt.logger.Printf("Could not show a notification: %v", err)
	}
}

func noticeText(n shellapi.Notice) (title, body string) {
	switch n.Kind {
	case noticeConflict:
		return fmt.Sprintf(appcore.TextWaitingConflict, n.Name), appcore.TextNoticeConflictBody
	case noticeKeyStopped:
		return appcore.TextWaitingSyncKeyRefused, appcore.TextNoticeKeyStoppedBody
	default:
		return fmt.Sprintf(appcore.TextNoticeAddedToTeam, n.Name), appcore.TextNoticeAddedToTeamBody
	}
}

// addedTeams is a notice for each team row in after that before did not have:
// a team the person was added to while connected. Rows a connect sets are not
// compared, so connecting tells nothing.
func addedTeams(before, after []config.SyncAccountRow) []shellapi.Notice {
	had := map[int64]bool{}
	for _, row := range before {
		had[row.AccountID] = true
	}
	var notices []shellapi.Notice
	for _, row := range after {
		if row.Kind != "team" || had[row.AccountID] {
			continue
		}
		name := row.DisplayName
		if name == "" {
			name = row.Username
		}
		folder := row.FolderID
		notices = append(notices, shellapi.Notice{ID: fmt.Sprintf("addedToTeam:%d", row.AccountID), Kind: noticeAddedToTeam, FolderID: &folder, Name: name})
	}
	return notices
}
