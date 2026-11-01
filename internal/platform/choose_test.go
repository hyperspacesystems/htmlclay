package platform

import "testing"

func TestChooseMapsEveryConfirmChoice(t *testing.T) {
	cases := map[ConfirmChoice]Choice{
		ConfirmDismissed:   ChoiceNone,
		ConfirmDeny:        ChoiceRefuse,
		ConfirmAllowOnce:   ChoiceMiddle,
		ConfirmAllowAlways: ChoiceAffirm,
	}
	for in, want := range cases {
		if got := choiceFromConfirm(in); got != want {
			t.Errorf("choiceFromConfirm(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestChooseLabelsKeepKeyboardOffDecisions(t *testing.T) {
	labels := chooseLabels("Deny", "More Options…", "Allow clay-search")
	if labels.Later == "" {
		t.Fatal("Choose must set a Later label, or Return lands on Deny")
	}
	if labels.denyLabel() != "Deny" || labels.Allow != "More Options…" || labels.Always != "Allow clay-search" {
		t.Fatalf("labels = %+v", labels)
	}
	if got := choiceFromOSAScript("Allow clay-search", labels); got != ConfirmAllowAlways {
		t.Fatalf("affirm button maps to %v", got)
	}
	if got := choiceFromZenity("", 1, labels); got != ConfirmDismissed {
		t.Fatalf("zenity Escape/Return on Not Now maps to %v, want dismissed", got)
	}
}
