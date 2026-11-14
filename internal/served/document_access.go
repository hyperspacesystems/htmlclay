package served

import (
	"fmt"
	"io/fs"
	"os"
)

// rootFor answers one served root, or the refusal every route gives an id that is
// not served.
func (h *Host) rootFor(rootID string) (*rootServer, error) {
	h.mu.Lock()
	server, ok := h.byID[rootID]
	h.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("served: root %q is not served", rootID)
	}
	return server, nil
}

// Read answers one served document's bytes as its own commit lane reads them, and
// the stamp that lane judges an If-Match against. It is the document on disk,
// never the page a browser is served: no runtime and no save credential is
// injected, no tab is opened, no version is kept and nothing is registered.
func (h *Host) Read(rootID, rel string) ([]byte, string, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return nil, "", err
	}
	filePath, err := resolveWriteTarget(server.registry, rel)
	if err != nil {
		return nil, "", err
	}
	state, err := readFile(filePath)
	if err != nil {
		return nil, "", err
	}
	if state == nil {
		return nil, "", fs.ErrNotExist
	}
	return state.Bytes, state.Checksum, nil
}

// Version answers one kept version's own bytes, through the lane
// GET /_/version/<rootRel>/<versionName> reads them.
func (h *Host) Version(rootID, rel, id string) ([]byte, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return nil, err
	}
	if !isVersionFileName(id) {
		return nil, fmt.Errorf("served: %q is not a version name", id)
	}
	if _, err := resolveWriteTarget(server.registry, rel); err != nil {
		return nil, err
	}
	filePath, err := versionFilePath(server.baseDir, rel, id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filePath)
}
