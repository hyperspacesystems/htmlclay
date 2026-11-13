// Package assets embeds the workspace's browser bundle (@panphora/hyperclay-workspace
// dist/local-app), copied by scripts/vendor-workspace.mjs and pinned in lock.json.
package assets

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

//go:embed local-app lock.json
var files embed.FS

// Manifest is the bundle's manifest.json: the entry script and stylesheets the
// dashboard's entry page loads, and the assets they reference.
type Manifest struct {
	Version  int    `json:"version"`
	Protocol int    `json:"protocol"`
	Package  string `json:"package"`
	Entry    struct {
		JavaScript string   `json:"javascript"`
		CSS        []string `json:"css"`
	} `json:"entry"`
	Assets []Asset `json:"assets"`
}

// Asset is one file the manifest lists with its size and digest.
type Asset struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Lock names the workspace commit the bundle came from and every file's digest.
type Lock struct {
	Package struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	} `json:"package"`
	Files map[string]string `json:"files"`
}

// Bundle is the bundle's files, rooted at local-app (manifest.json at its top).
func Bundle() fs.FS {
	sub, err := fs.Sub(files, "local-app")
	if err != nil {
		panic(err)
	}
	return sub
}

// LoadManifest reads and checks the embedded manifest: protocol 1 and an entry script.
func LoadManifest() (Manifest, error) {
	var m Manifest
	raw, err := fs.ReadFile(Bundle(), "manifest.json")
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("workspace manifest: %w", err)
	}
	if m.Protocol != 1 || m.Entry.JavaScript == "" {
		return m, fmt.Errorf("workspace manifest: protocol %d, entry %q; want protocol 1 and an entry script", m.Protocol, m.Entry.JavaScript)
	}
	return m, nil
}

// LoadLock reads lock.json.
func LoadLock() (Lock, error) {
	var l Lock
	raw, err := files.ReadFile("lock.json")
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(raw, &l)
}
