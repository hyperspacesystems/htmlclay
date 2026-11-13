package appcore

import (
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
	facts.HasSyncMarker = hasSyncMarkerAbove(resolved, home) || hasSyncMarkerBelow(resolved)
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

// hasSyncMarkerAbove reports whether path or a folder between it and home holds
// Hyperclay Local's sync marker.
func hasSyncMarkerAbove(path, home string) bool {
	for dir := path; ; {
		if isSyncMarker(filepath.Join(dir, ".hyperclay", "sync-root.json")) {
			return true
		}
		if dir == home {
			return false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// hasSyncMarkerBelow reports whether a folder at most three levels below path holds
// Hyperclay Local's sync marker, so serving a wide folder does not swallow a synced
// folder inside it.
func hasSyncMarkerBelow(path string) bool {
	found := false
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if found {
			return
		}
		if isSyncMarker(filepath.Join(dir, ".hyperclay", "sync-root.json")) {
			found = true
			return
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
			if name == "node_modules" {
				continue
			}
			if strings.HasPrefix(name, ".") {
				continue
			}
			if !entry.IsDir() {
				continue
			}
			walk(filepath.Join(dir, name), depth+1)
		}
	}
	walk(path, 0)
	return found
}

// isSyncMarker reports whether path is a regular file.
func isSyncMarker(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
