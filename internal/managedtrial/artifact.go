//go:build managedtrial

package managedtrial

import "net/http"

// controlArtifactBody is one POST /v1/artifact.
type controlArtifactBody struct {
	OpID       string           `json:"opId"`
	Root       string           `json:"root"`
	Name       string           `json:"name"`
	ArtifactID string           `json:"artifactId"`
	Type       string           `json:"type"`
	Logical    controlBytesBody `json:"logical"`
}

// serveArtifact is POST /v1/artifact: a recovered copy the service produced
// outside any consultation, written under the document's lock with Local's
// naming and answered with its receipt. A safety version exists only inside
// the write it protects, so it is refused here.
func (a *controlAPI) serveArtifact(w http.ResponseWriter, r *http.Request) {
	var body controlArtifactBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	link := root.commit.service
	if link == nil {
		writeControlJSON(w, http.StatusNotFound, controlError{Error: "artifacts need the service"})
		return
	}
	if body.Type != "recovered-copy" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "only a recovered-copy is written outside a consultation"})
		return
	}
	filePath, err := root.commit.resolveWrite(body.Name)
	if err != nil {
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	opID, ok := claimControlOp(w, root.events, body.OpID)
	if !ok {
		return
	}
	unlock := root.commit.locks.lock(filePath)
	defer unlock()

	carrier := link.begin(opID, nil, root.id, root.baseDir, body.Name, filePath, "artifact", "service")
	receipt := carrier.writeArtifact(serviceArtifact{
		ArtifactID: body.ArtifactID, Type: body.Type, Root: root.id, Name: body.Name, Logical: body.Logical,
	})
	state := "committed"
	if receipt.Error != nil {
		state = "refused"
	}
	root.events.finishOp(opID, state, "", "")
	writeControlJSON(w, http.StatusOK, receipt)
}
