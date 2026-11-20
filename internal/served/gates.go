package served

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var systemRoutes = map[string]bool{
	"save":              true,
	"live-sync":         true,
	"sync":              true,
	"wire":              true,
	"helper-setup":      true,
	"data-loss":         true,
	"unsaved":           true,
	"api":               true,
	"meta":              true,
	"upload":            true,
	"uploads":           true,
	"versions":          true,
	"version":           true,
	"restore":           true,
	"clayjs":            true,
	"presence":          true,
	"blockeditor-state": true,
	"reveal":            true,
	"versions-folder":   true,
	"export":            true,
	"code-editor":       true,
	"duplicate":         true,
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

// originPolicy is what one root's gate admits. A served folder has a name: only
// its exact Host, and for writes its exact Origin, are its own, so a sibling
// folder on the same port is refused even though browsers treat every
// *.htmlclay.localhost address as the same site. The trial binary serves each
// root on its own port, and admits any loopback host on that port.
type originPolicy struct {
	host     string
	origin   string
	rootPort int
	byPort   bool
}

// namePolicy is a served folder's gate: publicHost is "<name>.htmlclay.localhost:<port>".
func namePolicy(publicHost string) originPolicy {
	host := strings.ToLower(publicHost)
	return originPolicy{host: host, origin: "http://" + host}
}

// portPolicy is the trial's gate: the root's own port, or the connection's.
func portPolicy(rootPort int) originPolicy {
	return originPolicy{rootPort: rootPort, byPort: true}
}

func (p originPolicy) hostAllowed(hostHeader string) bool {
	if p.byPort {
		return isLoopbackHostHeader(hostHeader)
	}
	return strings.ToLower(hostHeader) == p.host
}

func (p originPolicy) ownOrigin(r *http.Request, origin string) bool {
	if p.byPort {
		return isLoopbackOrigin(origin) && isOwnOrigin(origin, localPort(r), p.rootPort)
	}
	return strings.ToLower(origin) == p.origin
}

// sameOriginFetch reports whether a browser marked the request as coming from
// this origin or from no page at all. A request without the header is a script
// or an older client, which the Host check already bounds.
func sameOriginFetch(r *http.Request) bool {
	sites := r.Header.Values("Sec-Fetch-Site")
	return len(sites) == 0 || sites[0] == "same-origin" || sites[0] == "none"
}

// admittedAsPage is how the gate let a write through. A browser attests Origin
// on every write and Sec-Fetch-Site where it knows the header, a page script can
// set or remove neither, and the gate has already held both to this origin; a
// write it admitted with neither came from a program. It is the classifier the
// loose host's wire and the router's bare-host data API use.
func admittedAsPage(r *http.Request) bool {
	return len(r.Header.Values("Sec-Fetch-Site")) != 0 || len(r.Header.Values("Origin")) != 0
}

func gate(w http.ResponseWriter, r *http.Request, p originPolicy) (string, bool, bool) {
	if !p.hostAllowed(r.Host) {
		sendText(w, r, http.StatusForbidden, "Invalid Host header")
		return "", false, false
	}

	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if !originAllowed(w, r, p) {
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
		// A sibling folder is same-site, so it could otherwise read this
		// folder's data, versions and live stream with a plain GET.
		if !p.byPort && !sameOriginFetch(r) {
			refuseOrigin(w, r)
			return "", false, false
		}
	}

	setSessionCookies(w)
	return path, marked, true
}

func originAllowed(w http.ResponseWriter, r *http.Request, p originPolicy) bool {
	if !sameOriginFetch(r) {
		refuseOrigin(w, r)
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	origin := origins[0]
	if origin == "null" || !p.ownOrigin(r, origin) {
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
