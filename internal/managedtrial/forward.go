//go:build managedtrial

package managedtrial

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"unicode/utf8"
)

// canonicalKey carries the canonical path Go resolved for a forwarded data API
// request to the proxy's rewrite.
type canonicalKey struct{}

// newServiceProxy forwards a room-owning public request to the service's
// /v1/route/* (protocol v1). Every client header named X-Trial-* and the
// client's Authorization are dropped; Go sets the bearer, both epochs, the
// root, the public origin and host, and for a data API request the canonical
// path it resolved. Answers stream with immediate flushes, and the upstream
// request ends with the client's.
func newServiceProxy(link *serviceLink, rootID string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			peer := link.currentPeer()
			target, _ := url.Parse(peer.URL)
			for name := range pr.Out.Header {
				if strings.HasPrefix(http.CanonicalHeaderKey(name), "X-Trial-") {
					pr.Out.Header.Del(name)
				}
			}
			pr.Out.Header.Del("Authorization")
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.URL.Path = "/v1/route" + pr.In.URL.Path
			pr.Out.URL.RawPath = ""
			if pr.In.URL.RawPath != "" {
				pr.Out.URL.RawPath = "/v1/route" + pr.In.URL.RawPath
			}
			pr.Out.Host = target.Host
			pr.Out.Header.Set("Authorization", controlScheme+peer.Secret)
			pr.Out.Header.Set("X-Trial-Epoch", link.selfEpoch)
			pr.Out.Header.Set("X-Trial-Peer-Epoch", peer.Epoch)
			pr.Out.Header.Set("X-Trial-Root", rootID)
			pr.Out.Header.Set("X-Trial-Public-Origin", "http://"+pr.In.Host)
			pr.Out.Header.Set("X-Trial-Public-Host", pr.In.Host)
			if canonical, ok := pr.In.Context().Value(canonicalKey{}).(string); ok {
				pr.Out.Header.Set("X-Trial-Canonical", canonical)
			}
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeServiceUnavailable(w, r, "service-unavailable")
		},
	}
}

// writeServiceUnavailable is the recoverable 503 a forwarded route answers when
// the service cannot answer it.
func writeServiceUnavailable(w http.ResponseWriter, r *http.Request, reason string) {
	refusal := retryableRefusal(reason)
	_ = writeRefusal(w, r, &refusal)
}

// forwardsToService says whether a public request belongs to a room, and for a
// data API request names the document it reads or writes.
func (s *rootServer) forwardsToService(r *http.Request, urlPath string, marked bool) (bool, string) {
	getOrHead := r.Method == http.MethodGet || r.Method == http.MethodHead
	post := r.Method == http.MethodPost
	systemPath := strings.TrimSuffix(urlPath, "/")
	switch {
	case getOrHead && (urlPath == "/sync" || urlPath == "/live-sync/stream"):
		return true, ""
	case post && relayAddress(urlPath) && bareAllowed(urlPath, marked):
		return true, ""
	case marked && (getOrHead || post) && systemPath == "/sync/data":
		return true, ""
	case marked && post && systemPath == "/presence":
		return true, ""
	}
	if !getOrHead && !post {
		return false, ""
	}
	if marked {
		if name := apiDocumentName(urlPath); name != "" {
			return true, name
		}
		if systemPath == "/api" {
			return true, "index.html"
		}
		return false, ""
	}
	if readDataQuery(r.RequestURI).present {
		if name, ok := plainDataName(urlPath); ok {
			return true, name
		}
	}
	return false, ""
}

// forwardToService answers a room-owning request through the service. A data
// API request is forwarded only when its document has a live page store;
// otherwise it stays on the native route (answered false).
func (s *rootServer) forwardToService(w http.ResponseWriter, r *http.Request, name string) bool {
	if name != "" {
		filePath, err := s.commit.resolveWrite(name)
		if err != nil {
			return false
		}
		store, err := s.composedHasPageStore(name, filePath)
		if err != nil || !store {
			return false
		}
		if r.Method == http.MethodPost && s.service.barred(filePath) {
			writeServiceUnavailable(w, r, "unconfirmed")
			return true
		}
		r = r.WithContext(context.WithValue(r.Context(), canonicalKey{}, filePath))
	}
	if s.service.currentPeer().URL == "" {
		writeServiceUnavailable(w, r, "service-unavailable")
		return true
	}
	s.serviceProxy.ServeHTTP(w, r)
	return true
}

// composedHasPageStore reads one document under its lock and tests its composed
// form, the document Local's hasPageStore is given.
func (s *rootServer) composedHasPageStore(name, filePath string) (bool, error) {
	unlock := s.commit.locks.lock(filePath)
	defer unlock()
	state, err := readFile(filePath)
	if err != nil || state == nil || !utf8.Valid(state.Bytes) {
		return false, err
	}
	logical, err := logicalDocument(s.baseDir, name, string(state.Bytes))
	if err != nil {
		return false, err
	}
	return hasPageStore(logical), nil
}
