package tier

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/codeset"
	"github.com/panphora/htmlclay/internal/platform"
)

const page = `<!DOCTYPE html><html><head><title>Notes</title><script src="https://clayjs.com/v1/clay.js"></script></head><body><ul><li>milk</li></ul><button onclick="add()">Add</button></body></html>`

func openStore(t *testing.T) *Store {
	t.Helper()
	s, created, err := Open(filepath.Join(t.TempDir(), "trust"))
	if err != nil || !created {
		t.Fatalf("Open = %v, created %v", err, created)
	}
	return s
}

func hashOf(t *testing.T, html string) string {
	t.Helper()
	set, err := codeset.Of([]byte(html))
	if err != nil {
		t.Fatal(err)
	}
	return set.Hash
}

func signed(t *testing.T, html string) (string, codeset.Keys) {
	t.Helper()
	seed := sha256.Sum256([]byte("tier test key"))
	key := ed25519.NewKeyFromSeed(seed[:])
	sig := ed25519.Sign(key, codeset.Statement("k1", "notes", "1.0.0", hashOf(t, html)))
	meta := `<meta name="clay-signature" content="v1 key=k1 app=notes version=1.0.0 sig=` + base64.RawURLEncoding.EncodeToString(sig) + `">`
	return strings.Replace(html, "<title>", meta+"<title>", 1),
		codeset.Keys{Signing: map[string]ed25519.PublicKey{"k1": key.Public().(ed25519.PublicKey)}}
}

func TestTiers(t *testing.T) {
	const doc = "/home/me/notes.html"
	withData := strings.Replace(page, "<li>milk</li>", "<li>milk</li><li>tea</li>", 1)
	signedPage, keys := signed(t, page)
	cases := []struct {
		name    string
		setup   func(s *Store)
		in      Input
		tier    Tier
		reason  string
		allowed bool
		denied  bool
	}{
		{name: "a file nobody vouches for", in: Input{}, tier: Unknown, reason: ReasonUnrecorded},
		{name: "developer mode", in: Input{Developer: true}, tier: Mine, reason: ReasonDeveloper},
		{name: "a download, even in developer mode", in: Input{Downloaded: true, Developer: true}, tier: Unknown, reason: ReasonDownloaded},
		{name: "a download whose mark is gone now",
			setup: func(s *Store) { s.NoteSeen(doc, true) },
			in:    Input{Developer: true}, tier: Unknown, reason: ReasonDownloaded},
		{name: "trusted code, downloaded and in a team folder",
			setup: func(s *Store) { s.TrustCode(hashOf(t, page), HowCommand) },
			in:    Input{Downloaded: true, Team: true}, tier: Mine, reason: ReasonTrustedCode},
		{name: "trusted code with other data",
			setup: func(s *Store) { s.TrustCode(hashOf(t, page), HowCommand) },
			in:    Input{Data: []byte(withData)}, tier: Mine, reason: ReasonTrustedCode},
		{name: "trusted path",
			setup: func(s *Store) { s.TrustPath(doc, false, HowCommand) },
			in:    Input{}, tier: Mine, reason: ReasonTrustedPath},
		{name: "trusted path replaced by a download",
			setup: func(s *Store) { s.TrustPath(doc, false, HowCommand) },
			in:    Input{Downloaded: true}, tier: Unknown, reason: ReasonDownloaded},
		{name: "a download the person trusted on purpose",
			setup: func(s *Store) { s.TrustPath(doc, true, HowAnswer) },
			in:    Input{Downloaded: true}, tier: Mine, reason: ReasonTrustedPath},
		{name: "a trusted path in a team folder",
			setup: func(s *Store) { s.TrustPath(doc, false, HowCommand) },
			in:    Input{Team: true}, tier: Team, reason: ReasonTeam},
		{name: "a team document allowed for this code",
			setup: func(s *Store) { s.Allow(doc, hashOf(t, page)) },
			in:    Input{Team: true}, tier: Team, reason: ReasonTeam, allowed: true},
		{name: "a team document whose code changed after the allow",
			setup: func(s *Store) { s.Allow(doc, hashOf(t, page)) },
			in:    Input{Team: true, Data: []byte(strings.Replace(page, "add()", "add(1)", 1))}, tier: Team, reason: ReasonTeam},
		{name: "an Unknown document the person allowed",
			setup: func(s *Store) { s.Allow(doc, hashOf(t, page)) },
			in:    Input{Downloaded: true}, tier: Unknown, reason: ReasonDownloaded, allowed: true},
		{name: "an allowed Unknown document whose code changed",
			setup: func(s *Store) { s.Allow(doc, hashOf(t, page)) },
			in:    Input{Downloaded: true, Data: []byte(strings.Replace(page, "add()", "add(1)", 1))}, tier: Unknown, reason: ReasonDownloaded},
		{name: "a stored Don't allow even in developer mode",
			setup: func(s *Store) { s.Deny(doc) },
			in:    Input{Developer: true}, tier: Unknown, reason: ReasonUnrecorded, denied: true},
		{name: "a download in a team folder",
			in: Input{Downloaded: true, Team: true}, tier: Unknown, reason: ReasonDownloaded},
		{name: "an Unknown document the person refused, whatever its code",
			setup: func(s *Store) { s.Deny(doc) },
			in:    Input{Downloaded: true, Data: []byte(strings.Replace(page, "add()", "add(1)", 1))}, tier: Unknown, reason: ReasonDownloaded, denied: true},
		{name: "signed by a known key, downloaded",
			in: Input{Data: []byte(signedPage), Keys: keys, Downloaded: true}, tier: Ecosystem, reason: ReasonSigned},
		{name: "signed by a key this build does not know",
			in: Input{Data: []byte(signedPage), Downloaded: true}, tier: Unknown, reason: ReasonDownloaded},
		{name: "a signature that does not count, even in developer mode",
			in: Input{Data: []byte(signedPage), Developer: true}, tier: Unknown, reason: ReasonBrokenSignature},
		{name: "a signed page whose code changed",
			in: Input{Data: []byte(strings.Replace(signedPage, "add()", "add(1)", 1)), Keys: keys, Developer: true}, tier: Unknown, reason: ReasonBrokenSignature},
		{name: "too large to read", in: Input{Data: make([]byte, codeset.MaxDocument+1), Developer: true}, tier: Unknown, reason: ReasonUnreadable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := openStore(t)
			if c.setup != nil {
				c.setup(s)
			}
			in := c.in
			in.Path = doc
			if in.Data == nil {
				in.Data = []byte(page)
			}
			v := s.Of(in)
			if v.Tier != c.tier || v.Reason != c.reason || v.Allowed != c.allowed || v.Denied != c.denied {
				t.Fatalf("Of = %s %s allowed=%v denied=%v, want %s %s allowed=%v denied=%v", v.Tier, v.Reason, v.Allowed, v.Denied, c.tier, c.reason, c.allowed, c.denied)
			}
			if v.Warn() != (!v.Trusted() && !c.allowed) {
				t.Fatalf("Warn = %v for %s allowed=%v", v.Warn(), v.Tier, c.allowed)
			}
		})
	}
}

func TestNoteKeepsTheFirstSight(t *testing.T) {
	s := openStore(t)
	const doc = "/home/me/Downloads/app.html"
	if err := s.Note(Sight{Path: doc, Downloaded: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Note(Sight{Path: doc, Developer: true}); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.Seen(doc); !seen.Downloaded {
		t.Fatal("a later sight without the mark rewrote the first one")
	}
	if _, ok := s.PathTrust(doc); ok {
		t.Fatal("a downloaded document was recorded as the person's")
	}
	if v := s.Of(Input{Path: doc, Data: []byte(page), Developer: true}); v.Tier != Unknown {
		t.Fatalf("a download whose mark was erased after first sight is %s", v.Tier)
	}
}

func TestNoteRecordsDeveloperFiles(t *testing.T) {
	s := openStore(t)
	const doc = "/home/me/code/tool.html"
	if err := s.Note(Sight{Path: doc, Developer: true}); err != nil {
		t.Fatal(err)
	}
	if v := s.Of(Input{Path: doc, Data: []byte(page)}); v.Tier != Mine || v.Reason != ReasonTrustedPath {
		t.Fatalf("after developer mode is turned off the file is %s %s", v.Tier, v.Reason)
	}
	const tampered = "/home/me/code/signed.html"
	if err := s.Note(Sight{Path: tampered, Developer: true, BrokenSignature: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.PathTrust(tampered); ok {
		t.Fatal("developer mode recorded a document whose signature does not count as the person's")
	}
	// A copy written without the signature tag keeps the doubt: dropping the
	// tag must not turn a tampered document into the developer's own.
	const copied = "/home/me/code/signed-new.html"
	if err := s.NoteCopy(tampered, copied); err != nil {
		t.Fatal(err)
	}
	if v := s.Of(Input{Path: copied, Data: []byte(page), Developer: true}); v.Tier != Unknown || v.Reason != ReasonBrokenSignature {
		t.Fatalf("an unsigned copy of a tampered document is %s %s", v.Tier, v.Reason)
	}
}

func TestNoteRecordsACopyOfTrustedCode(t *testing.T) {
	s := openStore(t)
	const doc = "/home/me/moved/notes.html"
	s.TrustCode(hashOf(t, page), HowCreated)
	if err := s.Note(Sight{Path: doc, Code: hashOf(t, page)}); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(page, "add()", "add(2)", 1)
	if v := s.Of(Input{Path: doc, Data: []byte(edited)}); v.Tier != Mine {
		t.Fatalf("a moved trusted file edited outside HTML Clay is %s", v.Tier)
	}
}

func TestNoteUpgrade(t *testing.T) {
	s := openStore(t)
	folder := t.TempDir()
	if err := s.MarkUpgrade([]string{folder}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUpgrade(nil); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.Upgrade(); len(u.Folders) != 1 {
		t.Fatalf("a second MarkUpgrade replaced the record: %+v", u)
	}
	before := time.Now().Add(-time.Hour)
	after := time.Now().Add(time.Hour)
	cases := []struct {
		path    string
		changed time.Time
		marked  bool
		mine    bool
	}{
		{filepath.Join(folder, "old.html"), before, false, true},
		{filepath.Join(folder, "sub", "old.html"), before, false, true},
		{filepath.Join(folder, "new.html"), after, false, false},
		{filepath.Join(folder, "downloaded.html"), before, true, false},
		{filepath.Join(t.TempDir(), "elsewhere.html"), before, false, false},
	}
	for _, c := range cases {
		if err := s.Note(Sight{Path: c.path, Changed: c.changed, Downloaded: c.marked}); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.PathTrust(c.path); ok != c.mine {
			t.Errorf("%s changed %v marked %v: recorded %v, want %v", c.path, c.changed, c.marked, ok, c.mine)
		}
	}
}

// TestNoteUpgradeIgnoresAChangeTime covers the later of the modification time
// and the creation time: a file whose attributes were touched after the upgrade,
// with its bytes unchanged, was still there before the upgrade and is the
// person's own.
func TestNoteUpgradeIgnoresAChangeTime(t *testing.T) {
	s := openStore(t)
	folder := t.TempDir()
	path := filepath.Join(folder, "old.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	// On Windows, Chtimes moves modification time but leaves NTFS creation time
	// at WriteFile's clock tick; let the wall clock advance past it before the
	// upgrade timestamp is recorded.
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writtenBefore := platform.WrittenAt(beforeInfo)
	for !time.Now().Round(0).After(writtenBefore) {
		time.Sleep(5 * time.Millisecond)
	}
	if err := s.MarkUpgrade([]string{folder}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Note(Sight{Path: path, Code: hashOf(t, page), Changed: platform.WrittenAt(info)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.PathTrust(path); !ok {
		t.Fatal("a file whose change time moved after the upgrade is no longer the person's")
	}
}

func TestRecordsShareOnePathOnADiskThatIgnoresCase(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("letter case names one file only on a disk that ignores it")
	}
	s := openStore(t)
	if err := s.TrustPath("/home/me/Doc.html", false, HowCommand); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.PathTrust("/home/me/doc.html"); !ok {
		t.Fatal("Doc.html and doc.html kept separate records")
	}
}

func TestForgetOfAMissingFileTakesBackItsHashes(t *testing.T) {
	s := openStore(t)
	hash := hashOf(t, page)
	missing := filepath.Join(t.TempDir(), "gone.html")
	if err := s.TrustPath(missing, false, HowCommand); err != nil {
		t.Fatal(err)
	}
	if err := s.TrustCodeViaPath(hash, missing, HowCommand); err != nil {
		t.Fatal(err)
	}
	if err := s.Allow(missing, hash); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(missing, ""); err != nil {
		t.Fatalf("untrust of a missing file = %v", err)
	}
	if _, ok := s.PathTrust(missing); ok {
		t.Fatal("untrust kept the path record of a missing file")
	}
	if _, ok := s.Answer(missing); ok {
		t.Fatal("untrust kept the answer for a missing file")
	}
	if s.CodeTrusted(hash) {
		t.Fatal("untrust kept a code set trusted through the missing file")
	}
}

func TestForgetKeepsAHashAnotherPathLists(t *testing.T) {
	s := openStore(t)
	hash := hashOf(t, page)
	first := "/home/me/one.html"
	second := "/home/me/two.html"
	for _, path := range []string{first, second} {
		if err := s.TrustPath(path, false, HowCommand); err != nil {
			t.Fatal(err)
		}
		if err := s.TrustCodeViaPath(hash, path, HowCommand); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Forget(first, ""); err != nil {
		t.Fatal(err)
	}
	if !s.CodeTrusted(hash) {
		t.Fatal("untrusting one document took back a code set another trusted document lists")
	}
	if err := s.Forget(second, ""); err != nil {
		t.Fatal(err)
	}
	if s.CodeTrusted(hash) {
		t.Fatal("untrusting the last document that lists a code set kept it")
	}
}

func TestForget(t *testing.T) {
	s := openStore(t)
	const doc = "/home/me/notes.html"
	s.Note(Sight{Path: doc, Downloaded: true})
	s.TrustPath(doc, true, HowAnswer)
	s.TrustCode(hashOf(t, page), HowAnswer)
	s.Deny(doc)
	if err := s.Forget(doc, hashOf(t, page)); err != nil {
		t.Fatal(err)
	}
	v := s.Of(Input{Path: doc, Data: []byte(page)})
	if v.Tier != Unknown || v.Reason != ReasonDownloaded || v.Allowed || v.Denied {
		t.Fatalf("after Forget: %s %s allowed=%v denied=%v", v.Tier, v.Reason, v.Allowed, v.Denied)
	}
	if err := s.Allow(doc, ""); err == nil {
		t.Fatal("Allow took an allow with no code")
	}
}

func TestProgramsFolderSwitch(t *testing.T) {
	s := openStore(t)
	if s.Programs() {
		t.Fatal("the programs folder is on in a new store")
	}
	s.SetDeveloper(true)
	if !s.Programs() {
		t.Fatal("developer mode did not switch the programs folder on")
	}
	s.SetDeveloper(false)
	s.SetPrograms(true)
	if !s.Programs() || s.Developer() {
		t.Fatalf("programs enable: programs=%v developer=%v", s.Programs(), s.Developer())
	}
}

func TestRecordsAreKeyedByExactPath(t *testing.T) {
	s := openStore(t)
	s.TrustPath("/home/me/a.html", false, HowCommand)
	if _, ok := s.PathTrust("/home/me/b.html"); ok {
		t.Fatal("a record answered for another path")
	}
	if _, ok := s.PathTrust("/home/me/x/../a.html"); !ok {
		t.Fatal("a record did not answer for the same path spelled with ..")
	}
	if err := s.TrustCode("not-a-hash", HowCommand); err == nil {
		t.Fatal("TrustCode took a value that is not a hash")
	}
}

func TestOpenTwice(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	if _, created, _ := Open(dir); !created {
		t.Fatal("first Open must report created")
	}
	if _, created, _ := Open(dir); created {
		t.Fatal("second Open must not report created")
	}
}

func TestDeniedDocumentsPreservesDisplayCaseAndClearDenyRemovesIt(t *testing.T) {
	s := openStore(t)
	doc := filepath.Join(t.TempDir(), "MyStrangerApp.html")
	if err := os.WriteFile(doc, []byte("<!doctype html><script>1</script>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Deny(doc); err != nil {
		t.Fatal(err)
	}
	got := s.DeniedDocuments()
	if len(got) != 1 || got[0] != doc {
		t.Fatalf("DeniedDocuments = %v, want [%s]", got, doc)
	}
	if err := s.ClearDeny(doc); err != nil {
		t.Fatal(err)
	}
	if after := s.DeniedDocuments(); len(after) != 0 {
		t.Fatalf("DeniedDocuments after ClearDeny = %v, want empty", after)
	}
}

func TestRebindByCaseKeepsRecordsAndRenamesTheDenial(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		s := openStore(t)
		const before, after = "/home/me/Notes.html", "/home/me/notes.html"
		if err := s.TrustPath(before, false, HowCreated); err != nil {
			t.Fatal(err)
		}
		if _, err := s.NoteSeen(before, false); err != nil {
			t.Fatal(err)
		}
		if err := s.Deny(before); err != nil {
			t.Fatal(err)
		}
		if err := s.Rebind(before, after); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.PathTrust(after); !ok {
			t.Fatal("a rename that only changes case lost the path record")
		}
		if _, ok := s.Seen(after); !ok {
			t.Fatal("a rename that only changes case lost the first sight")
		}
		if rec, ok := s.Answer(after); !ok || !rec.Denied {
			t.Fatalf("a rename that only changes case lost the Don't allow: %+v, %v", rec, ok)
		}
	}

	s := openStore(t)
	if err := s.Deny("/home/me/old.html"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rebind("/home/me/old.html", "/home/me/new.html"); err != nil {
		t.Fatal(err)
	}
	denied := s.DeniedDocuments()
	if len(denied) != 1 || filepath.Base(denied[0]) != "new.html" {
		t.Fatalf("denied after rename = %v, want new.html", denied)
	}
	if err := s.ClearDeny("/home/me/new.html"); err != nil {
		t.Fatal(err)
	}
	if denied := s.DeniedDocuments(); len(denied) != 0 {
		t.Fatalf("denied after clearing = %v, want none", denied)
	}
}
