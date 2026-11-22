package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/tier"
	servicefiles "github.com/panphora/htmlclay/service"
)

// maxTemplateNameTries is how many free names template.create tries:
// kanban.htmlclay, then kanban-2.htmlclay up to kanban-99.htmlclay.
const maxTemplateNameTries = 99

func templateSlug(templateID string) string {
	switch strings.ToLower(templateID) {
	case "blank":
		return "untitled"
	case "blockeditor":
		return "block-editor"
	default:
		return strings.ToLower(templateID)
	}
}

// createFromTemplate is template.create: it writes one of the app's own
// single-file templates to a free name in ~/htmlclay and opens it. The page
// names only the template, never a path or the content, and no file is ever
// replaced, so the action is safe for a page to send. The folder is not trusted.
func (a *app) createFromTemplate(templateID string) *appcore.Refusal {
	data, ok := servicefiles.Template(templateID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "templateID"}
	}
	slug := templateSlug(templateID)
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
	path, err := createFreeDocument(dir, numberedName(slug, "-", ".htmlclay"), data)
	if err != nil {
		a.rt.logger.Printf("Could not create a document from the %s template: %v", templateID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.rt.logger.Printf("Created %s from the %s template", path, templateID)
	if err := a.recordMadeDocument(path, tier.HowCreated); err != nil {
		a.rt.logger.Printf("Could not record %s as yours: %v", path, err)
	}
	return a.openPath(path)
}

// numberedName names the nth try at a free file name: base and ext, then base,
// sep and the number from 2 on.
func numberedName(base, sep, ext string) func(n int) string {
	return func(n int) string {
		if n == 1 {
			return base + ext
		}
		return fmt.Sprintf("%s%s%d%s", base, sep, n, ext)
	}
}

// createFreeDocument writes data to the first name in dir that name gives for
// 1, 2 and on, up to maxTemplateNameTries, and never replaces a file.
func createFreeDocument(dir string, name func(n int) string, data []byte) (string, error) {
	for n := 1; n <= maxTemplateNameTries; n++ {
		path := filepath.Join(dir, name(n))
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
	return "", fmt.Errorf("every name from %s to %s is taken", name(1), name(maxTemplateNameTries))
}
