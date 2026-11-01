package platform

// Choice is the button a Choose dialog was answered with.
type Choice int

const (
	// ChoiceNone is no answer: the close box, the timeout, Return or Escape
	// where the platform puts them on the Not Now button, and every error.
	ChoiceNone Choice = iota
	ChoiceRefuse
	ChoiceMiddle
	ChoiceAffirm
)

// chooseLaterLabel is the button that decides nothing. Supplying it is what
// makes Confirm keep the keyboard off every decision: macOS draws no default
// button at all, and Windows and Linux put Return and Escape on it.
const chooseLaterLabel = "Not Now"

// Choose shows a native dialog with three labelled buttons, laid out refuse,
// middle, affirm, and no keyboard default that decides anything: only a click
// answers. It is Confirm with a Not Now button, so it inherits Confirm's
// guarantees: a real OS dialog a page cannot spoof, failing closed to
// ChoiceNone on any error or timeout.
func Choose(title, message, refuse, middle, affirm string) (Choice, error) {
	choice, err := confirmDialog(title, message, chooseLabels(refuse, middle, affirm))
	return choiceFromConfirm(choice), err
}

func chooseLabels(refuse, middle, affirm string) ConfirmLabels {
	return ConfirmLabels{Deny: refuse, Allow: middle, Always: affirm, Later: chooseLaterLabel}
}

func choiceFromConfirm(c ConfirmChoice) Choice {
	switch c {
	case ConfirmDeny:
		return ChoiceRefuse
	case ConfirmAllowOnce:
		return ChoiceMiddle
	case ConfirmAllowAlways:
		return ChoiceAffirm
	}
	return ChoiceNone
}
