package main

import (
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/appcore"
)

// aiToolsFresh is how long the tool list is reused: finding Claude and Codex
// reads PATH, and the snapshot is built often.
const aiToolsFresh = 30 * time.Second

type aiToolsCache struct {
	mu    sync.Mutex
	at    time.Time
	tools []appcore.AIEditTool
}

// aiEditTools is the tool picker's list: Claude and Codex as found on the
// login PATH, then the custom commands, each with the model chosen for it.
func (a *app) aiEditTools() []appcore.AIEditTool {
	a.aiTools.mu.Lock()
	defer a.aiTools.mu.Unlock()
	if a.aiTools.tools == nil || time.Since(a.aiTools.at) > aiToolsFresh {
		_, engines := a.rt.cfg.AIEditEngines()
		models, efforts := a.rt.cfg.AIEditModels(), a.rt.cfg.AIEditEfforts()
		tools := []appcore.AIEditTool{}
		for _, t := range aiedit.Tools(engines, a.aiToolsEnv) {
			tools = append(tools, appcore.AIEditTool{
				ID:            t.ID,
				Custom:        t.Custom,
				Found:         t.Found,
				Model:         models[t.ID],
				DefaultModel:  t.DefaultModel,
				Models:        t.Models,
				Effort:        efforts[t.ID],
				DefaultEffort: t.DefaultEffort,
				Efforts:       t.Efforts,
				ModelEfforts:  t.ModelEfforts,
				Command:       aiedit.JoinCommand(t.Command),
			})
		}
		a.aiTools.tools, a.aiTools.at = tools, time.Now()
	}
	return append([]appcore.AIEditTool(nil), a.aiTools.tools...)
}

func (a *app) forgetAITools() {
	a.aiTools.mu.Lock()
	a.aiTools.tools = nil
	a.aiTools.mu.Unlock()
}

// aiEditTool is the engine AI editing uses when a request names none. With no
// choice saved, that is the one aiedit.Run picks.
func (a *app) aiEditTool() string {
	if name, _ := a.rt.cfg.AIEditEngines(); name != "" {
		return name
	}
	return aiedit.FirstFound(a.aiToolsEnv)
}

func (a *app) setAIEditModel(e appcore.Effect) *appcore.Refusal {
	if err := a.rt.cfg.SetAIEditModel(e.Tool, e.Model); err != nil {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "model"}
	}
	// If the newly selected model does not support the currently saved effort
	// (e.g., switching agy from gemini-4-neon at xhigh to gemini-4-carbon),
	// clear the stale effort back to the default in the same save.
	if curEffort := a.rt.cfg.AIEditEfforts()[e.Tool]; curEffort != "" {
		allowed := aiedit.EffortsForModel(e.Tool, e.Model)
		if len(allowed) > 0 && !slices.Contains(allowed, curEffort) {
			a.rt.cfg.SetAIEditEffort(e.Tool, "")
		}
	}
	a.saveConfig("AI editing model")
	a.forgetAITools()
	return nil
}

func (a *app) setAIEditEffort(e appcore.Effect) *appcore.Refusal {
	a.rt.cfg.SetAIEditEffort(e.Tool, e.Effort)
	a.saveConfig("AI editing effort")
	a.forgetAITools()
	return nil
}

func (a *app) setAIEditCustom(e appcore.Effect) *appcore.Refusal {
	argv, refusal := customCommand(e.Command)
	if refusal != nil {
		return refusal
	}
	a.rt.cfg.SetAIEditEngine(aiedit.CustomEngine, argv)
	a.rt.cfg.SetAIEditDefault(aiedit.CustomEngine)
	a.saveConfig("AI editing custom command")
	a.forgetAITools()
	return nil
}

func (a *app) setAIEditToolPreference(value json.RawMessage) *appcore.Refusal {
	var id string
	if json.Unmarshal(value, &id) != nil {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "value"}
	}
	for _, t := range a.aiEditTools() {
		if t.ID == id && t.Found {
			a.rt.cfg.SetAIEditDefault(id)
			a.saveConfig("AI editing tool")
			return nil
		}
	}
	return &appcore.Refusal{Code: appcore.RefuseNoSuchTool, Field: "value"}
}
