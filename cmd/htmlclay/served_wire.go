package main

// servedWirePolicy is what a served folder's wire may do: programs by the
// person's decisions for each document, and AI editing by the switch and, in a
// team folder, the person's allow for each document.
type servedWirePolicy struct{ a *app }

func (servedWirePolicy) CanAskProgram() bool                 { return true }
func (p servedWirePolicy) AskProgram(file, name string) bool { return p.a.askHelperForUse(file, name) }
func (p servedWirePolicy) ProgramGranted(file, name, id string) bool {
	return p.a.helperGranted(file, name, id)
}
func (servedWirePolicy) AIEditOffered() bool { return true }
func (p servedWirePolicy) AIEditFor(document string) (on, ask bool) {
	return p.a.aiEditFor(document)
}
func (p servedWirePolicy) AskAIEdit(document string) { p.a.askAIEdit(document) }
func (p servedWirePolicy) AIEditEngines() (string, map[string][]string) {
	return p.a.rt.cfg.AIEditEngines()
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
