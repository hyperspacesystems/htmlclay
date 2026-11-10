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
	"strings"
	"testing"
	"time"
)

type serveRequest struct {
	Name            string            `json:"name"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Headers         map[string]string `json:"headers"`
	IfNoneMatchFrom string            `json:"ifNoneMatchFrom"`
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
}

type serveFileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type serveGolden struct {
	Cases []serveAnswer    `json:"cases"`
	Files []serveFileEntry `json:"files"`
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

	var golden serveGolden
	raw, err := os.ReadFile(filepath.Join(serveGoldenDir(), "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var requests []serveRequest
	raw, err = os.ReadFile(filepath.Join(serveGoldenDir(), "requests.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &requests); err != nil {
		t.Fatal(err)
	}
	if len(requests) == 0 || len(requests) != len(golden.Cases) {
		t.Fatalf("requests.json holds %d cases, golden.json %d", len(requests), len(golden.Cases))
	}

	root := serveGoldenRoot(t)
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
		headers := make(map[string]string, len(request.Headers)+1)
		for name, value := range request.Headers {
			headers[name] = value
		}
		if request.IfNoneMatchFrom != "" {
			from, ok := answered[request.IfNoneMatchFrom]
			if !ok || from.Headers["etag"] == "" {
				t.Fatalf("%s: no etag recorded for %q", request.Name, request.IfNoneMatchFrom)
			}
			headers["If-None-Match"] = from.Headers["etag"]
		}

		t.Run(request.Name, func(t *testing.T) {
			status, gotHeaders, body := serveSend(t, address, request.Method, request.Path, headers, port)
			if status != want.Status {
				t.Errorf("status = %d, want %d", status, want.Status)
			}
			if !reflect.DeepEqual(gotHeaders, want.Headers) {
				t.Errorf("headers = %v, want %v", gotHeaders, want.Headers)
			}
			wantBody := serveDecodeBody(t, want.Body)
			if !bytes.Equal(body, wantBody) {
				t.Errorf("body = %q, want %q", body, wantBody)
			}
			answered[request.Name] = serveAnswer{Name: request.Name, Status: status, Headers: gotHeaders}
		})
	}

	got := serveFileList(t, root)
	if !reflect.DeepEqual(got, golden.Files) {
		t.Errorf("the root's files changed: got %v, want %v", got, golden.Files)
	}
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
	var want []byte
	for _, answer := range golden.Cases {
		if answer.Name == "folder-slash" {
			want = serveDecodeBody(t, answer.Body)
		}
	}
	if want == nil {
		t.Fatal("golden.json holds no folder-slash answer")
	}
	got := renderDirListing("folder", []listingEntry{
		{Name: "app.html", IsDir: false},
		{Name: "notes.txt", IsDir: false},
	})
	if !bytes.Equal(got, want) {
		t.Errorf("renderDirListing = %q, want %q", got, want)
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
func serveSend(t *testing.T, address, method, path string, headers map[string]string, port int) (int, map[string]string, []byte) {
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
	request.WriteString("Connection: close\r\n\r\n")
	if _, err := conn.Write(request.Bytes()); err != nil {
		t.Fatal(err)
	}

	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: method})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, normalizeServeHeaders(response.Header, port), normalizeServeBody(body, port)
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

// serveGoldenRoot copies the committed sample root and pins every mtime, the
// same way gen.cjs pinned the copy Local answered from.
func serveGoldenRoot(t *testing.T) string {
	t.Helper()
	mtime, err := time.Parse(time.RFC3339, serveGoldenMtime)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "root-a")
	if err := copyTree(filepath.Join(serveGoldenDir(), "root"), root); err != nil {
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
