package modals

import (
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
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
		case keys.Up, "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case keys.Down, "j":
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
	// Bound form values
	scope    string // "global" or "per-repo"
	repoPath string
	name     string
	command  string
	args     string

	repos       []string
	form        *huh.Form
	initialized bool
}

func (*AddMCPServerState) modalState() {}

func (s *AddMCPServerState) Title() string { return "Add MCP Server" }

func (s *AddMCPServerState) Help() string {
	return "Tab: next  Enter: save  Esc: cancel"
}

func (s *AddMCPServerState) Render() string {
	title := ModalTitleStyle.Render(s.Title())
	help := ModalHelpStyle.Render(s.Help())
	return lipgloss.JoinVertical(lipgloss.Left, title, s.form.View(), help)
}

func (s *AddMCPServerState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, &s.initialized, msg)
	return s, cmd
}

// GetValues returns the server configuration values
func (s *AddMCPServerState) GetValues() (name, command, args, repoPath string, isGlobal bool) {
	name = s.name
	command = s.command
	args = s.args
	isGlobal = s.scope == "global"
	if !isGlobal {
		repoPath = s.repoPath
	}
	return
}

// NewAddMCPServerState creates a new AddMCPServerState
func NewAddMCPServerState(repos []string) *AddMCPServerState {
	s := &AddMCPServerState{
		scope: "global",
		repos: repos,
	}

	// Build repo options for the select field
	repoOptions := make([]huh.Option[string], len(repos))
	for i, repo := range repos {
		repoOptions[i] = huh.NewOption(TruncatePath(repo, 40), repo)
	}
	if len(repos) > 0 {
		s.repoPath = repos[0]
	}

	fields := []huh.Field{
		huh.NewSelect[string]().
			Title("Scope").
			Options(
				huh.NewOption("Global", "global"),
				huh.NewOption("Per-repository", "per-repo"),
			).
			Value(&s.scope),
	}

	// Add repo selector if repos available
	repoGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Repository").
			Options(repoOptions...).
			Value(&s.repoPath),
	).WithHideFunc(func() bool {
		return s.scope == "global" || len(repos) == 0
	})

	inputGroup := huh.NewGroup(
		huh.NewInput().
			Title("Name").
			Placeholder("server-name").
			CharLimit(MCPServerNameCharLimit).
			Value(&s.name),
		huh.NewInput().
			Title("Command").
			Placeholder("npx").
			CharLimit(MCPCommandCharLimit).
			Value(&s.command),
		huh.NewInput().
			Title("Args").
			Placeholder("@modelcontextprotocol/server-github").
			CharLimit(MCPArgsCharLimit).
			Value(&s.args),
	)

	s.form = huh.NewForm(
		huh.NewGroup(fields...),
		repoGroup,
		inputGroup,
	).WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(ModalInputWidth).
		WithLayout(huh.LayoutStack)

	s.initialized = true
	initHuhForm(s.form)
	return s
}
