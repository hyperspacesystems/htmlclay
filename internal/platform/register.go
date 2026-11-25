package platform

// RegisterFileTypes makes the operating system open .htmlclay documents with
// the binary at exePath, and offers HTML Clay in the Open With list for plain
// .html and .htm without touching whichever browser owns them. It is idempotent
// and safe to call on every launch, which is how a binary the user moved keeps a
// working association.
//
// It is best effort by contract: the error is for the log, never for the exit
// code. The launch most likely to be running this is a double-click on a file,
// and refusing to open that file because a registry write failed would trade a
// cosmetic problem for the whole product.
func RegisterFileTypes(exePath string, opts FileTypeOptions) error {
	return registerFileTypes(exePath, opts)
}

// FileTypeOptions is what the person chose: whether HTML Clay opens .htmlclay
// files, and whether it is offered in Open With for .html files.
type FileTypeOptions struct {
	HTMLClay     bool
	HTMLOpenWith bool
}

// FileTypeState is what the system says about HTML Clay's file types.
type FileTypeState struct {
	// Default is "htmlclay", "other" or "unknown" for .htmlclay files.
	Default string
	// HTMLOpenWith is HTML Clay being offered for .html files.
	HTMLOpenWith bool
}

// ReadFileTypes asks the system who opens .htmlclay files and whether HTML Clay
// is offered for .html files. It may run a program (xdg-mime), so call it off
// the snapshot path.
func ReadFileTypes() FileTypeState { return readFileTypes() }

// MakeDefault makes HTML Clay open .htmlclay files. On Windows it may delete
// another app's stored choice for .htmlclay (never .html or .htm), and answers
// ErrFinishInSettings, with Settings open, when Windows still names that app.
func MakeDefault(exePath string) error { return makeDefault(exePath) }

// StopOpening hands .htmlclay files back to the system: the default browser on
// Linux, nothing registered on Windows.
func StopOpening() error { return stopOpening() }

// SetHTMLOpenWith adds or removes HTML Clay from Open With for .html files.
func SetHTMLOpenWith(exePath string, on bool) error { return setHTMLOpenWith(exePath, on) }

// UnregisterFileTypes removes everything RegisterFileTypes wrote. It exists
// because there is otherwise no way back out short of editing the registry by
// hand: HTML Clay ships as a binary, not an installer, so uninstalling is
// deleting the file, and a deleted file leaves its associations behind.
func UnregisterFileTypes() error {
	return unregisterFileTypes()
}
