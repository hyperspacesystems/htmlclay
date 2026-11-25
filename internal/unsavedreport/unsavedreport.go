// Package unsavedreport is the body of POST /_/unsaved: an open tab saying
// whether it holds edits that are not saved yet (the unsaved-report-v1
// extension). Both hosts read it: a folder site with the document's save
// token in the path, a served folder with the document named by Document-URL.
package unsavedreport

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
)

// Extension is the name a host announces in /_/meta when it accepts reports.
const Extension = "unsaved-report-v1"

const maxBody = 1024

// Report is one tab's state. Tab is random per page load; Seq rises with each
// report from that tab, so a late report never overrides a newer one. Frozen
// says the browser is freezing the tab, which then runs no timers and cannot
// repeat its report until it wakes.
type Report struct {
	Tab     string `json:"tab"`
	Seq     int64  `json:"seq"`
	Unsaved *bool  `json:"unsaved"`
	Frozen  bool   `json:"frozen,omitempty"`
}

var tabID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Read decodes and checks a report body of at most 1 KB.
func Read(w http.ResponseWriter, r *http.Request) (Report, error) {
	var rep Report
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&rep); err != nil {
		return rep, errors.New("a report is a JSON object with tab, seq and unsaved")
	}
	if !tabID.MatchString(rep.Tab) || rep.Seq < 1 || rep.Unsaved == nil {
		return rep, errors.New("tab is 1 to 64 letters, digits, - or _; seq is a positive number; unsaved is true or false")
	}
	return rep, nil
}
