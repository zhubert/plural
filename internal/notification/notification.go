// Package notification provides cross-platform desktop notifications.
// It uses the beeep library to send notifications on macOS, Linux, and Windows.
package notification

import (
	_ "embed"

	"github.com/gen2brain/beeep"
	"github.com/zhubert/plural-core/logger"
)

//go:embed assets/icon.png
var icon []byte

// NotifyFunc is the function signature for sending notifications.
// This allows for dependency injection in tests.
type NotifyFunc func(title, message string, icon any) error

// notifier is the function used to send notifications.
// It defaults to beeep.Notify but can be replaced for testing.
var notifier NotifyFunc = beeep.Notify

// SetNotifier sets the notification function. Used for testing.
func SetNotifier(fn NotifyFunc) {
	notifier = fn
}

// ResetNotifier resets the notification function to the default (beeep.Notify).
func ResetNotifier() {
	notifier = beeep.Notify
}

// Send sends a desktop notification with the given title and message.
// On macOS, it uses terminal-notifier or AppleScript.
// On Linux, it uses D-Bus or notify-send.
// On Windows, it uses the Windows Runtime COM API.
func Send(title, message string) error {
	log := logger.WithComponent("notification")
	log.Debug("sending notification", "title", title, "message", message)
	err := notifier(title, message, icon)
	if err != nil {
		log.Error("failed to send notification", "error", err)
	}
	return err
}

// SessionCompleted sends a notification that a Claude session has completed.
func SessionCompleted(sessionName string) error {
	return Send("Plural", sessionName+" is ready")
}
