//go:build managedtrial

package managedtrial

// The commit core never talks to the data-loss guard, the live channel, the
// version store's observer or Tailwind itself: every call it makes into one of
// them goes through this one interface. The guard (G7), live sync (G9) and
// Tailwind (G11) implement it later; until then the binary runs noopCommitHooks
// and a test runs the recording implementation beside this file.

// dataLossGuardArgs is the object Local's lanes hand runDataLossGuard.
type dataLossGuardArgs struct {
	BaseDir     string
	Name        string
	NewHTML     string
	PrevContent *string
	Prov        string
}

// liveMessage is the payload of a saved-lane broadcast.
type liveMessage struct {
	HTML   string
	Sender string
}

// broadcastOptions is the lane of a broadcast.
type broadcastOptions struct {
	Lane string
}

// externalChange is the data the data API's notify carries when the document is
// small enough to send whole.
type externalChange struct {
	Kind    string
	HTML    string
	Sender  string
	Etag    string
	HasHTML bool
}

// liveNotification is Local's live.notify payload.
type liveNotification struct {
	MsgType string
	Msg     string
	Action  string
	Data    externalChange
}

// commitHooks is every side effect of a commit that is not the bytes on disk.
type commitHooks interface {
	// NoteSeen and MarkPending take the stored bytes the way Local holds them: a
	// string for a document, a byte slice for an upload.
	NoteSeen(baseDir, name string, stored any)
	MarkPending(baseDir, name string, stored any)
	RunDataLossGuard(args dataLossGuardArgs)
	ProvenanceForLocalSave(userDriven bool) string
	MarkBrowserSave(name string)
	Broadcast(name string, message liveMessage, options broadcastOptions)
	Notify(name string, message liveNotification)
	Emit(event string, fields map[string]any)
	RefreshTailwind(content string) error
	RefreshDerivedArtifacts(baseDir, name, content string) error
}

// noopCommitHooks is what the binary runs until G7, G9 and G11 attach the real
// implementations.
type noopCommitHooks struct{}

func (noopCommitHooks) NoteSeen(baseDir, name string, stored any)    {}
func (noopCommitHooks) MarkPending(baseDir, name string, stored any) {}
func (noopCommitHooks) RunDataLossGuard(args dataLossGuardArgs)      {}
func (noopCommitHooks) ProvenanceForLocalSave(userDriven bool) string {
	return localSaveProvenance(userDriven)
}
func (noopCommitHooks) MarkBrowserSave(name string)                                          {}
func (noopCommitHooks) Broadcast(name string, message liveMessage, options broadcastOptions) {}
func (noopCommitHooks) Notify(name string, message liveNotification)                         {}
func (noopCommitHooks) Emit(event string, fields map[string]any)                             {}
func (noopCommitHooks) RefreshTailwind(content string) error                                 { return nil }
func (noopCommitHooks) RefreshDerivedArtifacts(baseDir, name, content string) error {
	return nil
}

// localSaveProvenance is data-loss-guard.js's provenanceForLocalSave: a UI save
// is ui-gestured when a person did it and ui-background when a script did.
func localSaveProvenance(userDriven bool) string {
	if userDriven {
		return "ui-gestured"
	}
	return "ui-background"
}
