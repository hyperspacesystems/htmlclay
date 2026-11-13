package shellapi

import (
	"strings"
	"testing"
)

func TestWriteFailed(t *testing.T) {
	var out strings.Builder
	if err := WriteFailed(&out, Failure{Code: FailPortTaken, Port: 1212}); err != nil {
		t.Fatalf("WriteFailed: %v", err)
	}
	want := "FAILED {\"v\":1,\"code\":\"port-taken\",\"port\":1212}\n"
	if got := out.String(); got != want {
		t.Fatalf("WriteFailed wrote %q, want %q", got, want)
	}
}
