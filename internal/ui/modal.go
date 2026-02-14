// Package ui provides the modal dialog component.
// Modal state types have been extracted to the modals subpackage for better organization.
package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/ui/modals"
)

// Re-export types from modals package for backwards compatibility
type (
	ModalState               = modals.ModalState
	MCPServerDisplay         = modals.MCPServerDisplay
	MarketplaceDisplay       = modals.MarketplaceDisplay
	PluginDisplay            = modals.PluginDisplay
	ChangelogEntry           = modals.ChangelogEntry
	OptionItem               = modals.OptionItem
	IssueItem                = modals.IssueItem
	HelpShortcut             = modals.HelpShortcut
	HelpShortcutTriggeredMsg = modals.HelpShortcutTriggeredMsg
	HelpSection              = modals.HelpSection
	SearchResult             = modals.SearchResult
	RepoItem                 = modals.RepoItem
	ReviewCommentItem        = modals.ReviewCommentItem

	AddRepoState             = modals.AddRepoState
	SelectRepoForIssuesState = modals.SelectRepoForIssuesState
	NewSessionState          = modals.NewSessionState
	ForkSessionState         = modals.ForkSessionState
	RenameSessionState       = modals.RenameSessionState
	MergeState               = modals.MergeState
	LoadingCommitState       = modals.LoadingCommitState
	EditCommitState          = modals.EditCommitState
	MergeConflictState       = modals.MergeConflictState
	ConfirmDeleteState       = modals.ConfirmDeleteState
	ConfirmDeleteRepoState   = modals.ConfirmDeleteRepoState
	ConfirmExitState         = modals.ConfirmExitState
	MCPServersState          = modals.MCPServersState
	AddMCPServerState        = modals.AddMCPServerState
	PluginsState             = modals.PluginsState
	AddMarketplaceState      = modals.AddMarketplaceState
	WelcomeState             = modals.WelcomeState
	ChangelogState           = modals.ChangelogState
	SettingsState            = modals.SettingsState
	ImportIssuesState        = modals.ImportIssuesState
	SelectIssueSourceState   = modals.SelectIssueSourceState
	IssueSource              = modals.IssueSource
	HelpState                = modals.HelpState
	ExploreOptionsState      = modals.ExploreOptionsState
	SearchMessagesState      = modals.SearchMessagesState
	PreviewActiveState       = modals.PreviewActiveState
	BroadcastState           = modals.BroadcastState
	BroadcastGroupState      = modals.BroadcastGroupState
	BroadcastGroupAction     = modals.BroadcastGroupAction
	SessionItem              = modals.SessionItem
	WorkspaceListState       = modals.WorkspaceListState
	NewWorkspaceState        = modals.NewWorkspaceState
	BulkActionState          = modals.BulkActionState
	BulkAction               = modals.BulkAction
	ReviewCommentsState      = modals.ReviewCommentsState
	ContainerBuildState      = modals.ContainerBuildState
	ContainerCommandState    = modals.ContainerCommandState
	AsanaProjectOption       = modals.AsanaProjectOption
	RepoSettingsState        = modals.RepoSettingsState
)

// Re-export broadcast action constants
const (
	BroadcastActionSendPrompt = modals.BroadcastActionSendPrompt
	BroadcastActionCreatePRs  = modals.BroadcastActionCreatePRs
)

// Re-export container constants
const ContainerAuthHelp = modals.ContainerAuthHelp

// Re-export bulk action constants
const (
	BulkActionDelete          = modals.BulkActionDelete
	BulkActionMoveToWorkspace = modals.BulkActionMoveToWorkspace
	BulkActionCreatePRs       = modals.BulkActionCreatePRs
	BulkActionSendPrompt      = modals.BulkActionSendPrompt
)

// Re-export constructor functions
var (
	NewAddRepoState                   = modals.NewAddRepoState
	NewSelectRepoForIssuesState       = modals.NewSelectRepoForIssuesState
	NewNewSessionState                = modals.NewNewSessionState
	NewForkSessionState               = modals.NewForkSessionState
	NewRenameSessionState             = modals.NewRenameSessionState
	NewMergeState                     = modals.NewMergeState
	NewLoadingCommitState             = modals.NewLoadingCommitState
	NewEditCommitState                = modals.NewEditCommitState
	NewMergeConflictState             = modals.NewMergeConflictState
	NewConfirmDeleteState             = modals.NewConfirmDeleteState
	NewConfirmDeleteRepoState         = modals.NewConfirmDeleteRepoState
	NewConfirmExitState               = modals.NewConfirmExitState
	NewMCPServersState                = modals.NewMCPServersState
	NewAddMCPServerState              = modals.NewAddMCPServerState
	NewPluginsState                   = modals.NewPluginsState
	NewPluginsStateWithData           = modals.NewPluginsStateWithData
	NewAddMarketplaceState            = modals.NewAddMarketplaceState
	NewWelcomeState                   = modals.NewWelcomeState
	NewChangelogState                 = modals.NewChangelogState
	NewImportIssuesState              = modals.NewImportIssuesState
	NewImportIssuesStateWithSource    = modals.NewImportIssuesStateWithSource
	NewSelectIssueSourceState         = modals.NewSelectIssueSourceState
	NewHelpStateFromSections          = modals.NewHelpStateFromSections
	NewExploreOptionsState            = modals.NewExploreOptionsState
	NewSearchMessagesState            = modals.NewSearchMessagesState
	NewPreviewActiveState             = modals.NewPreviewActiveState
	NewBroadcastState                 = modals.NewBroadcastState
	NewBroadcastGroupState            = modals.NewBroadcastGroupState
	NewWorkspaceListState             = modals.NewWorkspaceListState
	NewNewWorkspaceState              = modals.NewNewWorkspaceState
	NewRenameWorkspaceState           = modals.NewRenameWorkspaceState
	NewReviewCommentsState            = modals.NewReviewCommentsState
	NewContainerBuildState            = modals.NewContainerBuildState
	NewContainerCLINotInstalledState  = modals.NewContainerCLINotInstalledState
	NewContainerSystemNotRunningState = modals.NewContainerSystemNotRunningState
	ValidateContainerImage            = modals.ValidateContainerImage
	NewBulkActionState                = modals.NewBulkActionState
	SessionDisplayName                = modals.SessionDisplayName
	TruncatePath                      = modals.TruncatePath
	TruncateString                    = modals.TruncateString
	RenderSelectableList              = modals.RenderSelectableList
	RenderSelectableListWithFocus     = modals.RenderSelectableListWithFocus
	ExpandGlobToDirs                  = modals.ExpandGlobToDirs
	IsGlobPattern                     = modals.IsGlobPattern
)

// themeKeysAndNames returns parallel slices of theme keys and display names.
func themeKeysAndNames() ([]string, []string) {
	themes := ThemeNames()
	var themeKeys []string
	var themeDisplayNames []string
	for _, t := range themes {
		themeKeys = append(themeKeys, string(t))
		themeDisplayNames = append(themeDisplayNames, GetTheme(t).Name)
	}
	return themeKeys, themeDisplayNames
}

// NewSettingsState creates a new SettingsState with theme data injected automatically.
func NewSettingsState(currentBranchPrefix string, notificationsEnabled bool,
	containersSupported bool, containerImage string) *SettingsState {
	themeKeys, themeDisplayNames := themeKeysAndNames()
	currentTheme := string(CurrentThemeName())
	return modals.NewSettingsState(themeKeys, themeDisplayNames, currentTheme,
		currentBranchPrefix, notificationsEnabled,
		containersSupported, containerImage)
}

// NewRepoSettingsState creates a new RepoSettingsState for the given repo.
var NewRepoSettingsState = modals.NewRepoSettingsState

// GetSelectedSettingsTheme returns the selected theme from a SettingsState as a ThemeName.
func GetSelectedSettingsTheme(s *SettingsState) ThemeName {
	return ThemeName(s.GetSelectedTheme())
}

// Modal represents a popup dialog with type-safe state management.
// The State field is nil when no modal is visible.
type Modal struct {
	State ModalState
	error string
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

	// Use preferred width if the modal implements ModalWithPreferredWidth
	style := ModalStyle
	width := ModalWidth
	if modalWithWidth, ok := m.State.(modals.ModalWithPreferredWidth); ok {
		width = modalWithWidth.PreferredWidth()
	}

	// Clamp width to fit within the screen (account for border + padding overhead)
	// ModalStyle has Border (1 left + 1 right = 2) + Padding(1, 2) (2 left + 2 right = 4) = 6 total
	const modalHorizontalOverhead = 6
	if maxWidth := screenWidth - modalHorizontalOverhead; width > maxWidth {
		width = maxWidth
	}

	// Notify modal of its actual size before rendering (if it implements ModalWithSize)
	if modalWithSize, ok := m.State.(modals.ModalWithSize); ok {
		modalWithSize.SetSize(width, screenHeight)
	}

	content := m.State.Render()

	// Add error if present
	if m.error != "" {
		content += "\n" + StatusErrorStyle.Render(m.error)
	}

	style = style.Width(width)

	modal := style.Render(content)

	return lipgloss.Place(
		screenWidth, screenHeight,
		lipgloss.Center, lipgloss.Center,
		modal,
	)
}

// initModalStyles initializes the modal styles in the modals package.
// This should be called once at startup after the theme is loaded.
func initModalStyles() {
	modals.SetStyles(
		ModalTitleStyle,
		ModalHelpStyle,
		SidebarItemStyle,
		SidebarSelectedStyle,
		StatusErrorStyle,
		ColorPrimary,
		ColorSecondary,
		ColorText,
		ColorTextMuted,
		ColorTextInverse,
		ColorUser,
		ColorWarning,
		ModalInputWidth,
		ModalInputCharLimit,
		ModalWidth,
		ModalWidthWide,
	)
}

// initModalConstants initializes the modal constants in the modals package.
// This should be called once at startup.
func initModalConstants() {
	modals.SetConstants(modals.ModalConstants{
		HelpModalMaxVisible:        HelpModalMaxVisible,
		IssuesModalMaxVisible:      IssuesModalMaxVisible,
		SearchModalMaxVisible:      SearchModalMaxVisible,
		ChangelogModalMaxVisible:   ChangelogModalMaxVisible,
		BranchNameCharLimit:        BranchNameCharLimit,
		SessionNameCharLimit:       SessionNameCharLimit,
		SearchInputCharLimit:       SearchInputCharLimit,
		MCPServerNameCharLimit:     MCPServerNameCharLimit,
		MCPCommandCharLimit:        MCPCommandCharLimit,
		MCPArgsCharLimit:           MCPArgsCharLimit,
		PluginSearchCharLimit:      PluginSearchCharLimit,
		MarketplaceSourceCharLimit: MarketplaceSourceCharLimit,
		BranchPrefixCharLimit:      BranchPrefixCharLimit,
	})
}

// init ensures modal styles and constants are initialized
func init() {
	initModalStyles()
	initModalConstants()
}

// RefreshModalStyles refreshes the modal styles after a theme change.
// Call this after changing the theme to update the modals package styles.
func RefreshModalStyles() {
	initModalStyles()
}
