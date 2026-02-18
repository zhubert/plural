package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhubert/plural/internal/paths"
)

// Configuration constants
const (
	// MaxSessionMessageLines is the maximum number of lines to keep in session message history
	MaxSessionMessageLines = 10000
)

// Message represents a chat message for persistence
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SaveSessionMessages saves messages for a session (keeps last maxLines lines)
func SaveSessionMessages(sessionID string, messages []Message, maxLines int) error {
	dir, err := paths.SessionsDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Keep only the last maxLines worth of content
	if maxLines > 0 && len(messages) > 0 {
		// Trim messages to approximately maxLines of content
		var totalLines int
		startIdx := len(messages)
		for i := len(messages) - 1; i >= 0; i-- {
			lines := countLines(messages[i].Content)
			if totalLines+lines > maxLines && startIdx < len(messages) {
				break
			}
			totalLines += lines
			startIdx = i
		}
		messages = messages[startIdx:]
	}

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, sessionID+".json")
	return os.WriteFile(path, data, 0644)
}

// LoadSessionMessages loads messages for a session
func LoadSessionMessages(sessionID string) ([]Message, error) {
	dir, err := paths.SessionsDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// DeleteSessionMessages deletes the messages file for a session
func DeleteSessionMessages(sessionID string) error {
	dir, err := paths.SessionsDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, sessionID+".json")
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ClearAllSessionMessages deletes all session message files.
// Returns the number of files deleted.
func ClearAllSessionMessages() (int, error) {
	dir, err := paths.SessionsDir()
	if err != nil {
		return 0, err
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.Remove(path); err != nil {
			continue // Best-effort deletion
		}
		deleted++
	}

	return deleted, nil
}

// FindOrphanedSessionMessages finds session message files that don't have
// a matching session in the config. Returns the session IDs of orphaned files.
func FindOrphanedSessionMessages(cfg *Config) ([]string, error) {
	dir, err := paths.SessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Build set of known session IDs
	knownSessions := make(map[string]bool)
	for _, sess := range cfg.GetSessions() {
		knownSessions[sess.ID] = true
	}

	var orphans []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		// Extract session ID from filename (remove .json suffix)
		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		if !knownSessions[sessionID] {
			orphans = append(orphans, sessionID)
		}
	}

	return orphans, nil
}

// PruneOrphanedSessionMessages deletes session message files that don't have
// a matching session in the config. Returns the number of files deleted.
func PruneOrphanedSessionMessages(cfg *Config) (int, error) {
	orphans, err := FindOrphanedSessionMessages(cfg)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, sessionID := range orphans {
		if err := DeleteSessionMessages(sessionID); err == nil {
			deleted++
		}
	}

	return deleted, nil
}

// FormatTranscript formats session messages as a human-readable plain text transcript.
// Each message is prefixed with "User:" or "Assistant:" and separated by blank lines.
func FormatTranscript(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, msg := range messages {
		switch msg.Role {
		case "user":
			sb.WriteString("User:\n")
		case "assistant":
			sb.WriteString("Assistant:\n")
		default:
			sb.WriteString(msg.Role + ":\n")
		}
		sb.WriteString(msg.Content)
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// countLines counts the number of lines in a string
func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}
