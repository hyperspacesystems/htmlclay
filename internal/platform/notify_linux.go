//go:build linux

package platform

import "os/exec"

// notify uses notify-send (libnotify), present on essentially every GTK/Qt
// desktop. If it is missing the error is returned and the caller logs instead.
func notify(title, message string) error {
	return notifySend("critical", title, message)
}

func inform(title, message string) error {
	return notifySend("normal", title, message)
}

// notifySend names the sender HTML Clay with the icon and launcher install.sh
// puts in place; without them GNOME heads the notification "notify-send".
func notifySend(urgency, title, message string) error {
	return exec.Command("notify-send", notifySendArgs(urgency, title, message)...).Run()
}

func notifySendArgs(urgency, title, message string) []string {
	return []string{
		"--urgency=" + urgency,
		"--app-name=HTML Clay",
		"--icon=htmlclay",
		"--hint=string:desktop-entry:htmlclay",
		"--",
		title,
		message,
	}
}
