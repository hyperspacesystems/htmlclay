// Package tier decides who vouches for a document's code, which decides
// whether HTML Clay asks before the document's programs or AI reach a folder.
//
// Trust follows the code set (internal/codeset), never the data, so editing
// data never moves a document between tiers:
//
//   - Mine: the person trusts this code (a trusted code set), or this document
//     at its path (HTML Clay made it, htmlclay trust, the warning's second
//     answer, developer mode, or the upgrade). No dialogs.
//   - Ecosystem: the code carries a valid signature from a key compiled into
//     HTML Clay. No dialogs.
//   - Team: a document in a team folder whose code is not trusted. Asks once
//     per code change.
//   - Unknown: everything else: downloaded, carrying a signature that does not
//     count, or never recorded. One warning per document, remembered.
package tier

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/codeset"
	"github.com/panphora/htmlclay/internal/pathrel"
)

type Tier int

const (
	Unknown Tier = iota
	Team
	Ecosystem
	Mine
)

func (t Tier) String() string {
	switch t {
	case Mine:
		return "mine"
	case Ecosystem:
		return "ecosystem"
	case Team:
		return "team"
	}
	return "unknown"
}

// Why a document has its tier.
const (
	ReasonTrustedCode = "trusted-code"
	ReasonTrustedPath = "trusted-path"
	ReasonDeveloper   = "developer"
	ReasonSigned      = "signed"
	ReasonTeam        = "team"
	ReasonDownloaded  = "downloaded"
	// ReasonBrokenSignature is a document that carries a clay-signature that
	// does not count: its code changed since it was signed, its key is unknown
	// or revoked, or it loads a script the signature cannot cover.
	ReasonBrokenSignature = "broken-signature"
	ReasonUnrecorded      = "unrecorded"
	ReasonUnreadable      = "unreadable"
)

// Verdict is a document's tier and what it rests on.
type Verdict struct {
	Tier   Tier
	Reason string
	// Code is the document's code set hash, empty when it could not be read.
	Code string
	// Allowed is the person's "Allow for this document", given for the
	// document's current code.
	Allowed bool
	// Denied is the person's "Don't allow" for this document, whatever its
	// code. It stays until htmlclay untrust.
	Denied bool
	// Signed names the signer of an Ecosystem document.
	Signed codeset.Signature
}

// Trusted reports a document whose code someone vouches for: Mine or Ecosystem.
func (v Verdict) Trusted() bool { return v.Tier == Mine || v.Tier == Ecosystem }

// Warn reports a document that must show the Unknown warning before its
// programs or AI reach a folder.
func (v Verdict) Warn() bool { return !v.Trusted() && !v.Allowed }

// Input is what HTML Clay knows about a document at one moment.
type Input struct {
	// Path is the document's absolute path, symlinks resolved.
	Path string
	// Data is the document's bytes.
	Data []byte
	// Set is the document's code set when the caller already has it, so an
	// unchanged document is not parsed again. Nil means Of reads it from Data.
	Set *codeset.Set
	// Unreadable is a document HTML Clay could not read.
	Unreadable bool
	// Team is a document in a team folder, or one whose team record cannot
	// be read.
	Team bool
	// Downloaded is the download mark on the file now.
	Downloaded bool
	// Developer is developer mode.
	Developer bool
	// Keys are the ecosystem keys signatures are checked against.
	Keys codeset.Keys
}

// Of decides the document's tier. It only reads the store; Note writes.
//
// The order is the promise: someone vouching for the code comes first; the
// person's decision to trust this path on purpose outranks its download mark;
// a stored Don't allow outranks every doubt; a download mark outranks the
// document's folder; a team folder outranks a path trusted while unmarked; a
// signature that does not count outranks anything left; and developer mode
// comes last, because it acts only through the first sight.
func (s *Store) Of(in Input) Verdict {
	set, ok := in.codeSet()
	if !ok {
		return s.unknown(in.Path, "", ReasonUnreadable)
	}
	if s.CodeTrusted(set.Hash) {
		return Verdict{Tier: Mine, Reason: ReasonTrustedCode, Code: set.Hash}
	}
	sig, sigErr := codeset.Verify(set, in.Keys)
	if sigErr == nil {
		return Verdict{Tier: Ecosystem, Reason: ReasonSigned, Code: set.Hash, Signed: sig}
	}
	seen, _ := s.Seen(in.Path)
	doubt := ReasonUnrecorded
	switch {
	case in.Downloaded || seen.Downloaded:
		doubt = ReasonDownloaded
	case BrokenSignature(sigErr):
		doubt = ReasonBrokenSignature
	}
	rec, trustedPath := s.PathTrust(in.Path)
	// A document the person trusted on purpose keeps its path trust even
	// though it carries a download mark.
	if trustedPath && rec.Marked {
		return Verdict{Tier: Mine, Reason: ReasonTrustedPath, Code: set.Hash}
	}
	if a, ok := s.Answer(in.Path); ok && a.Denied {
		return Verdict{Tier: Unknown, Reason: doubt, Code: set.Hash, Denied: true}
	}
	if doubt == ReasonDownloaded {
		return s.unknown(in.Path, set.Hash, doubt)
	}
	if in.Team {
		return s.answered(Verdict{Tier: Team, Reason: ReasonTeam, Code: set.Hash}, in.Path)
	}
	if trustedPath {
		return Verdict{Tier: Mine, Reason: ReasonTrustedPath, Code: set.Hash}
	}
	if doubt != ReasonUnrecorded {
		return s.unknown(in.Path, set.Hash, doubt)
	}
	if in.Developer {
		return Verdict{Tier: Mine, Reason: ReasonDeveloper, Code: set.Hash}
	}
	return s.unknown(in.Path, set.Hash, ReasonUnrecorded)
}

func (in Input) codeSet() (codeset.Set, bool) {
	switch {
	case in.Unreadable:
		return codeset.Set{}, false
	case in.Set != nil:
		return *in.Set, true
	case len(in.Data) > codeset.MaxDocument:
		return codeset.Set{}, false
	}
	set, err := codeset.Of(in.Data)
	return set, err == nil
}

// BrokenSignature reports a codeset.Verify result for a document that carries
// a signature that does not count.
func BrokenSignature(verifyErr error) bool {
	return verifyErr != nil && !errors.Is(verifyErr, codeset.ErrUnsigned)
}

func (s *Store) unknown(path, code, reason string) Verdict {
	return s.answered(Verdict{Tier: Unknown, Reason: reason, Code: code}, path)
}

// answered adds the person's last answer for the document at path.
func (s *Store) answered(v Verdict, path string) Verdict {
	if a, ok := s.Answer(path); ok {
		v.Denied = a.Denied
		v.Allowed = !a.Denied && v.Code != "" && a.Code == v.Code
	}
	return v
}

// Sight is a document as HTML Clay first opens it.
type Sight struct {
	Path string
	// Code is the document's code set hash, empty when unreadable.
	Code       string
	Downloaded bool
	// BrokenSignature is a document carrying a signature that does not count.
	BrokenSignature bool
	Developer       bool
	// Changed is the later of the file's modification and creation times.
	Changed time.Time
}

// Note records the first sight of a document, before anything HTML Clay does
// can erase its download mark, and records it as the person's when nothing
// else vouches for it and one of these holds: its code is already trusted (a
// copy or a move), developer mode is on, or it sat unchanged in a folder the
// person trusted when this store was made. A downloaded file, or one whose
// signature does not count, is never recorded as the person's here. Later
// sights change nothing.
func (s *Store) Note(in Sight) error {
	first, err := s.noteSeen(in.Path, in.Downloaded, in.Developer)
	if err != nil || !first || in.Downloaded {
		return err
	}
	if _, ok := s.PathTrust(in.Path); ok {
		return nil
	}
	switch {
	case in.Code != "" && s.CodeTrusted(in.Code):
		return s.TrustPath(in.Path, false, HowCopy)
	case in.BrokenSignature:
		return nil
	case in.Developer:
		return s.TrustPath(in.Path, false, HowDeveloper)
	case s.upgradeCovers(in.Path, in.Changed):
		return s.TrustPath(in.Path, false, HowUpgrade)
	}
	return nil
}

// upgradeCovers reports a file that sat, unchanged since, in a folder the
// person trusted at the upgrade.
func (s *Store) upgradeCovers(path string, changed time.Time) bool {
	u, ok := s.Upgrade()
	if !ok || changed.IsZero() || !changed.Before(u.At) {
		return false
	}
	for _, folder := range u.Folders {
		if pathrel.SameOrBelow(filepath.Clean(path), filepath.Clean(folder)) {
			return true
		}
	}
	return false
}
