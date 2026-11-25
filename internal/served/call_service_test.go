package served

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestM4CallService(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	var auth string
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		auth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"actor":{"username":"maya"}}`))
	}))
	t.Cleanup(service.Close)
	secret := strings.Repeat("cd", 32)
	host := switchTestHost(t)
	if err := host.SetServicePeer(service.URL, secret, "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	seenNow := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}

	t.Run("a sync route reaches the service with its bearer", func(t *testing.T) {
		var answer struct {
			OK    bool `json:"ok"`
			Actor struct {
				Username string `json:"username"`
			} `json:"actor"`
		}
		if err := host.CallService(context.Background(), "/sync/v1/discover", map[string]any{}, &answer); err != nil {
			t.Fatal(err)
		}
		if !answer.OK || answer.Actor.Username != "maya" {
			t.Fatalf("answer = %+v", answer)
		}
		if got := seenNow(); len(got) != 1 || got[0] != "POST /sync/v1/discover" {
			t.Fatalf("service saw %v", got)
		}
		mu.Lock()
		defer mu.Unlock()
		if auth != controlScheme+secret {
			t.Fatalf("Authorization = %q", auth)
		}
	})

	t.Run("only sync routes", func(t *testing.T) {
		before := len(seenNow())
		for _, path := range []string{"/v1/consult", "/sync/v1/../v1/consult", "/sync/v1/", "/sync/v1/a/b", "/sync/v2/discover", "sync/v1/discover", "/sync/v1/discover?x=1"} {
			if err := host.CallService(context.Background(), path, map[string]any{}, nil); !errors.Is(err, ErrNotSyncRoute) {
				t.Errorf("CallService(%q) = %v, want ErrNotSyncRoute", path, err)
			}
		}
		if got := seenNow(); len(got) != before {
			t.Fatalf("a refused path reached the service: %v", got[before:])
		}
	})

	t.Run("a switched off or missing service is unavailable", func(t *testing.T) {
		host.DetachService()
		err := host.CallService(context.Background(), "/sync/v1/discover", map[string]any{}, nil)
		host.AttachService()
		if !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("detached: %v", err)
		}
		host.ClearServicePeer()
		if err := host.CallService(context.Background(), "/sync/v1/discover", map[string]any{}, nil); !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("no peer: %v", err)
		}
		bare := NewHost(Options{})
		t.Cleanup(bare.Close)
		if err := bare.CallService(context.Background(), "/sync/v1/discover", map[string]any{}, nil); !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("no link: %v", err)
		}
	})
}
