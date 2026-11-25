package platform

// ChooseFile asks the person for one HTML Clay document (.htmlclay, .html or
// .htm). ok is false when they cancelled.
func ChooseFile(title string) (path string, ok bool, err error) { return chooseFile(title) }
