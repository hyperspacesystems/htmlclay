//go:build managedtrial

package managedtrial

import (
	"errors"
	"net/http"
	"os"
	"regexp"
)

// tailwindRoutePattern is server.js's `/^\/tailwindcss\/(.+)\.css$/`: a regex
// route so the captured name can hold slashes, the way a nested site's
// stylesheet does.
var tailwindRoutePattern = regexp.MustCompile(`^/tailwindcss/(.+)\.css$`)

// serveTailwind is GET /tailwindcss/<name>.css: the stored stylesheet, or one
// compiled from the document its name stands for.
func (s *rootServer) serveTailwind(w http.ResponseWriter, r *http.Request, rawPath string) error {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	match := tailwindRoutePattern.FindStringSubmatch(rawPath)
	if match == nil {
		return fileNotFoundError()
	}
	// Express matches its regex routes against the still-encoded path and decodes
	// the capture itself.
	name, err := decodeOnce(match[1])
	if err != nil {
		return err
	}
	cssPath, err := s.tailwind.writePath(name)
	if err != nil {
		return tailwindRefusal(w, r, err)
	}

	// A read that follows a save waits for that save's compile, so it never
	// answers the stylesheet the save has already replaced.
	s.tailwind.wait(name)
	if css, readErr := os.ReadFile(cssPath); readErr == nil {
		sendText(w, r, http.StatusOK, string(css))
		return nil
	}

	sourceName := name + ".html"
	htmlPath, err := s.tailwind.sourcePath(sourceName)
	if err != nil {
		return tailwindRefusal(w, r, err)
	}
	if _, statErr := os.Stat(htmlPath); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return tailwindRefusal(w, r, statErr)
		}
		sourceName = name + ".htmlclay"
		htmlPath, err = s.tailwind.sourcePath(sourceName)
		if err != nil {
			return tailwindRefusal(w, r, err)
		}
	}

	s.tailwind.begin(name)
	defer s.tailwind.end(name)
	css, err := s.tailwind.compileOnMiss(cssPath, htmlPath)
	if err != nil {
		// Local answers an empty stylesheet when this compile fails.
		sendText(w, r, http.StatusOK, "")
		return nil
	}
	sendText(w, r, http.StatusOK, css)
	return nil
}

// tailwindRefusal is the answer this route gives a path the resolver refuses:
// 400 for an invalid path, 403 for everything else, and an empty body either
// way.
func tailwindRefusal(w http.ResponseWriter, r *http.Request, err error) error {
	status := http.StatusForbidden
	var pathErr *PathError
	if errors.As(err, &pathErr) && pathErr.Status == http.StatusBadRequest {
		status = http.StatusBadRequest
	}
	sendText(w, r, status, "")
	return nil
}
