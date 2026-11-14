package server

import (
	"errors"
	"io/fs"
	"os"

	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/versions"
)

// ReadDocument answers one loose document's bytes as its own save lane reads them,
// through the same descriptor check /_/save uses, and the stamp that lane judges
// an If-Match against. Nothing is registered, opened or kept: it is a read of the
// file on disk exactly as it is.
func (s *Server) ReadDocument(absPath string) ([]byte, string, error) {
	data, err := s.readRegisteredFile(absPath)
	if err != nil {
		return nil, "", err
	}
	return data, specwire.Etag(data), nil
}

// ReadDocumentVersion answers one kept version's own bytes, through the lane
// GET /_/versions/<name> reads them. The document's history identity is the open
// file's when this server holds one, since that is resolved once at first serve,
// and is derived from the document's bytes otherwise. A read creates no history.
func (s *Server) ReadDocumentVersion(absPath, name string) ([]byte, error) {
	if _, _, err := versions.ParseEntryName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key := ""
	if f, ok := s.sessions.LookupByPath(absPath); ok {
		key = f.HistoryKey()
	}
	if key == "" {
		key = versions.Key(absPath, data)
	}
	return s.versions.Read(key, absPath, name)
}
