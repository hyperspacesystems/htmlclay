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
func newControlHandler(secret string, roots []*controlRoot) http.Handler {
	api := newControlAPI(roots)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !controlAuthorized(r, secret) {
			writeControlJSON(w, http.StatusUnauthorized, controlError{Error: "unauthorized"})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			writeControlJSON(w, http.StatusOK, controlHealth{OK: true, Roots: len(roots)})
			return
		}
		if route := api.route(r); route != nil {
			route(w, r)
			return
		}
		writeControlJSON(w, http.StatusNotFound, controlError{Error: "not found"})
	})
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
