// Command stubservice is a stand-in live-room service for the Mac app's Quit
// tests. The host runs it through HTMLCLAY_SERVICE_CMD, speaks the service's
// bootstrap, attach and HTTP protocol with it, and it reports one unsaved
// document while the file HTMLCLAY_STUB_HOLD exists, and it answers the sync key
// lines. It is test-only: it is
// never bundled with the app.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// ready is the service's READY line, as internal/service.Ready reads it.
type ready struct {
	URL   string `json:"url"`
	Epoch string `json:"epoch"`
	PID   int    `json:"pid"`
}

// unsavedDoc is one document the service could not save, as
// cmd/htmlclay's unsavedDoc reads it.
type unsavedDoc struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// flushAnswer is the answer to POST /v1/flush, as cmd/htmlclay's flushAnswer
// reads it.
type flushAnswer struct {
	Unsaved  []unsavedDoc `json:"unsaved"`
	TimedOut bool         `json:"timedOut"`
}

// keptRoot is one root the service kept, as cmd/htmlclay's rootsAnswer reads it.
type keptRoot struct {
	ID       string       `json:"id"`
	Unsaved  []unsavedDoc `json:"unsaved"`
	TimedOut bool         `json:"timedOut"`
}

// rootsAnswer is the answer to POST /v1/roots, as cmd/htmlclay's rootsAnswer
// reads it.
type rootsAnswer struct {
	Added   []string   `json:"added"`
	Removed []string   `json:"removed"`
	Kept    []keptRoot `json:"kept"`
}

func main() {
	in := bufio.NewReader(os.Stdin)
	line, _ := in.ReadString('\n')
	var boot struct {
		V      int    `json:"v"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal([]byte(line), &boot); err != nil || boot.V != 1 || len(boot.Secret) != 64 {
		fmt.Fprintln(os.Stderr, "stub service: bad bootstrap line")
		os.Exit(3)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub service: %v\n", err)
		os.Exit(1)
	}
	name := os.Getenv("HTMLCLAY_STUB_NAME")
	if name == "" {
		name = "unsaved.htmlclay"
	}
	server := &http.Server{
		Handler: stub{
			secret: boot.Secret,
			hold:   os.Getenv("HTMLCLAY_STUB_HOLD"),
			root:   os.Getenv("HTMLCLAY_STUB_ROOT"),
			name:   name,
		},
		ReadHeaderTimeout: 10 * time.Second,
	}
	go server.Serve(listener)

	body, err := json.Marshal(ready{
		URL:   "http://" + listener.Addr().String(),
		Epoch: fmt.Sprintf("%016x", uint64(time.Now().UnixNano())),
		PID:   os.Getpid(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("READY %s\n", body)

	for {
		next, err := in.ReadString('\n')
		if strings.Contains(next, `"host"`) {
			fmt.Println("ATTACHED")
		} else if strings.Contains(next, `"sync"`) {
			fmt.Println(syncAnswer(next))
		}
		if err != nil {
			break
		}
	}
	os.Exit(0)
}

// syncAnswer answers one sync key line as the service does (08-sync.md, C2),
// and appends the line to HTMLCLAY_STUB_SYNC_LOG when that is set, so a test can
// see what the host sent. Only a test's own key ever reaches it.
func syncAnswer(line string) string {
	if path := os.Getenv("HTMLCLAY_STUB_SYNC_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = f.WriteString(strings.TrimRight(line, "\n") + "\n")
			_ = f.Close()
		}
	}
	var message struct {
		Sync *struct {
			V   int     `json:"v"`
			Key *string `json:"key"`
		} `json:"sync"`
	}
	if json.Unmarshal([]byte(line), &message) != nil || message.Sync == nil || message.Sync.V != 1 {
		return "SYNC KEY REFUSED"
	}
	if message.Sync.Key == nil {
		return "SYNC KEY CLEARED"
	}
	if !strings.HasPrefix(*message.Sync.Key, "hcsk_") {
		return "SYNC KEY REFUSED"
	}
	return "SYNC KEY HELD"
}

// stub is the service's HTTP side: the supervisor's bearer, then one answer per
// path it calls.
type stub struct {
	secret string
	hold   string
	root   string
	name   string
}

func (s stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.secret {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	switch r.URL.Path {
	case "/v1/flush":
		writeJSON(w, s.flush())
	case "/v1/roots":
		writeJSON(w, rootsAnswer{Added: []string{}, Removed: []string{}, Kept: []keptRoot{}})
	case "/sync/v1/discover":
		// The answer is the file HTMLCLAY_STUB_DISCOVER names, read on every call so
		// a test can change it between calls.
		body, err := os.ReadFile(os.Getenv("HTMLCLAY_STUB_DISCOVER"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case "/sync/v1/configure":
		// Each body goes on its own line of the file HTMLCLAY_STUB_CONFIGURE_LOG names.
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if name := os.Getenv("HTMLCLAY_STUB_CONFIGURE_LOG"); name != "" {
			if f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_, _ = f.Write(append(bytes.TrimSpace(body), '\n'))
				_ = f.Close()
			}
		}
		writeJSON(w, map[string]any{"ok": true, "running": []string{}})
	case "/sync/v1/preview":
		// The answer is the file HTMLCLAY_STUB_PREVIEW names, or nothing to sync.
		answer := []byte(`{"ok":true,"files":0,"bytes":0,"toUpload":0,"toDownload":0,"differ":0}`)
		if name := os.Getenv("HTMLCLAY_STUB_PREVIEW"); name != "" {
			if data, err := os.ReadFile(name); err == nil {
				answer = data
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(answer)
	case "/sync/v1/bind":
		// The body goes to HTMLCLAY_STUB_BIND_LOG; while the file HTMLCLAY_STUB_BIND_HOLD
		// names exists the answer waits, as a first download does; the answer is the
		// file HTMLCLAY_STUB_BIND names, or a finished empty download.
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if name := os.Getenv("HTMLCLAY_STUB_BIND_LOG"); name != "" {
			if f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_, _ = f.Write(append(bytes.TrimSpace(body), '\n'))
				_ = f.Close()
			}
		}
		if hold := os.Getenv("HTMLCLAY_STUB_BIND_HOLD"); hold != "" {
			for {
				if _, err := os.Stat(hold); err != nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		answer := []byte(`{"ok":true,"files":0,"bytes":0}`)
		if name := os.Getenv("HTMLCLAY_STUB_BIND"); name != "" {
			if data, err := os.ReadFile(name); err == nil {
				answer = data
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(answer)
	default:
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}

// flush answers the documents it could not save: the one it holds while the
// hold file exists, and none otherwise.
func (s stub) flush() flushAnswer {
	answer := flushAnswer{Unsaved: []unsavedDoc{}}
	if s.hold == "" {
		return answer
	}
	if _, err := os.Stat(s.hold); err != nil {
		return answer
	}
	answer.Unsaved = append(answer.Unsaved, unsavedDoc{Root: s.root, Name: s.name})
	return answer
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
