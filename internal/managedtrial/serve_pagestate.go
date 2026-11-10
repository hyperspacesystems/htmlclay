//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"

	"github.com/panphora/htmlclay/internal/specwire"
)

// servePageState is Local's GET /_/page-state: the merge state stored for the
// exact document bytes a client accepted, conditional on the same stamp
// /_/meta answered.
func (s *rootServer) servePageState(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	refuse := func(status int, code, etag string) error {
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		body := orderedObject{
			{Key: "error", Value: "page state unavailable"},
			{Key: "code", Value: code},
		}
		if etag != "" {
			body = append(body, kv{Key: "etag", Value: etag})
		}
		encoded, err := jsonStringify(body)
		if err != nil {
			return err
		}
		sendJSON(w, r, status, encoded)
		return nil
	}

	href := documentURLHeader(r)
	ifMatch := r.Header.Get("If-Match")
	if href == "" {
		return refuse(http.StatusNotFound, "no-page-state", "")
	}
	if strings.TrimSpace(ifMatch) == "" {
		return refuse(http.StatusPreconditionRequired, "precondition-required", "")
	}
	name := resolveResourceFromHref(href)
	filePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		return refuse(http.StatusNotFound, "no-page-state", "")
	}

	// Read, compare and select under the file's own lock, the one a save takes,
	// so a save landing in between cannot pair one revision's stamp with
	// another's state.
	unlock := fileLocks.lock(filePath)
	defer unlock()

	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		return refuse(http.StatusNotFound, "no-page-state", "")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return refuse(http.StatusNotFound, "no-page-state", "")
	}
	logical, err := logicalBytes(s.baseDir, name, raw)
	if err != nil {
		return err
	}
	etag := specwire.Etag(logical)
	field := strings.TrimSpace(ifMatch)
	if field == "*" || strings.Contains(field, ",") || !specwire.IfMatchSatisfied(field, logical) {
		return refuse(http.StatusPreconditionFailed, "etag-mismatch", etag)
	}

	record, err := readPageRecord(s.baseDir, name, pageRev(string(raw)))
	if err != nil {
		return err
	}
	if record == nil {
		return refuse(http.StatusNotFound, "no-page-state", etag)
	}
	w.Header().Set("ETag", etag)
	sum := sha256.Sum256([]byte(record.Tag))
	body := orderedObject{
		{Key: "version", Value: 1},
		{Key: "documentId", Value: nil},
		{Key: "generation", Value: record.Generation},
		{Key: "etag", Value: etag},
		{Key: "stateHash", Value: hex.EncodeToString(sum[:])},
		{Key: "tag", Value: record.Tag},
	}
	encoded, err := jsonStringify(body)
	if err != nil {
		return err
	}
	sendJSON(w, r, http.StatusOK, encoded)
	return nil
}
