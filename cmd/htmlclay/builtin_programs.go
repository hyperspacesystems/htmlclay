package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
)

const (
	builtinDialogTitle   = "Let %s use %s?"
	builtinDialogMessage = "Allowing applies to this document only, and it can only work with files inside its own folder. After allowing, go back to the page and send your request again."
	sampleImageName      = "sample-photo.png"
)

// builtinFor is both hosts' answer to whether document may run a built-in
// program now. The program's switch decides first; then the document must be
// in a trusted or served folder; a removed document is refused without asking;
// an allowed document runs; a document in a trusted folder of this computer's
// own runs when All documents in my trusted folders is on; anything else asks.
func (a *app) builtinFor(document, program string) builtin.Access {
	if !builtin.Is(program) || !a.rt.cfg.BuiltinOn(program) {
		return builtin.Access{State: builtin.Off}
	}
	owner, err := a.ResolveDocument(document)
	if err != nil || (owner.Kind != OwnerTrusted && owner.Kind != OwnerServed) {
		return builtin.Access{State: builtin.Untrusted}
	}
	if a.rt.cfg.BuiltinRemoved(program, document) {
		return builtin.Access{State: builtin.Removed}
	}
	ready := builtin.Access{State: builtin.Ready, Root: owner.FolderPath}
	if a.rt.cfg.BuiltinAllowed(program, document) {
		return ready
	}
	if owner.Kind == OwnerTrusted && a.rt.cfg.BuiltinAllTrusted(program) && a.scopeOf(document).Kind == config.ScopeLocal {
		return ready
	}
	return builtin.Access{State: builtin.Ask}
}

// askBuiltin raises the question for document and program and returns at once.
// One question per document and program is open at a time, and after one that
// allowed nothing the pair stays quiet for a minute.
func (a *app) askBuiltin(document, program string) {
	key := program + "\x00" + document
	a.helperStateMu.Lock()
	if a.builtinAsking[key] || time.Now().Before(a.builtinQuiet[key]) {
		a.helperStateMu.Unlock()
		return
	}
	if a.builtinAsking == nil {
		a.builtinAsking = make(map[string]bool)
	}
	a.builtinAsking[key] = true
	a.helperStateMu.Unlock()
	go a.runBuiltinAsk(key, document, program)
}

func (a *app) runBuiltinAsk(key, document, program string) {
	allowed := false
	defer func() {
		a.helperStateMu.Lock()
		delete(a.builtinAsking, key)
		if allowed {
			delete(a.builtinQuiet, key)
		} else {
			if a.builtinQuiet == nil {
				a.builtinQuiet = make(map[string]time.Time)
			}
			a.builtinQuiet[key] = time.Now().Add(pageAskQuietFor)
		}
		a.helperStateMu.Unlock()
	}()
	owner, _ := a.ResolveDocument(document)
	presenter := a.presenter(builtinDialog{app: a})
	reply, err := presenter.Ask(context.Background(), confirm.Ask{
		Kind:     appcore.RequestBuiltinProgram,
		Document: document,
		Program:  program,
		Folder:   filepath.Base(owner.FolderPath),
		Identity: platform.DirIdentity(document),
		Answers:  []string{helperAnswerDeny, helperAnswerAllowDocument},
	})
	if err != nil {
		a.rt.logger.Printf("Could not ask about %s for %s: %v", program, document, err)
		return
	}
	defer presenter.Close(reply.ID)
	if reply.Choice != helperAnswerAllowDocument {
		return
	}
	a.rt.cfg.AllowBuiltin(program, document)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.ForgetBuiltin(program, document)
		a.rt.cfg.AskBuiltinAgain(program, document)
		a.rt.logger.Printf("Could not save the %s decision for %s: %v", program, document, err)
		return
	}
	allowed = true
	a.rt.logger.Printf("Built-in program allowed: program=%s document=%s", program, document)
	a.changed()
}

// builtinDialog asks with today's two-button dialog where no Mac app answers.
type builtinDialog struct{ app *app }

func (d builtinDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	show := d.app.rt.confirm
	if show == nil {
		show = d.app.readPrompt
	}
	title := builtinDialogTitle
	if strings.Contains(title, "%") {
		title = fmt.Sprintf(title, filepath.Base(ask.Document), builtin.Title(ask.Program))
	}
	choice, err := show(title, builtinDialogMessage, false)
	if err != nil || choice == platform.ConfirmDeny {
		return confirm.Reply{ID: ask.ID, Choice: confirm.ChoiceDeny}, err
	}
	return confirm.Reply{ID: ask.ID, Choice: helperAnswerAllowDocument}, nil
}

func (builtinDialog) Close(string) {}

// builtinProgramFacts lists the three built-in programs first on the Programs page.
func (a *app) builtinProgramFacts() []appcore.Program {
	programs := make([]appcore.Program, 0, len(builtin.IDs))
	for _, id := range builtin.IDs {
		programs = append(programs, appcore.Program{
			ID:                id,
			Name:              id,
			Builtin:           true,
			On:                a.rt.cfg.BuiltinOn(id),
			AllowedDocuments:  a.rt.cfg.BuiltinDocuments(id),
			AllTrustedFolders: a.rt.cfg.BuiltinAllTrusted(id),
			RemovedDocuments:  a.rt.cfg.BuiltinRemovedDocuments(id),
		})
	}
	return programs
}

func (a *app) setBuiltinOn(id string, on bool) *appcore.Refusal {
	a.rt.cfg.SetBuiltinOn(id, on)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetBuiltinOn(id, !on)
		a.rt.logger.Printf("Could not save the %s switch: %v", id, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if !on {
		a.cancelBuiltins(id, "")
	}
	return nil
}

func (a *app) setBuiltinAllTrusted(e appcore.Effect) *appcore.Refusal {
	a.rt.cfg.SetBuiltinAllTrusted(e.ProgramID, e.On)
	a.saveConfig("built-in program access")
	if !e.On {
		a.cancelBuiltins(e.ProgramID, "")
	}
	return nil
}

// removeBuiltinAccess is program.removeAccess and program.removeAllAccess for a
// built-in program. A settings file that cannot be saved leaves every allow as
// it was.
func (a *app) removeBuiltinAccess(e appcore.Effect) *appcore.Refusal {
	all := e.Type == "program.removeAllAccess"
	previous, had := a.rt.cfg.BuiltinGrant(e.ProgramID)
	var removed []string
	if all {
		removed = a.rt.cfg.ForgetBuiltinAll(e.ProgramID)
	} else {
		removed = a.rt.cfg.ForgetBuiltin(e.ProgramID, e.DocumentID)
	}
	if err := a.rt.cfg.Save(); err != nil {
		if had {
			a.rt.cfg.RestoreBuiltin(e.ProgramID, previous)
		}
		a.rt.logger.Printf("Could not remove access to %s: %v", e.ProgramID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.changed()
	for _, document := range removed {
		a.cancelBuiltins(e.ProgramID, document)
	}
	if all {
		a.cancelBuiltins(e.ProgramID, "")
	}
	return nil
}

func (a *app) askBuiltinAgain(e appcore.Effect) *appcore.Refusal {
	a.rt.cfg.AskBuiltinAgain(e.ProgramID, e.DocumentID)
	a.saveConfig("built-in program access")
	return nil
}

// cancelBuiltins stops program's running requests: on one document, or on
// every document when document is "".
func (a *app) cancelBuiltins(program, document string) {
	a.mu.Lock()
	sites := append([]*site(nil), a.sites...)
	a.mu.Unlock()
	for _, s := range sites {
		if s.srv != nil {
			s.srv.CancelBuiltins(program, document)
		}
	}
	for _, core := range a.servedWireCores() {
		if document == "" {
			core.CancelBuiltins(program)
		} else {
			core.CancelBuiltinFor(program, document)
		}
	}
}

// examplesDir is ~/htmlclay/examples, where Open example writes.
func examplesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "htmlclay", "examples"), nil
}

// openBuiltinExample writes the program's example document (and, for the
// converter, a sample image) to ~/htmlclay/examples, trusts that folder, allows
// the example to run the program and opens it. An example already on disk is
// left as it is.
func (a *app) openBuiltinExample(id string) *appcore.Refusal {
	name, data, ok := builtin.Example(id)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchProgram, SubjectID: id}
	}
	dir, err := examplesDir()
	if err != nil {
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	path := filepath.Join(dir, name)
	if err := writeIfMissing(path, data); err != nil {
		a.rt.logger.Printf("Could not write the %s example: %v", id, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	if id == builtin.FileConvert {
		if err := writeIfMissing(filepath.Join(dir, sampleImageName), samplePhoto()); err != nil {
			a.rt.logger.Printf("Could not write the sample image: %v", err)
		}
	}
	if err := a.trustFolder(dir); err != nil {
		a.rt.logger.Printf("Could not trust %s for the %s example: %v", dir, id, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.rt.cfg.AllowBuiltin(id, path)
	a.saveConfig("built-in program example")
	a.openFile(path)
	return nil
}

func writeIfMissing(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

// samplePhoto is a 640 by 400 gradient the converter example starts with.
func samplePhoto() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 640, 400))
	for y := range 400 {
		for x := range 640 {
			img.Set(x, y, color.RGBA{R: uint8(40 + x*180/640), G: uint8(90 + y*120/400), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
