package platform

import "time"

const promptNameTimeout = 2 * time.Minute

// PromptName asks for one short text value. Cancel is ok=false with no error.
func PromptName(title, message, initial string) (value string, ok bool, err error) {
	return promptName(title, message, initial)
}

// PromptSecret asks for one value the screen must not show, such as a sync
// key. Cancel is ok=false with no error.
func PromptSecret(title, message string) (value string, ok bool, err error) {
	return promptSecret(title, message)
}
