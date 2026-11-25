package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// agentTokenName is the file in the config folder a local program reads to call
// the agent lane.
const agentTokenName = "agent-token"

var agentTokenPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// loadAgentToken answers the agent lane's token from dir, writing a new one when
// the file is missing or does not hold a token. A new token is 32 random bytes as
// hex, written 0600 to a temporary file and renamed into place, so a reader never
// sees half of one.
func loadAgentToken(dir string) (token, file string, err error) {
	file = filepath.Join(dir, agentTokenName)
	data, err := os.ReadFile(file)
	if err == nil {
		if existing := strings.TrimSpace(string(data)); agentTokenPattern.MatchString(existing) {
			return existing, file, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", file, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", file, err
	}
	token = hex.EncodeToString(raw)
	tmp, err := os.CreateTemp(dir, agentTokenName+".*.tmp")
	if err != nil {
		return "", file, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", file, err
	}
	if _, err := tmp.WriteString(token + "\n"); err != nil {
		tmp.Close()
		return "", file, err
	}
	if err := tmp.Close(); err != nil {
		return "", file, err
	}
	if err := os.Rename(tmpName, file); err != nil {
		return "", file, err
	}
	return token, file, nil
}
