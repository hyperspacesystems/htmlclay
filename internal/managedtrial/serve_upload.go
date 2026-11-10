//go:build managedtrial

package managedtrial

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// uploadsFolder is Local's UPLOADS_FOLDER: every document upload lands in this
// root's `uploads/assets-<stem>/`, never beside the document, so a synced folder
// holds identical bytes on both sides and renaming or moving the document never
// strands its files.
const uploadsFolder = "uploads"

// uploadCodes is the spec's §3 code for a status, so a client branches on the
// reason instead of pattern-matching the message.
var uploadCodes = map[int]string{
	http.StatusBadRequest:            "bad-request",
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not-found",
	http.StatusConflict:              "conflict",
	http.StatusRequestEntityTooLarge: "too-large",
	http.StatusUnsupportedMediaType:  "unsupported-type",
}

var (
	uploadPageExtension = regexp.MustCompile(`(?i)\.(html?|htmlclay|xhtml)$`)
	uploadSlugChars     = regexp.MustCompile(`[^a-z0-9_-]+`)
	uploadSlugDashes    = regexp.MustCompile(`-{2,}`)
	uploadUnsafeChars   = regexp.MustCompile("[<>:\"|?*\x01-\x1f]")
)

// documentActionSegments are the page menu's per-document actions. This host
// holds no Electron access, so Local's `ctx.documentActions` is absent here and
// every one of them answers its refusal.
var documentActionSegments = map[string]bool{
	"/reveal":          true,
	"/versions-folder": true,
	"/export":          true,
	"/code-editor":     true,
	"/duplicate":       true,
}

// uploadFolderHeader is Local's uploadFolderHeader: an upload into a folder with
// no document (the workspace window) names that folder instead.
func uploadFolderHeader(r *http.Request) (string, bool, error) {
	values := r.Header.Values("Upload-Folder")
	if len(values) == 0 {
		return "", false, nil
	}
	rel, err := url.PathUnescape(values[0])
	if err != nil {
		return "", true, &PathError{Status: http.StatusBadRequest, Message: "Invalid upload folder"}
	}
	if rel == "" {
		return "", true, nil
	}
	if _, err := validateSegments(rel); err != nil {
		return "", true, err
	}
	return rel, true, nil
}

// normalizeUploadName is the one cleanup a client-supplied filename gets. The
// refusal and the stored name both read this result, so the type that is checked
// is the type that is stored.
func normalizeUploadName(fileName string) string {
	if fileName == "" {
		fileName = "file"
	}
	base := fileName
	if at := strings.LastIndexByte(base, '/'); at >= 0 {
		base = base[at+1:]
	}
	base = strings.ReplaceAll(base, "\x00", "")
	base = uploadUnsafeChars.ReplaceAllString(base, "")
	return strings.TrimLeft(base, ".")
}

// splitUploadName splits a client-supplied filename into the parts the stored
// name is built from. A leading dot is stripped rather than preserved.
func splitUploadName(fileName string) (string, string) {
	base := normalizeUploadName(fileName)
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		if base == "" {
			base = "file"
		}
		return base, ""
	}
	stem := base[:dot]
	if stem == "" {
		stem = "file"
	}
	return stem, base[dot:]
}

// assetsFolderName is the folder a document's uploads land in, slugged to the
// alphabet a synced folder can hold: `My Board.v2.html` -> `assets-my-board-v2`.
func assetsFolderName(docRelPath string) string {
	stem := uploadPageExtension.ReplaceAllString(slashBase(docRelPath), "")
	slug := uploadSlugChars.ReplaceAllString(strings.ToLower(stem), "-")
	slug = uploadSlugDashes.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "document"
	}
	return "assets-" + slug
}

// slashBase is Node's posix path.basename: only `/` separates.
func slashBase(p string) string {
	if at := strings.LastIndexByte(p, '/'); at >= 0 {
		return p[at+1:]
	}
	return p
}

// strictSegment is Local's strictSegment: everything outside the unreserved set
// escaped, so the link is one unbroken run a zip exporter reads whole.
func strictSegment(segment string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~"
	var b strings.Builder
	b.Grow(len(segment))
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte("0123456789ABCDEF"[c>>4])
		b.WriteByte("0123456789ABCDEF"[c&0x0f])
	}
	return b.String()
}

// storedUpload is what storeUpload answers: the name the bytes are stored under,
// and whether this call is the one that wrote them.
type storedUpload struct {
	name  string
	bytes int
	wrote bool
}

// storeUpload is Local's storeUpload: content-hash naming, which is what removes
// the race rather than a lock. Two uploads of different bytes get different
// names and never contend; two uploads of the same bytes converge on one file,
// and whichever loses the exclusive create reads back what the winner wrote.
func storeUpload(registry *consentRegistry, dirRel, fileName string, content []byte) (storedUpload, error) {
	stem, ext := splitUploadName(fileName)
	digest := sha256.Sum256(content)
	sum := hex.EncodeToString(digest[:])
	for length := 6; length <= 32; length += 2 {
		name := stem + "-" + sum[:length] + ext
		rel := name
		if dirRel != "" {
			rel = dirRel + "/" + name
		}
		if _, err := validateSegments(rel); err != nil {
			return storedUpload{}, err
		}
		abs, err := resolveWritePath(registry, rel)
		if err != nil {
			return storedUpload{}, err
		}
		handle, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if !errors.Is(err, os.ErrExist) {
				return storedUpload{}, err
			}
			existing, readErr := os.ReadFile(abs)
			if readErr != nil {
				return storedUpload{}, readErr
			}
			if bytes.Equal(existing, content) {
				return storedUpload{name: name, bytes: len(content)}, nil
			}
			continue
		}
		_, writeErr := handle.Write(content)
		closeErr := handle.Close()
		if writeErr != nil {
			return storedUpload{}, writeErr
		}
		if closeErr != nil {
			return storedUpload{}, closeErr
		}
		return storedUpload{name: name, bytes: len(content), wrote: true}, nil
	}
	return storedUpload{}, &PathError{Status: http.StatusConflict, Message: "Could not find a free name for that file."}
}

// uploadAnswer writes one JSON answer of the upload route.
func uploadAnswer(w http.ResponseWriter, r *http.Request, status int, body orderedObject) error {
	encoded, err := jsonStringify(body)
	if err != nil {
		return err
	}
	sendJSON(w, r, status, encoded)
	return nil
}

// uploadRefusal is the route's error answer: the message, the error marker and
// the status's code.
func uploadRefusal(w http.ResponseWriter, r *http.Request, status int, message string) error {
	code := uploadCodes[status]
	if code == "" {
		code = "error"
	}
	return uploadAnswer(w, r, status, orderedObject{
		{Key: "msg", Value: message},
		{Key: "msgType", Value: "error"},
		{Key: "code", Value: code},
	})
}

// uploadCaught is the route's catch: an error that carries a status becomes the
// JSON answer for that status, and one that does not reaches the error handler.
func uploadCaught(w http.ResponseWriter, r *http.Request, err error) error {
	var pathErr *PathError
	if errors.As(err, &pathErr) && pathErr.Status != 0 {
		return uploadRefusal(w, r, pathErr.Status, pathErr.Message)
	}
	return err
}

// serveUpload is Local's POST /_/upload: store a file in this root's uploads
// folder instead of embedding it (spec §9). The upload is authorized by the
// document existing and being writable, exactly as a save is.
func (s *rootServer) serveUpload(w http.ResponseWriter, r *http.Request) error {
	href := documentURLHeader(r)
	folderRel := ""
	folderNamed := false
	if href == "" {
		rel, present, err := uploadFolderHeader(r)
		if err != nil {
			return uploadCaught(w, r, err)
		}
		folderRel, folderNamed = rel, present
	}
	if href == "" && !folderNamed {
		return uploadRefusal(w, r, http.StatusBadRequest, "Document-URL header required.")
	}

	docName := ""
	docPath := ""
	if href != "" {
		docName = resolveResourceFromHref(href)
		resolved, err := resolveWriteTarget(s.registry, docName)
		if err != nil {
			return uploadCaught(w, r, err)
		}
		docPath = resolved
		info, statErr := os.Stat(docPath)
		if statErr != nil || !info.Mode().IsRegular() {
			return uploadRefusal(w, r, http.StatusNotFound, "That document does not exist.")
		}
	} else {
		folderPath, err := resolveWritePath(s.registry, folderRel)
		if err != nil {
			return uploadCaught(w, r, err)
		}
		info, statErr := os.Stat(folderPath)
		if statErr != nil || !info.IsDir() {
			return uploadRefusal(w, r, http.StatusNotFound, "That folder does not exist.")
		}
	}

	part, err := readUploadPart(r.Body, r.Header.Get("Content-Type"), uploadMaxBytes)
	_, _ = io.Copy(io.Discard, r.Body)
	if err != nil {
		return uploadCaught(w, r, err)
	}
	if part == nil {
		return uploadRefusal(w, r, http.StatusBadRequest, "No file to upload.")
	}
	if refusedUploadPath(normalizeUploadName(part.filename)) {
		return uploadRefusal(w, r, http.StatusUnsupportedMediaType, "That kind of file cannot be uploaded.")
	}

	dirRel := folderRel
	if docName != "" {
		dirRel = uploadsFolder + "/" + assetsFolderName(docName)
	}
	dirPath, err := resolveWritePath(s.registry, dirRel)
	if err != nil {
		return uploadCaught(w, r, err)
	}
	if err := os.MkdirAll(dirPath, 0o777); err != nil {
		return uploadCaught(w, r, err)
	}
	stored, err := storeUpload(s.registry, dirRel, part.filename, part.content)
	if err != nil {
		return uploadCaught(w, r, err)
	}

	// A host path, resolved by GET /_/uploads/ on whatever port this root is
	// served from, and percent-encoded per segment while the stored name keeps
	// its own characters: a raw space breaks srcset, where a space separates
	// candidates. A document upload names its file inside `uploads/`; a folder
	// upload names the file where it sits, which the static route already serves.
	link := ""
	if docName != "" {
		link = "/_/uploads/" + strictSegment(slashBase(dirRel)) + "/" + strictSegment(stored.name)
	} else {
		if folderRel != "" {
			segments := strings.Split(folderRel, "/")
			for i, segment := range segments {
				segments[i] = strictSegment(segment)
			}
			link = strings.Join(segments, "/") + "/"
		}
		link = "/" + link + strictSegment(stored.name)
	}

	return uploadAnswer(w, r, http.StatusOK, orderedObject{
		{Key: "msg", Value: "Uploaded"},
		{Key: "msgType", Value: "success"},
		{Key: "uploads", Value: []orderedObject{{
			{Key: "name", Value: stored.name},
			{Key: "url", Value: link},
			{Key: "bytes", Value: stored.bytes},
			{Key: "access", Value: uploadAccess},
		}}},
	})
}

// serveDocumentAction is one of Local's per-document page menu routes. The
// request names its document exactly as /_/meta does, and only then does the
// route discover there is no action to run: this host holds no Electron access,
// so every segment answers Local's refusal.
func (s *rootServer) serveDocumentAction(w http.ResponseWriter, r *http.Request) error {
	refuse := func(status int, message string) error {
		return uploadAnswer(w, r, status, orderedObject{
			{Key: "ok", Value: false},
			{Key: "message", Value: message},
		})
	}
	href := documentURLHeader(r)
	if href == "" {
		return refuse(http.StatusBadRequest, "No document named.")
	}
	name := resolveResourceFromHref(href)
	filePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		return refuse(http.StatusNotFound, "No document named.")
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		return refuse(http.StatusNotFound, "No document named.")
	}
	return refuse(http.StatusNotFound, "These actions are not available.")
}

// serveUploadFile is Local's GET /_/uploads/<folder>/<name>: the host path every
// upload answers (spec §9). Files only, never a listing, and always served as an
// upload: nosniff, an explicit type, and a download for anything a browser would
// run.
func (s *rootServer) serveUploadFile(w http.ResponseWriter, r *http.Request) error {
	decoded, err := decodeOnce(r.URL.EscapedPath())
	if err != nil {
		return err
	}
	rel := uploadsFolder + "/" + strings.TrimPrefix(decoded, "/_/uploads/")
	if _, err := validateSegments(rel); err != nil {
		return err
	}
	realPath, err := resolveReadPath(s.registry, rel)
	if err != nil {
		return err
	}
	// The root's consented links are honoured elsewhere, but an upload is only
	// ever a file inside the real `uploads/`: a link there may point anywhere the
	// folder's owner consented to, which is not somewhere this route serves from.
	uploadsReal, linkErr := filepath.EvalSymlinks(filepath.Join(s.baseDir, uploadsFolder))
	if linkErr != nil || !isContained(uploadsReal, realPath) {
		return fileNotFoundError()
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fileNotFoundError()
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	if svgPathPattern.MatchString(realPath) || refusedUploadPath(realPath) {
		w.Header().Set("Content-Disposition", "attachment")
	}
	contentType := binaryContentType
	if filepath.Ext(realPath) != "" {
		contentType = mimeTypeFor(realPath)
	}
	sendFile(w, r, realPath, info, contentType)
	return nil
}
