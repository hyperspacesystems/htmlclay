package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type VectorIndex struct {
	Families []struct {
		File      string `json:"file"`
		GoPending bool   `json:"goPending"`
	} `json:"families"`
}

func VectorPath(t testing.TB, name string) string {
	t.Helper()
	dir := os.Getenv("HOST_VECTORS_DIR")
	if dir == "" {
		_, here, _, _ := runtime.Caller(0)
		dir = filepath.Join(filepath.Dir(here), "..", "..", "node_modules", "@panphora", "host-conformance", "vectors")
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("host vector %s not found: run npm ci in the htmlclay root, or set HOST_VECTORS_DIR (%v)", name, err)
	}
	return path
}

func LoadVectors(t testing.TB, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(VectorPath(t, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}
