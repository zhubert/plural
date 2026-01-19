package modals

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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
	return "up/down navigate  a: add  d: delete  Esc: close"
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
					Render(TruncatePath(server.RepoPath, 40)+":") + "\n"
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
				Render(TruncateString(server.Command+" "+server.Args, 35))
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
		globalPrefix = "* "
	}
	globalOpt := globalStyle.Render(globalPrefix + "Global")

	repoStyle := SidebarItemStyle
	repoPrefix := "  "
	if s.InputIndex == 0 && !s.IsGlobal {
		repoStyle = SidebarSelectedStyle
		repoPrefix = "> "
	} else if !s.IsGlobal {
		repoPrefix = "* "
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
				prefix = "* "
			}
			repoList += style.Render(prefix+TruncatePath(repo, 40)) + "\n"
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
			s.AdvanceInput()
			return s, nil
		case "shift+tab", "up":
			s.RetreatInput()
			return s, nil
		case "space":
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

// AdvanceInput moves focus to the next input field
func (s *AddMCPServerState) AdvanceInput() {
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

// RetreatInput moves focus to the previous input field
func (s *AddMCPServerState) RetreatInput() {
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
	nameInput.CharLimit = MCPServerNameCharLimit
	nameInput.SetWidth(ModalInputWidth)
	nameInput.Focus()

	cmdInput := textinput.New()
	cmdInput.Placeholder = "npx"
	cmdInput.CharLimit = MCPCommandCharLimit
	cmdInput.SetWidth(ModalInputWidth)

	argsInput := textinput.New()
	argsInput.Placeholder = "@modelcontextprotocol/server-github"
	argsInput.CharLimit = MCPArgsCharLimit
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
