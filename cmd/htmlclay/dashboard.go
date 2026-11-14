package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

const (
	// dashboardAPIPrefix is the one part of the dashboard's API the app answers in
	// Go. Everything else under /api/ is the live-room service's, through the
	// served host's proxy.
	dashboardAPIPrefix = "/api/app/"

	// whyNeedsApp is the reason a flag reads false on the dashboard: the action
	// needs the Mac app's consent, which a browser can never give.
	whyNeedsApp = "needs-app"

	// dashboardPingEvery is how often the event stream sends a comment line, so a
	// quiet stream is not closed for looking idle.
	dashboardPingEvery = 15 * time.Second

	// dashboardMaxBody and dashboardMaxActionID bound one action, as the shell's
	// listener bounds the Mac app's.
	dashboardMaxBody     = 64 << 10
	dashboardMaxActionID = 100
)

// dashboardAPI answers the dashboard's API: the app's own three routes from Go,
// and every other route through proxy, which is the served host's window onto the
// live-room service.
func (a *app) dashboardAPI(proxy http.Handler) http.Handler {
	app := &dashboardApp{a: a}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, dashboardAPIPrefix) {
			app.ServeHTTP(w, r)
			return
		}
		if proxy == nil {
			w.Header().Set("Retry-After", "1")
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": http.StatusText(http.StatusServiceUnavailable), "reason": "unavailable"})
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

// dashboardApp is the app's own part of the dashboard: the snapshot built for the
// dashboard channel, that snapshot whenever it changes, and the actions a browser
// on the dashboard's origin may send.
type dashboardApp struct {
	a *app
}

func (d *dashboardApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/api/app/snapshot":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, dashboardSnapshot(d.a.snapshot()))
	case "/api/app/events":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		d.events(w, r)
	case "/api/app/actions":
		if r.Method != http.MethodPost {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		d.act(w, r)
	default:
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
	}
}

// act runs one action as the dashboard channel, so the class check refuses every
// grant before any effect and a browser can never answer a request.
func (d *dashboardApp) act(w http.ResponseWriter, r *http.Request) {
	var action appcore.Action
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, dashboardMaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&action); err != nil || action.ID == "" || action.Type == "" || len(action.ID) > dashboardMaxActionID {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": http.StatusText(http.StatusBadRequest), "reason": "bad-arguments"})
		return
	}
	snapshot, refusal := d.a.do(r.Context(), action, ChannelDashboard)
	if refusal != nil {
		body := map[string]any{"code": refusal.Code, "text": appcore.RefusalMessage(refusal)}
		if refusal.Field != "" {
			body["field"] = refusal.Field
		}
		writeDashboardJSON(w, http.StatusConflict, map[string]any{"refusal": body})
		return
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{"snapshot": dashboardSnapshot(snapshot)})
}

// events streams one snapshot event per change, the current one first, and a
// comment line every dashboardPingEvery.
func (d *dashboardApp) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}
	updates, unsubscribe := d.a.subscribe()
	defer unsubscribe()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	send := func(snapshot appcore.Snapshot) bool {
		raw, err := json.Marshal(dashboardSnapshot(snapshot))
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", raw); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send(d.a.snapshot()) {
		return
	}

	ticker := time.NewTicker(dashboardPingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case snapshot, open := <-updates:
			if !open {
				return
			}
			if !send(snapshot) {
				return
			}
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// dashboardSnapshot is the snapshot the dashboard channel reads: every flag that
// needs a grant reads false, with a why that says the Mac app has to do it. The
// class check refuses those actions anyway; this is what the dashboard shows.
func dashboardSnapshot(s appcore.Snapshot) appcore.Snapshot {
	if s.Host.Can == nil {
		s.Host.Can = make(map[string]bool, len(appcore.ActionClasses))
	}
	for typ, class := range appcore.ActionClasses {
		if class == appcore.ClassGrant {
			s.Host.Can[typ] = false
		}
	}
	text := appcore.RefusalMessage(&appcore.Refusal{Code: appcore.RefuseNeedsConfirmation})
	for i := range s.Folders {
		s.Folders[i].Can.Serve = false
		s.Folders[i].Why.Serve = &appcore.Reason{Code: whyNeedsApp, Text: text}
	}
	return s
}

func writeDashboardJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
