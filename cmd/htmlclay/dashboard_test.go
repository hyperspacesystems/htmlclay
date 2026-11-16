package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/dashboard"
	"github.com/panphora/htmlclay/internal/platform"
)

// dashboardNoService is the dashboard over an app that runs no live-room service:
// the app's own routes answer, and the routes that would reach the service belong
// to a stub that fails the test when it is called.
type dashboardNoService struct {
	t          *testing.T
	app        *app
	host       string
	server     *httptest.Server
	cookie     string
	token      string
	entryRoute string
}

// openDashboardWithNoService enters the dashboard the way a person does, with no
// service running and no folder served.
func openDashboardWithNoService(t *testing.T, home string) *dashboardNoService {
	t.Helper()
	a := buildTestApp(t, home, t.TempDir())
	a.rt.port = servedTestPort
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the dashboard sent %s %s to the service, and no service is running", r.Method, r.URL.Path)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	return openDashboardOver(t, a, proxy)
}

// openDashboardOver enters the dashboard of a built app through proxy, the way a
// person does.
func openDashboardOver(t *testing.T, a *app, proxy http.Handler) *dashboardNoService {
	t.Helper()
	host := fmt.Sprintf("%s:%d", appcore.DashboardHost, a.rt.port)
	board, err := dashboard.New(dashboard.Options{Host: host, ProductName: "HTML Clay", API: a.dashboardAPI(proxy)})
	if err != nil {
		t.Fatal(err)
	}
	c := &dashboardNoService{t: t, app: a, host: host, server: httptest.NewServer(board)}
	t.Cleanup(c.server.Close)

	entry, err := board.EntryURL("/")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(entry)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest(http.MethodGet, c.server.URL+u.RequestURI(), nil)
	r.Host = host
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || len(res.Cookies()) != 1 {
		t.Fatalf("entering the dashboard = %d with %d cookies", res.StatusCode, len(res.Cookies()))
	}
	c.cookie = res.Cookies()[0].Name + "=" + res.Cookies()[0].Value
	c.entryRoute = res.Header.Get("Location")

	status, page := c.request(http.MethodGet, "/", nil, map[string]string{"Cookie": c.cookie})
	match := regexp.MustCompile(`<meta name="workspace-token" content="([^"]+)">`).FindStringSubmatch(page)
	if status != http.StatusOK || match == nil {
		t.Fatalf("the entry page = %d: %s", status, page)
	}
	c.token = match[1]
	return c
}

func (c *dashboardNoService) request(method, target string, body any, headers map[string]string) (int, string) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	r, err := http.NewRequest(method, c.server.URL+target, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	r.Host = c.host
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	text, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res.StatusCode, string(text)
}

// api calls one of the app's own routes as the entry page's script on the
// dashboard does: with the session cookie, the page's token, and from the
// dashboard's own origin.
func (c *dashboardNoService) api(method, target string, body any) (int, string) {
	c.t.Helper()
	headers := map[string]string{"Cookie": c.cookie, "X-Workspace-Token": c.token, "Origin": "http://" + c.host, "Sec-Fetch-Site": "same-origin"}
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	return c.request(method, target, body, headers)
}

// act sends one action to the dashboard's action route.
func (c *dashboardNoService) act(id, typ string, args map[string]any) (int, string) {
	c.t.Helper()
	return c.api(http.MethodPost, "/api/app/actions", map[string]any{"id": id, "type": typ, "args": args})
}

func (c *dashboardNoService) snapshot() appcore.Snapshot {
	c.t.Helper()
	status, body := c.api(http.MethodGet, "/api/app/snapshot", nil)
	var snapshot appcore.Snapshot
	if status != http.StatusOK || json.Unmarshal([]byte(body), &snapshot) != nil {
		c.t.Fatalf("the dashboard's snapshot = %d %s", status, body)
	}
	return snapshot
}

// refusalOf decodes the refusal in a 409 from the action route.
func refusalOf(t *testing.T, status int, body string) string {
	t.Helper()
	var answer struct {
		Refusal struct {
			Code  string `json:"code"`
			Field string `json:"field"`
			Text  string `json:"text"`
		} `json:"refusal"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("a refusal should be JSON: %d %s", status, body)
	}
	if status != http.StatusConflict || answer.Refusal.Text == "" {
		t.Fatalf("a refused action = %d %s", status, body)
	}
	return answer.Refusal.Code
}

// TestM3DashboardWithoutService is the dashboard's own routes answered by Go: a
// person who serves nothing still has a snapshot, a stream and the actions a
// browser may send, and none of it needs the live-room service.
func TestM3DashboardWithoutService(t *testing.T) {
	t.Run("the entry address opens Home", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		if c.entryRoute != "/" {
			t.Fatalf("the entry address opens %q, want Home", c.entryRoute)
		}
		status, page := c.request(http.MethodGet, "/", nil, map[string]string{"Cookie": c.cookie})
		if status != http.StatusOK || !strings.Contains(page, `<div id="workspace-root"></div>`) {
			t.Fatalf("Home = %d: %s", status, page)
		}
	})

	t.Run("the snapshot answers", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		snapshot := c.snapshot()
		if snapshot.V != appcore.SnapshotVersion || snapshot.Epoch == "" {
			t.Fatalf("the snapshot = v%d epoch %q", snapshot.V, snapshot.Epoch)
		}
		if len(snapshot.Folders) != 0 || len(snapshot.Documents) != 0 {
			t.Fatalf("a dashboard with no folders shows %d folders and %d documents", len(snapshot.Folders), len(snapshot.Documents))
		}
		for typ, class := range appcore.ActionClasses {
			if class == appcore.ClassGrant && snapshot.Host.Can[typ] {
				t.Fatalf("the dashboard offers %s, which needs a grant", typ)
			}
		}
		if !snapshot.Host.Can["folder.stopServing"] {
			t.Fatalf("the dashboard offers none of the app's own actions: %v", snapshot.Host.Can)
		}
	})

	t.Run("a change arrives on the event stream", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		r, _ := http.NewRequest(http.MethodGet, c.server.URL+"/api/app/events", nil)
		r.Host = c.host
		r.Header.Set("Cookie", c.cookie)
		r.Header.Set("X-Workspace-Token", c.token)
		r.Header.Set("Origin", "http://"+c.host)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("the event stream = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
		}
		events := make(chan appcore.Snapshot, 4)
		go func() {
			defer close(events)
			scanner := bufio.NewScanner(res.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var snapshot appcore.Snapshot
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snapshot) == nil {
					events <- snapshot
				}
			}
		}()
		first := nextSnapshot(t, events)
		status, body := c.act("change-1", "prefs.set", map[string]any{"key": "appearance", "value": "dark"})
		if status != http.StatusOK {
			t.Fatalf("prefs.set = %d %s", status, body)
		}
		next := nextSnapshot(t, events)
		if next.Rev <= first.Rev {
			t.Fatalf("a change arrived at rev %d, not past %d", next.Rev, first.Rev)
		}
		if next.Preferences.Appearance != "dark" {
			t.Fatalf("the change on the stream is %q", next.Preferences.Appearance)
		}
	})

	t.Run("folder.trust is refused and the config is unchanged", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		c := openDashboardWithNoService(t, home)
		status, body := c.act("trust-1", "folder.trust", map[string]any{"path": dir})
		if trusted := c.app.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
			t.Fatalf("the dashboard trusted %v", trusted)
		}
		if code := refusalOf(t, status, body); code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("folder.trust from the dashboard = %s", code)
		}
	})

	t.Run("folder.stopServing runs", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		c := openDashboardWithNoService(t, home)
		entry, err := c.app.rt.cfg.AddServedFolder(dir, platform.DirIdentity(dir), "notes")
		if err != nil {
			t.Fatal(err)
		}
		shown := c.snapshot()
		var served *appcore.Folder
		for i := range shown.Folders {
			if shown.Folders[i].ID == entry.ID {
				served = &shown.Folders[i]
			}
		}
		if served == nil || !served.Served {
			t.Fatalf("the served folder is not in the snapshot")
		}
		if served.Can.Serve {
			t.Fatalf("the dashboard offers to serve a folder")
		}
		if served.Why.Serve == nil || served.Why.Serve.Code != whyNeedsApp || served.Why.Serve.Text == "" {
			t.Fatalf("the folder's reason = %+v", served.Why.Serve)
		}
		status, body := c.act("stop-1", "folder.stopServing", map[string]any{"id": entry.ID})
		if status != http.StatusOK {
			t.Fatalf("folder.stopServing from the dashboard = %d %s", status, body)
		}
		var answer struct {
			Snapshot appcore.Snapshot `json:"snapshot"`
		}
		if err := json.Unmarshal([]byte(body), &answer); err != nil {
			t.Fatal(err)
		}
		stopped := false
		for _, folder := range answer.Snapshot.Folders {
			if folder.ID == entry.ID {
				if folder.Served {
					t.Fatalf("the folder is still served in the answer")
				}
				stopped = true
			}
		}
		if !stopped {
			t.Fatalf("the answer has no folder %s", entry.ID)
		}
		after, ok := c.app.rt.cfg.ServedFolderByID(entry.ID)
		if !ok || after.Serving {
			t.Fatalf("the config still serves the folder: %+v", after)
		}
	})

	t.Run("request.answer is refused", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		status, body := c.act("answer-1", "request.answer", map[string]any{"id": "r1", "revision": 1, "answer": "deny"})
		if code := refusalOf(t, status, body); code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("request.answer from the dashboard = %s", code)
		}
	})

	t.Run("a request without the cookie never reaches the handler", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		status, body := c.request(http.MethodGet, "/api/app/snapshot", nil, map[string]string{
			"X-Workspace-Token": c.token, "Origin": "http://" + c.host, "Sec-Fetch-Site": "same-origin",
		})
		if status != http.StatusForbidden || strings.Contains(body, `"v":`) {
			t.Fatalf("a request with the entry page's token and no cookie = %d %s", status, body)
		}
	})

	t.Run("a request with the shell's bearer and no cookie is refused", func(t *testing.T) {
		c := openDashboardWithNoService(t, evalTempHome(t))
		status, body := c.request(http.MethodPost, "/api/app/actions", map[string]any{"id": "a1", "type": "folder.stopServing"}, map[string]string{
			"Authorization": "Bearer " + strings.Repeat("a", 64), "X-Workspace-Token": c.token,
			"Origin": "http://" + c.host, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json",
		})
		if status != http.StatusForbidden || strings.Contains(body, "refusal") {
			t.Fatalf("a request with the shell's bearer and no cookie = %d %s", status, body)
		}
	})
}

// nextSnapshot waits for one snapshot from the event stream.
func nextSnapshot(t *testing.T, events <-chan appcore.Snapshot) appcore.Snapshot {
	t.Helper()
	select {
	case snapshot, ok := <-events:
		if !ok {
			t.Fatal("the event stream ended")
		}
		return snapshot
	case <-time.After(10 * time.Second):
		t.Fatal("no snapshot arrived on the event stream")
	}
	return appcore.Snapshot{}
}

// folderAsk is the question a page in a folder asks: to read one file, or to
// trust the folder it is in.
func folderAsk(kind, dir string) confirm.Ask {
	return confirm.Ask{
		Kind:     kind,
		Document: "index.html",
		Folder:   filepath.Base(dir),
		Path:     dir,
		Answers:  appcore.AnswersFor(kind),
	}
}

// requestWithID answers one request from a snapshot.
func requestWithID(t *testing.T, requests []appcore.Request, id string) appcore.Request {
	t.Helper()
	for _, request := range requests {
		if request.ID == id {
			return request
		}
	}
	t.Fatalf("the snapshot has no request %s: %+v", id, requests)
	return appcore.Request{}
}

// sameRequest reports whether two versions of one request show the same question
// and offer the same answers. Attention is what a show changes; nothing else is.
func sameRequest(a, b appcore.Request) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Revision == b.Revision &&
		a.Path == b.Path && a.Document == b.Document && a.Folder == b.Folder &&
		slices.Equal(a.Answers, b.Answers)
}

// TestM3DashboardRequests is what the dashboard may do about a question: raise
// it in the Mac app's window, and nothing else.
func TestM3DashboardRequests(t *testing.T) {
	t.Run("request.show raises attention and changes nothing else", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		c := openDashboardWithNoService(t, home)
		reply := askThroughRegistry(t, c.app, folderAsk(appcore.RequestFolderRead, dir))
		before := waitForOpenRequests(t, c.app, 1)[0]

		for i, action := range []string{"show-1", "show-2"} {
			// The second show carries an answer the way a forged request would:
			// a show may only raise the question, never decide it.
			args := map[string]any{"id": before.ID}
			if i == 1 {
				args["answer"] = "deny"
			}
			status, body := c.act(action, "request.show", args)
			after := requestWithID(t, c.snapshot().Requests, before.ID)
			if status != http.StatusOK {
				t.Fatalf("%s from the dashboard = %d %s", action, status, body)
			}
			if after.Attention != before.Attention+uint64(i+1) {
				t.Fatalf("attention after %d show(s) = %d, want %d", i+1, after.Attention, before.Attention+uint64(i+1))
			}
			if !sameRequest(after, before) {
				t.Fatalf("request.show changed the request: %+v -> %+v", before, after)
			}
			stillWaiting(t, reply)
		}
	})

	t.Run("the dashboard can never change a request's answers or path", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		c := openDashboardWithNoService(t, home)
		reply := askThroughRegistry(t, c.app, folderAsk(appcore.RequestFolderRead, dir))
		before := waitForOpenRequests(t, c.app, 1)[0]

		status, body := c.act("answer-1", "request.answer", map[string]any{
			"id": before.ID, "revision": before.Revision, "answer": "deny",
		})
		if code := refusalOf(t, status, body); code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("request.answer from the dashboard = %s", code)
		}
		if after := requestWithID(t, c.snapshot().Requests, before.ID); !sameRequest(after, before) {
			t.Fatalf("the refused answer changed the request: %+v -> %+v", before, after)
		}
		stillWaiting(t, reply)

		status, body = c.act("show-with-path", "request.show", map[string]any{
			"id": before.ID, "path": "/tmp/elsewhere",
		})
		if status != http.StatusOK {
			t.Fatalf("request.show carrying a path = %d %s", status, body)
		}
		after := requestWithID(t, c.snapshot().Requests, before.ID)
		if !sameRequest(after, before) {
			t.Fatalf("request.show changed the request's path or answers: %+v -> %+v", before, after)
		}
		stillWaiting(t, reply)
	})

	t.Run("after trust is removed, a folderReadAgain request for that folder is gone", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		c := openDashboardWithNoService(t, home)
		if err := c.app.trustFolder(dir); err != nil {
			t.Fatalf("trust %s: %v", dir, err)
		}
		folderID := ""
		for _, folder := range c.snapshot().Folders {
			if folder.Path == dir {
				folderID = folder.ID
			}
		}
		if folderID == "" {
			t.Fatalf("the trusted folder is not in the snapshot: %+v", c.snapshot().Folders)
		}
		reply := askThroughRegistry(t, c.app, folderAsk(appcore.RequestFolderReadAgain, dir))
		before := waitForOpenRequests(t, c.app, 1)[0]

		status, body := c.act("untrust-1", "folder.stopTrusting", map[string]any{"id": folderID})
		if status != http.StatusOK {
			t.Fatalf("folder.stopTrusting from the dashboard = %d %s", status, body)
		}
		if requests := c.snapshot().Requests; len(requests) != 0 {
			t.Fatalf("the snapshot still lists %d request(s) for the untrusted folder: %+v", len(requests), requests)
		}
		select {
		case answer := <-reply:
			if answer.Choice != confirm.ChoiceDeny {
				t.Fatalf("the ask settled with %+v, want deny", answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("the ask for the untrusted folder is still waiting")
		}

		status, body = c.act("show-old", "request.show", map[string]any{"id": before.ID})
		if code := refusalOf(t, status, body); code != appcore.RefuseNoSuchRequest {
			t.Fatalf("request.show for an old id = %s", code)
		}
	})
}

// TestDashboardSnapshotLeavesTheSharedSnapshot is the 8.57 browser pass's crash:
// the dashboard's view of a snapshot wrote into the map and folder list every
// other reader shares, so two open dashboards crashed the app with concurrent
// map writes, and the Mac app could read the dashboard's refusals.
func TestDashboardSnapshotLeavesTheSharedSnapshot(t *testing.T) {
	shared := appcore.Snapshot{
		Host:    appcore.HostState{Can: map[string]bool{}},
		Folders: []appcore.Folder{{ID: "f1", Can: appcore.FolderCan{Serve: true}}},
	}
	for typ, class := range appcore.ActionClasses {
		if class == appcore.ClassGrant {
			shared.Host.Can[typ] = true
		}
	}
	view := dashboardSnapshot(shared)
	if !shared.Folders[0].Can.Serve || shared.Folders[0].Why.Serve != nil {
		t.Fatalf("the shared folder changed: %+v", shared.Folders[0])
	}
	for typ, ok := range shared.Host.Can {
		if !ok {
			t.Fatalf("the shared snapshot lost %s", typ)
		}
	}
	if view.Folders[0].Can.Serve {
		t.Fatal("the dashboard's view can still serve")
	}
}
