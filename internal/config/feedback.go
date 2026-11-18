package config

import (
	"errors"
	"slices"
	"strings"
)

const (
	formerPortCap      = 4
	builtinDocumentCap = 512
	maxModelName       = 100
)

// BuiltinProgram is one built-in program's switch, the documents the person
// allowed to run it, whether every document in a trusted folder of this
// computer's own may run it, and the documents whose access was removed (they
// are refused without asking until Allow again). Paths are absolute, as the
// wire names them.
type BuiltinProgram struct {
	On         bool     `json:"on"`
	AllTrusted bool     `json:"allTrusted,omitempty"`
	Documents  []string `json:"documents,omitempty"`
	Removed    []string `json:"removed,omitempty"`
}

// MovePort records the port the app moves to. The port it leaves joins
// FormerPorts, and the new port leaves that list if it was in it.
func (c *Config) MovePort(to int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	from := c.ListenPort
	kept := make([]int, 0, len(c.FormerPorts)+1)
	for _, p := range c.FormerPorts {
		if p != to && p != from {
			kept = append(kept, p)
		}
	}
	if from != 0 && from != to {
		kept = append(kept, from)
	}
	if len(kept) > formerPortCap {
		kept = kept[len(kept)-formerPortCap:]
	}
	c.FormerPorts = kept
	c.ListenPort = to
}

// RestorePort puts back a port and former ports MovePort replaced, after a save failed.
func (c *Config) RestorePort(port int, former []int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ListenPort = port
	c.FormerPorts = slices.Clone(former)
}

// FormerPortList returns a copy of FormerPorts, oldest first.
func (c *Config) FormerPortList() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.FormerPorts)
}

// ForgetFormerPort drops a former port that something else holds now.
func (c *Config) ForgetFormerPort(port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FormerPorts = slices.DeleteFunc(c.FormerPorts, func(p int) bool { return p == port })
}

// BuiltinOn reports whether a built-in program is switched on.
func (c *Config) BuiltinOn(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	return p != nil && p.On
}

// SetBuiltinOn switches a built-in program on or off. The allowed documents stay.
func (c *Config) SetBuiltinOn(id string, on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.builtinLocked(id).On = on
}

// BuiltinAllowed reports whether the person allowed document to run program.
func (c *Config) BuiltinAllowed(id, document string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	return p != nil && slices.Contains(p.Documents, document)
}

// AllowBuiltin records the person's allow. A full list drops its oldest
// document, which then asks again.
func (c *Config) AllowBuiltin(id, document string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.builtinLocked(id)
	if slices.Contains(p.Documents, document) {
		return
	}
	if len(p.Documents) >= builtinDocumentCap {
		p.Documents = p.Documents[1:]
	}
	p.Documents = append(p.Documents, document)
	p.Removed = slices.DeleteFunc(p.Removed, func(d string) bool { return d == document })
}

// ForgetBuiltin takes a program away from one document, or from every document
// when document is "", and lists them as removed. Removing every document also
// turns off All documents in my trusted folders. It returns the documents it
// removed.
func (c *Config) ForgetBuiltin(id, document string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	if p == nil {
		return nil
	}
	var removed, kept []string
	for _, d := range p.Documents {
		if document == "" || d == document {
			removed = append(removed, d)
		} else {
			kept = append(kept, d)
		}
	}
	p.Documents = kept
	if document == "" {
		p.AllTrusted = false
	}
	for _, d := range removed {
		if !slices.Contains(p.Removed, d) {
			p.Removed = append(p.Removed, d)
		}
	}
	if len(p.Removed) > builtinDocumentCap {
		p.Removed = p.Removed[len(p.Removed)-builtinDocumentCap:]
	}
	return removed
}

// BuiltinRemoved reports whether the person removed document's access to program.
func (c *Config) BuiltinRemoved(id, document string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	return p != nil && slices.Contains(p.Removed, document)
}

// AskBuiltinAgain lets a removed document ask again: one document, or every
// removed document when document is "".
func (c *Config) AskBuiltinAgain(id, document string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	if p == nil {
		return
	}
	p.Removed = slices.DeleteFunc(p.Removed, func(d string) bool { return document == "" || d == document })
}

// SetBuiltinAllTrusted records All documents in my trusted folders for program.
func (c *Config) SetBuiltinAllTrusted(id string, on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.builtinLocked(id).AllTrusted = on
}

// BuiltinAllTrusted reports All documents in my trusted folders for program.
func (c *Config) BuiltinAllTrusted(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	return p != nil && p.AllTrusted
}

// BuiltinRemovedDocuments returns a sorted copy of the documents whose access was removed.
func (c *Config) BuiltinRemovedDocuments(id string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	if p := c.BuiltinPrograms[id]; p != nil {
		out = append(out, p.Removed...)
	}
	slices.Sort(out)
	return out
}

// BuiltinDocuments returns a sorted copy of the documents allowed to run program.
func (c *Config) BuiltinDocuments(id string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.BuiltinPrograms[id]
	if p == nil {
		return []string{}
	}
	out := slices.Clone(p.Documents)
	slices.Sort(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func (c *Config) builtinLocked(id string) *BuiltinProgram {
	if c.BuiltinPrograms == nil {
		c.BuiltinPrograms = make(map[string]*BuiltinProgram)
	}
	p := c.BuiltinPrograms[id]
	if p == nil {
		p = &BuiltinProgram{}
		c.BuiltinPrograms[id] = p
	}
	return p
}

// ErrModelName refuses a model name that is too long or holds a control
// character or a space.
var ErrModelName = errors.New("a model name is one word of at most 100 characters")

// AIEditModels returns a copy of the model chosen for each engine.
func (c *Config) AIEditModels() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]string{}
	if c.AIEdit != nil {
		for k, v := range c.AIEdit.Models {
			out[k] = v
		}
	}
	return out
}

// SetAIEditDefault makes engine the one AI editing uses when a request names none.
func (c *Config) SetAIEditDefault(engine string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AIEdit == nil {
		c.AIEdit = &AIEditSettings{}
	}
	c.AIEdit.Default = engine
}

// SetAIEditModel records the model an engine runs with. "" goes back to the
// engine's own default.
func (c *Config) SetAIEditModel(engine, model string) error {
	model = strings.TrimSpace(model)
	if len(model) > maxModelName || strings.ContainsFunc(model, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return ErrModelName
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AIEdit == nil {
		c.AIEdit = &AIEditSettings{}
	}
	if c.AIEdit.Models == nil {
		c.AIEdit.Models = make(map[string]string)
	}
	if model == "" {
		delete(c.AIEdit.Models, engine)
	} else {
		c.AIEdit.Models[engine] = model
	}
	return nil
}

// SetAIEditEngine records a user engine's command and returns the command it
// replaced, nil when there was none.
func (c *Config) SetAIEditEngine(name string, argv []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AIEdit == nil {
		c.AIEdit = &AIEditSettings{}
	}
	if c.AIEdit.Engines == nil {
		c.AIEdit.Engines = make(map[string][]string)
	}
	previous := c.AIEdit.Engines[name]
	c.AIEdit.Engines[name] = slices.Clone(argv)
	return previous
}

// SetStopOpeningFiles records whether the person stopped HTML Clay opening .htmlclay files.
func (c *Config) SetStopOpeningFiles(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.StopOpeningFiles = on
}

// StoppedOpeningFiles reports StopOpeningFiles under the lock.
func (c *Config) StoppedOpeningFiles() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.StopOpeningFiles
}

// SetHTMLOpenWith records whether HTML Clay is in Open With for .html files.
func (c *Config) SetHTMLOpenWith(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HTMLOpenWithOff = !on
}

// HTMLOpenWith reports whether HTML Clay should be in Open With for .html files.
func (c *Config) HTMLOpenWith() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.HTMLOpenWithOff
}
