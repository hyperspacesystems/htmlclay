package codeset

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// signBundle fills b's root fields and signs the statement, returning keys.json
// bytes as the root would publish them.
func signBundle(t *testing.T, root ed25519.PrivateKey, b KeyBundle) []byte {
	t.Helper()
	b.Schema = 1
	b.RootPublic = b64(root.Public().(ed25519.PublicKey))
	b.RootSig = b64(ed25519.Sign(root, KeysStatement(b)))
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// mutate edits a published key list without fixing its root signature, so the
// root no longer covers what the list says.
func mutate(t *testing.T, data []byte, edit func(map[string]any)) []byte {
	t.Helper()
	var b map[string]any
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	edit(b)
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func signDocument(t *testing.T, keyID string, key ed25519.PrivateKey, data []byte) Set {
	t.Helper()
	set, err := Of(data)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(key, Statement(keyID, "notes", "1.0.0", set.Hash))
	meta := `<meta name="clay-signature" content="v1 key=` + keyID + ` app=notes version=1.0.0 sig=` + b64(sig) + `">`
	set, err = Of([]byte(strings.Replace(string(data), `<meta charset="utf-8">`, `<meta charset="utf-8">`+meta, 1)))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestEmbeddedKeysValid(t *testing.T) {
	var b KeyBundle
	if err := json.Unmarshal(embeddedKeys, &b); err != nil {
		t.Fatalf("the embedded keys.json is not valid JSON: %v", err)
	}
	keys, err := ParseKeys(embeddedKeys)
	if err != nil {
		t.Fatalf("the embedded keys.json does not parse: %v", err)
	}
	if b.RootPublic != "" && len(keys.Signing) == 0 {
		t.Fatalf("a root-signed embedded keys.json must carry at least one signing key")
	}
	if len(Ecosystem.Signing) != len(keys.Signing) {
		t.Fatalf("Ecosystem has %d signing keys, embedded has %d", len(Ecosystem.Signing), len(keys.Signing))
	}
}

func TestParseKeysValidSignedBundleAndVerifyDocument(t *testing.T) {
	_, rootPriv := newKey(t)
	signPub, signPriv := newKey(t)
	const keyID = "clay-2027-01"

	keys, err := ParseKeys(signBundle(t, rootPriv, KeyBundle{Signing: map[string]string{keyID: b64(signPub)}}))
	if err != nil {
		t.Fatalf("a root-signed bundle does not parse: %v", err)
	}
	if got := keys.Signing[keyID]; !got.Equal(signPub) {
		t.Fatalf("signing key %s = %x, want %x", keyID, got, signPub)
	}

	set := signDocument(t, keyID, signPriv, read(t, "app.html"))
	sig, err := Verify(set, keys)
	if err != nil {
		t.Fatalf("a document signed by an ecosystem key does not verify: %v", err)
	}
	if sig != (Signature{Key: keyID, App: "notes", Version: "1.0.0"}) {
		t.Fatalf("Verify = %+v", sig)
	}

	signing := map[string]string{keyID: b64(signPub)}
	for name, b := range map[string]KeyBundle{
		"key":     {Signing: signing, RevokedKeys: []string{keyID}},
		"release": {Signing: signing, RevokedReleases: []string{"notes@1.0.0"}},
		"app":     {Signing: signing, RevokedReleases: []string{"notes@*"}},
		"code":    {Signing: signing, RevokedCodes: []string{set.Hash}},
	} {
		keys, err := ParseKeys(signBundle(t, rootPriv, b))
		if err != nil {
			t.Fatalf("%s: a revoked bundle does not parse: %v", name, err)
		}
		if _, err := Verify(set, keys); !errors.Is(err, ErrInvalid) {
			t.Errorf("revoked %s still verifies: %v", name, err)
		}
	}
}

func TestParseKeysRefusesTamperedOrUnsignedBundle(t *testing.T) {
	_, rootPriv := newKey(t)
	signPub, _ := newKey(t)
	const keyID = "clay-2027-01"

	signed := signBundle(t, rootPriv, KeyBundle{Signing: map[string]string{keyID: b64(signPub)}})

	cases := map[string][]byte{
		"tampered signing key": mutate(t, signed, func(b map[string]any) {
			b["signing"] = map[string]any{keyID: b64(make([]byte, ed25519.PublicKeySize))}
		}),
		"added key": mutate(t, signed, func(b map[string]any) {
			b["signing"] = map[string]any{keyID: b64(signPub), "sneaky-2027": b64(signPub)}
		}),
		"tampered revokedKeys": mutate(t, signed, func(b map[string]any) {
			b["revokedKeys"] = []string{keyID}
		}),
		"tampered revokedReleases": mutate(t, signed, func(b map[string]any) {
			b["revokedReleases"] = []string{"notes@1.0.0"}
		}),
		"tampered revokedCodes": mutate(t, signed, func(b map[string]any) {
			b["revokedCodes"] = []string{"00"}
		}),
		"schema 0":  mutate(t, signed, func(b map[string]any) { b["schema"] = 0 }),
		"schema 2":  mutate(t, signed, func(b map[string]any) { b["schema"] = 2 }),
		"no schema": mutate(t, signed, func(b map[string]any) { delete(b, "schema") }),
		"rootless key": mutate(t, signed, func(b map[string]any) {
			delete(b, "rootPublic")
			delete(b, "rootSig")
		}),
		"rootless revocation": mutate(t, signed, func(b map[string]any) {
			delete(b, "rootPublic")
			delete(b, "rootSig")
			b["revokedKeys"] = []string{keyID}
		}),
		"malformed root public": mutate(t, signed, func(b map[string]any) { b["rootPublic"] = "AAAA" }),
		"malformed root sig":    mutate(t, signed, func(b map[string]any) { b["rootSig"] = "AAAA" }),
		"not json":              []byte("{"),
		"malformed key id":      signBundle(t, rootPriv, KeyBundle{Signing: map[string]string{"bad id!": b64(signPub)}}),
		"truncated public key":  signBundle(t, rootPriv, KeyBundle{Signing: map[string]string{keyID: b64(signPub[:16])}}),
		"newline in revokedKey": signBundle(t, rootPriv, KeyBundle{
			Signing:     map[string]string{keyID: b64(signPub)},
			RevokedKeys: []string{"old\nrevoked-code:" + strings.Repeat("a", 64)},
		}),
		"duplicate revokedKey": signBundle(t, rootPriv, KeyBundle{
			Signing:     map[string]string{keyID: b64(signPub)},
			RevokedKeys: []string{keyID, keyID},
		}),
		"malformed revokedRelease": signBundle(t, rootPriv, KeyBundle{
			Signing:         map[string]string{keyID: b64(signPub)},
			RevokedReleases: []string{"journal"},
		}),
		"duplicate revokedRelease": signBundle(t, rootPriv, KeyBundle{
			Signing:         map[string]string{keyID: b64(signPub)},
			RevokedReleases: []string{"journal@*", "journal@*"},
		}),
		"uppercase revokedCode": signBundle(t, rootPriv, KeyBundle{
			Signing:      map[string]string{keyID: b64(signPub)},
			RevokedCodes: []string{strings.Repeat("A", 64)},
		}),
		"short revokedCode": signBundle(t, rootPriv, KeyBundle{
			Signing:      map[string]string{keyID: b64(signPub)},
			RevokedCodes: []string{"deadbeef"},
		}),
		"duplicate revokedCode": signBundle(t, rootPriv, KeyBundle{
			Signing:      map[string]string{keyID: b64(signPub)},
			RevokedCodes: []string{strings.Repeat("a", 64), strings.Repeat("a", 64)},
		}),
	}
	for name, data := range cases {
		if _, err := ParseKeys(data); err == nil {
			t.Errorf("%s parsed, want an error", name)
		}
	}
}

// TestVerifySignedFileEnv cross-checks a document signed elsewhere against a
// key list produced elsewhere:
//
//	CODESET_VERIFY_FILE=doc.html CODESET_VERIFY_KEYS=keys.json go test ./internal/codeset -run TestVerifySignedFileEnv
func TestVerifySignedFileEnv(t *testing.T) {
	docPath, keysPath := os.Getenv("CODESET_VERIFY_FILE"), os.Getenv("CODESET_VERIFY_KEYS")
	if docPath == "" || keysPath == "" {
		t.Skip("set CODESET_VERIFY_FILE and CODESET_VERIFY_KEYS to verify a signed file")
	}
	data, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ParseKeys(data)
	if err != nil {
		t.Fatalf("%s: %v", keysPath, err)
	}
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	set, err := Of(doc)
	if err != nil {
		t.Fatalf("%s: %v", docPath, err)
	}
	if _, err := Verify(set, keys); err != nil {
		t.Fatalf("%s: %v", docPath, err)
	}
}
