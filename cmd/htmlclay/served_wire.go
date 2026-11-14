package main

// servedWirePolicy is what a served folder's wire may do. Until programs reach
// served folders it carries frames and grants nothing.
type servedWirePolicy struct{ a *app }

func (servedWirePolicy) CanAskProgram() bool                          { return false }
func (servedWirePolicy) AskProgram(file, name string) bool            { return false }
func (servedWirePolicy) ProgramGranted(file, name, id string) bool    { return false }
func (servedWirePolicy) AIEditOffered() bool                          { return false }
func (servedWirePolicy) AIEditEnabled() bool                          { return false }
func (servedWirePolicy) AIEditEngines() (string, map[string][]string) { return "", nil }
