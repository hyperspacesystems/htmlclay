//go:build managedtrial

package managedtrial

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var systemRoutes = map[string]bool{
	"save":            true,
	"live-sync":       true,
	"sync":            true,
	"wire":            true,
	"data-loss":       true,
	"api":             true,
	"meta":            true,
	"upload":          true,
	"uploads":         true,
	"versions":        true,
	"version":         true,
	"restore":         true,
	"clayjs":          true,
	"presence":        true,
	"page-state":      true,
	"reveal":          true,
	"versions-folder": true,
	"export":          true,
	"code-editor":     true,
	"duplicate":       true,
}

var loopbackIPv4 = regexp.MustCompile(`^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

func isLoopbackHostname(hostname string) bool {
	if hostname == "" {
		return false
	}
	bare := hostname
	if strings.HasPrefix(bare, "[") && strings.HasSuffix(bare, "]") {
		bare = bare[1 : len(bare)-1]
	}
	bare = strings.ToLower(bare)
	return bare == "localhost" || bare == "::1" || bare == "0:0:0:0:0:0:0:1" || loopbackIPv4.MatchString(bare)
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHostname(parsed.Hostname())
}

func originPort(origin string) (int, bool) {
	parsed, err := url.Parse(origin)
	if err != nil || !parsed.IsAbs() {
		return 0, false
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil {
			return 0, false
		}
		return value, true
	}
	if parsed.Scheme == "https" {
		return 443, true
	}
	return 80, true
}

func isOwnOrigin(origin string, localPort int, rootPort int) bool {
	port, ok := originPort(origin)
	if !ok {
		return false
	}
	return port == rootPort || (localPort != 0 && port == localPort)
}

func isLoopbackHostHeader(hostHeader string) bool {
	if hostHeader == "" {
		return false
	}
	parsed, err := url.Parse("http://" + hostHeader)
	if err != nil {
		return false
	}
	return isLoopbackHostname(parsed.Hostname())
}

func stripSystemRouteMarker(path string) string {
	if strings.HasPrefix(path, "/_/") {
		return path[2:]
	}
	return path
}

func systemRouteSegment(path string) string {
	rest := strings.TrimPrefix(path, "/")
	if cut := strings.IndexAny(rest, "/?#"); cut >= 0 {
		rest = rest[:cut]
	}
	return rest
}

func localPort(r *http.Request) int {
	address, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return 0
	}
	tcp, ok := address.(*net.TCPAddr)
	if !ok {
		return 0
	}
	return tcp.Port
}

func gate(w http.ResponseWriter, r *http.Request, rootPort int) (string, bool, bool) {
	if !isLoopbackHostHeader(r.Host) {
		sendText(w, r, http.StatusForbidden, "Invalid Host header")
		return "", false, false
	}

	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if !originAllowed(w, r, rootPort) {
			return "", false, false
		}
	}

	path := r.URL.Path
	marked := false
	if strings.HasPrefix(path, "/_/") {
		path = stripSystemRouteMarker(path)
		if !systemRoutes[systemRouteSegment(path)] {
			sendText(w, r, http.StatusNotFound, "File not found")
			return "", false, false
		}
		marked = true
	}

	setSessionCookies(w)
	return path, marked, true
}

func originAllowed(w http.ResponseWriter, r *http.Request, rootPort int) bool {
	sites := r.Header.Values("Sec-Fetch-Site")
	if len(sites) > 0 && sites[0] != "same-origin" && sites[0] != "none" {
		refuseOrigin(w, r)
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	origin := origins[0]
	if origin == "null" || !isLoopbackOrigin(origin) {
		refuseOrigin(w, r)
		return false
	}
	if !isOwnOrigin(origin, localPort(r), rootPort) {
		refuseOrigin(w, r)
		return false
	}
	return true
}

func refuseOrigin(w http.ResponseWriter, r *http.Request) {
	refusal := orderedObject{
		{Key: "msg", Value: "Cross-origin requests are not allowed."},
		{Key: "msgType", Value: "error"},
	}
	requestPath := r.RequestURI
	if cut := strings.IndexByte(requestPath, '?'); cut >= 0 {
		requestPath = requestPath[:cut]
	}
	if requestPath == "/_/sync/data" {
		refusal = append(refusal, kv{Key: "code", Value: "forbidden"})
	}
	body, err := jsonStringify(refusal)
	if err != nil {
		sendText(w, r, http.StatusInternalServerError, "Internal server error")
		return
	}
	sendJSON(w, r, http.StatusForbidden, body)
}

func setSessionCookies(w http.ResponseWriter) {
	w.Header().Add("Set-Cookie", "isAdminOfCurrentResource=true; Path=/; SameSite=Lax")
	w.Header().Add("Set-Cookie", "isLoggedIn=true; Path=/; SameSite=Lax")
}

func bareAllowed(path string, marked bool) bool {
	if marked {
		return true
	}
	switch path {
	case "/sync/data", "/presence", "/sync/worker.js", "/sync/data-worker.js", "/meta":
		return false
	}
	if path == "/api" || strings.HasPrefix(path, "/api/") {
		return false
	}
	for _, prefix := range []string{"/uploads/", "/versions/", "/version/", "/restore/"} {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}
