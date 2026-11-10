//go:build managedtrial

package managedtrial

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	shutdownTimeout  = 5 * time.Second
	readHeaderLimit  = 10 * time.Second
	controlSecretLen = 64
)

// liveRoot names the folder no root may touch. Tests replace it.
var liveRoot = DefaultLiveRoot

type config struct {
	roots       []string
	port        int
	localSource string
	personName  string
	controlFD   int
	// events is the process's one frame stream: every root's commits and
	// watcher observations land in it, and the control listener reads it.
	events *eventRing
}

type rootSpec struct {
	id   string
	path string
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
}

type boundServer struct {
	srv *http.Server
	ln  net.Listener
}

// Run boots the trial host: it serves every root on its own loopback listener,
// runs the private control listener, prints one READY line on stdout and keeps
// serving until ctx is cancelled or stdin reaches EOF.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		return err
	}
	live, err := liveRoot()
	if err != nil {
		return err
	}
	roots, err := resolveRoots(cfg.roots, live)
	if err != nil {
		return err
	}
	if err := checkLocalSource(cfg.localSource); err != nil {
		return err
	}
	secret := ""
	if cfg.controlFD >= 0 {
		secret, err = readControlSecret(cfg.controlFD)
		if err != nil {
			return err
		}
	}

	var bound []boundServer
	closeBound := func() {
		for _, b := range bound {
			_ = b.srv.Close()
		}
	}

	cfg.events = newEventRing()
	var sites []*controlRoot
	ready := readyLine{PID: os.Getpid()}
	for i, root := range roots {
		ln, err := net.Listen("tcp", listenAddress(cfg.port, i))
		if err != nil {
			closeBound()
			return fmt.Errorf("--root %q: %w", root.path, err)
		}
		handler, err := newRootHandler(root, cfg)
		if err != nil {
			closeBound()
			return err
		}
		sites = append(sites, handler.control)
		bound = append(bound, boundServer{
			srv: &http.Server{
				Handler:           handler,
				ReadHeaderTimeout: readHeaderLimit,
			},
			ln: ln,
		})
		ready.Roots = append(ready.Roots, readyRoot{
			ID:   root.id,
			Path: root.path,
			URL:  "http://" + ln.Addr().String(),
		})
	}

	controlLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		closeBound()
		return err
	}
	bound = append(bound, boundServer{
		srv: &http.Server{
			Handler:           newControlHandler(secret, sites),
			ReadHeaderTimeout: readHeaderLimit,
		},
		ln: controlLn,
	})
	ready.Control = "http://" + controlLn.Addr().String()

	line, err := json.Marshal(ready)
	if err != nil {
		closeBound()
		return err
	}

	served := make(chan error, len(bound))
	for _, b := range bound {
		go func(b boundServer) { served <- b.srv.Serve(b.ln) }(b)
	}
	// Every root reports what changes on disk from here on, so a document
	// written outside this host reaches its guard and the event stream.
	for _, site := range sites {
		site.watcher.start()
	}
	if _, err := fmt.Fprintf(stdout, "READY %s\n", line); err != nil {
		shutdownAll(bound)
		return err
	}
	// Snapshot entries nobody uploaded go stale, on the interval Local's
	// root-server pool sweeps them.
	stopSweep := startSnapshotSweep()
	_, _ = fmt.Fprintf(stderr, "managed-trial: serving %d roots, control on %s\n", len(roots), ready.Control)

	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		close(stdinClosed)
	}()

	var runErr error
	select {
	case <-ctx.Done():
	case <-stdinClosed:
	case err := <-served:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	}

	stopSweep()
	for _, site := range sites {
		site.watcher.stop()
	}
	if err := shutdownAll(bound); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

func shutdownAll(bound []boundServer) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	var firstErr error
	for _, b := range bound {
		if err := b.srv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func parseArgs(args []string, stderr io.Writer) (config, error) {
	var (
		cfg   config
		roots stringList
	)
	fs := flag.NewFlagSet("managed-trial", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Var(&roots, "root", "directory to serve; repeat for more roots")
	fs.IntVar(&cfg.port, "port", 0, "port for the first root; 0 picks a free port for every root")
	fs.StringVar(&cfg.localSource, "local-source", "", "Hyperclay Local checkout to read templates and assets from")
	fs.StringVar(&cfg.personName, "person-name", "", "person name to serve documents as")
	fs.IntVar(&cfg.controlFD, "control-fd", -1, "inherited file descriptor holding the control secret")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg.roots = roots
	if len(cfg.roots) == 0 {
		return config{}, errors.New("--root is required")
	}
	if cfg.port < 0 {
		return config{}, errors.New("--port must not be negative")
	}
	return cfg, nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func resolveRoots(raw []string, live string) ([]rootSpec, error) {
	roots := make([]rootSpec, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for i, root := range raw {
		if err := RefuseLiveRoot(root, live); err != nil {
			return nil, err
		}
		resolved, err := realPath(root)
		if err != nil {
			return nil, fmt.Errorf("--root %q: %w", root, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("--root %q: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("--root %q is not a directory", root)
		}
		if seen[resolved] {
			return nil, fmt.Errorf("--root %q is listed twice", root)
		}
		seen[resolved] = true
		roots = append(roots, rootSpec{id: fmt.Sprintf("root-%d", i), path: resolved})
	}
	return roots, nil
}

func checkLocalSource(dir string) error {
	if dir == "" {
		return errors.New("--local-source is required")
	}
	entry := filepath.Join(dir, "src", "main", "server.js")
	info, err := os.Stat(entry)
	if err != nil {
		return fmt.Errorf("--local-source %q is not a Hyperclay Local checkout: %w", dir, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("--local-source %q is not a Hyperclay Local checkout: %s is not a file", dir, entry)
	}
	return nil
}

func readControlSecret(fd int) (string, error) {
	file := os.NewFile(uintptr(fd), "control-fd")
	if file == nil {
		return "", fmt.Errorf("--control-fd %d is not an open file descriptor", fd)
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return "", errors.New("could not read the control secret from --control-fd")
	}
	secret := strings.TrimSpace(string(raw))
	if len(secret) != controlSecretLen || strings.Trim(secret, "0123456789abcdef") != "" {
		return "", errors.New("the control secret from --control-fd must be exactly 64 lowercase hex characters")
	}
	return secret, nil
}

func listenAddress(port, index int) string {
	if port == 0 {
		return "127.0.0.1:0"
	}
	return fmt.Sprintf("127.0.0.1:%d", port+index)
}

// newRootHandler serves one root: Local's static catch-all, its document lane
// and its folder listings.
func newRootHandler(root rootSpec, cfg config) (*rootServer, error) {
	return newRootServer(root, cfg)
}
