package main

// servedWirePolicy is what a served folder's wire may do: programs by the
// person's decisions for each document, and no AI editing yet.
type servedWirePolicy struct{ a *app }

func (servedWirePolicy) CanAskProgram() bool                 { return true }
func (p servedWirePolicy) AskProgram(file, name string) bool { return p.a.askHelperForUse(file, name) }
func (p servedWirePolicy) ProgramGranted(file, name, id string) bool {
	return p.a.helperGranted(file, name, id)
}
func (servedWirePolicy) AIEditOffered() bool                          { return false }
func (servedWirePolicy) AIEditEnabled() bool                          { return false }
func (servedWirePolicy) AIEditEngines() (string, map[string][]string) { return "", nil }

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
