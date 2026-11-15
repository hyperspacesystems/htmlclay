package appcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/pathrel"
)

// ServeInputs is what the app knows when a person asks to serve a folder.
type ServeInputs struct {
	Path          string
	Name          string
	Home          string
	Protected     []string
	Served        []config.ServedFolder
	Trusted       []config.TrustedFolder
	LocalServed   []string                // already filtered to a live Local process by the caller
	NameAvailable func(name string) error // config.(*Config).ServedNameAvailable
	// OwnSyncRoots is the rootId of every folder this app syncs (sync.json's
	// syncSessions rows, stopped ones included): a marker naming one is this app's.
	OwnSyncRoots []string
}

// GatherServeFacts resolves the asked-for path and relates it to every other folder.
func GatherServeFacts(in ServeInputs) ServeFacts {
	facts := ServeFacts{}
	resolved, err := pathrel.RealPath(in.Path)
	if err != nil {
		facts.Path = in.Path
		facts.NameErr = in.NameAvailable(in.Name)
		return facts
	}
	facts.Path = resolved
	if info, err := os.Stat(resolved); err == nil {
		facts.Exists = true
		facts.IsDir = info.IsDir()
	}
	home := resolvePath(in.Home)
	facts.Home = pathrel.Relate(resolved, home)
	for _, other := range in.Protected {
		facts.Protected = append(facts.Protected, relateTo(resolved, other, ""))
	}
	servedPaths := make([]string, 0, len(in.Served))
	for _, folder := range in.Served {
		other := relateTo(resolved, folder.Path, folder.Name)
		servedPaths = append(servedPaths, other.Path)
		facts.Served = append(facts.Served, other)
	}
	for _, folder := range in.Trusted {
		path := resolvePath(folder.Path)
		if servedAlready(servedPaths, path) {
			continue
		}
		facts.Trusted = append(facts.Trusted, OtherFolder{Relation: pathrel.Relate(resolved, path), Path: path})
	}
	for _, other := range in.LocalServed {
		facts.LocalServed = append(facts.LocalServed, relateTo(resolved, other, ""))
	}
	facts.SyncMarkers = append(syncMarkersAbove(resolved, home), syncMarkersBelow(resolved)...)
	facts.OwnSyncRoots = in.OwnSyncRoots
	facts.NameErr = in.NameAvailable(in.Name)
	return facts
}

// relateTo resolves other the way the asked-for path was resolved and relates the two.
func relateTo(resolved, other, name string) OtherFolder {
	path := resolvePath(other)
	return OtherFolder{Relation: pathrel.Relate(resolved, path), Path: path, Name: name}
}

// resolvePath answers the real path, or the path as given when it cannot be resolved.
func resolvePath(path string) string {
	if resolved, err := pathrel.RealPath(path); err == nil {
		return resolved
	}
	return path
}

// servedAlready reports whether path is one of the served folders.
func servedAlready(served []string, path string) bool {
	for _, other := range served {
		if pathrel.Relate(path, other) == pathrel.Same {
			return true
		}
	}
	return false
}

// syncMarkersAbove answers the sync markers in path and in every folder between
// it and home.
func syncMarkersAbove(path, home string) []SyncMarker {
	var markers []SyncMarker
	for dir := path; ; {
		if marker, ok := readSyncMarker(dir); ok {
			markers = append(markers, marker)
		}
		if dir == home {
			return markers
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return markers
		}
		dir = parent
	}
}

// syncMarkersBelow answers the sync markers in folders at most three levels below
// path, so serving a wide folder does not swallow a synced folder inside it.
func syncMarkersBelow(path string) []SyncMarker {
	var markers []SyncMarker
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 0 {
			if marker, ok := readSyncMarker(dir); ok {
				markers = append(markers, marker)
				return
			}
		}
		if depth == 3 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "node_modules" || strings.HasPrefix(name, ".") || !entry.IsDir() {
				continue
			}
			walk(filepath.Join(dir, name), depth+1)
		}
	}
	walk(path, 0)
	return markers
}

// readSyncMarker answers dir's .hyperclay/sync-root.json when it is a regular
// file, with the rootId it names, or "" when that cannot be read.
func readSyncMarker(dir string) (SyncMarker, bool) {
	path := filepath.Join(dir, ".hyperclay", "sync-root.json")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return SyncMarker{}, false
	}
	marker := SyncMarker{Folder: dir}
	if data, err := os.ReadFile(path); err == nil {
		var body struct {
			RootID any `json:"rootId"`
		}
		if json.Unmarshal(data, &body) == nil {
			if id, ok := body.RootID.(string); ok {
				marker.RootID = id
			}
		}
	}
	return marker, true
}
