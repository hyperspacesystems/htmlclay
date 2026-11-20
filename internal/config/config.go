package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

type Config struct {
	// mu guards every field below. The one Config is shared across goroutines: the
	// route path writes SitePorts, the tray writes StartOnLogin, and the Trusted
	// Folders hooks write TrustedFolders, and Save marshals all of them at once.
	// Without one lock a concurrent SitePorts write + marshal panics
	// ("concurrent map iteration and write") and a slice append tears under marshal.
	// Every exported mutator and Save take it; callers must not touch the fields
	// directly. Lock order is a.mu -> cfg.mu (nothing takes cfg.mu then a.mu).
	mu sync.Mutex
	// holdSaves keeps every write in memory while an update is judged; saveOwed
	// records that one was asked for (HoldSaves, ReleaseSaves).
	holdSaves    bool
	saveOwed     bool
	StartOnLogin bool `json:"startOnLogin"`
	// ListenPort is the one loopback port every site is served on, held on 127.0.0.1 and
	// ::1 for as long as the app runs. 0 until the first launch chooses one. The port is
	// part of the browser origin, so a change here resets every page's browser storage,
	// which is why the app asks before moving it. Read through Port().
	ListenPort int `json:"port,omitempty"`
	// FormerPorts are ports the app served on before the person moved it, newest
	// last. Startup holds each one that is free and sends page visits on to the
	// same address on the current port. At most formerPortCap are kept.
	FormerPorts []int `json:"formerPorts,omitempty"`
	// SiteSlugs maps an origin key to the label it is served under, <label>.localhost:<port>.
	// The key is the site's anchor (a trusted folder, or the folder a loose file was opened
	// from), or the file's own path for a file loose in the home directory, which can
	// install no read root and so shares its anchor with every other such file. A slug is
	// chosen once and never changes, so a bookmark survives restarts. Trusted folders'
	// entries are never evicted; the rest are capped like SitePorts.
	SiteSlugs map[string]string `json:"siteSlugs,omitempty"`
	// RetiredSlugs are labels that were once issued and then forgotten (the folder was
	// untrusted, or the entry was evicted). They are never issued again, so no folder can
	// inherit another's browser storage, and their addresses answer the recovery page.
	RetiredSlugs []string `json:"retiredSlugs,omitempty"`
	// NextSiteNumber is the number the next new slug gets: 2 is htmlclay1, 3 is htmlclay2, and so
	// on. 1 was htmlclay, whose address is the dashboard's now, so it is never issued. It only
	// grows, so a label is never issued twice and no folder inherits another's browser storage.
	NextSiteNumber int `json:"nextSiteNumber,omitempty"`
	// MovedFirstSite is the origin key that was served under htmlclay before
	// htmlclay.localhost became the dashboard's address. The dashboard's host sends a
	// navigation for a document under this key to the address the key has now. Written
	// once, by moveFirstSite.
	MovedFirstSite string `json:"movedFirstSite,omitempty"`
	// DashboardCleanMark is made once for a config that ever served a site at
	// htmlclay.localhost. What that site stored in a browser sits at the dashboard's
	// address now, so each browser's first entry to the dashboard clears it and keeps
	// this value in a cookie as the record that it was cleared. Written by
	// markFirstSiteOrigin alone.
	DashboardCleanMark string `json:"dashboardCleanMark,omitempty"`
	// SitePorts is the pre-hostname record of the loopback port each origin was served on.
	// Kept so an old bookmark can be redirected: startup binds each remembered port that is
	// still free with a redirect to the folder's hostname, and drops the entry the first
	// time the port is found taken. Nothing new is ever written here.
	SitePorts map[string]int `json:"sitePorts,omitempty"`
	// TrustedFolders are folders the user declared theirs. HTML Clay files under
	// one open editable with no prompts, including files added later, and any
	// file in it can change any other. This is the app's one durable permission
	// fact: it grants read and write, it anchors exactly one origin, and it is
	// the key that origin's port is remembered under.
	//
	// These are never pruned on Load: a trusted folder is a standing write
	// capability, and a dead or identity-changed entry must surface in the tray
	// as dead rather than silently vanish from the record of what the user
	// granted.
	//
	// The on-disk key stays "workspaceFolders" even though the concept is now
	// called a trusted folder. Reusing the "trustedFolders" key would make
	// json.Unmarshal fail against 1.2.0 configs, where it held a []string, and
	// the corrupt-config path would then reset every setting the user has.
	TrustedFolders []TrustedFolder `json:"workspaceFolders,omitempty"`
	// LegacyTrusted is 1.2.0's read-only trusted folder list. It is promoted to
	// TrustedFolders once on Load and cleared on the next Save, which is the
	// completion marker. Distinct Go field, distinct JSON key, distinct type, so
	// the decoder never sees a shape it does not expect.
	LegacyTrusted []string `json:"trustedFolders,omitempty"`
	// ServedFolders are folders served whole at http://<name>.htmlclay.localhost:<port>/.
	// A served folder is always also a trusted folder.
	ServedFolders []ServedFolder `json:"servedFolders,omitempty"`
	// RetiredServedNames are names once given to a served folder that was then removed.
	// A name is never given out again, so an old address never reaches a different folder.
	RetiredServedNames []string         `json:"retiredServedNames,omitempty"`
	HelperPrograms     []HelperProgram  `json:"helperPrograms,omitempty"`
	HelperDecisions    []HelperDecision `json:"helperDecisions,omitempty"`
	// AIEdit is the built-in AI editing helper's setting. Nil, or a nil Enabled,
	// reads as on: AI editing is on unless the person turned it off.
	AIEdit *AIEditSettings `json:"aiEdit,omitempty"`
	// AIEditDocuments are the documents the person allowed AI editing for where
	// the switch alone is not enough: a team folder, or an unreadable team record.
	AIEditDocuments []AIEditDocument `json:"aiEditDocuments,omitempty"`
	// BuiltinPrograms are the built-in programs' switches and the documents the
	// person allowed for each, keyed by program id (internal/builtin). A missing
	// entry is on: documents the person made, and signed ecosystem apps, use the
	// built-in programs without a question unless the person switched one off.
	BuiltinPrograms map[string]*BuiltinProgram `json:"builtinPrograms,omitempty"`
	// StopOpeningFiles is true after the person chose "Stop opening .htmlclay
	// files", so the startup file type refresh leaves the system's choice alone.
	// Make HTML Clay the default clears it.
	StopOpeningFiles bool `json:"stopOpeningFiles,omitempty"`
	// HTMLOpenWithOff is true when the person took HTML Clay out of Open With for
	// .html files (Windows and Linux; the Mac's claim is fixed in Info.plist).
	HTMLOpenWithOff bool `json:"htmlOpenWithOff,omitempty"`
	// Profile is the person HTML Clay names in files it serves, when Enabled. Nil
	// means never set up, which reads as off. The id survives renames and toggles.
	Profile *Profile `json:"profile,omitempty"`
	// AppearanceSetting is the person's chosen appearance: "system", "light" or
	// "dark". Empty reads as system. Read through Appearance().
	AppearanceSetting string `json:"appearance,omitempty"`
	// Updates set to "off" keeps the self-updater from downloading or applying
	// anything, for machines whose software is managed some other way. The tray
	// still links to the download page. Anything else, or nothing, leaves it on.
	Updates string `json:"updates,omitempty"`
	// TeamRecord is true once team-folders.json has had its first entry. It is the
	// only trace of the team record in config.json, so that a record that has gone
	// missing reads as unknown -- nothing may be granted broadly -- rather than as a
	// machine that never had a team folder. Written by MarkTeamFolder alone.
	TeamRecord bool `json:"teamRecord,omitempty"`
	baseDir    string
	writes     int
}

// TrustedFolder is one declared folder. Identity is the folder's device+inode
// fingerprint at declaration time ("" where the platform cannot provide one);
// callers compare it before installing so a directory swapped for a symlink
// since declaration is refused under the old name instead of granting write
// over whatever tree the link now points at.
type TrustedFolder struct {
	// ID is the folder's own id, 16 lowercase hex, made once when the folder is
	// first trusted and kept for as long as the entry lives. Serving the folder
	// does not change it, so every holder follows the folder by id, not by path.
	ID       string `json:"id,omitempty"`
	Path     string `json:"path"`
	Identity string `json:"identity,omitempty"`
	// OwnAddress is the address this folder keeps when a folder around it is
	// trusted: its documents already answered there, and what the browser stored
	// for them (logins, local storage) lives at that address. It is set only when
	// a broader folder is trusted around a folder that already had an address, so
	// no configuration written before it existed has it, and every one of those
	// routes exactly as it always has.
	OwnAddress bool `json:"ownAddress,omitempty"`
}

// ServedFolder is one folder HTML Clay serves whole. ID is 16 lowercase hex, made once;
// it is the folder's root id on the control API. Serving is false while the person has
// stopped serving it but kept it in the list.
type ServedFolder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Identity string `json:"identity,omitempty"`
	Serving  bool   `json:"serving"`

	AllowFraming bool `json:"allowFraming,omitempty"`
}

var (
	ErrServedNameInvalid = errors.New("served-name-invalid")
	ErrServedNameTaken   = errors.New("served-name-taken")
	ErrServedNameRetired = errors.New("served-name-retired")
	ErrAlreadyServed     = errors.New("already-served")
)

// reservedServedNames can never name a served folder: they are, or could become, the
// app's own addresses.
var reservedServedNames = map[string]bool{"dashboard": true, "hyperclay": true, "htmlclay": true, "localhost": true, "www": true, "api": true}

var servedNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)
var servedIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// AIEditSettings mirrors Hyperclay Local's settings.aiEdit. Default names the engine
// used when a request names none ("" means claude). Engines are user-defined agents:
// name -> argv, where an argument containing "{prompt}" receives the prompt.
type AIEditSettings struct {
	Enabled *bool               `json:"enabled,omitempty"`
	Default string              `json:"default,omitempty"`
	Engines map[string][]string `json:"engines,omitempty"`
	// Models is the model each engine runs with, by engine name. A missing or
	// empty entry is the engine's own default.
	Models map[string]string `json:"models,omitempty"`
	// Efforts is the reasoning effort each engine runs with, by engine name. A
	// missing or empty entry is HTML Clay's default for that engine.
	Efforts map[string]string `json:"efforts,omitempty"`
}

// sitePortCap bounds the remembered-port map. Every entry becomes a bound
// listener at startup, so an unbounded map is an unbounded number of sockets
// held from login. A trusted folder's entry is never evicted: it is the
// bookmark contract. Ad-hoc entries are evicted only when the map is over the
// cap, stat-failures first, because a stat failure cannot distinguish a
// deleted folder from an unmounted volume and dropping a merely unmounted
// drive's port would move a URL the user still has open in a tab.
const sitePortCap = 32

// AddTrustedFolder records dir with its identity fingerprint, returning false
// if the path was already present. dir must already be canonical (resolved,
// home-contained); the caller validates before adding, so containment checks
// here can stay simple.
func (c *Config) AddTrustedFolder(dir, identity string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.addTrustedFolderLocked(dir, identity)
}

// addTrustedFolderLocked records dir unless it is already there, with the one id
// the entry keeps for life. The caller holds c.mu.
func (c *Config) addTrustedFolderLocked(dir, identity string) bool {
	for _, w := range c.TrustedFolders {
		if w.Path == dir {
			return false
		}
	}
	id, _ := c.newFolderIDLocked()
	c.TrustedFolders = append(c.TrustedFolders, TrustedFolder{ID: id, Path: dir, Identity: identity})
	return true
}

// ErrFolderIDHeld is TrustFolderWithID refusing an id another folder already holds,
// or a path already trusted under another id.
var ErrFolderIDHeld = errors.New("the folder id is held by another folder")

// TrustFolderWithID trusts dir under id, the id a synced account's folder was given
// before it was on this computer, so serving it later borrows that id. A path
// already trusted under the same id is left as it is.
func (c *Config) TrustFolderWithID(dir, identity, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range c.TrustedFolders {
		if w.Path == dir {
			if w.ID == id {
				return nil
			}
			return ErrFolderIDHeld
		}
		if w.ID == id {
			return ErrFolderIDHeld
		}
	}
	for _, f := range c.ServedFolders {
		if f.ID == id {
			return ErrFolderIDHeld
		}
	}
	c.TrustedFolders = append(c.TrustedFolders, TrustedFolder{ID: id, Path: dir, Identity: identity})
	return nil
}

// NewFolderID mints a folder id no folder in the config holds, for a folder that
// is listed before it is on this computer, such as a synced account's.
func (c *Config) NewFolderID() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.newFolderIDLocked()
}

// newFolderIDLocked mints a folder id: 16 lowercase hex, the shape served folders
// have always used, never one any folder in the config already holds. The caller
// holds c.mu.
func (c *Config) newFolderIDLocked() (string, error) {
	for {
		var raw [8]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", fmt.Errorf("cannot generate folder id: %w", err)
		}
		id := hex.EncodeToString(raw[:])
		taken := false
		for _, f := range c.ServedFolders {
			if f.ID == id {
				taken = true
				break
			}
		}
		if !taken {
			for _, w := range c.TrustedFolders {
				if w.ID == id {
					taken = true
					break
				}
			}
		}
		if !taken {
			return id, nil
		}
	}
}

// trustedFolderIDLocked answers the id the trusted entry at path keeps, giving an
// entry that has none the one id it will hold from now on. The caller holds c.mu
// and has already made sure the entry exists.
func (c *Config) trustedFolderIDLocked(path string) (string, bool, error) {
	for i := range c.TrustedFolders {
		if c.TrustedFolders[i].Path != path {
			continue
		}
		if c.TrustedFolders[i].ID == "" {
			id, err := c.newFolderIDLocked()
			if err != nil {
				return "", true, err
			}
			c.TrustedFolders[i].ID = id
		}
		return c.TrustedFolders[i].ID, true, nil
	}
	return "", false, nil
}

// RemoveTrustedFolder drops dir from the list, returning the entry it removed
// along with whether it was there at all.
//
// The entry comes back so a caller whose removal fails to reach disk can restore
// exactly what it took out. Re-adding a freshly built entry instead re-pins the
// folder to whatever is at that path now, which turns a dead entry (folder
// deleted and replaced, granting nothing) into a live grant over the newcomer —
// the one thing the identity pin exists to prevent, arrived at through an error
// path.
func (c *Config) RemoveTrustedFolder(dir string) (TrustedFolder, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, w := range c.TrustedFolders {
		if w.Path == dir {
			c.TrustedFolders = append(c.TrustedFolders[:i], c.TrustedFolders[i+1:]...)
			return w, true
		}
	}
	return TrustedFolder{}, false
}

// SetTrustedIdentity re-pins an existing entry and returns the pin it replaced,
// so a caller whose save fails can put it back. ok reports whether there was an
// entry to re-pin. The pin is what makes a replaced folder stop granting;
// moving it is how an explicit re-approval of the folder now on disk takes
// effect.
func (c *Config) SetTrustedIdentity(dir, identity string) (previous string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, w := range c.TrustedFolders {
		if w.Path == dir {
			previous = w.Identity
			c.TrustedFolders[i].Identity = identity
			return previous, true
		}
	}
	return "", false
}

// SetTrustedOwnAddress records whether the entry at dir keeps its own address, and
// returns the value it replaced along with whether there was an entry to change, so
// a caller whose save fails can put it back.
func (c *Config) SetTrustedOwnAddress(dir string, own bool) (previous bool, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, w := range c.TrustedFolders {
		if w.Path == dir {
			c.TrustedFolders[i].OwnAddress = own
			return w.OwnAddress, true
		}
	}
	return false, false
}

// RelinkTrustedFolder points the entry with id at path, keeping its id, and pins it
// to identity, the folder's fingerprint where it is now. It answers the entry it
// replaced, so a caller whose save fails can put it back by relinking to the
// previous path and pin. ok reports whether there was an entry with that id.
func (c *Config) RelinkTrustedFolder(id, path, identity string) (previous TrustedFolder, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, w := range c.TrustedFolders {
		if w.ID != id {
			continue
		}
		previous = w
		c.TrustedFolders[i].Path = path
		c.TrustedFolders[i].Identity = identity
		return previous, true
	}
	return TrustedFolder{}, false
}

// TrustedFolderList returns a copy of the trusted folders so callers can read
// the list without touching the field under the lock.
func (c *Config) TrustedFolderList() []TrustedFolder {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TrustedFolder(nil), c.TrustedFolders...)
}

// ValidServedName reports whether name may name a served folder at all, before
// uniqueness: ErrServedNameInvalid for the wrong shape or a reserved name, else nil.
func ValidServedName(name string) error {
	if !servedNamePattern.MatchString(name) || reservedServedNames[name] {
		return ErrServedNameInvalid
	}
	return nil
}

// ServedFolderList returns a copy of the served folders so callers can read the
// list without touching the field under the lock.
func (c *Config) ServedFolderList() []ServedFolder {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ServedFolder(nil), c.ServedFolders...)
}

// ServedFolderByID returns the served folder with that id.
func (c *Config) ServedFolderByID(id string) (ServedFolder, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.ServedFolders {
		if f.ID == id {
			return f, true
		}
	}
	return ServedFolder{}, false
}

// ServedFolderByName returns the served folder served under that name.
func (c *Config) ServedFolderByName(name string) (ServedFolder, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.ServedFolders {
		if f.Name == name {
			return f, true
		}
	}
	return ServedFolder{}, false
}

// ServedFolderForPath returns the served folder rooted at path. path must already be
// canonical, as for trusted folders.
func (c *Config) ServedFolderForPath(path string) (ServedFolder, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range c.ServedFolders {
		if f.Path == path {
			return f, true
		}
	}
	return ServedFolder{}, false
}

// ServedNameAvailable reports whether name may be given to a served folder now:
// well formed, not in use, and never given out before.
func (c *Config) ServedNameAvailable(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.servedNameAvailableLocked(name)
}

func (c *Config) servedNameAvailableLocked(name string) error {
	if err := ValidServedName(name); err != nil {
		return err
	}
	for _, f := range c.ServedFolders {
		if f.Name == name {
			return ErrServedNameTaken
		}
	}
	for _, retired := range c.RetiredServedNames {
		if retired == name {
			return ErrServedNameRetired
		}
	}
	return nil
}

// AddServedFolder records path as served under name and, under the same lock, as a
// trusted folder: a served folder is always also trusted. path must already be
// canonical; the caller validates before adding.
func (c *Config) AddServedFolder(path, identity, name string) (ServedFolder, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.servedNameAvailableLocked(name); err != nil {
		return ServedFolder{}, err
	}
	for _, f := range c.ServedFolders {
		if f.Path == path {
			return ServedFolder{}, ErrAlreadyServed
		}
	}
	// Serving a folder does not give it a new identity: the id belongs to the
	// trusted entry and serving borrows it for as long as the folder is served.
	c.addTrustedFolderLocked(path, identity)
	id, found, err := c.trustedFolderIDLocked(path)
	if err != nil {
		return ServedFolder{}, fmt.Errorf("cannot generate served folder id: %w", err)
	}
	if !found {
		return ServedFolder{}, fmt.Errorf("cannot generate served folder id: no trusted entry for %s", path)
	}
	entry := ServedFolder{ID: id, Name: name, Path: path, Identity: identity, Serving: true}
	c.ServedFolders = append(c.ServedFolders, entry)
	return entry, nil
}

// SetServing turns serving on or off for a served folder, returning false if there
// is no entry with that id.
func (c *Config) SetServing(id string, serving bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, f := range c.ServedFolders {
		if f.ID == id {
			c.ServedFolders[i].Serving = serving
			return true
		}
	}
	return false
}

// SetServedAllowFraming turns whether other sites may frame a folder's answers on
// or off, returning false if there is no entry with that id.
func (c *Config) SetServedAllowFraming(id string, allow bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, f := range c.ServedFolders {
		if f.ID == id {
			c.ServedFolders[i].AllowFraming = allow
			return true
		}
	}
	return false
}

// RelinkServedFolder is RelinkTrustedFolder for a served entry: the path and the pin
// move, while the id, the name its address is built from and the serving state stay.
func (c *Config) RelinkServedFolder(id, path, identity string) (previous ServedFolder, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, f := range c.ServedFolders {
		if f.ID != id {
			continue
		}
		previous = f
		c.ServedFolders[i].Path = path
		c.ServedFolders[i].Identity = identity
		return previous, true
	}
	return ServedFolder{}, false
}

// RemoveServedFolder drops the entry and retires its name, so an old address never
// reaches a different folder. The trusted folder stays: stopping serving a folder
// is not the same as no longer trusting it.
func (c *Config) RemoveServedFolder(id string) (ServedFolder, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, f := range c.ServedFolders {
		if f.ID == id {
			c.ServedFolders = append(c.ServedFolders[:i], c.ServedFolders[i+1:]...)
			c.retireServedNameLocked(f.Name)
			return f, true
		}
	}
	return ServedFolder{}, false
}

// retireServedNameLocked appends name to RetiredServedNames once. Like RetiredSlugs
// the list is never trimmed: a name that left it could be handed out again.
func (c *Config) retireServedNameLocked(name string) {
	for _, retired := range c.RetiredServedNames {
		if retired == name {
			return
		}
	}
	c.RetiredServedNames = append(c.RetiredServedNames, name)
}

// promoteLegacyTrusted turns 1.2.0's read-only trusted folders into ordinary
// trusted folders, which now grant write. This is a deliberate widening: the
// concept the user agreed to is gone, and the closest surviving one is the
// full trust. Each promoted entry is pinned to the directory that is there now,
// since no pin was ever stored for it. Returns whether anything changed.
func (c *Config) promoteLegacyTrusted(identity func(string) string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.LegacyTrusted) == 0 {
		return false
	}
	existing := make(map[string]bool, len(c.TrustedFolders))
	for _, w := range c.TrustedFolders {
		existing[w.Path] = true
	}
	for _, dir := range c.LegacyTrusted {
		if existing[dir] {
			continue
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		c.TrustedFolders = append(c.TrustedFolders, TrustedFolder{Path: dir, Identity: identity(dir)})
		existing[dir] = true
	}
	c.LegacyTrusted = nil
	return true
}

// backfillIdentities pins entries that were stored without a fingerprint, and
// reports whether it pinned any.
//
// Windows had no directory fingerprint until DirIdentity learned to read the
// file id, so every trusted folder a Windows build recorded before that carries
// an empty pin. An empty pin means the path is the entry's whole identity
// (trust.IdentityOK), which keeps those folders working, and must: they are
// standing write grants the user made, and turning them dead on upgrade would
// silently revoke them. Pinning them to the directory that is there now is the
// same trade promoteLegacyTrusted makes for the same reason, and it is not a
// weakening: a folder already swapped before the upgrade was granting to the
// newcomer anyway, and after this any later swap is caught.
//
// A folder that is gone, or that stats as something other than a directory, is
// left alone. Its entry is the record of a grant and has to stay visible in the
// tray as dead rather than gain a pin for whatever now sits at its path.
//
// Nothing is written to disk here. Load reports the change through its Result and
// the caller saves, exactly as the legacy promotion's does; until that Save lands
// the pin is re-derived on every Load.
func (c *Config) backfillIdentities(identity func(string) string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	pinned := false
	for i, w := range c.TrustedFolders {
		if w.Identity != "" {
			continue
		}
		if info, err := os.Stat(w.Path); err != nil || !info.IsDir() {
			continue
		}
		id := identity(w.Path)
		if id == "" {
			continue
		}
		c.TrustedFolders[i].Identity = id
		pinned = true
	}
	return pinned
}

// dedupeTrustedFolders drops entries that name a directory an earlier entry
// already names, keeping the first and its pin.
//
// Until 1.4.0 the list compared paths byte for byte while trust.Canonical stored
// whatever capitalization the caller supplied, so a page could add a second entry
// for one folder by asking for an asset under a different casing, and untrusting
// the spelling the user recognized left the other one granting write. Canonical
// now stores the filesystem's own spelling, which stops new duplicates; this
// clears out the ones already written to disk.
//
// Sameness is os.SameFile, never a case-folded string compare. session.EqualOrUnder
// follows the home volume's rule, and its own comment warns that a volume with
// different folding mounted inside home is misjudged: folding here would merge two
// genuinely distinct directories and hand one folder's pin to the other. A dead
// entry stats nothing, so it matches only its own byte-exact path and survives,
// which is what keeps it visible in the tray as dead.
func (c *Config) dedupeTrustedFolders() {
	c.mu.Lock()
	defer c.mu.Unlock()
	type seen struct {
		path string
		info os.FileInfo
	}
	var keptInfo []seen
	kept := make([]TrustedFolder, 0, len(c.TrustedFolders))
	for _, w := range c.TrustedFolders {
		info, err := os.Stat(w.Path)
		if err != nil {
			info = nil
		}
		duplicate := false
		for _, s := range keptInfo {
			if s.path == w.Path || (info != nil && s.info != nil && os.SameFile(info, s.info)) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		kept = append(kept, w)
		keptInfo = append(keptInfo, seen{path: w.Path, info: info})
	}
	c.TrustedFolders = kept
}

// sanitizeServedFolders drops served entries that could not have been written by
// AddServedFolder and duplicates of entries an earlier one already holds, first
// wins. Every kept entry's path becomes a trusted folder if it is not one already,
// because a served folder is always trusted. It reports nothing, the way the
// trusted-folder dedupe next to it reports nothing: the caller saves the config on
// its own schedule and the cleanup is re-derived on every load.
func (c *Config) sanitizeServedFolders() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var kept []ServedFolder
	ids := make(map[string]bool, len(c.ServedFolders))
	names := make(map[string]bool, len(c.ServedFolders))
	paths := make(map[string]bool, len(c.ServedFolders))
	for _, f := range c.ServedFolders {
		if !servedIDPattern.MatchString(f.ID) || ValidServedName(f.Name) != nil || f.Path == "" {
			continue
		}
		if ids[f.ID] || names[f.Name] || paths[f.Path] {
			continue
		}
		ids[f.ID] = true
		names[f.Name] = true
		paths[f.Path] = true
		kept = append(kept, f)
		c.addTrustedFolderLocked(f.Path, f.Identity)
	}
	c.ServedFolders = kept
	var retired []string
	retiredNames := make(map[string]bool, len(c.RetiredServedNames))
	for _, name := range c.RetiredServedNames {
		if ValidServedName(name) != nil || retiredNames[name] {
			continue
		}
		retiredNames[name] = true
		retired = append(retired, name)
	}
	c.RetiredServedNames = retired
}

// assignFolderIDs settles the id every trusted folder keeps. A served entry's id
// is the folder's id, so it is copied onto the trusted entry at the same path; an
// entry without one, or with an id that could not have been made by this build,
// gets a fresh id. Nothing here reuses another folder's id, so a folder removed and
// trusted again at the same path comes back as a new folder with a new id. It
// reports whether anything changed, and writes nothing: Load reports the change
// through its Result and the caller saves once.
func (c *Config) assignFolderIDs() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := false
	for i := range c.TrustedFolders {
		path := c.TrustedFolders[i].Path
		servedID := ""
		for _, f := range c.ServedFolders {
			if f.Path == path {
				servedID = f.ID
				break
			}
		}
		switch {
		case servedID != "" && servedID != c.TrustedFolders[i].ID:
			c.TrustedFolders[i].ID = servedID
			changed = true
		case servedID == "" && !servedIDPattern.MatchString(c.TrustedFolders[i].ID):
			id, err := c.newFolderIDLocked()
			if err != nil {
				continue
			}
			c.TrustedFolders[i].ID = id
			changed = true
		}
	}
	return changed
}

// SitePort returns the port previously used for an anchor, or 0 if there is none.
func (c *Config) SitePort(anchor string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SitePorts[anchor]
}

// RememberSitePort records the port an origin was served on so the next launch
// can reuse it and keep the origin stable.
func (c *Config) RememberSitePort(anchor string, port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SitePorts == nil {
		c.SitePorts = make(map[string]int)
	}
	c.SitePorts[anchor] = port
}

// ForgetSitePort drops the remembered port for anchor and returns it, so a
// caller whose change fails to reach disk can put it back.
//
// Untrusting a folder forgets its port. Keeping it would hand the folder's exact
// origin straight back to the first file re-homed out of it: that file's own
// folder IS the untrusted folder, so it anchors there and binds the same
// remembered port, leaving the untrusted folder's still-open pages same-origin
// with a file that has a live save token. "Files you had opened yourself
// survive, on a new address of their own" is the promise, and the new address is
// the whole of it.
func (c *Config) ForgetSitePort(anchor string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	port := c.SitePorts[anchor]
	delete(c.SitePorts, anchor)
	return port
}

// SitePortList returns a copy of the remembered ports for startup planning.
func (c *Config) SitePortList() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.SitePorts))
	for k, v := range c.SitePorts {
		out[k] = v
	}
	return out
}

// siteSlugCap bounds the slug map the way sitePortCap bounds the legacy one; a slug
// costs no socket, so the cap is only about config size.
const siteSlugCap = 64

// Port returns the remembered port, 0 when none has been chosen.
func (c *Config) Port() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ListenPort
}

// SetPort records the port the app is serving on.
func (c *Config) SetPort(port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ListenPort = port
}

// SiteSlug returns the slug remembered for key, or "" if there is none.
func (c *Config) SiteSlug(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SiteSlugs[key]
}

// SiteKeyForSlug is the reverse lookup the router needs for a hostname with no live site.
// A retired slug has no key.
func (c *Config) SiteKeyForSlug(slug string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, s := range c.SiteSlugs {
		if s == slug {
			return key, true
		}
	}
	return "", false
}

// SlugInUse reports whether slug is issued or retired, so an allocator never reissues it.
func (c *Config) SlugInUse(slug string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.slugInUseLocked(slug)
}

func (c *Config) slugInUseLocked(slug string) bool {
	for _, s := range c.SiteSlugs {
		if s == slug {
			return true
		}
	}
	for _, s := range c.RetiredSlugs {
		if s == slug {
			return true
		}
	}
	return false
}

// AllocateSiteSlug records and returns key's slug: the next label in the htmlclay1, htmlclay2,
// ... sequence, skipping any label already issued or retired. The counter only grows.
func (c *Config) AllocateSiteSlug(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.allocateSiteSlugLocked(key)
}

// allocateSiteSlugLocked is AllocateSiteSlug for a caller holding c.mu. It starts at 2,
// so it never issues firstSiteLabel.
func (c *Config) allocateSiteSlugLocked(key string) string {
	n := max(c.NextSiteNumber, 2)
	for {
		slug := numberedSlugFor(n)
		n++
		if c.slugInUseLocked(slug) {
			continue
		}
		if c.SiteSlugs == nil {
			c.SiteSlugs = make(map[string]string)
		}
		c.SiteSlugs[key] = slug
		c.NextSiteNumber = n
		return slug
	}
}

// RememberSiteSlug records key's slug. The caller has checked SlugInUse.
func (c *Config) RememberSiteSlug(key, slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SiteSlugs == nil {
		c.SiteSlugs = make(map[string]string)
	}
	c.SiteSlugs[key] = slug
}

// ForgetSiteSlug drops key's slug, retires it, and returns it so a caller whose change
// fails to reach disk can RestoreSiteSlug. Untrusting a folder forgets its slug for the
// reason ForgetSitePort gives: the re-homed survivors must land on a new origin.
func (c *Config) ForgetSiteSlug(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	slug := c.SiteSlugs[key]
	delete(c.SiteSlugs, key)
	if slug != "" {
		c.retireLocked(slug)
	}
	return slug
}

// DropSiteSlug removes key's slug without retiring it. It is for a label that was
// allocated and never served: no page ever ran on it, so it has no storage to protect.
func (c *Config) DropSiteSlug(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.SiteSlugs, key)
}

// RestoreSiteSlug undoes ForgetSiteSlug after a failed save.
func (c *Config) RestoreSiteSlug(key, slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.SiteSlugs == nil {
		c.SiteSlugs = make(map[string]string)
	}
	c.SiteSlugs[key] = slug
	kept := c.RetiredSlugs[:0]
	for _, s := range c.RetiredSlugs {
		if s != slug {
			kept = append(kept, s)
		}
	}
	c.RetiredSlugs = kept
}

// MovedFirstSiteKey returns the origin key moveFirstSite moved off htmlclay, or "".
func (c *Config) MovedFirstSiteKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.MovedFirstSite
}

// CleanMark returns the mark markFirstSiteOrigin made, or "" for a config that
// never served a site at the dashboard's address.
func (c *Config) CleanMark() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.DashboardCleanMark
}

// SiteSlugList returns a copy of the slug map.
func (c *Config) SiteSlugList() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.SiteSlugs))
	for k, v := range c.SiteSlugs {
		out[k] = v
	}
	return out
}

// retireLocked appends slug to RetiredSlugs once. The list is never trimmed: a label that
// left it could be issued again, and the folder that got it would inherit the old origin's
// browser storage.
func (c *Config) retireLocked(slug string) {
	for _, s := range c.RetiredSlugs {
		if s == slug {
			return
		}
	}
	c.RetiredSlugs = append(c.RetiredSlugs, slug)
}

var numberedSlug = regexp.MustCompile(`^htmlclay([1-9][0-9]*)?$`)

// firstSiteLabel is the label counter value 1 stood for. Its address, htmlclay.localhost,
// is the dashboard's, so no site is served under it.
const firstSiteLabel = "htmlclay"

// numberedSlugFor maps counter value n (from 1) to its label: htmlclay, htmlclay1, htmlclay2, ...
func numberedSlugFor(n int) string {
	if n == 1 {
		return firstSiteLabel
	}
	return firstSiteLabel + strconv.Itoa(n-1)
}

// moveFirstSite moves a config written while the first site answered at htmlclay.localhost:
// the key served under that label gets the next free numbered one, and the key is recorded
// so the dashboard's host can send an old link to it. It reports whether a key moved.
func (c *Config) moveFirstSite() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, slug := range c.SiteSlugs {
		if slug == firstSiteLabel {
			c.allocateSiteSlugLocked(key)
			c.MovedFirstSite = key
			return true
		}
	}
	return false
}

// markFirstSiteOrigin gives a config its DashboardCleanMark when a site was ever served
// under htmlclay: one that moved off it, or one whose label was retired while it was
// still issued. No build issues that label now, so no other config needs a mark. It
// reports whether it made one.
func (c *Config) markFirstSiteOrigin() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.DashboardCleanMark != "" || (c.MovedFirstSite == "" && !slices.Contains(c.RetiredSlugs, firstSiteLabel)) {
		return false
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return false
	}
	c.DashboardCleanMark = hex.EncodeToString(raw[:])
	return true
}

// retireNamedSlugs moves a config from the earlier <folder>-<4 hex> labels to numbered
// ones: each label of the old shape is retired, and its key gets the next number when its
// site is next built.
func (c *Config) retireNamedSlugs() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, slug := range c.SiteSlugs {
		if !numberedSlug.MatchString(slug) {
			c.retireLocked(slug)
			delete(c.SiteSlugs, key)
		}
	}
}

// dedupeSiteSlugs keeps one key per slug value. A trusted folder's entry wins, then the
// lexically first key; the others are deleted, not retired, because the label is still in
// use, and they get a fresh slug the next time they are opened. Two keys on one label would
// put two sites on one hostname.
func (c *Config) dedupeSiteSlugs() {
	c.mu.Lock()
	defer c.mu.Unlock()
	trusted := make(map[string]bool, len(c.TrustedFolders))
	for _, w := range c.TrustedFolders {
		trusted[w.Path] = true
	}
	keys := make([]string, 0, len(c.SiteSlugs))
	for key := range c.SiteSlugs {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if trusted[keys[i]] != trusted[keys[j]] {
			return trusted[keys[i]]
		}
		return keys[i] < keys[j]
	})
	owner := make(map[string]bool, len(keys))
	for _, key := range keys {
		slug := c.SiteSlugs[key]
		if owner[slug] {
			delete(c.SiteSlugs, key)
			continue
		}
		owner[slug] = true
	}
}

// capSiteSlugs enforces siteSlugCap with capSitePorts's rules: trusted folders are never
// evicted, and among the rest, keys that no longer stat go first. A key may be a file
// (a loose file in home), so the stat is for existence, not for a directory. Evicted
// slugs are retired.
func (c *Config) capSiteSlugs() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.SiteSlugs) <= siteSlugCap {
		return
	}
	protected := make(map[string]bool, len(c.TrustedFolders))
	for _, w := range c.TrustedFolders {
		protected[w.Path] = true
	}
	var missing, present []string
	for key := range c.SiteSlugs {
		if protected[key] {
			continue
		}
		if _, err := os.Stat(key); err != nil {
			missing = append(missing, key)
		} else {
			present = append(present, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(present)
	for _, key := range append(missing, present...) {
		if len(c.SiteSlugs) <= siteSlugCap {
			return
		}
		c.retireLocked(c.SiteSlugs[key])
		delete(c.SiteSlugs, key)
	}
}

// capSitePorts enforces sitePortCap. See the constant for why eviction is
// bounded to the over-cap case rather than run on every load.
func (c *Config) capSitePorts() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.SitePorts) <= sitePortCap {
		return
	}
	protected := make(map[string]bool, len(c.TrustedFolders))
	for _, w := range c.TrustedFolders {
		protected[w.Path] = true
	}
	var missing, present []string
	for root := range c.SitePorts {
		if protected[root] {
			continue
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			missing = append(missing, root)
		} else {
			present = append(present, root)
		}
	}
	sort.Strings(missing)
	sort.Strings(present)
	for _, root := range append(missing, present...) {
		if len(c.SitePorts) <= sitePortCap {
			return
		}
		delete(c.SitePorts, root)
	}
}

// StartOnLoginEnabled reports the start-on-login preference under the lock.
func (c *Config) StartOnLoginEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.StartOnLogin
}

// SetStartOnLogin sets the start-on-login preference under the lock.
func (c *Config) SetStartOnLogin(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.StartOnLogin = v
}

// SelfUpdateOff reports, under the lock, whether config turned the self-updater off.
func (c *Config) SelfUpdateOff() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Updates == "off"
}

// SetSelfUpdateOff records, under the lock, whether updates are off. Turning
// them back on clears the setting, so the file reads as it did before anyone
// chose.
func (c *Config) SetSelfUpdateOff(off bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if off {
		c.Updates = "off"
	} else {
		c.Updates = ""
	}
}

// AIEditEnabled reports whether AI editing is on, under the lock. A missing setting
// reads as on; only an explicit false turns it off.
func (c *Config) AIEditEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.AIEdit == nil || c.AIEdit.Enabled == nil || *c.AIEdit.Enabled
}

// SetAIEditEnabled records the AI editing switch under the lock, keeping the
// default engine and user engines.
func (c *Config) SetAIEditEnabled(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AIEdit == nil {
		c.AIEdit = &AIEditSettings{}
	}
	c.AIEdit.Enabled = &v
}

// Appearance reports the chosen appearance under the lock. Empty, or anything the
// app does not name, reads as system.
func (c *Config) Appearance() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AppearanceSetting == "" {
		return "system"
	}
	return c.AppearanceSetting
}

// SetAppearance records the chosen appearance under the lock.
func (c *Config) SetAppearance(v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AppearanceSetting = v
}

// AIEditEngines returns the default engine name and a copy of the user engines,
// under the lock. The copy is the caller's to keep.
func (c *Config) AIEditEngines() (string, map[string][]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AIEdit == nil {
		return "", nil
	}
	engines := make(map[string][]string, len(c.AIEdit.Engines))
	for name, argv := range c.AIEdit.Engines {
		engines[name] = append([]string(nil), argv...)
	}
	return c.AIEdit.Default, engines
}

// Profile is the app-wide person for files opened with HTML Clay: a random id and a
// display name, shared with documents only while Enabled.
type Profile struct {
	Enabled bool   `json:"enabled"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
}

var (
	profileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	emailPattern     = regexp.MustCompile(`\S+@\S+\.\S+`)
)

// The same rules ClayJS applies to a host's person, so a name the app accepts is
// never silently replaced by the browser's own prompt.
const maxProfileName = 120

var (
	ErrProfileNameEmpty   = errors.New("Enter a name.")
	ErrProfileNameLong    = errors.New("Use a name of 120 characters or fewer.")
	ErrProfileNameEmail   = errors.New("Use a name, not an email address.")
	ErrProfileIncomplete  = errors.New("A profile needs an id and a name before it can be used.")
	ErrProfileIDMalformed = errors.New("The profile id is damaged.")
)

// isJSSpace is JavaScript's whitespace set (String.prototype.trim and \s), so a name
// HTML Clay accepts is one ClayJS accepts too.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ',
		0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// CleanProfileName trims and collapses whitespace and checks the result.
func CleanProfileName(name string) (string, error) {
	clean := strings.Join(strings.FieldsFunc(name, isJSSpace), " ")
	switch {
	case clean == "":
		return "", ErrProfileNameEmpty
	case len(utf16.Encode([]rune(clean))) > maxProfileName:
		return "", ErrProfileNameLong
	case emailPattern.MatchString(clean):
		return "", ErrProfileNameEmail
	}
	return clean, nil
}

// ValidProfileID reports whether id has the shape ClayJS accepts for a person.
func ValidProfileID(id string) bool {
	return profileIDPattern.MatchString(id)
}

// NewProfileID mints a person id: 16 random bytes as unpadded base64url.
func NewProfileID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Shareable reports whether this profile may be given to documents: on, with an id
// and a name that pass the same checks the setter applies.
func (p Profile) Shareable() bool {
	if !p.Enabled || !profileIDPattern.MatchString(p.ID) {
		return false
	}
	name, err := CleanProfileName(p.Name)
	return err == nil && name == p.Name
}

// ProfileState returns a copy of the profile under the lock; the zero value when
// none was ever set up.
func (c *Config) ProfileState() Profile {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Profile == nil {
		return Profile{}
	}
	return *c.Profile
}

// SharedPerson is the person documents see: ok only while sharing is on and the
// stored record passes the same checks the setter applies.
func (c *Config) SharedPerson() (id, name string, ok bool) {
	p := c.ProfileState()
	if !p.Shareable() {
		return "", "", false
	}
	return p.ID, p.Name, true
}

// UpdateProfile validates next, writes it to disk, and only then lets readers see it. A
// field the change leaves as it is stored is not re-checked, so a damaged record can
// always be turned off. Turning sharing on needs a sound id and name. A failed write
// leaves the previous profile in memory and on disk.
func (c *Config) UpdateProfile(next Profile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var current Profile
	if c.Profile != nil {
		current = *c.Profile
	}
	if next.Name != current.Name || (next.Enabled && !current.Enabled) {
		if next.Name != "" || next.Enabled {
			name, err := CleanProfileName(next.Name)
			if err != nil {
				return err
			}
			next.Name = name
		}
	}
	if (next.ID != current.ID || (next.Enabled && !current.Enabled)) && next.ID != "" && !ValidProfileID(next.ID) {
		return ErrProfileIDMalformed
	}
	if next.Enabled && (next.ID == "" || next.Name == "") {
		return ErrProfileIncomplete
	}
	previous := c.Profile
	c.Profile = &next
	if err := c.saveLocked(); err != nil {
		c.Profile = previous
		return err
	}
	return nil
}

func defaultConfigDir() (string, error) {
	// HTMLCLAY_CONFIG_BASE replaces the OS config folder, so tests and seeded profiles never touch the real config. The app's own folder is still <base>/htmlclay.
	if base := os.Getenv("HTMLCLAY_CONFIG_BASE"); base != "" {
		return filepath.Abs(base)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine config directory: %w", err)
	}
	return base, nil
}

func Dir() (string, error) {
	base, err := defaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "htmlclay"), nil
}

func DirFrom(baseDir string) string {
	return filepath.Join(baseDir, "htmlclay")
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func EnsureDir() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// Result reports what Load had to do to the file, so startup can act on a
// one-time migration without a live field for a setting that no longer exists.
type Result struct {
	// HadAppMode is true when the loaded file still carried App Mode. Startup
	// uses it to delete the Chromium profile directory that mode created.
	HadAppMode bool
	// PromotedLegacy is true when 1.2.0 read-only trusted folders were widened
	// into trusted folders.
	PromotedLegacy bool
	// PinnedIdentities is true when trusted folders that carried no fingerprint
	// gained one, which is what a config written by a Windows build from before
	// DirIdentity answered there looks like.
	PinnedIdentities bool
	// AssignedFolderIDs is true when trusted folders that had no id gained the
	// one they keep, which is what every config written before ids existed, and
	// every config whose served folders predate them, looks like.
	AssignedFolderIDs bool
	// MovedFirstSite is true when the site served under htmlclay moved to a
	// numbered label, which is what a config written before htmlclay.localhost
	// became the dashboard's address looks like.
	MovedFirstSite bool
	// MarkedFirstSiteOrigin is true when the config gained its DashboardCleanMark:
	// with the move above, or on the first load by a build that clears the
	// dashboard's address.
	MarkedFirstSiteOrigin bool
	// DroppedHelperPrograms and DroppedHelperDecisions report malformed,
	// duplicate, dangling, or over-cap helper records removed during Load.
	DroppedHelperPrograms  int
	DroppedHelperDecisions int
}

func Load(identity func(string) string) (*Config, Result, error) {
	base, err := defaultConfigDir()
	if err != nil {
		return nil, Result{}, err
	}
	return LoadFrom(base, identity)
}

func LoadFrom(baseDir string, identity func(string) string) (*Config, Result, error) {
	cfg := &Config{StartOnLogin: false, baseDir: baseDir}
	var res Result

	path := filepath.Join(DirFrom(baseDir), "config.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, res, nil
	}
	if err != nil {
		return nil, res, err
	}

	if uErr := json.Unmarshal(data, cfg); uErr != nil {
		// A corrupt config must not brick startup, and it must not silently
		// erase what the user granted either. Falling back to defaults and
		// letting the next Save overwrite the file is how a single mis-shaped
		// field used to take the whole trusted-folder list with it. Move the bad
		// file aside first, so it is recoverable and so the user can be told.
		quarantine := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
		if rErr := os.Rename(path, quarantine); rErr != nil {
			fmt.Fprintf(os.Stderr, "[htmlclay] config.json is corrupt (%v) and could not be set aside (%v), using defaults\n", uErr, rErr)
		} else {
			fmt.Fprintf(os.Stderr, "[htmlclay] config.json is corrupt (%v); saved it as %s and starting from defaults\n", uErr, quarantine)
		}
		return &Config{StartOnLogin: false, baseDir: baseDir}, res, nil
	}

	// "mode" is App Mode's only trace once the field is gone. Read it straight
	// from the bytes rather than keeping a live setting for a feature that no
	// longer exists; the next Save drops the key, which completes the migration.
	var legacy struct {
		Mode string `json:"mode"`
	}
	if json.Unmarshal(data, &legacy) == nil && legacy.Mode == "app" {
		res.HadAppMode = true
	}

	res.PromotedLegacy = cfg.promoteLegacyTrusted(identity)
	// After the promotion, which appends straight to the list with a byte-exact
	// dedupe of its own and so can add an alias of an entry already there.
	cfg.dedupeTrustedFolders()
	// After the dedupe, which settles the trusted list, and before the pin backfill,
	// so a trusted folder added here for a served entry without a pin gets one on
	// this load rather than the next.
	cfg.sanitizeServedFolders()
	// After the sanitize, which is what makes every served entry also a trusted
	// one, so a served folder's id lands on its trusted entry on this load.
	res.AssignedFolderIDs = cfg.assignFolderIDs()
	// After the dedupe, so a pin is never derived for an entry that is about to
	// be dropped as an alias of one already kept.
	res.PinnedIdentities = cfg.backfillIdentities(identity)
	res.DroppedHelperPrograms, res.DroppedHelperDecisions = cfg.normalizeHelpers()
	cfg.normalizeAIEditDocuments()
	cfg.capSitePorts()
	cfg.retireNamedSlugs()
	cfg.dedupeSiteSlugs()
	cfg.capSiteSlugs()
	// After the dedupe and the cap, so the key that moves is the one that keeps a label.
	res.MovedFirstSite = cfg.moveFirstSite()
	res.MarkedFirstSiteOrigin = cfg.markFirstSiteOrigin()
	return cfg, res, nil
}

func (c *Config) Save() error {
	// Hold the lock across the marshal so a concurrent SitePorts/TrustedFolders
	// mutation cannot tear the snapshot or panic the map iteration. The disk write
	// stays under the lock too, which serialises Saves and prevents two writers'
	// temp-rename races from resurrecting a just-removed entry.
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

// HoldSaves keeps every later write in memory until ReleaseSaves: a version on
// trial must not change the file the version it replaced reads after a rollback.
func (c *Config) HoldSaves() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.holdSaves = true
}

// ReleaseSaves ends a hold. With write, a save asked for during the hold happens
// once now; without, the held changes stay in memory only.
func (c *Config) ReleaseSaves(write bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	owed := c.saveOwed
	c.holdSaves, c.saveOwed = false, false
	if !write || !owed {
		return nil
	}
	return c.saveLocked()
}

// saveLocked writes the config. The caller holds c.mu.
func (c *Config) saveLocked() error {
	if c.holdSaves {
		c.saveOwed = true
		return nil
	}
	dir := DirFrom(c.baseDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	c.writes++
	return nil
}

// Writes is how many times this Config has reached the disk since it was loaded.
func (c *Config) Writes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}
