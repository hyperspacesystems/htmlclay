package served

import (
	"io/fs"
	"regexp"
	"strings"
	"time"
)

const (
	shutdownTimeout = 5 * time.Second
	readHeaderLimit = 10 * time.Second
)

// liveRoot names the folder no root may touch, when there is one. Only the trial binary sets it (rootguard_trial.go); tests replace it.
var liveRoot func() (string, error)

type config struct {
	roots       []string
	port        int
	localSource string
	assets      fs.FS
	// compileTailwind turns one document into its generated stylesheet. Nil
	// means the Node compile over localSource.
	compileTailwind func(html string) (string, error)
	personName      string
	controlFD       int
	bootstrapStdin  bool
	// service is the JS service this host consults while it holds a document's
	// lock, when the bootstrap line attaches one.
	service *serviceLink
	// dashboard is the workspace origin a framed document load may be framed by.
	dashboard string
	// workspaceEntry is where a top-level load of a root's "/" goes, if anywhere.
	workspaceEntry func(rootID string) string
	// onPageLoad hears about a document that answered a top-level page load.
	onPageLoad func(rootID, rel string)
	// onStructure hears about a completed structure step that moved a name.
	onStructure func(rootID, from, to string, isDir bool)
	// internalDir is the app's own data folder (Options.InternalDir).
	internalDir string
	// events is the process's one frame stream: every root's commits and
	// watcher observations land in it, and the control listener reads it.
	events *eventRing
}

type rootSpec struct {
	id   string
	path string
	// publicHost is the served folder's own address, "<name>.htmlclay.localhost:<port>".
	// Empty in the trial binary, where each root has its own port.
	publicHost string
	// allowFraming is the folder's starting choice, from its config entry.
	allowFraming bool
}

type readyRoot struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

type readyLine struct {
	PID     int         `json:"pid"`
	Control string      `json:"control"`
	Roots   []readyRoot `json:"roots"`
	Epoch   string      `json:"epoch"`
}

// controlSecretPattern is the shape of the control secret: exactly 64 lowercase
// hex characters.
var controlSecretPattern = regexp.MustCompile("^[0-9a-f]{64}$")

// serviceEpochPattern is the shape of the JS service's epoch: 16 lowercase hex.
var serviceEpochPattern = regexp.MustCompile("^[0-9a-f]{16}$")

// validServicePeer is the shape every service peer must have: a loopback
// address, a 64-hex secret and a 16-hex epoch.
func validServicePeer(peer servicePeer) bool {
	return strings.HasPrefix(peer.URL, "http://127.0.0.1:") &&
		controlSecretPattern.MatchString(peer.Secret) &&
		serviceEpochPattern.MatchString(peer.Epoch)
}

// newRootHandler serves one root: Local's static catch-all, its document lane
// and its folder listings.
func newRootHandler(root rootSpec, cfg config) (*rootServer, error) {
	return newRootServer(root, cfg)
}
