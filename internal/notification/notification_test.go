package notification

import (
	"errors"
	"testing"
)

// mockNotification records calls to the notification function
type mockNotification struct {
	calls []struct {
		title   string
		message string
		icon    any
	}
	err error
}

func (m *mockNotification) notify(title, message string, icon any) error {
	m.calls = append(m.calls, struct {
		title   string
		message string
		icon    any
	}{title, message, icon})
	return m.err
}

func TestSend(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		message     string
		mockErr     error
		expectError bool
	}{
		{
			name:        "successful notification",
			title:       "Test Title",
			message:     "Test Message",
			mockErr:     nil,
			expectError: false,
		},
		{
			name:        "notification error",
			title:       "Test Title",
			message:     "Test Message",
			mockErr:     errors.New("notification failed"),
			expectError: true,
		},
		{
			name:        "empty title",
			title:       "",
			message:     "Message with empty title",
			mockErr:     nil,
			expectError: false,
		},
		{
			name:        "empty message",
			title:       "Title",
			message:     "",
			mockErr:     nil,
			expectError: false,
		},
		{
			name:        "unicode content",
			title:       "通知",
			message:     "🎉 Notification with emoji",
			mockErr:     nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNotification{err: tt.mockErr}
			SetNotifier(mock.notify)
			defer ResetNotifier()

			err := Send(tt.title, tt.message)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(mock.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(mock.calls))
			}

			call := mock.calls[0]
			if call.title != tt.title {
				t.Errorf("title = %q, want %q", call.title, tt.title)
			}
			if call.message != tt.message {
				t.Errorf("message = %q, want %q", call.message, tt.message)
			}
			// Verify icon is the embedded PNG bytes
			iconBytes, ok := call.icon.([]byte)
			if !ok {
				t.Errorf("icon type = %T, want []byte", call.icon)
			} else if len(iconBytes) == 0 {
				t.Error("icon is empty, expected embedded PNG bytes")
			}
		})
	}
}

func TestSessionCompleted(t *testing.T) {
	tests := []struct {
		name            string
		sessionName     string
		expectedTitle   string
		expectedMessage string
		mockErr         error
		expectError     bool
	}{
		{
			name:            "basic session",
			sessionName:     "my-session",
			expectedTitle:   "Plural",
			expectedMessage: "my-session is ready",
			mockErr:         nil,
			expectError:     false,
		},
		{
			name:            "empty session name",
			sessionName:     "",
			expectedTitle:   "Plural",
			expectedMessage: " is ready",
			mockErr:         nil,
			expectError:     false,
		},
		{
			name:            "session with spaces",
			sessionName:     "My Cool Session",
			expectedTitle:   "Plural",
			expectedMessage: "My Cool Session is ready",
			mockErr:         nil,
			expectError:     false,
		},
		{
			name:            "unicode session name",
			sessionName:     "会话-123",
			expectedTitle:   "Plural",
			expectedMessage: "会话-123 is ready",
			mockErr:         nil,
			expectError:     false,
		},
		{
			name:            "notification failure",
			sessionName:     "test-session",
			expectedTitle:   "Plural",
			expectedMessage: "test-session is ready",
			mockErr:         errors.New("notification system unavailable"),
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNotification{err: tt.mockErr}
			SetNotifier(mock.notify)
			defer ResetNotifier()

			err := SessionCompleted(tt.sessionName)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(mock.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(mock.calls))
			}

			call := mock.calls[0]
			if call.title != tt.expectedTitle {
				t.Errorf("title = %q, want %q", call.title, tt.expectedTitle)
			}
			if call.message != tt.expectedMessage {
				t.Errorf("message = %q, want %q", call.message, tt.expectedMessage)
			}
		})
	}
}

func TestResetNotifier(t *testing.T) {
	// Set a custom notifier
	mock := &mockNotification{}
	SetNotifier(mock.notify)

	// Reset should restore default behavior
	ResetNotifier()

	// We can't easily test that it's back to beeep.Notify without sending
	// a real notification, but we can verify the mock is no longer used
	// by checking that mock.calls stays empty after reset
	// (this is a bit indirect but avoids sending real notifications)

	// The notifier variable is private, so we just verify the API works
	// without panicking
}
