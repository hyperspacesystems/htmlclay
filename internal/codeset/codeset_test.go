package codeset

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the signed fixtures and testdata/vectors.json")

// The test key signs the vector fixtures only. Its seed is public on purpose.
const testKeyID = "test-2026-11"

func testKey() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("htmlclay codeset test key"))
	return ed25519.NewKeyFromSeed(seed[:])
}

func testKeys() Keys {
	return Keys{Signing: map[string]ed25519.PublicKey{testKeyID: testKey().Public().(ed25519.PublicKey)}}
}

type vector struct {
	File      string `json:"file"`
	Hash      string `json:"hash"`
	Parts     int    `json:"parts"`
	Signature string `json:"signature"`
}

type vectorFile struct {
	Version string            `json:"version"`
	TestKey map[string]string `json:"testKey"`
	Vectors []vector          `json:"vectors"`
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func setOf(t *testing.T, name string) Set {
	t.Helper()
	set, err := Of(read(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return set
}

func signatureState(set Set) string {
	_, err := Verify(set, testKeys())
	switch {
	case err == nil:
		return "valid"
	case errors.Is(err, ErrUnsigned):
		return "unsigned"
	case errors.Is(err, ErrUnpinnedScript):
		return "unpinned"
	default:
		return "invalid"
	}
}

// signTo writes from with a signature meta for its code set placed by place.
func signTo(t *testing.T, from, to string, place func(doc, meta string) string) {
	t.Helper()
	signText(t, string(read(t, from)), to, place)
}

func signText(t *testing.T, data, to string, place func(doc, meta string) string) {
	t.Helper()
	set, err := Of([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(testKey(), Statement(testKeyID, "notes", "1.0.0", set.Hash))
	meta := `<meta name="clay-signature" content="v1 key=` + testKeyID + ` app=notes version=1.0.0 sig=` + base64.RawURLEncoding.EncodeToString(sig) + `">`
	if err := os.WriteFile(filepath.Join("testdata", to), []byte(place(data, meta)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func inHead(doc, meta string) string {
	return strings.Replace(doc, `<meta charset="utf-8">`, `<meta charset="utf-8">`+meta, 1)
}

func writeSignedFixtures(t *testing.T) {
	signTo(t, "app.html", "signed.html", inHead)
	signTo(t, "app.html", "signed-moved-to-body.html", func(doc, meta string) string {
		return strings.Replace(doc, `<h1>Notes</h1>`, meta+`<h1>Notes</h1>`, 1)
	})
	signTo(t, "app.html", "signed-twice.html", func(doc, meta string) string { return inHead(inHead(doc, meta), meta) })
	app := string(read(t, "app.html"))
	for name, script := range map[string]string{
		"signed-unpinned-script.html":      `<script src="https://cdn.example.com/lib.js"></script>`,
		"signed-relative-script.html":      `<script src="lib.js"></script>`,
		"signed-unreadable-integrity.html": `<script src="https://cdn.example.com/lib.js" integrity="md5-AAAA"></script>`,
		"signed-svg-script.html":           `<svg><script href="https://cdn.example.com/lib.js"></script></svg>`,
		"signed-pinned-script.html":        `<script src="https://cdn.example.com/lib.js" integrity="sha384-oqVuAfXRKap7fdgcCY5uykM6+R9GqQ8K/uxy9rx7HNQlGYl1kPzQho1wx4JwY8wC"></script>`,
	} {
		signText(t, inHead(app, script), name, inHead)
	}
	signed := string(read(t, "signed.html"))
	meta := signed[strings.Index(signed, `<meta name="clay-signature"`):]
	meta = meta[:strings.Index(meta, ">")+1]
	saved := string(read(t, "app-saved.html"))
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join("testdata", name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("signed-saved.html", inHead(saved, meta))
	write("signed-handler-added.html", strings.Replace(signed, "<li>milk</li>", `<li onmouseover="steal()">milk</li>`, 1))
	write("signed-other-app.html", strings.Replace(signed, "app=notes", "app=kanban", 1))
}

func writeVectors(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("testdata", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	out := vectorFile{
		Version: Version,
		TestKey: map[string]string{"id": testKeyID, "public": base64.RawURLEncoding.EncodeToString(testKey().Public().(ed25519.PublicKey))},
	}
	for _, name := range names {
		set := setOf(t, filepath.Base(name))
		out.Vectors = append(out.Vectors, vector{File: filepath.Base(name), Hash: set.Hash, Parts: set.Parts, Signature: signatureState(set)})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "vectors.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVectors(t *testing.T) {
	if *update {
		writeSignedFixtures(t)
		writeVectors(t)
	}
	var file vectorFile
	if err := json.Unmarshal(read(t, "vectors.json"), &file); err != nil {
		t.Fatal(err)
	}
	if file.Version != Version {
		t.Fatalf("vectors are for %s, this is %s", file.Version, Version)
	}
	names, _ := filepath.Glob(filepath.Join("testdata", "*.html"))
	if len(file.Vectors) != len(names) || len(names) < 20 {
		t.Fatalf("%d vectors for %d fixtures; run go test -run TestVectors -update", len(file.Vectors), len(names))
	}
	for _, v := range file.Vectors {
		set := setOf(t, v.File)
		if set.Hash != v.Hash || set.Parts != v.Parts {
			t.Errorf("%s: hash %s parts %d, the vector says %s parts %d", v.File, set.Hash, set.Parts, v.Hash, v.Parts)
		}
		if got := signatureState(set); got != v.Signature {
			t.Errorf("%s: signature %s, the vector says %s", v.File, got, v.Signature)
		}
	}
}

func TestDataEditsKeepTheHash(t *testing.T) {
	app := setOf(t, "app.html")
	if app.Parts != 13 {
		t.Fatalf("app.html has %d code parts, want 13", app.Parts)
	}
	for _, name := range []string{"app-saved.html", "same-data-type-params.html", "signed.html", "signed-saved.html", "signed-moved-to-body.html"} {
		if got := setOf(t, name); got.Hash != app.Hash {
			t.Errorf("%s: hash %s, want app.html's %s", name, got.Hash, app.Hash)
		}
	}
	if plain := setOf(t, "plain.html"); plain.Parts != 0 {
		t.Errorf("plain.html has %d code parts, want none", plain.Parts)
	}
}

func TestEveryCodeChangeChangesTheHash(t *testing.T) {
	app := setOf(t, "app.html")
	names, _ := filepath.Glob(filepath.Join("testdata", "t-*.html"))
	if len(names) != 11 {
		t.Fatalf("%d tamper fixtures, want 11", len(names))
	}
	seen := map[string]string{app.Hash: "app.html"}
	for _, name := range names {
		set := setOf(t, filepath.Base(name))
		if other, dup := seen[set.Hash]; dup {
			t.Errorf("%s has the same code hash as %s", filepath.Base(name), other)
		}
		seen[set.Hash] = filepath.Base(name)
	}
}

func TestSignatures(t *testing.T) {
	cases := map[string]string{
		"app.html":                  "unsigned",
		"signed.html":               "valid",
		"signed-saved.html":         "valid",
		"signed-moved-to-body.html": "unsigned",
		"signed-twice.html":         "invalid",
		"signed-handler-added.html": "invalid",
		"signed-other-app.html":     "invalid",
		// A signature covers a script's URL, not what the URL serves, so it
		// counts only for scripts pinned with integrity or from HTML Clay's
		// own sites.
		"signed-unpinned-script.html":      "unpinned",
		"signed-relative-script.html":      "unpinned",
		"signed-unreadable-integrity.html": "unpinned",
		"signed-svg-script.html":           "unpinned",
		"signed-pinned-script.html":        "valid",
	}
	for name, want := range cases {
		if got := signatureState(setOf(t, name)); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	sig, err := Verify(setOf(t, "signed-saved.html"), testKeys())
	if err != nil || sig != (Signature{Key: testKeyID, App: "notes", Version: "1.0.0"}) {
		t.Fatalf("Verify = %+v, %v", sig, err)
	}
}

func TestScriptHosts(t *testing.T) {
	for src, want := range map[string]bool{
		"https://clayjs.com/v1/clay.js":   true,
		" https://HyperClay.com/x.js ":    true,
		"http://clayjs.com/v1/clay.js":    false,
		"//clayjs.com/v1/clay.js":         false,
		"https://clayjs.com:8443/x.js":    false,
		"https://clayjs.com.example/x.js": false,
		"https://x@clayjs.com/x.js":       false,
		"https:clayjs.com/x.js":           false,
	} {
		if got := fromScriptHost(src); got != want {
			t.Errorf("fromScriptHost(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestRevocationAndUnknownKeys(t *testing.T) {
	set := setOf(t, "signed.html")
	if _, err := Verify(set, Ecosystem); !errors.Is(err, ErrInvalid) {
		t.Fatalf("the built-in key list must not accept the test key: %v", err)
	}
	for name, keys := range map[string]Keys{
		"key":     {Signing: testKeys().Signing, RevokedKeys: map[string]bool{testKeyID: true}},
		"release": {Signing: testKeys().Signing, RevokedReleases: map[string]bool{"notes@1.0.0": true}},
		"app":     {Signing: testKeys().Signing, RevokedReleases: map[string]bool{"notes@*": true}},
		"code":    {Signing: testKeys().Signing, RevokedCodes: map[string]bool{set.Hash: true}},
	} {
		if _, err := Verify(set, keys); !errors.Is(err, ErrInvalid) {
			t.Errorf("revoked %s still verifies: %v", name, err)
		}
	}
}

func TestMalformedSignatures(t *testing.T) {
	good := setOf(t, "signed.html").Signatures[0]
	for _, content := range []string{
		"",
		"v2" + strings.TrimPrefix(good, "v1"),
		good + " extra=1",
		strings.Replace(good, "app=notes", "app=no tes", 1),
		strings.Replace(good, "app=notes", "key=again", 1),
		strings.Replace(good, "sig=", "sig=AA", 1),
	} {
		set := setOf(t, "app.html")
		set.Signatures = []string{content}
		if _, err := Verify(set, testKeys()); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q verified: %v", content, err)
		}
	}
}

func TestScriptURL(t *testing.T) {
	for value, want := range map[string]bool{
		"javascript:x":     true,
		" \x01JavaScript:": true,
		"java\tscr\nipt:x": true,
		"vbscript:x":       true,
		"data:text/html,x": true,
		"data:image/png,x": false,
		"https://x":        false,
		"#javascript:":     false,
	} {
		if got := IsScriptURL(value); got != want {
			t.Errorf("IsScriptURL(%q) = %v, want %v", value, got, want)
		}
	}
}
