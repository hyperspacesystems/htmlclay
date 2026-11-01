package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

type attachmentTerminal struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload struct {
		Program string `json:"program"`
		Code    string `json:"code"`
	} `json:"payload"`
}

// The test binary doubles as the helper program. The dispatcher starts a
// registered program with no arguments and passes this process's environment
// through, so TestMain picks the fake by environment variable, and the copy's
// file name tells it which program it is.
const attachmentHelperMode = "HTMLCLAY_ATTACHMENT_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(attachmentHelperMode) != "" {
		os.Exit(runAttachmentHelper())
	}
	http.DefaultTransport = loopbackTransport()
	os.Exit(m.Run())
}

func runAttachmentHelper() int {
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		return 1
	}
	self := os.Args[0]
	marker, err := os.OpenFile(self+".ran", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return 1
	}
	if _, err := marker.WriteString("ran\n"); err != nil {
		marker.Close()
		return 1
	}
	if err := marker.Close(); err != nil {
		return 1
	}
	label := strings.TrimSuffix(filepath.Base(self), ".exe")
	result, err := json.Marshal(map[string]any{"type": "result", "value": map[string]string{"program": label}})
	if err != nil {
		return 1
	}
	fmt.Println(string(result))
	return 0
}

func attachmentProgram(t *testing.T, label string) (path, marker string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), label)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	// Windows will not delete a hard link to a running image, so TempDir cleanup
	// would fail on the link to this test binary: copy there instead.
	if runtime.GOOS == "windows" || os.Link(self, path) != nil {
		source, err := os.Open(self)
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(target, source); err != nil {
			target.Close()
			t.Fatal(err)
		}
		if err := target.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(attachmentHelperMode, "1")
	return path, path + ".ran"
}

func attachmentRequestTerminal(t *testing.T, s *site, document, token, id string) attachmentTerminal {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := url.Values{"file": {document}, "token": {token}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+s.host+"/_/wire/subscribe?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("subscribe status %d", response.StatusCode)
	}
	wireHelperRequest(t, s, token, id, document)
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame attachmentTerminal
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.ID == id && (frame.Type == "wire/done" || frame.Type == "wire/error") {
			return frame
		}
	}
	t.Fatalf("no terminal result for %s: %v", id, scanner.Err())
	return attachmentTerminal{}
}

func TestHelperAttachmentUsesCurrentPermissions(t *testing.T) {
	for _, kind := range []string{"replacement", "remove", "forget", "new denial", "cleared denial"} {
		t.Run(kind, func(t *testing.T) {
			home, err := resolveSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			a := newTestApp(t, home)
			document := writeHelperPage(t, home, "search")
			s, _ := a.openForTest(t, document)
			oldPath, oldMarker := attachmentProgram(t, "old")
			newPath, newMarker := attachmentProgram(t, "replacement")
			old, err := a.rt.cfg.AddHelperProgram("search", oldPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: document, Name: "search", Program: old.ID, Allowed: true}); err != nil {
				t.Fatal(err)
			}
			if kind == "cleared denial" {
				a.setOpenDenials(document, []string{"search"})
			}
			stale := a.helpersForNavigation(document)
			if kind == "cleared denial" {
				if len(stale.allowed) != 0 || len(stale.denied) != 1 {
					t.Fatal("fixture did not capture denied permissions")
				}
			} else if stale.allowed["search"].ID != old.ID {
				t.Fatal("fixture did not capture old allowed permissions")
			}
			switch kind {
			case "replacement", "cleared denial":
				// The binding now names another file for the same registration, so a
				// stale attachment must not run the program it captured.
				a.rt.cfg.SetHelperProgramPath(old.ID, newPath)
				a.configureHelperDocumentWith(helperDocumentKey(document, "search"), helperApprovalDialogs{
					setup: func(string, string, string, string, string) (platform.Choice, error) {
						t.Error("configuring an already-allowed name opened the program dialog")
						return platform.ChoiceNone, nil
					},
					choose: func(string, string) (string, bool, error) {
						t.Error("configuring an already-allowed name opened the picker")
						return "", false, nil
					},
					prepare: func(string) (bool, error) {
						t.Error("configuring an already-allowed name prepared a program")
						return false, nil
					},
				})
			case "remove", "forget", "new denial":
				action := "revoke-all"
				if kind == "remove" {
					action = "remove"
					if err := os.Remove(oldPath); err != nil {
						t.Fatal(err)
					}
				}
				if outcome := a.programsAction(action, old.ID, "", ""); outcome == "failed" {
					t.Fatalf("%s = %q", action, outcome)
				}
				a.refreshHelperDispatchers()
				if kind == "new denial" {
					denied := a.helpersForUseWith(document, "search", allowDocumentDialogs(oldPath, platform.ConfirmDeny, nil))
					a.applyHelperPlan(s, document, denied)
				}
			}
			a.applyHelperPlan(s, document, stale)
			registered, ok := s.sessions.LookupByPath(document)
			if !ok {
				t.Fatal("document registration disappeared")
			}
			if kind == "remove" {
				// Remove from List remembers the removal for every document that
				// used the program, so the name is refused, not set up again.
				if state := helperDiscoveryState(t, s, document); state != "denied" {
					t.Fatalf("helper state %q after removing the program, want denied", state)
				}
				if _, err := os.Stat(oldMarker); !os.IsNotExist(err) {
					t.Fatalf("old program executed or marker inspection failed: %v", err)
				}
				return
			}
			terminal := attachmentRequestTerminal(t, s, document, registered.Token, "current-permission-request")
			if kind == "replacement" || kind == "cleared denial" {
				if terminal.Type != "wire/done" || terminal.Payload.Program != "replacement" {
					t.Fatalf("stale attachment executed wrong program: %+v", terminal)
				}
				current, _ := a.rt.cfg.ResolveHelper(document, "search", true)
				if !current.Allowed || current.Program.Path != newPath {
					t.Fatal("replacement config changed")
				}
				if _, err := os.Stat(newMarker); err != nil {
					t.Fatalf("replacement did not execute: %v", err)
				}
			} else {
				if terminal.Type != "wire/error" || terminal.Payload.Code != "helper_not_granted" {
					t.Fatalf("stale allow was restored: %+v", terminal)
				}
				state := helperDiscoveryState(t, s, document)
				// Remove All Access is remembered as a refusal, like a new Deny.
				want := "denied"
				if state != want {
					t.Fatalf("helper state %q, want %q", state, want)
				}
			}
			if _, err := os.Stat(oldMarker); !os.IsNotExist(err) {
				t.Fatalf("old program executed or marker inspection failed: %v", err)
			}
		})
	}
}

func TestHelperRefreshDoesNotNotifyForAnUnrelatedDeletedDocument(t *testing.T) {
	settledSetup(t)
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	gone := filepath.Join(home, "old-notes.htmlclay")
	writeTestFile(t, gone, `<head><meta name="htmlclay-helper" content="ocr"></head>`)
	a.openForTest(t, document)
	a.openForTest(t, gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	var notifications atomic.Int64
	a.rt.notify = func(string, string) error { notifications.Add(1); return nil }
	path := writeTestProgram(t, t.TempDir(), "search", 0755)
	a.configureHelperDocumentWith(helperDocumentKey(document, "search"), allowDocumentDialogs(path, platform.ConfirmAllowOnce, nil))
	current, _ := a.rt.cfg.ResolveHelper(document, "search", true)
	if !current.Allowed || current.Program.Path != path {
		t.Fatal("repair did not save the selected program")
	}
	if got := notifications.Load(); got != 0 {
		t.Fatalf("repair notified about an unrelated document %d times", got)
	}
}
