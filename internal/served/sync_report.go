package served

import (
	"encoding/json"
	"net/http"
)

// SyncReport is C4's POST /v1/sync/report: the service's sessions, the events since
// the last acknowledged report, and whether the server refused the key.
type SyncReport struct {
	Seq        int64               `json:"seq"`
	Sessions   []SyncSessionReport `json:"sessions"`
	KeyRefused bool                `json:"keyRefused"`
	// Accounts is a new discovery answer's rows when one arrived, else null.
	Accounts json.RawMessage `json:"accounts"`
	Events   []SyncEvent     `json:"events"`
}

type SyncSessionReport struct {
	SessionID    string           `json:"sessionId"`
	RootID       string           `json:"rootId"`
	AccountID    int64            `json:"accountId"`
	Status       string           `json:"status"`
	Running      bool             `json:"running"`
	Paused       json.RawMessage  `json:"paused"`
	PendingCount int              `json:"pendingCount"`
	Conflicts    int              `json:"conflicts"`
	Blocked      int              `json:"blocked"`
	LastSyncAt   *string          `json:"lastSyncAt"`
	LastError    json.RawMessage  `json:"lastError"`
	Progress     *SyncProgress    `json:"progress"`
	RemovalHeld  *SyncRemovalHeld `json:"removalHeld"`
	// Record is the session row as the engine last saved it; Go takes only the
	// fields 8.19 names from it.
	Record json.RawMessage `json:"record"`
}

type SyncProgress struct {
	Phase      string `json:"phase"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
}

type SyncRemovalHeld struct {
	Files int      `json:"files"`
	Paths []string `json:"paths"`
}

type SyncEvent struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	RootID    string `json:"rootId"`
	At        string `json:"at"`
	Action    string `json:"action"`
	File      string `json:"file"`
	Type      string `json:"type"`
}

// SetSyncSink names the function each accepted report goes to. Until one is set,
// reports are refused, so the service keeps them and sends them again.
func (h *Host) SetSyncSink(sink func(SyncReport)) {
	if h.cfg.service == nil {
		return
	}
	h.cfg.service.syncSink.Store(&sink)
}

// serveSyncReport is POST /v1/sync/report. A report is acknowledged only after the
// sink has it.
func (a *controlAPI) serveSyncReport(w http.ResponseWriter, r *http.Request, link *serviceLink) {
	if noService(w, link) {
		return
	}
	var report SyncReport
	if !decodeControlJSON(w, r, &report) {
		return
	}
	if report.Seq < 0 {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "seq must not be negative"})
		return
	}
	sink := link.syncSink.Load()
	if sink == nil || *sink == nil {
		writeControlJSON(w, http.StatusServiceUnavailable, controlError{Error: "not-ready"})
		return
	}
	(*sink)(report)
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "ok", Value: true}, {Key: "seq", Value: report.Seq}})
}
