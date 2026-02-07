package config

// Workspace represents a named group of sessions for organizational purposes.
// Sessions can be assigned to a workspace, and the sidebar can be filtered
// to show only sessions in the active workspace.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
