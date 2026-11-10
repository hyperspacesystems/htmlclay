//go:build managedtrial

package managedtrial

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	poweredByValue    = "Express"
	htmlContentType   = "text/html; charset=utf-8"
	jsonContentType   = "application/json; charset=utf-8"
	binaryContentType = "application/octet-stream"
	emptyEntityETag   = `W/"0-2jmj7l5rSw0yVb/vlWAYkK/YBwk"`
	etagHashPrefix    = 27
)

const (
	rangeInvalid       = -2
	rangeUnsatisfiable = -1
)

var (
	noCacheDirective = regexp.MustCompile(`(?:^|,)\s*?no-cache\s*?(?:,|$)`)
	bytesRangeSpec   = regexp.MustCompile(`^ *bytes=`)
)

func poweredBy(w http.ResponseWriter) {
	w.Header().Set("X-Powered-By", poweredByValue)
}

func sendText(w http.ResponseWriter, r *http.Request, status int, body string) {
	poweredBy(w)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", htmlContentType)
	}
	sendEntity(w, r, status, []byte(body))
}

func sendBytes(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	poweredBy(w)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", binaryContentType)
	}
	sendEntity(w, r, status, body)
}

func sendJSON(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	poweredBy(w)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", jsonContentType)
	}
	sendEntity(w, r, status, body)
}

func sendEntity(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if h.Get("Etag") == "" {
		h.Set("Etag", weakEntityETag(body))
	}
	if requestFresh(r, h, status) {
		status = http.StatusNotModified
	}
	if status == http.StatusNoContent || status == http.StatusNotModified {
		h.Del("Content-Type")
		h.Del("Content-Length")
		h.Del("Transfer-Encoding")
		body = nil
	}
	w.WriteHeader(status)
	if r.Method == http.MethodHead || len(body) == 0 {
		return
	}
	_, _ = w.Write(body)
}

func weakEntityETag(body []byte) string {
	if len(body) == 0 {
		return emptyEntityETag
	}
	sum := sha1.Sum(body)
	hash := base64.StdEncoding.EncodeToString(sum[:])
	if len(hash) > etagHashPrefix {
		hash = hash[:etagHashPrefix]
	}
	return fmt.Sprintf(`W/"%x-%s"`, len(body), hash)
}

func weakStatETag(info os.FileInfo) string {
	return fmt.Sprintf(`W/"%x-%x"`, info.Size(), info.ModTime().UnixMilli())
}

func requestFresh(r *http.Request, h http.Header, status int) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !((status >= 200 && status < 300) || status == http.StatusNotModified) {
		return false
	}
	return fresh(r, h)
}

func fresh(r *http.Request, h http.Header) bool {
	modifiedSince := r.Header.Get("If-Modified-Since")
	noneMatch := r.Header.Get("If-None-Match")
	if modifiedSince == "" && noneMatch == "" {
		return false
	}
	if cacheControl := r.Header.Get("Cache-Control"); cacheControl != "" && noCacheDirective.MatchString(cacheControl) {
		return false
	}
	if noneMatch != "" && noneMatch != "*" {
		etag := h.Get("Etag")
		if etag == "" {
			return false
		}
		stale := true
		for _, match := range tokenList(noneMatch) {
			if match == etag || match == "W/"+etag || "W/"+match == etag {
				stale = false
				break
			}
		}
		if stale {
			return false
		}
	}
	if modifiedSince != "" {
		lastModified := h.Get("Last-Modified")
		lm, okLastModified := parseHTTPDate(lastModified)
		ms, okModifiedSince := parseHTTPDate(modifiedSince)
		if lastModified == "" || !okLastModified || !okModifiedSince || lm.After(ms) {
			return false
		}
	}
	return true
}

func preconditionFailed(r *http.Request, h http.Header) bool {
	if match := r.Header.Get("If-Match"); match != "" {
		etag := h.Get("Etag")
		if etag == "" {
			return true
		}
		if match == "*" {
			return false
		}
		for _, candidate := range tokenList(match) {
			if candidate == etag || candidate == "W/"+etag || "W/"+candidate == etag {
				return false
			}
		}
		return true
	}
	if unmodifiedSince := r.Header.Get("If-Unmodified-Since"); unmodifiedSince != "" {
		if us, ok := parseHTTPDate(unmodifiedSince); ok {
			lm, okLastModified := parseHTTPDate(h.Get("Last-Modified"))
			return !okLastModified || lm.After(us)
		}
	}
	return false
}

func tokenList(value string) []string {
	tokens := make([]string, 0, 4)
	start, end := 0, 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ' ':
			if start == end {
				start = i + 1
				end = i + 1
			}
		case ',':
			if start != end {
				tokens = append(tokens, value[start:end])
			}
			start = i + 1
			end = i + 1
		default:
			end = i + 1
		}
	}
	if start != end {
		tokens = append(tokens, value[start:end])
	}
	return tokens
}

func parseHTTPDate(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func sendFile(w http.ResponseWriter, r *http.Request, path string, info os.FileInfo, contentType string) {
	h := w.Header()
	poweredBy(w)
	if h.Get("Accept-Ranges") == "" {
		h.Set("Accept-Ranges", "bytes")
	}
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "public, max-age=0")
	}
	if h.Get("Last-Modified") == "" {
		h.Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	}
	if h.Get("Etag") == "" {
		h.Set("Etag", weakStatETag(info))
	}
	if h.Get("Content-Type") == "" && contentType != "" {
		h.Set("Content-Type", contentType)
	}

	size := info.Size()

	if r.Header.Get("If-Match") != "" || r.Header.Get("If-Unmodified-Since") != "" ||
		r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
		if preconditionFailed(r, h) {
			fileFailure(w, r, http.StatusPreconditionFailed)
			return
		}
		if requestFresh(r, h, http.StatusOK) {
			h.Del("Content-Encoding")
			h.Del("Content-Language")
			h.Del("Content-Length")
			h.Del("Content-Range")
			h.Del("Content-Type")
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	status := http.StatusOK
	offset := int64(0)
	length := size

	if bytesRangeSpec.MatchString(r.Header.Get("Range")) {
		ranges, result := parseRanges(size, r.Header.Get("Range"))
		if !rangeFresh(r, h) {
			ranges, result = nil, rangeInvalid
		}
		if result == rangeUnsatisfiable {
			h.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			fileFailure(w, r, http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if result >= 1 && len(ranges) == 1 {
			offset = ranges[0].start
			length = ranges[0].end - ranges[0].start + 1
			status = http.StatusPartialContent
			h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", ranges[0].start, ranges[0].end, size))
		}
	}

	h.Set("Content-Length", strconv.FormatInt(length, 10))

	var body []byte
	if length > 0 {
		file, err := os.Open(path)
		if err != nil {
			fileFailure(w, r, http.StatusInternalServerError)
			return
		}
		body, err = io.ReadAll(io.NewSectionReader(file, offset, length))
		_ = file.Close()
		if err != nil {
			fileFailure(w, r, http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(status)
	if r.Method == http.MethodHead || len(body) == 0 {
		return
	}
	_, _ = w.Write(body)
}

func fileFailure(w http.ResponseWriter, r *http.Request, status int) {
	h := w.Header()
	h.Set("Content-Type", withCharset(h.Get("Content-Type"), "utf-8"))
	sendEntity(w, r, status, []byte("Internal server error"))
}

func withCharset(contentType, charset string) string {
	base := contentType
	if i := strings.Index(base, ";"); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = "text/html"
	}
	return base + "; charset=" + charset
}

type byteRange struct {
	start int64
	end   int64
}

func parseRanges(size int64, spec string) ([]byteRange, int) {
	equals := strings.Index(spec, "=")
	if equals < 0 {
		return nil, rangeInvalid
	}
	parts := strings.Split(spec[equals+1:], ",")
	ranges := make([]byteRange, 0, len(parts))
	for _, part := range parts {
		bounds := strings.SplitN(part, "-", 2)
		startText := bounds[0]
		endText := ""
		if len(bounds) > 1 {
			endText = bounds[1]
		}
		start, okStart := jsParseInt(startText)
		end, okEnd := jsParseInt(endText)
		switch {
		case !okStart && !okEnd:
			continue
		case !okStart:
			start = size - end
			end = size - 1
		case !okEnd:
			end = size - 1
		}
		if end > size-1 {
			end = size - 1
		}
		if start > end || start < 0 {
			continue
		}
		ranges = append(ranges, byteRange{start: start, end: end})
	}
	if len(ranges) < 1 {
		return nil, rangeUnsatisfiable
	}
	combined := combineRanges(ranges)
	return combined, len(combined)
}

type indexedRange struct {
	start int64
	end   int64
	index int
}

func combineRanges(ranges []byteRange) []byteRange {
	ordered := make([]indexedRange, len(ranges))
	for i, r := range ranges {
		ordered[i] = indexedRange{start: r.start, end: r.end, index: i}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].start < ordered[j].start })
	j := 0
	for i := 1; i < len(ordered); i++ {
		current := ordered[j]
		if ordered[i].start > current.end+1 {
			j++
			ordered[j] = ordered[i]
			continue
		}
		if ordered[i].end > current.end {
			ordered[j].end = ordered[i].end
			if ordered[i].index < ordered[j].index {
				ordered[j].index = ordered[i].index
			}
		}
	}
	ordered = ordered[:j+1]
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].index < ordered[j].index })
	combined := make([]byteRange, len(ordered))
	for i, r := range ordered {
		combined[i] = byteRange{start: r.start, end: r.end}
	}
	return combined
}

func jsParseInt(text string) (int64, bool) {
	trimmed := strings.TrimSpace(text)
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, false
	}
	value, err := strconv.ParseInt(trimmed[:digits], 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func rangeFresh(r *http.Request, h http.Header) bool {
	ifRange := r.Header.Get("If-Range")
	if ifRange == "" {
		return true
	}
	if strings.Contains(ifRange, `"`) {
		etag := h.Get("Etag")
		return etag != "" && strings.Contains(ifRange, etag)
	}
	lastModified, okLastModified := parseHTTPDate(h.Get("Last-Modified"))
	ifRangeTime, okIfRange := parseHTTPDate(ifRange)
	return okLastModified && okIfRange && !lastModified.After(ifRangeTime)
}

type kv struct {
	Key   string
	Value any
}

type orderedObject []kv

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, entry := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := jsonStringify(entry.Key)
		if err != nil {
			return nil, err
		}
		value, err := jsonStringify(entry.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func jsonStringify(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	encoded := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	encoded = bytes.ReplaceAll(encoded, []byte(`\u2028`), []byte("\u2028"))
	encoded = bytes.ReplaceAll(encoded, []byte(`\u2029`), []byte("\u2029"))
	return encoded, nil
}
