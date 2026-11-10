//go:build managedtrial

package managedtrial

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/dataguard"
)

type serveRequest struct {
	Name            string            `json:"name"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Headers         map[string]string `json:"headers"`
	IfNoneMatchFrom string            `json:"ifNoneMatchFrom"`
	IfMatchFrom     *serveIfMatchFrom `json:"ifMatchFrom"`
	// FromCase names a field of an earlier answer that this request's path
	// carries, the way IfMatchFrom names one a header carries.
	FromCase *serveIfMatchFrom `json:"fromCase"`
	Body     *string           `json:"body"`
	BodyFile string            `json:"bodyFile"`
}

type serveIfMatchFrom struct {
	Case string `json:"case"`
	JSON string `json:"json"`
}

type serveBody struct {
	Text   *string `json:"text"`
	Base64 *string `json:"base64"`
}

type serveAnswer struct {
	Name    string            `json:"name"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    serveBody         `json:"body"`
	Answer  []byte            `json:"-"`
}

type serveFileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type serveGolden struct {
	Cases []serveAnswer    `json:"cases"`
	Files []serveFileEntry `json:"files"`
	// Clock and Pages are recorded for a case directory that pins them: a write
	// names versions after the instant it happened, and a page-state generation
	// is random bytes, so both are replayed for the tree to match.
	Clock int64    `json:"clock"`
	Pages []string `json:"pages"`
}

// goldenOptions are a case directory's knowingly allowed differences.
type goldenOptions struct {
	normalize func(*serveAnswer)
	skipFile  func(path string) bool
	// dataJudgedCases and dataJudgedFiles are the one decided byte difference:
	// Local writes data (a guard Restore, a data API write) through cheerio, which
	// re-serializes the whole document, while dataapi splices only the changed
	// spans. A data-judged answer holds the same extracted data as Local's, and its
	// etag and content-length stamp this host's own body. A data-judged file holds
	// the data of the named case's recorded body. Everything else stays exact.
	dataJudgedCases map[string]bool
	dataJudgedFiles map[string]string
}

var serveDroppedHeaders = map[string]bool{
	"date":              true,
	"connection":        true,
	"keep-alive":        true,
	"transfer-encoding": true,
}

const serveGoldenMtime = "2020-01-02T03:04:05Z"

func serveGoldenDir() string { return filepath.Join("testdata", "serve-golden") }

func TestServeGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and the handler reads that checkout's bundled runtime release")
	}
	runGoldenCases(t, filepath.Join("testdata", "serve-golden"), localSource, nil)
}

// runGoldenCases replays one case directory's requests against this host and
// compares every answer, status, header and body, with the recorded file list,
// exactly. normalize rewrites a recorded answer before comparison when a case
// directory holds a knowingly allowed difference.
func runGoldenCases(t *testing.T, caseDir, localSource string, normalize func(*serveAnswer)) {
	t.Helper()
	runGoldenCasesWith(t, caseDir, localSource, goldenOptions{normalize: normalize})
}

func runGoldenCasesWith(t *testing.T, caseDir, localSource string, options goldenOptions) {
	t.Helper()
	normalize := options.normalize
	var golden serveGolden
	raw, err := os.ReadFile(filepath.Join(caseDir, "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var requests []serveRequest
	raw, err = os.ReadFile(filepath.Join(caseDir, "requests.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &requests); err != nil {
		t.Fatal(err)
	}
	if len(requests) == 0 || len(requests) != len(golden.Cases) {
		t.Fatalf("requests.json holds %d cases, golden.json %d", len(requests), len(golden.Cases))
	}

	if golden.Clock != 0 {
		clock := &commitClock{pages: golden.Pages}
		value := golden.Clock
		previousClock := versionClock
		previousGeneration := pageStateNewGeneration
		versionClock = func() int64 { return value }
		pageStateNewGeneration = clock.nextGeneration
		t.Cleanup(func() {
			versionClock = previousClock
			pageStateNewGeneration = previousGeneration
			if clock.exhausted {
				t.Error("the host asked for more page-state generations than Local did")
			}
		})
	}

	runtime, err := loadRuntimeReleases(localSource)
	if err != nil {
		t.Fatal(err)
	}
	for i := range requests {
		requests[i].Path = strings.ReplaceAll(requests[i].Path, "{CLAYJS_VERSION}", runtime.Selected)
	}

	root := serveGoldenRoot(t, caseDir)
	handler, err := newRootHandler(rootSpec{id: "root-a", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderLimit}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	address := fmt.Sprintf("127.0.0.1:%d", port)

	answered := make(map[string]serveAnswer, len(requests))
	for i, request := range requests {
		want := golden.Cases[i]
		if want.Name != request.Name {
			t.Fatalf("case %d is %q in requests.json and %q in golden.json", i, request.Name, want.Name)
		}
		headers := resolveGoldenHeaders(t, request, answered, port)
		requestPath := request.Path
		if request.FromCase != nil {
			requestPath = strings.ReplaceAll(requestPath, "{FROM}", resolveJSONField(t, request.Name, *request.FromCase, answered))
		}
		status, gotHeaders, body := serveSend(t, address, request.Method, requestPath, headers, serveRequestBody(t, caseDir, request), port)
		answered[request.Name] = serveAnswer{Name: request.Name, Status: status, Headers: gotHeaders, Answer: body}

		t.Run(request.Name, func(t *testing.T) {
			expected := want
			if normalize != nil {
				normalize(&expected)
			}
			if status != expected.Status {
				t.Errorf("status = %d, want %d", status, expected.Status)
			}
			wantBody := serveDecodeBody(t, expected.Body)
			if options.dataJudgedCases[request.Name] {
				wantHeaders := ownStampHeaders(expected.Headers, body)
				if !reflect.DeepEqual(gotHeaders, wantHeaders) {
					t.Errorf("headers = %v, want %v", gotHeaders, wantHeaders)
				}
				assertSameData(t, "body", body, wantBody)
				if !bytes.Equal(body, wantBody) {
					t.Logf("known difference: %s holds Local's data in dataapi's bytes, not cheerio's", request.Name)
				}
				return
			}
			if !reflect.DeepEqual(gotHeaders, expected.Headers) {
				t.Errorf("headers = %v, want %v", gotHeaders, expected.Headers)
			}
			if !bytes.Equal(body, wantBody) {
				t.Errorf("body = %q, want %q", body, wantBody)
			}
		})
	}

	got := serveFileList(t, root)
	want := golden.Files
	if options.skipFile != nil {
		got = filterServeFiles(got, options.skipFile)
		want = filterServeFiles(want, options.skipFile)
	}
	if len(options.dataJudgedFiles) > 0 {
		judged := func(path string) bool { _, ok := options.dataJudgedFiles[path]; return ok }
		for path, caseName := range options.dataJudgedFiles {
			if !hasServeFile(got, path) || !hasServeFile(want, path) {
				t.Errorf("%s: data-judged file missing (on this host: %v, in Local's tree: %v)", path, hasServeFile(got, path), hasServeFile(want, path))
				continue
			}
			bytesOnDisk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				t.Fatal(err)
			}
			recorded := goldenCaseBody(t, golden, caseName)
			assertSameData(t, path, bytesOnDisk, recorded)
		}
		got = filterServeFiles(got, judged)
		want = filterServeFiles(want, judged)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the root's files changed: got %v, want %v", got, want)
	}
}

// ownStampHeaders is Local's recorded headers with the two that stamp the body
// (etag, content-length) recomputed from this host's own body.
func ownStampHeaders(recorded map[string]string, body []byte) map[string]string {
	headers := make(map[string]string, len(recorded))
	for key, value := range recorded {
		headers[key] = value
	}
	if _, ok := headers["etag"]; ok {
		headers["etag"] = weakEntityETag(body)
	}
	if _, ok := headers["content-length"]; ok {
		headers["content-length"] = strconv.Itoa(len(body))
	}
	return headers
}

// assertSameData fails unless both documents carry a rules tag and the data it
// extracts is equal.
func assertSameData(t *testing.T, label string, got, want []byte) {
	t.Helper()
	gotIsland, gotOK := safeExtractIsland(string(got))
	wantIsland, wantOK := safeExtractIsland(string(want))
	if !gotOK || !wantOK || gotIsland == nil || wantIsland == nil {
		t.Errorf("%s: no data to compare (this host extracted: %v, Local's extracted: %v)", label, gotOK && gotIsland != nil, wantOK && wantIsland != nil)
		return
	}
	if !dataguard.IslandsEqual(gotIsland, wantIsland) {
		t.Errorf("%s: data = %s, want %s", label, dataguard.Stringify(gotIsland), dataguard.Stringify(wantIsland))
	}
}

func hasServeFile(files []serveFileEntry, path string) bool {
	for _, entry := range files {
		if entry.Path == path {
			return true
		}
	}
	return false
}

func goldenCaseBody(t *testing.T, golden serveGolden, name string) []byte {
	t.Helper()
	for _, answer := range golden.Cases {
		if answer.Name == name {
			return serveDecodeBody(t, answer.Body)
		}
	}
	t.Fatalf("no recorded case %q", name)
	return nil
}

// serveRequestBody is the bytes a case sends: `body` as written, or the case
// folder's own file named by `bodyFile`.
func serveRequestBody(t *testing.T, caseDir string, request serveRequest) []byte {
	t.Helper()
	if request.BodyFile != "" {
		body, err := os.ReadFile(filepath.Join(caseDir, request.BodyFile))
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	if request.Body != nil {
		return []byte(*request.Body)
	}
	return nil
}

// filterServeFiles drops the files a case directory's options leave out.
func filterServeFiles(files []serveFileEntry, skip func(string) bool) []serveFileEntry {
	kept := make([]serveFileEntry, 0, len(files))
	for _, entry := range files {
		if !skip(entry.Path) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// resolveGoldenHeaders fills a request's {PORT} with this host's port and
// resolves a conditional header from an earlier answer's JSON, the same way the
// recorder did when it recorded the golden.
func resolveGoldenHeaders(t *testing.T, request serveRequest, answered map[string]serveAnswer, port int) map[string]string {
	t.Helper()
	headers := make(map[string]string, len(request.Headers)+1)
	for name, value := range request.Headers {
		headers[name] = strings.ReplaceAll(value, "{PORT}", strconv.Itoa(port))
	}
	if request.IfNoneMatchFrom != "" {
		from, ok := answered[request.IfNoneMatchFrom]
		if !ok || from.Headers["etag"] == "" {
			t.Fatalf("%s: no etag recorded for %q", request.Name, request.IfNoneMatchFrom)
		}
		headers["If-None-Match"] = from.Headers["etag"]
	}
	if request.IfMatchFrom != nil {
		headers["If-Match"] = resolveJSONField(t, request.Name, *request.IfMatchFrom, answered)
	}
	return headers
}

// resolveJSONField reads one dotted field out of an earlier answer's JSON body:
// an object key, or an array index.
func resolveJSONField(t *testing.T, name string, from serveIfMatchFrom, answered map[string]serveAnswer) string {
	t.Helper()
	answer, ok := answered[from.Case]
	if !ok {
		t.Fatalf("%s: no answer recorded for %q", name, from.Case)
	}
	var decoded any
	if err := json.Unmarshal(answer.Answer, &decoded); err != nil {
		t.Fatalf("%s: %q's answer is not JSON: %v", name, from.Case, err)
	}
	value := decoded
	for _, part := range strings.Split(from.JSON, ".") {
		switch container := value.(type) {
		case map[string]any:
			value, ok = container[part]
			if !ok {
				t.Fatalf("%s: %q's answer has no %q", name, from.Case, from.JSON)
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(container) {
				t.Fatalf("%s: %q's answer has no %q", name, from.Case, from.JSON)
			}
			value = container[index]
		default:
			t.Fatalf("%s: %q's answer has no %q", name, from.Case, from.JSON)
		}
	}
	text, ok := value.(string)
	if !ok || text == "" {
		t.Fatalf("%s: %q's answer has no %q", name, from.Case, from.JSON)
	}
	return text
}

func TestListingRender(t *testing.T) {
	var golden serveGolden
	raw, err := os.ReadFile(filepath.Join(serveGoldenDir(), "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	listings := []struct {
		name    string
		path    string
		entries []listingEntry
	}{
		{"folder-slash", "folder", []listingEntry{
			{Name: "app.html", IsDir: false},
			{Name: "notes.txt", IsDir: false},
			{Name: "sub", IsDir: true},
		}},
		{"folder-two-deep", "folder/sub", []listingEntry{
			{Name: "deep", IsDir: true},
		}},
		{"folder-three-deep", "folder/sub/deep", []listingEntry{
			{Name: "note.txt", IsDir: false},
		}},
	}
	for _, listing := range listings {
		var want []byte
		for _, answer := range golden.Cases {
			if answer.Name == listing.name {
				want = serveDecodeBody(t, answer.Body)
			}
		}
		if want == nil {
			t.Fatalf("golden.json holds no %s answer", listing.name)
		}
		got := renderDirListing(listing.path, listing.entries)
		if !bytes.Equal(got, want) {
			t.Errorf("%s: renderDirListing = %q, want %q", listing.name, got, want)
		}
	}
}

func serveDecodeBody(t *testing.T, body serveBody) []byte {
	t.Helper()
	if body.Text != nil {
		return []byte(*body.Text)
	}
	if body.Base64 == nil {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(*body.Base64)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// serveSend is one request over a bare connection, so the path reaches the host
// exactly as the golden records it: Go's client would refuse a malformed
// percent escape and would rewrite nothing else, but it would also decode and
// re-encode the path first.
func serveSend(t *testing.T, address, method, path string, headers map[string]string, body []byte, port int) (int, map[string]string, []byte) {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var request bytes.Buffer
	fmt.Fprintf(&request, "%s %s HTTP/1.1\r\n", method, path)
	fmt.Fprintf(&request, "Host: 127.0.0.1:%d\r\n", port)
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&request, "%s: %s\r\n", name, headers[name])
	}
	if body != nil {
		fmt.Fprintf(&request, "Content-Length: %d\r\n", len(body))
	}
	request.WriteString("Connection: close\r\n\r\n")
	request.Write(body)
	if _, err := conn.Write(request.Bytes()); err != nil {
		t.Fatal(err)
	}

	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: method})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, normalizeServeHeaders(response.Header, port), normalizeServeBody(answer, port)
}

func normalizeServeHeaders(headers http.Header, port int) map[string]string {
	out := make(map[string]string, len(headers))
	for name, values := range headers {
		lower := strings.ToLower(name)
		if serveDroppedHeaders[lower] {
			continue
		}
		out[lower] = strings.ReplaceAll(strings.Join(values, ", "), fmt.Sprintf(":%d", port), ":{PORT}")
	}
	return out
}

func normalizeServeBody(body []byte, port int) []byte {
	return bytes.ReplaceAll(body, []byte(fmt.Sprintf(":%d", port)), []byte(":{PORT}"))
}

// serveGoldenRoot copies a case directory's committed sample root and pins every
// mtime, the same way the recorder pinned the copy Local answered from.
func serveGoldenRoot(t *testing.T, caseDir string) string {
	t.Helper()
	mtime, err := time.Parse(time.RFC3339, serveGoldenMtime)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "root-a")
	if err := copyTree(filepath.Join(caseDir, "root"), root); err != nil {
		t.Fatal(err)
	}

	var files []string
	var dirs []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		} else {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, path := range dirs {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func copyTree(from, to string) error {
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o777)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o666)
	})
}

func serveFileList(t *testing.T, root string) []serveFileEntry {
	t.Helper()
	files := make([]serveFileEntry, 0, 8)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		files = append(files, serveFileEntry{Path: filepath.ToSlash(rel), SHA256: hex.EncodeToString(sum[:])})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}
