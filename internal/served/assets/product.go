package assets

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/panphora/htmlclay/internal/clayruntime"
)

// runtimeDir is where Local keeps its bundled ClayJS releases.
const runtimeDir = "src/main/offline-runtime"

// ProductFS is FS with HTML Clay's own ClayJS releases mounted where Local keeps its
// bundled ones: src/main/offline-runtime/manifest.json is clayruntime's lock.json,
// and src/main/offline-runtime/<version>/... is clayruntime's <version>/....
// Every file reports the Local commit's time, as FS does.
func ProductFS() fs.FS {
	lock, err := ReadLock()
	if err != nil {
		panic(fmt.Sprintf("assets: %v", err))
	}
	return productFS{
		local:   FS(),
		runtime: stampedFS{fsys: clayruntime.FS(), modTime: lock.Local.CommittedAt},
	}
}

// productFS serves the Local files and HTML Clay's runtime releases as one tree.
type productFS struct {
	local   fs.FS
	runtime fs.FS
}

// route answers the filesystem and name to read for one name, or fs.ErrNotExist
// when the name is not served.
func (p productFS) route(name string) (fs.FS, string, error) {
	if name == runtimeDir+"/manifest.json" {
		return p.runtime, "lock.json", nil
	}
	if rest, ok := strings.CutPrefix(name, runtimeDir+"/"); ok {
		if rest == "lock.json" {
			return nil, "", fs.ErrNotExist
		}
		return p.runtime, rest, nil
	}
	return p.local, name, nil
}

func validName(op, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	return nil
}

func (p productFS) Open(name string) (fs.File, error) {
	if err := validName("open", name); err != nil {
		return nil, err
	}
	fsys, routed, err := p.route(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return fsys.Open(routed)
}

func (p productFS) Stat(name string) (fs.FileInfo, error) {
	if err := validName("stat", name); err != nil {
		return nil, err
	}
	fsys, routed, err := p.route(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	return fs.Stat(fsys, routed)
}

func (p productFS) ReadFile(name string) ([]byte, error) {
	if err := validName("readfile", name); err != nil {
		return nil, err
	}
	fsys, routed, err := p.route(name)
	if err != nil {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: err}
	}
	return fs.ReadFile(fsys, routed)
}
