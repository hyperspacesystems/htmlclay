//go:build managedtrial || servedfaults

package served

import "net/http"

// controlFaultBody is one POST /v1/fault: the fault point a harness arms, the
// logical path it fires for, and how many writes take it.
type controlFaultBody struct {
	Point string `json:"point"`
	Rel   string `json:"rel"`
	Count *int   `json:"count"`
}

// controlFaultMaxCount bounds how many writes one arming covers.
const controlFaultMaxCount = 100

// serveFault is POST /v1/fault: a harness arms one of the trial's fault points,
// or clears them all.
func (a *controlAPI) serveFault(w http.ResponseWriter, r *http.Request) {
	var body controlFaultBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	if body.Point == "clear" {
		clearFaults()
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "armed", Value: 0}})
		return
	}
	if body.Point == "release" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "released", Value: releaseTailwind(body.Rel)}})
		return
	}
	if body.Point == "held" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "held", Value: heldTailwind(body.Rel)}})
		return
	}
	if body.Point == "remaining" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "remaining", Value: remainingFaults(body.Rel)}})
		return
	}
	if body.Point == "finished" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "finished", Value: heldTailwindFinished(body.Rel)}})
		return
	}
	if !faultPoints[body.Point] {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "unknown fault point: " + body.Point})
		return
	}
	if body.Rel == "" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "fault needs a rel"})
		return
	}
	count := 1
	if body.Count != nil {
		count = *body.Count
	}
	if count < 1 || count > controlFaultMaxCount {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "fault count out of range"})
		return
	}
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "armed", Value: armFault(body.Point, body.Rel, count)}})
}

// faultRoute answers the trial's fault route, or nil when this request is not
// one.
func (a *controlAPI) faultRoute(r *http.Request) http.HandlerFunc {
	if r.URL.Path == "/v1/fault" && r.Method == http.MethodPost {
		return a.serveFault
	}
	return nil
}
