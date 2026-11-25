package main

import (
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/wire"
)

// servedWirePolicy is what a served folder's wire may do: programs by the
// person's decisions for each document, and AI editing by the switch and, in a
// team folder, the person's allow for each document.
type servedWirePolicy struct{ a *app }

func (servedWirePolicy) CanAskProgram() bool                 { return true }
func (p servedWirePolicy) AskProgram(file, name string) bool { return p.a.askHelperForUse(file, name) }
func (p servedWirePolicy) RaiseSetup(file, name string)      { p.a.raiseOpenQuestions(file) }
func (p servedWirePolicy) ProgramGranted(file, name, id string) bool {
	return p.a.helperGranted(file, name, id)
}
func (servedWirePolicy) AIEditOffered() bool { return true }
func (p servedWirePolicy) AIEditFor(document string) (on, ask bool) {
	return p.a.aiEditFor(document)
}
func (p servedWirePolicy) AskAIEdit(document string) { p.a.askAIEdit(document) }
func (p servedWirePolicy) AIEditFiles(document string) bool {
	return p.a.aiEditFiles(document)
}
func (p servedWirePolicy) AIEditEngines() (string, map[string][]string) {
	return p.a.rt.cfg.AIEditEngines()
}

func (p servedWirePolicy) BuiltinFor(document, program string) builtin.Access {
	return p.a.builtinFor(document, program)
}

func (p servedWirePolicy) AskBuiltin(document, program string) { p.a.askBuiltin(document, program) }

func (p servedWirePolicy) RevealFile(path string) error { return p.a.revealPath(path) }

func (p servedWirePolicy) DocumentTrust(document string) wire.Trust {
	return p.a.documentTrust(document)
}

func (p servedWirePolicy) AskDocument(document, program string) {
	p.a.askDocumentForWire(document, program)
}

func (p servedWirePolicy) AITools() wire.AIToolsHost { return p.a.aiToolsHost() }

func (p servedWirePolicy) Programs() wire.ProgramsHost { return p.a.programsHost() }

func (p servedWirePolicy) NoteAIEditResult(document, elementHTML, tag, resultHTML string) {
	p.a.noteAIEditResult(document, elementHTML, tag, resultHTML)
}

func (p servedWirePolicy) NoteDocumentSaved(document string, data []byte) {
	p.a.noteDocumentSaved(document, data)
}

func (p servedWirePolicy) AIEditModels() map[string]string {
	return p.a.rt.cfg.AIEditModels()
}

func (p servedWirePolicy) AIEditEfforts() map[string]string {
	return p.a.rt.cfg.AIEditEfforts()
}

// bindServedPage binds a served document's programs before its page loads,
// with the decisions already made. Nothing is asked here: a page load is not a
// request to set anything up.
func (a *app) bindServedPage(rootID, absPath string) {
	if a.served == nil {
		return
	}
	core, ok := a.served.Wire(rootID)
	if !ok {
		return
	}
	a.applyHelperPlan(core, absPath, a.helpersForNavigation(absPath))
}
