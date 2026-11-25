package wire

import (
	"net/http"
	"strings"
)

// NoStoreJSON marks a response as JSON that no cache may keep.
func NoStoreJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
}

// IsJSONContentType reports whether a Content-Type's media type is
// application/json or ends in +json, after cutting parameters and lowercasing.
func IsJSONContentType(ct string) bool {
	mediaType, _, _ := strings.Cut(ct, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
