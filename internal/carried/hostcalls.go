package carried

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/egress"
	"github.com/panphora/htmlclay/internal/gitread"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maxCalls       = 10000
	maxFileBytes   = gitread.MaxBytes
	maxWriteTotal  = 32 << 20
	maxFetchTotal  = 32 << 20
	maxFetchBody   = 1 << 20
	maxFetchHeader = 32
	maxListEntries = 5000
	maxOutputs     = 16
	fetchTimeout   = 30 * time.Second
	maxRedirects   = 5
)

var outputName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// hostCalls answers one run's calls. Every argument comes from the program
// and is checked here, whatever the worker already checked.
type hostCalls struct {
	job     *Job
	root    *os.Root
	client  *http.Client
	gate    *aitools.Gate
	calls   int
	written int64
	fetched int64
	outputs []Output
	kept    []byte
	sent    int
	cut     bool
}

func newHostCalls(job *Job) *hostCalls {
	h := &hostCalls{job: job, gate: job.Gate}
	if h.gate == nil {
		h.gate = &aitools.Gate{}
	}
	return h
}

func (h *hostCalls) close() {
	if h.root != nil {
		h.root.Close()
	}
	if h.client != nil {
		h.client.CloseIdleConnections()
	}
}

func (h *hostCalls) result() Result {
	return Result{Log: string(h.kept), LogCut: h.cut, Outputs: h.outputs}
}

// log keeps the end of the console log for the result and passes each piece
// on, until a run has sent 1 MB.
func (h *hostCalls) log(text string) {
	h.kept = append(h.kept, text...)
	if len(h.kept) > maxLogKept {
		h.kept = append([]byte(nil), h.kept[len(h.kept)-maxLogKept:]...)
	}
	if h.sent+len(text) > maxLogSent {
		h.cut = true
		return
	}
	h.sent += len(text)
	if h.job.Log != nil {
		h.job.Log(text)
	}
}

func (h *hostCalls) handle(ctx context.Context, name string, raw json.RawMessage) (json.RawMessage, *Error) {
	h.calls++
	if h.calls > maxCalls {
		return nil, fail(CodeLimit, "The program made more than %d calls to HTML Clay.", maxCalls)
	}
	var args []json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fail(CodeCallInvalid, "HTML Clay could not read the arguments of %s.", clip(name, 32))
	}
	value, err := h.dispatch(ctx, name, args)
	if err != nil {
		return nil, err
	}
	out, merr := json.Marshal(value)
	if merr != nil {
		return nil, fail(CodeCallInvalid, "HTML Clay could not send the answer of %s.", name)
	}
	if len(out) > maxReplyLine-4096 {
		return nil, fail(CodeTooLarge, "The answer from %s is too large for HTML Clay to pass to the program.", name)
	}
	return out, nil
}

func (h *hostCalls) dispatch(ctx context.Context, name string, args []json.RawMessage) (any, *Error) {
	c := h.job.Calls
	switch {
	case strings.HasPrefix(name, "files.") && !c.Files:
		return nil, fail(CodeCallRefused, "This program may not use files: its tag asks for the internet, which never comes with files.")
	case name == "files.write" && !c.Write:
		return nil, fail(CodeCallRefused, "This program may read files but not change them. Its tag must ask for change, and only a page you trust may change files.")
	case strings.HasPrefix(name, "git.") && !c.Git:
		return nil, fail(CodeCallRefused, "This program may not read git: its tag asks for the internet, which never comes with files.")
	case name == "fetch" && !c.Fetch:
		return nil, fail(CodeCallRefused, "This program may not use the internet. Its tag must ask for internet, and then it reaches no file.")
	}
	switch name {
	case "files.read":
		p, err := stringArg(args, 0, "path")
		if err != nil {
			return nil, err
		}
		return h.read(p)
	case "files.list":
		p := ""
		if len(args) > 0 && string(bytes.TrimSpace(args[0])) != "null" {
			var err *Error
			if p, err = stringArg(args, 0, "path"); err != nil {
				return nil, err
			}
		}
		return h.list(p)
	case "files.write":
		p, err := stringArg(args, 0, "path")
		if err != nil {
			return nil, err
		}
		text, err := stringArg(args, 1, "text")
		if err != nil {
			return nil, err
		}
		return h.write(p, text)
	case "git.log", "git.diff", "git.branches", "git.status", "git.show":
		return h.git(ctx, strings.TrimPrefix(name, "git."), args)
	case "fetch":
		return h.fetch(ctx, args)
	case "output":
		n, err := stringArg(args, 0, "name")
		if err != nil {
			return nil, err
		}
		text, err := stringArg(args, 1, "text")
		if err != nil {
			return nil, err
		}
		return h.put(n, text)
	}
	return nil, fail(CodeCallInvalid, "HTML Clay has no call named %q.", clip(name, 32))
}

func stringArg(args []json.RawMessage, i int, what string) (string, *Error) {
	if i >= len(args) {
		return "", fail(CodeCallInvalid, "The %s must be a string.", what)
	}
	raw := bytes.TrimSpace(args[i])
	if len(raw) == 0 || raw[0] != '"' {
		return "", fail(CodeCallInvalid, "The %s must be a string.", what)
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", fail(CodeCallInvalid, "The %s must be a string.", what)
	}
	return s, nil
}

// --- files -------------------------------------------------------------

func (h *hostCalls) openRoot() (*os.Root, *Error) {
	if h.root != nil {
		return h.root, nil
	}
	root, err := os.OpenRoot(h.job.Folder)
	if err != nil {
		return nil, fail(CodeFolderUnreadable, "HTML Clay cannot open the folder %s.", path.Base(h.job.Folder))
	}
	h.root = root
	return root, nil
}

// repositoryFolders hold a repository's own settings, which can name
// programs to run and hold sign-in tokens. No file call reaches them: a
// program reads history through clay.git.
var repositoryFolders = map[string]bool{".git": true, ".hg": true, ".svn": true}

// runsCommands are files and folders other tools run commands from when they
// open the folder, check it out or push it. A program may read them but never
// write them, so changing files never becomes running code.
var runsCommands = map[string]bool{
	".vscode": true, ".idea": true, ".claude": true, ".codex": true, ".cursor": true, ".gemini": true,
	".gitmodules": true, ".gitattributes": true, ".husky": true,
}

// partKeys returns the normalized keys a file system may match part under:
// full Unicode case folding + NFC (APFS: ſ->s, ﬂ->fl) and simple uppercase
// folded back to lowercase (NTFS), after stripping HFS+ ignorable code
// points and Windows trailing dots/spaces.
func partKeys(part string) [2]string {
	stripped := strings.Map(func(r rune) rune {
		switch {
		case r >= 0x200c && r <= 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x206a && r <= 0x206f, r == 0xfeff:
			return -1
		}
		return r
	}, part)
	trimmed := strings.TrimRight(stripped, ". ")
	folded := strings.TrimRight(norm.NFC.String(cases.Fold().String(trimmed)), ". ")
	upper := strings.TrimRight(strings.ToLower(strings.ToUpper(trimmed)), ". ")
	return [2]string{folded, upper}
}

func matchAnyKey(part string, set map[string]bool) (string, bool) {
	for _, k := range partKeys(part) {
		if set[k] {
			return k, true
		}
	}
	return "", false
}

func isKey(part, target string) bool {
	for _, k := range partKeys(part) {
		if k == target {
			return true
		}
	}
	return false
}

// shortName reports a Windows 8.3 short name such as GIT~1, which reaches
// .git on a drive that keeps short names.
func shortName(key string) bool {
	i := strings.IndexByte(key, '~')
	return i >= 0 && i+1 < len(key) && key[i+1] >= '0' && key[i+1] <= '9'
}

// inside turns a program's path into one os.Root opens. An absolute path, a
// drive, a backslash, "..", a short name and any part naming a repository's
// own folder are refused here, and so are the settings other tools run
// commands from when the call writes. os.Root refuses a link that leads out
// of the folder; parents refuses one that stays inside.
func inside(p string, folderOK, write bool) (string, *Error) {
	name := strings.TrimPrefix(p, "./")
	if folderOK && (name == "" || name == ".") {
		return ".", nil
	}
	if !gitread.ValidPath(name) {
		return "", fail(CodeCallRefused, "%q is not a path inside the folder.", clip(p, 200))
	}
	name = path.Clean(name)
	parts := strings.Split(name, "/")
	for i, part := range parts {
		keys := partKeys(part)
		key, keyed := matchAnyKey(part, repositoryFolders)
		var commands bool
		if write {
			if _, commands = matchAnyKey(part, runsCommands); !commands {
				commands = isKey(part, ".github") && i+1 < len(parts) && isKey(parts[i+1], "workflows")
			}
		}
		switch {
		case keyed:
			return "", fail(CodeCallRefused, "A program reads a repository only through clay.git, never the files inside %s.", key)
		case shortName(keys[0]) || shortName(keys[1]):
			return "", fail(CodeCallRefused, "%q looks like a Windows short name, which can stand for a hidden folder. Use the full name.", clip(part, 64))
		case commands:
			return "", fail(CodeCallRefused, "HTML Clay never lets a program write %q: other tools run commands from it.", clip(name, 200))
		}
	}
	return name, nil
}

// parents checks every folder on the way to name: each must be a plain
// folder, never a link, even one that stays inside the folder. With create, a
// missing folder is made.
func parents(root *os.Root, name string, create bool) *Error {
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		info, err := root.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			if !create {
				return fail(CodeNotFound, "There is no folder %q in the folder.", dir)
			}
			if err := root.Mkdir(dir, 0o755); err != nil {
				return fail(CodeCallRefused, "HTML Clay could not make the folder %q.", dir)
			}
			continue
		}
		if err != nil || !info.IsDir() {
			return fail(CodeCallRefused, "%q is a link or a file, not a folder HTML Clay goes through.", dir)
		}
	}
	return nil
}

func (h *hostCalls) plainFile(root *os.Root, name string) (fs.FileInfo, *Error) {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fail(CodeNotFound, "There is no file %q in the folder.", name)
	}
	if err != nil {
		return nil, fail(CodeCallRefused, "%q is not a file HTML Clay may read inside the folder.", name)
	}
	if !info.Mode().IsRegular() {
		return nil, fail(CodeNotAFile, "%q is a link, a folder or a device, not a plain file.", name)
	}
	return info, nil
}

func (h *hostCalls) read(p string) (any, *Error) {
	name, err := inside(p, false, false)
	if err != nil {
		return nil, err
	}
	root, err := h.openRoot()
	if err != nil {
		return nil, err
	}
	if err := parents(root, name, false); err != nil {
		return nil, err
	}
	info, err := h.plainFile(root, name)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFileBytes {
		return nil, fail(CodeTooLarge, "%q is larger than 4 MB.", name)
	}
	f, oerr := root.OpenFile(name, os.O_RDONLY|openNonBlock, 0)
	if oerr != nil {
		return nil, fail(CodeCallRefused, "%q is not a file HTML Clay may read inside the folder.", name)
	}
	defer f.Close()
	if st, serr := f.Stat(); serr != nil || !st.Mode().IsRegular() {
		return nil, fail(CodeNotAFile, "%q is not a plain file.", name)
	}
	data, rerr := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if rerr != nil {
		return nil, fail(CodeCallRefused, "HTML Clay could not read %q.", name)
	}
	if len(data) > maxFileBytes {
		return nil, fail(CodeTooLarge, "%q is larger than 4 MB.", name)
	}
	if !utf8.Valid(data) {
		return nil, fail(CodeNotText, "%q is not UTF-8 text, so the program cannot read it.", name)
	}
	return string(data), nil
}

type listEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

func (h *hostCalls) list(p string) (any, *Error) {
	name, err := inside(p, true, false)
	if err != nil {
		return nil, err
	}
	root, err := h.openRoot()
	if err != nil {
		return nil, err
	}
	if err := parents(root, name, false); err != nil {
		return nil, err
	}
	if info, lerr := root.Lstat(name); lerr != nil || !info.IsDir() {
		return nil, fail(CodeNotFound, "There is no folder %q in the folder.", name)
	}
	dir, oerr := root.Open(name)
	if oerr != nil {
		return nil, fail(CodeCallRefused, "%q is not a folder HTML Clay may read.", name)
	}
	defer dir.Close()
	entries, rerr := dir.ReadDir(maxListEntries + 1)
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return nil, fail(CodeCallRefused, "HTML Clay could not read the folder %q.", name)
	}
	if len(entries) > maxListEntries {
		return nil, fail(CodeTooLarge, "The folder %q holds more than %d entries.", name, maxListEntries)
	}
	out := make([]listEntry, 0, len(entries))
	for _, e := range entries {
		if _, hidden := matchAnyKey(e.Name(), repositoryFolders); hidden {
			continue
		}
		entry := listEntry{Name: e.Name(), Type: "other"}
		switch t := e.Type(); {
		case t.IsRegular():
			entry.Type = "file"
			if info, ierr := e.Info(); ierr == nil {
				entry.Size = info.Size()
			}
		case t.IsDir():
			entry.Type = "folder"
		case t&fs.ModeSymlink != 0:
			entry.Type = "link"
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// write replaces or creates a plain file through a new file renamed over
// it, so a run that stops halfway leaves the old file whole. Missing folders
// on the way are made.
func (h *hostCalls) write(p, text string) (any, *Error) {
	name, err := inside(p, false, true)
	if err != nil {
		return nil, err
	}
	if len(text) > maxFileBytes {
		return nil, fail(CodeTooLarge, "A program writes at most 4 MB to one file.")
	}
	if h.written+int64(len(text)) > maxWriteTotal {
		return nil, fail(CodeLimit, "A program writes at most 32 MB in one run.")
	}
	root, err := h.openRoot()
	if err != nil {
		return nil, err
	}
	if err := parents(root, name, true); err != nil {
		return nil, err
	}
	mode := os.FileMode(0o644)
	if info, lerr := root.Lstat(name); lerr == nil {
		if !info.Mode().IsRegular() {
			return nil, fail(CodeNotAFile, "%q is a link, a folder or a device, so HTML Clay does not write it.", name)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return nil, fail(CodeCallRefused, "%q is not a file HTML Clay may write inside the folder.", name)
	}
	var suffix [6]byte
	_, _ = rand.Read(suffix[:])
	tmp := path.Join(path.Dir(name), ".clay-write-"+hex.EncodeToString(suffix[:]))
	f, oerr := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if oerr != nil {
		return nil, fail(CodeCallRefused, "HTML Clay could not write %q.", name)
	}
	_, werr := f.WriteString(text)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = root.Rename(tmp, name)
	}
	if werr != nil {
		_ = root.Remove(tmp)
		return nil, fail(CodeCallRefused, "HTML Clay could not write %q.", name)
	}
	h.written += int64(len(text))
	return map[string]any{"path": name, "bytes": len(text)}, nil
}

// --- git ---------------------------------------------------------------

type gitOptions struct {
	Base  string   `json:"base"`
	Head  string   `json:"head"`
	Max   int      `json:"max"`
	Paths []string `json:"paths"`
}

func (h *hostCalls) git(ctx context.Context, verb string, args []json.RawMessage) (any, *Error) {
	var o gitOptions
	if verb != "show" && len(args) > 0 && string(args[0]) != "null" {
		if err := json.Unmarshal(args[0], &o); err != nil {
			return nil, fail(CodeCallInvalid, "clay.git.%s takes an options object.", verb)
		}
	}
	var rev, name string
	if verb == "show" {
		var e *Error
		if rev, e = stringArg(args, 0, "revision"); e != nil {
			return nil, e
		}
		if name, e = stringArg(args, 1, "path"); e != nil {
			return nil, e
		}
	}
	if o.Head == "" {
		o.Head = "HEAD"
	}
	var value any
	err := h.gate.Do(ctx, h.job.Folder, func(ctx context.Context) error {
		var err error
		switch verb {
		case "log":
			if o.Base == "" {
				value, err = gitread.LogFrom(ctx, h.job.Folder, o.Head, o.Max)
			} else {
				value, err = gitread.Log(ctx, h.job.Folder, o.Base, o.Head, o.Max)
			}
		case "diff":
			var p *gitread.Patch
			p, err = gitread.Diff(ctx, h.job.Folder, gitread.DiffOptions{Base: o.Base, Head: o.Head, Paths: o.Paths, Exclude: gitread.DefaultExclude})
			if err == nil {
				value = map[string]any{"base": p.Base, "head": p.Head, "mergeBase": p.MergeBase, "text": string(p.Text), "files": p.Files, "skipped": p.Skipped, "truncated": p.Truncated}
			}
		case "branches":
			value, err = gitread.Branches(ctx, h.job.Folder)
		case "status":
			value, err = gitread.Status(ctx, h.job.Folder)
		case "show":
			var data []byte
			if data, err = gitread.FileAt(ctx, h.job.Folder, rev, name); err == nil && !utf8.Valid(data) {
				err = errNotText
			}
			value = string(data)
		}
		return err
	})
	if err != nil {
		if errors.Is(err, errNotText) {
			return nil, fail(CodeNotText, "%q is not UTF-8 text, so the program cannot read it.", name)
		}
		return nil, gitFailure(err)
	}
	return value, nil
}

// errNotText marks a file at a commit that is not UTF-8 text, which a program
// gets back as CodeNotText rather than as mangled bytes.
var errNotText = errors.New("not UTF-8 text")

func gitFailure(err error) *Error {
	var refused *gitread.Refused
	var unsupported *gitread.Unsupported
	switch {
	case errors.As(err, &refused):
		return fail(CodeCallInvalid, "HTML Clay refused the %s %q.", refused.What, clip(refused.Value, 200))
	case errors.As(err, &unsupported):
		return fail(CodeGitFailed, "This repository uses %s, which HTML Clay cannot read.", unsupported.Feature)
	case errors.Is(err, gitread.ErrNotRepository):
		return fail(CodeGitFailed, "The folder is not a git repository.")
	case errors.Is(err, gitread.ErrUnknownRef):
		return fail(CodeGitFailed, "There is no such branch, tag or commit.")
	case errors.Is(err, gitread.ErrNoFile):
		return fail(CodeNotFound, "There is no such file at that commit.")
	case errors.Is(err, gitread.ErrTooLarge):
		return fail(CodeTooLarge, "The file is larger than 4 MB.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, gitread.ErrTimeout):
		return fail(CodeGitFailed, "Reading the repository took too long.")
	}
	return fail(CodeGitFailed, "HTML Clay could not read the repository.")
}

// --- fetch -------------------------------------------------------------

type fetchOptions struct {
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    *string           `json:"body"`
}

var refusedHeaders = map[string]bool{
	"host": true, "connection": true, "content-length": true, "transfer-encoding": true, "te": true,
	"trailer": true, "upgrade": true, "keep-alive": true, "proxy-authorization": true, "proxy-connection": true,
}

var fetchMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}

func publicHTTP(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

func (h *hostCalls) httpClient() *http.Client {
	if h.client != nil {
		return h.client
	}
	dial := h.job.dial
	if dial == nil {
		dial = egress.DialPublic
	}
	h.client = &http.Client{
		Transport: &http.Transport{
			Proxy:                  nil,
			DialContext:            dial,
			ForceAttemptHTTP2:      true,
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  fetchTimeout,
			MaxResponseHeaderBytes: 64 << 10,
		},
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("more than %d redirects", maxRedirects)
			}
			if !publicHTTP(req.URL) {
				return errors.New("a redirect to an address that is not http or https")
			}
			return nil
		},
	}
	return h.client
}

func (h *hostCalls) fetch(ctx context.Context, args []json.RawMessage) (any, *Error) {
	raw, err := stringArg(args, 0, "address")
	if err != nil {
		return nil, err
	}
	u, perr := url.Parse(raw)
	if perr != nil || !publicHTTP(u) {
		return nil, fail(CodeCallInvalid, "clay.fetch takes an http or https address.")
	}
	var o fetchOptions
	if len(args) > 1 && string(args[1]) != "null" {
		if json.Unmarshal(args[1], &o) != nil {
			return nil, fail(CodeCallInvalid, "clay.fetch's options must be an object of method, headers and body.")
		}
	}
	method := strings.ToUpper(o.Method)
	if method == "" {
		method = "GET"
	}
	if !fetchMethods[method] {
		return nil, fail(CodeCallInvalid, "clay.fetch does not send %q requests.", clip(method, 16))
	}
	if len(o.Headers) > maxFetchHeader {
		return nil, fail(CodeCallInvalid, "clay.fetch sends at most %d headers.", maxFetchHeader)
	}
	var body io.Reader
	if o.Body != nil {
		if len(*o.Body) > maxFetchBody {
			return nil, fail(CodeTooLarge, "clay.fetch sends a body of at most 1 MB.")
		}
		body = strings.NewReader(*o.Body)
	}
	req, rerr := http.NewRequestWithContext(ctx, method, u.String(), body)
	if rerr != nil {
		return nil, fail(CodeCallInvalid, "clay.fetch could not make that request.")
	}
	for k, v := range o.Headers {
		if refusedHeaders[strings.ToLower(k)] || strings.HasPrefix(strings.ToLower(k), "proxy-") {
			return nil, fail(CodeCallInvalid, "clay.fetch does not let a program set the %s header.", clip(k, 64))
		}
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "HTML Clay program")
	}
	resp, derr := h.httpClient().Do(req)
	if derr != nil {
		if errors.Is(derr, egress.ErrNotPublic) {
			return nil, fail(CodeFetchRefused, "HTML Clay lets a program reach only public internet addresses, never this computer or a private network.")
		}
		return nil, fail(CodeFetchFailed, "The request to %s failed.", u.Hostname())
	}
	defer resp.Body.Close()
	data, berr := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes+1))
	if berr != nil {
		return nil, fail(CodeFetchFailed, "The answer from %s could not be read.", u.Hostname())
	}
	if len(data) > maxFileBytes {
		return nil, fail(CodeTooLarge, "The answer from %s is larger than 4 MB.", u.Hostname())
	}
	h.fetched += int64(len(data))
	if h.fetched > maxFetchTotal {
		return nil, fail(CodeLimit, "A program fetches at most 32 MB in one run.")
	}
	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[strings.ToLower(k)] = strings.Join(v, ", ")
		}
	}
	return map[string]any{
		"status":  resp.StatusCode,
		"ok":      resp.StatusCode >= 200 && resp.StatusCode < 300,
		"url":     resp.Request.URL.String(),
		"headers": headers,
		"text":    string(data),
	}, nil
}

// --- output ------------------------------------------------------------

func (h *hostCalls) put(name, text string) (any, *Error) {
	if !outputName.MatchString(name) {
		return nil, fail(CodeCallInvalid, "An output name is 1 to 64 letters, digits, hyphens and underscores.")
	}
	if h.job.Put == nil {
		return nil, fail(CodeOutputRefused, "This host keeps no output files.")
	}
	if len(h.outputs) == maxOutputs {
		return nil, fail(CodeLimit, "A program writes at most %d outputs.", maxOutputs)
	}
	for _, f := range h.outputs {
		if f.Name == name {
			return nil, fail(CodeCallInvalid, "This run already has an output named %q.", name)
		}
	}
	addr, err := h.job.Put(name, []byte(text))
	if err != nil {
		var coded *Error
		if errors.As(err, &coded) {
			return nil, coded
		}
		return nil, fail(CodeOutputRefused, "HTML Clay could not keep the output %q.", name)
	}
	out := Output{Name: name, URL: addr, Bytes: len(text)}
	h.outputs = append(h.outputs, out)
	return out, nil
}
