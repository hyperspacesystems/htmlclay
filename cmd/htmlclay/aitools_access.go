package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/helper"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
	"github.com/panphora/htmlclay/internal/trust"
	"github.com/panphora/htmlclay/internal/wire"
)

// documentTrust is both hosts' DocumentTrust. Mine and Ecosystem documents
// are ready with no dialog; any other is ready once the person allowed this
// code, and never after Don't allow.
func (a *app) documentTrust(document string) wire.Trust {
	path, err := resolveSymlinks(document)
	if err != nil {
		return wire.Trust{Tier: tier.Unknown.String()}
	}
	v := a.tierOf(path)
	return wire.Trust{Tier: v.Tier.String(), Ready: !v.Warn() && !v.Denied, Denied: v.Denied}
}

// askDocumentForWire is both hosts' AskDocument: the question names the
// helper or carried program that asked, so the person can tell them apart.
func (a *app) askDocumentForWire(document, program string) { a.askDocument(document, program) }

// aiToolsService is the one ai-tools service, or nil when its store cannot be
// made. Its scratch folders go in the system's temporary folder, whose path
// agy's folder rule can name.
func (a *app) aiToolsService() *aitools.Service {
	a.toolsOnce.Do(func() {
		store, err := aitools.OpenStore(filepath.Join(a.rt.configDir, "aitools"))
		if err != nil {
			a.rt.logger.Printf("AI with tools is off: %v", err)
			return
		}
		home, _ := os.UserHomeDir()
		a.toolsService = aitools.NewService(aitools.Config{
			Store:    store,
			Scratch:  filepath.Join(os.TempDir(), "htmlclay-ai-tools"),
			Home:     home,
			LoginEnv: helper.LoginEnv,
		})
	})
	return a.toolsService
}

// closeAITools stops the self-tests and sign-in renewals still running, when
// AI Editing is turned off or HTML Clay quits.
func (a *app) closeAITools() {
	if a.toolsService != nil {
		a.toolsService.Close()
	}
}

// aiToolsHost is the app's side of the ai-tools wire helper.
type aiToolsHost struct{ a *app }

func (a *app) aiToolsHost() wire.AIToolsHost { return aiToolsHost{a} }

func (h aiToolsHost) Enabled() bool {
	return h.a.rt.cfg.AIEditEnabled() && h.a.aiToolsService() != nil
}

func (h aiToolsHost) Service() *aitools.Service { return h.a.aiToolsService() }

func (h aiToolsHost) Preferred() (string, map[string]string) {
	name, _ := h.a.rt.cfg.AIEditEngines()
	return name, h.a.rt.cfg.AIEditModels()
}

func (h aiToolsHost) HasFolder(document string) bool {
	s := h.a.aiToolsService()
	if s == nil {
		return false
	}
	_, ok := s.Store().Folder(closedPath(document))
	return ok
}

// Folder is the folder the person gave document, or a new one they choose
// now. A remembered folder that is gone is folder_missing, so the page can
// ask with folder "choose".
func (h aiToolsHost) Folder(ctx context.Context, document string, choose bool, level string, asking func()) (string, bool, error) {
	return h.a.pageFolder(ctx, document, aitools.HelperName, choose, level, asking)
}

// pageFolder is the one folder a page's AI agent and its carried programs
// read: the remembered one when it still fits the level and is the same
// folder, or a new choice. program names who asks, in the question.
func (a *app) pageFolder(ctx context.Context, document, program string, choose bool, level string, asking func()) (string, bool, error) {
	h := aiToolsHost{a}
	s := h.a.aiToolsService()
	if s == nil {
		return "", false, &aitools.Error{Code: "helper_off", Message: aiToolsUnavailable}
	}
	path := closedPath(document)
	rec, remembered := s.Store().Folder(path)
	if remembered && !choose && (level != aitools.LevelChange || rec.Level == aitools.LevelChange) {
		if info, err := os.Lstat(rec.Folder); err == nil && info.IsDir() && trust.IdentityOK(rec.Folder, rec.Identity, h.a.rt.home) {
			return rec.Folder, true, nil
		}
		return "", false, &aitools.Error{Code: aitools.CodeFolderMissing, Message: fmt.Sprintf("The folder %s that this page reads is gone. Choose its folder again.", filepath.Base(rec.Folder))}
	}
	asking()
	folder, err := h.a.askAIToolsFolder(ctx, path, program, level)
	if err != nil {
		return "", false, err
	}
	kept := ""
	if level == aitools.LevelChange {
		kept = level
	} else if remembered && rec.Folder == folder {
		kept = rec.Level
	}
	if err := s.Store().SetFolder(aitools.FolderRecord{Document: path, Folder: folder, Identity: platform.DirIdentity(folder), Level: kept, At: time.Now().UTC()}); err != nil {
		h.a.rt.logger.Printf("Could not remember the folder for %s: %v", path, err)
		return folder, false, nil
	}
	h.a.changed()
	return folder, true, nil
}

const (
	aiToolsUnavailable  = "AI with tools is not available in this copy of HTML Clay."
	aiToolsNoFolderText = "You did not choose a folder, so the agent did not start."
	programNoFolderText = "You did not choose a folder, so the program did not start."

	aiToolsChangeFolderPrompt = "Choose the folder %s may let your AI agent read and change"
	programFolderChangePrompt = "Choose the folder the program %s in %s may read and change"
	aiToolsFolderPrompt       = "Choose the folder %s may let your AI agent read"
	programFolderPrompt       = "Choose the folder the program %s in %s may read"
)

// askAIToolsFolder asks the person which folder document's agent reads: the
// request window under the Mac app, a folder picker otherwise.
func (a *app) askAIToolsFolder(ctx context.Context, document, program, level string) (string, error) {
	ask := confirm.Ask{
		Kind:     appcore.RequestAITools,
		Document: document,
		Program:  program,
		Folder:   filepath.Base(filepath.Dir(document)),
		Path:     filepath.Dir(document),
		Identity: platform.DirIdentity(filepath.Dir(document)),
		Answers:  appcore.AnswersFor(appcore.RequestAITools),
	}
	if level == aitools.LevelChange {
		ask.Detail = level
	}
	presenter := a.presenter(aiToolsFolderDialog{})
	reply, err := presenter.Ask(ctx, ask)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &aitools.Error{Code: "helper_not_granted", Message: noFolderText(program)}
	}
	defer presenter.Close(reply.ID)
	if reply.Choice != confirm.ChoiceChooseFolder || !filepath.IsAbs(reply.Path) {
		return "", &aitools.Error{Code: "helper_not_granted", Message: noFolderText(program)}
	}
	// A folder chosen through a link is remembered as the folder itself, which
	// is what a later request checks is still there.
	folder, err := filepath.EvalSymlinks(reply.Path)
	if err != nil {
		return "", &aitools.Error{Code: aitools.CodeFolderMissing, Message: "The folder you chose cannot be read. Choose another folder."}
	}
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return "", &aitools.Error{Code: aitools.CodeFolderMissing, Message: "The folder you chose cannot be read. Choose another folder."}
	}
	return folder, nil
}

// noFolderText says who did not start when the person gives no folder.
func noFolderText(program string) string {
	if program == aitools.HelperName {
		return aiToolsNoFolderText
	}
	return programNoFolderText
}

// aiToolsFolderDialog is the folder question where the request window is not
// there: the system's folder picker.
type aiToolsFolderDialog struct{}

func (aiToolsFolderDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	change := ask.Detail == aitools.LevelChange
	var prompt string
	switch {
	case ask.Program == aitools.HelperName && change:
		prompt = fmt.Sprintf(aiToolsChangeFolderPrompt, filepath.Base(ask.Document))
	case ask.Program == aitools.HelperName:
		prompt = fmt.Sprintf(aiToolsFolderPrompt, filepath.Base(ask.Document))
	case change:
		prompt = fmt.Sprintf(programFolderChangePrompt, ask.Program, filepath.Base(ask.Document))
	default:
		prompt = fmt.Sprintf(programFolderPrompt, ask.Program, filepath.Base(ask.Document))
	}
	path, ok, err := platform.SelectFolder(prompt)
	if err != nil {
		return confirm.Reply{}, err
	}
	if !ok {
		return confirm.Reply{ID: ask.ID, Choice: confirm.ChoiceDeny}, nil
	}
	return confirm.Reply{ID: ask.ID, Choice: confirm.ChoiceChooseFolder, Path: path}, nil
}

func (aiToolsFolderDialog) Close(string) {}
