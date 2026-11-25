package unsavedreport

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadChecksTheBody(t *testing.T) {
	oversized := `{"tab":"a","seq":1,"unsaved":true,"x":"` + strings.Repeat("x", 1100) + `"}`
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"valid", `{"tab":"a1-_B","seq":1,"unsaved":true}`, true},
		{"empty tab", `{"tab":"","seq":1,"unsaved":true}`, false},
		{"tab with a space", `{"tab":"a b","seq":1,"unsaved":true}`, false},
		{"zero seq", `{"tab":"a","seq":0,"unsaved":true}`, false},
		{"no unsaved", `{"tab":"a","seq":1}`, false},
		{"not json", `not json`, false},
		{"oversized", oversized, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			_, err := Read(w, r)
			if tc.ok && err != nil {
				t.Fatalf("Read(%s) = %v, want accepted", tc.body, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("Read(%s) = nil error, want refused", tc.body)
			}
		})
	}
}
