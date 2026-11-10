//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/panphora/htmlclay/internal/clayruntime"
	"github.com/panphora/htmlclay/internal/specwire"
)

// ephemeralAttribute is the response-scoped stamp a document GET carries, so a
// tab can tell whether the version it loaded is still the one on disk.
const ephemeralAttribute = "documentetag"

// attributePattern tokenizes one attribute of a start-tag the way a browser
// does, so a `>` inside a quoted value does not end the tag.
var attributePattern = regexp.MustCompile(`\s+([^\s=/>]+)(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+))?`)

// serveHtml serves a document: its logical bytes with the page editor's merge
// state put back, the stamp of those exact bytes on the root element, and its
// public runtime URLs projected onto this host's bundled release unless the
// client asked for the canonical bytes.
func (s *rootServer) serveHtml(w http.ResponseWriter, r *http.Request, filePath, name string) error {
	stored, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	html, err := logicalBytes(s.baseDir, name, stored)
	if err != nil {
		return err
	}
	stamp := specwire.Etag(html)
	served := html
	if r.Header.Get("Clay-Source") != "1" {
		served = s.projectServed(w, r, html)
	}
	w.Header().Set("Content-Type", withCharset("text/html", "utf-8"))
	w.Header().Set("Vary", "Clay-Source")
	poweredBy(w)
	sendEntity(w, r, http.StatusOK, injectDocumentEtag(served, stamp))
	return nil
}

// projectServed points a document's public runtime URLs at this host's copy of
// the bundled release. Only the response changes: the stamp above still
// describes the bytes on disk.
func (s *rootServer) projectServed(w http.ResponseWriter, r *http.Request, html []byte) []byte {
	// Never re-encode a file that is not valid UTF-8: decoding would replace
	// every bad byte, and the answer would no longer be the stored document.
	if !utf8.Valid(html) {
		return html
	}
	result := clayruntime.ProjectRuntimeHTML(html, clayruntime.Options{
		Releases:        s.releases,
		SelectedVersion: s.selected,
		DocumentURL:     "http://" + r.Host + r.RequestURI,
	})
	if result.Unavailable != nil {
		detail := strings.TrimSpace(result.Unavailable.Reason + " " + strings.Join(result.Unavailable.Versions, ","))
		w.Header().Set("Clay-Runtime-Unavailable", detail)
	}
	if len(result.Mappings) == 0 {
		return html
	}
	return []byte(result.HTML)
}

// injectDocumentEtag puts the stamp of these exact bytes on the document's root
// element, replacing any value already there, and returns what to serve. Bytes
// in, bytes out: every byte outside the insertion point is copied by offset, so
// a document that is not valid UTF-8 comes back unchanged except for the stamp.
func injectDocumentEtag(bytes []byte, stamp string) []byte {
	raw := string(bytes)
	scan := raw
	if strings.HasPrefix(raw, "\xef\xbb\xbf") {
		scan = "   " + raw[3:]
	}
	start, end, ok := scanRootHtmlTag(scan)
	if !ok {
		return bytes
	}
	attrs := attributePattern.ReplaceAllStringFunc(raw[start+5:end], func(whole string) string {
		match := attributePattern.FindStringSubmatch(whole)
		if len(match) > 1 && strings.ToLower(match[1]) == ephemeralAttribute {
			return ""
		}
		return whole
	})
	value := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;").Replace(stamp)
	return []byte(raw[:start+5] + ` documentetag="` + value + `"` + attrs + raw[end:])
}

// scanRootHtmlTag finds the document's root <html> start-tag, skipping
// whitespace, comments, the doctype and processing instructions before it. It
// answers where the tag starts and where its `>` is, and refuses one that never
// closes: an unterminated tag carries no root attribute.
func scanRootHtmlTag(str string) (int, int, bool) {
	length := len(str)
	i := 0
	for i < length {
		c := str[i]
		if isHTMLSpace(c) {
			i++
			continue
		}
		if c != '<' {
			return 0, 0, false
		}
		if i+3 < length && str[i+1] == '!' && str[i+2] == '-' && str[i+3] == '-' {
			end := htmlCommentEnd(str, i+4)
			if end < 0 {
				return 0, 0, false
			}
			i = end
			continue
		}
		if i+1 < length && (str[i+1] == '!' || str[i+1] == '?') {
			end := strings.IndexByte(str[i:], '>')
			if end < 0 {
				return 0, 0, false
			}
			i += end + 1
			continue
		}
		break
	}

	if i+5 > length || strings.ToLower(str[i:i+5]) != "<html" {
		return 0, 0, false
	}
	if i+5 == length {
		return 0, 0, false
	}
	boundary := str[i+5]
	if !isHTMLSpace(boundary) && boundary != '>' && boundary != '/' {
		return 0, 0, false
	}
	start := i
	i += 5

	for i < length {
		c := str[i]
		if c == '>' {
			return start, i, true
		}
		if isHTMLSpace(c) || c == '/' {
			i++
			continue
		}
		nameStart := i
		for i < length {
			c = str[i]
			if isHTMLSpace(c) || c == '=' || c == '>' || c == '/' {
				break
			}
			i++
		}
		if i == nameStart {
			i++
			for i < length {
				c = str[i]
				if isHTMLSpace(c) || c == '=' || c == '>' || c == '/' {
					break
				}
				i++
			}
		}
		for i < length && isHTMLSpace(str[i]) {
			i++
		}
		if i < length && str[i] == '=' {
			i++
			for i < length && isHTMLSpace(str[i]) {
				i++
			}
			if i < length {
				c = str[i]
			}
			if c == '"' || c == '\'' {
				close := strings.IndexByte(str[i+1:], c)
				if close < 0 {
					return 0, 0, false
				}
				i += close + 2
			} else {
				for i < length {
					c = str[i]
					if isHTMLSpace(c) || c == '>' {
						break
					}
					i++
				}
			}
		}
	}
	return 0, 0, false
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// htmlCommentEnd finds the end of a comment whose body starts at from.
func htmlCommentEnd(str string, from int) int {
	if from < len(str) && str[from] == '>' {
		return from + 1
	}
	if from+1 < len(str) && str[from] == '-' && str[from+1] == '>' {
		return from + 2
	}
	for k := from; k+1 < len(str); k++ {
		if str[k] == '-' && str[k+1] == '-' {
			end := k + 2
			if end < len(str) && str[end] == '!' {
				end++
			}
			if end < len(str) && str[end] == '>' {
				return end + 1
			}
		}
	}
	return -1
}

// runtimeRelease is one release Local bundles, as its own manifest names it.
type runtimeRelease struct {
	Selected string
	Releases map[string]clayruntime.Release
}

// loadRuntimeReleases reads the releases the Local checkout bundles, the same
// way Local reads them: the offline-runtime manifest names the selected release
// and the releases to load, and each release's index lists its files and
// aliases.
func loadRuntimeReleases(localSource string) (runtimeRelease, error) {
	dir := filepath.Join(localSource, "src", "main", "offline-runtime")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return runtimeRelease{}, fmt.Errorf("--local-source %q: %w", localSource, err)
	}
	var manifest struct {
		Selected string   `json:"selected"`
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return runtimeRelease{}, fmt.Errorf("--local-source %q: offline-runtime/manifest.json: %w", localSource, err)
	}
	if manifest.Selected == "" || len(manifest.Versions) == 0 {
		return runtimeRelease{}, errors.New("--local-source: offline-runtime/manifest.json names no release")
	}
	releases := make(map[string]clayruntime.Release, len(manifest.Versions))
	for _, version := range manifest.Versions {
		raw, err := os.ReadFile(filepath.Join(dir, version, "offline-manifest.json"))
		if err != nil {
			return runtimeRelease{}, fmt.Errorf("--local-source %q: %w", localSource, err)
		}
		var release clayruntime.Release
		if err := json.Unmarshal(raw, &release); err != nil {
			return runtimeRelease{}, fmt.Errorf("--local-source %q: %s/offline-manifest.json: %w", localSource, version, err)
		}
		releases[version] = release
	}
	if _, ok := releases[manifest.Selected]; !ok {
		return runtimeRelease{}, fmt.Errorf("--local-source: the selected release %q is not bundled", manifest.Selected)
	}
	return runtimeRelease{Selected: manifest.Selected, Releases: releases}, nil
}
