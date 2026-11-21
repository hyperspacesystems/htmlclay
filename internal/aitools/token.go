package aitools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/panphora/htmlclay/internal/runner"
)

// Token is the access token in Claude Code's own sign-in record.
type Token struct {
	Access  string
	Expires time.Time
}

// ErrNoToken is a missing or unreadable sign-in record.
var ErrNoToken = errors.New("aitools: Claude Code has no sign-in on this computer")

// ReadClaudeToken reads Claude Code's own sign-in record, again before every
// run because access tokens last hours: on macOS the keychain item "Claude
// Code-credentials" for this user, and ~/.claude/.credentials.json where
// there is no keychain record. The token is never logged or stored.
func ReadClaudeToken(ctx context.Context, home string) (Token, error) {
	if runtime.GOOS == "darwin" {
		if t, err := keychainToken(ctx, home); err == nil {
			return t, nil
		}
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return Token{}, ErrNoToken
	}
	return parseClaudeRecord(data)
}

func keychainToken(ctx context.Context, home string) (Token, error) {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res := run(ctx, proc{
		Argv: []string{"/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-a", name, "-w"},
		Dir:  home,
		Env:  runner.CleanEnv(home, "/usr/bin:/bin"),
	})
	if res.Err != nil {
		return Token{}, ErrNoToken
	}
	return parseClaudeRecord(res.Stdout)
}

func parseClaudeRecord(data []byte) (Token, error) {
	var record struct {
		ClaudeAIOauth struct {
			AccessToken string  `json:"accessToken"`
			ExpiresAt   float64 `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &record); err != nil || record.ClaudeAIOauth.AccessToken == "" {
		return Token{}, ErrNoToken
	}
	t := Token{Access: record.ClaudeAIOauth.AccessToken}
	if at := record.ClaudeAIOauth.ExpiresAt; at > 0 {
		if at > 1e12 {
			t.Expires = time.UnixMilli(int64(at))
		} else {
			t.Expires = time.Unix(int64(at), 0)
		}
	}
	return t, nil
}
