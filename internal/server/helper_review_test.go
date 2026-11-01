package server

import (
	"encoding/json"
	"github.com/panphora/htmlclay/internal/config"
	"testing"
	"time"
)

func TestReviewTrayStyleRegrantResumesParkedRequest(t *testing.T) {
	shortHelperParkLimit(t, 300*time.Millisecond)
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool { t.Fatal("tray regrant should not ask again"); return false }})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)
	srv.wire.publish(file.AbsPath, wireEnvelope{V: 1, Type: "wire/request", ID: "waiting", File: file.AbsPath, Helper: "search", Payload: json.RawMessage(`{}`)})
	receiveHelperType(t, observer, "wire/ack", "waiting")
	// configureHelperDocumentWith -> refreshHelperDispatchers -> applyHelperPlan calls AttachHelpers.
	p := helperProgram("search", structuredHelper(t, "result-null"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": p}, nil); err != nil {
		t.Fatal(err)
	}
	for {
		env := receiveHelperEnvelope(t, observer)
		if env.ID != "waiting" {
			continue
		}
		if env.Type == "wire/done" {
			return
		}
		if env.Type == "wire/error" {
			t.Fatalf("tray grant left request parked until %s", helperErrorCode(t, env))
		}
	}
}
