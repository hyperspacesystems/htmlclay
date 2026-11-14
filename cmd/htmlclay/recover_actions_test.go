package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

const (
	recoverOlderVersion  = "<!doctype html>\n<html><body><p>older</p></body></html>\n"
	recoverNewestVersion = "<!doctype html>\n<html><body><p>newest newest</p></body></html>\n"
)

// recoverFixture is a recent document whose file is gone but whose history is
// still kept, and a destination folder inside home.
type recoverFixture struct {
	app         *app
	document    string
	destination string
	id          string
}

func newRecoverFixture(t *testing.T) *recoverFixture {
	t.Helper()
	home := evalTempHome(t)
	document := filepath.Join(home, "notes", "page.html")
	writeTestFile(t, document, servedTestDocument)

	cfgBase, configDir := t.TempDir(), t.TempDir()
	a := newRecentTestApp(t, home, cfgBase, configDir)
	a.rt.launchBrowser = func(string) error { return nil }
	if _, refusal := a.do(context.Background(), coreAction(t, "recover-open", "file.open", map[string]string{"path": document}), ChannelShell); refusal != nil {
		t.Fatalf("file.open refused: %+v", refusal)
	}
	keepLooseVersion(t, a, document, recoverOlderVersion)
	keepLooseVersion(t, a, document, recoverNewestVersion)
	if err := os.Remove(document); err != nil {
		t.Fatalf("remove %s: %v", document, err)
	}
	row := recentRow(t, a, document)
	if !row.FileMissing {
		t.Fatalf("the row for %s is not marked missing", document)
	}
	destination := filepath.Join(home, "recovered", "page.html")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		t.Fatalf("make %s: %v", filepath.Dir(destination), err)
	}
	return &recoverFixture{app: a, document: document, destination: destination, id: row.ID}
}

func TestM3Recover(t *testing.T) {
	t.Run("the recovered bytes equal the newest version", func(t *testing.T) {
		fx := newRecoverFixture(t)
		if _, refusal := fx.app.do(context.Background(), coreAction(t, "recover-1", "document.recover", map[string]string{
			"id": fx.id, "destination": fx.destination,
		}), ChannelShell); refusal != nil {
			t.Fatalf("document.recover refused: %+v", refusal)
		}

		got, err := os.ReadFile(fx.destination)
		if err != nil {
			t.Fatalf("read %s: %v", fx.destination, err)
		}
		if string(got) != recoverNewestVersion {
			t.Fatalf("recovered bytes = %q, want the newest version %q", got, recoverNewestVersion)
		}

		items, err := fx.app.readHistory(historyDocument{Path: fx.destination}, "")
		if err != nil {
			t.Fatalf("read the history at %s: %v", fx.destination, err)
		}
		if len(items) == 0 || items[0].Size != int64(len(recoverNewestVersion)) {
			t.Fatalf("history at %s = %+v, want the newest version first", fx.destination, items)
		}

		row, ok := findRecentRow(t, fx.app, fx.destination)
		if !ok {
			t.Fatalf("no row for %s after the recovery", fx.destination)
		}
		if row.ID != fx.id {
			t.Fatalf("the recovered row id = %q, want %q", row.ID, fx.id)
		}
		if row.FileMissing {
			t.Fatalf("the recovered row still says its file is missing: %+v", row)
		}
	})

	t.Run("an existing destination is byte for byte unchanged and the action is refused", func(t *testing.T) {
		fx := newRecoverFixture(t)
		existing := []byte("<!doctype html>\n<html><body><p>keep me</p></body></html>\n")
		if err := os.WriteFile(fx.destination, existing, 0644); err != nil {
			t.Fatalf("write %s: %v", fx.destination, err)
		}

		_, refusal := fx.app.do(context.Background(), coreAction(t, "recover-2", "document.recover", map[string]string{
			"id": fx.id, "destination": fx.destination,
		}), ChannelShell)
		if refusal == nil || refusal.Code != appcore.RefuseDestinationExists {
			t.Fatalf("document.recover = %+v, want %q", refusal, appcore.RefuseDestinationExists)
		}
		got, err := os.ReadFile(fx.destination)
		if err != nil {
			t.Fatalf("read %s: %v", fx.destination, err)
		}
		if string(got) != string(existing) {
			t.Fatalf("an existing destination changed from %q to %q", existing, got)
		}
		if _, err := os.Stat(fx.document); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("the refused recovery created %s: %v", fx.document, err)
		}
	})

	t.Run("the dashboard channel is refused", func(t *testing.T) {
		fx := newRecoverFixture(t)
		_, refusal := fx.app.do(context.Background(), coreAction(t, "recover-3", "document.recover", map[string]string{
			"id": fx.id, "destination": fx.destination,
		}), ChannelDashboard)
		if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("document.recover from the dashboard = %+v, want %q", refusal, appcore.RefuseNeedsConfirmation)
		}
		if _, err := os.Stat(fx.destination); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("the refused dashboard recovery created %s: %v", fx.destination, err)
		}
	})
}
