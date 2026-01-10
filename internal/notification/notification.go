// Package notification provides cross-platform desktop notifications.
// It uses the beeep library to send notifications on macOS, Linux, and Windows.
package notification

import (
	"github.com/gen2brain/beeep"
	"github.com/zhubert/plural/internal/logger"
)

// Send sends a desktop notification with the given title and message.
// On macOS, it uses terminal-notifier or AppleScript.
// On Linux, it uses D-Bus or notify-send.
// On Windows, it uses the Windows Runtime COM API.
func Send(title, message string) error {
	logger.Log("Notification: Sending notification - title=%q, message=%q", title, message)
	// Use empty string for icon - beeep handles platform defaults
	err := beeep.Notify(title, message, "")
	if err != nil {
		logger.Log("Notification: Failed to send notification: %v", err)
	}
	return err
}

// SessionCompleted sends a notification that a Claude session has completed.
func SessionCompleted(sessionName string) error {
	return Send("Plural", sessionName+" is ready")
}
