//go:build managedtrial || servedfaults

package served

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 08-sync.md 8.51: the fault sweep over the writes sync makes. Every fault point
// is crossed with every sync route, and after each one, over a fresh control
// server on the same folder, the folder is judged: the file holds all of the
// bytes it had or all of the bytes the write carried, bytes a download replaced
// are a version, nothing is left beside it, a dropped reply is still answered by
// /v1/op, and the route's next write works.
//
// A move or a trash is a rename, and a remove is an unlink: no write fault point
// sits on them, so their rows prove the armed fault is left alone and the route
// still does its whole job. The sweep checks which faults fire on which route, so
// a fault point moved off a route fails here instead of passing quietly.

// huntSyncRoute is one write sync makes, with the file it touches.
type huntSyncRoute struct {
	name string
	// target is the path the fault is armed for and the write's file.
	target string
	// existing is what target holds before the write; empty when it is new.
	existing string
	// write sends the route's write of content with opID.
	write func(t *testing.T, s *huntSite, opID, content string) huntAnswer
	// fires is the faults that sit on this route's write.
	fires map[string]bool
	// gone is true for a route whose job is to take target away (remove, move,
	// trash); where names the one other place the bytes may be found after it.
	gone  bool
	where string
}

func huntSyncCommit(t *testing.T, s *huntSite, opID, name, content string) huntAnswer {
	t.Helper()
	return huntControl(t, s, "/v1/commit", map[string]any{
		"opId":  opID,
		"root":  huntRootID,
		"name":  name,
		"kind":  "sync-download",
		"bytes": map[string]any{"encoding": "utf8", "data": content},
	})
}

func huntSyncFile(t *testing.T, s *huntSite, opID, op, rel string, content *string) huntAnswer {
	t.Helper()
	body := map[string]any{"opId": opID, "root": huntRootID, "op": op, "rel": rel}
	if content != nil {
		body["bytes"] = map[string]any{"encoding": "utf8", "data": *content}
	}
	return huntControl(t, s, "/v1/sync/file", body)
}

func huntRestructure(t *testing.T, s *huntSite, opID, kind string, steps []map[string]any) huntAnswer {
	t.Helper()
	return huntControl(t, s, "/v1/restructure", map[string]any{"opId": opID, "root": huntRootID, "kind": kind, "steps": steps})
}

// huntControl is one authenticated control request.
func huntControl(t *testing.T, s *huntSite, path string, body any) huntAnswer {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, s.server.URL+path, strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+controlSecret)
	request.Header.Set("Content-Type", "application/json")
	return huntDo(request)
}

// huntCommitted reports whether a control answer is a committed write.
func huntCommitted(answer huntAnswer) bool {
	if answer.err != nil || answer.status != http.StatusOK {
		return false
	}
	outcome := map[string]any{}
	return json.Unmarshal([]byte(answer.body), &outcome) == nil && outcome["outcome"] == "committed"
}

// huntOpState is what GET /v1/op/<opID> answers.
func huntOpState(t *testing.T, s *huntSite, opID string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.server.URL+"/v1/op/"+opID, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+controlSecret)
	answer := huntDo(request)
	state := map[string]any{}
	if answer.err != nil || json.Unmarshal([]byte(answer.body), &state) != nil {
		t.Fatalf("GET /v1/op/%s: %s", opID, answer)
	}
	return state
}

// huntVersionOf reports whether a kept version of the document at rel holds content.
func huntVersionOf(t *testing.T, dir, rel, content string) bool {
	t.Helper()
	versionsDir := filepath.Join(dir, ".hyperclay", "versions", filepath.FromSlash(strings.TrimSuffix(rel, filepath.Ext(rel))))
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if data, err := os.ReadFile(filepath.Join(versionsDir, entry.Name())); err == nil && string(data) == content {
			return true
		}
	}
	return false
}

// huntArmedLeft is how many armed faults no write has taken.
func huntArmedLeft() int {
	faults.Lock()
	defer faults.Unlock()
	return len(faults.armed)
}

// huntReadOptional is a file's bytes, or "" and false when it is not there.
func huntReadOptional(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data), true
}

// huntLeftovers is every file in dir's tree, outside .hyperclay/ and .trash/,
// that is not one of the expected names.
func huntLeftovers(t *testing.T, dir string, expected ...string) []string {
	t.Helper()
	want := map[string]bool{}
	for _, name := range expected {
		want[name] = true
	}
	extra := []string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".hyperclay" || rel == ".trash" {
				return filepath.SkipDir
			}
			return nil
		}
		if !want[rel] {
			extra = append(extra, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return extra
}

func huntSyncRoutes() []huntSyncRoute {
	atomicFaults := map[string]bool{faultKillBeforeRename: true, faultKillAfterRename: true, faultDurableFail: true}
	commitFaults := map[string]bool{faultKillBeforeRename: true, faultKillAfterRename: true, faultDurableFail: true, faultDropReply: true}
	keptCopy := ".hyperclay/conflicts/s1/doc (conflicted copy).html"
	return []huntSyncRoute{
		{
			name: "download-changed", target: "doc.html", existing: huntDoc("Old"), fires: commitFaults,
			write: func(t *testing.T, s *huntSite, opID, content string) huntAnswer {
				return huntSyncCommit(t, s, opID, "doc.html", content)
			},
		},
		{
			name: "download-new", target: "new.html", fires: commitFaults,
			write: func(t *testing.T, s *huntSite, opID, content string) huntAnswer {
				return huntSyncCommit(t, s, opID, "new.html", content)
			},
		},
		{
			// A folder new on the server reaches Go as a folder restructure and
			// then the download into it.
			name: "download-new-folder", target: "sub/new.html", fires: commitFaults,
			write: func(t *testing.T, s *huntSite, opID, content string) huntAnswer {
				if answer := huntRestructure(t, s, "3a00000000000003", "folder", []map[string]any{{"to": "sub"}}); !huntCommitted(answer) {
					return answer
				}
				return huntSyncCommit(t, s, opID, "sub/new.html", content)
			},
		},
		{
			name: "sync-file-put", target: keptCopy, existing: huntDoc("Old copy"), fires: atomicFaults,
			write: func(t *testing.T, s *huntSite, opID, content string) huntAnswer {
				return huntSyncFile(t, s, opID, "put", keptCopy, &content)
			},
		},
		{
			name: "sync-file-remove", target: keptCopy, existing: huntDoc("Old copy"), fires: map[string]bool{}, gone: true,
			write: func(t *testing.T, s *huntSite, opID, _ string) huntAnswer {
				return huntSyncFile(t, s, opID, "remove", keptCopy, nil)
			},
		},
		{
			name: "sync-move", target: "doc.html", existing: huntDoc("Old"), fires: map[string]bool{}, gone: true, where: "moved.html",
			write: func(t *testing.T, s *huntSite, opID, _ string) huntAnswer {
				return huntRestructure(t, s, opID, "sync-move", []map[string]any{{"from": "doc.html", "to": "moved.html"}})
			},
		},
		{
			name: "sync-trash", target: "doc.html", existing: huntDoc("Old"), fires: map[string]bool{}, gone: true, where: ".trash/doc.html",
			write: func(t *testing.T, s *huntSite, opID, _ string) huntAnswer {
				return huntRestructure(t, s, opID, "sync-trash", []map[string]any{{"from": "doc.html", "to": ".trash/doc.html"}})
			},
		},
	}
}

// huntSyncStage puts the route's starting file in place with no fault armed.
func huntSyncStage(t *testing.T, s *huntSite, route huntSyncRoute) {
	t.Helper()
	if route.existing == "" {
		return
	}
	var answer huntAnswer
	if strings.HasPrefix(route.target, ".hyperclay/") {
		answer = huntSyncFile(t, s, "5a0000000000000a", "put", route.target, &route.existing)
	} else {
		answer = huntSyncCommit(t, s, "5a0000000000000a", route.target, route.existing)
	}
	if !huntCommitted(answer) {
		t.Fatalf("staging %s: %s", route.target, answer)
	}
}

func huntSyncOne(t *testing.T, route huntSyncRoute, fault string) {
	replaceKillSelf(t)
	dir := huntTempDir(t)
	lane := huntLane{name: route.name, control: true}
	site := huntOpen(t, lane, dir)
	huntSyncStage(t, site, route)

	newBytes := huntDoc("New " + route.name)
	const opID = "1a00000000000001"
	if fault != huntFaultNone {
		armFault(fault, route.target, 1)
	}
	answer := route.write(t, site, opID, newBytes)
	fired := fault != huntFaultNone && huntArmedLeft() == 0
	clearFaults()

	if fault != huntFaultNone && fired != route.fires[fault] {
		t.Errorf("%s/%s: the fault fired=%v, want %v", route.name, fault, fired, route.fires[fault])
	}
	switch {
	case fired && (fault == faultKillBeforeRename || fault == faultKillAfterRename):
		if answer.err == nil && answer.status < 500 {
			t.Errorf("%s/%s: the killed write answered %s, want a transport error or a 5xx", route.name, fault, answer)
		}
	case fired && fault == faultDropReply:
		// The write happened; the engine learns so from /v1/op before a retry.
		if answer.err == nil {
			t.Errorf("%s/%s: the dropped reply arrived: %s", route.name, fault, answer)
		}
		if state := huntOpState(t, site, opID); state["state"] != "committed" {
			t.Errorf("%s/%s: /v1/op answers %v after a dropped reply, want committed", route.name, fault, state)
		}
	case fired && fault == faultDurableFail:
		// A disk that cannot sync its folder: the write is not reported as kept.
		if huntCommitted(answer) {
			t.Errorf("%s/%s: a write the disk could not sync answered committed: %s", route.name, fault, answer)
		}
	default:
		if !huntCommitted(answer) {
			t.Errorf("%s/%s: the write answered %s, want committed", route.name, fault, answer)
		}
	}
	site.close()

	// A restart: a fresh control server over the same folder.
	fresh := huntOpen(t, lane, dir)
	defer fresh.close()
	target := filepath.Join(dir, filepath.FromSlash(route.target))
	got, present := huntReadOptional(t, target)
	landed := "absent"
	switch {
	case route.gone:
		elsewhere, there := "", false
		if route.where != "" {
			elsewhere, there = huntReadOptional(t, filepath.Join(dir, filepath.FromSlash(route.where)))
		}
		switch {
		case present && got == route.existing && !there:
			landed = "old"
		case !present && route.where == "":
			landed = "new"
		case !present && there && elsewhere == route.existing:
			landed = "new"
		default:
			t.Errorf("%s/%s: after the write, %s present=%v and %s present=%v; want the bytes in exactly one place, whole",
				route.name, fault, route.target, present, route.where, there)
		}
	case !present:
		if route.existing != "" {
			t.Errorf("%s/%s: %s is gone; it held %d bytes before the write", route.name, fault, route.target, len(route.existing))
		}
	case got == newBytes:
		landed = "new"
		if route.existing != "" && !strings.HasPrefix(route.target, ".hyperclay/") && !huntVersionOf(t, dir, route.target, route.existing) {
			t.Errorf("%s/%s: %s holds the downloaded bytes, but no version holds the bytes they replaced", route.name, fault, route.target)
		}
	case got == route.existing && route.existing != "":
		landed = "old"
	default:
		t.Errorf("%s/%s: %s holds neither its old bytes nor the new ones, %d bytes: %q", route.name, fault, route.target, len(got), huntShort(got))
	}

	expected := []string{}
	for _, rel := range []string{route.target, route.where} {
		if rel != "" && !strings.HasPrefix(rel, ".hyperclay/") && !strings.HasPrefix(rel, ".trash/") {
			expected = append(expected, rel)
		}
	}
	if extras := huntLeftovers(t, dir, expected...); len(extras) > 0 && !(fault == faultKillBeforeRename && huntOnlyTemps(extras)) {
		t.Errorf("%s/%s: the folder holds %v beside the route's own files", route.name, fault, extras)
	}

	// The route's next write, with no fault, still works over the fresh server.
	next := huntDoc("Next " + route.name)
	var nextAnswer huntAnswer
	switch {
	case route.name == "sync-file-remove":
		nextAnswer = huntSyncFile(t, fresh, "2a00000000000002", "put", route.target, &next)
	case route.gone:
		nextAnswer = huntSyncCommit(t, fresh, "2a00000000000002", "again.html", next)
	case strings.HasPrefix(route.target, ".hyperclay/"):
		nextAnswer = huntSyncFile(t, fresh, "2a00000000000002", "put", route.target, &next)
	default:
		nextAnswer = huntSyncCommit(t, fresh, "2a00000000000002", route.target, next)
	}
	if !huntCommitted(nextAnswer) {
		t.Errorf("%s/%s: the next write answered %s, want committed", route.name, fault, nextAnswer)
	}
	t.Logf("HUNT-SYNC route=%s fault=%s fired=%v landed=%s", route.name, fault, fired, landed)
}

// huntOnlyTemps reports whether every leftover is an atomic temp a kill before
// the rename leaves (hunt finding H1-a, as Local does).
func huntOnlyTemps(names []string) bool {
	for _, name := range names {
		base := filepath.Base(name)
		if !strings.HasPrefix(base, ".") || !strings.HasSuffix(base, ".tmp") {
			return false
		}
	}
	return true
}

// TestHuntSyncFaultSweep crosses every fault point with every write sync makes.
func TestHuntSyncFaultSweep(t *testing.T) {
	faults := []string{huntFaultNone, faultKillBeforeRename, faultKillAfterRename, faultDurableFail, faultDropReply, faultTailwindHold}
	ran := 0
	for _, route := range huntSyncRoutes() {
		for _, fault := range faults {
			if fault == faultDurableFail && runtime.GOOS == "windows" {
				continue
			}
			ran++
			t.Run(fmt.Sprintf("%s/%s", route.name, fault), func(t *testing.T) {
				huntSyncOne(t, route, fault)
			})
		}
	}
	want := len(huntSyncRoutes()) * len(faults)
	if runtime.GOOS == "windows" {
		want = len(huntSyncRoutes()) * (len(faults) - 1)
	}
	if ran != want || ran < 36 {
		t.Fatalf("the sweep ran %d cases, want %d (at least 36)", ran, want)
	}
}
