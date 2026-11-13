package assets

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"testing"

	"github.com/panphora/htmlclay/internal/clayruntime"
)

func TestProductFSRuntimeManifest(t *testing.T) {
	raw, err := fs.ReadFile(ProductFS(), runtimeDir+"/manifest.json")
	if err != nil {
		t.Fatalf("%s/manifest.json: %v", runtimeDir, err)
	}
	var manifest struct {
		Selected string   `json:"selected"`
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("%s/manifest.json: %v", runtimeDir, err)
	}
	if manifest.Selected != clayruntime.Selected {
		t.Fatalf("manifest.json selects %q, clayruntime selects %q", manifest.Selected, clayruntime.Selected)
	}
	want := make([]string, 0, len(clayruntime.Releases))
	for version := range clayruntime.Releases {
		want = append(want, version)
	}
	sort.Strings(want)
	got := append([]string{}, manifest.Versions...)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("manifest.json lists %v, clayruntime loads %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("manifest.json lists %v, clayruntime loads %v", got, want)
		}
	}
}

func TestProductFSRuntimeFiles(t *testing.T) {
	lock, err := ReadLock()
	if err != nil {
		t.Fatalf("the lock does not load: %v", err)
	}
	release, ok := clayruntime.Releases[clayruntime.Selected]
	if !ok {
		t.Fatalf("the selected release %q is not loaded", clayruntime.Selected)
	}
	if len(release.Files) == 0 {
		t.Fatalf("the selected release %q lists no files", clayruntime.Selected)
	}
	files := ProductFS()
	for name := range release.Files {
		_, want, ok := clayruntime.Asset(clayruntime.Selected, name)
		if !ok {
			t.Fatalf("%s: the asset is not embedded", name)
		}
		served := runtimeDir + "/" + clayruntime.Selected + "/" + name
		got, err := fs.ReadFile(files, served)
		if err != nil {
			t.Fatalf("%s: %v", served, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: reads %d bytes, clayruntime answers %d", served, len(got), len(want))
		}
	}
	const probed = "all.js"
	entry, _, ok := clayruntime.Asset(clayruntime.Selected, probed)
	if !ok {
		t.Fatalf("%s: the asset is not embedded", probed)
	}
	info, err := fs.Stat(files, runtimeDir+"/"+clayruntime.Selected+"/"+probed)
	if err != nil {
		t.Fatalf("%s: %v", probed, err)
	}
	if !info.ModTime().Equal(lock.Local.CommittedAt) {
		t.Fatalf("%s reports modified %s, the lock records %s", probed, info.ModTime(), lock.Local.CommittedAt)
	}
	if info.Size() != entry.Bytes {
		t.Fatalf("%s is %d bytes, the release index records %d", probed, info.Size(), entry.Bytes)
	}
}

func TestProductFSLocalFiles(t *testing.T) {
	local := FS()
	files := ProductFS()
	for _, name := range []string{"assets/favicon.svg", "src/main/assets/sync-worker.js"} {
		want, err := fs.ReadFile(local, name)
		if err != nil {
			t.Fatalf("%s through FS: %v", name, err)
		}
		got, err := fs.ReadFile(files, name)
		if err != nil {
			t.Fatalf("%s through ProductFS: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s reads %d bytes through ProductFS, %d through FS", name, len(got), len(want))
		}
	}
}

func TestProductFSRefuses(t *testing.T) {
	files := ProductFS()
	for _, name := range []string{
		runtimeDir + "/lock.json",
		"../x",
		runtimeDir + "/9.9.9/clay.js",
	} {
		if _, err := fs.ReadFile(files, name); !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("%s: err = %v", name, err)
		}
		if _, err := fs.Stat(files, name); !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("%s: stat err = %v", name, err)
		}
		if file, err := files.Open(name); !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			if file != nil {
				file.Close()
			}
			t.Fatalf("%s: open err = %v", name, err)
		}
	}
}
