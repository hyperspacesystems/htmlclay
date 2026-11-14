package server

import (
	"github.com/panphora/htmlclay/internal/wire"
	"testing"
)

func TestRunningWorkCountsAIEditsAndHelperRequests(t *testing.T) {
	srv, f := setupLiveSyncTest(t)
	if n := srv.RunningWork(); n != 0 {
		t.Fatalf("an idle server reports %d running", n)
	}
	srv.AIEdit.Mu.Lock()
	srv.AIEdit.Running = map[string]wire.AIEditRun{f.AbsPath: {ID: "e1", Cancel: func() {}}}
	srv.AIEdit.Mu.Unlock()
	fake := &wire.Dispatcher{Live: map[string]wire.LiveHelperRun{"r1": {Cancel: func() {}}, "r2": {Cancel: func() {}}}}
	srv.HelperMu.Lock()
	if srv.Helpers == nil {
		srv.Helpers = map[string]*wire.Dispatcher{}
	}
	srv.Helpers["fake-document"] = fake
	srv.HelperMu.Unlock()
	t.Cleanup(func() {
		srv.HelperMu.Lock()
		delete(srv.Helpers, "fake-document")
		srv.HelperMu.Unlock()
		srv.AIEdit.Mu.Lock()
		srv.AIEdit.Running = nil
		srv.AIEdit.Mu.Unlock()
	})
	if n := srv.RunningWork(); n != 3 {
		t.Fatalf("RunningWork = %d, want 3 (one AI edit, two helper requests)", n)
	}
}
