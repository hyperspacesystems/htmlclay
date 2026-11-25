package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

func TestSetPortKeepsSavingWhenTheRestartCannotStart(t *testing.T) {
	a := newTestAppWithConfigDir(t, t.TempDir(), t.TempDir())
	told := make(chan struct{}, 1)
	a.rt.notify = func(string, string) error { told <- struct{}{}; return nil }
	a.rt.startDetached = func(string, ...string) error { return errors.New("no such file") }
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "host.setPort", Port: freeLoopbackPort(t), Confirmed: true}); refusal != nil {
		t.Fatalf("confirmed host.setPort = %+v", refusal)
	}
	select {
	case <-told:
	case <-time.After(10 * time.Second):
		t.Fatal("the failed restart was never reported")
	}
	if !a.admitWrite() {
		t.Fatal("the app is still running on its old port and refuses every save as restarting")
	}
	a.rt.writesInFlight.Add(-1)
}
