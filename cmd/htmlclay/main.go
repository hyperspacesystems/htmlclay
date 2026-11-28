package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/carried"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/dashboard"
	"github.com/panphora/htmlclay/internal/egress"
	"github.com/panphora/htmlclay/internal/folderlock"
	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/mirror"
	"github.com/panphora/htmlclay/internal/panel"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recalc"
	"github.com/panphora/htmlclay/internal/secret"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/service"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/shellapi"
	"github.com/panphora/htmlclay/internal/synchistory"
	"github.com/panphora/htmlclay/internal/tray"
	"github.com/panphora/htmlclay/internal/trust"
	"github.com/panphora/htmlclay/internal/update"
	"github.com/panphora/htmlclay/internal/versions"
)

var version = "2.0.1"

//go:embed example.htmlclay
var exampleHTML []byte

// shellFailures is stdout when the Mac app started the host: fatal then prints a FAILED line it can read.
var shellFailures io.Writer

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[htmlclay] "+format+"\n", args...)
	if shellFailures != nil {
		_ = shellapi.WriteFailed(shellFailures, shellapi.Failure{Code: shellapi.FailFatal, Message: fmt.Sprintf(format, args...)})
	}
	os.Exit(1)
}

func resolveSymlinks(absPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// appRuntime holds everything shared across every origin: config, logging, the
// versions store, the single-instance lock, and the one live-sync runtime. Each
// origin becomes its own site (see sites.go) at its own hostname on the one port, so the
// browser treats unrelated trees as separate origins; the runtime is what they share.
type appRuntime struct {
	cfg      *config.Config
	logger   *logging.Logger
	versions *versions.Store
	si       platform.SingleInstance
	ls       *server.LiveSync
	recalc   *recalc.Runner
	// mirror answers the offline copy routes on every document origin; nil when its
	// store could not be opened, which leaves documents exactly as they were.
	mirror    *mirror.Handler
	mirrorDir string
	home      string
	configDir string
	// agentToken is what a local program sends to the agent lane, read or made at
	// start; agentTokenFile is where it lives. Never logged or put in a snapshot.
	agentToken     string
	agentTokenFile string
	// syncStore is the system's secret store holding the sync key; nil when it
	// could not be opened, and in tests that do not set one.
	syncStore secret.Store
	// syncActionMu makes connect and disconnect take turns. syncMu holds
	// syncRecord, sync.json as read once, only while it is read or changed, so a
	// snapshot never waits on a network call.
	syncActionMu sync.Mutex
	syncMu       sync.Mutex
	syncRecord   *config.Sync
	// syncHeld is whether a sync key is held, and syncConnecting counts connects
	// in progress: either keeps the service running with no folder served, since
	// sync runs inside it (serviceRunner.want).
	syncHeld       atomic.Bool
	syncConnecting atomic.Int32
	// syncKeyUnread is set when the last read of the sync key found none, so a
	// connected account whose key cannot be read asks for it again.
	syncKeyUnread atomic.Bool
	// syncConflicts is the engine's held conflicts as Go last read them
	// (sync_conflicts.go). Guarded by syncMu.
	syncConflicts syncConflicts
	// syncLast is the service's last sync report (C4), nil until one arrives.
	syncLast *served.SyncReport
	// syncHistory is each synced folder's stored history (sync_history.go),
	// made on first use under configDir.
	syncHistoryOnce sync.Once
	syncHistory     *synchistory.Store
	// notices is the attached Mac app streams that hear sync notices (8.40).
	notices noticeHub
	// syncWantedMu keeps configure sends one at a time (sendSyncWanted).
	syncWantedMu sync.Mutex
	// syncBinding is the sessions whose first download is running; configure leaves
	// them out until it ends. Guarded by syncMu.
	syncBinding map[string]bool
	// syncBindFailed is why a folder's last first download failed, by folder id,
	// until the next set-up of it starts. Guarded by syncMu.
	syncBindFailed map[string]string
	// syncBinds counts the first downloads still running, for a clean shutdown and tests.
	syncBinds sync.WaitGroup
	// moveRunning is true while a move from Hyperclay Local runs, and moveMoved is
	// the ids of Local's folders this process moved. moveMu guards both.
	moveMu      sync.Mutex
	moveRunning bool
	moveMoved   []string
	// folderLockDir is where the per-folder serve locks live. It is shared by every
	// profile on this account, so two HTML Clay processes never serve one folder.
	folderLockDir string
	// port is the one loopback port every site is served on, bound by startListener.
	port  int
	bound *boundPort
	// httpServer is the process's one server; serveHost routes each request by Host.
	httpServer *http.Server
	// portCandidates is the order a launch with no remembered port tries. main sets
	// defaultPorts; tests set {0} so every test app gets an ephemeral port.
	portCandidates []int
	// confirmPort overrides the port-taken dialog. Production leaves it nil.
	confirmPort func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error)
	// confirmQuit overrides the unsaved-edits quit dialog. Production leaves it nil.
	confirmQuit func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error)
	// syncQuiet overrides the wait for sync before an update restart. Production leaves it nil.
	syncQuiet func(limit time.Duration) bool
	// portOwner and quitProcess override the port-owner lookup and the quit request.
	portOwner   func(port int) (pid int, name string, ok bool)
	quitProcess func(pid int, force bool) error
	// randomPort overrides the last-resort random port pick. Production leaves it nil.
	randomPort func() int
	// afterLookup, when set, runs in the router between finding a live site and arming it.
	// Tests use it to change state inside that window.
	afterLookup func(*site)
	guard       func(dir string) bool
	// policy holds the rules about which folders may be trusted, and on whose
	// say-so. Pure functions; see package trust.
	policy trust.Policy
	// confirm, when set, overrides the native read-permission dialog. Production
	// leaves it nil (the server uses the real dialog); tests inject a decision.
	confirm func(title, message string, allowTrust bool) (platform.ConfirmChoice, error)
	// confirmDocument overrides the three-answer dialog behind the Unknown
	// document warning and the team code question, where closing it decides
	// nothing. Production leaves it nil.
	confirmDocument func(title, message string, labels platform.ConfirmLabels) (platform.ConfirmChoice, error)
	// confirmTrust overrides the two-button dialog behind a page's request to
	// trust its own folder. It is a separate seam from confirm on purpose: a test
	// that auto-approves read grants must not silently auto-approve the
	// write-granting dialog too.
	confirmTrust func(title, message, affirmative string) (bool, error)
	// dialogAttempt, when set, stands in for showing a real dialog: the dialog
	// seam refuses instead and reports which one was asked for, so a test fails
	// rather than opening a window on the machine. Production leaves it nil.
	dialogAttempt func(name string)
	// chooseHelperSetup overrides the dialog that connects a program to a
	// document. Production leaves it nil and uses platform.Choose.
	chooseHelperSetup func(title, message, refuse, middle, affirm string) (platform.Choice, error)
	// openProgramsURL overrides how the tray opens the Programs page's one-time
	// link, so a test can capture it instead of launching a browser.
	openProgramsURL func(string) error
	// launchBrowser overrides how openFile launches a document in the default
	// browser, so a test can capture the address instead of opening a window.
	// Production leaves it nil and uses the platform browser.
	launchBrowser func(string) error
	// reveal overrides how folder.reveal shows a folder in Finder, so no test
	// opens a Finder window. Production leaves it nil and uses browser.Reveal.
	reveal func(string) error
	// selectFolder overrides the native folder picker behind the Served Folders
	// submenu, so no test can open a real picker.
	selectFolder func(prompt string) (string, bool, error)
	// selectFile and promptSecret override the file picker and the hidden text
	// prompt a dashboard grant shows. Production leaves them nil.
	selectFile   func(prompt string) (string, bool, error)
	promptSecret func(title, message string) (string, bool, error)
	// copyText overrides the clipboard, so no test writes to the clipboard the
	// person is using.
	copyText func(text string) error
	// openKeptURL and revealKept override how the Kept Copies page opens its
	// one-time link and reveals a kept file, so a test can capture both instead
	// of launching a browser or a Finder window. Production leaves them nil.
	openKeptURL func(string) error
	revealKept  func(string) error
	// chooseHelperProgram and prepareHelperProgram override the file picker and
	// the executable check behind the helper permission dialog. Production
	// leaves them nil and uses the platform dialogs; tests inject a cancel and a
	// decline so no test can open a real picker or inspect a real program.
	chooseHelperProgram  func(prompt, dir string) (string, bool, error)
	prepareHelperProgram func(path string) (bool, error)
	// notify, when set, overrides the native notification. Production leaves it nil
	// (a real banner); tests capture the message instead of putting one on the
	// user's screen. Unlike confirm, this is read live from background goroutines
	// rather than snapshotted at buildSite, so set it before opening any site.
	notify func(title, message string) error
	// writesInFlight counts the requests that can change a file, so an update
	// restart can wait for the ones already sent. writesClosed, guarded by
	// writesMu, refuses new ones while it does. See countWrites.
	writesInFlight atomic.Int64
	writesMu       sync.Mutex
	writesClosed   bool
	// writesClosedForPort is a port change, not an update, closing writes.
	writesClosedForPort bool
	// quitForUpdate overrides how an applied update ends the process. Production
	// leaves it nil and quits the tray, which runs the ordinary shutdown.
	quitForUpdate func()
	// restartHost overrides how a port change ends this process. Production
	// leaves it nil and restarts for real; tests record the call.
	restartHost   func()
	startDetached func(path string, args ...string) error
	// held, while set, holds every request until startup has built the sites
	// and restored the update handoff, so a request can never register a file
	// under a fresh token before its old one comes back. main sets it before
	// binding; tests that do not set it serve at once.
	held atomic.Pointer[chan struct{}]
}

type app struct {
	rt       *appRuntime
	mu       sync.Mutex
	helperMu sync.Mutex
	sites    []*site
	parked   []*parked
	// bySlug is the router's view of the live sites, written wherever sites changes and
	// read on every request behind hostMu, so a request never waits on mu (route holds mu
	// across filesystem work, and one stalled mount must not block every other site).
	hostMu sync.RWMutex
	bySlug map[string]*site
	// servedByName is the router's view of the served folders: one mount per name,
	// read on every request behind servedMu so it never waits on a.mu or on the
	// served host. a.served owns the handlers; this map only routes to them.
	servedMu     sync.RWMutex
	servedByName map[string]*servedMount
	served       *served.Host
	// dashboard is the workspace at htmlclay.localhost, built with the served
	// host; nil when its bundle could not be read.
	dashboard *dashboard.Server
	// service runs the live-room service while a folder is served. It is nil, and
	// every call on it is a no-op, until startRuntime builds it.
	service *serviceRunner
	// tokens keeps save permission across a quit (savetokens.go). Nil until
	// startKeptTokens, and every use is nil-safe, so a test app without it keeps
	// the old contract.
	tokens atomic.Pointer[tokenStore]
	// unsaved holds what open tabs report about their unsaved edits.
	unsaved  unsavedReports
	stopping bool
	// shutdownOnce: the tray, host.quit and the shell's end of input can all shut down.
	shutdownOnce sync.Once
	noTray       bool
	// nativeGrants is true where the host confirms a dashboard page's grants
	// with its own dialogs: wherever it runs its own tray (grants.go).
	nativeGrants bool
	// grantOpen is true while one of those dialogs is open.
	grantOpen atomic.Bool
	// trayPanel is the Windows panel a left click on the tray icon opens; nil
	// where a left click shows the menu (panel.go).
	trayPanel *panel.Supervisor
	// shell: started by the Mac app (--shell-stdin)
	shell     bool
	autostart bool
	// shellServer is the private listener the Mac app talks to, and shellDone closes
	// when shutdown starts so every event stream ends with its bye line. Both are
	// zero on a launch without --shell-stdin; the two Once fields let a signal and
	// a closed stdin race to shut down without closing done twice.
	shellServer      *http.Server
	shellDone        chan struct{}
	shellStreamsOnce sync.Once
	shellServerOnce  sync.Once
	// helperStateMu guards the permission transactions themselves and the current
	// resolution through dispatcher attachment, so a read can see a whole one
	// without waiting behind helperMu, which a native dialog can hold for minutes.
	// Lock order is helperMu, then helperStateMu, then mu.
	// Never take helperMu while holding either of the others.
	helperStateMu sync.Mutex
	// programs holds the one-time links and sessions of the Programs page.
	programs programsAuth
	// kept holds the one-time links and sessions of the Kept Copies page.
	kept programsAuth
	// openDenials holds the names refused at a document's last direct open. A
	// refusal is never stored in config, so every re-resolve during that open
	// applies it from here; the next direct open clears it and asks again.
	// Guarded by helperStateMu.
	openDenials map[string][]string
	// pageAskQuiet holds, per document, when a page may next raise the helper
	// dialog after one that granted nothing. Guarded by helperStateMu.
	pageAskQuiet map[string]time.Time
	// helperSetupOpen holds the document and program a setup question is
	// waiting on, so a second ask about the same one does not stack another
	// question. Guarded by helperStateMu.
	helperSetupOpen map[string]bool
	// aiEditAsking holds the documents whose AI editing question is open, and
	// aiEditQuiet when each may next be asked after a question that allowed
	// nothing. Guarded by helperStateMu.
	aiEditAsking map[string]bool
	aiEditQuiet  map[string]time.Time
	// builtinAsking holds the document and program pairs whose question is open,
	// and builtinQuiet when each may next be asked after one that allowed
	// nothing. Guarded by helperStateMu.
	builtinAsking map[string]bool
	builtinQuiet  map[string]time.Time
	// documentAsking holds the documents whose warning or team code question
	// is open, and documentQuiet those whose question closed without an answer
	// HTML Clay keeps, until when. Guarded by helperStateMu.
	documentAsking map[string]bool
	documentQuiet  map[string]time.Time
	// toolsService is the ai-tools service, made on first use.
	toolsOnce    sync.Once
	toolsService *aitools.Service
	// programStore keeps the copies carried programs run from, made on
	// first use.
	programsOnce sync.Once
	programStore *carried.Store
	// dialogAdvice is non-empty when this machine cannot raise a permission
	// dialog at all. It is read once at startup and shown in two places, so it
	// is held rather than asked for again: platform.MissingDialogAdvice runs
	// exec.LookPath, and the tray builds its menu long after the check.
	dialogAdvice string
	// loaded records what config.Load had to do to the file on disk, so a
	// one-time upgrade (deleting App Mode's browser profile) can run after the
	// logger exists rather than during config parsing.
	loaded config.Result
	// updating is set while an update restart is under way, so a second click
	// on Restart to Update cannot start another.
	updating atomic.Bool
	// restartingForUpdate is set by startListener when this launch is the new
	// version of an update that is applying.
	restartingForUpdate bool
	// updateOff is why startSelfUpdater did not start the updater, and empty
	// when it did. update-now answers with it.
	updateOff string
	// core is the app core's own state: revision, epoch, seen waiting ids,
	// recent action answers, subscribers. See core.go.
	core coreState
	// lane is the agent lane's one handler, so its replay store outlives one
	// request. See agent.go.
	lane agentLaneHandler
	// aiTools is the AI editing tool picker's list, reused for aiToolsFresh so
	// building the snapshot does not read PATH every time.
	aiTools aiToolsCache
	// aiToolsEnv is the environment the picker finds Claude, Codex and agy in;
	// nil means the login shell's, which is what a person's HTML Clay uses.
	aiToolsEnv []string
	// recovered is the count of recovered changes the menus last saw, and
	// stopRecovered ends the loop that keeps it fresh.
	recovered     recoveredCount
	stopRecovered chan struct{}
	// updates is what the updater on Windows and Linux has just reported; the
	// Mac app reads Sparkle instead.
	updates updateState
	// former holds the ports the app served on before the person moved it, so a
	// bookmark at an old address still finds the app. Guarded by mu.
	former []formerPort
	// fileTypes is what the system last said about file types, on Windows and
	// Linux, where the host owns them.
	fileTypes fileTypeCache
	// stopFileTypes ends the loop that asks the system again every minute, a
	// person having changed the default in the system's own settings.
	stopFileTypes chan struct{}
	// stopWarming ends the offline copies folder warmer (startWarming).
	stopWarming context.CancelFunc
	// warmer is the running folder warmer, nil until startWarming.
	warmer *mirror.Warmer
}

func migrateConfigDir() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	oldDir := filepath.Join(home, ".htmlclay")

	newDir, err := config.Dir()
	if err != nil {
		return
	}

	if oldDir == newDir {
		return
	}

	oldInfo, err := os.Stat(oldDir)
	if err != nil || !oldInfo.IsDir() {
		return
	}

	if _, err := os.Stat(newDir); err == nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(newDir), 0755); err != nil {
		return
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] Could not migrate config from %s to %s: %v\n", oldDir, newDir, err)
	} else {
		fmt.Fprintf(os.Stderr, "[htmlclay] Migrated config from %s to %s\n", oldDir, newDir)
	}
}

// openLogOverride returns the replacement for launchBrowser that writes each URL
// the host would open instead of opening it, or nil when it does not apply.
//
// HTMLCLAY_OPEN_LOG is for the Mac app's tests: a host it started writes each URL
// it would open instead of opening the browser. It only applies to a host started
// with --shell-stdin, so an ordinary launch can never be told to stop opening the
// browser, and only to an absolute path, so the host never writes somewhere it
// guessed.
func openLogOverride(shell bool, env string) func(string) error {
	logPath, ok := openLogPath(shell, env)
	if !ok {
		return nil
	}
	return openLogLine(logPath, "")
}

// openLogPath answers where a host the app started writes what it did instead of
// doing it, and whether it does that at all.
func openLogPath(shell bool, env string) (string, bool) {
	if !shell || !filepath.IsAbs(env) {
		return "", false
	}
	return env, true
}

// openLogLine returns the replacement for a runtime seam that writes the line the
// host would have acted on, with prefix, instead of acting on it.
func openLogLine(logPath, prefix string) func(string) error {
	return func(target string) error {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintln(f, prefix+target)
		return err
	}
}

func main() {
	// A program worker is this binary started by HTML Clay to run one carried
	// program. It returns before anything else: it takes no lock, reads no
	// settings and opens nothing.
	if len(os.Args) > 1 && os.Args[1] == carried.WorkerArg {
		os.Exit(carried.RunWorker(os.Stdin, os.Stdout))
	}
	// The wire CLI is a client of a running app, so it must return before
	// anything below: initConfig takes the single-instance lock, and flag.Parse
	// stops at the first non-flag argument, which would leave a subcommand's own
	// flags unparsed and forward "wire" to openFile as a path.
	if code, handled := wireDispatch(os.Args); handled {
		os.Exit(code)
	}
	// update-now is a client of the running app too, for the same reasons.
	if code, handled := updateNowDispatch(os.Args); handled {
		os.Exit(code)
	}
	// serve and stop-serving are clients of the running app too, for the same reasons.
	if code, handled := serveDispatch(os.Args); handled {
		os.Exit(code)
	}
	if code, handled := trustDispatch(os.Args); handled {
		os.Exit(code)
	}
	if code, handled := syncDispatch(os.Args); handled {
		os.Exit(code)
	}

	releaseOwnConsole()

	noTray := flag.Bool("no-tray", false, "Run without system tray (signal-based shutdown)")
	shellStdin := flag.Bool("shell-stdin", false, "Started by the HTML Clay Mac app: read a secret from stdin and serve the app's private listener")
	unregister := flag.Bool("unregister", false, "Remove the file associations and the Start on Login entry, then exit")
	finishUpdate := flag.Bool("finish-update", false, "Internal: finish an update this binary's previous version started")
	afterPID := flag.Int("after-pid", 0, "Internal: wait for this process to exit first (a port change restart)")
	openDashboard := flag.String("open-dashboard", "", "Internal: open this dashboard page once running (after a port change)")
	autostart := flag.Bool("autostart", false, "Internal: started automatically at login (stays quiet in tray)")
	panelChild := flag.Bool("panel", false, "Internal: run the Windows panel's window for the running app")
	flag.Parse()

	// The panel's child is started by the running app, so it must never reach
	// the single-instance lock, which would forward its flag to that app.
	if *panelChild {
		os.Exit(panel.RunChild())
	}

	// Before initConfig, and so before the single-instance lock, on purpose. A
	// launch that finds the lock held forwards its arguments to the running app
	// and exits, so an --unregister placed after it would quietly do nothing
	// whenever HTML Clay happened to be running, which is the state a person is
	// most likely to be in while uninstalling.
	if *unregister {
		os.Exit(runUnregister())
	}
	// Also before the lock: the supervisor runs while the process it replaces
	// still holds it, and it never becomes the app itself.
	if *finishUpdate {
		os.Exit(runFinishUpdate())
	}

	// The Go host inside the Swift app's bundle never becomes the app itself.
	if code, handled := handOffToMacApp(*shellStdin, *noTray, flag.Args()); handled {
		os.Exit(code)
	}

	// The previous copy can take about 25 s to shut down (sync's flush, the
	// service, the listeners), and this copy cannot start until it has.
	if *afterPID > 0 && !update.WaitGone(*afterPID, 40*time.Second) {
		fmt.Fprintf(os.Stderr, "[htmlclay] The previous HTML Clay (pid %d) did not exit\n", *afterPID)
	}

	// The Mac app writes the secret on stdin and expects the private listener to
	// come up, so a bad line ends the launch before anything binds.
	var shellSecret string
	var shellIn io.Reader
	if *shellStdin {
		bootstrap, in, err := shellapi.ReadBootstrap(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[htmlclay] %v\n", err)
			_ = shellapi.WriteFailed(os.Stdout, shellapi.Failure{Code: shellapi.FailBadBootstrap})
			os.Exit(2)
		}
		shellSecret, shellIn = bootstrap.Secret, in
		shellFailures = os.Stdout
		exitWithParent()
	}

	migrateConfigDir()

	fmt.Fprintln(os.Stderr, "[htmlclay] Starting up...")

	a := &app{rt: &appRuntime{}, noTray: *noTray || *shellStdin, shell: *shellStdin, autostart: *autostart}
	a.nativeGrants = !a.noTray
	if logPath, ok := openLogPath(*shellStdin, os.Getenv("HTMLCLAY_OPEN_LOG")); ok {
		a.rt.launchBrowser = openLogLine(logPath, "")
		a.rt.reveal = openLogLine(logPath, "reveal ")
		a.rt.copyText = openLogLine(logPath, "copy ")
	}
	a.initCore()
	a.initConfig()
	defer a.rt.si.Unlock()
	// Holding the lock makes this the process an update supervisor should judge.
	markUpdateHealth(update.HealthStarted)

	a.initLogger()
	defer a.rt.logger.Close()
	if a.installWaitingUpdate(flag.Args()) {
		a.rt.logger.Close()
		a.rt.si.Unlock()
		os.Exit(0)
	}
	a.holdSavesWhileUnjudged()

	a.startRuntime()
	a.finishUpgrade()
	a.startWarming()

	// The Mac app owns the login item and file types; a host it started, or any
	// host inside its bundle, changes neither.
	if a.ownsLoginItem() {
		a.refreshLoginItem()
		a.refreshFileTypes()
		a.warnMissingDialogs()
	}
	if a.managesFileTypes() {
		go a.refreshFileTypeState()
		if a.stopFileTypes == nil {
			a.stopFileTypes = make(chan struct{})
		}
		go a.watchFileTypes(a.stopFileTypes)
	}

	// Bind the one port before argv is processed, asking the user what to do if it is held,
	// then build every live trusted folder's site, so a URL bookmarked before the last quit
	// answers on the first launch after it.
	a.holdRequests()
	if !a.startListener() {
		if a.restartingForUpdate {
			a.rt.logger.Close()
			a.rt.si.Unlock()
			os.Exit(1)
		}
		if a.shell {
			_ = shellapi.WriteFailed(os.Stdout, shellapi.Failure{Code: shellapi.FailPortTaken, Port: a.rt.cfg.Port()})
			a.rt.logger.Close()
			a.rt.si.Unlock()
			os.Exit(4)
		}
		return
	}
	a.startSites()
	a.startServed()
	a.seedDefaultPlayground()
	a.startFormerPorts()
	if *shellStdin {
		a.startShell(shellSecret, shellIn)
	}
	// An update restart left the previous process's sessions behind; take them
	// back before anything else can register, so every tab that was open keeps
	// saving at the address it already has. An ordinary launch finds no file.
	// Requests already accepted wait in holdRequests until this is done.
	if _, err := a.restoreUpdateHandoff(filepath.Join(a.rt.configDir, updateHandoffFile)); err != nil {
		a.rt.logger.Printf("Update handoff ignored: %v", err)
	}
	// Then the tokens kept at the last quit, so a tab left open across an ordinary
	// restart keeps saving too. Each site takes its own back on its first request.
	a.startKeptTokens(filepath.Join(a.rt.configDir, saveTokensFile))
	a.releaseRequests()
	if *openDashboard != "" {
		go a.trayDo("dashboard.open", map[string]any{"route": *openDashboard})
	} else if !a.shell && !a.noTray && a.loaded.FreshInstall && len(flag.Args()) == 0 && !*autostart {
		go a.trayDo("dashboard.open", map[string]any{"route": "/"})
	}
	a.stopRecovered = make(chan struct{})
	go a.watchRecovered(a.stopRecovered)
	if !a.shell {
		markUpdateHealth(update.HealthServing)
		go a.settleUpgradeSave()
	}

	args := flag.Args()
	if len(args) > 0 {
		a.rt.logger.Printf("Opening file: %s", args[0])
		a.openFile(args[0])
	}
	for _, path := range a.takeOpenAfterUpdate() {
		a.rt.logger.Printf("Opening file from before the update: %s", path)
		a.openFile(path)
	}

	a.rt.si.OnFileReceived(func(path string) {
		a.rt.logger.Printf("Received file from another instance: %s", path)
		a.openFile(path)
	})

	// macOS delivers Finder double-clicks as Apple Events, not argv; this hooks
	// them into the same open path. No-op on other platforms.
	platform.OnOpenFile(func(path string) {
		a.rt.logger.Printf("Received open-file event: %s", path)
		a.openFile(path)
	})

	if !a.startSelfUpdater() {
		go a.checkForUpdateLink()
	}
	a.rt.si.OnCommand(a.handleInstanceCommand)

	a.run()
}

func (a *app) initConfig() {
	if err := config.EnsureDir(); err != nil {
		fatal("Error creating config dir: %v", err)
	}
	configDir, err := config.Dir()
	if err != nil {
		fatal("Error resolving config dir: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[htmlclay] Config dir: %s\n", configDir)

	token, tokenFile, err := loadAgentToken(configDir)
	if err != nil {
		fatal("Error writing the agent token: %v", err)
	}
	a.rt.agentToken, a.rt.agentTokenFile = token, tokenFile

	a.rt.si = platform.NewSingleInstance(configDir)
	isPrimary, err := a.rt.si.TryLock()
	if err != nil {
		fatal("Error checking single instance: %v", err)
	}

	if !isPrimary {
		if a.shell {
			fmt.Fprintln(os.Stderr, "[htmlclay] Another instance is running")
			_ = shellapi.WriteFailed(os.Stdout, shellapi.Failure{Code: shellapi.FailAlreadyRunning})
			os.Exit(3)
		}
		args := flag.Args()
		if len(args) > 0 {
			fmt.Fprintln(os.Stderr, "[htmlclay] Another instance running, forwarding file...")
			filePath, err := filepath.Abs(args[0])
			if err != nil {
				fatal("Error resolving path: %v", err)
			}
			if err := a.rt.si.SendFilePath(filePath); err != nil {
				fatal("Error sending file to running instance: %v", err)
			}
		} else if !a.autostart {
			fmt.Fprintln(os.Stderr, "[htmlclay] Another instance running, opening dashboard...")
			reply, err := a.rt.si.SendCommand("open-dashboard")
			if err != nil {
				fmt.Fprintf(os.Stderr, "[htmlclay] Could not ask running instance to open dashboard: %v\n", err)
			}
			if errors.Is(err, platform.ErrNoAnswer) || strings.HasPrefix(reply, "refused") {
				_ = platform.Notify(
					"HTML Clay is already running",
					"Quit the older HTML Clay in the system tray, then open this one again.",
				)
			}
		}
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "[htmlclay] Primary instance, proceeding...")

	cfg, res, err := config.Load(platform.DirIdentity)
	if err != nil {
		fatal("Error loading config: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[htmlclay] Config loaded: %d trusted folder(s)\n", len(cfg.TrustedFolderList()))
	a.rt.cfg = cfg
	a.loaded = res
	// Set here, not in startRuntime: holdSavesWhileUnjudged runs before
	// startRuntime and reads the update state under this folder.
	a.rt.configDir = configDir
	if resolved, rErr := resolveSymlinks(configDir); rErr == nil {
		a.rt.configDir = resolved
	}
}

// appLogPath is the file initLogger writes and app.revealLog shows.
func (a *app) appLogPath() string {
	return filepath.Join(a.rt.configDir, "htmlclay.log")
}

func (a *app) initLogger() {
	logPath := a.appLogPath()
	logger, err := logging.NewDualWrite(logPath)
	if err != nil {
		fatal("Error creating logger: %v", err)
	}
	a.rt.logger = logger
	a.rt.logger.Printf("Logger initialized at %s", logPath)
}

// startRuntime builds the process-wide state shared by every site. It binds no
// port; startSites and route bind their own.
func (a *app) startRuntime() {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("Error resolving home directory: %v", err)
	}
	// Normalize once so containment checks and user-facing messages agree with
	// the session manager, which resolves symlinks in home internally.
	if resolved, rErr := resolveSymlinks(home); rErr == nil {
		home = resolved
	}
	a.rt.home = home

	configDir, err := config.Dir()
	if err != nil {
		fatal("Error resolving config dir: %v", err)
	}

	// An unresolved lock directory is left empty on purpose: serving then refuses
	// with that error instead of serving unlocked.
	lockDir, lockErr := folderlock.DefaultDir()
	if lockErr != nil {
		a.rt.logger.Printf("Could not resolve the folder lock directory: %v", lockErr)
	}
	a.rt.folderLockDir = lockDir

	a.rt.versions = versions.New(filepath.Join(configDir, "versions"))
	// Prune once at startup; every later pass is opportunistic after a backup.
	go a.rt.versions.PruneAll()

	a.rt.ls = server.NewLiveSync(a.rt.versions, a.rt.logger)
	a.rt.recalc = recalc.New(recalc.WithLogger(a.rt.logger.Printf))
	a.rt.mirror = a.openMirror()

	// A grant must not cover htmlclay's own config tree in either direction: not
	// a grant inside it, and not a grant of an ancestor that swallows it. The
	// versions dir lives under the config dir, so one check covers both. The
	// serve path denies the same tree structurally (Server.SetInternalDir); this
	// guard just stops the grant from being offered in the first place.
	forbidden := a.rt.configDir
	// The uploads library is refused as itself and as a folder inside it, for the
	// same reason: a grant that covers it would turn a page dropped in it into an
	// editable page. Its ancestors stay grantable on purpose, because ~/htmlclay
	// and home are the folders a person keeps documents in.
	// Resolved, so a ~/htmlclay symlink cannot hand the guard a spelling the
	// trust flow never uses: Canonical resolves the folder it is asked to trust,
	// and the guard has to compare against the folder that resolution lands on.
	library, lErr := server.ResolvedUploadsLibraryDir()
	if lErr != nil {
		library, _ = server.UploadsLibraryDir()
	}
	// The offline copies folder is refused as itself and inside it, like the uploads
	// library: its index lists what every folder's pages load.
	mirrorDir := a.rt.mirrorDir
	a.rt.guard = func(dir string) bool {
		return grantForbidden(dir, forbidden, library) || (mirrorDir != "" && session.EqualOrUnder(dir, mirrorDir))
	}
	a.rt.policy = trust.Policy{Home: a.rt.home, Guard: a.rt.guard}

	a.rt.portCandidates = defaultPorts

	a.rt.syncStore = a.openSyncStore()
	a.loadSyncAtStart()
	a.service = newServiceRunner(a)

	a.rt.logger.Printf("Runtime ready (home=%s)", a.rt.home)
}

// offlineCopiesDir is the folder offline copies live in: the OS cache folder, or
// under HTMLCLAY_CONFIG_BASE when a test or seeded profile set it, so those never
// share or evict the person's real copies.
func offlineCopiesDir() (string, error) {
	if base := os.Getenv("HTMLCLAY_CONFIG_BASE"); base != "" {
		return filepath.Join(base, "htmlclay-cache", "offline-copies"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "htmlclay", "offline-copies"), nil
}

// openMirror opens the offline copy store in the user's cache folder: copies the next
// online open can make again, so they never go in the config folder, a document or a
// synced folder.
func (a *app) openMirror() *mirror.Handler {
	dir, err := offlineCopiesDir()
	if err != nil {
		a.rt.logger.Printf("Offline copies are unavailable: %v", err)
		return nil
	}
	policy := offlinePolicy(a.rt.cfg.OfflineSettings())
	store, err := mirror.Open(dir, policy.Limits)
	if err != nil {
		a.rt.logger.Printf("Offline copies are unavailable: %v", err)
		return nil
	}
	store.SetPolicy(policy)
	// Resolved, so the grant guard and the serve path compare against the spelling
	// trust resolution lands on.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	a.rt.mirrorDir = dir
	off := func() bool { return a.rt.cfg != nil && !a.rt.cfg.OfflineCopies() }
	fetch := mirror.NewFetcher(store, testUpstreamDial(os.Getenv("HTMLCLAY_OFFLINE_TEST_UPSTREAM")))
	fetch.Off = off
	return &mirror.Handler{Store: store, Fetch: fetch, Off: off}
}

// offlinePolicy maps the offline settings to what the mirror keeps.
func offlinePolicy(s config.OfflineSettings) mirror.Policy {
	return mirror.PolicyFor(s.Mode, s.Space, s.Limited, s.Images, s.Video, s.Audio, s.LimitMB)
}

// offlinePolicyOf is the same settings as Settings shows them.
func offlinePolicyOf(s config.OfflineSettings) appcore.OfflinePolicy {
	return appcore.OfflinePolicy{
		Mode:    s.Mode,
		Space:   s.Space,
		Limited: s.Limited,
		Images:  s.Images,
		Video:   s.Video,
		Audio:   s.Audio,
		LimitMB: s.LimitMB,
	}
}

// testUpstreamDial lets the offline copies end-to-end test reach its stand-in CDN on this
// computer: list is comma-separated host:port addresses that skip the public internet
// check. Empty, as in every real launch, is egress.DialPublic alone.
func testUpstreamDial(list string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if list == "" {
		return nil
	}
	allowed := map[string]bool{}
	for _, addr := range strings.Split(list, ",") {
		allowed[strings.TrimSpace(addr)] = true
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if allowed[addr] {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}
		return egress.DialPublic(ctx, network, addr)
	}
}

// offlineFacts is what the offline copies take now, for Settings.
func (a *app) offlineFacts() appcore.OfflineState {
	if a.rt.mirror == nil {
		return appcore.OfflineState{}
	}
	bytes, files := a.rt.mirror.Store.Usage()
	return appcore.OfflineState{Available: true, Bytes: bytes, Files: files}
}

// clearOfflineCopies deletes every offline copy.
func (a *app) clearOfflineCopies() *appcore.Refusal {
	if a.rt.mirror == nil {
		return nil
	}
	if a.warmer != nil {
		a.warmer.Reset()
	}
	if err := a.rt.mirror.Store.Clear(); err != nil {
		a.rt.logger.Printf("Could not clear offline copies: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// startWarming copies the static files that HTML files in trusted and served folders
// load, before they are opened, while offline copies are on.
func (a *app) startWarming() {
	if a.rt.mirror == nil || a.rt.mirror.Fetch == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.stopWarming = cancel
	w := &mirror.Warmer{Fetch: a.rt.mirror.Fetch, Folders: a.warmFolders, Off: a.rt.mirror.Off}
	a.warmer = w
	go w.Run(ctx)
}

// warmFolders lists the folders being served, then the trusted folders, each once.
func (a *app) warmFolders() []string {
	var out []string
	seen := map[string]bool{}
	// The same checks the sites and served folders pass before serving: a folder that
	// went missing, or was replaced by another at the same path, is never walked.
	add := func(path, identity string) {
		if seen[path] {
			return
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() || !trust.IdentityOK(path, identity, a.rt.home) {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	for _, sf := range a.rt.cfg.ServedFolderList() {
		if sf.Serving {
			add(sf.Path, sf.Identity)
		}
	}
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		add(tf.Path, tf.Identity)
	}
	return out
}

// offlineBoot adds the offline copies boot script to a document load, unless offline
// copies are switched off or unavailable. The switch is read on every load.
func (a *app) offlineBoot(html []byte) []byte {
	if a.rt.mirror == nil || (a.rt.mirror.Off != nil && a.rt.mirror.Off()) {
		return html
	}
	return mirror.Inject(html)
}

// grantForbidden reports whether a read grant on dir must be refused. The config
// tree is refused in either direction, since a grant of an ancestor that swallows
// it is as good as a grant inside it. The uploads library is refused only as
// itself or as something inside it: ~/htmlclay, home and every other ancestor of
// the library stay grantable, because refusing them would refuse the folders
// people keep their documents in.
func grantForbidden(dir, configDir, library string) bool {
	if session.EqualOrUnder(dir, configDir) || session.EqualOrUnder(configDir, dir) {
		return true
	}
	return library != "" && session.EqualOrUnder(dir, library)
}

// repinTrustedFolders moves every pin that still matches onto the fingerprint the
// folder has now. That is a no-op for a current pin, and for one in the old
// dev:inode form it is the upgrade: macOS renumbers st_dev at boot, so those went
// stale at the first reboot, and MatchDirIdentity only vouches for them on home's
// volume. Once re-pinned, the entry no longer depends on that allowance.
func (a *app) repinTrustedFolders() bool {
	changed := false
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Identity == "" {
			continue
		}
		current, ok := platform.MatchDirIdentity(tf.Path, tf.Identity, a.rt.home)
		if !ok || current == tf.Identity {
			continue
		}
		a.rt.cfg.SetTrustedIdentity(tf.Path, current)
		a.rt.logger.Printf("Re-pinned trusted folder %s to its current fingerprint", tf.Path)
		changed = true
	}
	return changed
}

// finishUpgrade runs the one-time work a config from an older version implies.
// It lives here rather than in config because it touches the filesystem and
// talks to the user, and because this is already where one-shot upgrade work
// (migrateConfigDir) lives.
func (a *app) finishUpgrade() {
	// Both migrations rewrote the config in memory only. Persisting it here is
	// what makes them one-time. Without this they are re-derived on every Load,
	// and nothing else on a settled launch saves: trustFolder, untrustFolder, the
	// Start-on-Login toggle and rememberPort are the only writers, and a launch
	// that trusts nothing new and keeps its port touches none of them. A pin that
	// never lands is a pin re-taken from whatever directory sits at that path at
	// the time, which is exactly the swap it exists to catch.
	if a.loaded.DroppedHelperPrograms != 0 || a.loaded.DroppedHelperDecisions != 0 {
		a.rt.logger.Printf("Helper config normalization dropped records: programs=%d decisions=%d", a.loaded.DroppedHelperPrograms, a.loaded.DroppedHelperDecisions)
	}
	repinned := a.repinTrustedFolders()
	if a.loaded.PromotedLegacy || a.loaded.PinnedIdentities || a.loaded.AssignedFolderIDs || a.loaded.MovedFirstSite || a.loaded.MarkedFirstSiteOrigin || repinned {
		if err := a.rt.cfg.Save(); err != nil {
			a.rt.logger.Printf("Could not persist the upgraded config: %v", err)
		}
	}
	if a.loaded.PromotedLegacy {
		a.rt.logger.Printf("Promoted legacy read-only trusted folders to trusted folders")
	}
	if a.loaded.PinnedIdentities {
		a.rt.logger.Printf("Pinned trusted folders that were recorded without an identity fingerprint")
	}
	if a.loaded.AssignedFolderIDs {
		a.rt.logger.Printf("Gave trusted folders the id each one keeps")
	}
	if a.loaded.MovedFirstSite {
		key := a.rt.cfg.MovedFirstSiteKey()
		a.rt.logger.Printf("Moved %s from htmlclay.localhost to %s.localhost: the old address is the dashboard's now", key, a.rt.cfg.SiteSlug(key))
	}
	if a.loaded.MarkedFirstSiteOrigin {
		a.rt.logger.Printf("The dashboard clears what a site stored at htmlclay.localhost, once in each browser")
	}
	if !a.loaded.HadAppMode {
		return
	}
	// App Mode ran pages in a private Chromium profile. That profile is now
	// unreachable, so anything a page stored in it (localStorage, IndexedDB) is
	// already gone from the user's point of view; leaving the directory on disk
	// only hides that. Delete it and say so once.
	profile := filepath.Join(a.rt.configDir, "chrome-profile")
	if _, err := os.Stat(profile); err != nil {
		return
	}
	if err := os.RemoveAll(profile); err != nil {
		a.rt.logger.Printf("Could not remove the old App Mode profile at %s: %v", profile, err)
		return
	}
	a.rt.logger.Printf("Removed the old App Mode browser profile at %s", profile)
	go func() {
		msg := "HTML Clay now always opens files in your normal browser, so App Mode is gone.\n\n" +
			"Its separate browser profile has been removed. If a page saved anything in that " +
			"profile's own storage, it is no longer available."
		if err := a.notifyUser("HTML Clay", msg); err != nil {
			a.rt.logger.Printf("Could not notify about the removed App Mode profile: %v", err)
		}
	}()
}

func (a *app) run() {
	if a.noTray {
		a.rt.logger.Printf("Running without tray, waiting for signal...")
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		a.shutdown()
		return
	}
	a.rt.logger.Printf("Starting system tray...")
	hooks := &tray.MenuHooks{Rows: a.trayMenuRows, Do: a.doTrayMenu}
	if a.trayPanel = a.newTrayPanel(); a.trayPanel != nil {
		hooks.Tapped = a.panelTapped
	}
	tray.Run(hooks, a.confirmQuit, func() {
		a.shutdown()
	}, a.dialogAdvice)
	a.rt.logger.Printf("Tray exited")
}

// runUnregister undoes what a launch registers: the file associations and the
// Start on Login entry. It touches nothing else. config.json stays, because the
// trusted-folder list is the record of what the user granted rather than part of
// the installation, and so do the saved versions under the config directory.
func runUnregister() int {
	code := 0
	if err := platform.UnregisterFileTypes(); err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] Could not remove the file associations: %v\n", err)
		code = 1
	}
	if err := platform.SetLoginItem(false, ""); err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] Could not remove the Start on Login entry: %v\n", err)
		code = 1
	}
	if code == 0 {
		// "set up itself" is the honest wording on all three platforms: on Windows
		// that is four registry keys, and on macOS and Linux it is none, because
		// the app bundle and the freedesktop MIME database own those.
		fmt.Fprintln(os.Stderr, "[htmlclay] Removed the Start on Login entry and every file association HTML Clay set up itself.")
	}
	fmt.Fprintln(os.Stderr, "[htmlclay] Your files, your trusted folders and your saved versions are untouched.")
	fmt.Fprintln(os.Stderr, "[htmlclay] Starting HTML Clay again puts them back, so delete the program now if you are done with it.")
	return code
}

// runFinishUpdate is the supervisor behind --finish-update. It derives the
// config dir the way the app does and takes nothing from argv, because the old
// version spawns it and the new version's state is all it needs.
func runFinishUpdate() int {
	configDir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] Error resolving config dir: %v\n", err)
		return 1
	}
	if err := update.FinishUpdate(update.UpdatesDir(configDir), update.SupervisorOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] Update not finished: %v\n", err)
		return 1
	}
	return 0
}

// markUpdateHealth reports startup progress to an update supervisor. It writes
// nothing unless an update to this exact version is applying, so an ordinary
// launch only pays for reading a file that is not there.
func markUpdateHealth(phase string) {
	configDir, err := config.Dir()
	if err != nil {
		return
	}
	if err := update.MarkHealth(update.UpdatesDir(configDir), version, phase); err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] Could not report update health (%s): %v\n", phase, err)
	}
}

// holdSavesWhileUnjudged: a host started by an update that is still being judged
// keeps its config changes in memory until the verdict (settleUpgradeSave).
func (a *app) holdSavesWhileUnjudged() {
	if update.Unjudged(update.UpdatesDir(a.rt.configDir)) {
		a.rt.cfg.HoldSaves()
		a.rt.logger.Printf("An update is being judged; config changes wait for its verdict")
	}
}

// settleUpgradeSave waits up to 10 s for the verdict. On done the held changes are
// saved and the update handoff is removed; on a rollback, or no verdict, the
// changes stay in memory only and the handoff stays for the previous version.
func (a *app) settleUpgradeSave() {
	dir := update.UpdatesDir(a.rt.configDir)
	for i := 0; i < 40 && update.Unjudged(dir); i++ {
		time.Sleep(250 * time.Millisecond)
	}
	st, _ := update.ReadState(dir)
	write := st == nil || (st.Phase != update.PhaseRolledBack && st.Phase != update.PhaseApplying)
	if err := a.rt.cfg.ReleaseSaves(write); err != nil {
		a.rt.logger.Printf("config save after the update verdict failed: %v", err)
	}
	if write {
		a.removeUpdateHandoff(filepath.Join(a.rt.configDir, updateHandoffFile))
		// This version stays, so the previous version's service copy can go.
		service.PruneOthers(serviceRuntimeRoot(a.rt.configDir))
	}
}

// warnMissingDialogs turns a silent failure into an instruction. On a desktop
// with neither zenity nor kdialog every permission prompt fails closed, which is
// correct, but the result the user sees is a file that will not open and nothing
// on screen saying why.
//
// Three channels, because each one can be absent on exactly the machine that
// needs it: the log always works, the tray row is permanent and needs no
// notification daemon, and the banner is the only one that appears without the
// user going looking, but it runs through notify-send, which the same minimal
// desktop may not have either.
func (a *app) warnMissingDialogs() {
	a.dialogAdvice = platform.MissingDialogAdvice()
	if a.dialogAdvice == "" {
		return
	}
	a.rt.logger.Printf("%s", a.dialogAdvice)
	msg := a.dialogAdvice + "\n\nUntil then, a file opened from outside a trusted folder is refused, " +
		"because the prompt that would allow it cannot be shown. Trusted folders themselves still work: " +
		"add one from the HTML Clay menu and its files open with no prompt at all."
	go func() {
		if err := a.notifyUser("HTML Clay", msg); err != nil {
			a.rt.logger.Printf("Could not show the missing-dialog notice: %v", err)
		}
	}()
}

// refreshFileTypes re-registers the file associations on every launch, for the
// reason refreshLoginItem re-registers the login item: a binary the user moved
// leaves a stored path pointing at nothing. Failure is logged and the launch
// carries on, since the launch running this is usually a double-click on a file
// and that file still has to open.
func (a *app) refreshFileTypes() {
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		return
	}
	opts := platform.FileTypeOptions{
		HTMLClay:     !a.rt.cfg.StoppedOpeningFiles(),
		HTMLOpenWith: a.rt.cfg.HTMLOpenWith(),
	}
	if err := platform.RegisterFileTypes(execPath, opts); err != nil {
		a.rt.logger.Printf("Could not register the .htmlclay file association: %v", err)
	}
}

func (a *app) refreshLoginItem() {
	if !a.rt.cfg.StartOnLoginEnabled() {
		return
	}
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		return
	}
	// Re-register on every launch so a moved or updated binary keeps a valid path.
	if err := platform.SetLoginItem(true, execPath); err != nil {
		a.rt.logger.Printf("Could not refresh login item: %v", err)
	}
}

// ownsLoginItem: only a Go-main HTML Clay keeps the login item and file types.
func (a *app) ownsLoginItem() bool {
	_, mac := bundledMacHost()
	return !a.shell && !mac
}

// notifyUser sends a best-effort native message through the runtime seam, so a
// test asserting that the user was told never puts a real banner on their screen.
// Mirrors the confirm seam, for the same reason.
func (a *app) notifyUser(title, message string) error {
	if a.rt.notify != nil {
		return a.rt.notify(title, message)
	}
	return platform.Notify(title, message)
}

// informUser is notifyUser for good news: the same seam, a non-urgent notice.
func (a *app) informUser(title, message string) error {
	if a.rt.notify != nil {
		return a.rt.notify(title, message)
	}
	return platform.Inform(title, message)
}
