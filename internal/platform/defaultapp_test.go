package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestNextDefaultStep(t *testing.T) {
	cases := []struct {
		name      string
		effective string
		hasChoice bool
		acted     bool
		want      defaultStep
	}{
		{"windows already opens with us", "htmlclay.document", false, false, stepAlreadyDefault},
		{"our own choice is stored", progID, true, false, stepAlreadyDefault},
		{"nobody chosen and nothing registered", "", false, false, stepRegister},
		{"another app's class default and no choice", "Other.htmlclay", false, false, stepRegister},
		{"the person chose another app", "VSCode.htmlclay", true, false, stepClearChoice},
		{"windows kept the other app after the clear", "VSCode.htmlclay", true, true, stepOpenSettings},
		{"windows names another app after registering", `Applications\notepad.exe`, false, true, stepOpenSettings},
		{"the clear worked", progID, false, true, stepAlreadyDefault},
	}
	for _, c := range cases {
		if got := nextDefaultStep(c.effective, c.hasChoice, c.acted); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// fakeWindows answers the opener the way Windows does: a stored choice wins over
// the class default, and kept stands for a Windows that goes on naming the other
// app whatever HTML Clay changes.
type fakeWindows struct {
	choice, classDefault, kept string
	choiceStays, deleteRefused bool
	writes, deletes, settings  int
}

func (w *fakeWindows) effectiveProgID() string {
	switch {
	case w.kept != "":
		return w.kept
	case w.choice != "":
		return w.choice
	}
	return w.classDefault
}

func (w *fakeWindows) hasChoice() bool { return w.choice != "" }

func (w *fakeWindows) writeClassDefault() error {
	w.writes++
	w.classDefault = progID
	return nil
}

func (w *fakeWindows) deleteChoice() error {
	w.deletes++
	if w.deleteRefused {
		return errors.New("access is denied")
	}
	if w.deletes > 2 {
		return errors.New("deleted the choice again and again")
	}
	if !w.choiceStays {
		w.choice = ""
	}
	return nil
}

func (w *fakeWindows) openSettings() error {
	w.settings++
	return nil
}

// Make default deletes another app's .htmlclay choice only when one is stored,
// at most once per click, and sends the person to Settings rather than
// deleting again when Windows keeps the other app.
func TestMakeDefaultDeletesAnotherAppsChoiceAtMostOnce(t *testing.T) {
	cases := []struct {
		name                      string
		w                         fakeWindows
		writes, deletes, settings int
		finishInSettings          bool
	}{
		{"already ours: nothing changes", fakeWindows{classDefault: progID}, 0, 0, 0, false},
		{"nothing chosen: the class default is enough", fakeWindows{}, 1, 0, 0, false},
		{"another app chosen: its choice goes", fakeWindows{choice: "Other.htmlclay", classDefault: progID}, 1, 1, 0, false},
		{"windows keeps the other app: Settings, one delete", fakeWindows{choice: "Other.htmlclay", kept: "Other.htmlclay"}, 1, 1, 1, true},
		{"no choice and windows names another app: Settings, no delete", fakeWindows{kept: "Other.htmlclay"}, 1, 0, 1, true},
		{"the choice is back after the delete: Settings, one delete", fakeWindows{choice: "Other.htmlclay", choiceStays: true}, 1, 1, 1, true},
		{"windows refuses the delete: registered, then Settings", fakeWindows{choice: "Other.htmlclay", deleteRefused: true}, 1, 1, 1, true},
	}
	for _, c := range cases {
		w := c.w
		err := runMakeDefault(&w)
		if got := errors.Is(err, ErrFinishInSettings); got != c.finishInSettings {
			t.Errorf("%s: finish in settings = %v (err %v), want %v", c.name, got, err, c.finishInSettings)
		}
		if w.deletes != c.deletes {
			t.Errorf("%s: deleted another app's choice %d times, want %d", c.name, w.deletes, c.deletes)
		}
		if w.writes != c.writes || w.settings != c.settings {
			t.Errorf("%s: wrote the class default %d times and opened Settings %d times, want %d and %d", c.name, w.writes, w.settings, c.writes, c.settings)
		}
	}
}

// The delete reaches exactly the two .htmlclay choice keys and nothing for
// .html or .htm.
func TestMakeDefaultDeletesOnlyTheHtmlclayChoiceKeys(t *testing.T) {
	want := []string{
		`Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.htmlclay\UserChoiceLatest`,
		`Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.htmlclay\UserChoice`,
	}
	if strings.Join(htmlclayChoiceKeys, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Make default may delete %q, want exactly %q", htmlclayChoiceKeys, want)
	}
}
