package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ModalType represents the type of modal
type ModalType int

const (
	ModalNone ModalType = iota
	ModalAddRepo
	ModalNewSession
	ModalConfirmDelete
	ModalMerge
	ModalMCPServers
	ModalAddMCPServer
)

// Modal represents a popup dialog
type Modal struct {
	Type        ModalType
	input       textinput.Model
	title       string
	help        string
	error       string
	width       int
	height      int
	repoOptions []string // For session creation, list of repos
	repoIndex   int      // Selected repo index

	// Merge modal fields
	mergeOptions   []string // Available merge options
	mergeIndex     int      // Selected option index
	hasRemote      bool     // Whether remote origin exists
	changesSummary string   // Summary of uncommitted changes

	// Add repo modal fields
	suggestedRepo    string // Current directory if it's a git repo and not already added
	useSuggestedRepo bool   // Whether the suggestion is selected (vs text input)

	// Delete modal fields
	deleteOptions []string // Delete options (keep/delete worktree)
	deleteIndex   int      // Selected delete option index

	// New session modal fields
	branchInput      textinput.Model // Optional branch name input
	newSessionFocus  int             // 0=repo list, 1=branch input

	// MCP server modal fields
	mcpServers     []MCPServerDisplay // Flattened list of all servers for display
	mcpServerIndex int                // Selected server index
	mcpIsGlobal    bool               // Add modal: true for global, false for per-repo
	mcpRepos       []string           // Available repos for per-repo selection
	mcpRepoIndex   int                // Selected repo index in add modal
	mcpNameInput   textinput.Model    // Name input field
	mcpCmdInput    textinput.Model    // Command input field
	mcpArgsInput   textinput.Model    // Args input field
	mcpInputIndex  int                // Which input is focused (0=scope, 1=repo, 2=name, 3=cmd, 4=args)
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
	ti := textinput.New()
	ti.Placeholder = "Enter path..."
	ti.CharLimit = ModalInputCharLimit
	ti.SetWidth(ModalInputWidth)

	// MCP name input
	nameInput := textinput.New()
	nameInput.Placeholder = "server-name"
	nameInput.CharLimit = 50
	nameInput.SetWidth(ModalInputWidth)

	// MCP command input
	cmdInput := textinput.New()
	cmdInput.Placeholder = "npx"
	cmdInput.CharLimit = 100
	cmdInput.SetWidth(ModalInputWidth)

	// MCP args input
	argsInput := textinput.New()
	argsInput.Placeholder = "@modelcontextprotocol/server-github"
	argsInput.CharLimit = 200
	argsInput.SetWidth(ModalInputWidth)

	// Branch name input
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = 100
	branchInput.SetWidth(ModalInputWidth)

	return &Modal{
		Type:         ModalNone,
		input:        ti,
		branchInput:  branchInput,
		mcpNameInput: nameInput,
		mcpCmdInput:  cmdInput,
		mcpArgsInput: argsInput,
	}
}

// SetSuggestedRepo sets the suggested repo for the add repo modal
func (m *Modal) SetSuggestedRepo(path string) {
	m.suggestedRepo = path
	m.useSuggestedRepo = path != ""
}

// Show shows a modal of the specified type
func (m *Modal) Show(t ModalType) {
	m.Type = t
	m.error = ""
	m.input.Reset()

	switch t {
	case ModalAddRepo:
		m.title = "Add Repository"
		m.input.Placeholder = "/path/to/repo"
		if m.suggestedRepo != "" {
			m.useSuggestedRepo = true
			m.input.Blur()
			m.help = "↑/↓ to switch, Enter to confirm, Esc to cancel"
		} else {
			m.useSuggestedRepo = false
			m.input.Focus()
			m.help = "Enter the full path to a git repository"
		}
	case ModalNewSession:
		m.title = "New Session"
		m.help = "↑/↓ select repo  Tab: branch name  Enter: create"
		m.repoIndex = 0
		m.newSessionFocus = 0
		m.branchInput.Reset()
		m.branchInput.Blur()
	case ModalConfirmDelete:
		m.title = "Delete Session?"
		m.help = "↑/↓ to select, Enter to confirm, Esc to cancel"
		m.deleteOptions = []string{"Keep worktree", "Delete worktree"}
		m.deleteIndex = 0
	case ModalMerge:
		m.title = "Merge/PR"
		m.help = "↑/↓ to select, Enter to confirm, Esc to cancel"
		m.mergeIndex = 0
	case ModalMCPServers:
		m.title = "MCP Servers"
		m.help = "↑/↓ navigate  a: add  d: delete  Esc: close"
		m.mcpServerIndex = 0
	case ModalAddMCPServer:
		m.title = "Add MCP Server"
		m.help = "Tab: next  Enter: save  Esc: cancel"
		m.mcpIsGlobal = true
		m.mcpRepoIndex = 0
		m.mcpInputIndex = 0
		m.mcpNameInput.Reset()
		m.mcpCmdInput.Reset()
		m.mcpArgsInput.Reset()
		m.mcpNameInput.Focus()
	}
}

// Hide hides the modal
func (m *Modal) Hide() {
	m.Type = ModalNone
	m.error = ""
	m.input.Blur()
}

// IsVisible returns whether the modal is visible
func (m *Modal) IsVisible() bool {
	return m.Type != ModalNone
}

// SetError sets an error message
func (m *Modal) SetError(err string) {
	m.error = err
}

// GetInput returns the current input value
func (m *Modal) GetInput() string {
	return m.input.Value()
}

// SetRepoOptions sets the available repos for session creation
func (m *Modal) SetRepoOptions(repos []string) {
	m.repoOptions = repos
	m.repoIndex = 0
}

// GetSelectedRepo returns the selected repo for session creation
func (m *Modal) GetSelectedRepo() string {
	if len(m.repoOptions) == 0 {
		return ""
	}
	if m.repoIndex >= len(m.repoOptions) {
		m.repoIndex = 0
	}
	return m.repoOptions[m.repoIndex]
}

// GetBranchName returns the custom branch name entered by user (empty if not specified)
func (m *Modal) GetBranchName() string {
	return m.branchInput.Value()
}

// SetMergeOptions sets the available merge options based on remote availability
func (m *Modal) SetMergeOptions(hasRemote bool, changesSummary string) {
	m.hasRemote = hasRemote
	m.changesSummary = changesSummary
	m.mergeOptions = []string{"Merge to main"}
	if hasRemote {
		m.mergeOptions = append(m.mergeOptions, "Create PR")
	}
	m.mergeIndex = 0
}

// GetSelectedMergeOption returns the selected merge option
func (m *Modal) GetSelectedMergeOption() string {
	if len(m.mergeOptions) == 0 {
		return ""
	}
	if m.mergeIndex >= len(m.mergeOptions) {
		m.mergeIndex = 0
	}
	return m.mergeOptions[m.mergeIndex]
}

// ShouldDeleteWorktree returns true if user selected to delete the worktree
func (m *Modal) ShouldDeleteWorktree() bool {
	return m.deleteIndex == 1 // "Delete worktree" is index 1
}

// ShowMCPServers shows the MCP server list modal
func (m *Modal) ShowMCPServers(globalServers []MCPServerDisplay, perRepoServers map[string][]MCPServerDisplay, repos []string) {
	m.Show(ModalMCPServers)

	// Build flattened list for navigation
	m.mcpServers = nil
	for _, s := range globalServers {
		m.mcpServers = append(m.mcpServers, s)
	}
	for _, repo := range repos {
		for _, s := range perRepoServers[repo] {
			m.mcpServers = append(m.mcpServers, s)
		}
	}
	m.mcpRepos = repos
	m.mcpServerIndex = 0
}

// ShowAddMCPServer shows the add MCP server modal
func (m *Modal) ShowAddMCPServer(repos []string) {
	m.Show(ModalAddMCPServer)
	m.mcpRepos = repos
}

// GetNewMCPServer returns the MCP server info from add modal
// Returns: name, command, args, repoPath (empty if global), isGlobal
func (m *Modal) GetNewMCPServer() (name, command, args, repoPath string, isGlobal bool) {
	name = m.mcpNameInput.Value()
	command = m.mcpCmdInput.Value()
	args = m.mcpArgsInput.Value()
	isGlobal = m.mcpIsGlobal
	if !isGlobal && len(m.mcpRepos) > 0 && m.mcpRepoIndex < len(m.mcpRepos) {
		repoPath = m.mcpRepos[m.mcpRepoIndex]
	}
	return
}

// GetSelectedMCPServer returns the selected server for deletion
func (m *Modal) GetSelectedMCPServer() *MCPServerDisplay {
	if len(m.mcpServers) == 0 || m.mcpServerIndex >= len(m.mcpServers) {
		return nil
	}
	return &m.mcpServers[m.mcpServerIndex]
}

// Update handles messages
func (m *Modal) Update(msg tea.Msg) (*Modal, tea.Cmd) {
	if !m.IsVisible() {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.Type {
		case ModalNewSession:
			switch msg.String() {
			case "up", "k":
				if m.newSessionFocus == 0 && m.repoIndex > 0 {
					m.repoIndex--
				}
			case "down", "j":
				if m.newSessionFocus == 0 && m.repoIndex < len(m.repoOptions)-1 {
					m.repoIndex++
				}
			case "tab":
				// Toggle between repo list and branch input
				if m.newSessionFocus == 0 {
					m.newSessionFocus = 1
					m.branchInput.Focus()
				} else {
					m.newSessionFocus = 0
					m.branchInput.Blur()
				}
				return m, nil
			case "shift+tab":
				// Toggle back
				if m.newSessionFocus == 1 {
					m.newSessionFocus = 0
					m.branchInput.Blur()
				}
				return m, nil
			}
		case ModalConfirmDelete:
			switch msg.String() {
			case "up", "k":
				if m.deleteIndex > 0 {
					m.deleteIndex--
				}
			case "down", "j":
				if m.deleteIndex < len(m.deleteOptions)-1 {
					m.deleteIndex++
				}
			}
		case ModalMerge:
			switch msg.String() {
			case "up", "k":
				if m.mergeIndex > 0 {
					m.mergeIndex--
				}
			case "down", "j":
				if m.mergeIndex < len(m.mergeOptions)-1 {
					m.mergeIndex++
				}
			}
		case ModalMCPServers:
			switch msg.String() {
			case "up", "k":
				if m.mcpServerIndex > 0 {
					m.mcpServerIndex--
				}
			case "down", "j":
				if m.mcpServerIndex < len(m.mcpServers)-1 {
					m.mcpServerIndex++
				}
			}
		case ModalAddMCPServer:
			switch msg.String() {
			case "tab", "down":
				m.advanceMCPInput()
				return m, nil
			case "shift+tab", "up":
				m.retreatMCPInput()
				return m, nil
			}
		}
	}

	// Handle MCP add modal text input updates
	if m.Type == ModalAddMCPServer && m.mcpInputIndex >= 2 {
		var cmd tea.Cmd
		switch m.mcpInputIndex {
		case 2:
			m.mcpNameInput, cmd = m.mcpNameInput.Update(msg)
		case 3:
			m.mcpCmdInput, cmd = m.mcpCmdInput.Update(msg)
		case 4:
			m.mcpArgsInput, cmd = m.mcpArgsInput.Update(msg)
		}
		return m, cmd
	}

	// Handle New Session modal branch input updates
	if m.Type == ModalNewSession && m.newSessionFocus == 1 {
		var cmd tea.Cmd
		m.branchInput, cmd = m.branchInput.Update(msg)
		return m, cmd
	}

	if m.Type == ModalAddRepo {
		// Handle navigation between suggestion and text input
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && m.suggestedRepo != "" {
			switch keyMsg.String() {
			case "up", "down", "tab":
				m.useSuggestedRepo = !m.useSuggestedRepo
				if m.useSuggestedRepo {
					m.input.Blur()
				} else {
					m.input.Focus()
				}
				return m, nil
			}
		}

		// Only update text input when it's focused
		if !m.useSuggestedRepo {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View renders the modal
func (m *Modal) View(screenWidth, screenHeight int) string {
	if !m.IsVisible() {
		return ""
	}

	var content string

	switch m.Type {
	case ModalAddRepo:
		content = m.renderAddRepo()
	case ModalNewSession:
		content = m.renderNewSession()
	case ModalConfirmDelete:
		content = m.renderConfirmDelete()
	case ModalMerge:
		content = m.renderMerge()
	case ModalMCPServers:
		content = m.renderMCPServers()
	case ModalAddMCPServer:
		content = m.renderAddMCPServer()
	}

	modal := ModalStyle.Render(content)

	// Center the modal
	modalWidth := lipgloss.Width(modal)
	modalHeight := lipgloss.Height(modal)

	x := (screenWidth - modalWidth) / 2
	y := (screenHeight - modalHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return lipgloss.Place(
		screenWidth, screenHeight,
		lipgloss.Center, lipgloss.Center,
		modal,
	)
}

func (m *Modal) renderAddRepo() string {
	title := ModalTitleStyle.Render(m.title)

	var content string

	// Show suggested repo if available
	if m.suggestedRepo != "" {
		suggestionLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("Current directory:")

		style := SidebarItemStyle
		prefix := "  "
		if m.useSuggestedRepo {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		suggestionItem := style.Render(prefix + m.suggestedRepo)

		otherLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Or enter a different path:")

		inputStyle := lipgloss.NewStyle()
		if !m.useSuggestedRepo {
			inputStyle = inputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			inputStyle = inputStyle.PaddingLeft(2)
		}
		inputView := inputStyle.Render(m.input.View())

		content = lipgloss.JoinVertical(lipgloss.Left, suggestionLabel, suggestionItem, otherLabel, inputView)
	} else {
		content = m.input.View()
	}

	var errView string
	if m.error != "" {
		errView = "\n" + StatusErrorStyle.Render(m.error)
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, content, errView, help)
}

// GetAddRepoPath returns the path to add (either suggested or from input)
func (m *Modal) GetAddRepoPath() string {
	if m.suggestedRepo != "" && m.useSuggestedRepo {
		return m.suggestedRepo
	}
	return m.input.Value()
}

func (m *Modal) renderNewSession() string {
	title := ModalTitleStyle.Render(m.title)

	// Repository selection section
	repoLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Repository:")

	var repoList string
	if len(m.repoOptions) == 0 {
		repoList = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No repositories added. Press 'r' to add one first.")
	} else {
		for i, repo := range m.repoOptions {
			style := SidebarItemStyle
			prefix := "  "
			if m.newSessionFocus == 0 && i == m.repoIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			} else if i == m.repoIndex {
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
	if m.newSessionFocus == 1 {
		branchInputStyle = branchInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		branchInputStyle = branchInputStyle.PaddingLeft(2)
	}
	branchView := branchInputStyle.Render(m.branchInput.View())

	var errView string
	if m.error != "" {
		errView = "\n" + StatusErrorStyle.Render(m.error)
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoList, branchLabel, branchView, errView, help)
}

func (m *Modal) renderConfirmDelete() string {
	title := ModalTitleStyle.Render(m.title)

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		MarginBottom(1).
		Render("This will remove the session from the list.")

	var optionList string
	for i, opt := range m.deleteOptions {
		style := SidebarItemStyle
		prefix := "  "
		if i == m.deleteIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, message, optionList, help)
}

func (m *Modal) renderMerge() string {
	title := ModalTitleStyle.Render(m.title)

	// Show changes summary
	var summarySection string
	if m.changesSummary != "" {
		summaryStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)
		summarySection = summaryStyle.Render("Changes: " + m.changesSummary)
	} else {
		noChangesStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginBottom(1)
		summarySection = noChangesStyle.Render("No uncommitted changes")
	}

	var optionList string
	for i, opt := range m.mergeOptions {
		style := SidebarItemStyle
		prefix := "  "
		if i == m.mergeIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	if !m.hasRemote {
		note := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("(No remote origin - PR option unavailable)")
		optionList += "\n" + note
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, summarySection, optionList, help)
}

func (m *Modal) renderMCPServers() string {
	title := ModalTitleStyle.Render(m.title)

	var content string
	if len(m.mcpServers) == 0 {
		content = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No MCP servers configured.\nPress 'a' to add one.")
	} else {
		// Group servers for display
		currentRepo := ""
		globalShown := false
		idx := 0

		for _, server := range m.mcpServers {
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
			if idx == m.mcpServerIndex {
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

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (m *Modal) renderAddMCPServer() string {
	title := ModalTitleStyle.Render(m.title)

	// Scope selector
	scopeLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Scope:")

	globalStyle := SidebarItemStyle
	globalPrefix := "  "
	if m.mcpInputIndex == 0 && m.mcpIsGlobal {
		globalStyle = SidebarSelectedStyle
		globalPrefix = "> "
	} else if m.mcpIsGlobal {
		globalPrefix = "● "
	}
	globalOpt := globalStyle.Render(globalPrefix + "Global")

	repoStyle := SidebarItemStyle
	repoPrefix := "  "
	if m.mcpInputIndex == 0 && !m.mcpIsGlobal {
		repoStyle = SidebarSelectedStyle
		repoPrefix = "> "
	} else if !m.mcpIsGlobal {
		repoPrefix = "● "
	}
	repoOpt := repoStyle.Render(repoPrefix + "Per-repository")

	scopeSection := lipgloss.JoinVertical(lipgloss.Left, scopeLabel, globalOpt, repoOpt)

	// Repo selector (only if per-repo)
	var repoSection string
	if !m.mcpIsGlobal && len(m.mcpRepos) > 0 {
		repoLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Repository:")

		var repoList string
		for i, repo := range m.mcpRepos {
			style := SidebarItemStyle
			prefix := "  "
			if m.mcpInputIndex == 1 && i == m.mcpRepoIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			} else if i == m.mcpRepoIndex {
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

	nameLabel := inputLabel("Name:", m.mcpInputIndex == 2)
	nameInput := m.mcpNameInput.View()

	cmdLabel := inputLabel("Command:", m.mcpInputIndex == 3)
	cmdInput := m.mcpCmdInput.View()

	argsLabel := inputLabel("Args:", m.mcpInputIndex == 4)
	argsInput := m.mcpArgsInput.View()

	inputSection := lipgloss.JoinVertical(lipgloss.Left,
		nameLabel, nameInput,
		cmdLabel, cmdInput,
		argsLabel, argsInput,
	)

	help := ModalHelpStyle.Render(m.help)

	if repoSection != "" {
		return lipgloss.JoinVertical(lipgloss.Left, title, scopeSection, repoSection, inputSection, help)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, scopeSection, inputSection, help)
}

// advanceMCPInput moves to the next input field
func (m *Modal) advanceMCPInput() {
	m.blurAllMCPInputs()

	maxIndex := 4
	if m.mcpIsGlobal {
		// Skip repo selection (index 1) if global
		if m.mcpInputIndex == 0 {
			m.mcpInputIndex = 2
		} else if m.mcpInputIndex < maxIndex {
			m.mcpInputIndex++
		}
	} else {
		if m.mcpInputIndex < maxIndex {
			m.mcpInputIndex++
		}
	}

	m.focusMCPInput()
}

// retreatMCPInput moves to the previous input field
func (m *Modal) retreatMCPInput() {
	m.blurAllMCPInputs()

	if m.mcpIsGlobal {
		// Skip repo selection (index 1) if global
		if m.mcpInputIndex == 2 {
			m.mcpInputIndex = 0
		} else if m.mcpInputIndex > 0 {
			m.mcpInputIndex--
		}
	} else {
		if m.mcpInputIndex > 0 {
			m.mcpInputIndex--
		}
	}

	m.focusMCPInput()
}

func (m *Modal) blurAllMCPInputs() {
	m.mcpNameInput.Blur()
	m.mcpCmdInput.Blur()
	m.mcpArgsInput.Blur()
}

func (m *Modal) focusMCPInput() {
	switch m.mcpInputIndex {
	case 2:
		m.mcpNameInput.Focus()
	case 3:
		m.mcpCmdInput.Focus()
	case 4:
		m.mcpArgsInput.Focus()
	}
}

// ToggleMCPScope toggles between global and per-repo in add modal
func (m *Modal) ToggleMCPScope() {
	if m.mcpInputIndex == 0 {
		m.mcpIsGlobal = !m.mcpIsGlobal
	}
}

// MoveMCPRepoSelection moves repo selection up or down
func (m *Modal) MoveMCPRepoSelection(delta int) {
	if m.mcpInputIndex == 1 {
		newIdx := m.mcpRepoIndex + delta
		if newIdx >= 0 && newIdx < len(m.mcpRepos) {
			m.mcpRepoIndex = newIdx
		}
	}
}

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
