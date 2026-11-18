package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestM4SyncCLI(t *testing.T) {
	previous := serveOS
	serveOS = "linux"
	t.Cleanup(func() { serveOS = previous })

	t.Run("connect, status, pause, resume and disconnect through the socket", func(t *testing.T) {
		a, _, _ := connectTestApp(t, true)
		si := &appSI{app: a}
		var out, errOut bytes.Buffer
		if code := runSync(si, a.rt.configDir, []string{"connect", "@maya"}, strings.NewReader(connectKeyOne+"\n"), &out, &errOut); code != serveExitOK {
			t.Fatalf("connect = %d, stdout %q stderr %q", code, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), "maya") || storedKey(t, a) != connectKeyOne {
			t.Fatalf("connect printed %q, stored %q", out.String(), storedKey(t, a))
		}
		out.Reset()
		if code := runSync(si, a.rt.configDir, []string{"status"}, nil, &out, &errOut); code != serveExitOK {
			t.Fatalf("status = %d", code)
		}
		for _, want := range []string{"maya", "1111", "studio (team, viewer): can't sync: plan-lapsed"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("status %q lacks %q", out.String(), want)
			}
		}
		if strings.Contains(out.String(), connectKeyOne[5:]) {
			t.Fatal("status printed the key")
		}
		for _, verb := range []string{"pause", "resume"} {
			out.Reset()
			if code := runSync(si, a.rt.configDir, []string{verb}, nil, &out, &errOut); code != serveExitOK {
				t.Fatalf("%s = %d %q", verb, code, out.String())
			}
			if paused := savedSync(t, a).Paused; paused != (verb == "pause") {
				t.Fatalf("after %s paused = %v", verb, paused)
			}
		}
		out.Reset()
		if code := runSync(si, a.rt.configDir, []string{"disconnect"}, nil, &out, &errOut); code != serveExitOK {
			t.Fatalf("disconnect = %d %q", code, out.String())
		}
		if storedKey(t, a) != "" {
			t.Fatal("disconnect left the key")
		}
		out.Reset()
		runSync(si, a.rt.configDir, []string{"status"}, nil, &out, &errOut)
		if !strings.Contains(out.String(), "Not connected") {
			t.Fatalf("status after disconnect %q", out.String())
		}
	})

	t.Run("the key is never an argument", func(t *testing.T) {
		a, _, _ := connectTestApp(t, true)
		si := &appSI{app: a}
		for _, args := range [][]string{{"connect", connectKeyOne}, {"connect", "maya", connectKeyOne}} {
			var out, errOut bytes.Buffer
			if code := runSync(si, a.rt.configDir, args, strings.NewReader(""), &out, &errOut); code != serveExitUsage {
				t.Fatalf("%q = %d, want usage", args, code)
			}
			if len(si.sent) != 0 {
				t.Fatalf("%q sent %q", args, si.sent)
			}
		}
	})

	t.Run("a refused key exits refused and a malformed one never leaves", func(t *testing.T) {
		a, setDiscovery, _ := connectTestApp(t, true)
		si := &appSI{app: a}
		var out, errOut bytes.Buffer
		if code := runSync(si, a.rt.configDir, []string{"connect"}, strings.NewReader("hcsk_short\n"), &out, &errOut); code != serveExitRefused || len(si.sent) != 0 {
			t.Fatalf("malformed = %d, sent %q", code, si.sent)
		}
		setDiscovery(discoverRefused)
		out.Reset()
		if code := runSync(si, a.rt.configDir, []string{"connect"}, strings.NewReader(connectKeyOne+"\n"), &out, &errOut); code != serveExitRefused {
			t.Fatalf("refused key = %d %q", code, out.String())
		}
		if strings.Contains(out.String()+errOut.String(), connectKeyOne[5:]) {
			t.Fatal("the refusal printed the key")
		}
	})

	t.Run("on Windows the commands run as they do everywhere", func(t *testing.T) {
		serveOS = "windows"
		t.Cleanup(func() { serveOS = "linux" })
		a, _, _ := connectTestApp(t, true)
		si := &appSI{app: a}
		var out, errOut bytes.Buffer
		if code := runSync(si, a.rt.configDir, []string{"connect"}, strings.NewReader(connectKeyOne+"\n"), &out, &errOut); code != serveExitOK {
			t.Fatalf("connect on Windows = %d, stdout %q, stderr %q", code, out.String(), errOut.String())
		}
		if strings.Contains(errOut.String(), "Windows") {
			t.Fatalf("connect on Windows still says %q", errOut.String())
		}
	})
}
