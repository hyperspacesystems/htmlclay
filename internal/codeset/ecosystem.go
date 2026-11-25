package codeset

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
)

var (
	releaseToken = regexp.MustCompile(`^[A-Za-z0-9._/@-]{1,64}@([A-Za-z0-9._/@-]{1,64}|\*)$`)
	codeToken    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// keys.json is the ecosystem key list compiled into this build. The root key
// signs it, so the list cannot grow or shrink without the root's consent.
//
//go:embed keys.json
var embeddedKeys []byte

// KeyBundle is keys.json on the wire: the root's public key, the signing keys
// it endorses, what it revoked, and the root's signature over all of it.
type KeyBundle struct {
	Schema          int               `json:"schema"`
	RootPublic      string            `json:"rootPublic"`
	Signing         map[string]string `json:"signing"`
	RevokedKeys     []string          `json:"revokedKeys"`
	RevokedReleases []string          `json:"revokedReleases"`
	RevokedCodes    []string          `json:"revokedCodes"`
	RootSig         string            `json:"rootSig"`
}

// KeysStatement is the text the root key signs to endorse b.
func KeysStatement(b KeyBundle) []byte {
	out := []byte("clay-keys-v1\n")
	ids := make([]string, 0, len(b.Signing))
	for id := range b.Signing {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, "signing:"+id+"="+b.Signing[id]+"\n"...)
	}
	for _, id := range sortedCopy(b.RevokedKeys) {
		out = append(out, "revoked-key:"+id+"\n"...)
	}
	for _, r := range sortedCopy(b.RevokedReleases) {
		out = append(out, "revoked-release:"+r+"\n"...)
	}
	for _, h := range sortedCopy(b.RevokedCodes) {
		out = append(out, "revoked-code:"+h+"\n"...)
	}
	return out
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// ParseKeys reads a root-signed key list. It answers Keys{} for an unsigned
// placeholder, and an error for anything the root did not sign.
// Note: ParseKeys trusts the RootPublic inside data (safe for the build-embedded
// keys.json); any future remotely fetched list must also pin RootPublic against
// the embedded build root.
func ParseKeys(data []byte) (Keys, error) {
	var b KeyBundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Keys{}, fmt.Errorf("codeset: malformed key list: %w", err)
	}
	if b.Schema != 1 {
		return Keys{}, fmt.Errorf("codeset: key list schema %d, want 1", b.Schema)
	}
	if b.RootPublic == "" {
		if len(b.Signing) != 0 || len(b.RevokedKeys) != 0 || len(b.RevokedReleases) != 0 || len(b.RevokedCodes) != 0 || b.RootSig != "" {
			return Keys{}, errors.New("codeset: unsigned key list cannot carry keys or revocations")
		}
		return Keys{}, nil
	}
	rootPub, err := base64.RawURLEncoding.DecodeString(b.RootPublic)
	if err != nil || len(rootPub) != ed25519.PublicKeySize {
		return Keys{}, errors.New("codeset: malformed root public key")
	}
	sig, err := base64.RawURLEncoding.DecodeString(b.RootSig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Keys{}, errors.New("codeset: malformed root signature")
	}
	keys := Keys{
		Signing:         make(map[string]ed25519.PublicKey, len(b.Signing)),
		RevokedKeys:     make(map[string]bool, len(b.RevokedKeys)),
		RevokedReleases: make(map[string]bool, len(b.RevokedReleases)),
		RevokedCodes:    make(map[string]bool, len(b.RevokedCodes)),
	}
	for id, value := range b.Signing {
		if !signatureToken.MatchString(id) {
			return Keys{}, fmt.Errorf("codeset: malformed key id %q", id)
		}
		public, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil || len(public) != ed25519.PublicKeySize {
			return Keys{}, fmt.Errorf("codeset: malformed public key for %s", id)
		}
		keys.Signing[id] = ed25519.PublicKey(public)
	}
	for _, id := range b.RevokedKeys {
		if !signatureToken.MatchString(id) || keys.RevokedKeys[id] {
			return Keys{}, fmt.Errorf("codeset: malformed or duplicate revoked key %q", id)
		}
		keys.RevokedKeys[id] = true
	}
	for _, r := range b.RevokedReleases {
		if !releaseToken.MatchString(r) || keys.RevokedReleases[r] {
			return Keys{}, fmt.Errorf("codeset: malformed or duplicate revoked release %q", r)
		}
		keys.RevokedReleases[r] = true
	}
	for _, h := range b.RevokedCodes {
		if !codeToken.MatchString(h) || keys.RevokedCodes[h] {
			return Keys{}, fmt.Errorf("codeset: malformed or duplicate revoked code %q", h)
		}
		keys.RevokedCodes[h] = true
	}
	if !ed25519.Verify(rootPub, KeysStatement(b), sig) {
		return Keys{}, errors.New("codeset: the root signature does not match the key list")
	}
	return keys, nil
}

func init() {
	keys, err := ParseKeys(embeddedKeys)
	if err != nil {
		panic("codeset: invalid embedded keys.json: " + err.Error())
	}
	Ecosystem = keys
}
