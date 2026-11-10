//go:build managedtrial

package managedtrial

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type goldenRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type goldenCase struct {
	Request    goldenRequest     `json:"request"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	BodyBase64 string            `json:"bodyBase64"`
}

var goldenIgnoredHeaders = map[string]bool{
	"date":       true,
	"connection": true,
	"keep-alive": true,
}

func TestExpressGolden(t *testing.T) {
	goldenDir := filepath.Join("testdata", "express-golden")
	fixturePath := filepath.Join(goldenDir, "fixture.txt")
	mtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(fixturePath, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(goldenDir, "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("golden.json holds no cases")
	}

	jsonBody, err := jsonStringify(orderedObject{
		{Key: "html", Value: "<b>&</b>"},
		{Key: "accent", Value: "é"},
		{Key: "emoji", Value: "🎉"},
		{Key: "line", Value: "\u2028"},
		{Key: "n", Value: 1},
		{Key: "ok", Value: true},
		{Key: "nested", Value: orderedObject{{Key: "a", Value: "x"}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/text":
			sendText(w, r, http.StatusOK, "hello world")
		case "/empty":
			sendText(w, r, http.StatusOK, "")
		case "/buffer":
			sendBytes(w, r, http.StatusOK, []byte{0, 1, 2, 254, 255})
		case "/json":
			sendJSON(w, r, http.StatusOK, jsonBody)
		case "/forbidden":
			sendText(w, r, http.StatusForbidden, "Invalid Host header")
		case "/missing":
			sendText(w, r, http.StatusNotFound, "File not found")
		case "/cookies":
			setSessionCookies(w)
			sendText(w, r, http.StatusOK, "ok")
		case "/file":
			sendFile(w, r, fixturePath, info, "text/plain; charset=UTF-8")
		default:
			sendText(w, r, http.StatusNotFound, "Not found")
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	for _, testCase := range cases {
		name := testCase.Request.Method + " " + testCase.Request.Path + " " + headerSummary(testCase.Request.Headers)
		t.Run(name, func(t *testing.T) {
			request, err := http.NewRequest(testCase.Request.Method, server.URL+testCase.Request.Path, nil)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range testCase.Request.Headers {
				request.Header.Set(key, value)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}

			if response.StatusCode != testCase.Status {
				t.Errorf("status = %d, want %d", response.StatusCode, testCase.Status)
			}

			got := make(map[string]string, len(response.Header))
			for name, values := range response.Header {
				lower := strings.ToLower(name)
				if goldenIgnoredHeaders[lower] {
					continue
				}
				got[lower] = strings.Join(values, ", ")
			}
			for name, want := range testCase.Headers {
				if got[name] != want {
					t.Errorf("header %q = %q, want %q", name, got[name], want)
				}
			}
			for name, value := range got {
				if _, ok := testCase.Headers[name]; !ok {
					t.Errorf("unexpected header %q = %q", name, value)
				}
			}

			wantBody, err := base64.StdEncoding.DecodeString(testCase.BodyBase64)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != string(wantBody) {
				t.Errorf("body = %q, want %q", body, wantBody)
			}
		})
	}
}

func headerSummary(headers map[string]string) string {
	if len(headers) == 0 {
		return "plain"
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	return strings.Join(keys, ",")
}
