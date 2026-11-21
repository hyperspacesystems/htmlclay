package server

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/commitrecord"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
)

// The file contract (plan 09 step H-r1, David's answer to question 10). A sheet that carries a
// commit record is saved by ClayJS's file-only client: every save is the next commit of the file it
// read, sent with that file's ETag. This host has no room, so these checks are all that keeps an old
// tab or a stray tool from rewinding a sheet other windows are saving to. A document with no record
// is untouched by all of it.

const orderedReloadMessage = "This sheet is open in more than one window. Reload it, then save again."

// recordIDs names the generations and commits this host's record folds write.
var recordIDs = commitrecord.NewIDs()

// orderedSaveRefusal judges a browser save of a document that carries a commit record: it must say
// which file it answers (an exact If-Match; the stamp itself is judged by the caller), and it must
// carry the file's record either unchanged (a page save that did not touch the sheet) or as the
// file's next commit. A body that drops the record or builds on another commit is refused, and the
// tab is told to reload. A record this host cannot read is not judged.
func orderedSaveRefusal(ifMatch string, sent bool, current, body []byte) (int, string) {
	tag, tagged := commitrecord.Find(current)
	if !tagged {
		return 0, ""
	}
	cur, err := commitrecord.Read(current, tag)
	if err != nil {
		return 0, ""
	}
	if !sent || strings.TrimSpace(ifMatch) == "*" || strings.Contains(ifMatch, ",") {
		return http.StatusPreconditionRequired, "if-match-required"
	}
	nextTag, ok := commitrecord.Find(body)
	if !ok {
		return http.StatusConflict, "not-next-commit"
	}
	if bytes.Equal(current[tag.BodyStart:tag.BodyEnd], body[nextTag.BodyStart:nextTag.BodyEnd]) {
		return 0, ""
	}
	next, err := commitrecord.Read(body, nextTag)
	if err != nil || next.DocID() != cur.DocID() || next.Parent == nil || *next.Parent != cur.Commit {
		return http.StatusConflict, "not-next-commit"
	}
	return 0, ""
}

// foldRecord gives a sheet's commit record a new generation when a writer other than a tab
// replaces the document: a version restore or a data API write (plan 09 section 2.5, HTML Clay: the
// record fold, with no room to back up). The open tabs then read the file, see the cut, and judge
// their unsaved edits against it. A record that cannot be read leaves the incoming bytes as they
// are, the weaker promise for a raw overwrite (contract rule 23).
func (s *Server) foldRecord(f *session.File, incoming, current []byte) []byte {
	if !commitrecord.Has(current) {
		return incoming
	}
	folded, err := commitrecord.OutsideFold(incoming, current, recordIDs)
	if err != nil {
		s.Logger.Printf("Could not fold the sheet record of %s: %v", f.RelPath, err)
		return incoming
	}
	return folded
}

// storedEtagOf is the stamp an outside edit's saved-lane frame carries: the ETag its caller computed
// from the stored bytes, for a document with a commit record.
func storedEtagOf(html, etag string) string {
	if !commitrecord.Has([]byte(html)) {
		return ""
	}
	return etag
}

// storedEtag is the stamp a saved-lane frame carries: the ETag of the stored bytes, for a document
// with a commit record, whose file-only client takes a push as the file plus its ETag. Other
// documents' frames stay exactly as they were.
func storedEtag(stored []byte) string {
	if !commitrecord.Has(stored) {
		return ""
	}
	return specwire.Etag(stored)
}

// FlushWait is how long a quit waits for the open sheets to save (plan v2 H7).
const FlushWait = 2 * time.Second

type flushPayload struct {
	Type string `json:"type"`
	Seq  int64  `json:"seq"`
}

// FlushOpenSheets runs before a quit closes the streams (H7): every stream on a document that
// carries a commit record gets one {"type":"flush"} frame, asking its tab to save what it holds now
// and then close the stream, and the quit waits until those streams have closed or wait has passed,
// while the HTTP server still takes the saves. Story: a person types into a sheet and quits HTML Clay
// from the tray a second later; without this the window's last edit has no host to save to. Only
// documents with a record get the frame, because only ClayJS's live-room client knows it.
func (ls *LiveSync) FlushOpenSheets(wait time.Duration) {
	deadline := time.Now().Add(wait)
	reached := ls.hub.sendFlush(func(key string) bool {
		data, err := os.ReadFile(key)
		return err == nil && commitrecord.Has(data)
	})
	for len(reached) > 0 && time.Now().Before(deadline) {
		if !ls.hub.anyOpen(reached) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
