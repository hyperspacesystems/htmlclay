package recalc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// HiddenSourceHeader marks requests sent by the hidden browser.
	HiddenSourceHeader = "Clay-Recalc-Source"
	// HiddenSecretHeader carries a per-Runner random secret so the server can distinguish
	// real hidden-browser requests from anything an open page can forge.
	HiddenSecretHeader = "Clay-Recalc-Secret"
)

var (
	// ErrNoBrowser reports that no Chromium-family browser is available, the runner is closed,
	// or the document cannot recalculate on its own in the hidden browser.
	ErrNoBrowser = errors.New("no browser available")
	// ErrTimeout reports that the hidden page did not finish recalculating before the deadline.
	ErrTimeout = errors.New("recalculate timeout")
	// ErrPageError reports that the hidden page failed while recalculating.
	ErrPageError = errors.New("page error")
	// ErrSaveRefused reports that the document moved before the hidden page could save.
	ErrSaveRefused = errors.New("save refused")
)

const defaultIdleTimeout = 60 * time.Second

// Stats holds cumulative counts of guarded actions taken by the hidden browser.
type Stats struct {
	Dialogs   int64
	Popups    int64
	Downloads int64
	Blocked   int64
}

// Option configures a Runner.
type Option func(*Runner)

// WithBrowser pins the browser binary path (empty disables browser discovery).
func WithBrowser(bin string) Option {
	return func(r *Runner) {
		r.findBrowser = func() string { return bin }
	}
}

// WithBrowserFinder overrides browser discovery.
func WithBrowserFinder(fn func() string) Option {
	return func(r *Runner) {
		if fn != nil {
			r.findBrowser = fn
		}
	}
}

// WithIdleTimeout overrides how long the hidden browser stays warm between uses.
func WithIdleTimeout(d time.Duration) Option {
	return func(r *Runner) {
		if d > 0 {
			r.idleTimeout = d
		}
	}
}

// WithLogger sets a printf-style logger for unexpected browser failures.
func WithLogger(fn func(format string, args ...any)) Option {
	return func(r *Runner) {
		r.logf = fn
	}
}

type browserSession struct {
	proc       *browserProc
	cdp        *cdpConn
	profileDir string
	targetID   string
	sessionID  string
	loadCh     chan struct{}
}

// Runner drives one hidden Chromium-family browser over CDP pipes to recalculate documents
// when no open edit tab can. It starts lazily on first use, closes after idleTimeout, and
// shuts down cleanly on Close.
type Runner struct {
	sem         chan struct{}
	findBrowser func() string
	idleTimeout time.Duration
	logf        func(format string, args ...any)
	secret      string

	mu        sync.Mutex
	sess      *browserSession
	idleTimer *time.Timer
	closed    bool
	lastPID   atomic.Int64

	originMu    sync.RWMutex
	allowedHost string

	dialogs   atomic.Int64
	popups    atomic.Int64
	downloads atomic.Int64
	blocked   atomic.Int64
}

// New creates a lazy hidden-browser Runner.
func New(opts ...Option) *Runner {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	r := &Runner{
		sem:         make(chan struct{}, 1),
		findBrowser: FindBrowser,
		idleTimeout: defaultIdleTimeout,
		secret:      hex.EncodeToString(buf),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// HeaderSecret returns the per-Runner secret sent in HiddenSecretHeader.
func (r *Runner) HeaderSecret() string {
	return r.secret
}

// Stats returns the cumulative counts of intercepted dialogs, popups, canceled downloads, and blocked requests.
func (r *Runner) Stats() Stats {
	return Stats{
		Dialogs:   r.dialogs.Load(),
		Popups:    r.popups.Load(),
		Downloads: r.downloads.Load(),
		Blocked:   r.blocked.Load(),
	}
}

// LastPID returns the PID of the most recently started browser process (0 if none).
func (r *Runner) LastPID() int {
	return int(r.lastPID.Load())
}

func (r *Runner) isClosed() bool {
	r.mu.Lock()
	c := r.closed
	r.mu.Unlock()
	return c
}

// Close shuts down any running browser process group and prevents future runs.
func (r *Runner) Close() {
	r.mu.Lock()
	r.closed = true
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	sess := r.sess
	r.sess = nil
	r.mu.Unlock()
	if sess != nil {
		closeSession(sess)
	}
}

func closeSession(sess *browserSession) {
	if sess == nil {
		return
	}
	if sess.proc != nil {
		sess.proc.kill()
	}
	if sess.profileDir != "" {
		removeProfile(sess.profileDir)
	}
}

func removeProfile(dir string) {
	deadline := time.Now().Add(time.Second)
	for {
		err := os.RemoveAll(dir)
		if err == nil || runtime.GOOS != "windows" || time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *Runner) log(format string, args ...any) {
	if r.logf != nil {
		r.logf(format, args...)
	}
}

func (r *Runner) setAllowedHost(host string) {
	r.originMu.Lock()
	r.allowedHost = host
	r.originMu.Unlock()
}

func (r *Runner) getAllowedHost() string {
	r.originMu.RLock()
	h := r.allowedHost
	r.originMu.RUnlock()
	return h
}

// originAllowed reports whether rawURL is allowed by the origin lock for allowedHost.
// Only http://<allowedHost>/... is permitted. Under /_/, only /_/clayjs/, /_/meta/, and /_/save/
// are allowed: /_/sync* is suppressed so the hidden page never opens an SSE stream, and all other
// host action/dialog routes (/_/reveal/, /_/export/, /_/versions-folder/, /_/trust-request, etc.)
// are blocked. Returns (allowed, suppressSync).
func originAllowed(rawURL, allowedHost string) (allowed bool, suppressSync bool) {
	if allowedHost == "" {
		return false, false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, false
	}
	if !strings.EqualFold(u.Scheme, "http") || !strings.EqualFold(u.Host, allowedHost) {
		return false, false
	}
	if strings.HasPrefix(u.Path, "/_/") {
		if strings.HasPrefix(u.Path, "/_/sync") {
			return false, true
		}
		if strings.HasPrefix(u.Path, "/_/clayjs/") ||
			strings.HasPrefix(u.Path, "/_/meta/") ||
			strings.HasPrefix(u.Path, "/_/save/") {
			return true, false
		}
		return false, false
	}
	return true, false
}

// Recalculate loads docURL?editmode=true in the hidden browser and asks ClayJS to save the
// recalculated bytes against expectedEtag.
func (r *Runner) Recalculate(ctx context.Context, docURL, expectedEtag string) error {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return ErrTimeout
	}

	u, err := url.Parse(docURL)
	if err != nil || !strings.EqualFold(u.Scheme, "http") || u.Host == "" {
		return ErrPageError
	}
	q := u.Query()
	q.Set("editmode", "true")
	u.RawQuery = q.Encode()
	navURL := u.String()
	allowedHost := u.Host

	sess, err := r.acquireSession(ctx)
	if err != nil {
		return err
	}

	r.setAllowedHost(allowedHost)
	killBrowser := false
	defer func() {
		r.setAllowedHost("")
		if killBrowser || r.isClosed() {
			r.dropSession(sess)
			return
		}
		// Navigate back to about:blank and wait for its load event before releasing the semaphore,
		// so the document's scripts stop immediately and its about:blank load cannot race the next run.
		for len(sess.loadCh) > 0 {
			<-sess.loadCh
		}
		blankCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, blankErr := sess.cdp.call(blankCtx, sess.sessionID, "Page.navigate", map[string]any{"url": "about:blank"})
		if blankErr == nil {
			select {
			case <-sess.loadCh:
			case <-sess.cdp.closed:
				blankErr = errPipeClosed
			case <-blankCtx.Done():
				blankErr = blankCtx.Err()
			}
		}
		cancel()
		if blankErr != nil {
			r.dropSession(sess)
			return
		}
		r.armIdleTimer(sess)
	}()

	// Drain any stale load notification from a prior navigation.
	for len(sess.loadCh) > 0 {
		<-sess.loadCh
	}

	if _, err := sess.cdp.call(ctx, sess.sessionID, "Page.navigate", map[string]any{"url": navURL}); err != nil {
		killBrowser = true
		if r.isClosed() {
			return ErrNoBrowser
		}
		if ctx.Err() != nil {
			return ErrTimeout
		}
		r.log("Hidden recalculation navigate failed: %v", err)
		return ErrPageError
	}

	select {
	case <-sess.loadCh:
	case <-sess.cdp.closed:
		killBrowser = true
		if r.isClosed() {
			return ErrNoBrowser
		}
		r.log("Hidden browser pipe closed during page load")
		return ErrPageError
	case <-ctx.Done():
		killBrowser = true
		return ErrTimeout
	}

	waitMS := int64(4000)
	if dl, ok := ctx.Deadline(); ok {
		rem := time.Until(dl).Milliseconds() - 400
		if rem < 500 {
			rem = 500
		}
		if rem > 15000 {
			rem = 15000
		}
		waitMS = rem
	}

	expectedJSON, _ := json.Marshal(expectedEtag)
	expr := fmt.Sprintf(`(async () => {
		const expectedEtag = %s;
		const deadline = performance.now() + %d;
		while (!window.clay?.ready && performance.now() < deadline) {
			await new Promise(r => setTimeout(r, 10));
		}
		if (!window.clay?.ready) return JSON.stringify({ status: "no-clay" });
		await window.clay.ready;
		if (document.querySelector("[sap]")) {
			while (!window.Sap && performance.now() < deadline) {
				await new Promise(r => setTimeout(r, 10));
			}
		}
		const servedEtag = document.documentElement.getAttribute("documentetag") || "";
		if (expectedEtag && servedEtag !== expectedEtag) {
			return JSON.stringify({ status: "stale-load", servedEtag });
		}
		// Ensure host discovery (GET /_/meta/{token}) has settled so ClayJS sends If-Match.
		if (window.clay?.host?.versions) {
			try { await window.clay.host.versions(); } catch {}
		}
		await new Promise(r => setTimeout(r, 0));
		await new Promise(r => requestAnimationFrame(() => r()));
		const hasSap = !!document.querySelector("[sap]") && !!window.Sap;
		const hasEffects = (globalThis[Symbol.for("clay.effectsLive")] || 0) > 0;
		let computedChanged = false;
		if (Array.isArray(window.__clayRecalcSnap)) {
			for (const [el, txt, val] of window.__clayRecalcSnap) {
				if (!el.isConnected || el.textContent !== txt || ("value" in el ? el.value : "") !== val) {
					computedChanged = true;
					break;
				}
			}
		}
		if (!hasSap && !hasEffects && !computedChanged) {
			return JSON.stringify({ status: "cannot-recalc" });
		}
		if (!window.clay?.save?.force) return JSON.stringify({ status: "no-save" });
		const res = await window.clay.save.force(() => {}, { quietConflict: true });
		return JSON.stringify({
			status: "saved",
			ok: !!(res && res.ok),
			code: (res && res.code) || "",
			msgType: (res && res.msgType) || ""
		});
	})()`, string(expectedJSON), waitMS)

	raw, err := sess.cdp.call(ctx, sess.sessionID, "Runtime.evaluate", map[string]any{
		"expression":    expr,
		"awaitPromise":  true,
		"returnByValue": true,
	})
	if err != nil {
		killBrowser = true
		if r.isClosed() {
			return ErrNoBrowser
		}
		if ctx.Err() != nil {
			return ErrTimeout
		}
		r.log("Hidden recalculation evaluate failed: %v", err)
		return ErrPageError
	}

	var evalOut struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails any `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &evalOut); err != nil || evalOut.ExceptionDetails != nil {
		r.log("Hidden recalculation script exception")
		return ErrPageError
	}

	var outcome struct {
		Status  string `json:"status"`
		OK      bool   `json:"ok"`
		Code    string `json:"code"`
		MsgType string `json:"msgType"`
	}
	if err := json.Unmarshal([]byte(evalOut.Result.Value), &outcome); err != nil {
		r.log("Hidden recalculation returned unreadable result")
		return ErrPageError
	}
	switch outcome.Status {
	case "cannot-recalc", "no-clay", "no-save":
		r.log("Hidden recalculation skipped: status=%s", outcome.Status)
		return ErrNoBrowser
	case "stale-load":
		return ErrSaveRefused
	case "saved":
		if outcome.OK {
			return nil
		}
		if outcome.MsgType == "conflict" || outcome.Code == "conflict" || outcome.Code == "not-next-commit" || outcome.Code == "if-match-required" {
			return ErrSaveRefused
		}
		r.log("Hidden recalculation save failed: code=%s msgType=%s", outcome.Code, outcome.MsgType)
		return ErrPageError
	default:
		r.log("Hidden recalculation failed: status=%s", outcome.Status)
		return ErrPageError
	}
}

func (r *Runner) acquireSession(ctx context.Context) (*browserSession, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrNoBrowser
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	if r.sess != nil && !r.sess.cdp.isClosed() {
		sess := r.sess
		r.mu.Unlock()
		return sess, nil
	}
	old := r.sess
	r.sess = nil
	r.mu.Unlock()
	if old != nil {
		closeSession(old)
	}

	bin := r.findBrowser()
	if bin == "" {
		return nil, ErrNoBrowser
	}
	sess, err := r.launchSession(ctx, bin)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		closeSession(sess)
		return nil, ErrNoBrowser
	}
	r.sess = sess
	r.mu.Unlock()
	return sess, nil
}

func (r *Runner) dropSession(sess *browserSession) {
	r.mu.Lock()
	if r.sess == sess {
		r.sess = nil
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	r.mu.Unlock()
	closeSession(sess)
}

func (r *Runner) armIdleTimer(sess *browserSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.sess != sess {
		return
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
	}
	r.idleTimer = time.AfterFunc(r.idleTimeout, func() {
		// Only close when no recalculation is currently running.
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		default:
			return
		}
		r.mu.Lock()
		if r.sess != sess {
			r.mu.Unlock()
			return
		}
		r.sess = nil
		r.idleTimer = nil
		r.mu.Unlock()
		closeSession(sess)
	})
}

func (r *Runner) launchSession(ctx context.Context, bin string) (*browserSession, error) {
	profileDir, err := os.MkdirTemp(profileParentDir(bin), "htmlclay-recalc-*")
	if err != nil {
		return nil, fmt.Errorf("recalc temp profile: %w", err)
	}
	args := []string{
		headlessFlag(bin),
		"--remote-debugging-pipe",
		"--disable-gpu",
		"--mute-audio",
		"--use-mock-keychain",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-sync",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-default-apps",
		"--proxy-server=127.0.0.1:9",
		"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--user-data-dir=" + profileDir,
		"--host-resolver-rules=MAP localhost 127.0.0.1 , MAP *.localhost 127.0.0.1 , MAP * ~NOTFOUND",
	}
	if needsNoSandbox() {
		args = append(args, "--no-sandbox")
	}
	args = append(args, platformArgs()...)
	args = append(args, "about:blank")

	proc, err := startBrowser(bin, args)
	if err != nil {
		_ = os.RemoveAll(profileDir)
		if errors.Is(err, ErrNoBrowser) {
			return nil, ErrNoBrowser
		}
		r.log("Hidden browser start failed: %v", err)
		return nil, ErrNoBrowser
	}
	r.lastPID.Store(int64(proc.pid()))

	sess := &browserSession{
		proc:       proc,
		profileDir: profileDir,
		loadCh:     make(chan struct{}, 8),
	}
	sess.cdp = newCDPConn(proc.fromBrowser, proc.toBrowser, func(m cdpMsg) {
		r.handleEvent(sess, m)
	})

	cleanup := func(err error) (*browserSession, error) {
		if msg := strings.TrimSpace(proc.stderrText()); msg != "" {
			r.log("Hidden browser startup failed (%v): %s", err, msg)
		} else {
			r.log("Hidden browser startup failed: %v", err)
		}
		closeSession(sess)
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, ErrNoBrowser
	}

	if _, err := sess.cdp.call(ctx, "", "Target.setDiscoverTargets", map[string]any{"discover": true}); err != nil {
		return cleanup(err)
	}
	if _, err := sess.cdp.call(ctx, "", "Browser.setDownloadBehavior", map[string]any{"behavior": "deny", "eventsEnabled": true}); err != nil {
		return cleanup(err)
	}
	created, err := sess.cdp.call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		return cleanup(err)
	}
	var targetRes struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(created, &targetRes); err != nil || targetRes.TargetID == "" {
		return cleanup(errors.New("createTarget returned no targetId"))
	}
	sess.targetID = targetRes.TargetID

	attached, err := sess.cdp.call(ctx, "", "Target.attachToTarget", map[string]any{
		"targetId": targetRes.TargetID,
		"flatten":  true,
	})
	if err != nil {
		return cleanup(err)
	}
	var attachRes struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attached, &attachRes); err != nil || attachRes.SessionID == "" {
		return cleanup(errors.New("attachToTarget returned no sessionId"))
	}
	sess.sessionID = attachRes.SessionID

	if _, err := sess.cdp.call(ctx, sess.sessionID, "Page.enable", nil); err != nil {
		return cleanup(err)
	}
	if _, err := sess.cdp.call(ctx, sess.sessionID, "Runtime.enable", nil); err != nil {
		return cleanup(err)
	}
	if _, err := sess.cdp.call(ctx, sess.sessionID, "Network.enable", nil); err != nil {
		return cleanup(err)
	}
	if _, err := sess.cdp.call(ctx, sess.sessionID, "Network.setExtraHTTPHeaders", map[string]any{
		"headers": map[string]string{
			HiddenSourceHeader: "hidden",
			HiddenSecretHeader: r.secret,
		},
	}); err != nil {
		return cleanup(err)
	}
	if _, err := sess.cdp.call(ctx, sess.sessionID, "Fetch.enable", map[string]any{
		"patterns": []map[string]any{{"urlPattern": "*"}},
	}); err != nil {
		return cleanup(err)
	}
	if _, err := sess.cdp.call(ctx, sess.sessionID, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": `window.print = function() {};
window.open = function() { return null; };
document.addEventListener("DOMContentLoaded", function() {
  try {
    const snap = [];
    document.querySelectorAll('output, [clay~="no-dirty"]').forEach(el => {
      snap.push([el, el.textContent, "value" in el ? el.value : ""]);
    });
    window.__clayRecalcSnap = snap;
  } catch {}
}, { once: true });`,
	}); err != nil {
		return cleanup(err)
	}

	return sess, nil
}

func (r *Runner) handleEvent(sess *browserSession, m cdpMsg) {
	switch m.Method {
	case "Page.javascriptDialogOpening":
		var p struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(m.Params, &p)
		r.dialogs.Add(1)
		go sess.cdp.sendAsync(m.SessionID, "Page.handleJavaScriptDialog", map[string]any{
			"accept": p.Type == "beforeunload",
		})
	case "Target.targetCreated":
		var p struct {
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				OpenerID string `json:"openerId"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.TargetInfo.OpenerID != "" {
			r.popups.Add(1)
			go sess.cdp.sendAsync("", "Target.closeTarget", map[string]any{"targetId": p.TargetInfo.TargetID})
		}
	case "Browser.downloadProgress":
		var p struct {
			State string `json:"state"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.State == "canceled" {
			r.downloads.Add(1)
		}
	case "Page.loadEventFired":
		if m.SessionID == sess.sessionID {
			select {
			case sess.loadCh <- struct{}{}:
			default:
			}
		}
	case "Fetch.requestPaused":
		var p struct {
			RequestID string `json:"requestId"`
			Request   struct {
				URL string `json:"url"`
			} `json:"request"`
		}
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		allowed, suppressSync := originAllowed(p.Request.URL, r.getAllowedHost())
		if allowed {
			go sess.cdp.sendAsync(m.SessionID, "Fetch.continueRequest", map[string]any{"requestId": p.RequestID})
			return
		}
		if !suppressSync {
			r.blocked.Add(1)
		}
		go sess.cdp.sendAsync(m.SessionID, "Fetch.failRequest", map[string]any{
			"requestId":   p.RequestID,
			"errorReason": "BlockedByClient",
		})
	}
}
