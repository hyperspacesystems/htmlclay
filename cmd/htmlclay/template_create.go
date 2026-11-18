package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/session"
	servicefiles "github.com/panphora/htmlclay/service"
)

// maxTemplateNameTries is how many free names template.create tries:
// Kanban.htmlclay, then Kanban 2.htmlclay up to Kanban 99.htmlclay.
const maxTemplateNameTries = 99

// createFromTemplate is template.create: it writes one of the app's own
// single-file templates to a free name in ~/htmlclay and opens it. The page
// names only the template, never a path or the content, and no file is ever
// replaced, so the action is safe for a page to send. The folder is not trusted.
func (a *app) createFromTemplate(templateID string) *appcore.Refusal {
	data, ok := servicefiles.Template(templateID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "templateID"}
	}
	name := templateID
	for _, card := range servicefiles.Cards() {
		if card.ID == templateID {
			name = card.Name
		}
	}
	dir := filepath.Join(a.rt.home, "htmlclay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.rt.logger.Printf("Could not make %s for a new document: %v", dir, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	// ~/htmlclay may be a link to another drive. HTML Clay opens only files
	// inside home, so refuse before writing a document it could not open.
	if real, err := resolveSymlinks(dir); err != nil {
		a.rt.logger.Printf("Could not resolve %s: %v", dir, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	} else if _, ok := session.ContainWithinHome(a.rt.home, real); !ok {
		a.rt.logger.Printf("Not creating a document in %s: it leads outside home to %s", dir, real)
		return &appcore.Refusal{Code: appcore.RefuseFileOutsideHome}
	}
	path, err := createFreeDocument(dir, name, ".htmlclay", data)
	if err != nil {
		a.rt.logger.Printf("Could not create a document from the %s template: %v", templateID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.rt.logger.Printf("Created %s from the %s template", path, templateID)
	return a.openPath(path)
}

// createFreeDocument writes data to dir/base+ext, or "base 2"+ext and so on,
// with O_EXCL so it never replaces a file, and returns the path it wrote.
func createFreeDocument(dir, base, ext string, data []byte) (string, error) {
	for n := 1; n <= maxTemplateNameTries; n++ {
		name := base + ext
		if n > 1 {
			name = fmt.Sprintf("%s %d%s", base, n, ext)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		if err := errors.Join(werr, f.Close()); err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("every name from %s%s to %s %d%s is taken", base, ext, base, maxTemplateNameTries, ext)
}
