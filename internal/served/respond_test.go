package served

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespondOrderedKeys(t *testing.T) {
	body, err := jsonStringify(orderedObject{
		{Key: "msg", Value: "Cross-origin requests are not allowed."},
		{Key: "msgType", Value: "error"},
		{Key: "code", Value: "forbidden"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"msg":"Cross-origin requests are not allowed.","msgType":"error","code":"forbidden"}`
	if string(body) != want {
		t.Errorf("jsonStringify = %q, want %q", body, want)
	}
}

func TestRespondNotModifiedStripsEntityHeaders(t *testing.T) {
	etag := weakEntityETag([]byte("hello world"))
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("If-None-Match", etag)
	recorder := httptest.NewRecorder()
	sendText(recorder, request, http.StatusOK, "hello world")

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", recorder.Code)
	}
	if got := recorder.Body.Len(); got != 0 {
		t.Errorf("body length = %d, want 0", got)
	}
	header := recorder.Header()
	for _, name := range []string{"Content-Type", "Content-Length", "Transfer-Encoding"} {
		if got := header.Get(name); got != "" {
			t.Errorf("%s = %q, want it stripped", name, got)
		}
	}
	if got := header.Get("Etag"); got != etag {
		t.Errorf("etag = %q, want %q", got, etag)
	}
}

func TestRespondHeadHasHeadersAndNoBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	sendText(recorder, httptest.NewRequest("HEAD", "/", nil), http.StatusOK, "hello world")
	header := recorder.Header()
	if got := header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", got)
	}
	if got := header.Get("Content-Length"); got != "11" {
		t.Errorf("content-length = %q", got)
	}
	if got := header.Get("Etag"); got != weakEntityETag([]byte("hello world")) {
		t.Errorf("etag = %q", got)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body length = %d, want 0", recorder.Body.Len())
	}
}

func TestRespondStatus204HasNoBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	sendText(recorder, httptest.NewRequest("GET", "/", nil), http.StatusNoContent, "")
	if recorder.Code != http.StatusNoContent {
		t.Errorf("status = %d", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "" || recorder.Header().Get("Content-Length") != "" {
		t.Errorf("204 kept entity headers: %v", recorder.Header())
	}
}
