package modals

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// Plugin Display Types
// =============================================================================

// MarketplaceDisplay represents a marketplace for display in the modal
type MarketplaceDisplay struct {
	Name        string
	Source      string // "github" or "url"
	Repo        string
	LastUpdated string
}

// PluginDisplay represents a plugin for display in the modal
type PluginDisplay struct {
	Name        string
	Marketplace string
	FullName    string
	Description string
	Status      string // "available", "installed", "enabled"
	Version     string
}

// =============================================================================
// PluginsState - State for the Plugins modal with tabs
// =============================================================================

// Tab constants
const (
	TabMarketplaces = iota
	TabInstalled
	TabDiscover
)

type PluginsState struct {
	ActiveTab     int
	Marketplaces  []MarketplaceDisplay
	Plugins       []PluginDisplay
	SelectedIndex int
	ScrollOffset  int // For keeping selection visible
	Loading       bool
	Error         string
	SearchInput   textinput.Model
	SearchQuery   string
	SearchFocused bool // true = search box focused, false = list focused
}

func (*PluginsState) modalState() {}

func (s *PluginsState) Title() string { return "Plugins" }

func (s *PluginsState) Help() string {
	switch s.ActiveTab {
	case TabMarketplaces:
		return "Tab/←/→ tabs  ↑/↓ navigate  a: add  d: delete  u: update  Esc: close"
	case TabInstalled:
		return "Tab/←/→ tabs  ↑/↓ navigate  e: enable/disable  u: uninstall  Esc: close"
	case TabDiscover:
		if s.SearchFocused {
			return "Esc: exit search  ↑/↓ navigate  Enter: install"
		}
		return "Tab/←/→ tabs  /: search  ↑/↓ navigate  Enter: install  Esc: close"
	default:
		return "Tab/←/→ tabs  ↑/↓ navigate  Esc: close"
	}
}

// MaxContentHeight is the maximum height for the scrollable content area
const MaxContentHeight = 12

func (s *PluginsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Render tabs
	tabs := s.renderTabs()

	// Render content based on active tab
	var content string
	if s.Loading {
		content = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("Loading...")
	} else if s.Error != "" {
		content = StatusErrorStyle.Render(s.Error)
	} else {
		switch s.ActiveTab {
		case TabMarketplaces:
			content = s.renderMarketplaces()
		case TabInstalled:
			content = s.renderInstalledPlugins()
		case TabDiscover:
			content = s.renderDiscoverPlugins()
		}
	}

	// Constrain content height
	content = lipgloss.NewStyle().
		MaxHeight(MaxContentHeight).
		Render(content)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, tabs, content, help)
}

func (s *PluginsState) renderTabs() string {
	tabNames := []string{"Marketplaces", "Installed", "Discover"}

	var tabs []string
	for i, name := range tabNames {
		style := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Padding(0, 1)
		if i == s.ActiveTab {
			style = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Padding(0, 1).
				Underline(true)
		}
		tabs = append(tabs, style.Render(name))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + "\n"
}

func (s *PluginsState) renderMarketplaces() string {
	if len(s.Marketplaces) == 0 {
		return lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No marketplaces configured.\nPress 'a' to add one.")
	}

	var content string
	for i, m := range s.Marketplaces {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		info := m.Name
		if m.Source != "" {
			info += "  " + lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render("("+m.Source+")")
		}
		if m.Repo != "" {
			info += "  " + lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render(m.Repo)
		}
		content += style.Render(prefix+info) + "\n"
	}

	return content
}

func (s *PluginsState) renderInstalledPlugins() string {
	installed := s.getInstalledPlugins()
	if len(installed) == 0 {
		return lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No plugins installed.\nGo to 'Available' tab to install plugins.")
	}

	var content string
	for i, p := range installed {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		statusIcon := "○"
		if p.Status == "enabled" {
			statusIcon = "●"
		}

		info := statusIcon + " " + p.Name
		if p.Marketplace != "" {
			info += "  " + lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render("@"+p.Marketplace)
		}
		content += style.Render(prefix+info) + "\n"
	}

	return content
}

// MaxVisibleItems is the number of items visible in the scrollable list
const MaxVisibleItems = 5

func (s *PluginsState) renderDiscoverPlugins() string {
	// Render search input with focus indicator
	searchLabelStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if s.SearchFocused {
		searchLabelStyle = searchLabelStyle.Foreground(ColorPrimary)
	}
	searchLabel := searchLabelStyle.Render("Search: ")
	searchBox := searchLabel + s.SearchInput.View() + "\n\n"

	available := s.getFilteredAvailablePlugins()
	if len(available) == 0 {
		msg := "No plugins found."
		if s.SearchQuery == "" && len(s.Marketplaces) == 0 {
			msg = "No plugins available.\nAdd a marketplace first to see plugins."
		} else if s.SearchQuery != "" {
			msg = "No plugins match your search."
		}
		return searchBox + lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render(msg)
	}

	// Apply scroll offset to show visible items
	startIdx := s.ScrollOffset
	endIdx := startIdx + MaxVisibleItems
	if endIdx > len(available) {
		endIdx = len(available)
	}

	var content string

	// Show scroll indicator at top if needed
	if startIdx > 0 {
		content += lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ↑ more above") + "\n"
	}

	for i := startIdx; i < endIdx; i++ {
		p := available[i]
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		info := p.Name
		if p.Marketplace != "" {
			info += "  " + lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render("@"+p.Marketplace)
		}
		if p.Description != "" {
			info += "\n    " + lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render(TruncateString(p.Description, 50))
		}
		content += style.Render(prefix+info) + "\n"
	}

	// Show scroll indicator at bottom if needed
	if endIdx < len(available) {
		content += lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ↓ more below") + "\n"
	}

	return searchBox + content
}

func (s *PluginsState) getInstalledPlugins() []PluginDisplay {
	var installed []PluginDisplay
	for _, p := range s.Plugins {
		if p.Status == "installed" || p.Status == "enabled" {
			installed = append(installed, p)
		}
	}
	return installed
}

func (s *PluginsState) getAvailablePlugins() []PluginDisplay {
	var available []PluginDisplay
	for _, p := range s.Plugins {
		if p.Status == "available" {
			available = append(available, p)
		}
	}
	return available
}

func (s *PluginsState) getFilteredAvailablePlugins() []PluginDisplay {
	available := s.getAvailablePlugins()
	if s.SearchQuery == "" {
		return available
	}

	query := strings.ToLower(s.SearchQuery)
	var filtered []PluginDisplay
	for _, p := range available {
		// Search in name, marketplace, and description
		if strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.Marketplace), query) ||
			strings.Contains(strings.ToLower(p.Description), query) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func (s *PluginsState) getCurrentListLength() int {
	switch s.ActiveTab {
	case TabMarketplaces:
		return len(s.Marketplaces)
	case TabInstalled:
		return len(s.getInstalledPlugins())
	case TabDiscover:
		return len(s.getFilteredAvailablePlugins())
	default:
		return 0
	}
}

func (s *PluginsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	key := keyMsg.String()

	// When search is focused, handle special keys and send rest to input
	if s.ActiveTab == TabDiscover && s.SearchFocused {
		switch key {
		case keys.Escape:
			// Exit search mode
			s.SearchFocused = false
			s.SearchInput.Blur()
			return s, nil
		case keys.Up, keys.Down:
			// Allow up/down to navigate list even when search focused
			s.handleNavigation(key)
			return s, nil
		default:
			// Send all other keys to search input
			var cmd tea.Cmd
			s.SearchInput, cmd = s.SearchInput.Update(msg)
			newQuery := s.SearchInput.Value()
			if newQuery != s.SearchQuery {
				s.SearchQuery = newQuery
				s.SelectedIndex = 0
				s.ScrollOffset = 0
			}
			return s, cmd
		}
	}

	// Standard navigation when search is not focused
	switch key {
	case keys.Tab:
		// Tab cycles through tabs
		s.ActiveTab = (s.ActiveTab + 1) % 3
		s.SelectedIndex = 0
		s.ScrollOffset = 0
	case keys.Left, "h":
		if s.ActiveTab > 0 {
			s.ActiveTab--
			s.SelectedIndex = 0
			s.ScrollOffset = 0
		}
	case keys.Right, "l":
		if s.ActiveTab < TabDiscover {
			s.ActiveTab++
			s.SelectedIndex = 0
			s.ScrollOffset = 0
		}
	case "1":
		s.ActiveTab = TabMarketplaces
		s.SelectedIndex = 0
		s.ScrollOffset = 0
	case "2":
		s.ActiveTab = TabInstalled
		s.SelectedIndex = 0
		s.ScrollOffset = 0
	case "3":
		s.ActiveTab = TabDiscover
		s.SelectedIndex = 0
		s.ScrollOffset = 0
	case "/":
		// Enter search mode on Discover tab
		if s.ActiveTab == TabDiscover {
			s.SearchFocused = true
			s.SearchInput.Focus()
		}
	case keys.Up, "k", keys.Down, "j":
		s.handleNavigation(key)
	}
	return s, nil
}

// handleNavigation handles up/down navigation and scroll offset
func (s *PluginsState) handleNavigation(key string) {
	listLen := s.getCurrentListLength()
	if listLen == 0 {
		return
	}

	switch key {
	case keys.Up, "k":
		if s.SelectedIndex > 0 {
			s.SelectedIndex--
			// Adjust scroll if selection is above visible area
			if s.SelectedIndex < s.ScrollOffset {
				s.ScrollOffset = s.SelectedIndex
			}
		}
	case keys.Down, "j":
		if s.SelectedIndex < listLen-1 {
			s.SelectedIndex++
			// Adjust scroll if selection is below visible area
			if s.SelectedIndex >= s.ScrollOffset+MaxVisibleItems {
				s.ScrollOffset = s.SelectedIndex - MaxVisibleItems + 1
			}
		}
	}
}

// GetSelectedMarketplace returns the selected marketplace (for delete/update)
func (s *PluginsState) GetSelectedMarketplace() *MarketplaceDisplay {
	if s.ActiveTab != TabMarketplaces || len(s.Marketplaces) == 0 || s.SelectedIndex >= len(s.Marketplaces) {
		return nil
	}
	return &s.Marketplaces[s.SelectedIndex]
}

// GetSelectedInstalledPlugin returns the selected installed plugin (for enable/disable/uninstall)
func (s *PluginsState) GetSelectedInstalledPlugin() *PluginDisplay {
	if s.ActiveTab != TabInstalled {
		return nil
	}
	installed := s.getInstalledPlugins()
	if len(installed) == 0 || s.SelectedIndex >= len(installed) {
		return nil
	}
	return &installed[s.SelectedIndex]
}

// GetSelectedAvailablePlugin returns the selected available plugin (for install)
func (s *PluginsState) GetSelectedAvailablePlugin() *PluginDisplay {
	if s.ActiveTab != TabDiscover {
		return nil
	}
	available := s.getFilteredAvailablePlugins()
	if len(available) == 0 || s.SelectedIndex >= len(available) {
		return nil
	}
	return &available[s.SelectedIndex]
}

// NewPluginsState creates a new PluginsState with loading state
func NewPluginsState() *PluginsState {
	searchInput := textinput.New()
	searchInput.Placeholder = "filter plugins..."
	searchInput.CharLimit = PluginSearchCharLimit
	searchInput.SetWidth(30)
	// Don't focus by default - Tab toggles focus

	return &PluginsState{
		ActiveTab:     TabMarketplaces,
		Marketplaces:  []MarketplaceDisplay{},
		Plugins:       []PluginDisplay{},
		SelectedIndex: 0,
		ScrollOffset:  0,
		Loading:       true,
		SearchInput:   searchInput,
		SearchFocused: false,
	}
}

// NewPluginsStateWithData creates a new PluginsState with pre-loaded data
func NewPluginsStateWithData(marketplaces []MarketplaceDisplay, plugins []PluginDisplay) *PluginsState {
	searchInput := textinput.New()
	searchInput.Placeholder = "filter plugins..."
	searchInput.CharLimit = PluginSearchCharLimit
	searchInput.SetWidth(30)
	// Don't focus by default - Tab toggles focus

	return &PluginsState{
		ActiveTab:     TabMarketplaces,
		Marketplaces:  marketplaces,
		Plugins:       plugins,
		SelectedIndex: 0,
		ScrollOffset:  0,
		Loading:       false,
		SearchInput:   searchInput,
		SearchFocused: false,
	}
}

// SetError sets an error message
func (s *PluginsState) SetError(err string) {
	s.Error = err
	s.Loading = false
}

// SetData sets the marketplaces and plugins data
func (s *PluginsState) SetData(marketplaces []MarketplaceDisplay, plugins []PluginDisplay) {
	s.Marketplaces = marketplaces
	s.Plugins = plugins
	s.Loading = false
	s.Error = ""
}

// =============================================================================
// AddMarketplaceState - State for the Add Marketplace modal
// =============================================================================

type AddMarketplaceState struct {
	form   *huh.Form
	source string
}

func (*AddMarketplaceState) modalState() {}

func (s *AddMarketplaceState) Title() string { return "Add Marketplace" }

func (s *AddMarketplaceState) Help() string {
	return "Enter: add  Esc: cancel"
}

func (s *AddMarketplaceState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	example := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("e.g., anthropics/claude-code-plugins")

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, s.form.View(), example, help)
}

func (s *AddMarketplaceState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, msg)
	return s, cmd
}

// GetValue returns the source input value
func (s *AddMarketplaceState) GetValue() string {
	return s.source
}

// NewAddMarketplaceState creates a new AddMarketplaceState
func NewAddMarketplaceState() *AddMarketplaceState {
	s := &AddMarketplaceState{}
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("GitHub repo or URL").
				Placeholder("owner/repo or https://...").
				CharLimit(MarketplaceSourceCharLimit).
				Value(&s.source),
		),
	).WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(ModalInputWidth)

	initHuhForm(s.form)
	return s
}
