package codeset

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// A signature is one meta tag in the head:
//
//	<meta name="clay-signature" content="v1 key=<id> app=<app> version=<v> sig=<base64url>">
//
// The key signs Statement's text, which binds the key, the app, its version
// and the code set hash. Data never enters it, so saving data keeps a document
// signed, and any change to its code breaks the signature.

// Keys is the list of ecosystem keys HTML Clay accepts, and what it revoked.
type Keys struct {
	// Signing maps a key id to its public key.
	Signing map[string]ed25519.PublicKey
	// RevokedKeys are key ids whose signatures no longer count.
	RevokedKeys map[string]bool
	// RevokedReleases are "app@version" entries, or "app@*" for every version.
	RevokedReleases map[string]bool
	// RevokedCodes are code set hashes that no longer count, whoever signed them.
	RevokedCodes map[string]bool
}

// Ecosystem is the key list compiled into this build. It is empty until the
// owner creates the first signing key (milestone M4b), so no document is
// Ecosystem yet.
var Ecosystem = Keys{}

// Signature is who signed a document's code.
type Signature struct {
	Key     string
	App     string
	Version string
}

var (
	// ErrUnsigned means the head carries no clay-signature tag.
	ErrUnsigned = errors.New("codeset: the document is not signed")
	// ErrInvalid means a signature is there and does not count.
	ErrInvalid = errors.New("codeset: the signature does not match")
	// ErrUnpinnedScript means the document loads a script whose code the
	// signature cannot cover (Set.Unpinned), so the signature does not count.
	ErrUnpinnedScript = fmt.Errorf("%w: a script loads without integrity from outside clayjs.com and hyperclay.com", ErrInvalid)
)

var signatureToken = regexp.MustCompile(`^[A-Za-z0-9._/@-]{1,64}$`)

// Statement is the text a key signs for one release of one app's code.
func Statement(key, app, version, hash string) []byte {
	return []byte("clay-signature-v1\nkey=" + key + "\napp=" + app + "\nversion=" + version + "\ncode=" + hash + "\n")
}

// Verify checks set's one signature against keys. It answers ErrUnsigned when
// the head has no signature, and an error wrapping ErrInvalid for anything
// else that is not a valid, unrevoked signature: two signatures, a malformed
// one, an unknown or revoked key, a revoked release or code, a script the
// signature cannot cover (ErrUnpinnedScript), or a mismatch.
func Verify(set Set, keys Keys) (Signature, error) {
	switch len(set.Signatures) {
	case 0:
		return Signature{}, ErrUnsigned
	case 1:
	default:
		return Signature{}, fmt.Errorf("%w: %d signature tags in the head", ErrInvalid, len(set.Signatures))
	}
	sig, raw, err := parseSignature(set.Signatures[0])
	if err != nil {
		return Signature{}, err
	}
	public, ok := keys.Signing[sig.Key]
	if !ok || len(public) != ed25519.PublicKeySize {
		return Signature{}, fmt.Errorf("%w: unknown key %s", ErrInvalid, sig.Key)
	}
	switch {
	case keys.RevokedKeys[sig.Key]:
		return Signature{}, fmt.Errorf("%w: key %s is revoked", ErrInvalid, sig.Key)
	case keys.RevokedReleases[sig.App+"@"+sig.Version], keys.RevokedReleases[sig.App+"@*"]:
		return Signature{}, fmt.Errorf("%w: %s %s is revoked", ErrInvalid, sig.App, sig.Version)
	case keys.RevokedCodes[set.Hash]:
		return Signature{}, fmt.Errorf("%w: this code is revoked", ErrInvalid)
	case len(set.Unpinned) > 0:
		return Signature{}, fmt.Errorf("%w: %s", ErrUnpinnedScript, set.Unpinned[0])
	}
	if !ed25519.Verify(public, Statement(sig.Key, sig.App, sig.Version, set.Hash), raw) {
		return Signature{}, fmt.Errorf("%w: the code changed since it was signed", ErrInvalid)
	}
	return sig, nil
}

// parseSignature reads "v1 key=… app=… version=… sig=…", each field once, in
// any order after the version.
func parseSignature(content string) (Signature, []byte, error) {
	fields := strings.Fields(content)
	if len(fields) != 5 || fields[0] != "v1" {
		return Signature{}, nil, fmt.Errorf("%w: not a v1 signature", ErrInvalid)
	}
	values := map[string]string{}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok || values[k] != "" {
			return Signature{}, nil, fmt.Errorf("%w: malformed field %q", ErrInvalid, k)
		}
		values[k] = v
	}
	sig := Signature{Key: values["key"], App: values["app"], Version: values["version"]}
	for _, v := range []string{sig.Key, sig.App, sig.Version} {
		if !signatureToken.MatchString(v) {
			return Signature{}, nil, fmt.Errorf("%w: malformed key, app or version", ErrInvalid)
		}
	}
	raw, err := base64.RawURLEncoding.DecodeString(values["sig"])
	if err != nil || len(raw) != ed25519.SignatureSize {
		return Signature{}, nil, fmt.Errorf("%w: malformed sig", ErrInvalid)
	}
	return sig, raw, nil
}
