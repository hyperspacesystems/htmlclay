package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

// The export menu lists documents, not helper bindings: a document that declares
// no program is still a document the user can export, and a document opened by a
// bookmark counts because it is registered exactly like an OS-open one.
func TestExportDocumentRowsListEveryOpenDocumentWithoutHelperFiltering(t *testing.T) {
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, "proj")
	declaring := filepath.Join(folder, "page.htmlclay")
	writeTestFile(t, declaring, `<!doctype html><head><meta name="htmlclay-helper" content="search"></head>`)
	quiet := filepath.Join(folder, "quiet.htmlclay")
	writeTestFile(t, quiet, `<!doctype html><head></head>`)
	never := filepath.Join(folder, "never.htmlclay")
	writeTestFile(t, never, `<!doctype html><head></head>`)

	a := newTestApp(t, home)
	if err := a.trustFolder(folder); err != nil {
		t.Fatal(err)
	}
	s := liveSite(t, a, folder)
	if status, body := fetchNav(t, s.url(filepath.Join("proj", "page.htmlclay"))); status != 200 {
		t.Fatalf("bookmark navigation = %d: %s", status, body)
	}
	a.openForTest(t, quiet)

	rows := a.exportDocumentRows()
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want one per open document", rows)
	}
	if rows[0].Path != declaring || rows[0].Label != "~/proj/page.htmlclay" {
		t.Errorf("first row = %+v, want the bookmarked document labelled with its ~/ path", rows[0])
	}
	if rows[1].Path != quiet || rows[1].Label != "~/proj/quiet.htmlclay" {
		t.Errorf("second row = %+v, want the opened document labelled with its ~/ path", rows[1])
	}
	if _, ok := helperRowByPath(rows, never); ok {
		t.Errorf("rows = %+v, want no row for a document that was never opened", rows)
	}
}

// A row can be clicked after the document it names was closed. That export must
// be refused with a notification and must not touch the disk: half an export, or
// a Downloads folder the user never asked for, is worse than a message.
func TestExportDocumentRefusesAFileThatIsNoLongerOpen(t *testing.T) {
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, "proj")
	document := filepath.Join(folder, "board.htmlclay")
	writeTestFile(t, document, "<!doctype html><p>board</p>")

	a := newTestApp(t, home)
	var told []string
	a.rt.notify = func(title, message string) error {
		told = append(told, title+": "+message)
		return nil
	}

	rows := a.exportDocument(document)
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want no open documents", rows)
	}
	if len(told) != 1 || told[0] != "HTML Clay: That file is no longer open." {
		t.Errorf("told = %q, want the refusal alone", told)
	}
	if _, err := os.Stat(filepath.Join(home, "Downloads")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a refused export must not create the Downloads folder: %v", err)
	}
}

// exportHome makes a home with one document and an app whose browser never
// launches, so a test reads what would have been shown instead of opening Finder.
func exportHome(t *testing.T) (*app, string, *[]string) {
	t.Helper()
	home := evalTempHome(t)
	file := filepath.Join(home, "notes", "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)
	a := newServedTestApp(t, home, t.TempDir())
	opened := &[]string{}
	a.rt.launchBrowser = func(target string) error {
		*opened = append(*opened, target)
		return nil
	}
	return a, file, opened
}

// zipIn is every zip file in dir.
func zipIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var zips []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".zip") {
			zips = append(zips, entry.Name())
		}
	}
	return zips
}

func TestM3ExportAction(t *testing.T) {
	t.Run("a recent row exports a zip into Downloads", func(t *testing.T) {
		a, file, opened := exportHome(t)
		snapshot, refusal := a.do(context.Background(), coreAction(t, "export-open", "file.open", map[string]string{"path": file}), ChannelShell)
		if refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		id := ""
		for _, doc := range snapshot.Documents {
			if doc.Name == "page.htmlclay" {
				id = doc.ID
			}
		}
		if id == "" {
			t.Fatalf("file.open did not list page.htmlclay in %+v", snapshot.Documents)
		}

		_, refusal = a.do(context.Background(), coreAction(t, "export-1", "document.export", map[string]string{"id": id}), ChannelShell)
		if refusal != nil {
			t.Fatalf("document.export refused: %+v", refusal)
		}
		downloads := filepath.Join(a.rt.home, "Downloads")
		zips := zipIn(t, downloads)
		if len(zips) != 1 {
			t.Fatalf("Downloads holds %v, want one zip", zips)
		}
		info, err := os.Stat(filepath.Join(downloads, zips[0]))
		if err != nil || info.Size() == 0 {
			t.Fatalf("the zip %s is missing or empty: %v", zips[0], err)
		}
		if len(*opened) != 2 || (*opened)[1] != downloads {
			t.Fatalf("the export showed %q, want the Downloads folder %q after the open", *opened, downloads)
		}
	})

	t.Run("a row missing from the list is refused", func(t *testing.T) {
		a, _, opened := exportHome(t)
		_, refusal := a.do(context.Background(), coreAction(t, "export-missing", "document.export", map[string]string{"id": "dnowhere"}), ChannelShell)
		if refusal == nil || refusal.Code != appcore.RefuseNoSuchDocument {
			t.Fatalf("export of an unknown row = %+v, want %q", refusal, appcore.RefuseNoSuchDocument)
		}
		if len(*opened) != 0 {
			t.Fatalf("the refused export showed %q, want nothing", *opened)
		}
	})

	t.Run("the dashboard may export a row", func(t *testing.T) {
		a, file, _ := exportHome(t)
		snapshot, refusal := a.do(context.Background(), coreAction(t, "export-open-2", "file.open", map[string]string{"path": file}), ChannelShell)
		if refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		_, refusal = a.do(context.Background(), coreAction(t, "export-2", "document.export", map[string]string{"id": snapshot.Documents[0].ID}), ChannelDashboard)
		if refusal != nil {
			t.Fatalf("a safe action from the dashboard was refused: %+v", refusal)
		}
		if zips := zipIn(t, filepath.Join(a.rt.home, "Downloads")); len(zips) != 1 {
			t.Fatalf("Downloads holds %v, want one zip", zips)
		}
	})
}
