package served

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// DashboardAPI answers the dashboard's workspace API through the service's
// /v1/shell/api/... (service/shell.cjs), for a request the dashboard already
// admitted (internal/dashboard). Every client header named X-Trial-* and the
// client's Authorization are dropped; Go sets the bearer, both epochs and the
// dashboard's origin from the request's Host. The event stream flushes as it
// goes. With no running service the answer is the recoverable 503 with
// Retry-After: 1.
func (h *Host) DashboardAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc := h.cfg.service.active()
		if svc == nil || svc.currentPeer().URL == "" {
			writeServiceUnavailable(w, r, "service-unavailable")
			return
		}
		peer := svc.currentPeer()
		target, err := url.Parse(peer.URL)
		if err != nil {
			writeServiceUnavailable(w, r, "service-unavailable")
			return
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				for name := range pr.Out.Header {
					if strings.HasPrefix(http.CanonicalHeaderKey(name), "X-Trial-") {
						pr.Out.Header.Del(name)
					}
				}
				pr.Out.Header.Del("Authorization")
				pr.Out.Header.Del("Cookie")
				pr.Out.URL.Scheme = target.Scheme
				pr.Out.URL.Host = target.Host
				pr.Out.URL.Path = "/v1/shell" + pr.In.URL.Path
				pr.Out.URL.RawPath = ""
				if pr.In.URL.RawPath != "" {
					pr.Out.URL.RawPath = "/v1/shell" + pr.In.URL.RawPath
				}
				pr.Out.Host = target.Host
				pr.Out.Header.Set("Authorization", controlScheme+peer.Secret)
				pr.Out.Header.Set("X-Trial-Epoch", svc.selfEpoch)
				pr.Out.Header.Set("X-Trial-Peer-Epoch", peer.Epoch)
				pr.Out.Header.Set("X-Trial-Public-Origin", "http://"+pr.In.Host)
			},
			FlushInterval: -1,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				writeServiceUnavailable(w, r, "service-unavailable")
			},
		}
		proxy.ServeHTTP(w, r)
	})
}
