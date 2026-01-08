package ui

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ModalState is a discriminated union interface for modal-specific state.
// Each modal type implements this interface with its own state struct,
// ensuring type-safe access to modal-specific fields.
type ModalState interface {
	modalState() // marker method to restrict implementations
	Title() string
	Help() string
	Render() string
	Update(msg tea.Msg) (ModalState, tea.Cmd)
}

// Modal represents a popup dialog with type-safe state management.
// The State field is nil when no modal is visible.
type Modal struct {
	State ModalState
	error string
}

// MCPServerDisplay represents an MCP server for display in the modal
type MCPServerDisplay struct {
	Name     string
	Command  string
	Args     string // Args joined as string for display
	IsGlobal bool
	RepoPath string // Only set if per-repo
}

// NewModal creates a new modal
func NewModal() *Modal {
	return &Modal{}
}

// Show displays a modal with the given state
func (m *Modal) Show(state ModalState) {
	m.State = state
	m.error = ""
}

// Hide hides the modal
func (m *Modal) Hide() {
	m.State = nil
	m.error = ""
}

// IsVisible returns whether the modal is visible
func (m *Modal) IsVisible() bool {
	return m.State != nil
}

// SetError sets an error message
func (m *Modal) SetError(err string) {
	m.error = err
}

// GetError returns the current error message
func (m *Modal) GetError() string {
	return m.error
}

// Update handles messages by delegating to the current state
func (m *Modal) Update(msg tea.Msg) (*Modal, tea.Cmd) {
	if m.State == nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.State, cmd = m.State.Update(msg)
	return m, cmd
}

// View renders the modal
func (m *Modal) View(screenWidth, screenHeight int) string {
	if m.State == nil {
		return ""
	}

	content := m.State.Render()

	// Add error if present
	if m.error != "" {
		content += "\n" + StatusErrorStyle.Render(m.error)
	}

	modal := ModalStyle.Render(content)

	return lipgloss.Place(
		screenWidth, screenHeight,
		lipgloss.Center, lipgloss.Center,
		modal,
	)
}

// =============================================================================
// AddRepoState - State for the Add Repository modal
// =============================================================================

type AddRepoState struct {
	Input          textinput.Model
	SuggestedRepo  string
	UseSuggested   bool
}

func (*AddRepoState) modalState() {}

func (s *AddRepoState) Title() string { return "Add Repository" }

func (s *AddRepoState) Help() string {
	if s.SuggestedRepo != "" {
		return "↑/↓ to switch, Enter to confirm, Esc to cancel"
	}
	return "Enter the full path to a git repository"
}

func (s *AddRepoState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var content string

	if s.SuggestedRepo != "" {
		suggestionLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("Current directory:")

		style := SidebarItemStyle
		prefix := "  "
		if s.UseSuggested {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		suggestionItem := style.Render(prefix + s.SuggestedRepo)

		otherLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Or enter a different path:")

		inputStyle := lipgloss.NewStyle()
		if !s.UseSuggested {
			inputStyle = inputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			inputStyle = inputStyle.PaddingLeft(2)
		}
		inputView := inputStyle.Render(s.Input.View())

		content = lipgloss.JoinVertical(lipgloss.Left, suggestionLabel, suggestionItem, otherLabel, inputView)
	} else {
		content = s.Input.View()
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *AddRepoState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && s.SuggestedRepo != "" {
		switch keyMsg.String() {
		case "up", "down", "tab":
			s.UseSuggested = !s.UseSuggested
			if s.UseSuggested {
				s.Input.Blur()
			} else {
				s.Input.Focus()
			}
			return s, nil
		}
	}

	// Only update text input when it's focused
	if !s.UseSuggested {
		var cmd tea.Cmd
		s.Input, cmd = s.Input.Update(msg)
		return s, cmd
	}

	return s, nil
}

// GetPath returns the path to add (either suggested or from input)
func (s *AddRepoState) GetPath() string {
	if s.SuggestedRepo != "" && s.UseSuggested {
		return s.SuggestedRepo
	}
	return s.Input.Value()
}

// NewAddRepoState creates a new AddRepoState with proper initialization
func NewAddRepoState(suggestedRepo string) *AddRepoState {
	ti := textinput.New()
	ti.Placeholder = "/path/to/repo"
	ti.CharLimit = ModalInputCharLimit
	ti.SetWidth(ModalInputWidth)

	state := &AddRepoState{
		Input:         ti,
		SuggestedRepo: suggestedRepo,
		UseSuggested:  suggestedRepo != "",
	}

	if suggestedRepo == "" {
		state.Input.Focus()
	}

	return state
}

// =============================================================================
// NewSessionState - State for the New Session modal
// =============================================================================

type NewSessionState struct {
	RepoOptions []string
	RepoIndex   int
	BranchInput textinput.Model
	Focus       int // 0=repo list, 1=branch input
}

func (*NewSessionState) modalState() {}

func (s *NewSessionState) Title() string { return "New Session" }

func (s *NewSessionState) Help() string {
	return "↑/↓ select repo  Tab: branch name  Enter: create"
}

func (s *NewSessionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Repository selection section
	repoLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Repository:")

	var repoList string
	if len(s.RepoOptions) == 0 {
		repoList = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No repositories added. Press 'r' to add one first.")
	} else {
		for i, repo := range s.RepoOptions {
			style := SidebarItemStyle
			prefix := "  "
			if s.Focus == 0 && i == s.RepoIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			} else if i == s.RepoIndex {
				prefix = "● "
			}
			repoList += style.Render(prefix+repo) + "\n"
		}
	}

	// Branch name input section
	branchLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Branch name:")

	branchInputStyle := lipgloss.NewStyle()
	if s.Focus == 1 {
		branchInputStyle = branchInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		branchInputStyle = branchInputStyle.PaddingLeft(2)
	}
	branchView := branchInputStyle.Render(s.BranchInput.View())

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoList, branchLabel, branchView, help)
}

func (s *NewSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.Focus == 0 && s.RepoIndex > 0 {
				s.RepoIndex--
			}
		case "down", "j":
			if s.Focus == 0 && s.RepoIndex < len(s.RepoOptions)-1 {
				s.RepoIndex++
			}
		case "tab":
			if s.Focus == 0 {
				s.Focus = 1
				s.BranchInput.Focus()
			} else {
				s.Focus = 0
				s.BranchInput.Blur()
			}
			return s, nil
		case "shift+tab":
			if s.Focus == 1 {
				s.Focus = 0
				s.BranchInput.Blur()
			}
			return s, nil
		}
	}

	// Handle branch input updates when focused
	if s.Focus == 1 {
		var cmd tea.Cmd
		s.BranchInput, cmd = s.BranchInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// GetSelectedRepo returns the selected repository path
func (s *NewSessionState) GetSelectedRepo() string {
	if len(s.RepoOptions) == 0 || s.RepoIndex >= len(s.RepoOptions) {
		return ""
	}
	return s.RepoOptions[s.RepoIndex]
}

// GetBranchName returns the custom branch name
func (s *NewSessionState) GetBranchName() string {
	return s.BranchInput.Value()
}

// NewNewSessionState creates a new NewSessionState with proper initialization
func NewNewSessionState(repos []string) *NewSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = 100
	branchInput.SetWidth(ModalInputWidth)

	return &NewSessionState{
		RepoOptions: repos,
		RepoIndex:   0,
		BranchInput: branchInput,
		Focus:       0,
	}
}

// =============================================================================
// ConfirmDeleteState - State for the Confirm Delete modal
// =============================================================================

type ConfirmDeleteState struct {
	Options       []string
	SelectedIndex int
}

func (*ConfirmDeleteState) modalState() {}

func (s *ConfirmDeleteState) Title() string { return "Delete Session?" }

func (s *ConfirmDeleteState) Help() string {
	return "↑/↓ to select, Enter to confirm, Esc to cancel"
}

func (s *ConfirmDeleteState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		MarginBottom(1).
		Render("This will remove the session from the list.")

	var optionList string
	for i, opt := range s.Options {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, message, optionList, help)
}

func (s *ConfirmDeleteState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Options)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// ShouldDeleteWorktree returns true if user selected to delete the worktree
func (s *ConfirmDeleteState) ShouldDeleteWorktree() bool {
	return s.SelectedIndex == 1 // "Delete worktree" is index 1
}

// NewConfirmDeleteState creates a new ConfirmDeleteState
func NewConfirmDeleteState() *ConfirmDeleteState {
	return &ConfirmDeleteState{
		Options:       []string{"Keep worktree", "Delete worktree"},
		SelectedIndex: 0,
	}
}

// =============================================================================
// MergeState - State for the Merge/PR modal
// =============================================================================

type MergeState struct {
	Options        []string
	SelectedIndex  int
	HasRemote      bool
	ChangesSummary string
}

func (*MergeState) modalState() {}

func (s *MergeState) Title() string { return "Merge/PR" }

func (s *MergeState) Help() string {
	return "↑/↓ to select, Enter to confirm, Esc to cancel"
}

func (s *MergeState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show changes summary
	var summarySection string
	if s.ChangesSummary != "" {
		summaryStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)
		summarySection = summaryStyle.Render("Changes: " + s.ChangesSummary)
	} else {
		noChangesStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginBottom(1)
		summarySection = noChangesStyle.Render("No uncommitted changes")
	}

	var optionList string
	for i, opt := range s.Options {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	if !s.HasRemote {
		note := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("(No remote origin - PR option unavailable)")
		optionList += "\n" + note
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, summarySection, optionList, help)
}

func (s *MergeState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Options)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedOption returns the selected merge option
func (s *MergeState) GetSelectedOption() string {
	if len(s.Options) == 0 || s.SelectedIndex >= len(s.Options) {
		return ""
	}
	return s.Options[s.SelectedIndex]
}

// NewMergeState creates a new MergeState
func NewMergeState(hasRemote bool, changesSummary string) *MergeState {
	options := []string{"Merge to main"}
	if hasRemote {
		options = append(options, "Create PR")
	}

	return &MergeState{
		Options:        options,
		SelectedIndex:  0,
		HasRemote:      hasRemote,
		ChangesSummary: changesSummary,
	}
}

// =============================================================================
// EditCommitState - State for the Edit Commit Message modal
// =============================================================================

type EditCommitState struct {
	Textarea  textarea.Model
	MergeType string // "merge" or "pr"
}

func (*EditCommitState) modalState() {}

func (s *EditCommitState) Title() string { return "Edit Commit Message" }

func (s *EditCommitState) Help() string {
	return "Ctrl+s: commit  Esc: cancel"
}

func (s *EditCommitState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show what operation will follow
	operationLabel := "Merge to main"
	if s.MergeType == "pr" {
		operationLabel = "Create PR"
	}
	operationStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		MarginBottom(1)
	operationSection := operationStyle.Render("After commit: " + operationLabel)

	textareaView := s.Textarea.View()

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, operationSection, textareaView, help)
}

func (s *EditCommitState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.Textarea, cmd = s.Textarea.Update(msg)
	return s, cmd
}

// GetMessage returns the commit message
func (s *EditCommitState) GetMessage() string {
	return s.Textarea.Value()
}

// NewEditCommitState creates a new EditCommitState
func NewEditCommitState(message, mergeType string) *EditCommitState {
	ta := textarea.New()
	ta.Placeholder = "Enter commit message..."
	ta.CharLimit = 0
	ta.SetHeight(10)
	ta.SetWidth(ModalInputWidth)
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetValue(message)
	ta.Focus()

	return &EditCommitState{
		Textarea:  ta,
		MergeType: mergeType,
	}
}

// =============================================================================
// MCPServersState - State for the MCP Servers list modal
// =============================================================================

type MCPServersState struct {
	Servers       []MCPServerDisplay
	SelectedIndex int
	Repos         []string
}

func (*MCPServersState) modalState() {}

func (s *MCPServersState) Title() string { return "MCP Servers" }

func (s *MCPServersState) Help() string {
	return "↑/↓ navigate  a: add  d: delete  Esc: close"
}

func (s *MCPServersState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var content string
	if len(s.Servers) == 0 {
		content = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No MCP servers configured.\nPress 'a' to add one.")
	} else {
		// Group servers for display
		currentRepo := ""
		globalShown := false
		idx := 0

		for _, server := range s.Servers {
			// Show section headers
			if server.IsGlobal && !globalShown {
				if idx > 0 {
					content += "\n"
				}
				content += lipgloss.NewStyle().
					Foreground(ColorSecondary).
					Bold(true).
					Render("Global:") + "\n"
				globalShown = true
			} else if !server.IsGlobal && server.RepoPath != currentRepo {
				content += "\n"
				content += lipgloss.NewStyle().
					Foreground(ColorSecondary).
					Bold(true).
					Render(truncatePath(server.RepoPath, 40)+":") + "\n"
				currentRepo = server.RepoPath
			}

			// Render server entry
			style := SidebarItemStyle
			prefix := "  "
			if idx == s.SelectedIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			}

			display := server.Name + "  " + lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render(truncateString(server.Command+" "+server.Args, 35))
			content += style.Render(prefix+display) + "\n"
			idx++
		}
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *MCPServersState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Servers)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedServer returns the selected server for deletion
func (s *MCPServersState) GetSelectedServer() *MCPServerDisplay {
	if len(s.Servers) == 0 || s.SelectedIndex >= len(s.Servers) {
		return nil
	}
	return &s.Servers[s.SelectedIndex]
}

// NewMCPServersState creates a new MCPServersState
func NewMCPServersState(globalServers []MCPServerDisplay, perRepoServers map[string][]MCPServerDisplay, repos []string) *MCPServersState {
	// Build flattened list for navigation
	var servers []MCPServerDisplay
	for _, s := range globalServers {
		servers = append(servers, s)
	}
	for _, repo := range repos {
		for _, s := range perRepoServers[repo] {
			servers = append(servers, s)
		}
	}

	return &MCPServersState{
		Servers:       servers,
		SelectedIndex: 0,
		Repos:         repos,
	}
}

// =============================================================================
// AddMCPServerState - State for the Add MCP Server modal
// =============================================================================

type AddMCPServerState struct {
	IsGlobal   bool
	Repos      []string
	RepoIndex  int
	NameInput  textinput.Model
	CmdInput   textinput.Model
	ArgsInput  textinput.Model
	InputIndex int // 0=scope, 1=repo, 2=name, 3=cmd, 4=args
}

func (*AddMCPServerState) modalState() {}

func (s *AddMCPServerState) Title() string { return "Add MCP Server" }

func (s *AddMCPServerState) Help() string {
	return "Tab: next  Space: toggle scope  Enter: save  Esc: cancel"
}

func (s *AddMCPServerState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Scope selector
	scopeLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Scope:")

	globalStyle := SidebarItemStyle
	globalPrefix := "  "
	if s.InputIndex == 0 && s.IsGlobal {
		globalStyle = SidebarSelectedStyle
		globalPrefix = "> "
	} else if s.IsGlobal {
		globalPrefix = "● "
	}
	globalOpt := globalStyle.Render(globalPrefix + "Global")

	repoStyle := SidebarItemStyle
	repoPrefix := "  "
	if s.InputIndex == 0 && !s.IsGlobal {
		repoStyle = SidebarSelectedStyle
		repoPrefix = "> "
	} else if !s.IsGlobal {
		repoPrefix = "● "
	}
	repoOpt := repoStyle.Render(repoPrefix + "Per-repository")

	scopeSection := lipgloss.JoinVertical(lipgloss.Left, scopeLabel, globalOpt, repoOpt)

	// Repo selector (only if per-repo)
	var repoSection string
	if !s.IsGlobal && len(s.Repos) > 0 {
		repoLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Repository:")

		var repoList string
		for i, repo := range s.Repos {
			style := SidebarItemStyle
			prefix := "  "
			if s.InputIndex == 1 && i == s.RepoIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			} else if i == s.RepoIndex {
				prefix = "● "
			}
			repoList += style.Render(prefix+truncatePath(repo, 40)) + "\n"
		}
		repoSection = lipgloss.JoinVertical(lipgloss.Left, repoLabel, repoList)
	}

	// Input fields
	inputLabel := func(label string, focused bool) string {
		style := lipgloss.NewStyle().Foreground(ColorTextMuted).MarginTop(1)
		if focused {
			style = style.Foreground(ColorPrimary)
		}
		return style.Render(label)
	}

	nameLabel := inputLabel("Name:", s.InputIndex == 2)
	nameInput := s.NameInput.View()

	cmdLabel := inputLabel("Command:", s.InputIndex == 3)
	cmdInput := s.CmdInput.View()

	argsLabel := inputLabel("Args:", s.InputIndex == 4)
	argsInput := s.ArgsInput.View()

	inputSection := lipgloss.JoinVertical(lipgloss.Left,
		nameLabel, nameInput,
		cmdLabel, cmdInput,
		argsLabel, argsInput,
	)

	help := ModalHelpStyle.Render(s.Help())

	if repoSection != "" {
		return lipgloss.JoinVertical(lipgloss.Left, title, scopeSection, repoSection, inputSection, help)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, scopeSection, inputSection, help)
}

func (s *AddMCPServerState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "tab", "down":
			s.advanceInput()
			return s, nil
		case "shift+tab", "up":
			s.retreatInput()
			return s, nil
		case " ":
			// Space toggles scope when on scope selector
			if s.InputIndex == 0 {
				s.IsGlobal = !s.IsGlobal
			}
			return s, nil
		}
	}

	// Handle text input updates
	if s.InputIndex >= 2 {
		var cmd tea.Cmd
		switch s.InputIndex {
		case 2:
			s.NameInput, cmd = s.NameInput.Update(msg)
		case 3:
			s.CmdInput, cmd = s.CmdInput.Update(msg)
		case 4:
			s.ArgsInput, cmd = s.ArgsInput.Update(msg)
		}
		return s, cmd
	}

	return s, nil
}

func (s *AddMCPServerState) advanceInput() {
	s.blurAllInputs()

	maxIndex := 4
	if s.IsGlobal {
		// Skip repo selection (index 1) if global
		if s.InputIndex == 0 {
			s.InputIndex = 2
		} else if s.InputIndex < maxIndex {
			s.InputIndex++
		}
	} else {
		if s.InputIndex < maxIndex {
			s.InputIndex++
		}
	}

	s.focusInput()
}

func (s *AddMCPServerState) retreatInput() {
	s.blurAllInputs()

	if s.IsGlobal {
		// Skip repo selection (index 1) if global
		if s.InputIndex == 2 {
			s.InputIndex = 0
		} else if s.InputIndex > 0 {
			s.InputIndex--
		}
	} else {
		if s.InputIndex > 0 {
			s.InputIndex--
		}
	}

	s.focusInput()
}

func (s *AddMCPServerState) blurAllInputs() {
	s.NameInput.Blur()
	s.CmdInput.Blur()
	s.ArgsInput.Blur()
}

func (s *AddMCPServerState) focusInput() {
	switch s.InputIndex {
	case 2:
		s.NameInput.Focus()
	case 3:
		s.CmdInput.Focus()
	case 4:
		s.ArgsInput.Focus()
	}
}

// GetValues returns the server configuration values
func (s *AddMCPServerState) GetValues() (name, command, args, repoPath string, isGlobal bool) {
	name = s.NameInput.Value()
	command = s.CmdInput.Value()
	args = s.ArgsInput.Value()
	isGlobal = s.IsGlobal
	if !isGlobal && len(s.Repos) > 0 && s.RepoIndex < len(s.Repos) {
		repoPath = s.Repos[s.RepoIndex]
	}
	return
}

// NewAddMCPServerState creates a new AddMCPServerState
func NewAddMCPServerState(repos []string) *AddMCPServerState {
	nameInput := textinput.New()
	nameInput.Placeholder = "server-name"
	nameInput.CharLimit = 50
	nameInput.SetWidth(ModalInputWidth)
	nameInput.Focus()

	cmdInput := textinput.New()
	cmdInput.Placeholder = "npx"
	cmdInput.CharLimit = 100
	cmdInput.SetWidth(ModalInputWidth)

	argsInput := textinput.New()
	argsInput.Placeholder = "@modelcontextprotocol/server-github"
	argsInput.CharLimit = 200
	argsInput.SetWidth(ModalInputWidth)

	return &AddMCPServerState{
		IsGlobal:   true,
		Repos:      repos,
		RepoIndex:  0,
		NameInput:  nameInput,
		CmdInput:   cmdInput,
		ArgsInput:  argsInput,
		InputIndex: 0,
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
