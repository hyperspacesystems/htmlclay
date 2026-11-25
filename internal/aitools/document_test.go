package aitools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLCarriesAIDetectsPluginsInstructionsAndProgramsOnly(t *testing.T) {
	yes := []string{
		`<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js?plugins=ai,menu"></script></head></html>`,
		`<!doctype html><html><head><script src="/_/clayjs/1.26.0/clay.js?plugins=wire,ai"></script></head></html>`,
		`<!doctype html><html><head><script type="text/x-clay-instructions" name="review">Review this.</script></head></html>`,
		`<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js?plugins=programs"></script></head></html>`,
		`<!doctype html><html><head><script type="text/x-clay-program" name="notes">async function main() {}</script></head></html>`,
		`<html><body><svg><title><script src="https://clayjs.com/v1/clay.js?plugins=programs,sync"></script></title></svg></body></html>`,
		`<html><body><svg><title><script type="text/x-clay-program" name="p">async function main() {}</script></title></svg></body></html>`,
	}
	for _, doc := range yes {
		if !HTMLCarriesAI([]byte(doc)) {
			t.Errorf("HTMLCarriesAI(%s) = false, want true", doc)
		}
	}
	no := []string{
		`<!doctype html><html><head><script src="https://clayjs.com/v1/clay.js?plugins=blockeditor,ai-edit"></script></head></html>`,
		`<!doctype html><html><body><p>Plain notes</p></body></html>`,
	}
	for _, doc := range no {
		if HTMLCarriesAI([]byte(doc)) {
			t.Errorf("HTMLCarriesAI(%s) = true, want false", doc)
		}
	}
}

// A program tag runs from anywhere in the first 2.5 MiB, so live sync must see
// one that far in too.
func TestDocumentCarriesAIReadsAsFarAsProgramsRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.html")
	pad := "<!--" + strings.Repeat("x", 1<<20) + "-->"
	doc := "<!doctype html><html><head>" + pad + `<script type="text/x-clay-program" name="notes">async function main() {}</script></head></html>`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if !DocumentCarriesAI(path) {
		t.Fatal("a program tag 1 MB into the file does not block live sync, though HTML Clay runs it")
	}
}
