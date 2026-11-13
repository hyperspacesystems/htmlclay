//go:build managedtrial

package managedtrial

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

const controlScheme = "Bearer "

type controlError struct {
	Error string `json:"error"`
}

type controlHealth struct {
	OK    bool `json:"ok"`
	Roots int  `json:"roots"`
}

// newControlHandler answers the private control listener. Without a secret
// nothing authenticates, so every request is unauthorized.
func newControlHandler(secret string, roots []*controlRoot, link *serviceLink) http.Handler {
	api := newControlAPI(roots)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !controlAuthorized(r, secret) {
			writeControlJSON(w, http.StatusUnauthorized, controlError{Error: "unauthorized"})
			return
		}
		if !controlEpochAuthorized(w, r, link) {
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			writeControlJSON(w, http.StatusOK, controlHealth{OK: true, Roots: len(roots)})
			return
		}
		if route := api.serviceRoute(r, link); route != nil {
			route(w, r)
			return
		}
		if route := api.route(r); route != nil {
			route(w, r)
			return
		}
		writeControlJSON(w, http.StatusNotFound, controlError{Error: "not found"})
	})
}

// controlEpochAuthorized is the epoch rule: a request carrying X-Trial-Epoch
// comes from the attached service, whose X-Trial-Peer-Epoch names this
// process's epoch and whose X-Trial-Epoch names its own. A request without it
// (the supervisor, the adapter) is accepted as before.
func controlEpochAuthorized(w http.ResponseWriter, r *http.Request, link *serviceLink) bool {
	epoch := r.Header.Get("X-Trial-Epoch")
	if epoch == "" {
		return true
	}
	if link != nil && r.Header.Get("X-Trial-Peer-Epoch") == link.selfEpoch && epoch == link.currentPeer().Epoch {
		return true
	}
	answer := orderedObject{{Key: "error", Value: "epoch"}}
	if link != nil {
		answer = append(answer, kv{Key: "epoch", Value: link.selfEpoch})
	}
	writeControlJSON(w, http.StatusConflict, answer)
	return false
}

// serviceRoute routes the paths the JS service and the supervisor use: the
// service's own address, the documents it has reconciled, and the list of the
// ones it has not. Without a service every one of them is not found.
func (a *controlAPI) serviceRoute(r *http.Request, link *serviceLink) http.HandlerFunc {
	switch {
	case r.URL.Path == "/v1/peer" && r.Method == http.MethodPost:
		return func(w http.ResponseWriter, r *http.Request) { a.servePeer(w, r, link) }
	case r.URL.Path == "/v1/reconciled" && r.Method == http.MethodPost:
		return func(w http.ResponseWriter, r *http.Request) { a.serveReconciled(w, r, link) }
	case r.URL.Path == "/v1/unconfirmed" && r.Method == http.MethodGet:
		return func(w http.ResponseWriter, r *http.Request) { a.serveUnconfirmed(w, r, link) }
	}
	return nil
}

// noService answers a route that needs the JS service when none is attached.
func noService(w http.ResponseWriter, link *serviceLink) bool {
	if link != nil {
		return false
	}
	writeControlJSON(w, http.StatusNotFound, controlError{Error: "no service is attached"})
	return true
}

// servePeer is POST /v1/peer: the service's address after the supervisor
// restarted it.
func (a *controlAPI) servePeer(w http.ResponseWriter, r *http.Request, link *serviceLink) {
	if noService(w, link) {
		return
	}
	var peer servicePeer
	if !decodeControlJSON(w, r, &peer) {
		return
	}
	if !validServicePeer(peer) {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "the service needs a http://127.0.0.1: url, a 64-hex secret and a 16-hex epoch"})
		return
	}
	link.setPeer(peer)
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "ok", Value: true}})
}

// serveReconciled is POST /v1/reconciled: the service confirmed a document Go
// wrote without it, so writes to it are allowed again.
func (a *controlAPI) serveReconciled(w http.ResponseWriter, r *http.Request, link *serviceLink) {
	if noService(w, link) {
		return
	}
	var body struct {
		Root string `json:"root"`
		Name string `json:"name"`
	}
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	filePath, err := resolveWritePath(root.registry, body.Name)
	if err != nil {
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	link.unconfirmed.lift(filePath)
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "ok", Value: true}})
}

// serveUnconfirmed is GET /v1/unconfirmed: every page document Go wrote without
// the service confirming it.
func (a *controlAPI) serveUnconfirmed(w http.ResponseWriter, r *http.Request, link *serviceLink) {
	if noService(w, link) {
		return
	}
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "documents", Value: link.unconfirmed.list()}})
}

func controlAuthorized(r *http.Request, secret string) bool {
	if secret == "" {
		return false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, controlScheme) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header[len(controlScheme):]), []byte(secret)) == 1
}

func writeControlJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
